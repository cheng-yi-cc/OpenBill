package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/shopspring/decimal"
	"xorm.io/xorm"
)

// RefundInput is used only for creating a new refund. RequestId remains
// reserved after deletion, so an old retry can never recreate the cash flow.
type RefundInput struct {
	ExpenseId              string `json:"expenseId"`
	AccountId              string `json:"accountId"`
	Amount                 string `json:"amount"`
	Time                   int64  `json:"time"`
	TimeZone               string `json:"timeZone"`
	Comment                string `json:"comment"`
	RequestId              string `json:"requestId"`
	ExpectedOriginalAmount string `json:"expectedOriginalAmount"`
	ExpectedRefundedAmount string `json:"expectedRefundedAmount"`
	InstallmentMode        string `json:"installmentMode"`
}

type RefundEditInput struct {
	AccountId       string `json:"accountId"`
	Amount          string `json:"amount"`
	Time            int64  `json:"time"`
	TimeZone        string `json:"timeZone"`
	Comment         string `json:"comment"`
	InstallmentMode string `json:"installmentMode"`
	Version         int64  `json:"version"`
}

type RefundExcessEditInput struct {
	CategoryId            string   `json:"categoryId"`
	BookId                string   `json:"bookId"`
	Comment               string   `json:"comment"`
	TagIds                []string `json:"tagIds"`
	ExcludeFromStatistics bool     `json:"excludeFromStatistics"`
	ExcludeFromBudget     bool     `json:"excludeFromBudget"`
	Version               int64    `json:"version"`
}

type RefundActor struct {
	User           *models.User
	ClientTimezone *time.Location
}

func (actor RefundActor) valid(uid int64) bool {
	return actor.User != nil && actor.User.Uid == uid && actor.ClientTimezone != nil
}

func guardRefundEditTime(sess *xorm.Session, actor RefundActor, tx *models.Transaction) error {
	if !actor.valid(tx.Uid) {
		return investmentError("无法验证退款操作权限")
	}
	var account models.Account
	has, err := sess.ID(tx.AccountId).Where("uid=? AND deleted=?", tx.Uid, false).Get(&account)
	if err != nil {
		return err
	}
	if !has || !actor.User.CanEditTransactionByTransactionTime(tx.TransactionTime, actor.ClientTimezone, &account, nil) {
		return investmentError("账单或退款日期受编辑范围或账户对账限制")
	}
	return nil
}

type RefundView struct {
	Id              string                   `json:"id"`
	ExpenseId       string                   `json:"expenseId"`
	TransactionId   string                   `json:"transactionId"`
	AccountId       string                   `json:"accountId"`
	Amount          string                   `json:"amount"`
	OffsetAmount    string                   `json:"offsetAmount"`
	ExcessAmount    string                   `json:"excessAmount"`
	Time            int64                    `json:"time"`
	TimeZone        string                   `json:"timeZone"`
	Comment         string                   `json:"comment"`
	InstallmentMode string                   `json:"installmentMode,omitempty"`
	Version         int64                    `json:"version"`
	Excess          *models.RefundExcessInfo `json:"excess,omitempty"`
}

type RefundSummary struct {
	ExpenseId                     string       `json:"expenseId"`
	OriginalAmount                string       `json:"originalAmount"`
	RefundedAmount                string       `json:"refundedAmount"`
	NetExpense                    string       `json:"netExpense"`
	ExcessIncome                  string       `json:"excessIncome"`
	HasActiveInstallment          bool         `json:"hasActiveInstallment,omitempty"`
	SupportsInstallmentReduce     bool         `json:"supportsInstallmentReduce,omitempty"`
	InstallmentPrincipal          string       `json:"installmentPrincipal,omitempty"`
	InstallmentUnaccruedPrincipal string       `json:"installmentUnaccruedPrincipal,omitempty"`
	Refunds                       []RefundView `json:"refunds"`
}

// RefundProjection describes a reporting view, never an additional cash flow.
// Expense projections belong to the original expense date; cash projections
// belong to the date and account where a refund actually arrived.
type RefundProjection = models.RefundProjection

