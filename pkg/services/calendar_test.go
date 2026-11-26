package services

import (
	"fmt"
	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xorm.io/xorm"
)

func TestCalendarDueLifecycleDoesNotChangeLedger(t *testing.T) {
	f := newInvestmentDBFixture(t)
	require.NoError(t, f.engine.Sync2(&models.CalendarEvent{}))
	_, err := f.engine.Insert(&models.Account{AccountId: 2001, Uid: f.uid, Name: "信用卡", Type: 1, Category: 3, Currency: "CNY"}, &models.Account{AccountId: 2002, Uid: f.uid, Name: "定存", Type: 1, Category: 9, Currency: "USD"})
	require.NoError(t, err)
	balance, count := f.balance(), f.count(&models.Transaction{})
	req := models.CalendarSaveRequest{BookId: DefaultBookID(f.uid), AccountId: "2001", Kind: "repayment", Date: "2026-10-02", Amount: "123.40", Note: "分期"}
	saved, err := Calendar.Save(nil, f.uid, req)
	require.NoError(t, err)
	require.Equal(t, "123.40", saved.Amount)
	require.Equal(t, "CNY", saved.Currency)
	req.Id = saved.Id
	req.Date = "2026-11-03"
	req.Amount = "23.40"
	_, err = Calendar.Save(nil, f.uid, req)
	require.NoError(t, err)
	october, err := Calendar.List(nil, f.uid, "2026-10")
	require.NoError(t, err)
	require.Empty(t, october)
	november, err := Calendar.List(nil, f.uid, "2026-11")
	require.NoError(t, err)
	require.Len(t, november, 1)
	require.Equal(t, "23.40", november[0].Amount)
	_, err = Calendar.SetCompleted(nil, f.uid, saved.Id, true)
	require.NoError(t, err)
	november, err = Calendar.List(nil, f.uid, "2026-11")
	require.NoError(t, err)
	require.True(t, november[0].Completed)
	_, err = Calendar.SetCompleted(nil, f.uid, saved.Id, false)
	require.NoError(t, err)
	req.Id = ""
	req.AccountId = "2002"
	req.Kind = "deposit"
	req.Date = "2028-02-29"
	deposit, err := Calendar.Save(nil, f.uid, req)
	require.NoError(t, err)
	require.Equal(t, "USD", deposit.Currency)
	_, err = Calendar.Delete(nil, f.uid, saved.Id)
	require.NoError(t, err)
	november, err = Calendar.List(nil, f.uid, "2026-11")
	require.NoError(t, err)
	require.Empty(t, november)
	require.Equal(t, balance, f.balance())
	require.Equal(t, count, f.count(&models.Transaction{}))
}

