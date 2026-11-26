package services

import (
	"encoding/json"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/models"
)

// InvestmentSettlementDetails resolves optional labels without changing any
// investment fact or copying monetary values into the ordinary ledger.
func (s *InvestmentService) InvestmentSettlementDetails(c core.Context, uid int64, eventIDs []string) (map[string]*models.InvestmentSettlementInfo, error) {
	result := map[string]*models.InvestmentSettlementInfo{}
	if len(eventIDs) == 0 {
		return result, nil
	}
	sess := s.UserDataDB(uid).NewSession(c)
	defer sess.Close()
	var rows []models.InvestmentEventRecord
	for start := 0; start < len(eventIDs); start += 400 {
		end := start + 400
		if end > len(eventIDs) {
			end = len(eventIDs)
		}
		var part []models.InvestmentEventRecord
		if err := sess.Where("uid=? AND voided=?", uid, false).In("id", eventIDs[start:end]).Find(&part); err != nil {
			return nil, err
		}
		rows = append(rows, part...)
	}
	if len(rows) == 0 {
		return result, nil
	}
	var instruments []models.InvestmentInstrument
	if err := sess.Where("uid=?", uid).Find(&instruments); err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, item := range investmentPresets {
		names[item.Id] = item.Name
	}
	for _, item := range instruments {
		names[item.Id] = item.Name
	}
	var profiles []models.InvestmentHoldingProfile
	if err := sess.Where("uid=?", uid).Find(&profiles); err != nil {
		return nil, err
	}
	profileNames := map[string]string{}
	for _, item := range profiles {
		if item.Name != "" {
			profileNames[item.AccountId+"\x00"+item.InstrumentId] = item.Name
		}
	}
	for _, row := range rows {
		var event InvestmentEvent
		// Missing or unreadable optional metadata must not hide ordinary bills.
		if json.Unmarshal([]byte(row.Payload), &event) != nil || event.Wallet != nil || (event.Type != "BUY" && event.Type != "SELL") {
			continue
		}
		name := profileNames[event.AccountID+"\x00"+event.InstrumentID]
		if name == "" {
			name = names[event.InstrumentID]
		}
		if name != "" {
			result[row.Id] = &models.InvestmentSettlementInfo{Type: event.Type, Name: name}
		}
	}
	return result, nil
}