// RefundProjections resolves requested transactions against all active facts,
// including refunds outside the requested transaction date range. This must
// remain separate from cash amounts used for balances and account history.
func (s *InvestmentService) RefundProjections(c core.Context, uid int64, ids []int64) (map[int64]RefundProjection, error) {
	if uid <= 0 {
		return nil, errs.ErrUserIdInvalid
	}
	result := make(map[int64]RefundProjection)
	if len(ids) == 0 {
		return result, nil
	}
	requested := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id > 0 {
			requested[id] = true
		}
	}
	if len(requested) == 0 {
		return result, nil
	}
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		expenses := map[int64]bool{}
		for start := 0; start < len(ids); start += 400 {
			end := start + 400
			if end > len(ids) {
				end = len(ids)
			}
			for _, col := range []string{"expense_id", "transaction_id"} {
				var rows []models.Refund
				if err := sess.Where("uid=? AND deleted=?", uid, false).In(col, ids[start:end]).Find(&rows); err != nil {
					return err
				}
				for _, row := range rows {
					expenses[row.ExpenseId] = true
				}
			}
		}
		for expenseID := range expenses {
			summary, err := refundSummary(sess, uid, expenseID)
			if err != nil {
				return err
			}
			if requested[expenseID] {
				result[expenseID] = RefundProjection{Kind: "expense", ExpenseId: summary.ExpenseId, NetExpense: summary.NetExpense, RefundedAmount: summary.RefundedAmount}
			}
			for _, refund := range summary.Refunds {
				cashID, err := strconv.ParseInt(refund.TransactionId, 10, 64)
				if err != nil {
					return err
				}
				if requested[cashID] {
					result[cashID] = RefundProjection{Kind: "cash", RefundId: refund.Id, ExpenseId: summary.ExpenseId, OffsetAmount: refund.OffsetAmount, ExcessAmount: refund.ExcessAmount, Excess: refund.Excess}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// applyRefundExpenseProjection adjusts only in-memory report rows. The
// original transaction amount remains untouched for account reconciliation.
func applyRefundExpenseProjection(c core.Context, uid int64, transactions []*models.Transaction) error {
	ids := make([]int64, 0, len(transactions))
	for _, tx := range transactions {
		if tx.Type == models.TRANSACTION_DB_TYPE_EXPENSE && tx.TransactionId > 0 {
			ids = append(ids, tx.TransactionId)
		}
	}
	projections, err := Investments.RefundProjections(c, uid, ids)
	if err != nil {
		return err
	}
	for _, tx := range transactions {
		view, ok := projections[tx.TransactionId]
		if !ok || view.Kind != "expense" {
			continue
		}
		net, err := decimal.NewFromString(view.NetExpense)
		if err != nil || net.IsNegative() || net.Shift(2).GreaterThan(decimal.NewFromInt(tx.Amount)) {
			return investmentError("退款净额不一致，请先核对账本")
		}
		tx.Amount = net.Shift(2).IntPart()
	}
	return nil
}

// Until a mutation updates the refund fact and its cash flow atomically, an
// ordinary transaction API must not change either side of the relationship.
// An empty slice means all transactions, matching guardInvestmentTransactions.
func guardRefundTransactions(sess *xorm.Session, uid int64, ids []int64) error {
	for _, column := range []string{"expense_id", "transaction_id"} {
		query := sess.Where("uid=? AND deleted=?", uid, false)
		if len(ids) != 0 {
			query = query.In(column, ids)
		}
		has, err := query.Exist(&models.Refund{})
		if err != nil {
			return err
		}
		if has {
			return investmentError("账单关联退款，请先处理退款记录")
		}
	}
	return nil
}

func guardRefundCashTransactions(sess *xorm.Session, uid int64, ids []int64) error {
	query := sess.Where("uid=? AND deleted=?", uid, false)
	if len(ids) > 0 {
		query = query.In("transaction_id", ids)
	}
	has, err := query.Exist(&models.Refund{})
	if err != nil {
		return err
	}
	if has {
		return investmentError("退款流水请通过退款明细修改")
	}
	return nil
}

// A change in any refund or its original expense can change another refund's
// offset/excess split. Invalidate sibling editors in the same transaction.
func bumpRefundSiblings(sess *xorm.Session, uid, expenseID int64, exceptID string) error {
	query := sess.Where("uid=? AND expense_id=? AND deleted=?", uid, expenseID, false)
	if exceptID != "" {
		query = query.And("id<>?", exceptID)
	}
	_, err := query.Incr("version", 1).Update(&models.Refund{})
	return err
}

// prepareRefundExpenseRevision verifies all linked cash facts before an
// ordinary expense revision can change them. The caller owns the transaction.
func prepareRefundExpenseRevision(sess *xorm.Session, actor RefundActor, old, next *models.Transaction) ([]int64, error) {
	if !actor.valid(old.Uid) || next.Uid != old.Uid || old.Type != models.TRANSACTION_DB_TYPE_EXPENSE ||
		next.Type != models.TRANSACTION_DB_TYPE_EXPENSE || next.Amount <= 0 ||
		next.ReimbursementReceiptId != "" || next.InvestmentEventId != "" || next.DebtDueDate != "" ||
		next.TransferFeeParentId != 0 || next.TransferFeeAmount != "" && next.TransferFeeAmount != "0" && next.TransferFeeAmount != "0.00" {
		return nil, investmentError("关联退款的账单须保留普通支出类型")
	}
	if err := guardRefundEditTime(sess, actor, old); err != nil {
		return nil, err
	}
	if err := guardRefundEditTime(sess, actor, next); err != nil {
		return nil, err
	}
	var rows []models.Refund
	if err := sess.Where("uid=? AND expense_id=? AND deleted=?", old.Uid, old.TransactionId, false).Find(&rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, investmentError("账单没有关联退款，请刷新后重试")
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		cash, err := refundCash(sess, old.Uid, &rows[i])
		if err != nil {
			return nil, err
		}
		if err := guardRefundEditTime(sess, actor, cash); err != nil {
			return nil, err
		}
		if utils.GetUnixTimeFromTransactionTime(cash.TransactionTime) < utils.GetUnixTimeFromTransactionTime(next.TransactionTime) {
			return nil, investmentError("账单日期不能晚于已关联退款")
		}
		if err := refundAccount(sess, old.Uid, next, cash.AccountId); err != nil {
			return nil, err
		}
		ids = append(ids, cash.TransactionId)
	}
	return ids, nil
}

func guardRefundTagDeletion(sess *xorm.Session, uid, tagID int64) error {
	var refunds []models.Refund
	if err := sess.Cols("excess_tag_ids").Where("uid=? AND deleted=?", uid, false).Find(&refunds); err != nil {
		return err
	}
	for _, refund := range refunds {
		if tagID == 0 && len(refund.ExcessTagIds) > 0 || tagID > 0 && slices.Contains(refund.ExcessTagIds, tagID) {
			return errs.ErrTransactionTagInUseCannotBeDeleted
		}
	}
	return nil
}

func refundAmount(amount string) (int64, error) {
	if len(amount) > 20 || !assetAmountPattern.MatchString(amount) {
		return 0, investmentError("请检查退款金额")
	}
	value, err := decimal.NewFromString(amount)
	if err != nil || !value.IsPositive() || value.Shift(2).GreaterThan(decimal.NewFromInt(models.MaximumTransactionAmount)) {
		return 0, investmentError("退款金额须大于零且在允许范围内")
	}
	return value.Shift(2).IntPart(), nil
}

func validateRefundInput(account, amount, zone, comment string, at int64) (int64, int64, int16, error) {
	accountID, err := strconv.ParseInt(account, 10, 64)
	minor, amountErr := refundAmount(amount)
	location, zoneErr := time.LoadLocation(zone)
	if err != nil || accountID <= 0 || amountErr != nil || zoneErr != nil || zone == "" || len(zone) > 64 ||
		at <= 0 || at > 253402300799 || utf8.RuneCountInString(comment) > 200 {
		return 0, 0, 0, investmentError("请检查退款金额、账户和日期")
	}
	_, offset := time.Unix(at, 0).In(location).Zone()
	return accountID, minor, int16(offset / 60), nil
}

func refundExpense(sess *xorm.Session, uid, expenseID int64) (*models.Transaction, error) {
	return refundExpenseBase(sess, uid, expenseID, false)
}

func refundExpenseAllowInstallment(sess *xorm.Session, uid, expenseID int64) (*models.Transaction, error) {
	return refundExpenseBase(sess, uid, expenseID, true)
}

// refundExpenseBase excludes flows managed by other services. Their invariants
// require an explicit integrated workflow before they can support refunds.
func refundExpenseBase(sess *xorm.Session, uid, expenseID int64, allowActiveInstallment bool) (*models.Transaction, error) {
	var expense models.Transaction
	has, err := sess.ID(expenseID).Where("uid=? AND deleted=?", uid, false).Get(&expense)
	if err != nil {
		return nil, err
	}
	if !has || expense.Type != models.TRANSACTION_DB_TYPE_EXPENSE || expense.Amount <= 0 ||
		expense.InvestmentEventId != "" ||
		expense.ReimbursementReceiptId != "" || expense.TransferFeeParentId != 0 || expense.DebtDueDate != "" {
		return nil, investmentError("仅支持普通正数支出退款")
	}
	for _, query := range []struct {
		condition string
		args      []any
		bean      any
	}{
		{"uid=? AND transaction_id=?", []any{uid, expenseID}, &models.AssetAdjustment{}},
		{"uid=? AND (principal_transaction_id=? OR interest_transaction_id=?)", []any{uid, expenseID, expenseID}, &models.DebtMovement{}},
		{"uid=? AND transaction_id=?", []any{uid, expenseID}, &models.MonetaryIncomeDay{}},
		{"uid=? AND transaction_id=? AND deleted=?", []any{uid, expenseID, false}, &models.Refund{}},
	} {
		found, e := sess.Where(query.condition, query.args...).Exist(query.bean)
		if e != nil {
			return nil, e
		}
		if found {
			return nil, investmentError("该流水由其他业务管理，不能直接退款")
		}
	}
	if !allowActiveInstallment {
		if err := guardCreditInstallmentTransaction(sess, nil, &expense, true); err != nil {
			return nil, err
		}
	}
	var plans []models.CreditInstallment
	if err := sess.Where("uid=?", uid).Find(&plans); err != nil {
		return nil, err
	}
	for _, plan := range plans {
		if plan.Data != nil {
			for _, payment := range plan.Data.Payments {
				if payment.FeeTransactionId == fmt.Sprint(expenseID) {
					return nil, investmentError("分期费用不能直接退款")
				}
			}
		}
	}
	return &expense, nil
}

func refundInstallmentState(sess *xorm.Session, uid int64, expense *models.Transaction) ([]models.CreditInstallment, []models.CreditInstallment, error) {
	var plans []models.CreditInstallment
	if err := sess.Where("uid=? AND closed=?", uid, false).Find(&plans); err != nil {
		return nil, nil, err
	}
	var card models.Account
	cardLoaded := false
	var expensePlans, monthPlans []models.CreditInstallment
	expenseAt := time.Unix(utils.GetUnixTimeFromTransactionTime(expense.TransactionTime), 0)
	for _, plan := range plans {
		if plan.ExpenseId == expense.TransactionId {
			expensePlans = append(expensePlans, plan)
			continue
		}
		if plan.StatementMonth == "" || plan.AccountId != expense.AccountId || plan.Data == nil {
			continue
		}
		if !cardLoaded {
			has, err := sess.ID(expense.AccountId).Where("uid=?", uid).Get(&card)
			if err != nil {
				return nil, nil, err
			}
			if !has {
				continue
			}
			cardLoaded = true
		}
		zone, err := time.LoadLocation(plan.Data.TimeZone)
		if err != nil {
			return nil, nil, err
		}
		month, _, _, _ := creditCycle(card, expenseAt.In(zone))
		if month == plan.StatementMonth {
			monthPlans = append(monthPlans, plan)
		}
	}
	sort.Slice(expensePlans, func(i, j int) bool { return expensePlans[i].Id < expensePlans[j].Id })
	return expensePlans, monthPlans, nil
}

func installmentUnaccruedIndices(data *models.CreditInstallmentData) (decimal.Decimal, []int) {
	accrued := decimal.Zero
	if data == nil {
		return accrued, nil
	}
	startIdx := 0
	if data.Method == "immediate_fee" && len(data.Payments) == data.Periods+1 && data.Payments[0].Principal == "0.00" {
		startIdx = 1
	}
	indices := make([]int, 0, len(data.Payments))
	for i, p := range data.Payments {
		if p.Accrued {
			amt, _ := decimal.NewFromString(p.Principal)
			accrued = accrued.Add(amt)
		} else if i >= startIdx {
			indices = append(indices, i)
		}
	}
	return accrued, indices
}

func validatePostInstallmentMode(expense *models.Transaction, accountID int64, mode string, expensePlans, monthPlans []models.CreditInstallment) (string, error) {
	if mode != "" && mode != "offset" && mode != "reduce" {
		return "", investmentError("请选择有效的分期退款处理方式")
	}
	if len(expensePlans) == 0 && len(monthPlans) == 0 {
		if mode != "" {
			return "", investmentError("当前账单无有效分期，请刷新后重试")
		}
		return "", nil
	}
	if accountID != expense.AccountId {
		if mode == "reduce" {
			return "", investmentError("仅退回原信贷账户的退款支持缩减分期本金")
		}
		return "offset", nil
	}
	if mode == "" {
		return "", investmentError("这笔账单已有分期，请选择冲抵应还或缩减分期本金")
	}
	if mode == "reduce" {
		if len(expensePlans) == 0 {
			return "", investmentError("账单分期暂不支持按单笔退款缩减本金，请选择冲抵应还")
		}
		if len(expensePlans) > 1 {
			return "", investmentError("该账单存在多笔进行中分期，请选择冲抵应还或先结束多余分期")
		}
	}
	return mode, nil
}

// syncPostInstallmentPlan replays the active single-expense installment schedule
// from OriginalPrincipal minus active "reduce" refunds back to the card.
func syncPostInstallmentPlan(sess *xorm.Session, uid int64, expense *models.Transaction, expensePlans []models.CreditInstallment) error {
	if len(expensePlans) != 1 || expensePlans[0].Data == nil {
		return nil
	}
	plan := expensePlans[0]
	var rows []models.Refund
	if err := sess.Where("uid=? AND expense_id=? AND deleted=?", uid, expense.TransactionId, false).Find(&rows); err != nil {
		return err
	}
	reduceCardRefunds := decimal.Zero
	for _, r := range rows {
		if r.InstallmentMode != "reduce" {
			continue
		}
		cash, err := refundCash(sess, uid, &r)
		if err != nil {
			return err
		}
		if cash.AccountId == expense.AccountId {
			reduceCardRefunds = reduceCardRefunds.Add(decimal.New(-cash.Amount, -2))
		}
	}
	currentPrincipal, _ := decimal.NewFromString(plan.Data.Principal)
	orig := currentPrincipal
	if plan.Data.OriginalPrincipal != "" {
		orig, _ = decimal.NewFromString(plan.Data.OriginalPrincipal)
	}
	targetPrincipal := orig.Sub(reduceCardRefunds)
	if targetPrincipal.Equal(currentPrincipal) && (plan.Data.OriginalPrincipal != "" || reduceCardRefunds.IsZero()) {
		return nil
	}
	accruedPrincipal, unaccruedIndices := installmentUnaccruedIndices(plan.Data)
	if len(unaccruedIndices) == 0 {
		return investmentError("当前分期已全部入账，不能缩减分期本金，请改选冲抵应还")
	}
	if !targetPrincipal.IsPositive() {
		return investmentError("缩减后分期本金须大于零，请先结束分期或改选冲抵应还")
	}
	unaccruedTarget := targetPrincipal.Sub(accruedPrincipal)
	if unaccruedTarget.IsNegative() {
		return investmentError("缩减金额超过分期未入账本金，请改选冲抵应还或调整退款金额")
	}
	if plan.Data.Method == "balloon" {
		for j, idx := range unaccruedIndices {
			p := plan.Data.Payments[idx]
			if j == len(unaccruedIndices)-1 {
				p.Principal = unaccruedTarget.StringFixed(2)
			} else {
				p.Principal = "0.00"
			}
			plan.Data.Payments[idx] = p
		}
	} else {
		remainder := plan.Data.Remainder
		if remainder == "" {
			remainder = "first"
		}
		parts := splitCreditAmount(unaccruedTarget, len(unaccruedIndices), remainder)
		for j, idx := range unaccruedIndices {
			p := plan.Data.Payments[idx]
			p.Principal = parts[j].StringFixed(2)
			plan.Data.Payments[idx] = p
		}
	}
	plan.Data.OriginalPrincipal = orig.StringFixed(2)
	plan.Data.Principal = targetPrincipal.StringFixed(2)
	plan.Version++
	n, err := sess.ID(plan.Id).Where("uid=? AND closed=? AND version=?", uid, false, plan.Version-1).
		Cols("data", "version").Update(&plan)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvestmentConflict
	}
	return nil
}

func refundAccount(sess *xorm.Session, uid int64, expense *models.Transaction, accountID int64) error {
	var source, target models.Account
	has, err := sess.ID(expense.AccountId).Where("uid=? AND deleted=? AND system_role=?", uid, false, "").Get(&source)
	if err != nil {
		return err
	}
	if !has || source.IsReimbursement() || source.Type != models.ACCOUNT_TYPE_SINGLE_ACCOUNT {
		return investmentError("原支出账户不可用于退款")
	}
	has, err = sess.ID(accountID).Where("uid=? AND deleted=? AND hidden=? AND system_role=?", uid, false, false, "").Get(&target)
	if err != nil {
		return err
	}
	if !has || target.IsReimbursement() || target.Type != models.ACCOUNT_TYPE_SINGLE_ACCOUNT || target.Currency != source.Currency {
		return investmentError("退款到账账户须为同币种有效资金账户")
	}
	return nil
}

func refundCash(sess *xorm.Session, uid int64, row *models.Refund) (*models.Transaction, error) {
	var cash models.Transaction
	has, err := sess.ID(row.TransactionId).Where("uid=? AND deleted=?", uid, false).Get(&cash)
	if err != nil {
		return nil, err
	}
	if !has || cash.Type != models.TRANSACTION_DB_TYPE_EXPENSE || cash.Amount >= 0 ||
		!cash.ExcludeFromStatistics || !cash.IsExcludedFromBudget() || cash.InvestmentEventId != "" ||
		cash.ReimbursementAccountId != 0 || cash.ReimbursementReceiptId != "" {
		return nil, investmentError("退款关联流水不一致，请先核对账本")
	}
	return &cash, nil
}

func refundSummary(sess *xorm.Session, uid, expenseID int64) (*RefundSummary, error) {
	var expense models.Transaction
	has, err := sess.ID(expenseID).Where("uid=? AND deleted=?", uid, false).Get(&expense)
	if err != nil {
		return nil, err
	}
	if !has || expense.Type != models.TRANSACTION_DB_TYPE_EXPENSE || expense.Amount <= 0 {
		return nil, investmentError("退款原账单不存在或已改变")
	}
	var rows []models.Refund
	if err := sess.Where("uid=? AND expense_id=? AND deleted=?", uid, expenseID, false).Find(&rows); err != nil {
		return nil, err
	}
	type pair struct {
		row  models.Refund
		cash models.Transaction
	}
	pairs := make([]pair, 0, len(rows))
	for _, row := range rows {
		cash, e := refundCash(sess, uid, &row)
		if e != nil {
			return nil, e
		}
		pairs = append(pairs, pair{row, *cash})
	}
	// Date governs the income projection. IDs break equal-second ties and
	// remain stable when another refund is revised or deleted.
	sort.Slice(pairs, func(i, j int) bool {
		ti, tj := utils.GetUnixTimeFromTransactionTime(pairs[i].cash.TransactionTime), utils.GetUnixTimeFromTransactionTime(pairs[j].cash.TransactionTime)
		if ti != tj {
			return ti < tj
		}
		return pairs[i].row.Id < pairs[j].row.Id
	})
	money := func(v int64) string { return decimal.New(v, -2).StringFixed(2) }
	out := &RefundSummary{ExpenseId: fmt.Sprint(expenseID), OriginalAmount: money(expense.Amount), Refunds: []RefundView{}}
	remaining, total, excess := expense.Amount, int64(0), int64(0)
	for _, p := range pairs {
		amount := -p.cash.Amount
		var ok bool
		total, ok = utils.AddInt64(total, amount)
		if !ok {
			return nil, errs.ErrAccountBalanceOverflow
		}
		offset := amount
		if offset > remaining {
			offset = remaining
		}
		remaining -= offset
		extra := amount - offset
		excess, ok = utils.AddInt64(excess, extra)
		if !ok {
			return nil, errs.ErrAccountBalanceOverflow
		}
		var excessInfo *models.RefundExcessInfo
		if p.row.ExcessCategoryId > 0 {
			excessInfo = &models.RefundExcessInfo{CategoryId: fmt.Sprint(p.row.ExcessCategoryId), BookId: p.row.ExcessBookId,
				Comment: p.row.ExcessComment, TagIds: utils.Int64ArrayToStringArray(p.row.ExcessTagIds),
				ExcludeFromStatistics: p.row.ExcessExcludeFromStatistics, ExcludeFromBudget: p.row.ExcessExcludeFromBudget}
		}
		out.Refunds = append(out.Refunds, RefundView{Id: p.row.Id, ExpenseId: fmt.Sprint(expenseID), TransactionId: fmt.Sprint(p.row.TransactionId),
			AccountId: fmt.Sprint(p.cash.AccountId), Amount: money(amount), OffsetAmount: money(offset), ExcessAmount: money(extra),
			Time: utils.GetUnixTimeFromTransactionTime(p.cash.TransactionTime), TimeZone: p.row.TimeZone, Comment: p.cash.Comment,
			InstallmentMode: p.row.InstallmentMode, Version: p.row.Version, Excess: excessInfo})
	}
	out.RefundedAmount, out.NetExpense, out.ExcessIncome = money(total), money(remaining), money(excess)
	expensePlans, monthPlans, err := refundInstallmentState(sess, uid, &expense)
	if err != nil {
		return nil, err
	}
	out.HasActiveInstallment = len(expensePlans) > 0 || len(monthPlans) > 0
	if len(expensePlans) > 0 {
		totalPrincipal, unaccruedPrincipal := decimal.Zero, decimal.Zero
		for _, plan := range expensePlans {
			if plan.Data == nil {
				continue
			}
			p, _ := decimal.NewFromString(plan.Data.Principal)
			accrued, unaccruedIndices := installmentUnaccruedIndices(plan.Data)
			totalPrincipal = totalPrincipal.Add(p)
			unaccruedPrincipal = unaccruedPrincipal.Add(p.Sub(accrued))
			if len(expensePlans) == 1 && len(unaccruedIndices) > 0 {
				out.SupportsInstallmentReduce = true
			}
		}
		out.InstallmentPrincipal = totalPrincipal.StringFixed(2)
		out.InstallmentUnaccruedPrincipal = decimal.Max(decimal.Zero, unaccruedPrincipal).StringFixed(2)
	}
	return out, nil
}

// Assign default display metadata only when a refund first has an excess.
// If later replay shifts the excess to another refund, the same transaction
// initializes that refund without discarding any earlier user choices.
func syncRefundExcess(sess *xorm.Session, uid, expenseID int64) (*RefundSummary, error) {
	summary, err := refundSummary(sess, uid, expenseID)
	if err != nil {
		return nil, err
	}
	var expense models.Transaction
	has, err := sess.ID(expenseID).Where("uid=? AND deleted=?", uid, false).Get(&expense)
	if err != nil || !has {
		if err != nil {
			return nil, err
		}
		return nil, investmentError("退款原账单不存在")
	}
	changed := false
	for _, view := range summary.Refunds {
		amount, err := decimal.NewFromString(view.ExcessAmount)
		if err != nil {
			return nil, err
		}
		if !amount.IsPositive() || view.Excess != nil {
			continue
		}
		category, err := assetIncomeCategory(sess, uid, expense.BookId, models.CATEGORY_TYPE_INCOME, "其他收入")
		if err != nil {
			return nil, err
		}
		n, err := sess.ID(view.Id).Where("uid=? AND deleted=? AND excess_category_id=0", uid, false).
			Cols("excess_category_id", "excess_book_id", "excess_comment", "excess_tag_ids").
			Update(&models.Refund{ExcessCategoryId: category, ExcessBookId: expense.BookId, ExcessComment: "退款差额", ExcessTagIds: []int64{}})
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, ErrInvestmentConflict
		}
		changed = true
	}
	if changed {
		return refundSummary(sess, uid, expenseID)
	}
	return summary, nil
}

func (s *InvestmentService) GetRefunds(c core.Context, uid, expenseID int64) (*RefundSummary, error) {
	if uid <= 0 || expenseID <= 0 {
		return nil, investmentError("账单无效")
	}
	var out *RefundSummary
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		var err error
		out, err = refundSummary(sess, uid, expenseID)
		return err
	})
	return out, err
}

