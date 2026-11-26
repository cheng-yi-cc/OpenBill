package services

import (
	"regexp"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/shopspring/decimal"
	"xorm.io/xorm"
)

var transferFeePattern = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,12})(\.[0-9]{1,2})?$`)

// A metadata-only edit must not delete and recreate an unchanged cash fact.
// In particular, later processed monetary-income days may depend on its identity.
func (s *TransactionService) preserveUnchangedTransferFee(sess *xorm.Session, old, next *models.Transaction) (bool, error) {
	if old.Type != models.TRANSACTION_DB_TYPE_TRANSFER_OUT || next.Type != old.Type || old.AccountId != next.AccountId ||
		utils.GetUnixTimeFromTransactionTime(old.TransactionTime) != utils.GetUnixTimeFromTransactionTime(next.TransactionTime) {
		return false, nil
	}
	oldValue := old.TransferFeeAmount
	if oldValue == "" {
		oldValue = "0"
	}
	a, err := decimal.NewFromString(oldValue)
	if err != nil {
		return false, err
	}
	b, err := decimal.NewFromString(next.TransferFeeAmount)
	if err != nil {
		return false, err
	}
	if !a.Equal(b) {
		return false, nil
	}
	var fees []models.Transaction
	if err := sess.Where("uid=? AND transfer_fee_parent_id=? AND deleted=?", old.Uid, old.TransactionId, false).Find(&fees); err != nil {
		return false, err
	}
	if a.IsZero() {
		return len(fees) == 0, nil
	}
	if len(fees) != 1 {
		return false, nil
	}
	fee := fees[0]
	kind := models.TRANSACTION_DB_TYPE_EXPENSE
	if a.IsNegative() {
		kind = models.TRANSACTION_DB_TYPE_INCOME
	}
	if fee.Type != kind || fee.AccountId != old.AccountId ||
		utils.GetUnixTimeFromTransactionTime(fee.TransactionTime) != utils.GetUnixTimeFromTransactionTime(old.TransactionTime) ||
		!decimal.NewFromInt(fee.Amount).Equal(a.Abs().Shift(2)) {
		return false, nil
	}
	if fee.CategoryId != next.TransferFeeCategoryId {
		candidate := fee
		candidate.CategoryId = next.TransferFeeCategoryId
		if err := s.isCategoryValid(sess, &candidate); err != nil {
			return false, err
		}
	}
	if fee.BookId != next.BookId || fee.CategoryId != next.TransferFeeCategoryId || fee.HideAmount != next.HideAmount ||
		fee.ExcludeFromStatistics != next.ExcludeFromStatistics || fee.TimezoneUtcOffset != next.TimezoneUtcOffset {
		updated := &models.Transaction{BookId: next.BookId, CategoryId: next.TransferFeeCategoryId, HideAmount: next.HideAmount,
			ExcludeFromStatistics: next.ExcludeFromStatistics, TimezoneUtcOffset: next.TimezoneUtcOffset, UpdatedUnixTime: time.Now().Unix()}
		rows, err := sess.ID(fee.TransactionId).Where("uid=? AND deleted=?", old.Uid, false).
			Cols("book_id", "category_id", "hide_amount", "exclude_from_statistics", "timezone_utc_offset", "updated_unix_time").Update(updated)
		if err != nil {
			return false, err
		}
		if rows != 1 {
			return false, errs.ErrTransactionNotFound
		}
	}
	return true, nil
}

func validateTransferFee(transaction *models.Transaction) error {
	value := transaction.TransferFeeAmount
	if value == "" {
		value = "0"
	}
	if len(value) > 17 || !transferFeePattern.MatchString(value) {
		return investmentError("手续费金额无效，最多保留两位小数")
	}
	amount, err := decimal.NewFromString(value)
	if err != nil || amount.Abs().Mul(decimal.NewFromInt(100)).GreaterThan(decimal.NewFromInt(models.MaximumTransactionAmount)) {
		return investmentError("手续费金额超出允许范围")
	}
	if !amount.IsZero() && (transaction.Type != models.TRANSACTION_DB_TYPE_TRANSFER_OUT || transaction.TransferFeeCategoryId <= 0 || transaction.TransferFeeParentId != 0) {
		return investmentError("手续费只能关联转出账单，并须选择收支分类")
	}
	transaction.TransferFeeAmount = amount.StringFixed(2)
	if amount.IsZero() {
		transaction.TransferFeeCategoryId = 0
	}
	return nil
}

// A fee is an ordinary income/expense fact linked to its transfer. The two
// balances and all rows are changed inside the caller's database transaction.
func (s *TransactionService) createTransferFee(c core.Context, sess *xorm.Session, parent *models.Transaction) error {
	if err := validateTransferFee(parent); err != nil {
		return err
	}
	amount, _ := decimal.NewFromString(parent.TransferFeeAmount)
	if amount.IsZero() {
		return nil
	}
	kind, note := models.TRANSACTION_DB_TYPE_EXPENSE, "转账手续费"
	if amount.IsNegative() {
		kind, note = models.TRANSACTION_DB_TYPE_INCOME, "转账优惠"
	}
	fee := &models.Transaction{Uid: parent.Uid, BookId: parent.BookId, Type: kind, CategoryId: parent.TransferFeeCategoryId,
		AccountId: parent.AccountId, TransactionTime: parent.TransactionTime, TimezoneUtcOffset: parent.TimezoneUtcOffset,
		Amount: amount.Abs().Mul(decimal.NewFromInt(100)).IntPart(), Comment: note, HideAmount: parent.HideAmount,
		TransferFeeParentId: parent.TransactionId, ExcludeFromStatistics: parent.ExcludeFromStatistics}
	return s.createTransactionInSession(c, sess, fee, nil, nil)
}

func (s *TransactionService) removeTransferFee(sess *xorm.Session, uid, parentID int64, clearedAccounts ...map[int64]bool) error {
	var fees []models.Transaction
	if err := sess.Where("uid=? AND transfer_fee_parent_id=? AND deleted=?", uid, parentID, false).Find(&fees); err != nil {
		return err
	}
	for _, fee := range fees {
		if err := guardMonetaryFlowHistory(sess, uid, &fee, nil, clearedAccounts...); err != nil {
			return err
		}
		if fee.Type != models.TRANSACTION_DB_TYPE_INCOME && fee.Type != models.TRANSACTION_DB_TYPE_EXPENSE {
			return investmentError("手续费关联记录无效")
		}
		var account models.Account
		has, err := sess.ID(fee.AccountId).Where("uid=? AND deleted=? AND system_role=''", uid, false).Get(&account)
		if err != nil {
			return err
		}
		if !has {
			return errs.ErrAccountNotFound
		}
		change := fee.Amount
		if fee.Type == models.TRANSACTION_DB_TYPE_INCOME {
			change = -change
		}
		if rows, err := s.updateAccountBalance(sess, &account, change); err != nil {
			return err
		} else if rows != 1 {
			return errs.ErrDatabaseOperationFailed
		}
		if rows, err := sess.ID(fee.TransactionId).Where("uid=? AND deleted=?", uid, false).Cols("deleted", "deleted_unix_time").Update(&models.Transaction{Deleted: true, DeletedUnixTime: time.Now().Unix()}); err != nil {
			return err
		} else if rows != 1 {
			return errs.ErrTransactionNotFound
		}
	}
	return nil
}

func guardTransferFeeMutation(sess *xorm.Session, uid int64, ids []int64) error {
	query := sess.Where("uid=? AND deleted=? AND (transfer_fee_parent_id>0 OR transfer_fee_category_id>0 OR debt_due_date<>'')", uid, false)
	if len(ids) > 0 {
		query = query.In("transaction_id", ids)
	}
	has, err := query.Exist(&models.Transaction{})
	if err != nil {
		return err
	}
	if has {
		return investmentError("关联手续费或到期日请通过原转账账单修改或删除")
	}
	return nil
}
