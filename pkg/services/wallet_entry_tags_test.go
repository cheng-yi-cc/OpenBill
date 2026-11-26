package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestWalletTagsAtomicRevisionAndLegacyClient(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	_, err := f.engine.Insert(
		&models.TransactionTag{TagId: 801, Uid: f.uid, Name: "测试标签"},
		&models.TransactionTag{TagId: 802, Uid: f.uid, Name: "隐藏标签", Hidden: true},
		&models.TransactionTag{TagId: 803, Uid: f.uid + 1, Name: "其他用户"},
		&models.TransactionTag{TagId: 804, Uid: f.uid, Name: "撤销标签"},
	)
	require.NoError(t, err)
	input := walletEvent(f, first, second)
	ids := []string{"801"}
	input.Wallet.TagIDs = &ids
	_, err = f.s.Mutate(nil, f.uid, input, "", "create", true)
	require.NoError(t, err)
	require.Zero(t, f.count(&models.TransactionTagIndex{}))
	paid := f.create(input, "wallet-tags")
	require.Equal(t, paid.Event.ID, f.create(input, "wallet-tags").Event.ID)
	active := func() []models.TransactionTagIndex {
		t.Helper()
		var rows []models.TransactionTagIndex
		require.NoError(t, f.engine.Where("uid=? AND deleted=?", f.uid, false).Find(&rows))
		return rows
	}
	oldIndex := active()
	require.Len(t, oldIndex, 1)
	require.Error(t, TransactionTags.DeleteTag(nil, f.uid, 801))
	_, err = f.engine.ID(int64(801)).Cols("hidden").Update(&models.TransactionTag{Hidden: true})
	require.NoError(t, err)
	legacy := paid.Event
	wallet := *legacy.Wallet
	legacy.Wallet = &wallet
	legacy.Wallet.TagIDs = nil
	legacy.Note = "旧客户端修改"
	_, err = f.s.Mutate(nil, f.uid, legacy, "", "revise", true)
	require.NoError(t, err)
	require.Equal(t, oldIndex, active(), "preview must roll back index changes")
	_, err = f.engine.Exec("CREATE TRIGGER reject_wallet_tag_move BEFORE UPDATE OF transaction_id ON transaction_tag_index BEGIN SELECT RAISE(ABORT, 'injected failure'); END")
	require.NoError(t, err)
	_, err = f.s.Mutate(nil, f.uid, legacy, "", "revise", false)
	require.Error(t, err)
	require.Equal(t, oldIndex, active(), "failed revision must restore the original index")
	var stored models.InvestmentEventRecord
	_, err = f.engine.ID(paid.Event.ID).Get(&stored)
	require.NoError(t, err)
	require.Equal(t, 1, stored.Version)
	_, err = f.engine.Exec("DROP TRIGGER reject_wallet_tag_move")
	require.NoError(t, err)
	revised, err := f.s.Mutate(nil, f.uid, legacy, "", "revise", false)
	require.NoError(t, err)
	require.Equal(t, []string{"801"}, *revised.Event.Wallet.TagIDs)
	current := active()
	require.Len(t, current, 1)
	require.Equal(t, oldIndex[0].TagIndexId, current[0].TagIndexId)
	require.NotEqual(t, oldIndex[0].TransactionId, current[0].TransactionId)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "wallet-tags.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(path string) error { _, e := f.engine.Exec("VACUUM INTO ?", path); return e }))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	var savedIndex models.TransactionTagIndex
	found, err := restored.ID(current[0].TagIndexId).Get(&savedIndex)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, current[0], savedIndex)
	var savedEvent models.InvestmentEventRecord
	found, err = restored.ID(paid.Event.ID).Get(&savedEvent)
	require.NoError(t, err)
	require.True(t, found)
	var decoded InvestmentEvent
	require.NoError(t, json.Unmarshal([]byte(savedEvent.Payload), &decoded))
	require.Equal(t, []string{"801"}, *decoded.Wallet.TagIDs)
	var savedPosting models.Transaction
	found, err = restored.ID(savedIndex.TransactionId).Get(&savedPosting)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, savedPosting.Deleted)
	require.Equal(t, paid.Event.ID, savedPosting.InvestmentEventId)
	_, err = f.s.Mutate(nil, f.uid, legacy, "", "revise", false)
	require.ErrorIs(t, err, ErrInvestmentConflict)
	for _, values := range [][]string{{"801", "802"}, {"801", "803"}, {"801", "801"}, {"01"}, {"0"}} {
		bad := revised.Event
		w := *bad.Wallet
		bad.Wallet = &w
		bad.Wallet.TagIDs = &values
		_, err = f.s.Mutate(nil, f.uid, bad, "", "revise", false)
		require.Error(t, err, "%v", values)
		require.Equal(t, current, active())
	}
	empty := []string{}
	cleared := revised.Event
	w := *cleared.Wallet
	cleared.Wallet = &w
	cleared.Wallet.TagIDs = &empty
	clearedResult, err := f.s.Mutate(nil, f.uid, cleared, "", "revise", false)
	require.NoError(t, err)
	require.Empty(t, active())
	require.NoError(t, TransactionTags.DeleteTag(nil, f.uid, 801))
	_, err = f.s.Mutate(nil, f.uid, clearedResult.Event, "", "void", false)
	require.NoError(t, err)
	require.Empty(t, active())
	voidIDs := []string{"804"}
	input.Wallet.TagIDs = &voidIDs
	toVoid := f.create(input, "wallet-tags-void")
	require.Len(t, active(), 1)
	_, err = f.s.Mutate(nil, f.uid, toVoid.Event, "", "void", false)
	require.NoError(t, err)
	require.Empty(t, active())
	require.NoError(t, TransactionTags.DeleteTag(nil, f.uid, 804))
}

func TestWalletTagsAccountDeletionRollbackAndCleanup(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	_, err := f.engine.Insert(&models.TransactionTag{TagId: 805, Uid: f.uid, Name: "钱包标签"})
	require.NoError(t, err)
	input := walletEvent(f, first, second)
	ids := []string{"805"}
	input.Wallet.TagIDs = &ids
	paid := f.create(input, "wallet-tag-account-delete")
	deletion := AccountDeletionInput{ID: f.portfolioID, Kind: "portfolio", DeleteRelated: true}
	preview := deletionPreview(t, f, deletion)
	deletion.Token = preview.Token
	_, err = f.engine.Exec("CREATE TRIGGER reject_wallet_account_delete BEFORE DELETE ON portfolio_account BEGIN SELECT RAISE(ABORT, 'injected account failure'); END")
	require.NoError(t, err)
	_, err = f.s.DeleteAssetAccount(nil, f.uid, deletion, false)
	require.Error(t, err)
	var index models.TransactionTagIndex
	found, err := f.engine.Where("uid=? AND tag_id=? AND deleted=?", f.uid, 805, false).Get(&index)
	require.NoError(t, err)
	require.True(t, found, "failed deletion must retain the tag index")
	var posting models.Transaction
	found, err = f.engine.ID(index.TransactionId).Get(&posting)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, posting.Deleted)
	require.Equal(t, paid.Event.ID, posting.InvestmentEventId)
	_, err = f.engine.Exec("DROP TRIGGER reject_wallet_account_delete")
	require.NoError(t, err)
	_, err = f.s.DeleteAssetAccount(nil, f.uid, deletion, false)
	require.NoError(t, err)
	found, err = f.engine.Where("uid=? AND tag_id=? AND deleted=?", f.uid, 805, false).Exist(&models.TransactionTagIndex{})
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, TransactionTags.DeleteTag(nil, f.uid, 805))
}