func createRefundCash(c core.Context, sess *xorm.Session, actor RefundActor, uid int64, expense *models.Transaction, accountID, minor, at int64, offset int16, comment string) (*models.Transaction, error) {
	if at < utils.GetUnixTimeFromTransactionTime(expense.TransactionTime) {
		return nil, investmentError("退款时间不能早于原支出")
	}
	if err := refundAccount(sess, uid, expense, accountID); err != nil {
		return nil, err
	}
	tx := &models.Transaction{Uid: uid, BookId: expense.BookId, AccountId: accountID, CategoryId: expense.CategoryId,
		Type: models.TRANSACTION_DB_TYPE_EXPENSE, Amount: -minor, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(at),
		TimezoneUtcOffset: offset, Comment: comment, ExcludeFromStatistics: true, BudgetExclusionMode: 2}
	if err := guardRefundEditTime(sess, actor, tx); err != nil {
		return nil, err
	}
	if err := Transactions.createTransactionInSession(c, sess, tx, nil, nil); err != nil {
		return nil, err
	}
	return tx, nil
}

func (s *InvestmentService) CreateRefund(c core.Context, uid int64, actor RefundActor, input RefundInput) (*RefundSummary, error) {
	expenseID, parseErr := strconv.ParseInt(input.ExpenseId, 10, 64)
	accountID, minor, offset, inputErr := validateRefundInput(input.AccountId, input.Amount, input.TimeZone, input.Comment, input.Time)
	if uid <= 0 || !actor.valid(uid) || parseErr != nil || expenseID <= 0 || inputErr != nil || len(input.RequestId) < 8 || len(input.RequestId) > 64 {
		return nil, investmentError("请核对退款金额、账户和日期")
	}
	raw, _ := json.Marshal(input)
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	defer s.lock(uid)()
	var out *RefundSummary
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		var row models.Refund
		has, err := sess.Where("uid=? AND request_id=?", uid, input.RequestId).Get(&row)
		if err != nil {
			return err
		}
		if has {
			if row.RequestDigest != digest || row.Deleted {
				return ErrInvestmentConflict
			}
			out, err = refundSummary(sess, uid, row.ExpenseId)
			return err
		}
		expense, err := refundExpenseAllowInstallment(sess, uid, expenseID)
		if err != nil {
			return err
		}
		expensePlans, monthPlans, err := refundInstallmentState(sess, uid, expense)
		if err != nil {
			return err
		}
		mode, err := validatePostInstallmentMode(expense, accountID, input.InstallmentMode, expensePlans, monthPlans)
		if err != nil {
			return err
		}
		if input.ExpectedOriginalAmount != "" || input.ExpectedRefundedAmount != "" {
			if !assetAmountPattern.MatchString(input.ExpectedOriginalAmount) || !assetAmountPattern.MatchString(input.ExpectedRefundedAmount) || len(input.ExpectedOriginalAmount) > 20 || len(input.ExpectedRefundedAmount) > 20 {
				return investmentError("原账单刚有变动，请刷新确认最新金额后再退款")
			}
			original, e1 := decimal.NewFromString(input.ExpectedOriginalAmount)
			refunded, e2 := decimal.NewFromString(input.ExpectedRefundedAmount)
			current, e3 := refundSummary(sess, uid, expenseID)
			if e1 != nil || e2 != nil || e3 != nil {
				if e3 != nil {
					return e3
				}
				return investmentError("原账单刚有变动，请刷新确认最新金额后再退款")
			}
			currentOriginal, _ := decimal.NewFromString(current.OriginalAmount)
			currentRefunded, _ := decimal.NewFromString(current.RefundedAmount)
			if !original.Equal(currentOriginal) || !refunded.Equal(currentRefunded) {
				return ErrInvestmentConflict
			}
		}
		if err := guardRefundEditTime(sess, actor, expense); err != nil {
			return err
		}
		tx, err := createRefundCash(c, sess, actor, uid, expense, accountID, minor, input.Time, offset, input.Comment)
		if err != nil {
			return err
		}
		now := time.Now().Unix()
		row = models.Refund{Id: "refund-" + investmentID(), Uid: uid, ExpenseId: expenseID, TransactionId: tx.TransactionId,
			RequestId: input.RequestId, RequestDigest: digest, TimeZone: input.TimeZone, InstallmentMode: mode, Version: 1, CreatedAt: now, UpdatedAt: now}
		if _, err := sess.Insert(&row); err != nil {
			return err
		}
		if mode != "" {
			if err := syncPostInstallmentPlan(sess, uid, expense, expensePlans); err != nil {
				return err
			}
		}
		if err := bumpRefundSiblings(sess, uid, expenseID, row.Id); err != nil {
			return err
		}
		out, err = syncRefundExcess(sess, uid, expenseID)
		if err != nil {
			return err
		}
		if expense.ReimbursementAccountId > 0 {
			return refreshReimbursementBalances(sess, uid)
		}
		return nil
	})
	return out, err
}

