package services

import (
	"fmt"
	"github.com/mayswind/ezbookkeeping/pkg/investments"
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

func TestMonetaryHistoryBackupStageRetainsFlowGuard(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "money-history.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, snapshotErr := f.engine.Exec("VACUUM INTO ?", target)
		return snapshotErr
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-29 12:00", zone)
	require.NoError(t, err)
	tx := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 100000, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at.Unix())}
	sess := restored.NewSession()
	defer sess.Close()
	require.ErrorIs(t, guardMonetaryFlowHistory(sess, f.uid, nil, tx), ErrMonetaryFlowHistory)
	var day models.MonetaryIncomeDay
	has, err := sess.Where("uid=? AND account_id=?", f.uid, f.bankID).Get(&day)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "20000.00", day.Principal)
	require.Equal(t, "2.00", day.Amount)
}

func TestMonetaryHistoryReproBackdatedExpense(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := f.engine.Insert(&models.TransactionCategory{CategoryId: 9200, Uid: f.uid, Name: "支出", Type: models.CATEGORY_TYPE_EXPENSE}, &models.TransactionCategory{CategoryId: 9201, Uid: f.uid, ParentCategoryId: 9200, Name: "其他", Type: models.CATEGORY_TYPE_EXPENSE})
	require.NoError(t, err)
	_, err = MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-29 12:00", zone)
	require.NoError(t, err)
	tx := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 100000, CategoryId: 9201, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at.Unix())}
	beforeBalance := f.balance()
	err = Transactions.CreateTransaction(nil, tx, nil, nil)
	if err != nil {
		require.ErrorIs(t, err, ErrMonetaryFlowHistory)
		require.Equal(t, beforeBalance, f.balance(), "rejected history must leave balance untouched")
		var rejected models.Transaction
		found, queryErr := f.engine.ID(tx.TransactionId).Where("uid=? AND deleted=?", f.uid, false).Get(&rejected)
		require.NoError(t, queryErr)
		require.False(t, found, "rejected history must not leave a transaction")
		return
	}
	require.NoError(t, err)
	var day models.MonetaryIncomeDay
	found, err := f.engine.Where("uid=? AND account_id=? AND date=?", f.uid, f.bankID, "2026-09-29").Get(&day)
	require.NoError(t, err)
	require.True(t, found)
	t.Logf("after backdated outflow, recorded principal=%s, recalculated principal should be 19000.00, recorded amount=%s", day.Principal, day.Amount)
	require.Equal(t, "19000.00", day.Principal, "history must not silently retain an inconsistent principal")
}

func TestMonetaryHistoryConcurrentSettlementAndExpense(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := f.engine.Insert(&models.TransactionCategory{CategoryId: 9200, Uid: f.uid, Name: "支出", Type: models.CATEGORY_TYPE_EXPENSE}, &models.TransactionCategory{CategoryId: 9201, Uid: f.uid, ParentCategoryId: 9200, Name: "其他", Type: models.CATEGORY_TYPE_EXPENSE})
	require.NoError(t, err)
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-29 12:00", zone)
	require.NoError(t, err)
	tx := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 100000, CategoryId: 9201, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at.Unix())}
	var settlementErr, expenseErr error
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		_, settlementErr = MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	}()
	go func() { defer group.Done(); expenseErr = Transactions.CreateTransaction(nil, tx, nil, nil) }()
	group.Wait()
	require.NoError(t, settlementErr)
	var day models.MonetaryIncomeDay
	has, err := f.engine.Where("uid=? AND account_id=?", f.uid, f.bankID).Get(&day)
	require.NoError(t, err)
	require.True(t, has)
	if expenseErr == nil {
		require.Equal(t, "19000.00", day.Principal)
		require.Equal(t, "1.90", day.Amount)
		require.Equal(t, int64(1900190), f.balance())
	} else {
		require.ErrorIs(t, expenseErr, ErrMonetaryFlowHistory)
		require.Equal(t, "20000.00", day.Principal)
		require.Equal(t, "2.00", day.Amount)
		require.Equal(t, int64(2000200), f.balance())
	}
}

