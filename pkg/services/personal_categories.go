package services

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/uuid"
	"xorm.io/xorm"
)

type personalCategoryPreset struct {
	name     string
	typeID   models.TransactionCategoryType
	icon     int64
	children []string
}

// Every primary category has a selectable leaf, preserving the existing
// transaction/category model and its primary/secondary statistics.
var personalCategoryPresets = []personalCategoryPreset{
	{"其他", models.CATEGORY_TYPE_EXPENSE, 1000, nil},
	{"购物消费", models.CATEGORY_TYPE_EXPENSE, 100, []string{"日常家居", "个护美妆", "手机数码", "虚拟充值", "生活电器", "配饰腕表", "母婴玩具", "服饰运动", "宠物用品", "办公用品", "装修装饰"}},
	{"食品餐饮", models.CATEGORY_TYPE_EXPENSE, 1, []string{"早餐", "午餐", "晚餐", "饮料酒水", "休闲零食", "生鲜食品", "请客吃饭", "粮油调味"}},
	{"出行交通", models.CATEGORY_TYPE_EXPENSE, 300, []string{"打车", "公共交通", "停车费", "加油", "火车", "飞机", "保养修车"}},
	{"休闲娱乐", models.CATEGORY_TYPE_EXPENSE, 550, []string{"旅游度假", "电影唱歌", "运动健身", "足浴按摩", "棋牌桌游", "酒吧", "演出"}},
	{"居家生活", models.CATEGORY_TYPE_EXPENSE, 200, []string{"话费宽带", "电费", "水费", "燃气费", "物业费", "房租还贷", "车位费", "家政清洁"}},
	{"文化教育", models.CATEGORY_TYPE_EXPENSE, 600, []string{"学费", "书报杂志", "培训考试"}},
	{"送礼人情", models.CATEGORY_TYPE_EXPENSE, 700, []string{"孝敬长辈", "礼物", "借出", "红包", "打赏"}},
	{"健康医疗", models.CATEGORY_TYPE_EXPENSE, 800, []string{"滋补保健", "医院", "买药"}},
	{"创业", models.CATEGORY_TYPE_EXPENSE, 2000, nil},
	{"恋爱", models.CATEGORY_TYPE_EXPENSE, 500, nil},
	{"工资薪酬", models.CATEGORY_TYPE_INCOME, 2000, []string{"工资", "奖金"}},
	{"兼职副业", models.CATEGORY_TYPE_INCOME, 2010, nil},
	{"创业收入", models.CATEGORY_TYPE_INCOME, 2000, nil},
	{"投资收益", models.CATEGORY_TYPE_INCOME, 2100, nil},
	{"礼金红包", models.CATEGORY_TYPE_INCOME, 700, nil},
	{"其他收入", models.CATEGORY_TYPE_INCOME, 3010, nil},
	{"资金往来", models.CATEGORY_TYPE_TRANSFER, 4000, []string{"借入", "借出", "还款", "收回"}},
}

var legacyAutoCategoryNames = map[string]bool{
	"余额校准":      true,
	"分期服务费":     true,
	"借入（外部账户）": true,
	"借出（外部账户）": true,
	"还款（外部账户）": true,
	"收款（外部账户）": true,
	"借款利息":      true,
	"借出利息":      true,
	"收款利息":      true,
	"借款利息优惠":    true,
	"借出利息优惠":    true,
	"收款利息优惠":    true,
	"报销到账":      true,
}

var legacyDeprecatedSubcategoriesByParent = map[string]map[string]bool{
	"购物消费": {"服饰鞋包": true, "数码产品": true, "日用百货": true},
	"食品餐饮": {"零食饮料": true},
	"出行交通": {"自驾": true, "火车机票": true},
	"休闲娱乐": {"影音游戏": true, "旅行": true},
	"居家生活": {"房租": true, "燃气": true, "通讯网络": true},
	"文化教育": {"书籍": true, "课程培训": true, "学习用品": true},
	"送礼人情": {"红包礼金": true, "聚会请客": true},
	"健康医疗": {"看病": true, "药品": true, "体检": true},
}

