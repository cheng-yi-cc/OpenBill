package services

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/datastore"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func refundTestActor(uid int64) RefundActor {
	return RefundActor{User: &models.User{Uid: uid, TransactionEditScope: models.TRANSACTION_EDIT_SCOPE_ALL}, ClientTimezone: time.UTC}
}

func TestRefundSchemaUpgradeAndBackupStagePreserveFacts(t *testing.T) {
	f, expense := refundFixture(t)
	// Simulate a pre-refund database, then run the same table sync as startup.
	_, err := f.engine.Exec(`DROP TABLE refund`)
	require.NoError(t, err)
	require.NoError(t, datastore.Container.UserDataStore.SyncStructs(new(models.Refund)))
	require.Equal(t, int64(1998800), f.balance())
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-backup-test"}
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "refund-backup.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, snapshotErr := f.engine.Exec("VACUUM INTO ?", target)
		return snapshotErr
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	db, err := sql.Open("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer db.Close()
	var expenseID, transactionID, cashAmount, version, balance int64
	var requestID string
	err = db.QueryRow(`SELECT r.expense_id, r.transaction_id, t.amount, r.version, r.request_id FROM refund r JOIN "transaction" t ON t.transaction_id=r.transaction_id WHERE r.id=? AND r.deleted=0 AND t.deleted=0`, summary.Refunds[0].Id).Scan(&expenseID, &transactionID, &cashAmount, &version, &requestID)
	require.NoError(t, err)
	require.Equal(t, expense.TransactionId, expenseID)
	require.Equal(t, summary.Refunds[0].TransactionId, fmt.Sprint(transactionID))
	require.Equal(t, int64(-500), cashAmount)
	require.Equal(t, int64(1), version)
	require.Equal(t, input.RequestId, requestID)
	require.NoError(t, db.QueryRow(`SELECT balance FROM account WHERE account_id=?`, f.bankID).Scan(&balance))
	require.Equal(t, int64(1999300), balance)
}

func TestRefundBackupRestoresExcessMetadataAndReplaysNet(t *testing.T) {
	f, expense := refundFixture(t)
	first := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-backup-first"}
	_, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), first)
	require.NoError(t, err)
	second := first
	second.Amount, second.Time, second.RequestId = "8", f.at+40*86400, "refund-backup-second"
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), second)
	require.NoError(t, err)
	tag := &models.TransactionTag{Uid: f.uid, Name: "备份标签"}
	require.NoError(t, TransactionTags.CreateTag(nil, tag))
	ref := summary.Refunds[1]
	summary, err = Investments.UpdateRefundExcess(nil, f.uid, refundTestActor(f.uid), ref.Id, RefundExcessEditInput{CategoryId: ref.Excess.CategoryId, BookId: ref.Excess.BookId, Comment: "备份差额", TagIds: []string{fmt.Sprint(tag.TagId)}, ExcludeFromStatistics: true, Version: ref.Version})
	require.NoError(t, err)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "refund-metadata.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, snapshotErr := f.engine.Exec("VACUUM INTO ?", target)
		return snapshotErr
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	session := restored.NewSession()
	defer session.Close()
	replayed, err := refundSummary(session, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "0.00", replayed.NetExpense)
	require.Equal(t, "1.00", replayed.ExcessIncome)
	require.Equal(t, summary.Refunds[1].Version, replayed.Refunds[1].Version)
	require.Equal(t, "备份差额", replayed.Refunds[1].Excess.Comment)
	require.Equal(t, []string{fmt.Sprint(tag.TagId)}, replayed.Refunds[1].Excess.TagIds)
	require.True(t, replayed.Refunds[1].Excess.ExcludeFromStatistics)
	var account models.Account
	has, err := restored.ID(f.bankID).Get(&account)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(2000100), account.Balance)
}

func refundFixture(t *testing.T) (*investmentDBFixture, *models.Transaction) {
	f := newInvestmentDBFixture(t)
	require.NoError(t, f.engine.Sync2(&models.Refund{}))
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	var category models.TransactionCategory
	has, err := f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_EXPENSE).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	expense := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 1200,
		CategoryId: category.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + 1)}
	require.NoError(t, Transactions.CreateTransaction(nil, expense, nil, nil))
	return f, expense
}

func TestRefundProtectsBothSidesFromLegacyMutations(t *testing.T) {
	f, expense := refundFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 7777, Uid: f.uid, Name: "other", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT, Currency: "CNY"})
	require.NoError(t, err)
	out, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: "7777", Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-guard-test"})
	require.NoError(t, err)
	cashID, err := strconv.ParseInt(out.Refunds[0].TransactionId, 10, 64)
	require.NoError(t, err)
	for _, id := range []int64{expense.TransactionId, cashID} {
		require.ErrorContains(t, Transactions.DeleteTransaction(nil, f.uid, id), "关联退款")
		var tx models.Transaction
		has, getErr := f.engine.ID(id).Get(&tx)
		require.NoError(t, getErr)
		require.True(t, has)
		tx.Comment = "changed"
		require.ErrorContains(t, Transactions.ModifyTransaction(nil, &tx, false, 0, nil, nil, nil, nil), "关联退款")
		require.ErrorContains(t, Transactions.BatchUpdateTransactionsCategory(nil, f.uid, []int64{id}, expense.CategoryId), "关联退款")
	}
	require.ErrorContains(t, Transactions.MoveAllTransactionsBetweenAccounts(nil, f.uid, f.bankID, 7777), "关联退款")
	beforeCount, err := f.engine.Where("uid=? AND deleted=?", f.uid, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.ErrorContains(t, Transactions.DeleteAllTransactionsOfAccount(nil, f.uid, f.bankID, 100), "关联退款")
	afterCount, err := f.engine.Where("uid=? AND deleted=?", f.uid, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Equal(t, beforeCount, afterCount, "account clearing must reject before deleting unrelated transactions")
	_, err = Investments.DeleteAssetAccount(nil, f.uid, AccountDeletionInput{ID: "7777", Kind: "cash"}, true)
	require.ErrorContains(t, err, "关联退款")
	require.Equal(t, int64(1998800), f.balance())
	remaining, err := Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "7.00", remaining.NetExpense)
}

