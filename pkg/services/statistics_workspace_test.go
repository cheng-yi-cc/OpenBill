package services

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/datastore"
	"github.com/mayswind/ezbookkeeping/pkg/localbackup"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestStatisticsSQLiteBudgetsNotesAndOwnership(t *testing.T) {
	f := newInvestmentDBFixture(t)
	s := StatisticsWorkspace
	b, err := s.SaveBudget(nil, f.uid, models.StatisticsBudget{BookId: DefaultBookID(f.uid), Amount: "2000.01", Kind: "monthly", StartDate: "2026-10-01", EndDate: "2026-10-31", Repeat: true})
	require.NoError(t, err)
	require.Equal(t, "2000.01", b.Amount)
	require.Equal(t, int64(2000000), f.balance())
	_, err = s.SaveBudget(nil, f.uid, models.StatisticsBudget{BookId: b.BookId, Amount: "100", Kind: "monthly", StartDate: b.StartDate, EndDate: b.EndDate})
	require.ErrorIs(t, err, ErrStatisticsConflict)
	other, err := Books.Create(nil, f.uid+1, models.BookCreateRequest{Name: "其他人"})
	require.NoError(t, err)
	for _, amount := range []string{"-1", "1e9", "0.001", "NaN", strings.Repeat("9", 1000)} {
		invalid := *b
		invalid.Amount = amount
		_, err := s.SaveBudget(nil, f.uid, invalid)
		require.ErrorIs(t, err, ErrStatisticsInvalid)
	}
	invalid := *b
	invalid.BookId = other.Id
	_, err = s.SaveBudget(nil, f.uid, invalid)
	require.ErrorIs(t, err, ErrBookNotFound)
	invalid = *b
	invalid.EndDate = "2026-10-30"
	_, err = s.SaveBudget(nil, f.uid, invalid)
	require.ErrorIs(t, err, ErrStatisticsInvalid)
	note, err := s.SaveNote(nil, f.uid, models.StatisticsNote{BookId: b.BookId, Period: "2026-10", Content: "本月总结"})
	require.NoError(t, err)
	stale := *note
	note.Content = "修订总结"
	_, err = s.SaveNote(nil, f.uid, *note)
	require.NoError(t, err)
	_, err = s.SaveNote(nil, f.uid, stale)
	require.ErrorIs(t, err, ErrStatisticsConflict)
	items, err := s.Notes(nil, f.uid+1)
	require.NoError(t, err)
	require.Empty(t, items)
	require.ErrorIs(t, s.DeleteBudget(nil, f.uid+1, models.StatisticsDeleteRequest{Id: b.Id, Revision: b.Revision}), ErrStatisticsConflict)
	require.NoError(t, s.DeleteBudget(nil, f.uid, models.StatisticsDeleteRequest{Id: b.Id, Revision: b.Revision}))
}

func TestStatisticsSQLitePreferenceValidationAndRevision(t *testing.T) {
	f := newInvestmentDBFixture(t)
	p, err := StatisticsWorkspace.Preferences(nil, f.uid)
	require.NoError(t, err)
	p.Modules["month"][0].Visible = false
	p.CarrySurplus = true
	saved, err := StatisticsWorkspace.SavePreferences(nil, f.uid, *p)
	require.NoError(t, err)
	_, err = StatisticsWorkspace.SavePreferences(nil, f.uid, *p)
	require.ErrorIs(t, err, ErrStatisticsConflict)
	loaded, err := StatisticsWorkspace.Preferences(nil, f.uid)
	require.NoError(t, err)
	require.Equal(t, saved, loaded)
	loaded.Modules["month"][1].Id = loaded.Modules["month"][0].Id
	_, err = StatisticsWorkspace.SavePreferences(nil, f.uid, *loaded)
	require.ErrorIs(t, err, ErrStatisticsInvalid)
}