// reverseRefundCash operates only on a previously verified system-owned refund
// transaction. It must not go through ordinary transaction mutation guards.
func reverseRefundCash(sess *xorm.Session, uid int64, cash *models.Transaction) error {
	if err := guardMonetaryFlowHistory(sess, uid, cash, nil); err != nil {
		return err
	}
	var account models.Account
	has, err := sess.ID(cash.AccountId).Where("uid=? AND deleted=? AND system_role=?", uid, false, "").Get(&account)
	if err != nil {
		return err
	}
	if !has || account.IsReimbursement() || account.Type != models.ACCOUNT_TYPE_SINGLE_ACCOUNT {
		return investmentError("退款账户不可用")
	}
	if err := InvalidateWealthSnapshots(sess, uid, 0); err != nil {
		return err
	}
	updated, err := Transactions.updateAccountBalance(sess, &account, cash.Amount)
	if err != nil {
		return err
	}
	if updated != 1 {
		return errs.ErrDatabaseOperationFailed
	}
	n, err := sess.ID(cash.TransactionId).Where("uid=? AND deleted=?", uid, false).
		Cols("deleted", "deleted_unix_time").Update(&models.Transaction{Deleted: true, DeletedUnixTime: time.Now().Unix()})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvestmentConflict
	}
	return nil
}

