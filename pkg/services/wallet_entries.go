package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/investments"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/mayswind/ezbookkeeping/pkg/uuid"
	"github.com/shopspring/decimal"
	"xorm.io/xorm"
)

// WalletEntry stores the original payment unit and category alongside the
// investment fact. The ordinary ledger contains one linked CNY posting for
// reporting; that system posting is excluded from all asset balances.
type WalletEntry struct {
	Currency              string                                       `json:"currency"`
	CategoryID            string                                       `json:"categoryId"`
	FXDate                string                                       `json:"fxDate"`
	FXSource              string                                       `json:"fxSource"`
	ExcludeFromStatistics *bool                                        `json:"excludeFromStatistics,omitempty"`
	ExcludeFromBudget     *bool                                        `json:"excludeFromBudget,omitempty"`
	HideAmount            *bool                                        `json:"hideAmount,omitempty"`
	LocationName          *string                                      `json:"locationName,omitempty"`
	TagIDs                *[]string                                    `json:"tagIds,omitempty"`
	DiscountAmount        *string                                      `json:"discountAmount,omitempty"`
	PictureIDs            *[]string                                    `json:"pictureIds,omitempty"`
	Pictures              models.TransactionPictureInfoBasicResponseSlice `json:"pictures,omitempty"`
}

// walletTagIndexes reads the active posting before settlement is reversed.
// An omitted field from an older client preserves these authoritative indexes.
func walletTagIndexes(sess *xorm.Session, uid int64, eventID string) ([]models.TransactionTagIndex, error) {
	var postings []models.Transaction
	if err := sess.Where("uid=? AND investment_event_id=? AND deleted=?", uid, eventID, false).Find(&postings); err != nil {
		return nil, err
	}
	if len(postings) != 1 {
		return nil, investmentError("钱包流水关联异常，请先检查账本")
	}
	var indexes []models.TransactionTagIndex
	err := sess.Where("uid=? AND transaction_id=? AND deleted=?", uid, postings[0].TransactionId, false).Find(&indexes)
	return indexes, err
}

