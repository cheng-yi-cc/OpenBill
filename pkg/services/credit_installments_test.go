package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestCreditScheduleMonthEndsAndExactRemainders(t *testing.T) {
	for _, method := range []string{"monthly", "first_fee", "immediate_fee", "balloon"} {
		for _, remainder := range []string{"first", "last", "except_first", "except_last"} {
			d, e := makeCreditSchedule(CreditInstallmentInput{Principal: "100.01", TotalFee: "10.01", Periods: 3, FirstDate: "2026-01-31", Method: method, Remainder: remainder, TimeZone: "Asia/Shanghai"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			require.NoError(t, e)
			p, f := decimal.Zero, decimal.Zero
			for _, payment := range d.Payments {
				a, _ := decimal.NewFromString(payment.Principal)
				b, _ := decimal.NewFromString(payment.Fee)
				p = p.Add(a)
				f = f.Add(b)
			}
			require.Equal(t, "100.01", p.String())
			require.Equal(t, "10.01", f.String())
			require.Equal(t, "2026-02-28", d.Payments[len(d.Payments)-2].Date)
			require.Equal(t, "2026-03-31", d.Payments[len(d.Payments)-1].Date)
		}
	}
	zone, _ := time.LoadLocation("Asia/Shanghai")
	day := 31
	a := models.Account{Extend: &models.AccountExtend{CreditCardStatementDate: &day, AssetProfile: &models.AccountAssetProfile{RepaymentAfterDays: 20}}}
	month, _, _, statement := creditCycle(a, time.Date(2026, 2, 28, 10, 0, 0, 0, zone))
	require.Equal(t, "2026-02", month)
	require.Equal(t, "2026-03-20", creditDueDate(a, statement))
	a.Extend.AssetProfile.StatementNextCycle = true
	month, _, _, _ = creditCycle(a, time.Date(2026, 2, 28, 0, 0, 0, 0, zone))
	require.Equal(t, "2026-03", month)
}

func TestCreditInstallmentPrincipalFeesIdempotenceAndRepayment(t *testing.T) {
	f := newInvestmentDBFixture(t)
	zone, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(zone)
	_, e := f.engine.Insert(&models.Account{AccountId: 8501, Uid: f.uid, Name: "测试信贷", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CREDIT_CARD, Currency: "CNY"})
	require.NoError(t, e)
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	var category models.TransactionCategory
	_, e = f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_EXPENSE).Get(&category)
	require.NoError(t, e)
	expense := &models.Transaction{Uid: f.uid, AccountId: 8501, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 10000, CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at)}
	require.NoError(t, Transactions.CreateTransaction(nil, expense, nil, nil))
	input := CreditInstallmentInput{AccountId: "8501", ExpenseId: fmt.Sprint(expense.TransactionId), Principal: "100", TotalFee: "6", Periods: 3, FirstDate: now.Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "Asia/Shanghai", RequestId: "credit-plan-test-01"}
	plan, e := Investments.SaveCreditInstallment(nil, f.uid, input)
	require.NoError(t, e)
	same, e := Investments.SaveCreditInstallment(nil, f.uid, input)
	require.NoError(t, e)
	require.Equal(t, plan.Id, same.Id)
	n, e := Investments.SyncCreditInstallments(nil, f.uid, now)
	require.NoError(t, e)
	require.Equal(t, 1, n)
	n, e = Investments.SyncCreditInstallments(nil, f.uid, now)
	require.NoError(t, e)
	require.Zero(t, n)
	var a models.Account
	_, e = f.engine.ID(8501).Get(&a)
	require.NoError(t, e)
	require.Equal(t, int64(-10200), a.Balance)
	report, e := Investments.CreditReports(nil, f.uid, "8501", "Asia/Shanghai")
	require.NoError(t, e)
	require.Equal(t, "102.00", report[0].Outstanding)
	remaining := decimal.Zero
	for _, row := range report[0].Statements {
		v, _ := decimal.NewFromString(row.Remaining)
		remaining = remaining.Add(v)
	}
	require.Equal(t, "102", remaining.String())
	require.Error(t, Transactions.DeleteTransaction(nil, f.uid, expense.TransactionId))
	modified := *expense
	modified.Amount = 5000
	require.Error(t, Transactions.ModifyTransaction(nil, &modified, false, 0, nil, nil, nil, nil))
	var transferCategory models.TransactionCategory
	_, e = f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_TRANSFER).Get(&transferCategory)
	require.NoError(t, e)
	repayment := &models.Transaction{Uid: f.uid, AccountId: f.bankID, RelatedAccountId: 8501, Type: models.TRANSACTION_DB_TYPE_TRANSFER_OUT, CategoryId: transferCategory.CategoryId, Amount: 3500, RelatedAccountAmount: 3500, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(now.Unix())}
	require.NoError(t, Transactions.CreateTransaction(nil, repayment, nil, nil))
	report, e = Investments.CreditReports(nil, f.uid, "8501", "Asia/Shanghai")
	require.NoError(t, e)
	require.Equal(t, "67.00", report[0].Outstanding)
	rows, e := Investments.CreditInstallments(nil, f.uid)
	require.NoError(t, e)
	_, e = Investments.CloseCreditInstallment(nil, f.uid, plan.Id, fmt.Sprint(rows[0].Version))
	require.NoError(t, e)
	report, e = Investments.CreditReports(nil, f.uid, "8501", "Asia/Shanghai")
	require.NoError(t, e)
	actualFees := decimal.Zero
	for _, statement := range report[0].Statements {
		fee, err := decimal.NewFromString(statement.Fees)
		require.NoError(t, err)
		actualFees = actualFees.Add(fee)
	}
	require.Equal(t, "2.00", actualFees.StringFixed(2), "closing a plan must retain its already posted fee in the statement")
	n, e = Investments.SyncCreditInstallments(nil, f.uid, now.AddDate(1, 0, 0))
	require.NoError(t, e)
	require.Zero(t, n)
	require.NoError(t, Transactions.DeleteTransaction(nil, f.uid, expense.TransactionId))
}