// EnsurePersonalCategories initializes a genuinely empty personal category list
// and converges pre-upgrade preset and auto-created categories once.
// Deleted rows count too, so intentionally removed categories never reappear.
func (s *TransactionCategoryService) EnsurePersonalCategories(c core.Context, uid int64) error {
	if uid <= 0 {
		return errs.ErrUserIdInvalid
	}
	return s.UserDataDB(uid).DoTransaction(c, func(session *xorm.Session) error {
		count, err := session.Where("uid=?", uid).Count(&models.TransactionCategory{})
		if err != nil {
			return err
		}
		if count > 0 {
			return s.reconcilePersonalCategories(session, uid)
		}
		now := time.Now().Unix()
		orders := map[models.TransactionCategoryType]int32{}
		for _, preset := range personalCategoryPresets {
			orders[preset.typeID]++
			parent := &models.TransactionCategory{CategoryId: s.GenerateUuid(uuid.UUID_TYPE_CATEGORY), Uid: uid,
				Name: preset.name, Type: preset.typeID, Icon: preset.icon, Color: "12786f", BookIds: []string{},
				DisplayOrder: orders[preset.typeID], CreatedUnixTime: now, UpdatedUnixTime: now}
			if parent.CategoryId <= 0 {
				return errs.ErrSystemIsBusy
			}
			if _, err = session.Insert(parent); err != nil {
				return err
			}
			for index, name := range append([]string{preset.name}, preset.children...) {
				child := &models.TransactionCategory{CategoryId: s.GenerateUuid(uuid.UUID_TYPE_CATEGORY), Uid: uid,
					ParentCategoryId: parent.CategoryId, Name: name, Type: preset.typeID, Icon: preset.icon,
					Color: parent.Color, BookIds: []string{}, DisplayOrder: int32(index + 1), CreatedUnixTime: now, UpdatedUnixTime: now}
				if child.CategoryId <= 0 {
					return errs.ErrSystemIsBusy
				}
				if _, err = session.Insert(child); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *TransactionCategoryService) reconcilePersonalCategories(session *xorm.Session, uid int64) error {
	var allRows []*models.TransactionCategory
	if err := session.Where("uid=?", uid).
		Asc("type").Asc("parent_category_id").Asc("display_order").Asc("category_id").Find(&allRows); err != nil {
		return err
	}
	legacyParents, legacyRows := findLegacyAutoCategories(allRows)
	expenseParents := activePrimaryCategories(allRows, models.CATEGORY_TYPE_EXPENSE)
	hasPresetRoots := findCategoryByName(expenseParents, "其他") != nil && findCategoryByName(expenseParents, "购物消费") != nil

	needsPresetAlign := false
	if hasPresetRoots {
		seenChildrenByParent := buildSeenChildNamesMap(allRows)
		activeChildrenByParent := buildActiveChildrenMap(allRows)
		needsPresetAlign = shouldAlignExpensePresets(expenseParents, seenChildrenByParent, activeChildrenByParent)
	}
	if len(legacyRows) == 0 && !needsPresetAlign {
		return nil
	}

	now := time.Now().Unix()
	refMap := make(map[int64]int64)
	deletedIDs := make([]int64, 0, len(legacyRows))

	if len(legacyRows) > 0 {
		for _, row := range legacyRows {
			deletedIDs = append(deletedIDs, row.CategoryId)
			isLeafOrSoloParent := row.ParentCategoryId != models.LevelOneTransactionCategoryParentId ||
				!legacyParents[row.CategoryId] ||
				len(findActiveChildrenOf(allRows, row.CategoryId)) == 0
			if isLeafOrSoloParent || row.ParentCategoryId == models.LevelOneTransactionCategoryParentId {
				targetID, err := s.ensureMigrationTargetLeaf(session, uid, row.Type, &allRows, legacyParents, now)
				if err != nil {
					return err
				}
				if row.CategoryId != targetID {
					refMap[row.CategoryId] = targetID
				}
			}
		}
	}

	if needsPresetAlign {
		if err := s.alignExpensePresets(session, uid, &allRows, refMap, &deletedIDs, now); err != nil {
			return err
		}
	}

	if err := remapCategoryReferences(session, uid, refMap, deletedIDs, now); err != nil {
		return err
	}
	return softDeleteCategories(session, uid, deletedIDs, now)
}

func findLegacyAutoCategories(allRows []*models.TransactionCategory) (map[int64]bool, []*models.TransactionCategory) {
	legacyParents := make(map[int64]bool)
	for _, row := range allRows {
		if row.Deleted {
			continue
		}
		if row.ParentCategoryId == models.LevelOneTransactionCategoryParentId && row.DisplayOrder == 0 && legacyAutoCategoryNames[row.Name] {
			legacyParents[row.CategoryId] = true
		}
	}
	var legacyRows []*models.TransactionCategory
	for _, row := range allRows {
		if row.Deleted {
			continue
		}
		if legacyParents[row.CategoryId] || legacyParents[row.ParentCategoryId] ||
			(row.ParentCategoryId != models.LevelOneTransactionCategoryParentId && row.DisplayOrder == 0 && legacyAutoCategoryNames[row.Name]) {
			legacyRows = append(legacyRows, row)
		}
	}
	return legacyParents, legacyRows
}

func activePrimaryCategories(allRows []*models.TransactionCategory, kind models.TransactionCategoryType) []*models.TransactionCategory {
	var result []*models.TransactionCategory
	for _, row := range allRows {
		if !row.Deleted && row.Type == kind && row.ParentCategoryId == models.LevelOneTransactionCategoryParentId {
			result = append(result, row)
		}
	}
	sortCategoriesByOrder(result)
	return result
}

func findActiveChildrenOf(allRows []*models.TransactionCategory, parentID int64) []*models.TransactionCategory {
	var result []*models.TransactionCategory
	for _, row := range allRows {
		if !row.Deleted && row.ParentCategoryId == parentID {
			result = append(result, row)
		}
	}
	sortCategoriesByOrder(result)
	return result
}

func sortCategoriesByOrder(items []*models.TransactionCategory) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].DisplayOrder != items[j].DisplayOrder {
			return items[i].DisplayOrder < items[j].DisplayOrder
		}
		return items[i].CategoryId < items[j].CategoryId
	})
}