func TestFullClearPreservesScheduledTemplatesWhenLedgerClearFails(t *testing.T) {
	f, expense := refundFixture(t)
	template := &models.TransactionTemplate{TemplateId: 9123, Uid: f.uid, TemplateType: models.TRANSACTION_TEMPLATE_TYPE_SCHEDULE, Name: "虚构周期账单", Type: models.TRANSACTION_TYPE_EXPENSE, AccountId: f.bankID, CategoryId: expense.CategoryId}
	_, err := f.engine.Insert(template)
	require.NoError(t, err)
	result, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "full-clear-refund-guard"})
	require.NoError(t, err)
	before := f.balance()
	require.ErrorContains(t, Transactions.DeleteAllTransactions(nil, f.uid, true), "关联退款")
	var stored models.TransactionTemplate
	has, err := f.engine.ID(template.TemplateId).Get(&stored)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, stored.Deleted)
	require.Equal(t, before, f.balance())
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), result.Refunds[0].Id, result.Refunds[0].Version)
	require.NoError(t, err)
	before = f.balance()
	_, err = f.engine.Exec(`CREATE TRIGGER reject_template_clear BEFORE UPDATE ON transaction_template BEGIN SELECT RAISE(ABORT, 'test template failure'); END`)
	require.NoError(t, err)
	require.Error(t, Transactions.DeleteAllTransactions(nil, f.uid, true))
	require.Equal(t, before, f.balance())
	stored = models.TransactionTemplate{}
	has, err = f.engine.ID(template.TemplateId).Get(&stored)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, stored.Deleted)
	var original models.Transaction
	has, err = f.engine.ID(expense.TransactionId).Get(&original)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, original.Deleted)
	_, err = f.engine.Exec(`DROP TRIGGER reject_template_clear`)
	require.NoError(t, err)
	require.NoError(t, Transactions.DeleteAllTransactions(nil, f.uid, true))
	has, err = f.engine.ID(template.TemplateId).Get(&stored)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, stored.Deleted)
}

