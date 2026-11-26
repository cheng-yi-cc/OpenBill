package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/datastore"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"xorm.io/xorm"
)

type BookService struct{ ServiceUsingDB }

var Books = &BookService{ServiceUsingDB{container: datastore.Container}}
var ErrBookNotFound = errs.NewNormalError(22, 1, 400, "账本不存在或不属于当前用户")
var ErrBookArchived = errs.NewNormalError(22, 2, 400, "账本已归档，请先恢复账本")
var ErrDefaultBookArchived = errs.NewNormalError(22, 3, 400, "请先将其他账本设为默认，再归档此账本")
var ErrBookInvalid = errs.NewNormalError(22, 4, 400, "账本名称或筛选条件无效")
var ErrBookCategoryScopeConflict = errs.NewNormalError(22, 5, 409, "分类显示设置已变化，请重新打开后再试")
var ErrBookInvestmentMove = errs.NewNormalError(22, 6, 409, "该账本含投资记录，暂不支持整本迁移")

// DefaultBookID is the stable migration identity, not the user's current choice.
func DefaultBookID(uid int64) string { return fmt.Sprintf("default-%d", uid) }

func (s *BookService) ensureDefaultInSession(sess *xorm.Session, uid int64) error {
	if uid <= 0 {
		return errs.ErrUserIdInvalid
	}
	book := &models.Book{}
	has, err := sess.ID(DefaultBookID(uid)).Where("uid=?", uid).Get(book)
	if err != nil || has {
		return err
	}
	hasDefault, err := sess.Where("uid=? AND is_default=?", uid, true).Exist(&models.Book{})
	if err != nil {
		return err
	}
	_, err = sess.Insert(&models.Book{Id: DefaultBookID(uid), Uid: uid, Name: "日常账本", Icon: "📒", IsDefault: !hasDefault, ShowTransfers: true, ShowInvestments: true})
	return err
}

// ResolveInSession enforces ownership and write eligibility inside the caller's transaction.
func (s *BookService) ResolveInSession(sess *xorm.Session, uid int64, id string, allowArchived bool) (string, error) {
	if uid <= 0 {
		return "", errs.ErrUserIdInvalid
	}
	if len(id) > 64 {
		return "", ErrBookInvalid
	}
	if id == "" {
		if err := s.ensureDefaultInSession(sess, uid); err != nil {
			return "", err
		}
	}
	book := &models.Book{}
	query := sess.Where("uid=?", uid)
	if id == "" {
		query = query.And("is_default=?", true)
	} else {
		query = query.And("id=?", id)
	}
	has, err := query.Get(book)
	if err != nil {
		return "", err
	}
	if !has {
		return "", ErrBookNotFound
	}
	if book.Archived && !allowArchived {
		return "", ErrBookArchived
	}
	return book.Id, nil
}

func (s *BookService) List(c core.Context, uid int64) ([]models.Book, error) {
	books := make([]models.Book, 0)
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		if err := s.ensureDefaultInSession(sess, uid); err != nil {
			return err
		}
		return sess.Where("uid=?", uid).OrderBy("display_order asc,id asc").Find(&books)
	})
	return books, err
}

func (s *BookService) Create(c core.Context, uid int64, req models.BookCreateRequest) (*models.Book, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 || utf8.RuneCountInString(req.Icon) > 32 {
		return nil, ErrBookInvalid
	}
	if req.Icon == "" {
		req.Icon = "📒"
	}
	book := &models.Book{Id: "book-" + investmentID(), Uid: uid, Name: name, Icon: req.Icon, ShowTransfers: true, ShowInvestments: true}
	if req.ShowTransfers != nil {
		book.ShowTransfers = *req.ShowTransfers
	}
	if req.ShowInvestments != nil {
		book.ShowInvestments = *req.ShowInvestments
	}
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		if err := s.ensureDefaultInSession(sess, uid); err != nil {
			return err
		}
		count, err := sess.Where("uid=?", uid).Count(&models.Book{})
		if err != nil {
			return err
		}
		book.DisplayOrder = int32(count)
		if req.HideExistingCategories {
			var categories []models.TransactionCategory
			if err := sess.Where("uid=? AND deleted=? AND parent_category_id=?", uid, false, models.LevelOneTransactionCategoryParentId).
				In("type", []models.TransactionCategoryType{models.CATEGORY_TYPE_EXPENSE, models.CATEGORY_TYPE_INCOME}).Find(&categories); err != nil {
				return err
			}
			book.HiddenCategoryIds = make([]string, 0, len(categories))
			for _, category := range categories {
				book.HiddenCategoryIds = append(book.HiddenCategoryIds, strconv.FormatInt(category.CategoryId, 10))
			}
		}
		_, err = sess.Insert(book)
		return err
	})
	return book, err
}

