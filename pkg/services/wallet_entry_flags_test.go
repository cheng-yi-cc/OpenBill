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

func TestWalletReportingFlagsRevisionLegacyClientAndBackup(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	input := walletEvent(f, first, second)
	initialCash := f.balance()
	current := f.create(input, "wallet-flags-create").Event
	for _, flags := range []struct{ statistics, budget, hidden bool }{
		{false, false, false}, {true, false, true}, {false, true, false}, {true, true, true},
	} {
		current.Wallet.ExcludeFromStatistics = &flags.statistics
		current.Wallet.ExcludeFromBudget = &flags.budget
		current.Wallet.HideAmount = &flags.hidden
		result, err := f.s.Mutate(nil, f.uid, current, "", "revise", false)
		require.NoError(t, err)
		current = result.Event
		var postings []models.Transaction
		require.NoError(t, f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, current.ID, false).Find(&postings))
		require.Len(t, postings, 1)
		require.Equal(t, int64(10800), postings[0].Amount)
		require.Equal(t, flags.statistics, postings[0].ExcludeFromStatistics)
		require.Equal(t, flags.budget, postings[0].IsExcludedFromBudget())
		require.Equal(t, flags.hidden, postings[0].HideAmount)
		require.Equal(t, initialCash, f.balance())
	}
	// Simulate an older client that sends the known wallet fields only.
	legacy := current
	wallet := *current.Wallet
	legacy.Wallet = &wallet
	legacy.Wallet.ExcludeFromStatistics, legacy.Wallet.ExcludeFromBudget, legacy.Wallet.HideAmount = nil, nil, nil
	legacy.Note = "旧客户端修改备注"
	revised, err := f.s.Mutate(nil, f.uid, legacy, "", "revise", false)
	require.NoError(t, err)
	current = revised.Event
	require.True(t, *current.Wallet.ExcludeFromStatistics)
	require.True(t, *current.Wallet.ExcludeFromBudget)
	require.True(t, *current.Wallet.HideAmount)

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "wallet-flags.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error { _, e := f.engine.Exec("VACUUM INTO ?", target); return e }))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	restored, err := xorm.NewEngine("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer restored.Close()
	var record models.InvestmentEventRecord
	has, err := restored.Where("uid=? AND id=?", f.uid, current.ID).Get(&record)
	require.NoError(t, err)
	require.True(t, has)
	var restoredEvent InvestmentEvent
	require.NoError(t, json.Unmarshal([]byte(record.Payload), &restoredEvent))
	require.True(t, *restoredEvent.Wallet.ExcludeFromStatistics)
	require.True(t, *restoredEvent.Wallet.ExcludeFromBudget)
	require.True(t, *restoredEvent.Wallet.HideAmount)
	var posting models.Transaction
	has, err = restored.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, current.ID, false).Get(&posting)
	require.NoError(t, err)
	require.True(t, has)
	require.True(t, posting.ExcludeFromStatistics)
	require.True(t, posting.IsExcludedFromBudget())
	require.True(t, posting.HideAmount)

	_, err = f.s.Mutate(nil, f.uid, current, "", "void", false)
	require.NoError(t, err)
	var active []models.Transaction
	require.NoError(t, f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, current.ID, false).Find(&active))
	require.Empty(t, active)
	require.Equal(t, initialCash, f.balance())
}