func TestFullUserClearRollsBackAllDatabaseChangesOnLateFailure(t *testing.T) {
	f, expense := refundFixture(t)
	require.NoError(t, f.engine.Sync2(&models.TransactionTagGroup{}, &models.UserCustomIcon{}, &models.UserCustomExchangeRate{}, &models.InsightsExplorer{}))
	_, err := f.engine.Insert(&models.TransactionTemplate{TemplateId: 9124, Uid: f.uid, TemplateType: models.TRANSACTION_TEMPLATE_TYPE_SCHEDULE, Name: "虚构周期账单", Type: models.TRANSACTION_TYPE_EXPENSE, AccountId: f.bankID, CategoryId: expense.CategoryId})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.CalendarEvent{Id: "clear-event", Uid: f.uid, BookId: "book", Date: "2026-11-09", Kind: "manual"}, &models.CalendarEvent{Id: "other-event", Uid: f.uid + 1, BookId: "other", Date: "2026-11-09", Kind: "manual"})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.LocalLedgerItem{Id: "clear-wish", Uid: f.uid, Kind: "wish"}, &models.LocalLedgerItem{Id: "other-wish", Uid: f.uid + 1, Kind: "wish"})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.Account{AccountId: 9911, Uid: f.uid, Name: "系统结算", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_VIRTUAL, Currency: "CNY", SystemRole: InvestmentSettlementRole, Balance: 123})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.Transaction{TransactionId: 9912, Uid: f.uid, AccountId: 9911, Type: models.TRANSACTION_DB_TYPE_INCOME, Amount: 123, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + 2)})
	require.NoError(t, err)
	require.ErrorIs(t, Transactions.ClearAllUserData(nil, f.uid), ErrInvestmentLinked)
	require.Equal(t, int64(1998800), f.balance())
	_, err = f.engine.ID(9912).Delete(&models.Transaction{})
	require.NoError(t, err)
	refund, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "atomic-full-clear-refund"})
	require.NoError(t, err)
	beforeGuard := f.balance()
	require.ErrorContains(t, Transactions.ClearAllUserData(nil, f.uid), "关联退款")
	require.Equal(t, beforeGuard, f.balance())
	has, err := f.engine.ID("clear-wish").Exist(&models.LocalLedgerItem{})
	require.NoError(t, err)
	require.True(t, has)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), refund.Refunds[0].Id, refund.Refunds[0].Version)
	require.NoError(t, err)
	imported := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 250, CategoryId: expense.CategoryId, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + 4)}
	batch := &ImportBatch{Id: "atomic-clear-import", Fingerprint: "atomic-clear-hash"}
	require.NoError(t, Transactions.BatchCreateTransactions(nil, f.uid, []*models.Transaction{imported}, nil, nil, batch))
	before := f.balance()
	_, err = f.engine.Exec(`CREATE TRIGGER reject_calendar_clear BEFORE DELETE ON calendar_event BEGIN SELECT RAISE(ABORT, 'test calendar failure'); END`)
	require.NoError(t, err)
	require.Error(t, Transactions.ClearAllUserData(nil, f.uid))
	require.Equal(t, before, f.balance())
	var original models.Transaction
	has, err = f.engine.ID(expense.TransactionId).Get(&original)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, original.Deleted)
	var template models.TransactionTemplate
	has, err = f.engine.ID(9124).Get(&template)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, template.Deleted)
	var category models.TransactionCategory
	has, err = f.engine.ID(expense.CategoryId).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, category.Deleted)
	has, err = f.engine.ID("clear-wish").Exist(&models.LocalLedgerItem{})
	require.NoError(t, err)
	require.True(t, has)
	_, exists, err := LedgerWorkspace.ExistingImportBatch(nil, f.uid, batch.Id, batch.Fingerprint)
	require.NoError(t, err)
	require.True(t, exists)
	_, err = f.engine.Exec(`DROP TRIGGER reject_calendar_clear`)
	require.NoError(t, err)
	require.NoError(t, Transactions.ClearAllUserData(nil, f.uid))
	_, exists, err = LedgerWorkspace.ExistingImportBatch(nil, f.uid, batch.Id, batch.Fingerprint)
	require.True(t, exists)
	require.ErrorIs(t, err, ErrLedgerItemConflict)
	template = models.TransactionTemplate{}
	has, err = f.engine.ID(9124).Get(&template)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, template.Deleted)
	category = models.TransactionCategory{}
	has, err = f.engine.ID(expense.CategoryId).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, category.Deleted)
	has, err = f.engine.ID("clear-wish").Exist(&models.LocalLedgerItem{})
	require.NoError(t, err)
	require.False(t, has)
	has, err = f.engine.ID("clear-event").Exist(&models.CalendarEvent{})
	require.NoError(t, err)
	require.False(t, has)
	has, err = f.engine.ID("other-wish").Exist(&models.LocalLedgerItem{})
	require.NoError(t, err)
	require.True(t, has)
	has, err = f.engine.ID("other-event").Exist(&models.CalendarEvent{})
	require.NoError(t, err)
	require.True(t, has)
	var cleared models.Transaction
	has, err = f.engine.ID(expense.TransactionId).Get(&cleared)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, cleared.Deleted)
	var system models.Account
	has, err = f.engine.ID(9911).Get(&system)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, system.Deleted)
	require.Equal(t, int64(123), system.Balance)
}

func TestRefundProjectionIncludesFactsOutsideRequestedDates(t *testing.T) {
	f, expense := refundFixture(t)
	base := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "projection-first"}
	_, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), base)
	require.NoError(t, err)
	base.Amount, base.Time, base.RequestId = "8", f.at+40*86400, "projection-next-month"
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), base)
	require.NoError(t, err)
	laterID, err := strconv.ParseInt(summary.Refunds[1].TransactionId, 10, 64)
	require.NoError(t, err)
	originalOnly, err := Investments.RefundProjections(nil, f.uid, []int64{expense.TransactionId})
	require.NoError(t, err)
	require.Equal(t, "0.00", originalOnly[expense.TransactionId].NetExpense)
	require.Equal(t, "13.00", originalOnly[expense.TransactionId].RefundedAmount)
	laterOnly, err := Investments.RefundProjections(nil, f.uid, []int64{laterID})
	require.NoError(t, err)
	require.Equal(t, "7.00", laterOnly[laterID].OffsetAmount)
	require.Equal(t, "1.00", laterOnly[laterID].ExcessAmount)
	require.Equal(t, "cash", laterOnly[laterID].Kind)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), summary.Refunds[0].Id, summary.Refunds[0].Version)
	require.NoError(t, err)
	after, err := Investments.RefundProjections(nil, f.uid, []int64{expense.TransactionId, laterID})
	require.NoError(t, err)
	require.Equal(t, "4.00", after[expense.TransactionId].NetExpense)
	require.Equal(t, "8.00", after[laterID].OffsetAmount)
	require.Equal(t, "0.00", after[laterID].ExcessAmount)
	otherUser, err := Investments.RefundProjections(nil, f.uid+1, []int64{expense.TransactionId, laterID})
	require.NoError(t, err)
	require.Empty(t, otherUser)
}