func findCategoryByName(items []*models.TransactionCategory, name string) *models.TransactionCategory {
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	return nil
}

func buildSeenChildNamesMap(allRows []*models.TransactionCategory) map[int64]map[string]bool {
	result := make(map[int64]map[string]bool)
	for _, row := range allRows {
		if row.ParentCategoryId == models.LevelOneTransactionCategoryParentId {
			continue
		}
		set := result[row.ParentCategoryId]
		if set == nil {
			set = make(map[string]bool)
			result[row.ParentCategoryId] = set
		}
		set[row.Name] = true
	}
	return result
}

func buildActiveChildrenMap(allRows []*models.TransactionCategory) map[int64][]*models.TransactionCategory {
	result := make(map[int64][]*models.TransactionCategory)
	for _, row := range allRows {
		if row.Deleted || row.ParentCategoryId == models.LevelOneTransactionCategoryParentId {
			continue
		}
		result[row.ParentCategoryId] = append(result[row.ParentCategoryId], row)
	}
	for _, list := range result {
		sortCategoriesByOrder(list)
	}
	return result
}

func shouldAlignExpensePresets(expenseParents []*models.TransactionCategory, seenByParent map[int64]map[string]bool, activeByParent map[int64][]*models.TransactionCategory) bool {
	byName := make(map[string]*models.TransactionCategory, len(expenseParents))
	for _, parent := range expenseParents {
		if _, exists := byName[parent.Name]; !exists {
			byName[parent.Name] = parent
		}
	}
	for _, preset := range personalCategoryPresets {
		if preset.typeID != models.CATEGORY_TYPE_EXPENSE || len(preset.children) == 0 {
			continue
		}
		parent := byName[preset.name]
		if parent == nil {
			continue
		}
		deprecated := legacyDeprecatedSubcategoriesByParent[preset.name]
		for _, child := range activeByParent[parent.CategoryId] {
			if deprecated[child.Name] && !child.Hidden && len(child.BookIds) == 0 {
				return true
			}
		}
		seen := seenByParent[parent.CategoryId]
		for _, childName := range preset.children {
			if !seen[childName] {
				return true
			}
		}
	}
	return false
}

