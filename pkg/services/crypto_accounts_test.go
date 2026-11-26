package services

import (
	"github.com/mayswind/ezbookkeeping/pkg/marketquotes"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func conversionFixture(now int64) []InvestmentValuationQuote {
	return []InvestmentValuationQuote{
		{Quote: marketquotes.Quote{InstrumentID: "crypto:tether", Price: "0.999", Currency: "USD", SourceTime: now, ReceivedAt: now, State: marketquotes.StateDelayed, Source: marketquotes.SourceCoinbaseREST}, FXRate: "7", FXDate: "2026-10-02", FXSource: "fixture FX", FXState: "fresh"},
		{Quote: marketquotes.Quote{InstrumentID: "crypto:bitcoin", Price: "60000", Currency: "USD", SourceTime: now, ReceivedAt: now, State: marketquotes.StateLive, Source: marketquotes.SourceCoinbaseWS}, FXRate: "7", FXDate: "2026-10-02", FXSource: "fixture FX", FXState: "fresh"},
	}
}
func TestCryptoConversionUsesObservedStablecoinPriceAndDecimalAmounts(t *testing.T) {
	now := time.Now().Unix()
	quotes := conversionFixture(now)
	q, err := conversionFromQuotes(ConversionInput{ToInstrumentID: "crypto:tether", FromQuantity: "6993"}, quotes, now)
	require.NoError(t, err)
	require.Equal(t, "1000", q.ToQuantity)
	require.Equal(t, "6.993", q.ToPrice)
	btc, err := conversionFromQuotes(ConversionInput{FromInstrumentID: "crypto:tether", ToInstrumentID: "crypto:bitcoin", FromQuantity: "600"}, quotes, now)
	require.NoError(t, err)
	require.Equal(t, "0.00999", btc.ToQuantity)
	require.Equal(t, "6.993", btc.FromPrice)
	sale, err := conversionFromQuotes(ConversionInput{FromInstrumentID: "crypto:bitcoin", FromQuantity: "0.00999"}, quotes, now)
	require.NoError(t, err)
	require.Equal(t, "4195.8", sale.ToQuantity)
	for _, input := range []ConversionInput{{FromQuantity: "1e5", ToInstrumentID: "crypto:tether"}, {FromQuantity: "0", ToInstrumentID: "crypto:tether"}, {FromQuantity: "-1", ToInstrumentID: "crypto:tether"}, {FromQuantity: "1.001", ToInstrumentID: "crypto:tether"}, {FromQuantity: "1", FromInstrumentID: "crypto:tether", ToInstrumentID: "crypto:tether"}} {
		_, err = conversionFromQuotes(input, quotes, now)
		require.Error(t, err)
	}
	quotes[0].State = marketquotes.StateStale
	_, err = conversionFromQuotes(ConversionInput{ToInstrumentID: "crypto:tether", FromQuantity: "1"}, quotes, now)
	require.Error(t, err)
	quotes = conversionFixture(now)
	quotes[0].FXRate = ""
	_, err = conversionFromQuotes(ConversionInput{ToInstrumentID: "crypto:tether", FromQuantity: "1"}, quotes, now)
	require.Error(t, err)
}
func TestCryptoAccountSetupIsAtomicAndIdempotent(t *testing.T) {
	f := newInvestmentDBFixture(t)
	input := CryptoAccountInput{Name: "币安演示", Kind: "EXCHANGE", Platform: "binance", Holdings: []CryptoHoldingInput{{InstrumentID: "crypto:tether", Quantity: "100"}, {InstrumentID: "crypto:bitcoin", Quantity: "0.001"}, {InstrumentID: "crypto:ethereum", Quantity: "0"}}}
	before := f.balance()
	a, err := f.s.CreateCryptoAccount(nil, f.uid, input, "crypto-setup-request")
	require.NoError(t, err)
	require.Len(t, a.Instruments, 3)
	again, err := f.s.CreateCryptoAccount(nil, f.uid, input, "crypto-setup-request")
	require.NoError(t, err)
	require.Equal(t, a.Id, again.Id)
	require.Equal(t, int64(2), f.count(&models.InvestmentEventRecord{}))
	require.Equal(t, before, f.balance())
	accounts, err := f.s.Accounts(nil, f.uid)
	require.NoError(t, err)
	for _, v := range accounts {
		if v.Id == a.Id {
			require.Equal(t, "binance", v.Platform)
			require.Equal(t, a.Instruments, v.Instruments)
		}
	}
	events, err := f.s.Events(nil, f.uid)
	require.NoError(t, err)
	for _, e := range events {
		require.Nil(t, e.Cost)
	}
	input.Name = "different"
	_, err = f.s.CreateCryptoAccount(nil, f.uid, input, "crypto-setup-request")
	require.ErrorIs(t, err, ErrInvestmentConflict)
	input.Holdings[1].Quantity = "-1"
	_, err = f.s.CreateCryptoAccount(nil, f.uid, input, "crypto-invalid-quantity")
	require.Error(t, err)
	require.Equal(t, int64(2), f.count(&models.PortfolioAccount{}))
	require.Equal(t, int64(2), f.count(&models.InvestmentEventRecord{}))
	input.Holdings[1].Quantity = "1"
	input.Holdings[1].InstrumentID = "custom:other-user"
	_, err = f.s.CreateCryptoAccount(nil, f.uid, input, "crypto-invalid-instrument")
	require.Error(t, err)
	input.Platform = "metamask"
	_, err = f.s.CreateCryptoAccount(nil, f.uid, input, "crypto-invalid-platform")
	require.Error(t, err)
}

func TestCryptoWalletOptionalBrandPreservesAtomicSetup(t *testing.T) {
	for _, platform := range []string{"", "bitget-wallet"} {
		t.Run("platform="+platform, func(t *testing.T) {
			f := newInvestmentDBFixture(t)
			before := f.balance()
			input := CryptoAccountInput{Name: "自定义钱包", Kind: "WALLET", Platform: platform, Holdings: []CryptoHoldingInput{{InstrumentID: "crypto:tether", Quantity: "12.34"}}}
			a, err := f.s.CreateCryptoAccount(nil, f.uid, input, "optional-wallet-setup")
			require.NoError(t, err)
			require.Equal(t, platform, a.Platform)
			again, err := f.s.CreateCryptoAccount(nil, f.uid, input, "optional-wallet-setup")
			require.NoError(t, err)
			require.Equal(t, a.Id, again.Id)
			events, err := f.s.Events(nil, f.uid)
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.Equal(t, "12.34", events[0].Quantity)
			require.Nil(t, events[0].Cost)
			require.Equal(t, before, f.balance())
			input.Holdings[0].Quantity = "-1"
			_, err = f.s.CreateCryptoAccount(nil, f.uid, input, "optional-wallet-invalid")
			require.Error(t, err)
			require.Equal(t, int64(2), f.count(&models.PortfolioAccount{}))
			require.Equal(t, int64(1), f.count(&models.InvestmentEventRecord{}))
		})
	}
	f := newInvestmentDBFixture(t)
	for _, input := range []CryptoAccountInput{
		{Name: "钱包", Kind: "WALLET", Platform: "bitget"},
		{Name: "钱包", Kind: "WALLET", Platform: "unknown-wallet"},
		{Name: "交易所", Kind: "EXCHANGE", Platform: "bitget-wallet"},
		{Name: "交易所", Kind: "EXCHANGE"},
		{Name: "其他", Kind: "OTHER"},
		{Kind: "WALLET"},
	} {
		_, err := f.s.CreateCryptoAccount(nil, f.uid, input, "invalid-wallet-brand")
		require.Error(t, err)
	}
	require.Equal(t, int64(1), f.count(&models.PortfolioAccount{}))
	require.Zero(t, f.count(&models.InvestmentEventRecord{}))
}
func TestCryptoConversionSettlementAndExpiredQuote(t *testing.T) {
	f := newInvestmentDBFixture(t)
	now := time.Now().Unix()
	quotes := conversionFixture(now)
	quote, err := conversionFromQuotes(ConversionInput{ToInstrumentID: "crypto:tether", FromQuantity: "6993"}, quotes, now)
	require.NoError(t, err)
	event := f.event("BUY", quote.ToQuantity, "6993", "0", 0)
	event.InstrumentID = "crypto:tether"
	event.ExchangeRate = "1"
	event.OccurredAt = now
	event.Conversion = quote
	saved := f.create(event, "crypto-rmb-usdt")
	require.Equal(t, int64(1300700), f.balance())
	retry, err := f.s.Mutate(nil, f.uid, event, "crypto-rmb-usdt", "create", false)
	require.NoError(t, err)
	require.Equal(t, saved.Event.ID, retry.Event.ID)
	require.Equal(t, int64(1300700), f.balance())
	swap, err := conversionFromQuotes(ConversionInput{FromInstrumentID: "crypto:tether", ToInstrumentID: "crypto:bitcoin", FromQuantity: "600"}, quotes, now)
	require.NoError(t, err)
	buy := f.event("BUY", swap.ToQuantity, "600", "0", 0)
	buy.OccurredAt = now
	buy.CashAccountID = ""
	buy.SettlementAccountID = f.portfolioID
	buy.SettlementInstrumentID = "crypto:tether"
	buy.ExchangeRate = swap.FromPrice
	buy.Conversion = swap
	result := f.create(buy, "crypto-usdt-btc")
	for _, p := range result.Positions {
		if p.InstrumentID == "crypto:tether" {
			require.Equal(t, "400", p.Quantity)
		}
		if p.InstrumentID == "crypto:bitcoin" {
			require.Equal(t, "0.00999", p.Quantity)
		}
	}
	require.Equal(t, int64(1300700), f.balance())
	buy.Amount = "900"
	_, err = f.s.Mutate(nil, f.uid, buy, "crypto-insufficient", "create", false)
	require.Error(t, err)
	require.Equal(t, int64(2), f.count(&models.InvestmentEventRecord{}))
	event.Conversion.ExpiresAt = now - 1
	_, err = f.s.Mutate(nil, f.uid, event, "crypto-expired", "create", false)
	require.Error(t, err)
	require.Equal(t, int64(1300700), f.balance())
}

func TestCryptoAccountOpeningCostsPreserveCurrencyAndUnknown(t *testing.T) {
	for _, kind := range []string{"EXCHANGE", "WALLET"} {
		t.Run(kind, func(t *testing.T) {
			f := newInvestmentDBFixture(t)
			cost, zero := "12.3456789123456789", "0"
			platform := "binance"
			if kind == "WALLET" {
				platform = "metamask"
			}
			input := CryptoAccountInput{Name: "initial costs", Kind: kind, Platform: platform, Currency: "USD", Holdings: []CryptoHoldingInput{
				{InstrumentID: "crypto:bitcoin", Quantity: "3", Cost: &cost, CostExchangeRate: "7.2"},
				{InstrumentID: "crypto:ethereum", Quantity: "2", Cost: &zero, CostExchangeRate: "7.2"},
				{InstrumentID: "crypto:tether", Quantity: "100"},
			}}
			before := f.balance()
			a, err := f.s.CreateCryptoAccount(nil, f.uid, input, "opening-cost-currency")
			require.NoError(t, err)
			again, err := f.s.CreateCryptoAccount(nil, f.uid, input, "opening-cost-currency")
			require.NoError(t, err)
			require.Equal(t, a.Id, again.Id)
			events, err := f.s.Events(nil, f.uid)
			require.NoError(t, err)
			require.Len(t, events, 3)
			for _, e := range events {
				switch e.InstrumentID {
				case "crypto:bitcoin":
					requireMoney(t, "88.88888816888888808", e.Cost)
					require.Contains(t, e.Note, cost+" USD")
					require.Contains(t, e.Note, "7.2 CNY/USD")
				case "crypto:ethereum":
					requireMoney(t, "0", e.Cost)
				case "crypto:tether":
					require.Nil(t, e.Cost)
				}
			}
			replay, err := replayInvestments(events)
			require.NoError(t, err)
			for _, pos := range replay.Positions {
				if pos.InstrumentID == "crypto:bitcoin" {
					requireMoney(t, "88.88888816888888808", pos.Cost)
				}
			}
			// Changing display currency cannot rewrite historical CNY costs.
			a.Currency = "CNY"
			_, err = f.s.UpdatePortfolioAccount(nil, f.uid, *a)
			require.NoError(t, err)
			unchanged, err := f.s.Events(nil, f.uid)
			require.NoError(t, err)
			require.Equal(t, events, unchanged)
			require.Equal(t, before, f.balance())
			changed := "99"
			input.Holdings[0].Cost = &changed
			_, err = f.s.CreateCryptoAccount(nil, f.uid, input, "opening-cost-currency")
			require.ErrorIs(t, err, ErrInvestmentConflict)
		})
	}
}

func TestCryptoAccountInvalidCostRollsBackAllHoldings(t *testing.T) {
	f := newInvestmentDBFixture(t)
	valid := "10"
	for _, tc := range []struct{ quantity, cost, rate, currency string }{
		{"1", "-1", "1", "CNY"}, {"1", "1e9", "1", "CNY"}, {"1", "10", "", "USD"}, {"1", "10", "0", "USD"},
		{"1", "10", "-1", "USD"}, {"0", "10", "1", "CNY"}, {"", "0", "1", "CNY"}, {"1", "10", "7", "CNY"},
		{"1", "0.000000000000000001", "0.000000000000000001", "USD"},
	} {
		input := CryptoAccountInput{Name: "invalid cost", Kind: "EXCHANGE", Platform: "binance", Currency: tc.currency, Holdings: []CryptoHoldingInput{
			{InstrumentID: "crypto:tether", Quantity: "100", Cost: &valid, CostExchangeRate: "1"},
			{InstrumentID: "crypto:bitcoin", Quantity: tc.quantity, Cost: &tc.cost, CostExchangeRate: tc.rate},
		}}
		_, err := f.s.CreateCryptoAccount(nil, f.uid, input, "invalid-opening-cost")
		require.Error(t, err)
		require.Equal(t, int64(1), f.count(&models.PortfolioAccount{}))
		require.Zero(t, f.count(&models.InvestmentEventRecord{}))
		require.Zero(t, f.count(&models.InvestmentIdempotency{}))
	}
}

func TestCryptoAccountSetupPlansAreAtomicAndStartAfterOpening(t *testing.T) {
	f := newInvestmentDBFixture(t)
	today := time.Now().UTC().Format("2006-01-02")
	cost := "700"
	input := CryptoAccountInput{Name: "with plans", Kind: "EXCHANGE", Platform: "binance", Holdings: []CryptoHoldingInput{
		{InstrumentID: "crypto:tether", Quantity: "100", Cost: &cost}, {InstrumentID: "crypto:bitcoin", Quantity: "0"}, {InstrumentID: "crypto:ethereum", Quantity: "0"},
	}, DCAPlans: []CryptoDCASetupInput{
		{InstrumentID: "crypto:bitcoin", PaymentInstrumentID: "crypto:tether", Amount: "10", DailyTime: "00:00", StartDate: today, TimeZone: "UTC"},
		{InstrumentID: "crypto:ethereum", PaymentInstrumentID: "crypto:tether", Amount: "5", DailyTime: "00:00", StartDate: today, TimeZone: "UTC"},
	}}
	before := f.balance()
	// An invalid second plan must roll back the account, opening and first plan.
	input.DCAPlans[1].Amount = "-1"
	_, err := f.s.CreateCryptoAccount(nil, f.uid, input, "initial-plan-atomic")
	require.Error(t, err)
	require.Equal(t, int64(1), f.count(&models.PortfolioAccount{}))
	require.Zero(t, f.count(&models.CryptoDCAPlan{}))
	require.Zero(t, f.count(&models.InvestmentEventRecord{}))
	require.Zero(t, f.count(&models.InvestmentIdempotency{}))
	input.DCAPlans[1].Amount = "5"
	a, err := f.s.CreateCryptoAccount(nil, f.uid, input, "initial-plan-atomic")
	require.NoError(t, err)
	again, err := f.s.CreateCryptoAccount(nil, f.uid, input, "initial-plan-atomic")
	require.NoError(t, err)
	require.Equal(t, a.Id, again.Id)
	var plans []models.CryptoDCAPlan
	require.NoError(t, f.engine.Where("uid=?", f.uid).Find(&plans))
	require.Len(t, plans, 2)
	for _, p := range plans {
		require.Equal(t, a.Id, p.AccountId)
		require.True(t, p.Enabled)
		require.Equal(t, cryptoDCANextDate(today), p.NextDate)
		require.NotEmpty(t, p.BookId)
	}
	require.Equal(t, int64(1), f.count(&models.InvestmentEventRecord{}))
	require.Zero(t, f.count(&models.CryptoDCADay{}))
	require.Equal(t, before, f.balance())
	// Old dates and wallets cannot silently enable initial plans.
	input.DCAPlans[0].StartDate = "2020-01-01"
	_, err = f.s.CreateCryptoAccount(nil, f.uid, input, "initial-plan-old-date")
	require.Error(t, err)
	input.Kind = "WALLET"
	input.Platform = "metamask"
	_, err = f.s.CreateCryptoAccount(nil, f.uid, input, "initial-plan-wallet")
	require.Error(t, err)
	require.Equal(t, int64(2), f.count(&models.CryptoDCAPlan{}))
	require.Equal(t, int64(2), f.count(&models.PortfolioAccount{}))
}