func (s *InvestmentService) UpdateRefund(c core.Context, uid int64, actor RefundActor, id string, input RefundEditInput) (*RefundSummary, error) {
	accountID, minor, offset, inputErr := validateRefundInput(input.AccountId, input.Amount, input.TimeZone, input.Comment, input.Time)
	if uid <= 0 || !actor.valid(uid) || id == "" || len(id) > 64 || input.Version <= 0 || inputErr != nil {
		return nil, investmentError("请检查退款内容")
	}
	defer s.lock(uid)()
	var out *RefundSummary
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		var row models.Refund
		has, err := sess.ID(id).Where("uid=? AND deleted=?", uid, false).Get(&row)
		if err != nil {
			return err
		}
		if !has || row.Version != input.Version {
			return ErrInvestmentConflict
		}
		expense, err := refundExpenseAllowInstallment(sess, uid, row.ExpenseId)
		if err != nil {
			return err
		}
		expensePlans, monthPlans, err := refundInstallmentState(sess, uid, expense)
		if err != nil {
			return err
		}
		mode := ""
		if len(expensePlans) > 0 || len(monthPlans) > 0 {
			if row.InstallmentMode == "" {
				return investmentError("这笔账单已有分期，请先结束分期")
			}
			desiredMode := input.InstallmentMode
			if desiredMode == "" && accountID == expense.AccountId {
				desiredMode = row.InstallmentMode
			}
			mode, err = validatePostInstallmentMode(expense, accountID, desiredMode, expensePlans, monthPlans)
			if err != nil {
				return err
			}
		} else if input.InstallmentMode != "" && input.InstallmentMode != row.InstallmentMode {
			return investmentError("当前账单无有效分期，请刷新后重试")
		}
		cash, err := refundCash(sess, uid, &row)
		if err != nil {
			return err
		}
		if err := guardRefundEditTime(sess, actor, expense); err != nil {
			return err
		}
		if err := guardRefundEditTime(sess, actor, cash); err != nil {
			return err
		}
		if err = reverseRefundCash(sess, uid, cash); err != nil {
			return err
		}
		newCash, err := createRefundCash(c, sess, actor, uid, expense, accountID, minor, input.Time, offset, input.Comment)
		if err != nil {
			return err
		}
		n, err := sess.ID(id).Where("uid=? AND deleted=? AND version=?", uid, false, input.Version).
			Cols("transaction_id", "time_zone", "installment_mode", "version", "updated_at").
			Update(&models.Refund{TransactionId: newCash.TransactionId, TimeZone: input.TimeZone, InstallmentMode: mode, Version: input.Version + 1, UpdatedAt: time.Now().Unix()})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInvestmentConflict
		}
		if len(expensePlans) > 0 {
			if err := syncPostInstallmentPlan(sess, uid, expense, expensePlans); err != nil {
				return err
			}
		}
		if err := bumpRefundSiblings(sess, uid, row.ExpenseId, row.Id); err != nil {
			return err
		}
		out, err = syncRefundExcess(sess, uid, row.ExpenseId)
		if err != nil {
			return err
		}
		if expense.ReimbursementAccountId > 0 {
			return refreshReimbursementBalances(sess, uid)
		}
		return nil
	})
	return out, err
}

