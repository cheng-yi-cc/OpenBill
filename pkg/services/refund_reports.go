package services

import (
	"math/big"
	"slices"
	"strconv"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/shopspring/decimal"
	"xorm.io/xorm"
)

// RefundExcessReportRow is an unassigned-account income projection. Currency
// comes from the actual refund account, without attributing income to it.
type RefundExcessReportRow struct {
	CategoryId        int64
	BookId            string
	Currency          string
	Amount            int64
	TransactionTime   int64
	TimezoneUtcOffset int16
	TagIds            []int64
	Comment           string
}

// RefundUnassignedDailyIncome supplies currency totals to amount APIs without
// pretending that the separate reporting income belongs to a cash account.
func (s *InvestmentService) RefundUnassignedDailyIncome(c core.Context, uid, startUnix, endUnix int64, excludedCategories []int64, clientTimezone *time.Location, useTransactionTimezone bool) (map[int32]map[string]*big.Int, error) {
	if clientTimezone == nil {
		return nil, investmentError("统计时区无效")
	}
	startLocal := utils.FormatUnixTimeToNumericLocalDateTime(startUnix, clientTimezone)
	endLocal := utils.FormatUnixTimeToNumericLocalDateTime(endUnix, clientTimezone)
	minimum := utils.GetMinUnixTimeWithSameLocalDateTime(startUnix, utils.GetTimezoneOffsetMinutes(startUnix, clientTimezone))
	maximum := utils.GetMaxUnixTimeWithSameLocalDateTime(endUnix, utils.GetTimezoneOffsetMinutes(endUnix, clientTimezone))
	rows, err := s.RefundExcessReportRows(c, uid, utils.GetMinTransactionTimeFromUnixTime(minimum), utils.GetMaxTransactionTimeFromUnixTime(maximum), nil, false, "", core.MATCH_MODE_DEFAULT)
	if err != nil {
		return nil, err
	}
	result := map[int32]map[string]*big.Int{}
	for _, row := range rows {
		if slices.Contains(excludedCategories, row.CategoryId) {
			continue
		}
		zone := clientTimezone
		if useTransactionTimezone {
			zone = time.FixedZone("Transaction Timezone", int(row.TimezoneUtcOffset)*60)
		}
		at := utils.GetUnixTimeFromTransactionTime(row.TransactionTime)
		local := utils.FormatUnixTimeToNumericLocalDateTime(at, zone)
		if local < startLocal || local > endLocal {
			continue
		}
		day := utils.FormatUnixTimeToNumericYearMonthDay(at, zone)
		if result[day] == nil {
			result[day] = map[string]*big.Int{}
		}
		if result[day][row.Currency] == nil {
			result[day][row.Currency] = big.NewInt(0)
		}
		result[day][row.Currency].Add(result[day][row.Currency], big.NewInt(row.Amount))
	}
	return result, nil
}

func refundTagsMatch(tags []int64, filters []*models.TransactionTagFilter, noTags bool) bool {
	if noTags {
		return len(tags) == 0
	}
	for _, filter := range filters {
		found := 0
		for _, id := range filter.TagIds {
			if slices.Contains(tags, id) {
				found++
			}
		}
		switch filter.Type {
		case models.TRANSACTION_TAG_FILTER_HAS_ANY:
			if found == 0 {
				return false
			}
		case models.TRANSACTION_TAG_FILTER_HAS_ALL:
			if found != len(filter.TagIds) {
				return false
			}
		case models.TRANSACTION_TAG_FILTER_NOT_HAS_ANY:
			if found != 0 {
				return false
			}
		case models.TRANSACTION_TAG_FILTER_NOT_HAS_ALL:
			if found == len(filter.TagIds) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// RefundExcessReportRows reads all refund facts for each related expense so a
// range or book filter cannot hide an earlier fact needed to calculate excess.
// start/end are transaction sequence bounds, zero means unbounded.
func (s *InvestmentService) RefundExcessReportRows(c core.Context, uid, start, end int64, filters []*models.TransactionTagFilter, noTags bool, keyword string, matchMode core.MatchMode) ([]RefundExcessReportRow, error) {
	if uid <= 0 {
		return nil, errs.ErrUserIdInvalid
	}
	result := []RefundExcessReportRow{}
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		var rows []models.Refund
		if err := sess.Where("uid=? AND deleted=?", uid, false).Find(&rows); err != nil {
			return err
		}
		expenses := map[int64]bool{}
		for _, row := range rows {
			expenses[row.ExpenseId] = true
		}
		var scopedBooks []string
		if c != nil {
			scopedBooks, _ = c.Value(bookFilterKey{}).([]string)
		}
		for expenseID := range expenses {
			summary, err := refundSummary(sess, uid, expenseID)
			if err != nil {
				return err
			}
			for _, view := range summary.Refunds {
				value, err := decimal.NewFromString(view.ExcessAmount)
				if err != nil || value.IsNegative() {
					return investmentError("退款差额不一致，请先核对账本")
				}
				if value.IsZero() {
					continue
				}
				info := view.Excess
				if info == nil {
					return investmentError("退款差额信息不完整，请先核对账本")
				}
				if info.ExcludeFromStatistics || len(scopedBooks) > 0 && !slices.Contains(scopedBooks, info.BookId) {
					continue
				}
				cashID, err := strconv.ParseInt(view.TransactionId, 10, 64)
				if err != nil {
					return err
				}
				var cash models.Transaction
				has, err := sess.ID(cashID).Where("uid=? AND deleted=?", uid, false).Get(&cash)
				if err != nil {
					return err
				}
				if !has {
					return investmentError("退款流水不存在")
				}
				if start > 0 && cash.TransactionTime < start || end > 0 && cash.TransactionTime > end {
					continue
				}
				tagIDs := make([]int64, 0, len(info.TagIds))
				for _, text := range info.TagIds {
					id, err := strconv.ParseInt(text, 10, 64)
					if err != nil {
						return err
					}
					tagIDs = append(tagIDs, id)
				}
				if !refundTagsMatch(tagIDs, filters, noTags) {
					continue
				}
				if keyword != "" {
					query := "SELECT CASE WHEN ? LIKE ? THEN 1 ELSE 0 END AS matched"
					if matchMode == core.MATCH_MODE_IGNORE_CASE {
						query = "SELECT CASE WHEN LOWER(?) LIKE LOWER(?) THEN 1 ELSE 0 END AS matched"
					}
					var match struct{ Matched int }
					_, err := sess.SQL(query, info.Comment, "%"+keyword+"%").Get(&match)
					if err != nil {
						return err
					}
					if match.Matched != 1 {
						continue
					}
				}
				var account models.Account
				has, err = sess.ID(cash.AccountId).Where("uid=? AND deleted=?", uid, false).Get(&account)
				if err != nil {
					return err
				}
				if !has || account.Currency == "" {
					return investmentError("退款币种不存在")
				}
				categoryID, err := strconv.ParseInt(info.CategoryId, 10, 64)
				if err != nil || categoryID <= 0 {
					return investmentError("退款差额分类不存在")
				}
				result = append(result, RefundExcessReportRow{CategoryId: categoryID, BookId: info.BookId, Currency: account.Currency,
					Amount: value.Shift(2).IntPart(), TransactionTime: cash.TransactionTime, TimezoneUtcOffset: cash.TimezoneUtcOffset, TagIds: tagIDs, Comment: info.Comment})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
