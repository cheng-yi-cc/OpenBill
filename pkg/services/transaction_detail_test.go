package services

import (
	"fmt"
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestTransactionDetailMetadataAndLinkedRelations(t *testing.T) {
	f, tx := feeFixture(t)
	tx.Type = models.TRANSACTION_DB_TYPE_EXPENSE
	tx.CategoryId = 602
	tx.RelatedAccountId = 0
	tx.RelatedAccountAmount = 0
	tx.TransferFeeAmount = "0"
	tx.TransferFeeCategoryId = 0
	require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))

	budgetExcluded := true
	tx.ExcludeFromStatistics = false
	tx.SetBudgetExclusion(&budgetExcluded)
	tx.LocationName = "测试商场"
	tx.LocationAddress = "测试路1号"
	require.NoError(t, Transactions.ModifyTransaction(nil, tx, false, 0, nil, nil, nil, nil))

	stored, err := Transactions.GetTransactionByTransactionId(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.False(t, stored.ExcludeFromStatistics)
	require.True(t, stored.IsExcludedFromBudget())
	require.Equal(t, tx.LocationAddress, stored.LocationAddress)

	other := *tx
	other.TransactionId = 0
	other.LinkedTransactionIds = []string{fmt.Sprint(tx.TransactionId)}
	other.TransactionTime = utils.GetMinTransactionTimeFromUnixTime(f.at + 3)
	require.NoError(t, Transactions.CreateTransaction(nil, &other, nil, nil))

	relations, err := Transactions.Relations(nil, f.uid, fmt.Sprint(tx.TransactionId))
	require.NoError(t, err)
	require.Contains(t, relations.IDs, fmt.Sprint(other.TransactionId))

	// User isolation & invalid self-link guard.
	_, err = Transactions.Relations(nil, f.uid+1, fmt.Sprint(tx.TransactionId))
	require.Error(t, err)
	other.LinkedTransactionIds = []string{fmt.Sprint(other.TransactionId)}
	require.Error(t, Transactions.ModifyTransaction(nil, &other, false, 0, nil, nil, nil, nil))

	// Refund relationships from the canonical Refund table also surface bidirectionally.
	summary, err := Investments.CreateRefund(nil, f.uid, refundTestActor(f.uid), RefundInput{
		ExpenseId: fmt.Sprint(tx.TransactionId),
		AccountId: fmt.Sprint(f.bankID),
		Amount:    "30",
		Time:      f.at + 10,
		TimeZone:  "Asia/Shanghai",
		RequestId: "detail-relations-refund-001",
	})
	require.NoError(t, err)
	require.Len(t, summary.Refunds, 1)
	afterRefund, err := Transactions.Relations(nil, f.uid, fmt.Sprint(tx.TransactionId))
	require.NoError(t, err)
	require.Contains(t, afterRefund.IDs, summary.Refunds[0].TransactionId)
}