func (s *BookService) Modify(c core.Context, uid int64, req models.BookModifyRequest) (*models.Book, error) {
	book := &models.Book{}
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		has, err := sess.ID(req.Id).Where("uid=?", uid).Get(book)
		if err != nil {
			return err
		}
		if !has {
			return ErrBookNotFound
		}
		if req.Name != nil {
			book.Name = strings.TrimSpace(*req.Name)
			if book.Name == "" || utf8.RuneCountInString(book.Name) > 64 {
				return ErrBookInvalid
			}
		}
		if req.Icon != nil {
			book.Icon = *req.Icon
			if utf8.RuneCountInString(book.Icon) > 32 {
				return ErrBookInvalid
			}
		}
		if req.DisplayOrder != nil {
			book.DisplayOrder = *req.DisplayOrder
		}
		if req.Archived != nil {
			book.Archived = *req.Archived
		}
		if req.Hidden != nil {
			book.Hidden = *req.Hidden
		}
		if req.ShowTransfers != nil {
			book.ShowTransfers = *req.ShowTransfers
		}
		if req.ShowInvestments != nil {
			book.ShowInvestments = *req.ShowInvestments
		}
		if req.IsDefault != nil && !*req.IsDefault && book.IsDefault {
			return ErrDefaultBookArchived
		}
		if req.IsDefault != nil && *req.IsDefault {
			if book.Archived {
				return ErrBookArchived
			}
			if _, err = sess.Where("uid=? AND is_default=?", uid, true).Cols("is_default").Update(&models.Book{IsDefault: false}); err != nil {
				return err
			}
			book.IsDefault = true
		}
		if book.Archived && book.IsDefault {
			return ErrDefaultBookArchived
		}

		// Visibility has its own optimistic revision; a stale book edit must not
		// overwrite a concurrently saved category scope.
		_, err = sess.ID(book.Id).Where("uid=?", uid).Cols("name", "icon", "display_order", "archived", "hidden", "is_default", "show_transfers", "show_investments").Update(book)
		return err
	})
	return book, err
}

