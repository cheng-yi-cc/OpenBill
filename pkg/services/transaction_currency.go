package services

import (
	"regexp"
	"strings"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/shopspring/decimal"
)

var (
	originalCurrencyAmountPattern = regexp.MustCompile(`^(0|[1-9]\d{0,12})(\.\d{1,2})?$`)
	originalExchangeRatePattern   = regexp.MustCompile(`^(0|[1-9]\d{0,12})(\.\d{1,12})?$`)
)

func clearTransactionOriginalCurrency(tx *models.Transaction) {
	tx.OriginalCurrency = ""
	tx.OriginalAmount = ""
	tx.OriginalExchangeRate = ""
	tx.OriginalFXDate = ""
}

func validateTransactionOriginalCurrency(tx *models.Transaction, sourceAccount *models.Account) error {
	if tx.OriginalCurrency == "" && tx.OriginalAmount == "" && tx.OriginalExchangeRate == "" && tx.OriginalFXDate == "" {
		return nil
	}
	if tx.Type != models.TRANSACTION_DB_TYPE_EXPENSE && tx.Type != models.TRANSACTION_DB_TYPE_INCOME {
		return investmentError("仅支出或收入账单支持切换记账币种")
	}
	if tx.OriginalCurrency == "" || tx.OriginalAmount == "" || tx.OriginalExchangeRate == "" {
		return investmentError("跨币种账单缺少原币金额或汇率信息")
	}
	tx.OriginalCurrency = strings.ToUpper(strings.TrimSpace(tx.OriginalCurrency))
	tx.OriginalAmount = strings.TrimSpace(tx.OriginalAmount)
	tx.OriginalExchangeRate = strings.TrimSpace(tx.OriginalExchangeRate)
	tx.OriginalFXDate = strings.TrimSpace(tx.OriginalFXDate)

	if sourceAccount == nil {
		return investmentError("账户信息缺失，无法校验记账币种")
	}
	if (tx.OriginalCurrency != "CNY" && tx.OriginalCurrency != "USD") || (sourceAccount.Currency != "CNY" && sourceAccount.Currency != "USD") {
		return investmentError("目前仅支持人民币与美元之间的记账币种切换")
	}
	if tx.OriginalCurrency == sourceAccount.Currency {
		return investmentError("原币币种不能与账户币种相同")
	}
	if !originalCurrencyAmountPattern.MatchString(tx.OriginalAmount) {
		return investmentError("原币金额格式无效")
	}
	if !originalExchangeRatePattern.MatchString(tx.OriginalExchangeRate) {
		return investmentError("记账汇率格式无效")
	}
	if tx.OriginalFXDate != "" {
		if len(tx.OriginalFXDate) != 10 {
			return investmentError("汇率日期格式无效")
		}
		if _, err := time.Parse("2006-01-02", tx.OriginalFXDate); err != nil {
			return investmentError("汇率日期无效")
		}
	}

	origAmount, err := decimal.NewFromString(tx.OriginalAmount)
	if err != nil || !origAmount.IsPositive() {
		return investmentError("原币金额必须大于零")
	}
	rate, err := decimal.NewFromString(tx.OriginalExchangeRate)
	if err != nil || !rate.IsPositive() {
		return investmentError("记账汇率必须大于零")
	}

	var converted decimal.Decimal
	if tx.OriginalCurrency == "USD" && sourceAccount.Currency == "CNY" {
		converted = origAmount.Mul(rate)
	} else {
		converted = origAmount.Div(rate)
	}
	expectedMinor := converted.Mul(decimal.NewFromInt(100)).Round(0)
	if !expectedMinor.Equal(decimal.NewFromInt(tx.Amount)) {
		return investmentError("折算金额与原币汇率不一致，请刷新汇率后重试")
	}
	return nil
}