func TestCreditPostedFeeFollowsRealTransactionAndSurvivesCloseRestore(t *testing.T) {
	f := newInvestmentDBFixture(t)
	now := time.Now().UTC()
	_, err := f.engine.Insert(&models.Account{AccountId: 8504, Uid: f.uid, Name: "手续费测试卡", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CREDIT_CARD, Currency: "CNY"})
	require.NoError(t, err)
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	var category models.TransactionCategory
	has, err := f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_EXPENSE).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	expense := &models.Transaction{Uid: f.uid, AccountId: 8504, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 10000, CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at)}
	require.NoError(t, Transactions.CreateTransaction(nil, expense, nil, nil))
	plan, err := Investments.SaveCreditInstallment(nil, f.uid, CreditInstallmentInput{AccountId: "8504", ExpenseId: fmt.Sprint(expense.TransactionId), Principal: "100", TotalFee: "6", Periods: 3, FirstDate: now.Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "UTC", RequestId: "credit-fee-edit-test"})
	require.NoError(t, err)
	_, err = Investments.SyncCreditInstallments(nil, f.uid, now)
	require.NoError(t, err)
	plans, err := Investments.CreditInstallments(nil, f.uid)
	require.NoError(t, err)
	var fee models.Transaction
	has, err = f.engine.ID(plans[0].Data.Payments[0].FeeTransactionId).Get(&fee)
	require.NoError(t, err)
	require.True(t, has)
	changedAt := now.AddDate(0, -1, 0)
	fee.Amount = 250
	fee.TransactionTime = utils.GetMinTransactionTimeFromUnixTime(changedAt.Unix())
	require.NoError(t, Transactions.ModifyTransaction(nil, &fee, false, 0, nil, nil, nil, nil))
	var account models.Account
	has, err = f.engine.ID(8504).Get(&account)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(-10250), account.Balance)
	check := func(sess *xorm.Session, a models.Account) {
		report, e := creditReportInSession(sess, f.uid, a, now)
		require.NoError(t, e)
		feeMonth, _, _, _ := creditCycle(a, changedAt)
		total := decimal.Zero
		for _, statement := range report.Statements {
			amount, e := decimal.NewFromString(statement.Fees)
			require.NoError(t, e)
			total = total.Add(amount)
			if statement.Month == feeMonth {
				require.Equal(t, "2.50", statement.Fees)
			}
		}
		require.Equal(t, "2.50", total.StringFixed(2))
		require.Equal(t, "102.50", report.Outstanding)
	}
	sess := f.engine.NewSession()
	check(sess, account)
	sess.Close()
	_, err = Investments.CloseCreditInstallment(nil, f.uid, plan.Id, fmt.Sprint(plans[0].Version))
	require.NoError(t, err)
	sess = f.engine.NewSession()
	check(sess, account)
	sess.Close()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "credit-fee.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, e := f.engine.Exec("VACUUM INTO ?", target)
		return e
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	sess = restored.NewSession()
	defer sess.Close()
	has, err = sess.ID(8504).Get(&account)
	require.NoError(t, err)
	require.True(t, has)
	check(sess, account)
}

