package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/investments"
	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func walletFixture(f *investmentDBFixture) (string, string) {
	asset, err := f.s.CreateInstrument(nil, f.uid, models.InvestmentInstrument{Type: "CRYPTO", Name: "Fiat24 USD QA", Symbol: "USD24"})
	require.NoError(f.t, err)
	_, err = f.engine.Insert(
		&models.TransactionCategory{CategoryId: 7001, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "消费"},
		&models.TransactionCategory{CategoryId: 7002, Uid: f.uid, ParentCategoryId: 7001, Type: models.CATEGORY_TYPE_EXPENSE, Name: "服务"},
		&models.TransactionCategory{CategoryId: 7003, Uid: f.uid, Type: models.CATEGORY_TYPE_INCOME, Name: "收入"},
		&models.TransactionCategory{CategoryId: 7004, Uid: f.uid, ParentCategoryId: 7003, Type: models.CATEGORY_TYPE_INCOME, Name: "报酬"})
	require.NoError(f.t, err)
	for i, v := range []struct{ id, q, c string }{{asset.Id, "10", "70"}, {"crypto:usd-coin", "20", "140"}} {
		cost := v.c
		f.create(InvestmentEvent{Event: investments.Event{Type: investments.Opening, AccountID: f.portfolioID, InstrumentID: v.id, Quantity: v.q, Amount: "0", Fee: "0", Cost: &cost, OccurredAt: f.at + int64(i)}}, fmt.Sprintf("wallet-opening-%d", i))
	}
	return asset.Id, "crypto:usd-coin"
}

func walletEvent(f *investmentDBFixture, first, second string) InvestmentEvent {
	return InvestmentEvent{Event: investments.Event{Type: investments.Expense, AccountID: f.portfolioID, InstrumentID: first, Quantity: "10", AdditionalMovements: []investments.AssetMovement{{InstrumentID: second, Quantity: "5"}}, Amount: "15", Fee: "0", ExchangeRate: "7.2", OccurredAt: f.at + 10, Note: "美元产品"}, Wallet: &WalletEntry{Currency: "USD", CategoryID: "7002", FXDate: time.Unix(f.at, 0).UTC().Format("2006-01-02"), FXSource: "用户确认"}}
}

func TestWalletExpenseIncomeReviseVoidAtomicAndNoDuplicateWealth(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	initialCash := f.balance()
	input := walletEvent(f, first, second)
	before := f.count(&models.Transaction{})
	preview, err := f.s.Mutate(nil, f.uid, input, "", "create", true)
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.Equal(t, before, f.count(&models.Transaction{}))
	paid := f.create(input, "wallet-expense-1")
	again := f.create(input, "wallet-expense-1")
	require.Equal(t, paid.Event.ID, again.Event.ID)
	var postings []models.Transaction
	require.NoError(t, f.engine.Where("investment_event_id=? AND deleted=?", paid.Event.ID, false).Find(&postings))
	require.Len(t, postings, 1)
	require.Equal(t, int64(10800), postings[0].Amount)
	require.Equal(t, models.TRANSACTION_DB_TYPE_EXPENSE, postings[0].Type)
	require.Equal(t, int64(7002), postings[0].CategoryId)
	require.Equal(t, initialCash, f.balance())
	values := map[string]investments.Position{}
	for _, p := range paid.Positions {
		values[p.InstrumentID] = p
	}
	require.Equal(t, "0", values[first].Quantity)
	require.Equal(t, "15", values[second].Quantity)
	requireMoney(t, "105", values[second].Cost)
	details, err := f.s.WalletTransactionDetails(nil, f.uid, []string{paid.Event.ID})
	require.NoError(t, err)
	require.Equal(t, "15", details[paid.Event.ID].Amount)
	require.Equal(t, "USD", details[paid.Event.ID].Currency)
	require.ErrorIs(t, Transactions.DeleteTransaction(nil, f.uid, postings[0].TransactionId), ErrInvestmentLinked)
	bad := paid.Event
	bad.Quantity = "11"
	_, err = f.s.Mutate(nil, f.uid, bad, "", "revise", false)
	require.Error(t, err)
	var stored models.InvestmentEventRecord
	_, err = f.engine.ID(paid.Event.ID).Get(&stored)
	require.NoError(t, err)
	require.Equal(t, 1, stored.Version)
	changed := paid.Event
	changed.Quantity = "9"
	changed.Amount = "14"
	revised, err := f.s.Mutate(nil, f.uid, changed, "", "revise", false)
	require.NoError(t, err)
	var current []models.Transaction
	require.NoError(t, f.engine.Where("investment_event_id=? AND deleted=?", paid.Event.ID, false).Find(&current))
	require.Len(t, current, 1)
	require.Equal(t, int64(10080), current[0].Amount)
	_, err = f.s.Mutate(nil, f.uid, paid.Event, "", "void", false)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	income := InvestmentEvent{Event: investments.Event{Type: investments.Income, AccountID: f.portfolioID, InstrumentID: second, Quantity: "4", Amount: "4", Fee: "0", ExchangeRate: "7.2", OccurredAt: f.at + 20}, Wallet: &WalletEntry{Currency: "USD", CategoryID: "7004", FXDate: input.Wallet.FXDate, FXSource: "用户确认"}}
	received := f.create(income, "wallet-income-1")
	require.NotNil(t, received)
	_, err = f.s.Mutate(nil, f.uid, revised.Event, "", "void", false)
	require.NoError(t, err)
	events, err := f.s.Events(nil, f.uid)
	require.NoError(t, err)
	replayed, err := replayInvestments(events)
	require.NoError(t, err)
	for _, p := range replayed.Positions {
		if p.InstrumentID == first {
			require.Equal(t, "10", p.Quantity)
		}
		if p.InstrumentID == second {
			require.Equal(t, "24", p.Quantity)
			requireMoney(t, "168.8", p.Cost)
		}
	}
	require.Equal(t, initialCash, f.balance())
	wealth := f.summary(false)
	require.Len(t, wealth.CashAccounts, 1)
	_, err = f.s.Mutate(nil, f.uid, received.Event, "", "void", false)
	require.NoError(t, err)
	var system models.Account
	_, err = f.engine.Where("uid=? AND system_role=?", f.uid, InvestmentSettlementRole).Get(&system)
	require.NoError(t, err)
	require.Zero(t, system.Balance)
}

