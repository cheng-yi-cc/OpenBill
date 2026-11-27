package services

import (
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestTransactionOriginalCurrencyCreateModifyAndValidation(t *testing.T) {
	f, tx := feeFixture(t)
	_, err := f.engine.Insert(&models.Account{
		AccountId: 1004,
		Uid:       f.uid,
		Name:      "美元卡",
		Type:      models.ACCOUNT_TYPE_SINGLE_ACCOUNT,
		Category:  models.ACCOUNT_CATEGORY_CASH,
		Currency:  "USD",
		Balance:   100000,
	})
	require.NoError(t, err)

	// USD entry recorded into CNY bank account at 1 USD = 7.25 CNY -> $10.00 = ¥72.50 (7250 fen)
	tx.Type = models.TRANSACTION_DB_TYPE_EXPENSE
	tx.CategoryId = 602
	tx.RelatedAccountId = 0
	tx.RelatedAccountAmount = 0
	tx.TransferFeeAmount = "0"
	tx.TransferFeeCategoryId = 0
	tx.Amount = 7250
	tx.OriginalCurrency = "USD"
	tx.OriginalAmount = "10"
	tx.OriginalExchangeRate = "7.25"
	tx.OriginalFXDate = "2026-11-27"
	require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))
	require.Equal(t, int64(1992750), f.balance())

	stored, err := Transactions.GetTransactionByTransactionId(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "USD", stored.OriginalCurrency)
	require.Equal(t, "10", stored.OriginalAmount)
	require.Equal(t, "7.25", stored.OriginalExchangeRate)
	require.Equal(t, "2026-11-27", stored.OriginalFXDate)

	// Modifying comment preserves original currency metadata and updates persisted value.
	tx.Comment = "AWS 月费"
	tx.Amount = 14500
	tx.OriginalAmount = "20"
	require.NoError(t, Transactions.ModifyTransaction(nil, tx, false, 0, nil, nil, nil, nil))
	require.Equal(t, int64(1985500), f.balance())

	updated, err := Transactions.GetTransactionByTransactionId(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "AWS 月费", updated.Comment)
	require.Equal(t, "20", updated.OriginalAmount)
	require.Equal(t, "7.25", updated.OriginalExchangeRate)

	// CNY entry recorded into USD account at 1 USD = 7.25 CNY -> ¥100.00 / 7.25 = 13.7931... -> $13.79 (1379 cents)
	cnyToUsd := &models.Transaction{
		Uid:                  f.uid,
		AccountId:            1004,
		Type:                 models.TRANSACTION_DB_TYPE_INCOME,
		CategoryId:           702,
		Amount:               1379,
		TransactionTime:      utils.GetMinTransactionTimeFromUnixTime(f.at + 5),
		OriginalCurrency:     "CNY",
		OriginalAmount:       "100",
		OriginalExchangeRate: "7.25",
		OriginalFXDate:       "2026-11-27",
	}
	require.NoError(t, Transactions.CreateTransaction(nil, cnyToUsd, nil, nil))

	// Reject mismatched converted amount.
	badAmount := &models.Transaction{
		Uid:                  f.uid,
		AccountId:            f.bankID,
		Type:                 models.TRANSACTION_DB_TYPE_EXPENSE,
		CategoryId:           602,
		Amount:               7200,
		TransactionTime:      utils.GetMinTransactionTimeFromUnixTime(f.at + 6),
		OriginalCurrency:     "USD",
		OriginalAmount:       "10",
		OriginalExchangeRate: "7.25",
		OriginalFXDate:       "2026-11-27",
	}
	require.Error(t, Transactions.CreateTransaction(nil, badAmount, nil, nil))

	// Reject same-currency original metadata or unsupported transfer metadata.
	sameCurr := *badAmount
	sameCurr.Amount = 1000
	sameCurr.OriginalCurrency = "CNY"
	sameCurr.TransactionTime = utils.GetMinTransactionTimeFromUnixTime(f.at + 7)
	require.Error(t, Transactions.CreateTransaction(nil, &sameCurr, nil, nil))

	transferWithOrig := *badAmount
	transferWithOrig.Type = models.TRANSACTION_DB_TYPE_TRANSFER_OUT
	transferWithOrig.CategoryId = 502
	transferWithOrig.RelatedAccountId = 1003
	transferWithOrig.Amount = 7250
	transferWithOrig.RelatedAccountAmount = 7250
	transferWithOrig.TransactionTime = utils.GetMinTransactionTimeFromUnixTime(f.at + 8)
	require.Error(t, Transactions.CreateTransaction(nil, &transferWithOrig, nil, nil))
}