// Future fees are reminders only: they must survive principal repayment without
// changing the ledger balance, and an overpayment must offset them once.
func TestCreditInstallmentForecastDoesNotChangeBalance(t *testing.T) {
	f := newInvestmentDBFixture(t)
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month()+1, 5, 12, 0, 0, 0, time.UTC)
	_, err := f.engine.Insert(&models.Account{AccountId: 8503, Uid: f.uid, Name: "预测信用卡", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CREDIT_CARD, Currency: "CNY"})
	require.NoError(t, err)
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	var expenseCategory, transferCategory models.TransactionCategory
	has, err := f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_EXPENSE).Get(&expenseCategory)
	require.NoError(t, err)
	require.True(t, has)
	has, err = f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_TRANSFER).Get(&transferCategory)
	require.NoError(t, err)
	require.True(t, has)
	expense := &models.Transaction{Uid: f.uid, AccountId: 8503, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 300, CategoryId: expenseCategory.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at)}
	require.NoError(t, Transactions.CreateTransaction(nil, expense, nil, nil))
	plan, err := Investments.SaveCreditInstallment(nil, f.uid, CreditInstallmentInput{AccountId: "8503", ExpenseId: fmt.Sprint(expense.TransactionId), Principal: "3", TotalFee: "0.30", Periods: 3, FirstDate: first.Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "UTC", RequestId: "credit-forecast-test"})
	require.NoError(t, err)
	forecast := func() (string, string) {
		reports, e := Investments.CreditReports(nil, f.uid, "8503", "UTC")
		require.NoError(t, e)
		total := decimal.Zero
		for _, row := range reports[0].Statements {
			amount, e := decimal.NewFromString(row.ProjectedRemaining)
			require.NoError(t, e)
			total = total.Add(amount)
		}
		return reports[0].Outstanding, total.StringFixed(2)
	}
	outstanding, projected := forecast()
	require.Equal(t, "3.00", outstanding)
	require.Equal(t, "3.30", projected)
	repayment := &models.Transaction{Uid: f.uid, AccountId: f.bankID, RelatedAccountId: 8503, Type: models.TRANSACTION_DB_TYPE_TRANSFER_OUT, CategoryId: transferCategory.CategoryId, Amount: 300, RelatedAccountAmount: 300, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + 2)}
	require.NoError(t, Transactions.CreateTransaction(nil, repayment, nil, nil))
	outstanding, projected = forecast()
	require.Equal(t, "0.00", outstanding)
	require.Equal(t, "0.30", projected)
	reports, err := Investments.CreditReports(nil, f.uid, "8503", "UTC")
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		month := first.AddDate(0, i, 0).Format("2006-01")
		found := false
		for _, row := range reports[0].Statements {
			if row.Month == month {
				found = true
				require.Equal(t, "0.10", row.ProjectedRemaining, "each future fee belongs to its own cycle")
			}
		}
		require.True(t, found)
	}
	var account models.Account
	has, err = f.engine.ID(8503).Get(&account)
	require.NoError(t, err)
	require.True(t, has)
	require.Zero(t, account.Balance)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "forecast.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, e := f.engine.Exec("VACUUM INTO ?", target)
		return e
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	sess := restored.NewSession()
	defer sess.Close()
	var restoredAccount models.Account
	has, err = sess.ID(8503).Get(&restoredAccount)
	require.NoError(t, err)
	require.True(t, has)
	copyReport, err := creditReportInSession(sess, f.uid, restoredAccount, now)
	require.NoError(t, err)
	copyTotal := decimal.Zero
	for _, row := range copyReport.Statements {
		amount, e := decimal.NewFromString(row.ProjectedRemaining)
		require.NoError(t, e)
		copyTotal = copyTotal.Add(amount)
	}
	require.Equal(t, "0.00", copyReport.Outstanding)
	require.Equal(t, "0.30", copyTotal.StringFixed(2))
	overpayment := &models.Transaction{Uid: f.uid, AccountId: f.bankID, RelatedAccountId: 8503, Type: models.TRANSACTION_DB_TYPE_TRANSFER_OUT, CategoryId: transferCategory.CategoryId, Amount: 10, RelatedAccountAmount: 10, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + 3)}
	require.NoError(t, Transactions.CreateTransaction(nil, overpayment, nil, nil))
	outstanding, projected = forecast()
	require.Equal(t, "0.00", outstanding)
	require.Equal(t, "0.20", projected)
	created, err := Investments.SyncCreditInstallments(nil, f.uid, first)
	require.NoError(t, err)
	require.Equal(t, 1, created)
	outstanding, projected = forecast()
	require.Equal(t, "0.00", outstanding)
	require.Equal(t, "0.20", projected, "posting one fee must not count it twice")
	plans, err := Investments.CreditInstallments(nil, f.uid)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	_, err = Investments.CloseCreditInstallment(nil, f.uid, plan.Id, fmt.Sprint(plans[0].Version))
	require.NoError(t, err)
	outstanding, projected = forecast()
	require.Equal(t, "0.00", outstanding)
	require.Equal(t, "0.00", projected, "closing the plan removes unposted fees")
}