func TestWalletPaymentValidationRollbackAndAccountCurrencyPreservation(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	base := walletEvent(f, first, second)
	for _, mutation := range []func(*InvestmentEvent){
		func(e *InvestmentEvent) { e.ExchangeRate = "" }, func(e *InvestmentEvent) { e.ExchangeRate = "0" }, func(e *InvestmentEvent) { e.Amount = "1e3" }, func(e *InvestmentEvent) { e.Amount = "0.001" },
		func(e *InvestmentEvent) { e.Wallet.Currency = "EUR" }, func(e *InvestmentEvent) { e.Wallet.Currency = "CNY" }, func(e *InvestmentEvent) { e.Wallet.CategoryID = "7004" }, func(e *InvestmentEvent) { e.Wallet.CategoryID = "7001" },
		func(e *InvestmentEvent) { e.AccountID = "someone-else" }, func(e *InvestmentEvent) { e.AdditionalMovements[0].InstrumentID = e.InstrumentID }, func(e *InvestmentEvent) { e.AdditionalMovements[0].Quantity = "21" }, func(e *InvestmentEvent) { e.CashAccountID = fmt.Sprint(f.bankID) },
	} {
		e := base
		w := *base.Wallet
		e.Wallet = &w
		e.AdditionalMovements = append([]investments.AssetMovement{}, base.AdditionalMovements...)
		mutation(&e)
		beforeTx, beforeEvent := f.count(&models.Transaction{}), f.count(&models.InvestmentEventRecord{})
		_, err := f.s.Mutate(nil, f.uid, e, "wallet-invalid-key", "create", false)
		require.Error(t, err)
		require.Equal(t, beforeTx, f.count(&models.Transaction{}))
		require.Equal(t, beforeEvent, f.count(&models.InvestmentEventRecord{}))
	}
	account, err := f.s.UpdatePortfolioAccount(nil, f.uid, models.PortfolioAccount{Id: f.portfolioID, Name: "美元钱包", Kind: "WALLET", Currency: "USD", Instruments: []string{first, second}, PaymentInstruments: []string{first, second}})
	require.NoError(t, err)
	require.Equal(t, "USD", account.Currency)
	paid := f.create(base, "wallet-original-usd")
	account.Currency = "CNY"
	_, err = f.s.UpdatePortfolioAccount(nil, f.uid, *account)
	require.NoError(t, err)
	events, err := f.s.Events(nil, f.uid)
	require.NoError(t, err)
	for _, e := range events {
		if e.ID == paid.Event.ID {
			require.Equal(t, "USD", e.Wallet.Currency)
			require.Equal(t, "7.2", e.ExchangeRate)
			require.Equal(t, "15", e.Amount)
		}
	}
	account.Currency = "EUR"
	_, err = f.s.UpdatePortfolioAccount(nil, f.uid, *account)
	require.Error(t, err)
}

