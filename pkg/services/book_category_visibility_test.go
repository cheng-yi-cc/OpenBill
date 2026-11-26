package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestBookCategoryVisibilitySnapshotRevisionAndRestore(t *testing.T) {
	f := newInvestmentDBFixture(t)
	_, err := f.engine.Insert(
		&models.TransactionCategory{CategoryId: 9301, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "支出"},
		&models.TransactionCategory{CategoryId: 9302, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, ParentCategoryId: 9301, Name: "子分类"},
		&models.TransactionCategory{CategoryId: 9303, Uid: f.uid, Type: models.CATEGORY_TYPE_INCOME, Name: "收入"},
		&models.TransactionCategory{CategoryId: 9304, Uid: f.uid + 1, Type: models.CATEGORY_TYPE_EXPENSE, Name: "其他用户"},
	)
	require.NoError(t, err)
	balance := f.balance()
	book, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "独立分类", HideExistingCategories: true})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"9301", "9303"}, book.HiddenCategoryIds)
	require.NotContains(t, book.HiddenCategoryIds, "9302", "child choices remain independent")
	other, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "普通账本"})
	require.NoError(t, err)
	require.Empty(t, other.HiddenCategoryIds)
	_, err = f.engine.Insert(&models.TransactionCategory{CategoryId: 9305, Uid: f.uid, Type: models.CATEGORY_TYPE_EXPENSE, Name: "之后新增"})
	require.NoError(t, err)
	changed, err := Books.SetCategoryVisibility(nil, f.uid, models.BookCategoryVisibilityRequest{BookId: book.Id, Revision: 0, Changes: []models.BookCategoryVisibilityChange{{CategoryId: "9301", Hidden: false}, {CategoryId: "9302", Hidden: true}}})
	require.NoError(t, err)
	require.Equal(t, int64(1), changed.CategoryScopeRevision)
	require.ElementsMatch(t, []string{"9302", "9303"}, changed.HiddenCategoryIds)
	require.NotContains(t, changed.HiddenCategoryIds, "9305", "new categories are not hidden by the creation snapshot")
	_, err = Books.SetCategoryVisibility(nil, f.uid, models.BookCategoryVisibilityRequest{BookId: book.Id, Revision: 0, Changes: []models.BookCategoryVisibilityChange{{CategoryId: "9303", Hidden: false}}})
	require.ErrorIs(t, err, ErrBookCategoryScopeConflict)
	_, err = Books.SetCategoryVisibility(nil, f.uid, models.BookCategoryVisibilityRequest{BookId: book.Id, Revision: 1, Changes: []models.BookCategoryVisibilityChange{{CategoryId: "9303", Hidden: false}, {CategoryId: "9304", Hidden: true}}})
	require.Error(t, err, "foreign category aborts the whole update")
	renamed := "已重命名"
	modified, err := Books.Modify(nil, f.uid, models.BookModifyRequest{Id: book.Id, Name: &renamed})
	require.NoError(t, err)
	require.Equal(t, int64(1), modified.CategoryScopeRevision)
	require.ElementsMatch(t, []string{"9302", "9303"}, modified.HiddenCategoryIds)
	require.Equal(t, balance, f.balance())
	var category models.TransactionCategory
	has, err := f.engine.ID(9301).Get(&category)
	require.NoError(t, err)
	require.True(t, has)
	require.Empty(t, category.BookIds, "do not rewrite existing category scopes")

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "category-scope.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error { _, e := f.engine.Exec("VACUUM INTO ?", target); return e }))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	var saved models.Book
	has, err = restored.ID(book.Id).Get(&saved)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, int64(1), saved.CategoryScopeRevision)
	require.ElementsMatch(t, []string{"9302", "9303"}, saved.HiddenCategoryIds)
}