func walletTagIDs(w *WalletEntry, previous []models.TransactionTagIndex) ([]int64, error) {
	if w.TagIDs == nil {
		ids := make([]string, 0, len(previous))
		for _, index := range previous {
			ids = append(ids, strconv.FormatInt(index.TagId, 10))
		}
		w.TagIDs = &ids
	}
	if len(*w.TagIDs) > models.MaximumTagsCountOfTransaction {
		return nil, investmentError("钱包收支标签数量超出上限")
	}
	ids := make([]int64, 0, len(*w.TagIDs))
	seen := make(map[int64]bool, len(*w.TagIDs))
	for _, text := range *w.TagIDs {
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != text || seen[id] {
			return nil, investmentError("钱包收支标签无效或重复")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func walletPictureInfos(sess *xorm.Session, uid int64, eventID string) ([]models.TransactionPictureInfo, error) {
	var postings []models.Transaction
	if err := sess.Where("uid=? AND investment_event_id=? AND deleted=?", uid, eventID, false).Find(&postings); err != nil {
		return nil, err
	}
	if len(postings) != 1 {
		return nil, investmentError("钱包流水关联异常，请先检查账本")
	}
	var infos []models.TransactionPictureInfo
	err := sess.Where("uid=? AND transaction_id=? AND deleted=?", uid, postings[0].TransactionId, false).OrderBy("picture_id asc").Find(&infos)
	return infos, err
}

func walletPictureResponses(infos []models.TransactionPictureInfo) models.TransactionPictureInfoBasicResponseSlice {
	if len(infos) == 0 {
		return nil
	}
	result := make(models.TransactionPictureInfoBasicResponseSlice, 0, len(infos))
	for _, info := range infos {
		p := info
		result = append(result, p.ToTransactionPictureInfoBasicResponse(fmt.Sprintf("/pictures/%d.%s", p.PictureId, p.PictureExtension)))
	}
	return result
}

func walletPictureIDs(sess *xorm.Session, uid int64, w *WalletEntry, previous []models.TransactionPictureInfo) ([]int64, models.TransactionPictureInfoBasicResponseSlice, error) {
	if w.PictureIDs == nil {
		ids := make([]string, 0, len(previous))
		picIDs := make([]int64, 0, len(previous))
		for _, info := range previous {
			ids = append(ids, strconv.FormatInt(info.PictureId, 10))
			picIDs = append(picIDs, info.PictureId)
		}
		w.PictureIDs = &ids
		return picIDs, walletPictureResponses(previous), nil
	}
	if len(*w.PictureIDs) > models.MaximumPicturesCountOfTransaction {
		return nil, nil, investmentError("钱包收支附件数量超出上限")
	}
	ids := make([]int64, 0, len(*w.PictureIDs))
	seen := make(map[int64]bool, len(*w.PictureIDs))
	prevMap := make(map[int64]models.TransactionPictureInfo, len(previous))
	for _, info := range previous {
		prevMap[info.PictureId] = info
	}
	var lookupNew []int64
	for _, text := range *w.PictureIDs {
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != text || seen[id] {
			return nil, nil, investmentError("钱包收支附件无效或重复")
		}
		seen[id] = true
		ids = append(ids, id)
		if _, ok := prevMap[id]; !ok {
			lookupNew = append(lookupNew, id)
		}
	}
	newMap := make(map[int64]models.TransactionPictureInfo, len(lookupNew))
	if len(lookupNew) > 0 {
		var rows []models.TransactionPictureInfo
		if err := sess.Where("uid=? AND deleted=? AND transaction_id=?", uid, false, models.TransactionPictureNewPictureTransactionId).In("picture_id", lookupNew).Find(&rows); err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			newMap[row.PictureId] = row
		}
	}
	infos := make([]models.TransactionPictureInfo, 0, len(ids))
	for _, id := range ids {
		if info, ok := prevMap[id]; ok {
			infos = append(infos, info)
			continue
		}
		if info, ok := newMap[id]; ok {
			infos = append(infos, info)
			continue
		}
		return nil, nil, investmentError("钱包收支附件无效或已被其他账单使用")
	}
	return ids, walletPictureResponses(infos), nil
}

func (s *InvestmentService) validateWalletEntry(sess *xorm.Session, uid int64, e *InvestmentEvent, old *InvestmentEvent) error {
	w := e.Wallet
	if w == nil || (w.Currency != "CNY" && w.Currency != "USD") || len(w.CategoryID) > 20 || len(w.FXSource) > 100 || len(w.FXDate) > 10 || utf8.RuneCountInString(e.Note) > 255 || e.CashAccountID != "" || e.Conversion != nil || e.OccurredAt <= 0 || e.OccurredAt > time.Now().Unix()+60 {
		return investmentError("请核对钱包收支的币种、日期和备注")
	}
	if _, err := time.Parse("2006-01-02", w.FXDate); err != nil || w.FXSource == "" {
		return investmentError("请确认本笔收支的人民币汇率及日期")
	}
	if w.LocationName != nil && utf8.RuneCountInString(*w.LocationName) > 200 {
		return investmentError("地点名称不能超过 200 个字符")
	}
	if w.Currency == "CNY" && e.ExchangeRate != "1" {
		return investmentError("人民币记账汇率必须为 1")
	}
	for _, value := range []string{e.Amount, e.ExchangeRate} {
		if err := investments.ValidateDecimal(value); err != nil {
			return investmentError("金额和汇率请输入有效数字（最多 18 位小数）")
		}
	}
	amount, _ := decimal.NewFromString(e.Amount)
	rate, _ := decimal.NewFromString(e.ExchangeRate)
	minor := amount.Mul(rate).Round(2).Shift(2)
	if !amount.IsPositive() || amount.Exponent() < -2 || !rate.IsPositive() || !minor.IsPositive() || minor.GreaterThan(decimal.NewFromInt(models.MaximumTransactionAmount)) {
		return investmentError("金额或折算金额超出范围，记账金额最多两位小数")
	}
	if w.DiscountAmount != nil {
		trimmed := strings.TrimSpace(*w.DiscountAmount)
		if trimmed == "" {
			trimmed = "0"
		}
		normalized, err := normalizeStatisticsMoney(trimmed)
		if err != nil {
			return investmentError("优惠金额格式无效，请填写非负且最多两位小数的金额")
		}
		discount, _ := decimal.NewFromString(normalized)
		limit := decimal.NewFromInt(models.MaximumTransactionAmount)
		if discount.Shift(2).GreaterThan(limit) || discount.Mul(rate).Round(2).Shift(2).GreaterThan(limit) {
			return investmentError("优惠金额超出范围")
		}
		*w.DiscountAmount = normalized
	}
	var account models.PortfolioAccount
	found, err := sess.Where("uid=? AND id=?", uid, e.AccountID).Get(&account)
	if err != nil {
		return err
	}
	if !found || (account.Kind != "WALLET" && account.Kind != "EXCHANGE") {
		return investmentError("请选择自己的加密钱包或交易所账户")
	}
	prefs, err := readAssetPreferences(sess, uid)
	if err != nil {
		return err
	}
	for _, bookID := range prefs.Rules["portfolio:"+e.AccountID].DisabledBooks {
		if bookID == e.BookID {
			return investmentError("此账户在所选账本中未启用")
		}
	}
	movements := append([]investments.AssetMovement{{InstrumentID: e.InstrumentID, Quantity: e.Quantity}}, e.AdditionalMovements...)
	if len(movements) > 2 {
		return investmentError("一笔钱包收支最多使用两种币")
	}
	for _, movement := range movements {
		if len(movement.InstrumentID) > 100 {
			return investmentError("币种无效")
		}
		known := false
		for _, preset := range investmentPresets {
			if preset.Id == movement.InstrumentID && preset.Type == "CRYPTO" {
				known = true
			}
		}
		if !known {
			known, err = sess.Where("uid=? AND id=? AND type=?", uid, movement.InstrumentID, "CRYPTO").Exist(&models.InvestmentInstrument{})
			if err != nil {
				return err
			}
		}
		if !known {
			return investmentError("请选择当前账本中的加密货币")
		}
	}
	category, err := strconv.ParseInt(w.CategoryID, 10, 64)
	if err != nil || category <= 0 {
		return investmentError("请选择收支分类")
	}
	tx := &models.Transaction{Uid: uid, CategoryId: category, Type: models.TRANSACTION_DB_TYPE_EXPENSE}
	if e.Type == investments.Income {
		tx.Type = models.TRANSACTION_DB_TYPE_INCOME
	}
	// Linked investment postings normally bypass category validation; wallet
	// consumption is ordinary income/expense and must validate it explicitly.
	oldBook := ""
	if old != nil {
		oldBook = old.BookID
		if oldBook == "" {
			oldBook = DefaultBookID(uid)
		}
	}
	unchanged := old != nil && old.Wallet != nil && old.Type == e.Type && oldBook == e.BookID && old.Wallet.CategoryID == w.CategoryID
	return Transactions.isCategoryValidWithHidden(sess, tx, unchanged)
}

func (s *InvestmentService) settleWallet(c core.Context, sess *xorm.Session, uid int64, e InvestmentEvent, zone string, tagIDs []int64, previous []models.TransactionTagIndex, pictureIDs []int64, previousPictures []models.TransactionPictureInfo) error {
	system := new(models.Account)
	has, err := sess.Where("uid=? AND currency=? AND system_role=? AND deleted=?", uid, "CNY", InvestmentSettlementRole, false).Get(system)
	if err != nil {
		return err
	}
	if !has {
		system = &models.Account{AccountId: Accounts.GenerateUuid(uuid.UUID_TYPE_ACCOUNT), Uid: uid, Category: models.ACCOUNT_CATEGORY_VIRTUAL, Type: models.ACCOUNT_TYPE_SINGLE_ACCOUNT, Name: "投资结算（系统）", Currency: "CNY", SystemRole: InvestmentSettlementRole, Icon: 1, Color: "387F79", CreatedUnixTime: time.Now().Unix()}
		if system.AccountId <= 0 {
			return errs.ErrSystemIsBusy
		}
		if _, err = sess.Insert(system); err != nil {
			return err
		}
	}
	amount, _ := decimal.NewFromString(e.Amount)
	rate, _ := decimal.NewFromString(e.ExchangeRate)
	category, _ := strconv.ParseInt(e.Wallet.CategoryID, 10, 64)
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return err
	}
	_, offset := time.Unix(e.OccurredAt, 0).In(loc).Zone()
	kind := models.TRANSACTION_DB_TYPE_EXPENSE
	if e.Type == investments.Income {
		kind = models.TRANSACTION_DB_TYPE_INCOME
	}
	discountCNY := "0"
	if e.Wallet.DiscountAmount != nil && *e.Wallet.DiscountAmount != "" {
		discount, _ := decimal.NewFromString(*e.Wallet.DiscountAmount)
		discountCNY = discount.Mul(rate).Round(2).StringFixed(2)
	}
	tx := &models.Transaction{Uid: uid, BookId: e.BookID, Type: kind, AccountId: system.AccountId, Amount: amount.Mul(rate).Round(2).Shift(2).IntPart(), DiscountAmount: discountCNY, CategoryId: category, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(e.OccurredAt), TimezoneUtcOffset: int16(offset / 60), Comment: e.Note, InvestmentEventId: e.ID}
	if e.Wallet.ExcludeFromStatistics != nil {
		tx.ExcludeFromStatistics = *e.Wallet.ExcludeFromStatistics
	}
	// A missing budget flag on legacy wallet records means included, independently
	// of the income/expense flag on newer records.
	budgetExcluded := e.Wallet.ExcludeFromBudget != nil && *e.Wallet.ExcludeFromBudget
	tx.SetBudgetExclusion(&budgetExcluded)
	if e.Wallet.HideAmount != nil {
		tx.HideAmount = *e.Wallet.HideAmount
	}
	if e.Wallet.LocationName != nil {
		tx.LocationName = *e.Wallet.LocationName
	}
	oldByTag := make(map[int64]models.TransactionTagIndex, len(previous))
	for _, index := range previous {
		if _, exists := oldByTag[index.TagId]; exists {
			return investmentError("钱包流水标签关联异常，请先检查账本")
		}
		oldByTag[index.TagId] = index
	}
	newIDs := make([]int64, 0, len(tagIDs))
	for _, id := range tagIDs {
		if _, exists := oldByTag[id]; !exists {
			newIDs = append(newIDs, id)
		}
	}
	oldByPic := make(map[int64]models.TransactionPictureInfo, len(previousPictures))
	for _, info := range previousPictures {
		oldByPic[info.PictureId] = info
	}
	newPicIDs := make([]int64, 0, len(pictureIDs))
	keptPicIDs := make([]int64, 0, len(pictureIDs))
	keptPicSet := make(map[int64]bool, len(pictureIDs))
	for _, id := range pictureIDs {
		if _, exists := oldByPic[id]; exists {
			keptPicIDs = append(keptPicIDs, id)
			keptPicSet[id] = true
		} else {
			newPicIDs = append(newPicIDs, id)
		}
	}
	if err = Transactions.createTransactionInSession(c, sess, tx, newIDs, newPicIDs); err != nil {
		return err
	}
	now := time.Now().Unix()
	for _, id := range tagIDs {
		index, exists := oldByTag[id]
		if !exists {
			continue
		}
		valid, err := sess.Where("uid=? AND tag_id=? AND deleted=?", uid, id, false).Exist(&models.TransactionTag{})
		if err != nil {
			return err
		}
		if !valid {
			return errs.ErrTransactionTagNotFound
		}
		n, err := sess.ID(index.TagIndexId).Where("uid=? AND transaction_id=? AND deleted=?", uid, index.TransactionId, true).
			Cols("transaction_id", "transaction_time", "deleted", "deleted_unix_time", "updated_unix_time").
			Update(&models.TransactionTagIndex{TransactionId: tx.TransactionId, TransactionTime: tx.TransactionTime, Deleted: false, DeletedUnixTime: 0, UpdatedUnixTime: now})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInvestmentConflict
		}
	}
	for _, id := range keptPicIDs {
		prev := oldByPic[id]
		n, err := sess.ID(id).Where("uid=? AND transaction_id=? AND deleted=?", uid, prev.TransactionId, false).
			Cols("transaction_id", "updated_unix_time").
			Update(&models.TransactionPictureInfo{TransactionId: tx.TransactionId, UpdatedUnixTime: now})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInvestmentConflict
		}
	}
	for _, prev := range previousPictures {
		if keptPicSet[prev.PictureId] {
			continue
		}
		n, err := sess.ID(prev.PictureId).Where("uid=? AND transaction_id=? AND deleted=?", uid, prev.TransactionId, false).
			Cols("deleted", "deleted_unix_time").
			Update(&models.TransactionPictureInfo{Deleted: true, DeletedUnixTime: now})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInvestmentConflict
		}
	}
	if _, err = sess.Insert(&models.InvestmentTransactionLink{Uid: uid, EventId: e.ID, TransactionId: tx.TransactionId}); err != nil {
		return err
	}
	return nil
}