func TestCreditStatementRefundStaysWithOriginalPeriod(t *testing.T) {
	f := newInvestmentDBFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 8502, Uid: f.uid, Name: "虚构信用卡", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CREDIT_CARD, Currency: "CNY"})
	require.NoError(t, err)
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	var category models.TransactionCategory
	has, err := f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_EXPENSE).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC)
	oldAt, newAt := month.AddDate(0, -2, 14), month.AddDate(0, -1, 14)
	oldExpense := &models.Transaction{Uid: f.uid, AccountId: 8502, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 1000, CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(oldAt.Unix())}
	newExpense := &models.Transaction{Uid: f.uid, AccountId: 8502, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 800, CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(newAt.Unix())}
	require.NoError(t, Transactions.CreateTransaction(nil, oldExpense, nil, nil))
	require.NoError(t, Transactions.CreateTransaction(nil, newExpense, nil, nil))
	refunded, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(newExpense.TransactionId), AccountId: "8502", Amount: "5", Time: f.at, TimeZone: "UTC", RequestId: "credit-period-refund-test"})
	require.NoError(t, err)
	reports, err := Investments.CreditReports(nil, f.uid, "8502", "UTC")
	require.NoError(t, err)
	require.Len(t, reports, 1)
	require.Equal(t, "13.00", reports[0].Outstanding)
	byMonth := map[string]CreditStatement{}
	for _, row := range reports[0].Statements {
		byMonth[row.Month] = row
	}
	require.Equal(t, "10.00", byMonth[oldAt.Format("2006-01")].Remaining)
	require.Equal(t, "3.00", byMonth[newAt.Format("2006-01")].Remaining)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "credit-refund.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, snapshotErr := f.engine.Exec("VACUUM INTO ?", target)
		return snapshotErr
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	sess := restored.NewSession()
	defer sess.Close()
	var restoredAccount models.Account
	has, err = sess.ID(8502).Get(&restoredAccount)
	require.NoError(t, err)
	require.True(t, has)
	restoredReport, err := creditReportInSession(sess, f.uid, restoredAccount, now)
	require.NoError(t, err)
	for _, row := range restoredReport.Statements {
		byMonth[row.Month] = row
	}
	require.Equal(t, "13.00", restoredReport.Outstanding)
	require.Equal(t, "10.00", byMonth[oldAt.Format("2006-01")].Remaining)
	require.Equal(t, "3.00", byMonth[newAt.Format("2006-01")].Remaining)
	var transferCategory models.TransactionCategory
	has, err = f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_TRANSFER).Get(&transferCategory)
	require.NoError(t, err)
	require.True(t, has)
	repayment := &models.Transaction{Uid: f.uid, AccountId: f.bankID, RelatedAccountId: 8502, Type: models.TRANSACTION_DB_TYPE_TRANSFER_OUT, CategoryId: transferCategory.CategoryId, Amount: 400, RelatedAccountAmount: 400, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + 2)}
	require.NoError(t, Transactions.CreateTransaction(nil, repayment, nil, nil))
	reports, err = Investments.CreditReports(nil, f.uid, "8502", "UTC")
	require.NoError(t, err)
	require.Equal(t, "9.00", reports[0].Outstanding)
	for _, row := range reports[0].Statements {
		byMonth[row.Month] = row
	}
	require.Equal(t, "6.00", byMonth[oldAt.Format("2006-01")].Remaining)
	require.Equal(t, "3.00", byMonth[newAt.Format("2006-01")].Remaining)
	require.NoError(t, Transactions.DeleteTransaction(nil, f.uid, repayment.TransactionId))
	_, err = Investments.SaveCreditInstallment(nil, f.uid, CreditInstallmentInput{AccountId: "8502", ExpenseId: fmt.Sprint(newExpense.TransactionId), Principal: "3", TotalFee: "0", Periods: 2, FirstDate: now.Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "UTC", RequestId: "credit-refund-plan-test"})
	require.ErrorContains(t, err, "关联退款")
	input := CreditInstallmentInput{AccountId: "8502", ExpenseId: fmt.Sprint(newExpense.TransactionId), Principal: "3.01", TotalFee: "0", Periods: 2, FirstDate: now.AddDate(0, 1, 0).Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "UTC", RequestId: "credit-refund-plan-optin", AllowRefundedExpense: true}
	_, err = Investments.SaveCreditInstallment(nil, f.uid, input)
	require.Error(t, err, "refunded amount cannot be financed")
	input.Principal = "3"
	input.FirstDate = now.AddDate(0, 0, -1).Format("2006-01-02")
	_, err = Investments.SaveCreditInstallment(nil, f.uid, input)
	require.Error(t, err, "installment dates cannot precede the refund")
	input.FirstDate = now.AddDate(0, 1, 0).Format("2006-01-02")
	plan, err := Investments.SaveCreditInstallment(nil, f.uid, input)
	require.NoError(t, err)
	// Editing the original expense must not leave financed principal greater
	// than the amount still owed to this card after its own refund.
	revised, err := Transactions.GetTransactionByTransactionId(nil, f.uid, newExpense.TransactionId)
	require.NoError(t, err)
	revised.Amount = 400
	err = Transactions.ModifyRefundExpense(nil, refundTestActor(f.uid), revised, 0, nil, nil, nil, nil)
	require.Error(t, err, "4 minus the 5 returned to the card cannot fund a 3 installment")
	revised, err = Transactions.GetTransactionByTransactionId(nil, f.uid, newExpense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, int64(800), revised.Amount)
	reports, err = Investments.CreditReports(nil, f.uid, "8502", "UTC")
	require.NoError(t, err)
	require.Equal(t, "13.00", reports[0].Outstanding, "rejected revision must not change the card balance")
	again, err := Investments.SaveCreditInstallment(nil, f.uid, input)
	require.NoError(t, err)
	require.Equal(t, plan.Id, again.Id)
	changed := input
	changed.Principal = "2"
	_, err = Investments.SaveCreditInstallment(nil, f.uid, changed)
	require.Error(t, err, "a request identity cannot be reused for different terms")
	changed.RequestId = "credit-refund-second-plan"
	changed.Principal = "0.01"
	_, err = Investments.SaveCreditInstallment(nil, f.uid, changed)
	require.Error(t, err, "overlapping plans cannot reuse the refunded principal")
	reports, err = Investments.CreditReports(nil, f.uid, "8502", "UTC")
	require.NoError(t, err)
	require.Equal(t, "13.00", reports[0].Outstanding)
	total := decimal.Zero
	for _, row := range reports[0].Statements {
		amount, e := decimal.NewFromString(row.Remaining)
		require.NoError(t, e)
		total = total.Add(amount)
		byMonth[row.Month] = row
	}
	require.Equal(t, "13.00", total.StringFixed(2))
	require.Equal(t, "10.00", byMonth[oldAt.Format("2006-01")].Remaining)
	require.Equal(t, "0.00", byMonth[newAt.Format("2006-01")].Remaining)
	firstDate, err := time.ParseInLocation("2006-01-02", input.FirstDate, time.UTC)
	require.NoError(t, err)
	secondDate := creditDate(firstDate.Year(), firstDate.Month()+1, firstDate.Day(), time.UTC)
	require.Equal(t, "1.50", byMonth[firstDate.Format("2006-01")].Remaining)
	require.Equal(t, "1.50", byMonth[secondDate.Format("2006-01")].Remaining)
	refundedArchive := filepath.Join(t.TempDir(), "refunded-plan.zip")
	require.NoError(t, localbackup.Create(root, refundedArchive, []byte(`{}`), func(target string) error {
		_, e := f.engine.Exec("VACUUM INTO ?", target)
		return e
	}))
	refundedStage, err := localbackup.Stage(refundedArchive, t.TempDir())
	require.NoError(t, err)
	refundedEngine, err := xorm.NewEngine("sqlite3", filepath.Join(refundedStage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer refundedEngine.Close()
	refundedSession := refundedEngine.NewSession()
	defer refundedSession.Close()
	var refundedAccount models.Account
	has, err = refundedSession.ID(8502).Get(&refundedAccount)
	require.NoError(t, err)
	require.True(t, has)
	restoredWithPlan, err := creditReportInSession(refundedSession, f.uid, refundedAccount, now)
	require.NoError(t, err)
	require.Equal(t, "13.00", restoredWithPlan.Outstanding)
	total = decimal.Zero
	for _, row := range restoredWithPlan.Statements {
		amount, e := decimal.NewFromString(row.Remaining)
		require.NoError(t, e)
		total = total.Add(amount)
	}
	require.Equal(t, "13.00", total.StringFixed(2))
	var restoredPlan models.CreditInstallment
	has, err = refundedSession.ID(plan.Id).Get(&restoredPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "3.00", restoredPlan.Data.Principal)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), refunded.Refunds[0].Id, refunded.Refunds[0].Version)
	require.Error(t, err, "active installment must protect the refund")
	_, err = Investments.CloseCreditInstallment(nil, f.uid, plan.Id, fmt.Sprint(plan.Version))
	require.NoError(t, err)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), refunded.Refunds[0].Id, refunded.Refunds[0].Version)
	require.NoError(t, err)
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(newExpense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at, TimeZone: "UTC", RequestId: "credit-refund-other-account"})
	require.NoError(t, err)
	reports, err = Investments.CreditReports(nil, f.uid, "8502", "UTC")
	require.NoError(t, err)
	require.Equal(t, "18.00", reports[0].Outstanding)
	input.RequestId = "credit-refund-other-plan"
	_, err = Investments.SaveCreditInstallment(nil, f.uid, input)
	require.ErrorContains(t, err, "未全部退回原信贷账户")
	for _, row := range reports[0].Statements {
		byMonth[row.Month] = row
	}
	require.Equal(t, "10.00", byMonth[oldAt.Format("2006-01")].Remaining)
	require.Equal(t, "8.00", byMonth[newAt.Format("2006-01")].Remaining)
	// Opting in explicitly preserves older clients' refusal while allowing the
	// card's full outstanding principal when the refund went elsewhere.
	bankAfterRefund := f.balance()
	input.AllowOtherAccountRefund = true
	input.RequestId = "credit-cross-plan-optin"
	input.Principal = "8.01"
	_, err = Investments.SaveCreditInstallment(nil, f.uid, input)
	require.Error(t, err)
	input.Principal = "8"
	crossPlan, err := Investments.SaveCreditInstallment(nil, f.uid, input)
	require.NoError(t, err)
	again, err = Investments.SaveCreditInstallment(nil, f.uid, input)
	require.NoError(t, err)
	require.Equal(t, crossPlan.Id, again.Id)
	require.Equal(t, bankAfterRefund, f.balance())
	reports, err = Investments.CreditReports(nil, f.uid, "8502", "UTC")
	require.NoError(t, err)
	require.Equal(t, "18.00", reports[0].Outstanding)
	_, err = Investments.CloseCreditInstallment(nil, f.uid, crossPlan.Id, fmt.Sprint(crossPlan.Version))
	require.NoError(t, err)
	// Mixed destinations: only the 3 returned to the credit card reduce the
	// original card principal; the other 5 remain in the separate account.
	mixed, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(newExpense.TransactionId), AccountId: "8502", Amount: "3", Time: f.at, TimeZone: "UTC", RequestId: "credit-refund-mixed-card"})
	require.NoError(t, err)
	input.RequestId = "credit-mixed-plan-optin"
	input.Principal = "5.01"
	_, err = Investments.SaveCreditInstallment(nil, f.uid, input)
	require.Error(t, err)
	input.Principal = "5"
	mixedPlan, err := Investments.SaveCreditInstallment(nil, f.uid, input)
	require.NoError(t, err)
	require.Equal(t, bankAfterRefund, f.balance())
	reports, err = Investments.CreditReports(nil, f.uid, "8502", "UTC")
	require.NoError(t, err)
	require.Equal(t, "15.00", reports[0].Outstanding)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), mixed.Refunds[0].Id, mixed.Refunds[0].Version)
	require.Error(t, err, "an active plan protects refunds even when sent elsewhere")
	mixedArchive := filepath.Join(t.TempDir(), "mixed-refund-plan.zip")
	require.NoError(t, localbackup.Create(root, mixedArchive, []byte(`{}`), func(target string) error {
		_, e := f.engine.Exec("VACUUM INTO ?", target)
		return e
	}))
	mixedStage, err := localbackup.Stage(mixedArchive, t.TempDir())
	require.NoError(t, err)
	mixedEngine, err := xorm.NewEngine("sqlite3", filepath.Join(mixedStage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer mixedEngine.Close()
	mixedSession := mixedEngine.NewSession()
	defer mixedSession.Close()
	var mixedAccount, mixedBank models.Account
	has, err = mixedSession.ID(8502).Get(&mixedAccount)
	require.NoError(t, err)
	require.True(t, has)
	has, err = mixedSession.ID(f.bankID).Get(&mixedBank)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, bankAfterRefund, mixedBank.Balance)
	mixedReport, err := creditReportInSession(mixedSession, f.uid, mixedAccount, now)
	require.NoError(t, err)
	require.Equal(t, "15.00", mixedReport.Outstanding)
	var restoredMixedPlan models.CreditInstallment
	has, err = mixedSession.ID(mixedPlan.Id).Get(&restoredMixedPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "5.00", restoredMixedPlan.Data.Principal)
	var restoredRefunds []models.Refund
	require.NoError(t, mixedSession.Where("uid=? AND expense_id=? AND deleted=?", f.uid, newExpense.TransactionId, false).Find(&restoredRefunds))
	require.Len(t, restoredRefunds, 2)
	revised, err = Transactions.GetTransactionByTransactionId(nil, f.uid, newExpense.TransactionId)
	require.NoError(t, err)
	revised.Amount = 700
	err = Transactions.ModifyRefundExpense(nil, refundTestActor(f.uid), revised, 0, nil, nil, nil, nil)
	require.Error(t, err, "7 minus 3 returned to the card cannot fund a 5 installment")
	revised, err = Transactions.GetTransactionByTransactionId(nil, f.uid, newExpense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, int64(800), revised.Amount)
	revised.Amount = 900
	require.NoError(t, Transactions.ModifyRefundExpense(nil, refundTestActor(f.uid), revised, 0, nil, nil, nil, nil))
	require.Equal(t, bankAfterRefund, f.balance(), "editing the expense cannot alter cash refunded elsewhere")
	reports, err = Investments.CreditReports(nil, f.uid, "8502", "UTC")
	require.NoError(t, err)
	require.Equal(t, "16.00", reports[0].Outstanding)
}

