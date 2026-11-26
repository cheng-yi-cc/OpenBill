package services

import (
	"fmt"
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/investments"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestInvestmentSettlementDetailsUsesOwnEventAndHoldingName(t *testing.T) {
	f := newInvestmentDBFixture(t)
	buy := f.create(f.event(investments.Buy, "0.01", "20", "0", 1), "display-buy")
	sell := f.create(f.event(investments.Sell, "0.005", "10", "0", 2), "display-sell")
	_, err := f.engine.Insert(&models.InvestmentHoldingProfile{Id: "display-profile", Uid: f.uid, AccountId: f.portfolioID, InstrumentId: "crypto:bitcoin", Name: "虚构持仓"})
	require.NoError(t, err)
	_, err = f.engine.Insert(&models.InvestmentEventRecord{Id: "foreign-display", Uid: f.uid + 1, Version: 1, Payload: `{"type":"BUY","instrumentId":"crypto:bitcoin"}`}, &models.InvestmentEventRecord{Id: "broken-display", Uid: f.uid, Version: 1, Payload: `{`})
	require.NoError(t, err)
	before := f.balance()
	ids := make([]string, 405)
	for i := range ids {
		ids[i] = fmt.Sprintf("missing-%d", i)
	}
	ids = append(ids, buy.Event.ID, sell.Event.ID, "foreign-display", "broken-display")
	labels, err := f.s.InvestmentSettlementDetails(nil, f.uid, ids)
	require.NoError(t, err)
	require.Equal(t, &models.InvestmentSettlementInfo{Type: "BUY", Name: "虚构持仓"}, labels[buy.Event.ID])
	require.Equal(t, &models.InvestmentSettlementInfo{Type: "SELL", Name: "虚构持仓"}, labels[sell.Event.ID])
	require.NotContains(t, labels, "foreign-display")
	require.NotContains(t, labels, "broken-display")
	require.Equal(t, before, f.balance())
	_, err = f.engine.ID("display-profile").Cols("name").Update(&models.InvestmentHoldingProfile{Name: ""})
	require.NoError(t, err)
	labels, err = f.s.InvestmentSettlementDetails(nil, f.uid, []string{buy.Event.ID})
	require.NoError(t, err)
	require.Equal(t, "比特币", labels[buy.Event.ID].Name)
}