func TestWalletDiscountAndPicturesLifecycleAndBackupRestore(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	_, err := f.engine.Insert(
		&models.TransactionPictureInfo{Uid: f.uid, PictureId: 8801, TransactionId: models.TransactionPictureNewPictureTransactionId, PictureExtension: "jpg"},
		&models.TransactionPictureInfo{Uid: f.uid, PictureId: 8802, TransactionId: models.TransactionPictureNewPictureTransactionId, PictureExtension: "png"},
	)
	require.NoError(t, err)
	base := walletEvent(f, first, second)
	discount := "2.5"
	pics := []string{"8801"}
	base.Wallet.DiscountAmount = &discount
	base.Wallet.PictureIDs = &pics
	paid := f.create(base, "wallet-discount-pic-create")
	require.NotNil(t, paid.Event.Wallet.DiscountAmount)
	require.Equal(t, "2.50", *paid.Event.Wallet.DiscountAmount)
	require.NotNil(t, paid.Event.Wallet.PictureIDs)
	require.Equal(t, []string{"8801"}, *paid.Event.Wallet.PictureIDs)
	require.Len(t, paid.Event.Wallet.Pictures, 1)
	require.Equal(t, "/pictures/8801.jpg", paid.Event.Wallet.Pictures[0].OriginalUrl)

	var tx models.Transaction
	has, err := f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, paid.Event.ID, false).Get(&tx)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(10800), tx.Amount)
	require.Equal(t, "18.00", tx.DiscountAmount)

	var p1 models.TransactionPictureInfo
	has, err = f.engine.ID(8801).Get(&p1)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, p1.Deleted)
	require.Equal(t, tx.TransactionId, p1.TransactionId)

	// Older client omits DiscountAmount & PictureIDs on revise -> preserves both cleanly.
	legacyEdit := paid.Event
	wCopy := *paid.Event.Wallet
	wCopy.DiscountAmount = nil
	wCopy.PictureIDs = nil
	legacyEdit.Wallet = &wCopy
	revisedLegacy, err := f.s.Mutate(nil, f.uid, legacyEdit, "", "revise", false)
	require.NoError(t, err)
	require.Equal(t, "2.50", *revisedLegacy.Event.Wallet.DiscountAmount)
	require.Equal(t, []string{"8801"}, *revisedLegacy.Event.Wallet.PictureIDs)
	var txAfterLegacy models.Transaction
	has, err = f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, paid.Event.ID, false).Get(&txAfterLegacy)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "18.00", txAfterLegacy.DiscountAmount)
	p1 = models.TransactionPictureInfo{}
	has, err = f.engine.ID(8801).Get(&p1)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, p1.Deleted)
	require.Equal(t, txAfterLegacy.TransactionId, p1.TransactionId)

	// Switch to new discount and replace picture 8801 -> 8802.
	newDiscount := "1.00"
	newPics := []string{"8802"}
	edit := revisedLegacy.Event
	wEdit := *revisedLegacy.Event.Wallet
	wEdit.DiscountAmount = &newDiscount
	wEdit.PictureIDs = &newPics
	edit.Wallet = &wEdit
	revised, err := f.s.Mutate(nil, f.uid, edit, "", "revise", false)
	require.NoError(t, err)
	require.Equal(t, "1.00", *revised.Event.Wallet.DiscountAmount)
	require.Equal(t, []string{"8802"}, *revised.Event.Wallet.PictureIDs)
	require.Len(t, revised.Event.Wallet.Pictures, 1)
	require.Equal(t, "/pictures/8802.png", revised.Event.Wallet.Pictures[0].OriginalUrl)

	var txFinal models.Transaction
	has, err = f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, paid.Event.ID, false).Get(&txFinal)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "7.20", txFinal.DiscountAmount)
	p1 = models.TransactionPictureInfo{}
	has, err = f.engine.ID(8801).Get(&p1)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, p1.Deleted)
	var p2 models.TransactionPictureInfo
	has, err = f.engine.ID(8802).Get(&p2)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, p2.Deleted)
	require.Equal(t, txFinal.TransactionId, p2.TransactionId)

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "wallet-discount-pic.zip")
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
	has, err = restored.ID(txFinal.TransactionId).Get(&restoredTx)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, "7.20", restoredTx.DiscountAmount)
	var restoredPic models.TransactionPictureInfo
	has, err = restored.ID(8802).Get(&restoredPic)
	require.NoError(t, err)
	require.True(t, has)
	require.False(t, restoredPic.Deleted)
	require.Equal(t, txFinal.TransactionId, restoredPic.TransactionId)

	// Void deletes remaining attached pictures cleanly.
	_, err = f.s.Mutate(nil, f.uid, revised.Event, "", "void", false)
	require.NoError(t, err)
	p2 = models.TransactionPictureInfo{}
	has, err = f.engine.ID(8802).Get(&p2)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, p2.Deleted)
}