func TestStatisticsSQLiteDiscountIsInformationalAndAtomic(t *testing.T) {
	f := newInvestmentDBFixture(t)
	tx := addBookExpense(f, "", 8000, 1)
	tx.DiscountAmount = "20.10"
	require.NoError(t, Transactions.ModifyTransaction(nil, tx, false, 0, nil, nil, nil, nil))
	require.Equal(t, int64(1992000), f.balance())
	stored, err := Transactions.GetTransactionByTransactionId(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.Equal(t, "20.10", stored.DiscountAmount)
	tx.DiscountAmount = "-1"
	tx.Amount = 5000
	require.ErrorIs(t, Transactions.ModifyTransaction(nil, tx, false, 0, nil, nil, nil, nil), ErrStatisticsInvalid)
	require.Equal(t, int64(1992000), f.balance())
	tx.DiscountAmount = "0"
	tx.Amount = -1000
	require.NoError(t, Transactions.ModifyTransaction(nil, tx, false, 0, nil, nil, nil, nil))
	require.Equal(t, int64(2001000), f.balance())
}

func TestBudgetExclusionLegacySQLiteAndIndependentUpdates(t *testing.T) {
	f := newInvestmentDBFixture(t)
	tx := addBookExpense(f, "", 1200, 1)
	_, err := f.engine.ID(tx.TransactionId).Cols("exclude_from_statistics").Update(&models.Transaction{ExcludeFromStatistics: true})
	require.NoError(t, err)
	// Simulate a database created before the new column and the normal startup sync.
	_, err = f.engine.Exec(`ALTER TABLE "transaction" DROP COLUMN budget_exclusion_mode`)
	require.NoError(t, err)
	require.NoError(t, datastore.Container.UserDataStore.SyncStructs(new(models.Transaction)))
	stored, err := Transactions.GetTransactionByTransactionId(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.True(t, stored.ExcludeFromStatistics)
	require.True(t, stored.IsExcludedFromBudget())
	require.True(t, stored.ToTransactionInfoResponse(nil, true).ExcludeFromBudget)
	before := f.balance()
	include := false
	stored.SetBudgetExclusion(&include)
	require.NoError(t, Transactions.ModifyTransaction(nil, stored, false, 0, nil, nil, nil, nil))
	require.NoError(t, datastore.Container.UserDataStore.SyncStructs(new(models.Transaction)))
	stored, err = Transactions.GetTransactionByTransactionId(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.True(t, stored.ExcludeFromStatistics)
	require.False(t, stored.IsExcludedFromBudget())
	require.False(t, stored.ToTransactionInfoResponse(nil, true).ExcludeFromBudget)
	require.Equal(t, before, f.balance())
	stored.SetBudgetExclusion(nil)
	stored.ExcludeFromStatistics = false
	require.NoError(t, Transactions.ModifyTransaction(nil, stored, false, 0, nil, nil, nil, nil))
	stored, err = Transactions.GetTransactionByTransactionId(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.False(t, stored.IsExcludedFromBudget())
	exclude := true
	stored.SetBudgetExclusion(&exclude)
	require.NoError(t, Transactions.ModifyTransaction(nil, stored, false, 0, nil, nil, nil, nil))
	stored, err = Transactions.GetTransactionByTransactionId(nil, f.uid, tx.TransactionId)
	require.NoError(t, err)
	require.False(t, stored.ExcludeFromStatistics)
	require.True(t, stored.IsExcludedFromBudget())
	require.Equal(t, before, f.balance())
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "cyledger.ini"), []byte("[database]\ntype=sqlite3\n"), 0600))
	archive := filepath.Join(t.TempDir(), "budget-backup.zip")
	require.NoError(t, localbackup.Create(root, archive, []byte(`{}`), func(target string) error {
		_, snapshotErr := f.engine.Exec("VACUUM INTO ?", target)
		return snapshotErr
	}))
	stage, err := localbackup.Stage(archive, t.TempDir())
	require.NoError(t, err)
	db, err := sql.Open("sqlite3", filepath.Join(stage, "data", "cyledger.db"))
	require.NoError(t, err)
	defer db.Close()
	var mode int
	var excluded bool
	err = db.QueryRow(`SELECT budget_exclusion_mode, exclude_from_statistics FROM "transaction" WHERE transaction_id=?`, tx.TransactionId).Scan(&mode, &excluded)
	require.NoError(t, err)
	require.Equal(t, 2, mode)
	require.False(t, excluded)
}

func TestStatisticsSQLiteTagHierarchyCannotCycleCrossUsersOrOrphan(t *testing.T) {
	f := newInvestmentDBFixture(t)
	parent := &models.TransactionTag{Uid: f.uid, Name: "旅行"}
	require.NoError(t, TransactionTags.CreateTag(nil, parent))
	child := &models.TransactionTag{Uid: f.uid, Name: "杭州", ParentTagId: parent.TagId}
	require.NoError(t, TransactionTags.CreateTag(nil, child))
	grandchild := &models.TransactionTag{Uid: f.uid, Name: "西湖", ParentTagId: child.TagId}
	require.ErrorIs(t, TransactionTags.CreateTag(nil, grandchild), ErrTagHierarchy)
	foreign := &models.TransactionTag{Uid: f.uid + 1, Name: "外部", ParentTagId: parent.TagId}
	require.ErrorIs(t, TransactionTags.CreateTag(nil, foreign), ErrTagHierarchy)
	parent.ParentTagId = child.TagId
	require.ErrorIs(t, TransactionTags.ModifyTag(nil, parent, false), ErrTagHierarchy)
	require.ErrorIs(t, TransactionTags.DeleteTag(nil, f.uid, parent.TagId), ErrTagHasChildren)
	child.ParentTagId = 0
	require.NoError(t, TransactionTags.ModifyTag(nil, child, false))
	require.NoError(t, TransactionTags.DeleteTag(nil, f.uid, parent.TagId))
}
