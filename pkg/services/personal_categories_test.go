package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestPersonalCategoriesInitializeOnceWithoutChangingLedger(t *testing.T) {
	f := newInvestmentDBFixture(t)
	balance, transactions := f.balance(), f.count(&models.Transaction{})
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	all, err := TransactionCategories.GetAllCategoriesByUid(nil, f.uid, 0, -1)
	require.NoError(t, err)
	parents := map[int64]*models.TransactionCategory{}
	var expenseNames []string
	for _, category := range all {
		if category.ParentCategoryId == 0 {
			parents[category.CategoryId] = category
			if category.Type == models.CATEGORY_TYPE_EXPENSE {
				expenseNames = append(expenseNames, category.Name)
			}
		}
	}
	require.Len(t, parents, len(personalCategoryPresets))
	require.Equal(t, []string{"其他", "购物消费", "食品餐饮", "出行交通", "休闲娱乐", "居家生活", "文化教育", "送礼人情", "健康医疗", "创业", "恋爱"}, expenseNames)
	for _, category := range all {
		if category.ParentCategoryId == 0 {
			continue
		}
		require.Contains(t, parents, category.ParentCategoryId)
		require.Equal(t, parents[category.ParentCategoryId].Type, category.Type)
		require.Empty(t, category.BookIds)
	}
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	require.Equal(t, int64(len(all)), f.count(&models.TransactionCategory{}))
	require.Equal(t, balance, f.balance())
	require.Equal(t, transactions, f.count(&models.Transaction{}))
}

func TestPersonalCategoriesPreserveCustomAndDeletedCategories(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		f := newInvestmentDBFixture(t)
		existing := &models.TransactionCategory{CategoryId: 901, Uid: f.uid, Name: "我的分类", Deleted: deleted}
		_, err := f.engine.Insert(existing)
		require.NoError(t, err)
		require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
		require.Equal(t, int64(1), f.count(&models.TransactionCategory{}))
		var got models.TransactionCategory
		_, err = f.engine.ID(existing.CategoryId).Get(&got)
		require.NoError(t, err)
		require.Equal(t, existing.Name, got.Name)
		require.Equal(t, deleted, got.Deleted)
	}
}

