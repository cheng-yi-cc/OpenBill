package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/investments"
	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestBookMoveAllLargeAtomicAndTransferFee(t *testing.T) {
	f, transfer := feeFixture(t)
	source, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "源"})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "目标"})
	require.NoError(t, err)
	transfer.BookId = source.Id
	require.NoError(t, Transactions.CreateTransaction(nil, transfer, nil, nil))
	var last int64
	for i := 0; i < 1002; i++ {
		last = addBookExpense(f, source.Id, 1, int64(i+2)).TransactionId
	}
	before := f.balance()
	countBefore, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Greater(t, countBefore, int64(1000))
	// Force a failure after earlier SQL chunks have executed. The transaction
	// must roll all prior chunks back, including the paired transfer and fee.
	_, err = f.engine.Exec(fmt.Sprintf(`CREATE TRIGGER reject_book_move_late BEFORE UPDATE ON "transaction" WHEN OLD.transaction_id=%d AND NEW.book_id='%s' BEGIN SELECT RAISE(ABORT,'late failure'); END`, last, target.Id))
	require.NoError(t, err)
	count, err := Books.MoveAllTransactions(nil, f.uid, source.Id, target.Id, refundTestActor(f.uid))
	require.Error(t, err)
	require.Zero(t, count)
	remaining, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Equal(t, countBefore, remaining)
	moved, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, target.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Zero(t, moved)
	require.Equal(t, before, f.balance())
	_, err = f.engine.Exec("DROP TRIGGER reject_book_move_late")
	require.NoError(t, err)
	count, err = Books.MoveAllTransactions(nil, f.uid, source.Id, target.Id, refundTestActor(f.uid))
	require.NoError(t, err)
	require.Equal(t, int(countBefore), count)
	remaining, err = f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Zero(t, remaining)
	moved, err = f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, target.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Equal(t, countBefore, moved)
	require.Equal(t, before, f.balance())
	count, err = Books.MoveAllTransactions(nil, f.uid, source.Id, target.Id, refundTestActor(f.uid))
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestBookMoveAllPreservesInstallmentAndFutureAccrual(t *testing.T) {
	f := newInvestmentDBFixture(t)
	now := time.Now().UTC()
	source, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "分期来源"})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "分期目标"})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.Account{AccountId: 8508, Uid: f.uid, Name: "迁移信用卡", Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Category: models.ACCOUNT_CATEGORY_CREDIT_CARD, Currency: "CNY"})
	require.NoError(t, err)
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	var category models.TransactionCategory
	has, err := f.engine.Where("uid=? AND type=? AND parent_category_id>0", f.uid, models.CATEGORY_TYPE_EXPENSE).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	expense := &models.Transaction{Uid: f.uid, BookId: source.Id, AccountId: 8508, CategoryId: category.CategoryId, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 1200, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at)}
	require.NoError(t, Transactions.CreateTransaction(nil, expense, nil, nil))
	input := CreditInstallmentInput{AccountId: "8508", ExpenseId: fmt.Sprint(expense.TransactionId), Principal: "12", TotalFee: "0.30", Periods: 3, FirstDate: now.Format("2006-01-02"), Method: "monthly", Remainder: "first", BookId: source.Id, TimeZone: "UTC", RequestId: "book-move-plan-fixture"}
	plan, err := Investments.SaveCreditInstallment(nil, f.uid, input)
	require.NoError(t, err)
	n, err := Investments.SyncCreditInstallments(nil, f.uid, now)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	plans, err := Investments.CreditInstallments(nil, f.uid)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	firstFee := plans[0].Data.Payments[0].FeeTransactionId
	var before models.Account
	has, err = f.engine.ID(8508).Get(&before)
	require.NoError(t, err)
	require.True(t, has)
	count, err := Books.MoveAllTransactions(nil, f.uid, source.Id, target.Id, refundTestActor(f.uid))
	require.NoError(t, err)
	require.Equal(t, 2, count)
	var after models.Account
	has, err = f.engine.ID(8508).Get(&after)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, before.Balance, after.Balance)
	for _, id := range []string{fmt.Sprint(expense.TransactionId), firstFee} {
		var tx models.Transaction
		has, err = f.engine.ID(id).Get(&tx)
		require.NoError(t, err)
		require.True(t, has)
		require.Equal(t, target.Id, tx.BookId)
	}
	plans, err = Investments.CreditInstallments(nil, f.uid)
	require.NoError(t, err)
	require.Equal(t, plan.Id, plans[0].Id)
	require.Equal(t, expense.TransactionId, plans[0].ExpenseId)
	require.Equal(t, firstFee, plans[0].Data.Payments[0].FeeTransactionId)
	// Current implementation retains the explicitly chosen plan book. This
	// records actual behavior; reference behavior for future fees is unknown.
	require.Equal(t, source.Id, plans[0].Data.BookId)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "move-installment.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(path string) error { _, e := f.engine.Exec("VACUUM INTO ?", path); return e }))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	var restoredPlan models.CreditInstallment
	has, err = restored.ID(plan.Id).Where("uid=?", f.uid).Get(&restoredPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, expense.TransactionId, restoredPlan.ExpenseId)
	require.Equal(t, firstFee, restoredPlan.Data.Payments[0].FeeTransactionId)
	var restoredExpense models.Transaction
	has, err = restored.ID(expense.TransactionId).Get(&restoredExpense)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, target.Id, restoredExpense.BookId)
	n, err = Investments.SyncCreditInstallments(nil, f.uid, now.AddDate(0, 1, 1))
	require.NoError(t, err)
	require.Equal(t, 1, n)
	n, err = Investments.SyncCreditInstallments(nil, f.uid, now.AddDate(0, 1, 1))
	require.NoError(t, err)
	require.Zero(t, n)
	plans, err = Investments.CreditInstallments(nil, f.uid)
	require.NoError(t, err)
	var secondFee models.Transaction
	has, err = f.engine.ID(plans[0].Data.Payments[1].FeeTransactionId).Get(&secondFee)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, source.Id, secondFee.BookId)
}