func TestAssetAdjustmentConcurrencyIdempotenceAndStatistics(t *testing.T) {
	f := newInvestmentDBFixture(t)
	input := AssetAdjustmentInput{AccountId: fmt.Sprint(f.bankID), Balance: "21000.25", ExpectedBalance: "20000", TimeZone: "Asia/Shanghai", RequestId: "adjust-test-0001"}
	row, e := Investments.AdjustAssetBalance(nil, f.uid, input)
	require.NoError(t, e)
	require.Equal(t, int64(2100025), f.balance())
	same, e := Investments.AdjustAssetBalance(nil, f.uid, input)
	require.NoError(t, e)
	require.Equal(t, row.Id, same.Id)
	require.Equal(t, int64(2100025), f.balance())
	input.RequestId = "adjust-test-0002"
	_, e = Investments.AdjustAssetBalance(nil, f.uid, input)
	require.Error(t, e)
	var tx models.Transaction
	_, e = f.engine.ID(row.TransactionId).Get(&tx)
	require.NoError(t, e)
	require.True(t, tx.ExcludeFromStatistics)
	input.ExpectedBalance = "21000.25"
	input.Balance = "20000"
	input.CountInStatistics = true
	row, e = Investments.AdjustAssetBalance(nil, f.uid, input)
	require.NoError(t, e)
	tx = models.Transaction{}
	_, e = f.engine.ID(row.TransactionId).Get(&tx)
	require.NoError(t, e)
	require.False(t, tx.ExcludeFromStatistics)
	require.Equal(t, int64(2000000), f.balance())
}