func TestRefundHonorsEditScopeAndAccountReconciliation(t *testing.T) {
	f, expense := refundFixture(t)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-edit-scope"}
	denied := refundTestActor(f.uid)
	denied.User.TransactionEditScope = models.TRANSACTION_EDIT_SCOPE_NONE
	_, err := Investments.CreateRefund(nil, f.uid, denied, input)
	require.Error(t, err)
	_, err = Investments.CreateRefund(nil, f.uid, RefundActor{}, input)
	require.Error(t, err)
	require.Equal(t, int64(1998800), f.balance())
	out, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	ref := out.Refunds[0]
	lockedAt := f.at + 10
	_, err = f.engine.Insert(&models.Account{AccountId: 7777, Uid: f.uid, Name: "reconciled", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT, Currency: "CNY", Extend: &models.AccountExtend{LastReconciledTime: &lockedAt}})
	require.NoError(t, err)
	actor := refundTestActor(f.uid)
	actor.User.TransactionEditScope = models.TRANSACTION_EDIT_SCOPE_LAST_RECONCILED_TIME_OR_LATER
	actor.User.UseLastReconciledTime = true
	edit := RefundEditInput{AccountId: "7777", Amount: "8", Time: f.at + 4, TimeZone: "UTC", Version: ref.Version}
	_, err = Investments.UpdateRefund(nil, f.uid, actor, ref.Id, edit)
	require.ErrorContains(t, err, "对账限制")
	require.Equal(t, int64(1999300), f.balance(), "failed edit must preserve the prior refund")
	_, err = Investments.DeleteRefund(nil, f.uid, denied, ref.Id, ref.Version)
	require.ErrorContains(t, err, "编辑范围")
	out, err = Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, ref.TransactionId, out.Refunds[0].TransactionId)
}

func TestRefundDeleteExpenseReversesActualFlowsAcrossAccountsAtomically(t *testing.T) {
	f, expense := refundFixture(t)
	lockedAt := f.at + 100
	_, err := f.engine.Insert(&models.Account{AccountId: 7777, Uid: f.uid, Name: "other", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT, Currency: "CNY", Extend: &models.AccountExtend{LastReconciledTime: &lockedAt}})
	require.NoError(t, err)
	first := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-delete-parent-first"}
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), first)
	require.NoError(t, err)
	second := RefundInput{ExpenseId: first.ExpenseId, AccountId: "7777", Amount: "2", Time: f.at + 4, TimeZone: "UTC", RequestId: "refund-delete-parent-second"}
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), second)
	require.NoError(t, err)
	secondRef := summary.Refunds[1]
	summary, err = Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), secondRef.Id, RefundEditInput{AccountId: "7777", Amount: "8", Time: second.Time, TimeZone: "UTC", Version: secondRef.Version})
	require.NoError(t, err)
	require.Equal(t, "1.00", summary.ExcessIncome)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), summary.Refunds[0].Id, summary.Refunds[0].Version)
	require.NoError(t, err)
	actor := refundTestActor(f.uid)
	actor.User.TransactionEditScope = models.TRANSACTION_EDIT_SCOPE_LAST_RECONCILED_TIME_OR_LATER
	actor.User.UseLastReconciledTime = true
	err = Investments.DeleteExpenseAndRefunds(nil, f.uid, actor, expense.TransactionId)
	require.ErrorContains(t, err, "对账限制")
	require.Equal(t, int64(1998800), f.balance())
	before, err := Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "4.00", before.NetExpense)
	require.NoError(t, Investments.DeleteExpenseAndRefunds(nil, f.uid, refundTestActor(f.uid), expense.TransactionId))
	require.Equal(t, int64(2000000), f.balance())
	var other models.Account
	has, err := f.engine.ID(7777).Get(&other)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(0), other.Balance)
	var saved models.Transaction
	has, err = f.engine.ID(expense.TransactionId).Get(&saved)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, saved.Deleted)
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), second)
	require.ErrorIs(t, err, ErrInvestmentConflict)
}