// SetCategoryVisibility changes only display metadata, never category facts or transactions.
func (s *BookService) SetCategoryVisibility(c core.Context, uid int64, req models.BookCategoryVisibilityRequest) (*models.Book, error) {
	if uid <= 0 || len(req.Changes) == 0 || len(req.Changes) > 1000 || req.Revision < 0 {
		return nil, ErrBookInvalid
	}
	book := &models.Book{}
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		has, err := sess.ID(req.BookId).Where("uid=?", uid).Get(book)
		if err != nil {
			return err
		}
		if !has {
			return ErrBookNotFound
		}
		if book.CategoryScopeRevision != req.Revision {
			return ErrBookCategoryScopeConflict
		}
		hidden := make(map[string]bool, len(book.HiddenCategoryIds))
		for _, id := range book.HiddenCategoryIds {
			hidden[id] = true
		}
		seen := make(map[string]bool, len(req.Changes))
		for _, change := range req.Changes {
			id, err := strconv.ParseInt(change.CategoryId, 10, 64)
			if err != nil || id <= 0 || strconv.FormatInt(id, 10) != change.CategoryId || seen[change.CategoryId] {
				return ErrBookInvalid
			}
			seen[change.CategoryId] = true
			has, err := sess.ID(id).Where("uid=? AND deleted=?", uid, false).Exist(&models.TransactionCategory{})
			if err != nil {
				return err
			}
			if !has {
				return errs.ErrTransactionCategoryNotFound
			}
			if change.Hidden {
				hidden[change.CategoryId] = true
			} else {
				delete(hidden, change.CategoryId)
			}
		}
		ids := make([]string, 0, len(hidden))
		for id := range hidden {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		book.HiddenCategoryIds = ids
		book.CategoryScopeRevision++
		updated, err := sess.ID(book.Id).Where("uid=? AND category_scope_revision=?", uid, req.Revision).
			Cols("hidden_category_ids", "category_scope_revision").Update(book)
		if err != nil {
			return err
		}
		if updated != 1 {
			return ErrBookCategoryScopeConflict
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return book, nil
}

type bookFilterContext struct {
	core.Context
	ids []string
}
type bookFilterKey struct{}

func (c *bookFilterContext) Value(key any) any {
	if _, ok := key.(bookFilterKey); ok {
		return c.ids
	}
	if c.Context == nil {
		return nil
	}
	return c.Context.Value(key)
}

// FilterContext validates every requested identity; unknown IDs are never silently ignored.
func (s *BookService) FilterContext(c core.Context, uid int64, raw string) (core.Context, error) {
	if raw == "" {
		return c, nil
	}
	if len(raw) > 6500 {
		return nil, ErrBookInvalid
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 100 {
		return nil, ErrBookInvalid
	}
	ids := make([]string, 0, len(parts))
	seen := map[string]bool{}
	sess := s.UserDataDB(uid).NewSession(c)
	defer sess.Close()
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" || len(id) > 64 {
			return nil, ErrBookInvalid
		}
		if _, err := s.ResolveInSession(sess, uid, id, true); err != nil {
			return nil, err
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if c == nil {
		c = core.NewNullContext()
	}
	return &bookFilterContext{Context: c, ids: ids}, nil
}

func appendBookFilter(c core.Context, condition string, params []any) (string, []any) {
	if c == nil {
		return condition, params
	}
	ids, _ := c.Value(bookFilterKey{}).([]string)
	if len(ids) == 0 {
		return condition, params
	}
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		params = append(params, id)
	}
	return condition + " AND book_id IN (" + strings.Join(placeholders, ",") + ")", params
}

// MoveTransactions changes classification only, including both sides of a transfer.
func (s *BookService) MoveTransactions(c core.Context, uid int64, ids []int64, bookID string, actors ...RefundActor) error {
	if len(ids) == 0 || len(ids) > 1000 {
		return ErrBookInvalid
	}
	defer Investments.lock(uid)()
	return s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		return s.moveTransactionsInSession(sess, uid, ids, bookID, false, actors...)
	})
}