func (s *TransactionCategoryService) ensureMigrationTargetLeaf(session *xorm.Session, uid int64, kind models.TransactionCategoryType, allRows *[]*models.TransactionCategory, legacyParents map[int64]bool, now int64) (int64, error) {
	preferredParentName := "其他"
	if kind == models.CATEGORY_TYPE_INCOME {
		preferredParentName = "其他收入"
	} else if kind == models.CATEGORY_TYPE_TRANSFER {
		preferredParentName = "资金往来"
	}
	var parentOrder []*models.TransactionCategory
	var maxParentOrder int32
	for _, row := range *allRows {
		if row.Deleted || row.Type != kind || row.ParentCategoryId != models.LevelOneTransactionCategoryParentId {
			continue
		}
		if row.DisplayOrder > maxParentOrder {
			maxParentOrder = row.DisplayOrder
		}
		if legacyParents[row.CategoryId] || (row.DisplayOrder == 0 && legacyAutoCategoryNames[row.Name]) {
			continue
		}
		parentOrder = append(parentOrder, row)
	}
	sortCategoriesByOrder(parentOrder)

	if preferredParent := findCategoryByName(parentOrder, preferredParentName); preferredParent != nil {
		children := findActiveChildrenOf(*allRows, preferredParent.CategoryId)
		if leaf := findCategoryByName(children, preferredParent.Name); leaf != nil {
			return leaf.CategoryId, nil
		}
		if len(children) > 0 {
			return children[0].CategoryId, nil
		}
		leaf, err := s.insertSubCategory(session, uid, preferredParent, preferredParent.Name, 1, now)
		if err != nil {
			return 0, err
		}
		*allRows = append(*allRows, leaf)
		return leaf.CategoryId, nil
	}

	for _, parent := range parentOrder {
		if children := findActiveChildrenOf(*allRows, parent.CategoryId); len(children) > 0 {
			return children[0].CategoryId, nil
		}
	}

	order := maxParentOrder + 1
	if order < 1 {
		order = 1
	}
	parent := &models.TransactionCategory{
		CategoryId:      s.GenerateUuid(uuid.UUID_TYPE_CATEGORY),
		Uid:             uid,
		Name:            preferredParentName,
		Type:            kind,
		Icon:            assetCategoryIcon(preferredParentName),
		Color:           "12786f",
		BookIds:         []string{},
		DisplayOrder:    order,
		CreatedUnixTime: now,
		UpdatedUnixTime: now,
	}
	if parent.CategoryId <= 0 {
		return 0, errs.ErrSystemIsBusy
	}
	if _, err := session.Insert(parent); err != nil {
		return 0, err
	}
	leaf, err := s.insertSubCategory(session, uid, parent, preferredParentName, 1, now)
	if err != nil {
		return 0, err
	}
	*allRows = append(*allRows, parent, leaf)
	return leaf.CategoryId, nil
}

func (s *TransactionCategoryService) insertSubCategory(session *xorm.Session, uid int64, parent *models.TransactionCategory, name string, order int32, now int64) (*models.TransactionCategory, error) {
	child := &models.TransactionCategory{
		CategoryId:       s.GenerateUuid(uuid.UUID_TYPE_CATEGORY),
		Uid:              uid,
		ParentCategoryId: parent.CategoryId,
		Name:             name,
		Type:             parent.Type,
		Icon:             parent.Icon,
		Color:            parent.Color,
		BookIds:          slices.Clone(parent.BookIds),
		DisplayOrder:     order,
		CreatedUnixTime:  now,
		UpdatedUnixTime:  now,
	}
	if child.BookIds == nil {
		child.BookIds = []string{}
	}
	if child.CategoryId <= 0 {
		return nil, errs.ErrSystemIsBusy
	}
	if _, err := session.Insert(child); err != nil {
		return nil, err
	}
	return child, nil
}

