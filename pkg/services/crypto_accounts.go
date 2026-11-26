package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/investments"
	"github.com/mayswind/ezbookkeeping/pkg/marketquotes"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/shopspring/decimal"
	"xorm.io/xorm"
)

var cryptoPlatforms = map[string]string{
	"gate": "EXCHANGE", "kucoin": "EXCHANGE", "htx": "EXCHANGE", "mexc": "EXCHANGE", "crypto-com": "EXCHANGE", "gemini": "EXCHANGE", "other-exchange": "EXCHANGE",
	"okx-wallet": "WALLET", "imtoken": "WALLET", "tokenpocket": "WALLET", "onekey": "WALLET", "exodus": "WALLET", "safe": "WALLET",
	"binance": "EXCHANGE", "okx": "EXCHANGE", "coinbase": "EXCHANGE", "kraken": "EXCHANGE", "bybit": "EXCHANGE", "bitget": "EXCHANGE",
	"bitget-wallet": "WALLET", "metamask": "WALLET", "trust": "WALLET", "phantom": "WALLET", "rabby": "WALLET", "ledger": "WALLET", "trezor": "WALLET",
}

func validCryptoPlatform(kind, platform string) bool {
	if kind != "EXCHANGE" && kind != "WALLET" {
		return false
	}
	return kind == "WALLET" && platform == "" || cryptoPlatforms[platform] == kind
}

type CryptoHoldingInput struct {
	InstrumentID     string  `json:"instrumentId"`
	Quantity         string  `json:"quantity"`
	Cost             *string `json:"cost,omitempty"` // Total cost in the account currency; nil remains unknown.
	CostExchangeRate string  `json:"costExchangeRate,omitempty"`
}
type CryptoAccountInput struct {
	Currency           string                `json:"currency,omitempty"`
	PaymentInstruments []string              `json:"paymentInstruments,omitempty"`
	Name               string                `json:"name"`
	Kind               string                `json:"kind"`
	Platform           string                `json:"platform"`
	BookID             string                `json:"bookId"`
	Holdings           []CryptoHoldingInput  `json:"holdings"`
	DCAPlans           []CryptoDCASetupInput `json:"dcaPlans,omitempty"`
}

// Setup plans use the new account and its opening book, sharing its atomic request.
type CryptoDCASetupInput struct {
	InstrumentID        string `json:"instrumentId"`
	PaymentInstrumentID string `json:"paymentInstrumentId"`
	Amount              string `json:"amount"`
	DailyTime           string `json:"dailyTime"`
	StartDate           string `json:"startDate"`
	TimeZone            string `json:"timeZone"`
}

// UpdatePortfolioAccount edits account metadata without manufacturing or revising holdings.
func (s *InvestmentService) UpdatePortfolioAccount(c core.Context, uid int64, input models.PortfolioAccount) (*models.PortfolioAccount, error) {
	input.Name = strings.TrimSpace(input.Name)
	if uid <= 0 || len(input.Id) == 0 || len(input.Id) > 64 || len([]rune(input.Name)) == 0 || len([]rune(input.Name)) > 64 || len(input.Instruments) > 100 || len(input.Platform) > 32 || !validatePortfolioCurrency(input.Currency) {
		return nil, investmentError("请填写有效的账户信息")
	}
	defer s.lock(uid)()
	var result models.PortfolioAccount
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		if _, err := sess.Where("uid=?", uid).Incr("revision", 1).Update(&models.InvestmentSettings{}); err != nil {
			return err
		}
		has, err := sess.Where("uid=? AND id=?", uid, input.Id).Get(&result)
		if err != nil {
			return err
		}
		if !has {
			return investmentError("找不到这个投资账户")
		}
		if input.Kind != result.Kind {
			return investmentError("已有账户的类型不能更改")
		}
		result.Name = input.Name
		if input.Currency != "" {
			result.Currency = input.Currency
		}
		if result.Kind == "EXCHANGE" || result.Kind == "WALLET" {
			if !validCryptoPlatform(result.Kind, input.Platform) {
				return investmentError("所选品牌与账户类型不一致；钱包品牌可留空")
			}
			known := map[string]bool{}
			for _, v := range investmentPresets {
				known[v.Id] = v.Type == "CRYPTO"
			}
			var own []models.InvestmentInstrument
			if err = sess.Where("uid=?", uid).Find(&own); err != nil {
				return err
			}
			for _, v := range own {
				known[v.Id] = v.Type == "CRYPTO"
			}
			seen := map[string]bool{}
			for _, id := range input.Instruments {
				if len(id) > 100 || !known[id] || seen[id] {
					return investmentError("请选择不重复的加密货币")
				}
				seen[id] = true
			}
			events, err := readInvestmentEvents(sess, uid)
			if err != nil {
				return err
			}
			positions, err := replayInvestments(events)
			if err != nil {
				return err
			}
			for _, p := range positions.Positions {
				quantity, err := decimal.NewFromString(p.Quantity)
				if err != nil {
					return err
				}
				if p.AccountID == result.Id && quantity.IsPositive() && !seen[p.InstrumentID] {
					return investmentError("持有中的币种不能移除，请先处理持仓")
				}
			}
			result.Platform, result.Instruments = input.Platform, input.Instruments
			if input.PaymentInstruments != nil {
				if err := validatePaymentInstruments(input.PaymentInstruments, input.Instruments); err != nil {
					return err
				}
				result.PaymentInstruments = input.PaymentInstruments
			} else if err := validatePaymentInstruments(result.PaymentInstruments, input.Instruments); err != nil {
				return err
			}
		}
		_, err = sess.Where("uid=? AND id=?", uid, result.Id).Cols("name", "platform", "instruments", "currency", "payment_instruments").Update(&result)
		return err
	})
	return &result, err
}