func TestBookMoveAllMigratesInvestmentsAtomically(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	source, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "投资来源"})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "投资目标"})
	require.NoError(t, err)

	// 1. A book containing only an unlinked OPENING event (zero cash transactions).
	openingBook, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "仅期初账本"})
	require.NoError(t, err)
	opening := InvestmentEvent{Event: investments.Event{Type: investments.Opening, AccountID: f.portfolioID, InstrumentID: first, Quantity: "2", OccurredAt: f.at + 1}, BookID: openingBook.Id}
	createdOpening := f.create(opening, "move-all-unlinked-opening")
	movedOpening, err := Books.MoveAllTransactions(nil, f.uid, openingBook.Id, target.Id, refundTestActor(f.uid))
	require.NoError(t, err)
	require.Equal(t, 1, movedOpening)
	movedAgain, err := Books.MoveAllTransactions(nil, f.uid, openingBook.Id, target.Id, refundTestActor(f.uid))
	require.NoError(t, err)
	require.Zero(t, movedAgain)
	var rec models.InvestmentEventRecord
	has, err := f.engine.Where("uid=? AND id=?", f.uid, createdOpening.Event.ID).Get(&rec)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, 2, rec.Version)
	var decoded InvestmentEvent
	require.NoError(t, json.Unmarshal([]byte(rec.Payload), &decoded))
	require.Equal(t, target.Id, decoded.BookID)
	revs, err := f.engine.Where("uid=? AND event_id=?", f.uid, createdOpening.Event.ID).Count(&models.InvestmentEventRevision{})
	require.NoError(t, err)
	require.Equal(t, int64(2), revs)

	// 2. Mixed source book with ordinary expense, settled BUY transfer, wallet entry,
	// plan/order/DCA/monetary binding, and an active wealth snapshot.
	tx := &models.Transaction{Uid: f.uid, BookId: source.Id, AccountId: f.bankID, CategoryId: 7002, Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: 100, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + 2)}
	require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))
	buy := InvestmentEvent{Event: investments.Event{Type: investments.Buy, AccountID: f.portfolioID, InstrumentID: first, Quantity: "1", Amount: "10", Fee: "0", ExchangeRate: "1", OccurredAt: f.at + 5, Note: "买入测试"}, CashAccountID: fmt.Sprint(f.bankID), BookID: source.Id}
	createdBuy := f.create(buy, "move-all-buy")
	wInput := walletEvent(f, first, second)
	wInput.BookID = source.Id
	createdWallet := f.create(wInput, "move-all-wallet")
	_, err = f.engine.Insert(
		&models.InvestmentPlan{Id: "plan-move-all", Uid: f.uid, AccountId: f.portfolioID, InstrumentId: first, CashAccountId: fmt.Sprint(f.bankID), BookId: source.Id, Amount: "10", FeePercent: "0", Cycle: "MONTHLY", StartDate: "2026-11-01", NextDate: "2026-12-01", Time: "10:00", TimeZone: "UTC", Version: 1},
		&models.InvestmentOrder{Id: "order-move-all", Uid: f.uid, RequestKey: "order-move-req", PlanId: "plan-move-all", AccountId: f.portfolioID, InstrumentId: first, CashAccountId: fmt.Sprint(f.bankID), BookId: source.Id, Type: investments.Buy, Amount: "10", TradeDate: "2026-11-10", ConfirmDate: "2026-11-11", Time: "10:00", TimeZone: "UTC", Status: "pending", Version: 1},
		&models.CryptoDCAPlan{Id: "dca-move-all", Uid: f.uid, AccountId: f.portfolioID, InstrumentId: "crypto:bitcoin", PaymentInstrumentId: "crypto:tether", Amount: "5", DailyTime: "09:00", StartDate: "2026-11-01", NextDate: "2026-11-15", TimeZone: "UTC", BookId: source.Id, Enabled: true, Revision: 1},
		&models.CryptoDCADay{Id: "dca-move-all:2026-11-14", Uid: f.uid, PlanId: "dca-move-all", Date: "2026-11-14", ScheduledAt: f.at + 30, PlanRevision: 1, AccountId: f.portfolioID, InstrumentId: "crypto:bitcoin", PaymentInstrumentId: "crypto:tether", Amount: "5", BookId: source.Id, Status: "pending"},
		&models.MonetaryIncomeBinding{Id: "mon-move-all", Uid: f.uid, AccountId: f.bankID, Code: "000198", Name: "货币基金", StartDate: "2026-11-01", NextDate: "2026-11-15", BookId: source.Id, TimeZone: "UTC", CategoryId: 7004, Enabled: true},
		&models.WealthSnapshot{Id: "snap-move-all", Uid: f.uid, RecordedAt: f.at + 100, ValuedAssets: "100", Complete: true, Invalidated: false, Payload: "{}"},
	)
	require.NoError(t, err)

	sourceTxCountBefore, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, sourceTxCountBefore, int64(4)) // ordinary + 2 transfer sides + wallet
	before := f.balance()

	// 3. Inject late SQLite failure during investment_event_record update and verify full rollback.
	_, err = f.engine.Exec(fmt.Sprintf(`CREATE TRIGGER reject_inv_book_move BEFORE UPDATE ON "investment_event_record" WHEN OLD.id='%s' BEGIN SELECT RAISE(ABORT,'late investment failure'); END`, createdBuy.Event.ID))
	require.NoError(t, err)
	count, err := Books.MoveAllTransactions(nil, f.uid, source.Id, target.Id, refundTestActor(f.uid))
	require.Error(t, err)
	require.Zero(t, count)
	sourceTxAfterFail, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Equal(t, sourceTxCountBefore, sourceTxAfterFail)
	targetTxAfterFail, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, target.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Zero(t, targetTxAfterFail)
	require.Equal(t, before, f.balance())
	_, err = f.engine.Exec("DROP TRIGGER reject_inv_book_move")
	require.NoError(t, err)

	// 4. Execute full atomic migration.
	count, err = Books.MoveAllTransactions(nil, f.uid, source.Id, target.Id, refundTestActor(f.uid))
	require.NoError(t, err)
	require.Equal(t, int(sourceTxCountBefore), count)
	require.Equal(t, before, f.balance())
	sourceTxAfter, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source.Id, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Zero(t, sourceTxAfter)

	for _, eventID := range []string{createdBuy.Event.ID, createdWallet.Event.ID} {
		var r models.InvestmentEventRecord
		has, err = f.engine.Where("uid=? AND id=?", f.uid, eventID).Get(&r)
		require.NoError(t, err)
		require.True(t, has)
		var ev InvestmentEvent
		require.NoError(t, json.Unmarshal([]byte(r.Payload), &ev))
		require.Equal(t, target.Id, ev.BookID)
	}
	var movedPlan models.InvestmentPlan
	has, err = f.engine.ID("plan-move-all").Get(&movedPlan)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, target.Id, movedPlan.BookId)
	require.Equal(t, 2, movedPlan.Version)
	var movedOrder models.InvestmentOrder
	has, err = f.engine.ID("order-move-all").Get(&movedOrder)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, target.Id, movedOrder.BookId)
	var movedDCA models.CryptoDCAPlan
	has, err = f.engine.ID("dca-move-all").Get(&movedDCA)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, target.Id, movedDCA.BookId)
	var movedDCADay models.CryptoDCADay
	has, err = f.engine.ID("dca-move-all:2026-11-14").Get(&movedDCADay)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, target.Id, movedDCADay.BookId)
	var movedBinding models.MonetaryIncomeBinding
	has, err = f.engine.ID("mon-move-all").Get(&movedBinding)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, target.Id, movedBinding.BookId)
	var snap models.WealthSnapshot
	has, err = f.engine.ID("snap-move-all").Get(&snap)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, snap.Invalidated)

	// Selective MoveTransactions of an investment settlement transaction must still fail.
	var walletTx models.Transaction
	has, err = f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, createdWallet.Event.ID, false).Get(&walletTx)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, target.Id, walletTx.BookId)
	require.ErrorIs(t, Books.MoveTransactions(nil, f.uid, []int64{walletTx.TransactionId}, source.Id, refundTestActor(f.uid)), ErrInvestmentLinked)

	// 5. Verify backup archive/restore retains migrated bookId across SQLite tables.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "move-all-investment.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(path string) error { _, e := f.engine.Exec("VACUUM INTO ?", path); return e }))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	var restoredRec models.InvestmentEventRecord
	has, err = restored.Where("uid=? AND id=?", f.uid, createdWallet.Event.ID).Get(&restoredRec)
	require.NoError(t, err)
	require.True(t, has)
	var restoredEv InvestmentEvent
	require.NoError(t, json.Unmarshal([]byte(restoredRec.Payload), &restoredEv))
	require.Equal(t, target.Id, restoredEv.BookID)
	var restoredBinding models.MonetaryIncomeBinding
	has, err = restored.ID("mon-move-all").Get(&restoredBinding)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, target.Id, restoredBinding.BookId)
}