func TestRefundExcessMetadataIndependentAndReferencesProtected(t *testing.T) {
	f, expense := refundFixture(t)
	expense.ExcludeFromStatistics = true
	require.NoError(t, Transactions.ModifyTransaction(nil, expense, false, 0, nil, nil, nil, nil))
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "13", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-excess-metadata"}
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	ref := summary.Refunds[0]
	require.Equal(t, "1.00", ref.ExcessAmount)
	require.NotNil(t, ref.Excess)
	require.False(t, ref.Excess.ExcludeFromStatistics, "excess income has its own statistics flag")
	require.Equal(t, "退款差额", ref.Excess.Comment)
	categoryID, err := strconv.ParseInt(ref.Excess.CategoryId, 10, 64)
	require.NoError(t, err)
	var category models.TransactionCategory
	has, err := f.engine.ID(categoryID).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, models.CATEGORY_TYPE_INCOME, category.Type)
	tag := &models.TransactionTag{Uid: f.uid, Name: "退款标签"}
	require.NoError(t, TransactionTags.CreateTag(nil, tag))
	edit := RefundExcessEditInput{CategoryId: ref.Excess.CategoryId, BookId: ref.Excess.BookId, Comment: "额外到账", TagIds: []string{fmt.Sprint(tag.TagId)}, ExcludeFromStatistics: true, ExcludeFromBudget: true, Version: ref.Version}
	invalid := edit
	invalid.CategoryId = fmt.Sprint(expense.CategoryId)
	_, err = Investments.UpdateRefundExcess(nil, f.uid, refundTestActor(f.uid), ref.Id, invalid)
	require.Error(t, err)
	before := f.balance()
	summary, err = Investments.UpdateRefundExcess(nil, f.uid, refundTestActor(f.uid), ref.Id, edit)
	require.NoError(t, err)
	require.Equal(t, before, f.balance())
	ref = summary.Refunds[0]
	require.Equal(t, "额外到账", ref.Excess.Comment)
	require.True(t, ref.Excess.ExcludeFromStatistics)
	require.True(t, ref.Excess.ExcludeFromBudget)
	require.Equal(t, edit.TagIds, ref.Excess.TagIds)
	_, err = Investments.UpdateRefundExcess(nil, f.uid, refundTestActor(f.uid), ref.Id, edit)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	require.ErrorIs(t, TransactionTags.DeleteTag(nil, f.uid, tag.TagId), errs.ErrTransactionTagInUseCannotBeDeleted)
	require.ErrorIs(t, TransactionCategories.DeleteCategory(nil, f.uid, categoryID), errs.ErrTransactionCategoryInUseCannotBeDeleted)
	require.NoError(t, TransactionTags.HideTag(nil, f.uid, []int64{tag.TagId}, true))
	require.NoError(t, TransactionCategories.HideCategory(nil, f.uid, []int64{categoryID}, true))
	edit.Version = ref.Version
	summary, err = Investments.UpdateRefundExcess(nil, f.uid, refundTestActor(f.uid), ref.Id, edit)
	require.NoError(t, err, "existing hidden references must survive a metadata revision")
	ref = summary.Refunds[0]
	summary, err = Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, RefundEditInput{AccountId: input.AccountId, Amount: "4", Time: input.Time, TimeZone: "UTC", Version: ref.Version})
	require.NoError(t, err)
	require.Equal(t, "0.00", summary.Refunds[0].ExcessAmount)
	require.ErrorIs(t, TransactionTags.DeleteTag(nil, f.uid, tag.TagId), errs.ErrTransactionTagInUseCannotBeDeleted)
	ref = summary.Refunds[0]
	summary, err = Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, RefundEditInput{AccountId: input.AccountId, Amount: "13", Time: input.Time, TimeZone: "UTC", Version: ref.Version})
	require.NoError(t, err)
	require.Equal(t, "额外到账", summary.Refunds[0].Excess.Comment)
	require.True(t, summary.Refunds[0].Excess.ExcludeFromStatistics)
	ref = summary.Refunds[0]
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, ref.Version)
	require.NoError(t, err)
	require.NoError(t, TransactionTags.DeleteTag(nil, f.uid, tag.TagId))
}

func TestRefundEarlierInsertionInitializesLaterExcess(t *testing.T) {
	f, expense := refundFixture(t)
	later := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "10", Time: f.at + 50, TimeZone: "UTC", RequestId: "refund-later-first"}
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), later)
	require.NoError(t, err)
	require.Nil(t, summary.Refunds[0].Excess)
	earlier := later
	earlier.Time, earlier.Amount, earlier.RequestId = f.at+5, "5", "refund-earlier-second"
	summary, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), earlier)
	require.NoError(t, err)
	require.Equal(t, "3.00", summary.Refunds[1].ExcessAmount)
	require.NotNil(t, summary.Refunds[1].Excess)
	require.False(t, summary.Refunds[1].Excess.ExcludeFromStatistics)
	require.Equal(t, "退款差额", summary.Refunds[1].Excess.Comment)
	cashID, err := strconv.ParseInt(summary.Refunds[1].TransactionId, 10, 64)
	require.NoError(t, err)
	projected, err := Investments.RefundProjections(nil, f.uid, []int64{cashID})
	require.NoError(t, err)
	require.Equal(t, "3.00", projected[cashID].ExcessAmount)
	require.Equal(t, summary.Refunds[1].Excess, projected[cashID].Excess)
}

func TestRefundNetExpenseReachesLegacyStatisticsOutsideRefundDate(t *testing.T) {
	f, expense := refundFixture(t)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 40*86400, TimeZone: "UTC", RequestId: "refund-legacy-statistics"}
	_, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	start, end := f.at, f.at+60
	income, expenseTotals, err := Transactions.GetAccountsTotalIncomeAndExpense(nil, f.uid, start, end, nil, nil, time.UTC, false)
	require.NoError(t, err)
	require.Empty(t, income)
	require.Equal(t, "700", expenseTotals[f.bankID].String())
	_, daily, err := Transactions.GetAccountsDailyIncomeAndExpense(nil, f.uid, start, end, nil, nil, time.UTC, false)
	require.NoError(t, err)
	day := utils.FormatUnixTimeToNumericYearMonthDay(f.at+1, time.UTC)
	require.Equal(t, "700", daily[day][f.bankID].String())
	grouped, err := Transactions.GetAccountsAndCategoriesTotalInflowAndOutflow(nil, f.uid, start, end, nil, false, "", core.MATCH_MODE_DEFAULT, time.UTC, false)
	require.NoError(t, err)
	require.Len(t, grouped, 1)
	require.Equal(t, "700", grouped[0].Amount.String())
	date := time.Unix(f.at+1, 0).UTC()
	year, month := int32(date.Year()), int32(date.Month())
	monthly, err := Transactions.GetAccountsAndCategoriesMonthlyInflowAndOutflow(nil, f.uid, year, month, year, month, nil, false, "", core.MATCH_MODE_DEFAULT, time.UTC, false)
	require.NoError(t, err)
	require.Len(t, monthly[year*100+month], 1)
	require.Equal(t, "700", monthly[year*100+month][0].Amount.String())
	require.Equal(t, int64(1999300), f.balance(), "report projection must not rewrite the account")
}

