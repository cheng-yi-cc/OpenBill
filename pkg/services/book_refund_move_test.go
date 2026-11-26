package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestBookMoveRefundsPreservesFactsAndExcessScope(t *testing.T) {
	f, expense := refundFixture(t)
	actor := refundTestActor(f.uid)
	first := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "move-refund-one"}
	_, err := Investments.CreateRefund(nil, f.uid, actor, first)
	require.NoError(t, err)
	second := first
	second.Amount, second.Time, second.RequestId = "8", f.at+4, "move-refund-two"
	summary, err := Investments.CreateRefund(nil, f.uid, actor, second)
	require.NoError(t, err)
	require.Equal(t, "0.00", summary.NetExpense)
	require.Equal(t, "1.00", summary.ExcessIncome)
	excess := summary.Refunds[1]
	require.NotNil(t, excess.Excess)
	oldExcessBook := excess.Excess.BookId
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "目标"})
	require.NoError(t, err)
	cashID, err := strconv.ParseInt(summary.Refunds[0].TransactionId, 10, 64)
	require.NoError(t, err)
	before := f.balance()
	require.Error(t, Books.MoveTransactions(nil, f.uid, []int64{cashID}, target.Id, actor))
	require.Error(t, Books.MoveTransactions(nil, f.uid, []int64{expense.TransactionId}, target.Id))
	require.NoError(t, Books.MoveTransactions(nil, f.uid, []int64{expense.TransactionId}, target.Id, actor))
	require.Equal(t, before, f.balance())
	for _, refund := range summary.Refunds {
		id, err := strconv.ParseInt(refund.TransactionId, 10, 64)
		require.NoError(t, err)
		var cash models.Transaction
		has, err := f.engine.ID(id).Get(&cash)
		require.NoError(t, err)
		require.True(t, has)
		require.Equal(t, target.Id, cash.BookId)
	}
	after, err := Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "0.00", after.NetExpense)
	require.Equal(t, "1.00", after.ExcessIncome)
	require.Equal(t, oldExcessBook, after.Refunds[1].Excess.BookId)
	require.Greater(t, after.Refunds[1].Version, excess.Version)
	_, err = Investments.UpdateRefund(nil, f.uid, actor, excess.Id, RefundEditInput{AccountId: second.AccountId, Amount: second.Amount, Time: second.Time, TimeZone: "UTC", Version: excess.Version})
	require.ErrorIs(t, err, ErrInvestmentConflict)
}

func TestBookMoveRefundAndReimbursementTogetherRestores(t *testing.T) {
	f, expense := reimbursementFixture(t)
	actor := refundTestActor(f.uid)
	receipt, err := Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{
		ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "40",
		Time: f.at + 10, TimeZone: "UTC", RequestId: "move-combined-receipt",
	})
	require.NoError(t, err)
	refund, err := Investments.CreateRefund(nil, f.uid, actor, RefundInput{
		ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "30",
		Time: f.at + 20, TimeZone: "UTC", RequestId: "move-combined-refund",
	})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "目标"})
	require.NoError(t, err)
	balance, pending := f.balance(), reimbursementBalance(t, f)
	require.NoError(t, Books.MoveTransactions(nil, f.uid, []int64{expense.TransactionId}, target.Id, actor))
	require.Equal(t, balance, f.balance())
	require.Equal(t, pending, reimbursementBalance(t, f))
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "refund-reimbursement-move.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(path string) error {
		_, err := f.engine.Exec("VACUUM INTO ?", path)
		return err
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	refundCashID, err := strconv.ParseInt(refund.Refunds[0].TransactionId, 10, 64)
	require.NoError(t, err)
	for _, id := range []int64{expense.TransactionId, receipt.IncomeId, refundCashID} {
		var tx models.Transaction
		found, err := restored.ID(id).Get(&tx)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, target.Id, tx.BookId)
	}
	sess := restored.NewSession()
	defer sess.Close()
	replayed, err := refundSummary(sess, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "70.00", replayed.NetExpense)
	paid, err := reimbursementPaid(sess, f.uid, expense.TransactionId, 0)
	require.NoError(t, err)
	require.Equal(t, int64(4000), paid)
}

func TestBookMoveRefundChecksDestinationAccountReconciliation(t *testing.T) {
	f, expense := refundFixture(t)
	const destination int64 = 99103
	_, err := f.engine.Insert(&models.Account{AccountId: destination, Uid: f.uid, Name: "退款账户", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT, Currency: "CNY"})
	require.NoError(t, err)
	actor := refundTestActor(f.uid)
	summary, err := Investments.CreateRefund(nil, f.uid, actor, RefundInput{
		ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(destination), Amount: "5",
		Time: f.at + 10, TimeZone: "UTC", RequestId: "move-refund-reconciled",
	})
	require.NoError(t, err)
	cutoff := f.at + 20
	_, err = f.engine.ID(destination).Cols("extend").Update(&models.Account{Extend: &models.AccountExtend{LastReconciledTime: &cutoff}})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "目标"})
	require.NoError(t, err)
	actor.User.TransactionEditScope = models.TRANSACTION_EDIT_SCOPE_LAST_RECONCILED_TIME_OR_LATER
	actor.User.UseLastReconciledTime = true
	require.Error(t, Books.MoveTransactions(nil, f.uid, []int64{expense.TransactionId}, target.Id, actor))
	cashID, err := strconv.ParseInt(summary.Refunds[0].TransactionId, 10, 64)
	require.NoError(t, err)
	for _, id := range []int64{expense.TransactionId, cashID} {
		var stored models.Transaction
		found, err := f.engine.ID(id).Get(&stored)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, expense.BookId, stored.BookId)
	}
	after, err := Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, summary.Refunds[0].Version, after.Refunds[0].Version)
}