// UpdateRefundExcess edits only the reporting metadata of an existing excess;
// it never creates an additional account transaction or changes the balance.
func (s *InvestmentService) UpdateRefundExcess(c core.Context, uid int64, actor RefundActor, id string, input RefundExcessEditInput) (*RefundSummary, error) {
	categoryID, parseErr := strconv.ParseInt(input.CategoryId, 10, 64)
	if uid <= 0 || !actor.valid(uid) || id == "" || len(id) > 64 || parseErr != nil || categoryID <= 0 || input.Version <= 0 || len(input.BookId) == 0 || len(input.BookId) > 64 || utf8.RuneCountInString(input.Comment) > 200 || len(input.TagIds) > models.MaximumTagsCountOfTransaction {
		return nil, investmentError("请检查退款差额分类、账本和标签")
	}
	tagIDs := make([]int64, 0, len(input.TagIds))
	seen := map[int64]bool{}
	for _, text := range input.TagIds {
		tagID, err := strconv.ParseInt(text, 10, 64)
		if err != nil || tagID <= 0 || seen[tagID] {
			return nil, investmentError("退款差额标签无效")
		}
		seen[tagID] = true
		tagIDs = append(tagIDs, tagID)
	}
	defer s.lock(uid)()
	var out *RefundSummary
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		var row models.Refund
		has, err := sess.ID(id).Where("uid=? AND deleted=?", uid, false).Get(&row)
		if err != nil {
			return err
		}
		if !has || row.Version != input.Version {
			return ErrInvestmentConflict
		}
		expense, err := refundExpenseAllowInstallment(sess, uid, row.ExpenseId)
		if err != nil {
			return err
		}
		cash, err := refundCash(sess, uid, &row)
		if err != nil {
			return err
		}
		if err := guardRefundEditTime(sess, actor, expense); err != nil {
			return err
		}
		if err := guardRefundEditTime(sess, actor, cash); err != nil {
			return err
		}
		summary, err := refundSummary(sess, uid, row.ExpenseId)
		if err != nil {
			return err
		}
		hasExcess := false
		for _, view := range summary.Refunds {
			if view.Id == id {
				amount, parseErr := decimal.NewFromString(view.ExcessAmount)
				if parseErr != nil {
					return parseErr
				}
				hasExcess = amount.IsPositive()
				break
			}
		}
		if !hasExcess {
			return investmentError("退款差额已变化，请刷新后重试")
		}
		book, err := Books.ResolveInSession(sess, uid, input.BookId, input.BookId == row.ExcessBookId)
		if err != nil {
			return err
		}
		var category models.TransactionCategory
		has, err = sess.ID(categoryID).Where("uid=? AND deleted=? AND type=? AND parent_category_id>0", uid, false, models.CATEGORY_TYPE_INCOME).Get(&category)
		if err != nil {
			return err
		}
		unchangedCategory := categoryID == row.ExcessCategoryId && book == row.ExcessBookId
		if !has || category.Hidden && !unchangedCategory || len(category.BookIds) > 0 && !slices.Contains(category.BookIds, book) && !unchangedCategory {
			return investmentError("请选择该账本可用的收入分类")
		}
		var parent models.TransactionCategory
		has, err = sess.ID(category.ParentCategoryId).Where("uid=? AND deleted=? AND type=?", uid, false, models.CATEGORY_TYPE_INCOME).Get(&parent)
		if err != nil {
			return err
		}
		if !has || parent.Hidden && !unchangedCategory || len(parent.BookIds) > 0 && !slices.Contains(parent.BookIds, book) && !unchangedCategory {
			return investmentError("请选择该账本可用的收入分类")
		}
		if len(tagIDs) > 0 {
			var selectedTags []models.TransactionTag
			err := sess.Where("uid=? AND deleted=?", uid, false).In("tag_id", tagIDs).Find(&selectedTags)
			if err != nil {
				return err
			}
			if len(selectedTags) != len(tagIDs) {
				return investmentError("请选择有效标签")
			}
			for _, tag := range selectedTags {
				if tag.Hidden && !slices.Contains(row.ExcessTagIds, tag.TagId) {
					return investmentError("请选择有效标签")
				}
			}
		}
		n, err := sess.ID(id).Where("uid=? AND deleted=? AND version=?", uid, false, input.Version).
			Cols("excess_category_id", "excess_book_id", "excess_comment", "excess_tag_ids", "excess_exclude_from_statistics", "excess_exclude_from_budget", "version", "updated_at").
			Update(&models.Refund{ExcessCategoryId: categoryID, ExcessBookId: book, ExcessComment: input.Comment, ExcessTagIds: tagIDs,
				ExcessExcludeFromStatistics: input.ExcludeFromStatistics, ExcessExcludeFromBudget: input.ExcludeFromBudget, Version: input.Version + 1, UpdatedAt: time.Now().Unix()})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInvestmentConflict
		}
		out, err = refundSummary(sess, uid, row.ExpenseId)
		return err
	})
	return out, err
}