// MoveAllTransactions captures the complete source inside the same transaction
// that moves it. SQL batches only limit parameter counts, not commit boundaries.
func (s *BookService) MoveAllTransactions(c core.Context, uid int64, sourceID, targetID string, actor RefundActor) (int, error) {
	if sourceID == "" || targetID == "" || sourceID == targetID || !actor.valid(uid) {
		return 0, ErrBookInvalid
	}
	defer Investments.lock(uid)()
	count := 0
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		source, err := s.ResolveInSession(sess, uid, sourceID, true)
		if err != nil {
			return err
		}
		target, err := s.ResolveInSession(sess, uid, targetID, false)
		if err != nil {
			return err
		}
		type sourceInvestment struct {
			record models.InvestmentEventRecord
			event  InvestmentEvent
		}
		var events []models.InvestmentEventRecord
		if err := sess.Where("uid=? AND voided=?", uid, false).Find(&events); err != nil {
			return err
		}
		activeEventBooks := make(map[string]string, len(events))
		sourceEvents := make([]sourceInvestment, 0)
		for _, record := range events {
			var event InvestmentEvent
			if err := json.Unmarshal([]byte(record.Payload), &event); err != nil {
				return investmentError("投资记录数据异常，无法安全迁移账本")
			}
			book := event.BookID
			if book == "" {
				book = DefaultBookID(uid)
			}
			activeEventBooks[record.Id] = book
			if book == source {
				sourceEvents = append(sourceEvents, sourceInvestment{record: record, event: event})
			}
		}
		var rows []models.Transaction
		if err := sess.Cols("transaction_id", "investment_event_id").Where("uid=? AND book_id=? AND deleted=?", uid, source, false).Find(&rows); err != nil {
			return err
		}
		linkedEvents := make(map[string]bool)
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			if row.InvestmentEventId != "" {
				eventBook, ok := activeEventBooks[row.InvestmentEventId]
				if !ok || eventBook != source {
					return ErrInvestmentLinked
				}
				linkedEvents[row.InvestmentEventId] = true
			}
			ids = append(ids, row.TransactionId)
		}
		unlinkedCount := 0
		for _, item := range sourceEvents {
			crossBook, err := sess.Where("uid=? AND investment_event_id=? AND deleted=? AND book_id<>?", uid, item.record.Id, false, source).Exist(&models.Transaction{})
			if err != nil {
				return err
			}
			if crossBook {
				return ErrInvestmentLinked
			}
			if !linkedEvents[item.record.Id] {
				unlinkedCount++
			}
		}
		if len(rows) == 0 && unlinkedCount == 0 {
			return nil
		}
		if len(ids) > 0 {
			if err := s.moveTransactionsInSession(sess, uid, ids, target, true, actor); err != nil {
				return err
			}
		}
		now := time.Now().Unix()
		for _, item := range sourceEvents {
			item.event.BookID = target
			item.event.Version = item.record.Version + 1
			payload, err := json.Marshal(item.event)
			if err != nil {
				return err
			}
			n, err := sess.Where("uid=? AND id=? AND version=? AND voided=?", uid, item.record.Id, item.record.Version, false).
				Cols("version", "payload").Update(&models.InvestmentEventRecord{Version: item.event.Version, Payload: string(payload)})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrInvestmentConflict
			}
			if _, err := sess.Insert(&models.InvestmentEventRevision{Id: investmentID(), Uid: uid, EventId: item.record.Id, Version: item.event.Version, RecordedAt: now, Payload: string(payload)}); err != nil {
				return err
			}
		}
		var plans []models.InvestmentPlan
		if err := sess.Where("uid=? AND book_id=? AND deleted=?", uid, source, false).Find(&plans); err != nil {
			return err
		}
		for _, plan := range plans {
			n, err := sess.Where("uid=? AND id=? AND deleted=? AND version=?", uid, plan.Id, false, plan.Version).
				Cols("book_id", "version").Update(&models.InvestmentPlan{BookId: target, Version: plan.Version + 1})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrInvestmentConflict
			}
		}
		var orders []models.InvestmentOrder
		if err := sess.Where("uid=? AND book_id=? AND status=?", uid, source, "pending").Find(&orders); err != nil {
			return err
		}
		for _, order := range orders {
			n, err := sess.Where("uid=? AND id=? AND status=? AND version=?", uid, order.Id, "pending", order.Version).
				Cols("book_id", "version").Update(&models.InvestmentOrder{BookId: target, Version: order.Version + 1})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrInvestmentConflict
			}
		}
		var dcaPlans []models.CryptoDCAPlan
		if err := sess.Where("uid=? AND book_id=?", uid, source).Find(&dcaPlans); err != nil {
			return err
		}
		for _, plan := range dcaPlans {
			n, err := sess.Where("uid=? AND id=? AND revision=?", uid, plan.Id, plan.Revision).
				Cols("book_id", "revision").Update(&models.CryptoDCAPlan{BookId: target, Revision: plan.Revision + 1})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrInvestmentConflict
			}
		}
		if _, err := sess.Where("uid=? AND book_id=? AND status=?", uid, source, "pending").Cols("book_id").Update(&models.CryptoDCADay{BookId: target}); err != nil {
			return err
		}
		if _, err := sess.Where("uid=? AND book_id=?", uid, source).Cols("book_id").Update(&models.MonetaryIncomeBinding{BookId: target}); err != nil {
			return err
		}
		if len(sourceEvents) > 0 || len(plans) > 0 || len(orders) > 0 || len(dcaPlans) > 0 {
			if err := InvalidateWealthSnapshots(sess, uid, 0); err != nil {
				return err
			}
		}
		count = len(ids) + unlinkedCount
		return nil
	})
	return count, err
}