func TestRefundExpenseRevisionReplaysAndKeepsCashFacts(t *testing.T) {
	f, expense := refundFixture(t)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 30, TimeZone: "UTC", RequestId: "refund-revise-parent"}
	_, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	stored, err := Transactions.GetTransactionByTransactionId(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	stored.Amount = 400
	stored.ExcludeFromStatistics = true
	include := false
	stored.SetBudgetExclusion(&include)
	require.NoError(t, Transactions.ModifyRefundExpense(nil, refundTestActor(f.uid), stored, 0, nil, nil, nil, nil))
	require.Equal(t, int64(2000100), f.balance())
	summary, err := Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "0.00", summary.NetExpense)
	require.Equal(t, "1.00", summary.ExcessIncome)
	require.NotNil(t, summary.Refunds[0].Excess)
	require.False(t, summary.Refunds[0].Excess.ExcludeFromStatistics)
	stored, err = Transactions.GetTransactionByTransactionId(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.True(t, stored.ExcludeFromStatistics)
	require.False(t, stored.IsExcludedFromBudget())
	stored.Amount = 1500
	stored.TransactionTime = utils.GetMinTransactionTimeFromUnixTime(input.Time + 1)
	err = Transactions.ModifyRefundExpense(nil, refundTestActor(f.uid), stored, 0, nil, nil, nil, nil)
	require.ErrorContains(t, err, "不能晚于")
	require.Equal(t, int64(2000100), f.balance())
	stored.TransactionTime = expense.TransactionTime
	require.NoError(t, Transactions.ModifyRefundExpense(nil, refundTestActor(f.uid), stored, 0, nil, nil, nil, nil))
	require.Equal(t, int64(1999000), f.balance())
	summary, err = Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "10.00", summary.NetExpense)
	require.Equal(t, "0.00", summary.ExcessIncome)
	var cash models.Transaction
	_, err = f.engine.ID(summary.Refunds[0].TransactionId).Get(&cash)
	require.NoError(t, err)
	require.Equal(t, int64(-500), cash.Amount)
}

func TestRefundSiblingVersionsRejectStaleSplitAfterOtherChanges(t *testing.T) {
	f, expense := refundFixture(t)
	first := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-sibling-first"}
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), first)
	require.NoError(t, err)
	staleFirst := summary.Refunds[0]
	second := first
	second.Amount, second.Time, second.RequestId = "2", f.at+4, "refund-sibling-second"
	summary, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), second)
	require.NoError(t, err)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), staleFirst.Id, staleFirst.Version)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	staleSecond := summary.Refunds[1]
	firstRef := summary.Refunds[0]
	summary, err = Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), firstRef.Id, RefundEditInput{AccountId: first.AccountId, Amount: "11", Time: first.Time, TimeZone: "UTC", Version: firstRef.Version})
	require.NoError(t, err)
	require.Equal(t, "1.00", summary.Refunds[1].ExcessAmount)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), staleSecond.Id, staleSecond.Version)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	staleSecond = summary.Refunds[1]
	stored, err := Transactions.GetTransactionByTransactionId(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	stored.Amount = 1500
	require.NoError(t, Transactions.ModifyRefundExpense(nil, refundTestActor(f.uid), stored, 0, nil, nil, nil, nil))
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), staleSecond.Id, staleSecond.Version)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	require.Equal(t, int64(1999800), f.balance())
}

func TestRefundExpenseRevisionMovesAccountBookAndCategoryAtomically(t *testing.T) {
	f, expense := refundFixture(t)
	book, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "旅行"})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.Account{AccountId: 7777, Uid: f.uid, Name: "other", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT, Currency: "CNY"})
	require.NoError(t, err)
	var category models.TransactionCategory
	has, err := f.engine.Where("uid=? AND type=? AND parent_category_id>0 AND category_id<>?", f.uid, models.CATEGORY_TYPE_EXPENSE, expense.CategoryId).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "13", Time: f.at + 30, TimeZone: "UTC", RequestId: "refund-parent-move"}
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	oldExcessBook := summary.Refunds[0].Excess.BookId
	stored, err := Transactions.GetTransactionByTransactionId(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	stored.AccountId, stored.BookId, stored.CategoryId = 7777, book.Id, category.CategoryId
	require.NoError(t, Transactions.ModifyRefundExpense(nil, refundTestActor(f.uid), stored, 0, nil, nil, nil, nil))
	require.Equal(t, int64(2001300), f.balance())
	var target models.Account
	has, err = f.engine.ID(7777).Get(&target)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(-1200), target.Balance)
	summary, err = Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, oldExcessBook, summary.Refunds[0].Excess.BookId, "difference-income book is independently editable")
	var cash models.Transaction
	has, err = f.engine.ID(summary.Refunds[0].TransactionId).Get(&cash)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, f.bankID, cash.AccountId)
	require.Equal(t, int64(-1300), cash.Amount)
	require.Equal(t, book.Id, cash.BookId)
	require.Equal(t, category.CategoryId, cash.CategoryId)
}