func (s *TransactionCategoryService) alignExpensePresets(session *xorm.Session, uid int64, allRows *[]*models.TransactionCategory, refMap map[int64]int64, deletedIDs *[]int64, now int64) error {
	expenseParents := activePrimaryCategories(*allRows, models.CATEGORY_TYPE_EXPENSE)
	orderedParents := make([]*models.TransactionCategory, 0, len(expenseParents))
	usedParents := make(map[int64]bool, len(expenseParents))
	parentByName := make(map[string]*models.TransactionCategory)

	for _, preset := range personalCategoryPresets {
		if preset.typeID != models.CATEGORY_TYPE_EXPENSE {
			continue
		}
		if match := findCategoryByName(expenseParents, preset.name); match != nil && !usedParents[match.CategoryId] {
			usedParents[match.CategoryId] = true
			orderedParents = append(orderedParents, match)
			parentByName[preset.name] = match
		}
	}
	for _, parent := range expenseParents {
		if !usedParents[parent.CategoryId] {
			orderedParents = append(orderedParents, parent)
		}
	}
	if err := applyCategoryOrder(session, uid, orderedParents, now); err != nil {
		return err
	}

	seenByParent := buildSeenChildNamesMap(*allRows)
	for _, preset := range personalCategoryPresets {
		if preset.typeID != models.CATEGORY_TYPE_EXPENSE || len(preset.children) == 0 {
			continue
		}
		parent := parentByName[preset.name]
		if parent == nil {
			continue
		}
		activeChildren := findActiveChildrenOf(*allRows, parent.CategoryId)
		leaf := findCategoryByName(activeChildren, parent.Name)
		if leaf == nil {
			var err error
			leaf, err = s.insertSubCategory(session, uid, parent, parent.Name, 1, now)
			if err != nil {
				return err
			}
			*allRows = append(*allRows, leaf)
			activeChildren = append([]*models.TransactionCategory{leaf}, activeChildren...)
		}

		deprecatedNames := legacyDeprecatedSubcategoriesByParent[preset.name]
		var keptChildren []*models.TransactionCategory
		for _, child := range activeChildren {
			if deprecatedNames[child.Name] && !child.Hidden && len(child.BookIds) == 0 {
				*deletedIDs = append(*deletedIDs, child.CategoryId)
				if child.CategoryId != leaf.CategoryId {
					refMap[child.CategoryId] = leaf.CategoryId
				}
				continue
			}
			keptChildren = append(keptChildren, child)
		}

		childByName := make(map[string]*models.TransactionCategory, len(keptChildren))
		for _, child := range keptChildren {
			if _, exists := childByName[child.Name]; !exists {
				childByName[child.Name] = child
			}
		}

		seenNames := seenByParent[parent.CategoryId]
		orderedChildren := make([]*models.TransactionCategory, 0, 1+len(preset.children)+len(keptChildren))
		usedChildren := make(map[int64]bool, len(keptChildren))
		if existingLeaf := childByName[parent.Name]; existingLeaf != nil {
			usedChildren[existingLeaf.CategoryId] = true
			orderedChildren = append(orderedChildren, existingLeaf)
		}
		for _, childName := range preset.children {
			if existing := childByName[childName]; existing != nil && !usedChildren[existing.CategoryId] {
				usedChildren[existing.CategoryId] = true
				orderedChildren = append(orderedChildren, existing)
				continue
			}
			if seenNames[childName] {
				// User already created and soft-deleted this child earlier; do not recreate it.
				continue
			}
			created, err := s.insertSubCategory(session, uid, parent, childName, int32(len(orderedChildren)+1), now)
			if err != nil {
				return err
			}
			*allRows = append(*allRows, created)
			usedChildren[created.CategoryId] = true
			orderedChildren = append(orderedChildren, created)
		}
		for _, child := range keptChildren {
			if !usedChildren[child.CategoryId] {
				orderedChildren = append(orderedChildren, child)
			}
		}
		if err := applyCategoryOrder(session, uid, orderedChildren, now); err != nil {
			return err
		}
	}
	return nil
}