func forBookMoveChunks(ids []int64, f func([]int64) error) error {
	const chunkSize = 400
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := f(ids[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *BookService) moveTransactionsInSession(sess *xorm.Session, uid int64, ids []int64, bookID string, allowInvestmentEvents bool, actors ...RefundActor) error {
	target, err := s.ResolveInSession(sess, uid, bookID, false)
	if err != nil {
		return err
	}
	allIDs := map[int64]bool{}
	refundExpenseIDs := map[int64]bool{}
	selected := map[int64]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	for _, id := range ids {
		var tx models.Transaction
		has, err := sess.ID(id).Where("uid=? AND deleted=?", uid, false).Get(&tx)
		if err != nil {
			return err
		}
		if !has {
			return errs.ErrTransactionNotFound
		}
		if tx.TransferFeeParentId > 0 && !selected[tx.TransferFeeParentId] {
			return investmentError("请选中原转账账单移动账本")
		}
		var selectedRefund models.Refund
		isRefund, err := sess.Where("uid=? AND transaction_id=? AND deleted=?", uid, id, false).Get(&selectedRefund)
		if err != nil {
			return err
		}
		if isRefund && !selected[selectedRefund.ExpenseId] {
			return investmentError("请选中退款原支出一起移动账本")
		}
		if tx.ReimbursementReceiptId != "" {
			var receipt models.ReimbursementReceipt
			has, err := sess.ID(tx.ReimbursementReceiptId).Where("uid=? AND income_id=?", uid, id).Get(&receipt)
			if err != nil {
				return err
			}
			if !has || !selected[receipt.ExpenseId] {
				return investmentError("请选中报销原支出一起移动账本")
			}
		}
		allIDs[id] = true
		if tx.RelatedId != 0 {
			allIDs[tx.RelatedId] = true
		}
		var refunds []models.Refund
		if err := sess.Where("uid=? AND expense_id=? AND deleted=?", uid, id, false).Find(&refunds); err != nil {
			return err
		}
		if len(refunds) > 0 {
			if len(actors) != 1 || !actors[0].valid(uid) {
				return investmentError("无法验证退款流水的编辑权限")
			}
			if _, err := refundSummary(sess, uid, id); err != nil {
				return err
			}
			refundExpenseIDs[id] = true
			for i := range refunds {
				cash, err := refundCash(sess, uid, &refunds[i])
				if err != nil {
					return err
				}
				allIDs[cash.TransactionId] = true
			}
		}
		if tx.ReimbursementAccountId > 0 {
			var rows []models.ReimbursementReceipt
			if err := sess.Where("uid=? AND expense_id=?", uid, id).Find(&rows); err != nil {
				return err
			}
			for _, row := range rows {
				var payment models.Transaction
				active, err := sess.ID(row.IncomeId).Where("uid=? AND deleted=?", uid, false).Get(&payment)
				if err != nil {
					return err
				}
				if !active {
					continue
				} // Deleted receipts preserve their idempotency identity.
				if payment.ReimbursementReceiptId != row.Id {
					return investmentError("报销到账关联异常，请先检查账单")
				}
				if len(actors) != 1 || !actors[0].valid(uid) {
					return investmentError("无法验证报销到账的编辑权限")
				}
				allIDs[payment.TransactionId] = true
			}
		}
	}
	paired := make([]int64, 0, len(allIDs))
	for id := range allIDs {
		paired = append(paired, id)
	}
	var fees []models.Transaction
	if err := forBookMoveChunks(paired, func(chunk []int64) error {
		var part []models.Transaction
		if err := sess.Where("uid=? AND deleted=?", uid, false).In("transfer_fee_parent_id", chunk).Find(&part); err != nil {
			return err
		}
		fees = append(fees, part...)
		return nil
	}); err != nil {
		return err
	}
	for _, fee := range fees {
		if !allIDs[fee.TransactionId] {
			paired = append(paired, fee.TransactionId)
		}
		allIDs[fee.TransactionId] = true
	}
	sort.Slice(paired, func(i, j int) bool { return paired[i] < paired[j] })
	// A derived transfer or fee may itself have unexpected refund links. Do
	// not move a partial relationship even if the stored data is unusual.
	for _, column := range []string{"expense_id", "transaction_id"} {
		if err := forBookMoveChunks(paired, func(chunk []int64) error {
			var refunds []models.Refund
			if err := sess.Where("uid=? AND deleted=?", uid, false).In(column, chunk).Find(&refunds); err != nil {
				return err
			}
			for _, row := range refunds {
				if !selected[row.ExpenseId] || !allIDs[row.ExpenseId] || !allIDs[row.TransactionId] {
					return investmentError("退款关联不完整，请选中原支出后重试")
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if len(actors) > 1 || len(actors) == 1 && !actors[0].valid(uid) {
		return investmentError("无法验证账单编辑权限")
	}
	if len(actors) == 1 {
		for _, id := range paired {
			var tx models.Transaction
			has, err := sess.ID(id).Where("uid=? AND deleted=?", uid, false).Get(&tx)
			if err != nil {
				return err
			}
			if !has {
				return errs.ErrTransactionNotFound
			}
			var account, related models.Account
			has, err = sess.ID(tx.AccountId).Where("uid=? AND deleted=?", uid, false).Get(&account)
			if err != nil {
				return err
			}
			if !has {
				return errs.ErrAccountNotFound
			}
			var relatedAccount *models.Account
			if tx.RelatedAccountId > 0 {
				has, err = sess.ID(tx.RelatedAccountId).Where("uid=? AND deleted=?", uid, false).Get(&related)
				if err != nil {
					return err
				}
				if !has {
					return errs.ErrAccountNotFound
				}
				relatedAccount = &related
			}
			if !actors[0].User.CanEditTransactionByTransactionTime(tx.TransactionTime, actors[0].ClientTimezone, &account, relatedAccount) {
				return errs.ErrCannotModifyTransactionWithThisTransactionTime
			}
		}
	}
	if !allowInvestmentEvents {
		if err := forBookMoveChunks(paired, func(chunk []int64) error {
			return guardInvestmentEventTransactions(sess, uid, chunk)
		}); err != nil {
			return err
		}
	}
	if err := forBookMoveChunks(paired, func(chunk []int64) error {
		_, err := sess.Where("uid=? AND deleted=?", uid, false).In("transaction_id", chunk).Cols("book_id", "updated_unix_time").Update(&models.Transaction{BookId: target, UpdatedUnixTime: time.Now().Unix()})
		if err != nil {
			return err
		}
		_, err = sess.Where("uid=?", uid).In("transaction_id", chunk).Cols("book_id").Update(&models.CalendarEvent{BookId: target})
		return err
	}); err != nil {
		return err
	}
	for expenseID := range refundExpenseIDs {
		if err := bumpRefundSiblings(sess, uid, expenseID, ""); err != nil {
			return err
		}
		if _, err := syncRefundExcess(sess, uid, expenseID); err != nil {
			return err
		}
	}
	return nil
}

// MigrateUser is idempotent and preserves all amounts, IDs, and settlement links.
func (s *BookService) MigrateUser(c core.Context, uid int64) error {
	return s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		if err := s.ensureDefaultInSession(sess, uid); err != nil {
			return err
		}
		id := DefaultBookID(uid)
		if _, err := sess.Where("uid=? AND (book_id=? OR book_id IS NULL)", uid, "").Cols("book_id").Update(&models.Transaction{BookId: id}); err != nil {
			return err
		}
		if _, err := sess.Where("uid=? AND (book_id=? OR book_id IS NULL)", uid, "").Cols("book_id").Update(&models.TransactionTemplate{BookId: id}); err != nil {
			return err
		}
		var events []models.InvestmentEventRecord
		if err := sess.Where("uid=?", uid).Find(&events); err != nil {
			return err
		}
		for _, event := range events {
			payload, changed, err := backfillBookPayload(event.Payload, id)
			if err != nil {
				return err
			}
			if changed {
				if _, err = sess.ID(event.Id).Where("uid=?", uid).Cols("payload").Update(&models.InvestmentEventRecord{Payload: payload}); err != nil {
					return err
				}
			}
		}
		var revisions []models.InvestmentEventRevision
		if err := sess.Where("uid=?", uid).Find(&revisions); err != nil {
			return err
		}
		for _, revision := range revisions {
			payload, changed, err := backfillBookPayload(revision.Payload, id)
			if err != nil {
				return err
			}
			if changed {
				if _, err = sess.ID(revision.Id).Where("uid=?", uid).Cols("payload").Update(&models.InvestmentEventRevision{Payload: payload}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func backfillBookPayload(payload, id string) (string, bool, error) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		return "", false, err
	}
	if item == nil {
		return "", false, ErrBookInvalid
	}
	var previous string
	if value, ok := item["bookId"]; ok {
		if err := json.Unmarshal(value, &previous); err != nil {
			return "", false, err
		}
	}
	if previous != "" {
		return payload, false, nil
	}
	item["bookId"], _ = json.Marshal(id)
	data, err := json.Marshal(item)
	return string(data), true, err
}

func (s *BookService) MigrateAll(c core.Context) error {
	for i := 0; i < s.container.UserStore.Count(); i++ {
		var users []models.User
		sess := s.container.UserStore.Get(i).NewSession(c)
		err := sess.Cols("uid").Find(&users)
		sess.Close()
		if err != nil {
			return err
		}
		for _, user := range users {
			if err := s.MigrateUser(c, user.Uid); err != nil {
				return err
			}
		}
	}
	return nil
}

// WithoutBookFilter keeps shared-account balance calculations independent of reporting scope.
func WithoutBookFilter(c core.Context) core.Context {
	if scoped, ok := c.(*bookFilterContext); ok {
		return scoped.Context
	}
	return c
}