func TestRefundExcessReportRespectsOwnBookTagsAndStatisticsFlag(t *testing.T) {
	f, expense := refundFixture(t)
	expense.ExcludeFromStatistics = true
	require.NoError(t, Transactions.ModifyTransaction(nil, expense, false, 0, nil, nil, nil, nil))
	book, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "旅行"})
	require.NoError(t, err)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "13", Time: f.at + 40*86400, TimeZone: "UTC", RequestId: "refund-excess-report"}
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	tag := &models.TransactionTag{Uid: f.uid, Name: "行程"}
	require.NoError(t, TransactionTags.CreateTag(nil, tag))
	ref := summary.Refunds[0]
	edit := RefundExcessEditInput{CategoryId: ref.Excess.CategoryId, BookId: book.Id, Comment: "补偿费用", TagIds: []string{fmt.Sprint(tag.TagId)}, Version: ref.Version}
	summary, err = Investments.UpdateRefundExcess(nil, f.uid, refundTestActor(f.uid), ref.Id, edit)
	require.NoError(t, err)
	from, to := utils.GetMinTransactionTimeFromUnixTime(input.Time-1), utils.GetMaxTransactionTimeFromUnixTime(input.Time+1)
	ctx, err := Books.FilterContext(nil, f.uid, book.Id)
	require.NoError(t, err)
	filters := []*models.TransactionTagFilter{{Type: models.TRANSACTION_TAG_FILTER_HAS_ALL, TagIds: []int64{tag.TagId}}}
	rows, err := Investments.RefundExcessReportRows(ctx, f.uid, from, to, filters, false, "补偿", core.MATCH_MODE_DEFAULT)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(100), rows[0].Amount)
	require.Equal(t, "CNY", rows[0].Currency)
	require.Equal(t, book.Id, rows[0].BookId)
	require.Equal(t, []int64{tag.TagId}, rows[0].TagIds)
	grouped, err := Transactions.GetAccountsAndCategoriesTotalInflowAndOutflow(ctx, f.uid, input.Time-1, input.Time+1, filters, false, "补偿", core.MATCH_MODE_DEFAULT, time.UTC, false)
	require.NoError(t, err)
	require.Len(t, grouped, 1)
	require.Equal(t, "100", grouped[0].Amount.String())
	require.Equal(t, int64(0), grouped[0].AccountId)
	require.True(t, grouped[0].Unassigned)
	require.Equal(t, "CNY", grouped[0].Currency)
	when := time.Unix(input.Time, 0).UTC()
	year, month := int32(when.Year()), int32(when.Month())
	monthly, err := Transactions.GetAccountsAndCategoriesMonthlyInflowAndOutflow(ctx, f.uid, year, month, year, month, filters, false, "补偿", core.MATCH_MODE_DEFAULT, time.UTC, false)
	require.NoError(t, err)
	require.Len(t, monthly[year*100+month], 1)
	require.True(t, monthly[year*100+month][0].Unassigned)
	require.Equal(t, "100", monthly[year*100+month][0].Amount.String())
	daily, err := Investments.RefundUnassignedDailyIncome(ctx, f.uid, input.Time-1, input.Time+1, nil, time.UTC, false)
	require.NoError(t, err)
	day := utils.FormatUnixTimeToNumericYearMonthDay(input.Time, time.UTC)
	require.Equal(t, "100", daily[day]["CNY"].String())
	daily, err = Investments.RefundUnassignedDailyIncome(ctx, f.uid, input.Time-1, input.Time+1, []int64{rows[0].CategoryId}, time.UTC, false)
	require.NoError(t, err)
	require.Empty(t, daily)
	rows, err = Investments.RefundExcessReportRows(ctx, f.uid, from, to, filters, false, "不存在", core.MATCH_MODE_DEFAULT)
	require.NoError(t, err)
	require.Empty(t, rows)
	rows, err = Investments.RefundExcessReportRows(ctx, f.uid, from, to, nil, true, "", core.MATCH_MODE_DEFAULT)
	require.NoError(t, err)
	require.Empty(t, rows)
	originalCtx, err := Books.FilterContext(nil, f.uid, expense.BookId)
	require.NoError(t, err)
	rows, err = Investments.RefundExcessReportRows(originalCtx, f.uid, from, to, nil, false, "", core.MATCH_MODE_DEFAULT)
	require.NoError(t, err)
	require.Empty(t, rows)
	edit.Version = summary.Refunds[0].Version
	edit.ExcludeFromStatistics = true
	_, err = Investments.UpdateRefundExcess(nil, f.uid, refundTestActor(f.uid), ref.Id, edit)
	require.NoError(t, err)
	rows, err = Investments.RefundExcessReportRows(ctx, f.uid, from, to, nil, false, "", core.MATCH_MODE_DEFAULT)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestRefundReplaysOverpaymentAndPreservesCashAndIdempotence(t *testing.T) {
	f, expense := refundFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 7777, Uid: f.uid, Name: "second", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT, Currency: "CNY"})
	require.NoError(t, err)
	first := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5.00", Time: f.at + 3, TimeZone: "Asia/Shanghai", RequestId: "refund-first"}
	one, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), first)
	require.NoError(t, err)
	require.Equal(t, "7.00", one.NetExpense)
	require.Equal(t, int64(1999300), f.balance())
	retry, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), first)
	require.NoError(t, err)
	require.Equal(t, one.Refunds[0].Id, retry.Refunds[0].Id)
	require.Equal(t, int64(1999300), f.balance())
	changed := first
	changed.Amount = "6"
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), changed)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	second := RefundInput{ExpenseId: first.ExpenseId, AccountId: "7777", Amount: "8", Time: f.at + 86400, TimeZone: "Asia/Shanghai", RequestId: "refund-second"}
	two, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), second)
	require.NoError(t, err)
	require.Equal(t, "0.00", two.NetExpense)
	require.Equal(t, "1.00", two.ExcessIncome)
	require.Equal(t, "7.00", two.Refunds[1].OffsetAmount)
	require.Equal(t, "1.00", two.Refunds[1].ExcessAmount)
	require.Equal(t, second.Time, two.Refunds[1].Time)
	var other models.Account
	has, err := f.engine.ID(7777).Get(&other)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(800), other.Balance)
	two, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), two.Refunds[0].Id, two.Refunds[0].Version)
	require.NoError(t, err)
	require.Equal(t, "4.00", two.NetExpense)
	require.Equal(t, "0.00", two.ExcessIncome)
	require.Equal(t, "8.00", two.Refunds[0].OffsetAmount)
	require.Equal(t, int64(1998800), f.balance())
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), first)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	_, err = Investments.GetRefunds(nil, f.uid+1, expense.TransactionId)
	require.Error(t, err)
}

