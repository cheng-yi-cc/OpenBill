package services

import (
	"fmt"
	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"xorm.io/xorm"
)

func reimbursementFixture(t *testing.T) (*investmentDBFixture, *models.Transaction) {
	f := newInvestmentDBFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 8401, Uid: f.uid, Name: "测试报销", Category: models.ACCOUNT_CATEGORY_RECEIVABLES, Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Currency: "CNY", Extend: &models.AccountExtend{AssetProfile: &models.AccountAssetProfile{Kind: "reimbursement"}}})
	require.NoError(t, err)
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	var category models.TransactionCategory
	_, err = f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_EXPENSE).Get(&category)
	require.NoError(t, err)
	tx := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 10000, CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + 1), ReimbursementAccountId: 8401}
	require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))
	return f, tx
}
func reimbursementBalance(t *testing.T, f *investmentDBFixture) int64 {
	var a models.Account
	_, err := f.engine.ID(8401).Get(&a)
	require.NoError(t, err)
	return a.Balance
}
func TestReimbursementPartialReceiptIdempotenceHistoricalBalanceAndDeletion(t *testing.T) {
	f, tx := reimbursementFixture(t)
	require.Equal(t, int64(1990000), f.balance())
	require.Equal(t, int64(10000), reimbursementBalance(t, f))
	req := ReimbursementReceiveRequest{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "40.00", Time: f.at + 20, TimeZone: "Asia/Shanghai", RequestId: "receipt-request-1"}
	receipt, err := Investments.ReceiveReimbursement(nil, f.uid, req)
	require.NoError(t, err)
	require.Equal(t, int64(1994000), f.balance())
	require.Equal(t, int64(6000), reimbursementBalance(t, f))
	same, err := Investments.ReceiveReimbursement(nil, f.uid, req)
	require.NoError(t, err)
	require.Equal(t, receipt.Id, same.Id)
	require.Equal(t, int64(1994000), f.balance())
	req.Amount = "70.00"
	req.RequestId = "receipt-request-2"
	_, err = Investments.ReceiveReimbursement(nil, f.uid, req)
	require.Error(t, err)
	_, err = Investments.ReceiveReimbursement(nil, f.uid+1, req)
	require.Error(t, err)
	_, err = f.engine.Transaction(func(sess *xorm.Session) (any, error) {
		before, e := reimbursementBalancesAt(sess, f.uid, f.at+10)
		require.NoError(t, e)
		require.Equal(t, int64(10000), before[8401])
		after, e := reimbursementBalancesAt(sess, f.uid, f.at+30)
		require.NoError(t, e)
		require.Equal(t, int64(6000), after[8401])
		return nil, nil
	})
	require.NoError(t, err)
	income, expense, err := Transactions.GetAccountsTotalIncomeAndExpense(nil, f.uid, f.at, f.at+60, nil, nil, time.UTC, false)
	require.NoError(t, err)
	require.Empty(t, income)
	require.Empty(t, expense)
	require.Error(t, Transactions.DeleteTransaction(nil, f.uid, tx.TransactionId))
	require.NoError(t, Transactions.DeleteTransaction(nil, f.uid, receipt.IncomeId))
	require.Equal(t, int64(10000), reimbursementBalance(t, f))
	require.NoError(t, Transactions.DeleteTransaction(nil, f.uid, tx.TransactionId))
	require.Zero(t, reimbursementBalance(t, f))
	require.Equal(t, int64(2000000), f.balance())
}