func TestBookMoveAllRefundReimbursementAndInvestmentGuard(t *testing.T) {
	f, expense := reimbursementFixture(t)
	actor := refundTestActor(f.uid)
	_, err := Investments.ReceiveReimbursement(nil, f.uid, ReimbursementReceiveRequest{
		ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "40",
		Time: f.at + 10, TimeZone: "UTC", RequestId: "all-move-receipt",
	})
	require.NoError(t, err)
	_, err = Investments.CreateRefund(nil, f.uid, actor, RefundInput{
		ExpenseId: fmt.Sprint(expense.TransactionId), AccountId: fmt.Sprint(f.bankID), Amount: "30",
		Time: f.at + 20, TimeZone: "UTC", RequestId: "all-move-refund",
	})
	require.NoError(t, err)
	target, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "目标"})
	require.NoError(t, err)
	source := expense.BookId
	before, pending := f.balance(), reimbursementBalance(t, f)
	countBefore, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source, false).Count(&models.Transaction{})
	require.NoError(t, err)
	denied := refundTestActor(f.uid)
	denied.User.TransactionEditScope = models.TRANSACTION_EDIT_SCOPE_NONE
	_, err = Books.MoveAllTransactions(nil, f.uid, source, target.Id, denied)
	require.Error(t, err)
	remaining, err := f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Equal(t, countBefore, remaining)

	// A linked investment settlement must reject the entire operation until
	// event revisions can be migrated safely alongside accounting facts.
	linked := addBookExpense(f, source, 1, 30)
	_, err = f.engine.ID(linked.TransactionId).Cols("investment_event_id").Update(&models.Transaction{InvestmentEventId: "test-linked-event"})
	require.NoError(t, err)
	_, err = Books.MoveAllTransactions(nil, f.uid, source, target.Id, actor)
	require.ErrorIs(t, err, ErrInvestmentLinked)
	remaining, err = f.engine.Where("uid=? AND book_id=? AND deleted=?", f.uid, source, false).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Equal(t, countBefore+1, remaining)
	_, err = f.engine.ID(linked.TransactionId).Cols("investment_event_id").Update(&models.Transaction{InvestmentEventId: ""})
	require.NoError(t, err)
	count, err := Books.MoveAllTransactions(nil, f.uid, source, target.Id, actor)
	require.NoError(t, err)
	require.Equal(t, int(countBefore+1), count)
	// The synthetic linked expense above debits one cent and no move changes it.
	require.Equal(t, before-1, f.balance())
	require.Equal(t, pending, reimbursementBalance(t, f))
	summary, err := Investments.GetRefunds(nil, f.uid, expense.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "70.00", summary.NetExpense)
	claims, err := Investments.ReimbursementClaims(nil, f.uid)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, target.Id, claims[0].BookId)
	require.Equal(t, "30.00", claims[0].Pending)
}