func TestRefundEditRollbackVersionAndHistory(t *testing.T) {
	f, expense := refundFixture(t)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 2, TimeZone: "Asia/Shanghai", RequestId: "refund-edit-test"}
	out, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	ref := out.Refunds[0]
	_, err = f.engine.Insert(&models.WealthSnapshot{Id: "refund-snapshot", Uid: f.uid, RecordedAt: time.Now().Unix(), Payload: "{}"})
	require.NoError(t, err)
	edit := RefundEditInput{AccountId: "999999", Amount: "8", Time: f.at + 3, TimeZone: "Asia/Shanghai", Version: ref.Version}
	_, err = Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, edit)
	require.Error(t, err)
	require.Equal(t, int64(1999300), f.balance(), "failed edit must roll back the old refund reversal")
	out, err = Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, ref.TransactionId, out.Refunds[0].TransactionId)
	edit.AccountId = fmt.Sprint(f.bankID)
	out, err = Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, edit)
	require.NoError(t, err)
	require.Equal(t, "4.00", out.NetExpense)
	require.Equal(t, int64(1999600), f.balance())
	require.NotEqual(t, ref.TransactionId, out.Refunds[0].TransactionId)
	require.Equal(t, int64(2), out.Refunds[0].Version)
	var snapshot models.WealthSnapshot
	_, err = f.engine.ID("refund-snapshot").Get(&snapshot)
	require.NoError(t, err)
	require.True(t, snapshot.Invalidated)
	_, err = Investments.UpdateRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, edit)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	_, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, ref.Version)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	out, err = Investments.DeleteRefund(nil, f.uid, refundTestActor(f.uid), ref.Id, 2)
	require.NoError(t, err)
	require.Equal(t, "12.00", out.NetExpense)
	require.Equal(t, int64(1998800), f.balance())
}

func TestRefundRejectsWrongCurrencyAndManagedExpenses(t *testing.T) {
	f, expense := refundFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 8888, Uid: f.uid, Name: "USD", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CHECKING_ACCOUNT, Currency: "USD"})
	require.NoError(t, err)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: "8888", Amount: "2", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-bad-currency"}
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.Error(t, err)
	input.AccountId = fmt.Sprint(f.bankID)
	input.Time = f.at
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.Error(t, err)
	input.Time = f.at + 3
	_, err = f.engine.Insert(&models.DebtMovement{Id: "debt", Uid: f.uid, PrincipalTransactionId: expense.TransactionId})
	require.NoError(t, err)
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.Error(t, err)
	require.Equal(t, int64(1998800), f.balance())
}

func TestRefundConcurrentRetryCreatesOneCashMovement(t *testing.T) {
	f, expense := refundFixture(t)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-concurrent-test"}
	var wg sync.WaitGroup
	results := make(chan *RefundSummary, 2)
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
			results <- result
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	var id string
	for result := range results {
		require.Len(t, result.Refunds, 1)
		if id == "" {
			id = result.Refunds[0].Id
		} else {
			require.Equal(t, id, result.Refunds[0].Id)
		}
	}
	require.Equal(t, int64(1999300), f.balance())
	require.Equal(t, int64(1), f.count(&models.Refund{}))
}

func TestRefundCreateRejectsStalePreviewBeforeMovingCash(t *testing.T) {
	f, expense := refundFixture(t)
	input := RefundInput{ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "8", Time: f.at + 4, TimeZone: "UTC", RequestId: "refund-stale-preview", ExpectedOriginalAmount: "12.00", ExpectedRefundedAmount: "0.00"}
	other := RefundInput{ExpenseId: input.ExpenseId, AccountId: input.AccountId, Amount: "5", Time: f.at + 3, TimeZone: "UTC", RequestId: "refund-other-first"}
	_, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), other)
	require.NoError(t, err)
	_, err = Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	require.Equal(t, int64(1999300), f.balance())
	input.ExpectedRefundedAmount = "5"
	result, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), input)
	require.NoError(t, err)
	require.Equal(t, "1.00", result.ExcessIncome)
	require.Equal(t, int64(2000100), f.balance())
}