func TestAssetAdjustmentCustomDetailsAreValidatedAndAtomic(t *testing.T) {
	f := newInvestmentDBFixture(t)
	book, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "差额测试"})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.TransactionCategory{CategoryId: 9210, Uid: f.uid, Type: models.CATEGORY_TYPE_INCOME, Name: "收入", BookIds: []string{book.Id}}, &models.TransactionCategory{CategoryId: 9211, Uid: f.uid, Type: models.CATEGORY_TYPE_INCOME, ParentCategoryId: 9210, Name: "其他收入", BookIds: []string{book.Id}}, &models.TransactionCategory{CategoryId: 9212, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "支出"}, &models.TransactionCategory{CategoryId: 9213, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, ParentCategoryId: 9212, Name: "其他支出"})
	require.NoError(t, err)
	tag := &models.TransactionTag{Uid: f.uid, Name: "虚构标签"}
	require.NoError(t, TransactionTags.CreateTag(nil, tag))
	comment := "测试差额"
	input := AssetAdjustmentInput{AccountId: fmt.Sprint(f.bankID), Balance: "20001", ExpectedBalance: "20000", BookId: book.Id, CountInStatistics: true, TimeZone: "UTC", RequestId: "custom-adjust-test", CategoryId: "9213", Comment: &comment, TagIds: []string{fmt.Sprint(tag.TagId)}}
	_, err = Investments.AdjustAssetBalance(nil, f.uid, input)
	require.Error(t, err, "income cannot use expense category")
	require.Equal(t, int64(2000000), f.balance())
	input.CategoryId = "9211"
	input.BookId = DefaultBookID(f.uid)
	_, err = Investments.AdjustAssetBalance(nil, f.uid, input)
	require.Error(t, err, "category must be available in selected book")
	require.Equal(t, int64(2000000), f.balance())
	input.BookId = book.Id
	input.TagIds = []string{"999999"}
	_, err = Investments.AdjustAssetBalance(nil, f.uid, input)
	require.Error(t, err, "unknown tag cannot create an adjustment")
	require.Equal(t, int64(2000000), f.balance())
	input.TagIds = []string{fmt.Sprint(tag.TagId)}
	row, err := Investments.AdjustAssetBalance(nil, f.uid, input)
	require.NoError(t, err)
	require.Equal(t, int64(2000100), f.balance())
	same, err := Investments.AdjustAssetBalance(nil, f.uid, input)
	require.NoError(t, err)
	require.Equal(t, row.TransactionId, same.TransactionId)
	require.Equal(t, int64(2000100), f.balance())
	var tx models.Transaction
	has, err := f.engine.ID(row.TransactionId).Get(&tx)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, book.Id, tx.BookId)
	require.Equal(t, int64(9211), tx.CategoryId)
	require.Equal(t, comment, tx.Comment)
	require.False(t, tx.ExcludeFromStatistics)
	count, err := f.engine.Where("uid=? AND transaction_id=? AND tag_id=? AND deleted=?", f.uid, tx.TransactionId, tag.TagId, false).Count(&models.TransactionTagIndex{})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "adjustment.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, e := f.engine.Exec("VACUUM INTO ?", target)
		return e
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	var restoredTx models.Transaction
	has, err = restored.ID(row.TransactionId).Get(&restoredTx)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, book.Id, restoredTx.BookId)
	require.Equal(t, int64(9211), restoredTx.CategoryId)
	require.Equal(t, comment, restoredTx.Comment)
	count, err = restored.Where("uid=? AND transaction_id=? AND tag_id=? AND deleted=?", f.uid, row.TransactionId, tag.TagId, false).Count(&models.TransactionTagIndex{})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	var restoredAccount models.Account
	has, err = restored.ID(f.bankID).Get(&restoredAccount)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(2000100), restoredAccount.Balance)
	changed := "不同内容"
	input.Comment = &changed
	_, err = Investments.AdjustAssetBalance(nil, f.uid, input)
	require.Error(t, err, "reusing request identity with new details must fail")
	require.Equal(t, int64(2000100), f.balance())
}

