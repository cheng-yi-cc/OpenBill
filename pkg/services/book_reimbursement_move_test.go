package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestBookMoveReimbursementKeepsReceiptWithExpense(t *testing.T) {
	f, expense := reimbursementFixture(t)
	receipt, err := Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{
		ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID),
		Amount: "40", Time: f.at + 10, TimeZone: "UTC", RequestId: "book-move-reimbursement",
	})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "目标"})
	require.NoError(t, err)
	balance, receivable := f.balance(), reimbursementBalance(t, f)
	// A receipt cannot move alone, and an implicit receipt needs an actor whose
	// time restrictions allow the change. Both failures must be atomic.
	require.Error(t, Books.MoveTransactions(nil, f.uid, []int64{receipt.IncomeId}, target.Id, refundTestActor(f.uid)))
	require.Error(t, Books.MoveTransactions(nil, f.uid, []int64{expense.TransactionId}, target.Id))
	denied := refundTestActor(f.uid)
	denied.User.TransactionEditScope = models.TRANSACTION_EDIT_SCOPE_NONE
	require.Error(t, Books.MoveTransactions(nil, f.uid, []int64{expense.TransactionId}, target.Id, denied))
	var before models.Transaction
	found, err := f.engine.ID(expense.TransactionId).Get(&before)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, expense.BookId, before.BookId)
	require.NoError(t, Books.MoveTransactions(nil, f.uid, []int64{expense.TransactionId}, target.Id, refundTestActor(f.uid)))
	var savedExpense, savedIncome models.Transaction
	found, err = f.engine.ID(expense.TransactionId).Get(&savedExpense)
	require.NoError(t, err)
	require.True(t, found)
	found, err = f.engine.ID(receipt.IncomeId).Get(&savedIncome)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, target.Id, savedExpense.BookId)
	require.Equal(t, target.Id, savedIncome.BookId)
	require.Equal(t, balance, f.balance())
	require.Equal(t, receivable, reimbursementBalance(t, f))

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "book-reimbursement.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(path string) error {
		_, e := f.engine.Exec("VACUUM INTO ?", path)
		return e
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	var stored models.Transaction
	found, err = restored.ID(receipt.IncomeId).Get(&stored)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, target.Id, stored.BookId)
	require.Equal(t, receipt.Id, stored.ReimbursementReceiptId)
}

func TestReimbursementBookEditCannotSplitReceipt(t *testing.T) {
	f, expense := reimbursementFixture(t)
	receipt, err := Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{
		ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID),
		Amount: "40", Time: f.at + 10, TimeZone: "UTC", RequestId: "book-edit-reimbursement",
	})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "目标"})
	require.NoError(t, err)
	var original, income models.Transaction
	found, err := f.engine.ID(expense.TransactionId).Get(&original)
	require.NoError(t, err)
	require.True(t, found)
	found, err = f.engine.ID(receipt.IncomeId).Get(&income)
	require.NoError(t, err)
	require.True(t, found)
	original.BookId = target.Id
	require.Error(t, Transactions.ModifyTransaction(nil, &original, false, 0, nil, nil, nil, nil))
	income.BookId = target.Id
	require.Error(t, Transactions.ModifyTransaction(nil, &income, false, 0, nil, nil, nil, nil))
	var saved models.Transaction
	found, err = f.engine.ID(expense.TransactionId).Get(&saved)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, expense.BookId, saved.BookId)
}

func TestBookMoveReimbursementChecksReceiptAccountReconciliation(t *testing.T) {
	f, expense := reimbursementFixture(t)
	const cashID int64 = 99102
	_, err := f.engine.Insert(&models.Account{AccountId: cashID, Uid: f.uid, Name: "到账账户", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT, Currency: "CNY"})
	require.NoError(t, err)
	receipt, err := Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{
		ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(cashID),
		Amount: "40", Time: f.at + 10, TimeZone: "UTC", RequestId: "book-move-reconciled-receipt",
	})
	require.NoError(t, err)
	cutoff := f.at + 20
	_, err = f.engine.ID(cashID).Cols("extend").Update(&models.Account{Extend: &models.AccountExtend{LastReconciledTime: &cutoff}})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "目标"})
	require.NoError(t, err)
	actor := refundTestActor(f.uid)
	actor.User.TransactionEditScope = models.TRANSACTION_EDIT_SCOPE_LAST_RECONCILED_TIME_OR_LATER
	actor.User.UseLastReconciledTime = true
	require.Error(t, Books.MoveTransactions(nil, f.uid, []int64{expense.TransactionId}, target.Id, actor))
	for _, id := range []int64{expense.TransactionId, receipt.IncomeId} {
		var stored models.Transaction
		found, err := f.engine.ID(id).Get(&stored)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, expense.BookId, stored.BookId)
	}
}