// CreateCryptoAccount saves the account, selected coins and all initial quantities together.
// Empty/zero quantities select a coin without fabricating an opening transaction or cost.
func (s *InvestmentService) CreateCryptoAccount(c core.Context, uid int64, input CryptoAccountInput, key string) (*models.PortfolioAccount, error) {
	if uid <= 0 || len(key) < 8 || len(key) > 128 {
		return nil, investmentError("页面状态已过期，请刷新后重试")
	}
	if len(input.DCAPlans) > 100 || (len(input.DCAPlans) > 0 && input.Kind != "EXCHANGE") {
		return nil, investmentError("首次定投仅支持交易所账户，最多 100 条计划")
	}
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) == 0 || len([]rune(input.Name)) > 64 || len(input.Holdings) > 100 || !validCryptoPlatform(input.Kind, input.Platform) || !validatePortfolioCurrency(input.Currency) {
		return nil, investmentError("请填写账户名称并核对账户类型；钱包品牌可留空")
	}
	raw, _ := json.Marshal(input)
	hash := sha256.Sum256(append([]byte("crypto-account:"), raw...))
	digest := hex.EncodeToString(hash[:])
	defer s.lock(uid)()
	var result *models.PortfolioAccount
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		// Serialize with investment mutations before reading any facts.
		if _, err := sess.Where("uid=?", uid).Incr("revision", 1).Update(&models.InvestmentSettings{}); err != nil {
			return err
		}
		previous := new(models.InvestmentIdempotency)
		has, err := sess.Where("uid=? AND request_key=?", uid, key).Get(previous)
		if err != nil {
			return err
		}
		if has {
			if previous.Digest != digest {
				return ErrInvestmentConflict
			}
			result = new(models.PortfolioAccount)
			return json.Unmarshal([]byte(previous.Response), result)
		}
		bookID, err := Books.ResolveInSession(sess, uid, input.BookID, false)
		if err != nil {
			return err
		}
		known := map[string]bool{}
		for _, v := range investmentPresets {
			known[v.Id] = v.Type == "CRYPTO"
		}
		var own []models.InvestmentInstrument
		if err = sess.Where("uid=?", uid).Find(&own); err != nil {
			return err
		}
		for _, v := range own {
			known[v.Id] = v.Type == "CRYPTO"
		}
		currency := input.Currency
		if currency == "" {
			currency = "CNY"
		}
		result = &models.PortfolioAccount{Id: investmentID(), Uid: uid, Name: input.Name, Kind: input.Kind, Platform: input.Platform, Instruments: []string{}, Currency: currency, PaymentInstruments: input.PaymentInstruments}
		seen := map[string]bool{}
		for _, h := range input.Holdings {
			if !known[h.InstrumentID] || seen[h.InstrumentID] {
				return investmentError("请选择不重复的加密货币")
			}
			seen[h.InstrumentID] = true
			result.Instruments = append(result.Instruments, h.InstrumentID)
		}
		if err = validatePaymentInstruments(input.PaymentInstruments, result.Instruments); err != nil {
			return err
		}
		if _, err = sess.Insert(result); err != nil {
			return err
		}
		events, err := readInvestmentEvents(sess, uid)
		if err != nil {
			return err
		}
		now := time.Now().Unix()
		for _, h := range input.Holdings {
			if h.Quantity == "" {
				h.Quantity = "0"
			}
			if err = investments.ValidateDecimal(h.Quantity); err != nil {
				return investmentError("数量格式无效，最多支持 18 位小数")
			}
			q, _ := decimal.NewFromString(h.Quantity)
			if q.IsNegative() {
				return investmentError("持有数量不能为负数")
			}
			var cost *string
			note := "录入已有加密货币持仓"
			if h.Cost != nil {
				if !q.IsPositive() {
					return investmentError("填写持有成本时，持有数量必须大于零")
				}
				original, err := investmentDecimal(*h.Cost, "持有成本", true)
				if err != nil {
					return err
				}
				rate := decimal.NewFromInt(1)
				if currency == "USD" {
					rate, err = investmentDecimal(h.CostExchangeRate, "美元成本汇率", true)
					if err != nil || !rate.IsPositive() {
						return investmentError("请填写大于零的美元成本汇率")
					}
				} else if h.CostExchangeRate != "" && h.CostExchangeRate != "1" {
					return investmentError("人民币成本汇率必须为 1")
				}
				converted := original.Mul(rate).Round(18).String()
				if original.IsPositive() && converted == "0" {
					return investmentError("折算后的成本小于支持的最小精度")
				}
				if investments.ValidateDecimal(converted) != nil {
					return investmentError("折算后的持有成本超出支持范围")
				}
				cost = &converted
				note += fmt.Sprintf("；录入总成本 %s %s，成本汇率 %s CNY/%s", original.String(), currency, rate.String(), currency)
			} else if h.CostExchangeRate != "" {
				return investmentError("请先填写持有成本，再填写成本汇率")
			}
			if q.IsZero() {
				continue
			}
			event := InvestmentEvent{Event: investments.Event{ID: fmt.Sprintf("%020d-%s", time.Now().UnixNano(), investmentID()), Version: 1, Type: "OPENING", AccountID: result.Id, InstrumentID: h.InstrumentID, Quantity: q.String(), Cost: cost, Amount: "0", Fee: "0", ExchangeRate: "1", OccurredAt: now, Note: note}, BookID: bookID}
			if err = s.validateEvent(sess, uid, &event); err != nil {
				return err
			}
			events = append(events, event)
			payload, _ := json.Marshal(event)
			if _, err = sess.Insert(&models.InvestmentEventRecord{Id: event.ID, Uid: uid, OccurredAt: now, Version: 1, Payload: string(payload)}); err != nil {
				return err
			}
			if _, err = sess.Insert(&models.InvestmentEventRevision{Id: investmentID(), Uid: uid, EventId: event.ID, Version: 1, RecordedAt: now, Payload: string(payload)}); err != nil {
				return err
			}
		}
		if _, err = replayInvestments(events); err != nil {
			return investmentError(localizeReplayError(sess, uid, err))
		}
		if len(input.DCAPlans) > 0 {
			count, err := sess.Where("uid=?", uid).Count(&models.CryptoDCAPlan{})
			if err != nil {
				return err
			}
			if count+int64(len(input.DCAPlans)) > 100 {
				return investmentError("定投计划数量已达上限")
			}
			for _, draft := range input.DCAPlans {
				if !seen[draft.InstrumentID] || !seen[draft.PaymentInstrumentID] {
					return investmentError("请在持有币种中保留定投的买入和支付币种")
				}
				p, err := newCryptoDCAPlan(uid, models.CryptoDCASaveRequest{AccountId: result.Id, BookId: bookID, InstrumentId: draft.InstrumentID, PaymentInstrumentId: draft.PaymentInstrumentID, Amount: draft.Amount, DailyTime: draft.DailyTime, StartDate: draft.StartDate, TimeZone: draft.TimeZone})
				if err != nil {
					return err
				}
				zone, _ := time.LoadLocation(p.TimeZone)
				if p.StartDate < time.Unix(now, 0).In(zone).Format("2006-01-02") {
					return investmentError("新账户定投不能早于今天；已有买入请计入初始持仓")
				}
				p.Id = investmentID()
				p.NextDate = cryptoDCAFutureDate(*p, time.Unix(now, 0))
				if _, err = sess.Insert(p); err != nil {
					return err
				}
			}
		}
		if err = InvalidateWealthSnapshots(sess, uid, now); err != nil {
			return err
		}
		response, _ := json.Marshal(result)
		_, err = sess.Insert(&models.InvestmentIdempotency{Id: investmentID(), Uid: uid, RequestKey: key, Digest: digest, Response: string(response)})
		return err
	})
	return result, err
}