func TestCreditPostInstallmentRefundModesAndStatementMonthWithRefundsAndExistingInstallments(t *testing.T) {
	f := newInvestmentDBFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 8509, Uid: f.uid, Name: "分期后退款卡", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CREDIT_CARD, Currency: "CNY"})
	require.NoError(t, err)
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	var category models.TransactionCategory
	has, err := f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_EXPENSE).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC)
	m2At := month.AddDate(0, -2, 10)
	m1At := month.AddDate(0, -1, 10)
	next1 := month.AddDate(0, 1, 10)
	expenseA := &models.Transaction{Uid: f.uid, AccountId: 8509, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 30000, CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(m1At.Unix())}
	expenseB := &models.Transaction{Uid: f.uid, AccountId: 8509, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 10000, CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(m2At.Unix())}
	expenseC := &models.Transaction{Uid: f.uid, AccountId: 8509, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 8000, CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(m2At.Unix() + 1)}
	for _, tx := range []*models.Transaction{expenseA, expenseB, expenseC} {
		require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))
	}
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expenseB.TransactionId), AccountId: "8509", Amount: "40", Time: f.at + 10, TimeZone: "UTC", RequestId: "post-inst-refund-b"})
	require.NoError(t, err)
	_, err = Investments.SaveCreditInstallment(nil, f.uid, CreditInstallmentInput{AccountId: "8509", ExpenseId: fmt.Sprint(expenseC.TransactionId), Principal: "80", TotalFee: "0", Periods: 2, FirstDate: m2At.Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "UTC", RequestId: "post-inst-plan-c"})
	require.NoError(t, err)
	planA, err := Investments.SaveCreditInstallment(nil, f.uid, CreditInstallmentInput{AccountId: "8509", ExpenseId: fmt.Sprint(expenseA.TransactionId), Principal: "300", TotalFee: "9", Periods: 3, FirstDate: next1.Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "UTC", RequestId: "post-inst-plan-a"})
	require.NoError(t, err)
	summary, err := Investments.GetRefunds(nil, f.uid, expenseA.TransactionId)
	require.NoError(t, err)
	require.True(t, summary.HasActiveInstallment)
	require.True(t, summary.SupportsInstallmentReduce)
	// Post-installment offset leaves principal 300.00 untouched while reducing outstanding debt.
	r1Sum, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expenseA.TransactionId), AccountId: "8509", Amount: "60", Time: f.at + 11, TimeZone: "UTC", InstallmentMode: "offset", RequestId: "post-inst-r1-offset"})
	require.NoError(t, err)
	r1 := r1Sum.Refunds[len(r1Sum.Refunds)-1]
	var storedPlan models.CreditInstallment
	has, err = f.engine.ID(planA.Id).Get(&storedPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "300.00", storedPlan.Data.OriginalPrincipal)
	require.Equal(t, "300.00", storedPlan.Data.Principal)
	for _, p := range storedPlan.Data.Payments {
		require.Equal(t, "100.00", p.Principal)
		require.Equal(t, "3.00", p.Fee)
	}
	reports, err := Investments.CreditReports(nil, f.uid, "8509", "UTC")
	require.NoError(t, err)
	require.Equal(t, "380.00", reports[0].Outstanding)
	// Post-installment reduce scales down unaccrued plan principal from 300.00 to 210.00.
	r2Sum, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expenseA.TransactionId), AccountId: "8509", Amount: "90", Time: f.at + 12, TimeZone: "UTC", InstallmentMode: "reduce", RequestId: "post-inst-r2-reduce"})
	require.NoError(t, err)
	var r2 RefundView
	for _, item := range r2Sum.Refunds {
		if item.Id != r1.Id {
			r2 = item
		}
	}
	storedPlan = models.CreditInstallment{}
	has, err = f.engine.ID(planA.Id).Get(&storedPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "300.00", storedPlan.Data.OriginalPrincipal)
	require.Equal(t, "210.00", storedPlan.Data.Principal)
	for _, p := range storedPlan.Data.Payments {
		require.Equal(t, "70.00", p.Principal)
		require.Equal(t, "3.00", p.Fee)
	}
	// Editing mode from reduce -> offset restores 300.00; switching back to reduce (120) shrinks to 180.00.
	_, err = Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), r2.Id, RefundEditInput{AccountId: "8509", Amount: "90", Time: f.at + 12, TimeZone: "UTC", Version: r2.Version, InstallmentMode: "offset"})
	require.NoError(t, err)
	storedPlan = models.CreditInstallment{}
	has, err = f.engine.ID(planA.Id).Get(&storedPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "300.00", storedPlan.Data.Principal)
	r2Upd, err := Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), r2.Id, RefundEditInput{AccountId: "8509", Amount: "120", Time: f.at + 12, TimeZone: "UTC", Version: r2.Version + 1, InstallmentMode: "reduce"})
	require.NoError(t, err)
	for _, item := range r2Upd.Refunds {
		if item.Id == r1.Id {
			r1 = item
		} else if item.Id == r2.Id {
			r2 = item
		}
	}
	storedPlan = models.CreditInstallment{}
	has, err = f.engine.ID(planA.Id).Get(&storedPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "180.00", storedPlan.Data.Principal)
	for _, p := range storedPlan.Data.Payments {
		require.Equal(t, "60.00", p.Principal)
		require.Equal(t, "3.00", p.Fee)
	}
	// Deleting offset and reduce refunds restores principal back to 300.00.
	r1Del, err := Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), r1.Id, r1.Version)
	require.NoError(t, err)
	for _, item := range r1Del.Refunds {
		if item.Id == r2.Id {
			r2 = item
		}
	}
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), r2.Id, r2.Version)
	require.NoError(t, err)
	storedPlan = models.CreditInstallment{}
	has, err = f.engine.ID(planA.Id).Get(&storedPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "300.00", storedPlan.Data.Principal)
	// Re-apply reduce refund (90) and verify bill-month installment on m2At (60 net expenseB + 40 planC cycle payment = 100.00).
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expenseA.TransactionId), AccountId: "8509", Amount: "90", Time: f.at + 13, TimeZone: "UTC", InstallmentMode: "reduce", RequestId: "post-inst-r2-final"})
	require.NoError(t, err)
	billMonth := m2At.Format("2006-01")
	_, err = Investments.SaveCreditInstallment(nil, f.uid, CreditInstallmentInput{AccountId: "8509", StatementMonth: billMonth, Principal: "100.01", TotalFee: "0", Periods: 2, FirstDate: next1.Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "UTC", AllowRefundedExpense: true, RequestId: "post-inst-bill-over"})
	require.Error(t, err)
	billPlan, err := Investments.SaveCreditInstallment(nil, f.uid, CreditInstallmentInput{AccountId: "8509", StatementMonth: billMonth, Principal: "100", TotalFee: "0", Periods: 2, FirstDate: next1.Format("2006-01-02"), Method: "monthly", Remainder: "first", TimeZone: "UTC", AllowRefundedExpense: true, RequestId: "post-inst-bill-ok"})
	require.NoError(t, err)
	require.Equal(t, "100.00", billPlan.Data.OriginalPrincipal)
	require.Equal(t, "100.00", billPlan.Data.Principal)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "post-installment-refund.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, e := f.engine.Exec("VACUUM INTO ?", target)
		return e
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	sess := restored.NewSession()
	defer sess.Close()
	var restoredPlan models.CreditInstallment
	has, err = sess.ID(planA.Id).Get(&restoredPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "300.00", restoredPlan.Data.OriginalPrincipal)
	require.Equal(t, "210.00", restoredPlan.Data.Principal)
	var restoredRefunds []models.Refund
	require.NoError(t, sess.Where("uid=? AND expense_id=? AND deleted=?", f.uid, expenseA.TransactionId, false).Find(&restoredRefunds))
	require.Len(t, restoredRefunds, 1)
	require.Equal(t, "reduce", restoredRefunds[0].InstallmentMode)
	var restoredAccount models.Account
	has, err = sess.ID(8509).Get(&restoredAccount)
	require.NoError(t, err)
	require.True(t, has)
	restoredReport, err := creditReportInSession(sess, f.uid, restoredAccount, now)
	require.NoError(t, err)
	require.Equal(t, "350.00", restoredReport.Outstanding)
}