func TestMonetaryHistoryAccountClearKeepsDayIdentityAndPausesBinding(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	require.NoError(t, Transactions.DeleteAllTransactionsOfAccount(nil, f.uid, f.bankID, 100))
	require.Zero(t, f.balance())
	var binding models.MonetaryIncomeBinding
	has, err := f.engine.ID(b.Id).Get(&binding)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, binding.Enabled)
	var day models.MonetaryIncomeDay
	has, err = f.engine.Where("uid=? AND account_id=?", f.uid, f.bankID).Get(&day)
	require.NoError(t, err)
	require.True(t, has, "clear must retain the processed identity")
	var tx models.Transaction
	has, err = f.engine.ID(day.TransactionId).Get(&tx)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, tx.Deleted)
}

func TestMonetaryHistoryInvestmentVoidRollsBack(t *testing.T) {
	f, b := monetaryFixture(t)
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-29 12:00", zone)
	require.NoError(t, err)
	event := f.event(investments.Buy, "0.01", "1000", "0", 0)
	event.OccurredAt = at.Unix()
	buy := f.create(event, "monetary-history-investment-buy")
	_, err = MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	var day models.MonetaryIncomeDay
	has, err := f.engine.Where("uid=? AND account_id=?", f.uid, f.bankID).Get(&day)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "19000.00", day.Principal)
	beforeBalance := f.balance()
	_, err = f.s.Mutate(nil, f.uid, buy.Event, "", "void", false)
	require.ErrorIs(t, err, ErrMonetaryFlowHistory)
	require.Equal(t, beforeBalance, f.balance())
	events, err := f.s.Events(nil, f.uid)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.False(t, events[0].Voided)
	require.Equal(t, buy.Event.Version, events[0].Version)
}

func TestMonetaryHistoryRefundReversalRollsBack(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := f.engine.Insert(&models.TransactionCategory{CategoryId: 9200, Uid: f.uid, Name: "支出", Type: models.CATEGORY_TYPE_EXPENSE}, &models.TransactionCategory{CategoryId: 9201, Uid: f.uid, ParentCategoryId: 9200, Name: "其他", Type: models.CATEGORY_TYPE_EXPENSE})
	require.NoError(t, err)
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 12:00", zone)
	require.NoError(t, err)
	expense := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 1200, CategoryId: 9201, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at.Unix())}
	require.NoError(t, Transactions.CreateTransaction(nil, expense, nil, nil))
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: at.Add(time.Hour).Unix(), TimeZone: "Asia/Shanghai", RequestId: "history-refund"}
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	_, err = MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	var day models.MonetaryIncomeDay
	has, err := f.engine.Where("uid=? AND account_id=?", f.uid, f.bankID).Get(&day)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "19993.00", day.Principal)
	beforeBalance := f.balance()
	ref := summary.Refunds[0]
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, ref.Version)
	require.ErrorIs(t, err, ErrMonetaryFlowHistory)
	require.Equal(t, beforeBalance, f.balance())
	after, err := Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "7.00", after.NetExpense)
	require.Len(t, after.Refunds, 1)
}

func TestMonetaryHistoryAccountDeletionProtectsSurvivingTransferAccount(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 2002, Uid: f.uid, Name: "对端", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CASH, Currency: "CNY"}, &models.TransactionCategory{CategoryId: 9300, Uid: f.uid, Type: models.CATEGORY_TYPE_TRANSFER, Name: "转账"}, &models.TransactionCategory{CategoryId: 9301, Uid: f.uid, Type: models.CATEGORY_TYPE_TRANSFER, ParentCategoryId: 9300, Name: "互转"})
	require.NoError(t, err)
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-29 12:00", zone)
	require.NoError(t, err)
	tx := &models.Transaction{Uid: f.uid, Type: models.TRANSACTION_DB_TYPE_TRANSFER_OUT, CategoryId: 9301, AccountId: f.bankID, RelatedAccountId: 2002, Amount: 100000, RelatedAccountAmount: 100000, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at.Unix())}
	require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))
	_, err = MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	input := AccountDeletionInput{ID: "2002", Kind: "cash", DeleteRelated: true}
	preview := deletionPreview(t, f, input)
	require.NotEmpty(t, preview.BlockedReason, "removing the opposite account cannot rewrite a surviving account's settled principal")
	input.Token = preview.Token
	beforeBalance := f.balance()
	err = Transactions.DeleteAllTransactionsOfAccount(nil, f.uid, 2002, 100)
	require.ErrorIs(t, err, ErrMonetaryFlowHistory, "clearing the other account must protect the surviving account")
	require.Equal(t, beforeBalance, f.balance())
	_, err = f.s.DeleteAssetAccount(nil, f.uid, input, false)
	require.Error(t, err)
	require.Equal(t, beforeBalance, f.balance())
	var account models.Account
	has, err := f.engine.ID(2002).Get(&account)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, account.Deleted)
	var day models.MonetaryIncomeDay
	has, err = f.engine.Where("uid=? AND account_id=?", f.uid, f.bankID).Get(&day)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "19000.00", day.Principal)
}

