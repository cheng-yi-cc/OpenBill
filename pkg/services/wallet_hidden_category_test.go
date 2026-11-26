package services

import (
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestWalletRevisionRetainsOriginalHiddenCategoryOnly(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	input := walletEvent(f, first, second)
	created := f.create(input, "hidden-category-wallet").Event
	_, err := f.engine.ID(int64(7002)).Cols("hidden").Update(&models.TransactionCategory{Hidden: true})
	require.NoError(t, err)
	input.Note = "新记录不能使用隐藏分类"
	_, err = f.s.Mutate(nil, f.uid, input, "hidden-category-new", "create", false)
	require.Error(t, err)
	changed := created
	changed.Note = "仅修改备注"
	revised, err := f.s.Mutate(nil, f.uid, changed, "", "revise", false)
	require.NoError(t, err)
	require.Equal(t, "仅修改备注", revised.Event.Note)
	var postings []models.Transaction
	require.NoError(t, f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, created.ID, false).Find(&postings))
	require.Len(t, postings, 1)
	require.Equal(t, int64(7002), postings[0].CategoryId)
	other, err := Books.Create(nil, f.uid, models.BookCreateRequest{Name: "另一个账本"})
	require.NoError(t, err)
	crossBook := revised.Event
	crossBook.BookID = other.Id
	_, err = f.s.Mutate(nil, f.uid, crossBook, "", "revise", false)
	require.Error(t, err, "hidden category cannot be moved to another book")
	_, err = f.engine.ID(int64(7002)).Cols("hidden").Update(&models.TransactionCategory{Hidden: false})
	require.NoError(t, err)
	_, err = f.engine.ID(int64(7001)).Cols("hidden").Update(&models.TransactionCategory{Hidden: true})
	require.NoError(t, err)
	changed = revised.Event
	changed.Note = "父分类隐藏后修改备注"
	_, err = f.s.Mutate(nil, f.uid, changed, "", "revise", false)
	require.NoError(t, err)
}