func (s *InvestmentService) DeleteRefund(c core.Context, uid int64, actor RefundActor, id string, version int64) (*RefundSummary, error) {
	if uid <= 0 || !actor.valid(uid) || id == "" || len(id) > 64 || version <= 0 {
		return nil, investmentError("退款记录无效")
	}
	defer s.lock(uid)()
	var out *RefundSummary
	err := s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		var row models.Refund
		has, err := sess.ID(id).Where("uid=? AND deleted=?", uid, false).Get(&row)
		if err != nil {
			return err
		}
		if !has || row.Version != version {
			return ErrInvestmentConflict
		}
		cash, err := refundCash(sess, uid, &row)
		if err != nil {
			return err
		}
		expense, err := refundExpenseAllowInstallment(sess, uid, row.ExpenseId)
		if err != nil {
			return err
		}
		expensePlans, monthPlans, err := refundInstallmentState(sess, uid, expense)
		if err != nil {
			return err
		}
		if (len(expensePlans) > 0 || len(monthPlans) > 0) && row.InstallmentMode == "" {
			return investmentError("这笔账单已有分期，请先结束分期")
		}
		if err := guardRefundEditTime(sess, actor, expense); err != nil {
			return err
		}
		if err := guardRefundEditTime(sess, actor, cash); err != nil {
			return err
		}
		if err = reverseRefundCash(sess, uid, cash); err != nil {
			return err
		}
		n, err := sess.ID(id).Where("uid=? AND deleted=? AND version=?", uid, false, version).
			Cols("deleted", "version", "updated_at").Update(&models.Refund{Deleted: true, Version: version + 1, UpdatedAt: time.Now().Unix()})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInvestmentConflict
		}
		if len(expensePlans) > 0 {
			if err := syncPostInstallmentPlan(sess, uid, expense, expensePlans); err != nil {
				return err
			}
		}
		if err := bumpRefundSiblings(sess, uid, row.ExpenseId, ""); err != nil {
			return err
		}
		out, err = syncRefundExcess(sess, uid, row.ExpenseId)
		if err != nil {
			return err
		}
		if expense.ReimbursementAccountId > 0 {
			return refreshReimbursementBalances(sess, uid)
		}
		return nil
	})
	return out, err
}