func applyCategoryOrder(session *xorm.Session, uid int64, items []*models.TransactionCategory, now int64) error {
	for index, item := range items {
		order := int32(index + 1)
		if item.DisplayOrder == order {
			continue
		}
		item.DisplayOrder = order
		item.UpdatedUnixTime = now
		if _, err := session.ID(item.CategoryId).Where("uid=?", uid).Cols("display_order", "updated_unix_time").Update(item); err != nil {
			return err
		}
	}
	return nil
}

func remapCategoryReferences(session *xorm.Session, uid int64, refMap map[int64]int64, deletedIDs []int64, now int64) error {
	if len(refMap) > 0 {
		byTarget := make(map[int64][]int64)
		for oldID, targetID := range refMap {
			if oldID != targetID {
				byTarget[targetID] = append(byTarget[targetID], oldID)
			}
		}
		for targetID, oldIDs := range byTarget {
			if _, err := session.Where("uid=?", uid).In("category_id", oldIDs).
				Cols("category_id", "updated_unix_time").Update(&models.Transaction{CategoryId: targetID, UpdatedUnixTime: now}); err != nil {
				return err
			}
			if _, err := session.Where("uid=?", uid).In("transfer_fee_category_id", oldIDs).
				Cols("transfer_fee_category_id", "updated_unix_time").Update(&models.Transaction{TransferFeeCategoryId: targetID, UpdatedUnixTime: now}); err != nil {
				return err
			}
			if _, err := session.Where("uid=?", uid).In("category_id", oldIDs).
				Cols("category_id", "updated_unix_time").Update(&models.TransactionTemplate{CategoryId: targetID, UpdatedUnixTime: now}); err != nil {
				return err
			}
			if _, err := session.Where("uid=?", uid).In("category_id", oldIDs).
				Cols("category_id").Update(&models.MonetaryIncomeBinding{CategoryId: targetID}); err != nil {
				return err
			}
			if _, err := session.Where("uid=?", uid).In("excess_category_id", oldIDs).
				Cols("excess_category_id", "updated_at").Update(&models.Refund{ExcessCategoryId: targetID, UpdatedAt: now}); err != nil {
				return err
			}
			if _, err := session.Where("uid=?", uid).In("category_id", oldIDs).
				Cols("category_id").Update(&models.StatisticsBudget{CategoryId: targetID}); err != nil {
				return err
			}
		}
	}

	if len(deletedIDs) == 0 {
		return nil
	}
	deletedSet := make(map[string]bool, len(deletedIDs))
	for _, id := range deletedIDs {
		deletedSet[strconv.FormatInt(id, 10)] = true
	}
	var books []*models.Book
	if err := session.Where("uid=?", uid).Find(&books); err != nil {
		return err
	}
	for _, book := range books {
		if len(book.HiddenCategoryIds) == 0 {
			continue
		}
		filtered := make([]string, 0, len(book.HiddenCategoryIds))
		changed := false
		for _, id := range book.HiddenCategoryIds {
			if deletedSet[id] {
				changed = true
				continue
			}
			filtered = append(filtered, id)
		}
		if changed {
			book.HiddenCategoryIds = filtered
			if _, err := session.ID(book.Id).Where("uid=?", uid).Cols("hidden_category_ids").Update(book); err != nil {
				return err
			}
		}
	}
	return nil
}

func softDeleteCategories(session *xorm.Session, uid int64, deletedIDs []int64, now int64) error {
	if len(deletedIDs) == 0 {
		return nil
	}
	update := &models.TransactionCategory{
		Deleted:         true,
		DeletedUnixTime: now,
		UpdatedUnixTime: now,
	}
	_, err := session.Where("uid=? AND deleted=?", uid, false).In("category_id", deletedIDs).
		Cols("deleted", "deleted_unix_time", "updated_unix_time").Update(update)
	if err != nil {
		return fmt.Errorf("soft delete migrated categories: %w", err)
	}
	return nil
}