func validatePortfolioCurrency(currency string) bool {
	return currency == "" || currency == "CNY" || currency == "USD"
}

func validatePaymentInstruments(ids []string, selected []string) error {
	if len(ids) > 2 {
		return investmentError("最多选择两个收支币种")
	}
	seen := map[string]bool{}
	allowed := map[string]bool{}
	for _, id := range selected {
		allowed[id] = true
	}
	for _, id := range ids {
		if !allowed[id] || seen[id] {
			return investmentError("收支币种须为账户内不重复的持有币种")
		}
		seen[id] = true
	}
	return nil
}

// Transaction response metadata is resolved from the immutable wallet event,
// never from today's account currency or FX. No second original-amount copy is
// stored in the ordinary transaction table.
func (s *InvestmentService) WalletTransactionDetails(c core.Context, uid int64, eventIDs []string) (map[string]*models.WalletTransactionInfo, error) {
	result := map[string]*models.WalletTransactionInfo{}
	if len(eventIDs) == 0 {
		return result, nil
	}
	var rows []models.InvestmentEventRecord
	sess := s.UserDataDB(uid).NewSession(c)
	defer sess.Close()
	if err := sess.Where("uid=? AND voided=?", uid, false).In("id", eventIDs).Find(&rows); err != nil {
		return nil, err
	}
	var accounts []models.PortfolioAccount
	if err := sess.Where("uid=?", uid).Find(&accounts); err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, a := range accounts {
		names[a.Id] = a.Name
	}
	for _, row := range rows {
		e, err := decodeWalletEvent(row.Payload)
		if err != nil {
			return nil, err
		}
		if e.Wallet == nil {
			continue
		}
		result[row.Id] = &models.WalletTransactionInfo{AccountID: e.AccountID, AccountName: names[e.AccountID], Currency: e.Wallet.Currency, Amount: e.Amount, ExchangeRate: e.ExchangeRate, FXDate: e.Wallet.FXDate}
	}
	return result, nil
}

// Keep decoding shared with the service event schema.
func decodeWalletEvent(payload string) (InvestmentEvent, error) {
	var e InvestmentEvent
	err := json.Unmarshal([]byte(payload), &e)
	if err != nil {
		return e, fmt.Errorf("invalid wallet event: %w", err)
	}
	return e, nil
}