// DeleteExpenseAndRefunds removes a linked expense as one operation. Refund
// request identities remain reserved, and each actual cash movement is
// reversed against its own account before the original expense is removed.
func (s *InvestmentService) DeleteExpenseAndRefunds(c core.Context, uid int64, actor RefundActor, expenseID int64) error {
	if uid <= 0 || expenseID <= 0 || !actor.valid(uid) {
		return investmentError("账单或操作权限无效")
	}
	defer s.lock(uid)()
	return s.UserDataDB(uid).DoTransaction(c, func(sess *xorm.Session) error {
		expense, err := refundExpense(sess, uid, expenseID)
		if err != nil {
			return err
		}
		if err := guardRefundEditTime(sess, actor, expense); err != nil {
			return err
		}
		var rows []models.Refund
		if err := sess.Where("uid=? AND expense_id=? AND deleted=?", uid, expenseID, false).Find(&rows); err != nil {
			return err
		}
		if len(rows) == 0 {
			return investmentError("账单没有关联退款，请刷新后重试")
		}
		cashFlows := make([]*models.Transaction, 0, len(rows))
		for i := range rows {
			cash, err := refundCash(sess, uid, &rows[i])
			if err != nil {
				return err
			}
			if err := guardRefundEditTime(sess, actor, cash); err != nil {
				return err
			}
			cashFlows = append(cashFlows, cash)
		}
		for i := range rows {
			if err := reverseRefundCash(sess, uid, cashFlows[i]); err != nil {
				return err
			}
			n, err := sess.ID(rows[i].Id).Where("uid=? AND deleted=? AND version=?", uid, false, rows[i].Version).
				Cols("deleted", "version", "updated_at").Update(&models.Refund{Deleted: true, Version: rows[i].Version + 1, UpdatedAt: time.Now().Unix()})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrInvestmentConflict
			}
		}
		return Transactions.deleteTransactionInSession(c, sess, uid, expenseID)
	})
}