func TestMonetaryHistoryTransactionEditsAndDeletionAreAtomic(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := f.engine.Insert(&models.TransactionCategory{CategoryId: 9200, Uid: f.uid, Name: "支出", Type: models.CATEGORY_TYPE_EXPENSE}, &models.TransactionCategory{CategoryId: 9201, Uid: f.uid, ParentCategoryId: 9200, Name: "其他", Type: models.CATEGORY_TYPE_EXPENSE})
	require.NoError(t, err)
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-29 12:00", zone)
	require.NoError(t, err)
	tx := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 100000, CategoryId: 9201, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at.Unix())}
	require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))
	_, err = MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	var stored models.Transaction
	has, err := f.engine.ID(tx.TransactionId).Get(&stored)
	require.NoError(t, err)
	require.True(t, has)
	changed := stored
	changed.Comment = "仅修改备注"
	require.NoError(t, Transactions.ModifyTransaction(nil, &changed, false, 0, nil, nil, nil, nil))
	beforeBalance := f.balance()
	changed.Amount = 120000
	require.ErrorIs(t, Transactions.ModifyTransaction(nil, &changed, false, 0, nil, nil, nil, nil), ErrMonetaryFlowHistory)
	require.ErrorIs(t, Transactions.DeleteTransaction(nil, f.uid, tx.TransactionId), ErrMonetaryFlowHistory)
	require.Equal(t, beforeBalance, f.balance())
	stored = models.Transaction{}
	has, err = f.engine.ID(tx.TransactionId).Where("deleted=?", false).Get(&stored)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(100000), stored.Amount)
	require.Equal(t, "仅修改备注", stored.Comment)
	var day models.MonetaryIncomeDay
	has, err = f.engine.Where("uid=? AND date=?", f.uid, "2026-09-29").Get(&day)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "19000.00", day.Principal)
	newTx := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 100, CategoryId: 9201, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at.AddDate(0, 0, 1).Unix())}
	require.NoError(t, Transactions.CreateTransaction(nil, newTx, nil, nil))
}

func TestMonetaryHistoryCashDirectionAndTradingCutoffs(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	timeAt := func(value string) int64 {
		t.Helper()
		at, parseErr := time.ParseInLocation("2006-01-02 15:04", value, zone)
		require.NoError(t, parseErr)
		return utils.GetMinTransactionTimeFromUnixTime(at.Unix())
	}
	rows := []struct {
		name    string
		tx      models.Transaction
		blocked bool
	}{
		{"income before cutoff", models.Transaction{Type: models.TRANSACTION_DB_TYPE_INCOME, AccountId: f.bankID, Amount: 100, TransactionTime: timeAt("2026-09-28 14:59")}, true},
		{"income at cutoff", models.Transaction{Type: models.TRANSACTION_DB_TYPE_INCOME, AccountId: f.bankID, Amount: 100, TransactionTime: timeAt("2026-09-28 15:00")}, false},
		{"expense at end of day", models.Transaction{Type: models.TRANSACTION_DB_TYPE_EXPENSE, AccountId: f.bankID, Amount: 100, TransactionTime: timeAt("2026-09-29 23:59")}, true},
		{"expense next day", models.Transaction{Type: models.TRANSACTION_DB_TYPE_EXPENSE, AccountId: f.bankID, Amount: 100, TransactionTime: timeAt("2026-09-30 00:00")}, false},
		{"refund cash uses inflow cutoff", models.Transaction{Type: models.TRANSACTION_DB_TYPE_EXPENSE, AccountId: f.bankID, Amount: -100, TransactionTime: timeAt("2026-09-29 12:00")}, false},
		{"transfer source", models.Transaction{Type: models.TRANSACTION_DB_TYPE_TRANSFER_OUT, AccountId: f.bankID, RelatedAccountId: 2002, Amount: 100, RelatedAccountAmount: 100, TransactionTime: timeAt("2026-09-29 12:00")}, true},
		{"transfer destination after inflow cutoff", models.Transaction{Type: models.TRANSACTION_DB_TYPE_TRANSFER_OUT, AccountId: 2002, RelatedAccountId: f.bankID, Amount: 100, RelatedAccountAmount: 100, TransactionTime: timeAt("2026-09-29 12:00")}, false},
	}
	sess := f.engine.NewSession()
	defer sess.Close()
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			err := guardMonetaryFlowHistory(sess, f.uid, nil, &row.tx)
			if row.blocked {
				require.ErrorIs(t, err, ErrMonetaryFlowHistory)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, guardMonetaryFlowHistory(sess, f.uid+1, nil, &row.tx), "other users' processed days must not affect this user")
		})
	}
}

