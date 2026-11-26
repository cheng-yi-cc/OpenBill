package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/shopspring/decimal"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"
	"xorm.io/xorm"
)

type AssetAdjustmentInput struct {
	AccountId         string   `json:"accountId"`
	Balance           string   `json:"balance"`
	ExpectedBalance   string   `json:"expectedBalance"`
	BookId            string   `json:"bookId"`
	CountInStatistics bool     `json:"countInStatistics"`
	TimeZone          string   `json:"timeZone"`
	RequestId         string   `json:"requestId"`
	CategoryId        string   `json:"categoryId,omitempty"`
	Comment           *string  `json:"comment,omitempty"`
	TagIds            []string `json:"tagIds,omitempty"`
}

func (s *InvestmentService) AdjustAssetBalance(c core.Context, uid int64, input AssetAdjustmentInput) (*models.AssetAdjustment, error) {
	id, err := strconv.ParseInt(input.AccountId, 10, 64)
	zone, zoneErr := time.LoadLocation(input.TimeZone)
	if err != nil || uid <= 0 || id <= 0 || zoneErr != nil || input.TimeZone == "" || len(input.TimeZone) > 64 || len(input.RequestId) < 8 || len(input.RequestId) > 64 || len(input.BookId) > 64 || !assetAmountPattern.MatchString(input.Balance) || !assetAmountPattern.MatchString(input.ExpectedBalance) {
		return nil, investmentError("请检查余额、账户和时区")
	}
	if input.Comment != nil && utf8.RuneCountInString(*input.Comment) > 200 || len(input.TagIds) > models.MaximumTagsCountOfTransaction || !input.CountInStatistics && (input.CategoryId != "" || input.Comment != nil || len(input.TagIds) > 0) {
		return nil, investmentError("请检查差额账单的分类、备注和标签")
	}
	categoryID := int64(0)
	if input.CategoryId != "" {
		categoryID, err = strconv.ParseInt(input.CategoryId, 10, 64)
		if err != nil || categoryID <= 0 {
			return nil, investmentError("请选择有效分类")
		}
	}
	tagIDs := make([]int64, 0, len(input.TagIds))
	seenTags := make(map[int64]bool, len(input.TagIds))
	for _, tag := range input.TagIds {
		tagID, parseErr := strconv.ParseInt(tag, 10, 64)
		if parseErr != nil || tagID <= 0 || seenTags[tagID] {
			return nil, investmentError("请选择有效标签")
		}
		seenTags[tagID] = true
		tagIDs = append(tagIDs, tagID)
	}
	target, _ := decimal.NewFromString(input.Balance)
	expected, _ := decimal.NewFromString(input.ExpectedBalance)
	data, _ := json.Marshal(input)
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	row := &models.AssetAdjustment{Id: fmt.Sprintf("%d:%s", uid, input.RequestId), Uid: uid, Digest: digest}
	defer s.lock(uid)()
	err = s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		var existing models.AssetAdjustment
		has, e := sess.ID(row.Id).Where("uid=?", uid).Get(&existing)
		if e != nil {
			return e
		}
		if has {
			if existing.Digest != digest {
				return investmentError("页面状态已过期，请刷新后重试")
			}
			*row = existing
			return nil
		}
		var a models.Account
		has, e = sess.ID(id).Where("uid=? AND deleted=? AND hidden=? AND system_role=?", uid, false, false, "").Get(&a)
		if e != nil {
			return e
		}
		if !has || a.Type != models.ACCOUNT_TYPE_SINGLE_ACCOUNT || a.IsReimbursement() {
			return investmentError("该账户不能直接校准余额")
		}
		if a.Balance != expected.Shift(2).IntPart() {
			return investmentError("账户余额已变化，请刷新后重新核对")
		}
		delta := target.Sub(expected)
		if delta.IsZero() {
			_, e = sess.Insert(row)
			return e
		}
		if delta.Abs().Shift(2).GreaterThan(decimal.NewFromInt(models.MaximumTransactionAmount)) {
			return investmentError("调整差额超出支持范围")
		}
		book, e := Books.ResolveInSession(sess, uid, input.BookId, false)
		if e != nil {
			return e
		}
		kind := models.CATEGORY_TYPE_INCOME
		txType := models.TRANSACTION_DB_TYPE_INCOME
		if delta.IsNegative() {
			kind = models.CATEGORY_TYPE_EXPENSE
			txType = models.TRANSACTION_DB_TYPE_EXPENSE
		}
		category := categoryID
		if category == 0 {
			category, e = assetIncomeCategory(sess, uid, book, kind, "余额校准")
			if e != nil {
				return e
			}
		} else {
			var child, parent models.TransactionCategory
			has, e := sess.ID(category).Where("uid=? AND deleted=? AND hidden=? AND type=? AND parent_category_id>0", uid, false, false, kind).Get(&child)
			if e != nil {
				return e
			}
			if !has || len(child.BookIds) > 0 && !slices.Contains(child.BookIds, book) {
				return investmentError("请选择该账本可用的差额分类")
			}
			has, e = sess.ID(child.ParentCategoryId).Where("uid=? AND deleted=? AND hidden=? AND type=?", uid, false, false, kind).Get(&parent)
			if e != nil {
				return e
			}
			if !has || len(parent.BookIds) > 0 && !slices.Contains(parent.BookIds, book) {
				return investmentError("请选择该账本可用的差额分类")
			}
		}
		now := time.Now()
		_, offset := now.In(zone).Zone()
		comment := "余额校准"
		if input.Comment != nil {
			comment = *input.Comment
		}
		tx := &models.Transaction{Uid: uid, BookId: book, AccountId: id, Type: txType, CategoryId: category, Amount: delta.Abs().Shift(2).IntPart(), TransactionTime: utils.GetMinTransactionTimeFromUnixTime(now.Unix()), TimezoneUtcOffset: int16(offset / 60), Comment: comment, ExcludeFromStatistics: !input.CountInStatistics}
		if e = Transactions.createTransactionInSession(c, sess, tx, tagIDs, nil); e != nil {
			return e
		}
		row.TransactionId = tx.TransactionId
		_, e = sess.Insert(row)
		return e
	})
	return row, err
}