func TestReimbursementEndReopenAndCascadeAccountDeletion(t *testing.T) {
	f, tx := reimbursementFixture(t)
	_, err := Investments.SetReimbursementState(nil, f.uid, tx.TransactionId, "end")
	require.NoError(t, err)
	require.Zero(t, reimbursementBalance(t, f))
	_, err = Investments.SetReimbursementState(nil, f.uid, tx.TransactionId, "reopen")
	require.NoError(t, err)
	require.Equal(t, int64(10000), reimbursementBalance(t, f))
	_, err = Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "40", Time: f.at + 10, TimeZone: "Asia/Shanghai", RequestId: "receipt-test-cascade"})
	require.NoError(t, err)
	require.Error(t, Accounts.DeleteAccount(nil, f.uid, 8401))
	input := AccountDeletionInput{ID: "8401", Kind: "cash", DeleteRelated: true}
	preview, err := Investments.DeleteAssetAccount(nil, f.uid, input, true)
	require.NoError(t, err)
	require.Equal(t, 2, preview.TransactionCount)
	input.Token = preview.Token
	_, err = Investments.DeleteAssetAccount(nil, f.uid, input, false)
	require.NoError(t, err)
	require.Equal(t, int64(2000000), f.balance())
	claims, err := Investments.ReimbursementClaims(nil, f.uid)
	require.NoError(t, err)
	require.Empty(t, claims)
}

func TestReimbursementRefundReplaysPendingAndHistoricalBalances(t *testing.T) {
	f, tx := reimbursementFixture(t)
	require.NoError(t, f.engine.Sync2(&models.Refund{}))
	actor := refundTestActor(f.uid)
	_, err := Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "40", Time: f.at + 20, TimeZone: "UTC", RequestId: "claim-refund-receipt"})
	require.NoError(t, err)
	input := RefundInput{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "30", Time: f.at + 30, TimeZone: "UTC", RequestId: "claim-refund-first"}
	summary, err := Investments.CreateRefund(nil, f.uid, actor, input)
	require.NoError(t, err)
	require.Equal(t, "70.00", summary.NetExpense)
	require.Equal(t, int64(3000), reimbursementBalance(t, f))
	beforeRetry := f.balance()
	retry, err := Investments.CreateRefund(nil, f.uid, actor, input)
	require.NoError(t, err)
	require.Equal(t, summary.Refunds[0].Id, retry.Refunds[0].Id)
	require.Equal(t, beforeRetry, f.balance())
	claims, err := Investments.ReimbursementClaims(nil, f.uid)
	require.NoError(t, err)
	require.Equal(t, "70.00", claims[0].Amount)
	require.Equal(t, "40.00", claims[0].Paid)
	require.Equal(t, "30.00", claims[0].Pending)
	_, err = f.engine.Transaction(func(sess *xorm.Session) (any, error) {
		for _, check := range []struct{ at, amount int64 }{{f.at + 10, 10000}, {f.at + 25, 6000}, {f.at + 35, 3000}} {
			balances, e := reimbursementBalancesAt(sess, f.uid, check.at)
			require.NoError(t, e)
			require.Equal(t, check.amount, balances[8401])
		}
		return nil, nil
	})
	require.NoError(t, err)
	row := summary.Refunds[0]
	summary, err = Investments.UpdateRefund(nil, f.uid, actor, row.Id, RefundEditInput{AccountId: fmt.Sprint(f.bankID), Amount: "80", Time: f.at + 30, TimeZone: "UTC", Version: row.Version})
	require.NoError(t, err)
	require.Equal(t, int64(0), reimbursementBalance(t, f))
	claims, err = Investments.ReimbursementClaims(nil, f.uid)
	require.NoError(t, err)
	require.Equal(t, "20.00", claims[0].Amount)
	require.Equal(t, "40.00", claims[0].Paid)
	require.Equal(t, "0.00", claims[0].Pending)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "claim-refund.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, e := f.engine.Exec("VACUUM INTO ?", target)
		return e
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	session := restored.NewSession()
	defer session.Close()
	restoredSummary, err := refundSummary(session, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "20.00", restoredSummary.NetExpense)
	restoredPaid, err := reimbursementPaid(session, f.uid, tx.TransactionId, 0)
	require.NoError(t, err)
	require.Equal(t, int64(4000), restoredPaid)
	restoredBalances, err := reimbursementBalancesAt(session, f.uid, 0)
	require.NoError(t, err)
	require.Zero(t, restoredBalances[8401])
	_, err = Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "1", Time: f.at + 40, TimeZone: "UTC", RequestId: "claim-refund-excess-receipt"})
	require.Error(t, err)
	require.Error(t, Investments.DeleteExpenseAndRefunds(nil, f.uid, actor, tx.TransactionId))
	still, err := Investments.GetRefunds(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "80.00", still.RefundedAmount)
	_, err = Investments.DeleteRefund(nil, f.uid, actor, summary.Refunds[0].Id, summary.Refunds[0].Version)
	require.NoError(t, err)
	require.Equal(t, int64(6000), reimbursementBalance(t, f))
}