func TestMonetaryHistoryTransferFeeMetadataEditPreservesCash(t *testing.T) {
	f, b := monetaryFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 2002, Uid: f.uid, Name: "对端", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CASH, Currency: "CNY"}, &models.TransactionCategory{CategoryId: 9300, Uid: f.uid, Type: models.CATEGORY_TYPE_TRANSFER, Name: "转账"}, &models.TransactionCategory{CategoryId: 9301, Uid: f.uid, Type: models.CATEGORY_TYPE_TRANSFER, ParentCategoryId: 9300, Name: "互转"}, &models.TransactionCategory{CategoryId: 9200, Uid: f.uid, Name: "支出", Type: models.CATEGORY_TYPE_EXPENSE}, &models.TransactionCategory{CategoryId: 9201, Uid: f.uid, ParentCategoryId: 9200, Name: "手续费", Type: models.CATEGORY_TYPE_EXPENSE})
	require.NoError(t, err)
	zone, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	at, err := time.ParseInLocation("2006-01-02 15:04", "2026-09-29 12:00", zone)
	require.NoError(t, err)
	tx := &models.Transaction{Uid: f.uid, Type: models.TRANSACTION_DB_TYPE_TRANSFER_OUT, CategoryId: 9301, AccountId: f.bankID, RelatedAccountId: 2002, Amount: 100000, RelatedAccountAmount: 100000, TransferFeeAmount: "1.00", TransferFeeCategoryId: 9201, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at.Unix())}
	require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))
	_, err = MonetaryIncome.settleDay(nil, f.uid, b.Id, b.NextDate, "1")
	require.NoError(t, err)
	var stored, fee models.Transaction
	has, err := f.engine.ID(tx.TransactionId).Get(&stored)
	require.NoError(t, err)
	require.True(t, has)
	has, err = f.engine.Where("uid=? AND transfer_fee_parent_id=? AND deleted=?", f.uid, tx.TransactionId, false).Get(&fee)
	require.NoError(t, err)
	require.True(t, has)
	beforeBalance := f.balance()
	stored.Comment = "仅改转账备注"
	require.NoError(t, Transactions.ModifyTransaction(nil, &stored, false, 0, nil, nil, nil, nil))
	require.Equal(t, beforeBalance, f.balance())
	var after models.Transaction
	has, err = f.engine.ID(fee.TransactionId).Where("deleted=?", false).Get(&after)
	require.NoError(t, err)
	require.True(t, has, "unchanged fee cash must retain its identity")
	require.Equal(t, fee.Amount, after.Amount)
	stored.HideAmount = true
	stored.ExcludeFromStatistics = true
	require.NoError(t, Transactions.ModifyTransaction(nil, &stored, false, 0, nil, nil, nil, nil))
	after = models.Transaction{}
	has, err = f.engine.ID(fee.TransactionId).Where("deleted=?", false).Get(&after)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, after.HideAmount)
	require.True(t, after.ExcludeFromStatistics)
	require.Equal(t, beforeBalance, f.balance())
	_, err = f.engine.Insert(&models.TransactionCategory{CategoryId: 9202, Uid: f.uid, ParentCategoryId: 9200, Name: "其他手续费", Type: models.CATEGORY_TYPE_EXPENSE})
	require.NoError(t, err)
	stored.TransferFeeCategoryId = 9202
	require.NoError(t, Transactions.ModifyTransaction(nil, &stored, false, 0, nil, nil, nil, nil))
	stored.TransferFeeCategoryId = 99999
	require.Error(t, Transactions.ModifyTransaction(nil, &stored, false, 0, nil, nil, nil, nil))
	after = models.Transaction{}
	has, err = f.engine.ID(fee.TransactionId).Where("deleted=?", false).Get(&after)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(9202), after.CategoryId, "invalid category must roll back without changing fee")
	require.Equal(t, beforeBalance, f.balance())
}