func TestPersonalCategoriesMigrateLegacyAutoAndAlignExpensePresetsIdempotently(t *testing.T) {
	f := newInvestmentDBFixture(t)
	now := time.Now().Unix()

	legacyExpenseSpecs := []struct {
		id   int64
		name string
	}{
		{8001, "余额校准"},
		{8003, "分期服务费"},
		{8005, "借入（外部账户）"},
		{8007, "借款利息"},
	}
	for _, spec := range legacyExpenseSpecs {
		_, err := f.engine.Insert(
			&models.TransactionCategory{CategoryId: spec.id, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: spec.name, DisplayOrder: 0, CreatedUnixTime: now, UpdatedUnixTime: now},
			&models.TransactionCategory{CategoryId: spec.id + 1, Uid: f.uid, ParentCategoryId: spec.id, Type: models.CATEGORY_TYPE_EXPENSE, Name: spec.name, DisplayOrder: 0, CreatedUnixTime: now, UpdatedUnixTime: now},
		)
		require.NoError(t, err)
	}
	_, err := f.engine.Insert(
		&models.TransactionCategory{CategoryId: 8011, Uid: f.uid, Type: models.CATEGORY_TYPE_INCOME, Name: "报销到账", DisplayOrder: 0, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8012, Uid: f.uid, ParentCategoryId: 8011, Type: models.CATEGORY_TYPE_INCOME, Name: "报销到账", DisplayOrder: 0, CreatedUnixTime: now, UpdatedUnixTime: now},
	)
	require.NoError(t, err)

	// Old preset order: 创业(1), 恋爱(2), 其他(3), 购物消费(4)... + custom(12)
	_, err = f.engine.Insert(
		&models.TransactionCategory{CategoryId: 8101, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "创业", DisplayOrder: 1, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8102, Uid: f.uid, ParentCategoryId: 8101, Type: models.CATEGORY_TYPE_EXPENSE, Name: "创业", DisplayOrder: 1, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8103, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "恋爱", DisplayOrder: 2, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8104, Uid: f.uid, ParentCategoryId: 8103, Type: models.CATEGORY_TYPE_EXPENSE, Name: "恋爱", DisplayOrder: 1, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8105, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "其他", DisplayOrder: 3, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8106, Uid: f.uid, ParentCategoryId: 8105, Type: models.CATEGORY_TYPE_EXPENSE, Name: "其他", DisplayOrder: 1, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8107, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "购物消费", DisplayOrder: 4, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8108, Uid: f.uid, ParentCategoryId: 8107, Type: models.CATEGORY_TYPE_EXPENSE, Name: "购物消费", DisplayOrder: 1, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8109, Uid: f.uid, ParentCategoryId: 8107, Type: models.CATEGORY_TYPE_EXPENSE, Name: "服饰鞋包", DisplayOrder: 2, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8110, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "自定义分类", DisplayOrder: 5, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8111, Uid: f.uid, ParentCategoryId: 8110, Type: models.CATEGORY_TYPE_EXPENSE, Name: "自定义分类", DisplayOrder: 1, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8201, Uid: f.uid, Type: models.CATEGORY_TYPE_INCOME, Name: "其他收入", DisplayOrder: 1, CreatedUnixTime: now, UpdatedUnixTime: now},
		&models.TransactionCategory{CategoryId: 8202, Uid: f.uid, ParentCategoryId: 8201, Type: models.CATEGORY_TYPE_INCOME, Name: "其他收入", DisplayOrder: 1, CreatedUnixTime: now, UpdatedUnixTime: now},
	)
	require.NoError(t, err)

	// Record real transactions using legacy auto categories and old placeholder subcategory.
	for i, catID := range []int64{8002, 8004, 8006, 8008, 8109} {
		tx := &models.Transaction{Uid: f.uid, AccountId: f.bankID, Type: models.TRANSACTION_DB_TYPE_EXPENSE, CategoryId: catID, Amount: 500, TransactionTime: utils.GetMinTransactionTimeFromUnixTime(f.at + int64(i+1))}
		require.NoError(t, Transactions.CreateTransaction(nil, tx, nil, nil))
	}
	beforeBalance := f.balance()
	beforeTxCount := f.count(&models.Transaction{})

	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))
	require.Equal(t, beforeBalance, f.balance())
	require.Equal(t, beforeTxCount, f.count(&models.Transaction{}))

	activeExpense, err := TransactionCategories.GetAllCategoriesByUid(nil, f.uid, models.CATEGORY_TYPE_EXPENSE, -1)
	require.NoError(t, err)
	var parentNames []string
	var shoppingChildNames []string
	for _, cat := range activeExpense {
		if cat.ParentCategoryId == 0 {
			parentNames = append(parentNames, cat.Name)
		} else if cat.ParentCategoryId == 8107 {
			shoppingChildNames = append(shoppingChildNames, cat.Name)
		}
	}
	require.Equal(t, []string{"其他", "购物消费", "创业", "恋爱", "自定义分类"}, parentNames)
	require.Equal(t, []string{"购物消费", "日常家居", "个护美妆", "手机数码", "虚拟充值", "生活电器", "配饰腕表", "母婴玩具", "服饰运动", "宠物用品", "办公用品", "装修装饰"}, shoppingChildNames)

	// Verify migrated transactions now point to 其他(8106) and 购物消费(8108), and assetIncomeCategory reuses 其他(8106).
	var migratedCount int64
	migratedCount, err = f.engine.Where("uid=? AND category_id=?", f.uid, 8106).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Equal(t, int64(4), migratedCount)
	var shoppingLeafCount int64
	shoppingLeafCount, err = f.engine.Where("uid=? AND category_id=?", f.uid, 8108).Count(&models.Transaction{})
	require.NoError(t, err)
	require.Equal(t, int64(1), shoppingLeafCount)

	// Second call must stay idempotent even if user later reorders 创业 to the front or deletes a child.
	_, err = f.engine.ID(8101).Cols("display_order").Update(&models.TransactionCategory{DisplayOrder: 0})
	require.NoError(t, err)
	_, err = f.engine.Where("uid=? AND parent_category_id=? AND name=?", f.uid, 8107, "宠物用品").Cols("deleted").Update(&models.TransactionCategory{Deleted: true})
	require.NoError(t, err)
	require.NoError(t, TransactionCategories.EnsurePersonalCategories(nil, f.uid))

	var shopChildrenAfter []*models.TransactionCategory
	require.NoError(t, f.engine.Where("uid=? AND deleted=? AND parent_category_id=?", f.uid, false, 8107).Find(&shopChildrenAfter))
	for _, child := range shopChildrenAfter {
		require.NotEqual(t, "宠物用品", child.Name, "soft-deleted preset child must not be recreated")
	}

	// New automatic asset operations reuse 其他 / 其他收入 without creating standalone categories.
	_, err = f.engine.Transaction(func(sess *xorm.Session) (any, error) {
		for _, op := range []string{"余额校准", "分期服务费", "借入（外部账户）", "借款利息"} {
			id, e := assetIncomeCategory(sess, f.uid, "", models.CATEGORY_TYPE_EXPENSE, op)
			require.NoError(t, e)
			require.Equal(t, int64(8106), id, fmt.Sprintf("expected %s to reuse 其他", op))
		}
		incomeID, e := assetIncomeCategory(sess, f.uid, "", models.CATEGORY_TYPE_INCOME, "报销到账")
		require.NoError(t, e)
		require.Equal(t, int64(8202), incomeID)
		return nil, nil
	})
	require.NoError(t, err)
}