func TestReimbursementAfterRefundCannotExceedNetExpense(t *testing.T) {
	f, tx := reimbursementFixture(t)
	require.NoError(t, f.engine.Sync2(&models.Refund{}))
	_, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "30", Time: f.at + 5, TimeZone: "UTC", RequestId: "claim-refund-before-receipt"})
	require.NoError(t, err)
	req := ReimbursementReceiveRequest{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "71", Time: f.at + 20, TimeZone: "UTC", RequestId: "claim-refund-invalid-receipt"}
	_, err = Investments.ReceiveReimbursement(nil, f.uid, req)
	require.Error(t, err)
	req.Amount, req.RequestId = "70", "claim-refund-valid-receipt"
	_, err = Investments.ReceiveReimbursement(nil, f.uid, req)
	require.NoError(t, err)
	require.Zero(t, reimbursementBalance(t, f))
}

func TestReimbursementReceiptEditAfterRefundKeepsExistingAmountButCannotIncrease(t *testing.T) {
	f, expense := reimbursementFixture(t)
	require.NoError(t, f.engine.Sync2(&models.Refund{}))
	receipt, err := Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "40", Time: f.at + 20, TimeZone: "UTC", RequestId: "edit-overpaid-receipt"})
	require.NoError(t, err)
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "80", Time: f.at + 30, TimeZone: "UTC", RequestId: "edit-overpaid-refund"})
	require.NoError(t, err)
	var income models.Transaction
	has, err := f.engine.ID(receipt.IncomeId).Where("uid=? AND deleted=?", f.uid, false).Get(&income)
	require.NoError(t, err)
	require.True(t, has)
	before := f.balance()
	income.Amount = 4100
	require.Error(t, Transactions.ModifyTransaction(nil, &income, false, 0, nil, nil, nil, nil))
	require.Equal(t, before, f.balance())
	income.Amount = 3000
	require.NoError(t, Transactions.ModifyTransaction(nil, &income, false, 0, nil, nil, nil, nil))
	require.Equal(t, before-1000, f.balance())
	require.Zero(t, reimbursementBalance(t, f))
	income.Amount = 3100
	require.Error(t, Transactions.ModifyTransaction(nil, &income, false, 0, nil, nil, nil, nil))
	income.Amount = 1000
	require.NoError(t, Transactions.ModifyTransaction(nil, &income, false, 0, nil, nil, nil, nil))
	require.Equal(t, int64(1000), reimbursementBalance(t, f))
	claims, err := Investments.ReimbursementClaims(nil, f.uid)
	require.NoError(t, err)
	require.Equal(t, "10.00", claims[0].Paid)
	require.Equal(t, "10.00", claims[0].Pending)
}