func TestCalendarOwnershipAndValidation(t *testing.T) {
	f := newInvestmentDBFixture(t)
	require.NoError(t, f.engine.Sync2(&models.CalendarEvent{}))
	_, err := f.engine.Insert(&models.Account{AccountId: 2001, Uid: f.uid, Name: "借入", Type: 1, Category: 5, Currency: "CNY"}, &models.Account{AccountId: 2002, Uid: f.uid + 1, Name: "other", Type: 1, Category: 3, Currency: "CNY"}, &models.Account{AccountId: 2003, Uid: f.uid, Type: 1, Category: 3, SystemRole: "investment_settlement"})
	require.NoError(t, err)
	base := models.CalendarSaveRequest{BookId: DefaultBookID(f.uid), AccountId: "2001", Kind: "repayment", Date: "2026-10-02", Amount: "1.01"}
	saved, err := Calendar.Save(nil, f.uid, base)
	require.NoError(t, err)
	for _, value := range []string{"0", "-1", "1e2", "NaN", "1.001", "01", "99999999999999", strings.Repeat("9", 100)} {
		req := base
		req.Amount = value
		_, err = Calendar.Save(nil, f.uid, req)
		require.Error(t, err, value)
	}
	for _, value := range []string{"2026-02-29", "2026-13-01", "2026-1-01", "1899-01-01"} {
		req := base
		req.Date = value
		_, err = Calendar.Save(nil, f.uid, req)
		require.Error(t, err, value)
	}
	for _, id := range []string{"2002", "2003", fmt.Sprint(f.bankID), "99999"} {
		req := base
		req.AccountId = id
		_, err = Calendar.Save(nil, f.uid, req)
		require.Error(t, err, id)
	}
	req := base
	req.Kind = "deposit"
	_, err = Calendar.Save(nil, f.uid, req)
	require.Error(t, err)
	otherBook, err := Books.Create(nil, f.uid+1, models.BookCreateRequest{Name: "other"})
	require.NoError(t, err)
	req = base
	req.BookId = otherBook.Id
	_, err = Calendar.Save(nil, f.uid, req)
	require.Error(t, err)
	req = base
	req.Id = saved.Id
	_, err = Calendar.Save(nil, f.uid+1, req)
	require.Error(t, err)
	_, err = Calendar.SetCompleted(nil, f.uid+1, saved.Id, true)
	require.Error(t, err)
	_, err = Calendar.Delete(nil, f.uid+1, saved.Id)
	require.Error(t, err)
	other, err := Calendar.List(nil, f.uid+1, "2026-10")
	require.NoError(t, err)
	require.Empty(t, other)
	rows, err := Calendar.List(nil, f.uid, "2026-10")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.False(t, rows[0].Completed)
	req = base
	req.BookId = otherBook.Id
	req.AccountId = "2002"
	_, err = Calendar.Save(nil, f.uid+1, req)
	require.NoError(t, err)
	require.NoError(t, Calendar.DeleteAll(nil, f.uid))
	rows, err = Calendar.List(nil, f.uid, "2026-10")
	require.NoError(t, err)
	require.Empty(t, rows)
	other, err = Calendar.List(nil, f.uid+1, "2026-10")
	require.NoError(t, err)
	require.Len(t, other, 1)
}

func TestCalendarCannotDetachFixedDepositReminder(t *testing.T) {
	f := newInvestmentDBFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 2002, Uid: f.uid, Name: "定存", Type: 1, Category: 9, Currency: "CNY"})
	require.NoError(t, err)
	deposit, err := f.s.SaveFixedDeposit(nil, f.uid, models.FixedDeposit{AccountId: "2002", BookId: DefaultBookID(f.uid), Principal: "100.00", AnnualRate: "2", StartDate: "2026-11-10", Term: 1, Unit: "year", TimeZone: "Asia/Shanghai", Note: "测试定存"})
	require.NoError(t, err)
	before, err := Calendar.List(nil, f.uid, "2027-11")
	require.NoError(t, err)
	require.Len(t, before, 1)
	balance := f.balance()
	req := models.CalendarSaveRequest{Id: deposit.Id, BookId: deposit.BookId, AccountId: deposit.AccountId, Kind: "deposit", Date: "2027-11-11", Amount: "1.00"}
	_, err = Calendar.Save(nil, f.uid, req)
	require.Error(t, err)
	_, err = Calendar.SetCompleted(nil, f.uid, deposit.Id, true)
	require.Error(t, err)
	_, err = Calendar.Delete(nil, f.uid, deposit.Id)
	require.Error(t, err)
	after, err := Calendar.List(nil, f.uid, "2027-11")
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, balance, f.balance())
	deposit.Principal = "150.00"
	deposit, err = f.s.SaveFixedDeposit(nil, f.uid, *deposit)
	require.NoError(t, err)
	after, err = Calendar.List(nil, f.uid, "2027-11")
	require.NoError(t, err)
	require.Equal(t, "150.00", after[0].Amount)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "deposit.zip")
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
	var restoredEvent models.CalendarEvent
	has, err := sess.ID(deposit.Id).Where("uid=?", f.uid).Get(&restoredEvent)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "150.00", restoredEvent.Amount)
	require.Error(t, guardFixedDepositCalendarEvent(sess, f.uid, deposit.Id))
	_, err = f.s.CloseFixedDeposit(nil, f.uid, deposit.Id)
	require.NoError(t, err)
	after, err = Calendar.List(nil, f.uid, "2027-11")
	require.NoError(t, err)
	require.Empty(t, after)
}