type ConversionInput struct {
	FromInstrumentID string `json:"fromInstrumentId"` // Empty denotes CNY cash.
	ToInstrumentID   string `json:"toInstrumentId"`   // Empty denotes CNY cash.
	FromQuantity     string `json:"fromQuantity"`
}
type InvestmentConversion struct {
	ConversionInput
	ToQuantity string                    `json:"toQuantity"`
	FromPrice  string                    `json:"fromPrice"`
	ToPrice    string                    `json:"toPrice"`
	ObservedAt int64                     `json:"observedAt"`
	ExpiresAt  int64                     `json:"expiresAt"`
	FromQuote  *InvestmentValuationQuote `json:"fromQuote,omitempty"`
	ToQuote    *InvestmentValuationQuote `json:"toQuote,omitempty"`
}

func conversionFromQuotes(input ConversionInput, quotes []InvestmentValuationQuote, now int64) (*InvestmentConversion, error) {
	if input.FromInstrumentID == input.ToInstrumentID {
		return nil, investmentError("请选择不同的转出和转入币种")
	}
	if err := investments.ValidateDecimal(input.FromQuantity); err != nil {
		return nil, investmentError("请填写有效的转出金额或数量")
	}
	quantity, _ := decimal.NewFromString(input.FromQuantity)
	if !quantity.IsPositive() {
		return nil, investmentError("转出金额或数量必须大于零")
	}
	if input.FromInstrumentID == "" && quantity.Exponent() < -2 {
		return nil, investmentError("人民币金额最多支持两位小数")
	}
	lookup := func(id string) (decimal.Decimal, *InvestmentValuationQuote, error) {
		if id == "" {
			return decimal.NewFromInt(1), nil, nil
		}
		for _, q := range quotes {
			if q.InstrumentID != id {
				continue
			}
			if (q.State != marketquotes.StateLive && q.State != marketquotes.StateDelayed) || q.SourceTime <= 0 || q.SourceTime > now+120 || (q.State != marketquotes.StateLive && now-q.SourceTime > 300) || q.FXState == marketquotes.StateStale {
				break
			}
			price, e := decimal.NewFromString(q.Price)
			if e != nil || !price.IsPositive() {
				break
			}
			fx, e := decimal.NewFromString(q.FXRate)
			if e != nil || !fx.IsPositive() {
				break
			}
			if q.Currency != "CNY" && (q.FXDate == "" || q.FXSource == "" || q.FXState == marketquotes.StateUnavailable) {
				break
			}
			return price.Mul(fx), &q, nil
		}
		return decimal.Zero, nil, investmentError("暂时没有可用的最新行情或人民币汇率，请稍后重试")
	}
	from, fq, err := lookup(input.FromInstrumentID)
	if err != nil {
		return nil, err
	}
	to, tq, err := lookup(input.ToInstrumentID)
	if err != nil {
		return nil, err
	}
	precision := int32(18)
	if input.ToInstrumentID == "" {
		precision = 2
	}
	received := quantity.Mul(from).DivRound(to, 36).Truncate(precision)
	if !received.IsPositive() {
		return nil, investmentError("兑换数量过小，请增加转出金额")
	}
	return &InvestmentConversion{ConversionInput: input, ToQuantity: received.String(), FromPrice: from.Round(18).String(), ToPrice: to.Round(18).String(), ObservedAt: now, ExpiresAt: now + 120, FromQuote: fq, ToQuote: tq}, nil
}
func (s *InvestmentService) Conversion(c core.Context, uid int64, input ConversionInput) (*InvestmentConversion, error) {
	items, err := s.Instruments(c, uid)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{"": true}
	for _, item := range items {
		known[item.Id] = item.Type == "CRYPTO"
	}
	if !known[input.FromInstrumentID] || !known[input.ToInstrumentID] {
		return nil, investmentError("请选择当前账本中的加密货币")
	}
	ids := []string{}
	bindings := []marketquotes.Binding{}
	for _, id := range []string{input.FromInstrumentID, input.ToInstrumentID} {
		if id == "" {
			continue
		}
		key := id
		for _, item := range items {
			if item.Id == id && item.Provider != "" {
				b := instrumentBinding(item)
				bindings = append(bindings, b)
				key = b.Key()
			}
		}
		ids = append(ids, key)
	}
	marketquotes.Default.RefreshCryptoConversion(investmentContext(c), ids, bindings)
	quotes, err := s.Quotes(c, uid)
	if err != nil {
		return nil, err
	}
	return conversionFromQuotes(input, quotes, time.Now().Unix())
}
