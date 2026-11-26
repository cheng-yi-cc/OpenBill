package services

import (
	"strings"
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestWalletLocationValidationRevisionAndLegacyClient(t *testing.T) {
	f := newInvestmentDBFixture(t)
	first, second := walletFixture(f)
	input := walletEvent(f, first, second)
	tooLong := strings.Repeat("测", 201)
	input.Wallet.LocationName = &tooLong
	before := f.count(&models.Transaction{})
	_, err := f.s.Mutate(nil, f.uid, input, "wallet-location-invalid", "create", false)
	require.Error(t, err)
	require.Equal(t, before, f.count(&models.Transaction{}))
	name := "虚构测试地点"
	input.Wallet.LocationName = &name
	current := f.create(input, "wallet-location-create").Event
	var posting models.Transaction
	has, err := f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, current.ID, false).Get(&posting)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, name, posting.LocationName)
	legacy := current
	wallet := *current.Wallet
	legacy.Wallet = &wallet
	legacy.Wallet.LocationName = nil
	legacy.Note = "旧客户端修订"
	revised, err := f.s.Mutate(nil, f.uid, legacy, "", "revise", false)
	require.NoError(t, err)
	require.Equal(t, name, *revised.Event.Wallet.LocationName)
	posting = models.Transaction{}
	has, err = f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, current.ID, false).Get(&posting)
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, name, posting.LocationName)
	cleared := ""
	revised.Event.Wallet.LocationName = &cleared
	_, err = f.s.Mutate(nil, f.uid, revised.Event, "", "revise", false)
	require.NoError(t, err)
	posting = models.Transaction{}
	has, err = f.engine.Where("uid=? AND investment_event_id=? AND deleted=?", f.uid, current.ID, false).Get(&posting)
	require.NoError(t, err)
	require.True(t, has)
	require.Empty(t, posting.LocationName)
}