func TestReconciliationCountsRefundAsCashInflow(t *testing.T) {
	f, expense := reimbursementFixture(t)
	require.NoError(t, f.engine.Sync2(&models.Refund{}))
	_, err := Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "40", Time: f.at + 20, TimeZone: "UTC", RequestId: "statement-receipt"})
	require.NoError(t, err)
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "30", Time: f.at + 30, TimeZone: "UTC", RequestId: "statement-refund"})
	require.NoError(t, err)
	items, inflows, outflows, opening, closing, err := Transactions.GetAllTransactionsInOneAccountWithAccountBalanceByMaxTime(nil, f.uid, 2, utils.GetMaxTransactionTimeFromUnixTime(f.at+40), utils.GetMinTransactionTimeFromUnixTime(f.at+10), f.bankID, models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "7000", inflows.String())
	require.Equal(t, "0", outflows.String())
	require.Equal(t, "1990000", opening.String())
	require.Equal(t, "1997000", closing.String())
	require.Equal(t, "1994000", items[0].AccountClosingBalance.String())
	require.Equal(t, "1997000", items[1].AccountClosingBalance.String())
	_, inflows, outflows, _, closing, err = Transactions.GetAllTransactionsInOneAccountWithAccountBalanceByMaxTime(nil, f.uid, 2, utils.GetMaxTransactionTimeFromUnixTime(f.at+40), utils.GetMinTransactionTimeFromUnixTime(f.at+1), f.bankID, models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT)
	require.NoError(t, err)
	require.Equal(t, "7000", inflows.String())
	require.Equal(t, "10000", outflows.String())
	require.Equal(t, "1997000", closing.String())
}

func TestReimbursementAndRefundConcurrentCommitPreservesFacts(t *testing.T) {
	f, tx := reimbursementFixture(t)
	require.NoError(t, f.engine.Sync2(&models.Refund{}))
	var wg sync.WaitGroup
	wg.Add(2)
	start := make(chan struct{})
	var refundErr, receiptErr error
	go func() {
		defer wg.Done()
		<-start
		_, refundErr = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "70", Time: f.at + 10, TimeZone: "UTC", RequestId: "concurrent-claim-refund"})
	}()
	go func() {
		defer wg.Done()
		<-start
		_, receiptErr = Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{ExpenseId: fmt.Sprint(tx.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "40", Time: f.at + 20, TimeZone: "UTC", RequestId: "concurrent-claim-receipt"})
	}()
	close(start)
	wg.Wait()
	require.NoError(t, refundErr)
	claims, err := Investments.ReimbursementClaims(nil, f.uid)
	require.NoError(t, err)
	require.Equal(t, "30.00", claims[0].Amount)
	if receiptErr == nil {
		require.Equal(t, "40.00", claims[0].Paid)
		require.Equal(t, "0.00", claims[0].Pending)
		require.Equal(t, int64(2001000), f.balance())
	} else {
		require.Equal(t, "0.00", claims[0].Paid)
		require.Equal(t, "30.00", claims[0].Pending)
		require.Equal(t, int64(1997000), f.balance())
	}
}

func TestMonetaryPrincipalExcludesOnlyActiveFixedDeposits(t *testing.T) {
	f, b := monetaryFixture(t)
	for _, d := range []models.FixedDeposit{
		{Id: "active", Principal: "5000", StartDate: "2026-09-01", MaturityDate: "2026-10-01"},
		{Id: "future", Principal: "3000", StartDate: "2026-10-01", MaturityDate: "2026-11-01"},
		{Id: "matured", Principal: "2000", StartDate: "2026-08-01", MaturityDate: "2026-09-29"},
		{Id: "closed", Principal: "1000", StartDate: "2026-09-01", MaturityDate: "2026-10-01", Closed: true, ClosedDate: "2026-09-20"},
		{Id: "closed-later", Principal: "1000", StartDate: "2026-09-01", MaturityDate: "2026-10-01", Closed: true, ClosedDate: "2026-09-30"},
	} {
		d.Uid = f.uid
		d.AccountId = fmt.Sprint(f.bankID)
		_, err := f.engine.Insert(&d)
		require.NoError(t, err)
	}
	n, err := MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	require.Equal(t, 1, n)
	var day models.MonetaryIncomeDay
	_, err = f.engine.Where("uid=?", f.uid).Get(&day)
	require.NoError(t, err)
	require.Equal(t, "14000.00", day.Principal)
	require.Equal(t, "1.40", day.Amount)
}