func TestCalendarFixedDepositSettlementStillCompletesReminder(t *testing.T) {
	f := newInvestmentDBFixture(t)
	_, err := f.engine.Insert(&models.Account{AccountId: 2002, Uid: f.uid, Name: "定存", Type: 1, Category: 9, Currency: "CNY"})
	require.NoError(t, err)
	deposit, err := f.s.SaveFixedDeposit(nil, f.uid, models.FixedDeposit{AccountId: "2002", BookId: DefaultBookID(f.uid), Principal: "100.00", AnnualRate: "2", StartDate: "2026-11-10", Term: 1, Unit: "year", TimeZone: "Asia/Shanghai"})
	require.NoError(t, err)
	created, err := f.s.SyncFixedDeposits(nil, f.uid, time.Date(2027, 11, 11, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, 1, created)
	created, err = f.s.SyncFixedDeposits(nil, f.uid, time.Date(2027, 11, 12, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Zero(t, created)
	var event models.CalendarEvent
	has, err := f.engine.ID(deposit.Id).Get(&event)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, event.Completed)
}

func TestLedgerClearPreservesDepositReminderConsistency(t *testing.T) {
	f := newInvestmentDBFixture(t)
	deposit, err := f.s.SaveFixedDeposit(nil, f.uid, models.FixedDeposit{AccountId: fmt.Sprint(f.bankID), BookId: DefaultBookID(f.uid), Principal: "100.00", AnnualRate: "2", StartDate: "2026-11-10", Term: 1, Unit: "year", TimeZone: "Asia/Shanghai"})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.CalendarEvent{Id: "due-manual", Uid: f.uid, AccountId: fmt.Sprint(f.bankID), Kind: "repayment", Date: "2027-11-10", Amount: "5"}, &models.CalendarEvent{Id: "other-user-due", Uid: f.uid + 1, AccountId: "2003", Kind: "deposit", Date: "2027-11-10", Amount: "9"})
	require.NoError(t, err)
	balance := f.balance()
	_, err = f.engine.Exec(`CREATE TRIGGER reject_deposit_due_delete BEFORE DELETE ON calendar_event WHEN OLD.id='` + deposit.Id + `' BEGIN SELECT RAISE(ABORT, 'injected'); END`)
	require.NoError(t, err)
	require.Error(t, Transactions.DeleteAllTransactions(nil, f.uid, false))
	var row models.FixedDeposit
	has, err := f.engine.ID(deposit.Id).Get(&row)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, row.Closed)
	require.Equal(t, balance, f.balance())
	_, err = f.engine.Exec("DROP TRIGGER reject_deposit_due_delete")
	require.NoError(t, err)
	require.NoError(t, Transactions.DeleteAllTransactions(nil, f.uid, false))
	has, err = f.engine.ID(deposit.Id).Get(&row)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, row.Closed)
	has, err = f.engine.ID(deposit.Id).Exist(&models.CalendarEvent{})
	require.NoError(t, err)
	require.False(t, has)
	for _, id := range []string{"due-manual", "other-user-due"} {
		has, err = f.engine.ID(id).Exist(&models.CalendarEvent{})
		require.NoError(t, err)
		require.True(t, has)
	}
	created, err := f.s.SyncFixedDeposits(nil, f.uid, time.Date(2027, 11, 11, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Zero(t, created)
}

func TestAccountClearRejectsUnsettledFixedDeposit(t *testing.T) {
	f := newInvestmentDBFixture(t)
	deposit, err := f.s.SaveFixedDeposit(nil, f.uid, models.FixedDeposit{AccountId: fmt.Sprint(f.bankID), BookId: DefaultBookID(f.uid), Principal: "100.00", AnnualRate: "2", StartDate: "2026-11-10", Term: 1, Unit: "year", TimeZone: "Asia/Shanghai"})
	require.NoError(t, err)
	balance := f.balance()
	require.Error(t, Transactions.DeleteAllTransactionsOfAccount(nil, f.uid, f.bankID, 100))
	require.Equal(t, balance, f.balance())
	has, err := f.engine.ID(deposit.Id).Exist(&models.CalendarEvent{})
	require.NoError(t, err)
	require.True(t, has)
	_, err = f.s.CloseFixedDeposit(nil, f.uid, deposit.Id)
	require.NoError(t, err)
	require.NoError(t, Transactions.DeleteAllTransactionsOfAccount(nil, f.uid, f.bankID, 100))
}
