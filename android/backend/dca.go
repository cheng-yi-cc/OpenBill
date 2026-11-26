package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
)

type dcaNotificationAlert struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Text     string `json:"text"`
	Route    string `json:"route"`
	Day      string `json:"day"`
}

type dcaCheckPayload struct {
	Alerts []dcaNotificationAlert `json:"alerts"`
	NextAt []int64                `json:"nextAt"`
}

func existingPersonalOwner() (*models.User, error) {
	if services.Users == nil || services.Investments == nil {
		return nil, fmt.Errorf("services unavailable")
	}
	ctx := core.NewNullContext()
	session := services.Users.UserDB().NewSession(ctx)
	var users []*models.User
	err := session.Limit(2).Find(&users)
	session.Close()
	if err != nil {
		return nil, err
	}
	if len(users) == 1 && !users[0].Deleted && !users[0].Disabled {
		return users[0], nil
	}
	return nil, fmt.Errorf("personal owner unavailable")
}

func dcaPaymentSymbol(instrumentID string) string {
	switch instrumentID {
	case "crypto:tether":
		return "USDT"
	case "crypto:usd-coin":
		return "USDC"
	case "crypto:bitcoin":
		return "BTC"
	case "crypto:ethereum":
		return "ETH"
	case "crypto:solana":
		return "SOL"
	default:
		return instrumentID
	}
}

func joinPlanNames(names []string) string {
	if len(names) == 0 {
		return "基金定投"
	}
	if len(names) <= 2 {
		return strings.Join(names, "、")
	}
	return strings.Join(names[:2], "、") + "等"
}

func checkDCANotifications(runSync bool) string {
	core.LocalRequestGate.RLock()
	defer core.LocalRequestGate.RUnlock()

	empty := `{"alerts":[],"nextAt":[]}`
	owner, err := existingPersonalOwner()
	if err != nil {
		return empty
	}
	ctx := core.NewNullContext()
	uid := owner.Uid
	now := time.Now()

	zone := time.Local
	if cfg, e := services.Investments.Settings(ctx, uid); e == nil && cfg != nil && cfg.TimeZone != "" {
		if loc, le := time.LoadLocation(cfg.TimeZone); le == nil {
			zone = loc
		}
	}
	day := now.In(zone).Format("2006-01-02")

	var cryptoState *services.CryptoDCAState
	if runSync {
		cryptoState, _ = services.Investments.SyncCryptoDCA(ctx, uid, false)
	}
	if cryptoState == nil {
		cryptoState, _ = services.Investments.CryptoDCAState(ctx, uid, now)
	}

	var fundAlerts []services.FundDCAAlert
	if runSync {
		if res, e := services.Investments.SyncInvestmentPlans(ctx, uid, false); e == nil && res != nil {
			fundAlerts = res.Alerts
		}
	}
	if fundAlerts == nil {
		fundAlerts, _ = services.Investments.FundDCAAlerts(ctx, uid, now)
	}

	nextAt, _ := services.Investments.NextDCAScheduleTimes(ctx, uid, now)
	if nextAt == nil {
		nextAt = []int64{}
	}

	alerts := []dcaNotificationAlert{}
	if cryptoState != nil {
		for _, a := range cryptoState.Alerts {
			sym := dcaPaymentSymbol(a.PaymentInstrumentId)
			severity := "low"
			title := "定投稳定币余额即将不足"
			text := fmt.Sprintf("%s · %s 当前 %s，未来 3 天定投需 %s，请及时补充", a.AccountName, sym, a.Balance, a.Required)
			if a.Paused {
				severity = "paused"
				title = "定投因余额不足已暂停"
				if a.Required != "" && a.Required != "0" {
					text = fmt.Sprintf("%s · %s 当前 %s，部分定投已暂停，其余未来 3 天需 %s；补足后请手动恢复", a.AccountName, sym, a.Balance, a.Required)
				} else {
					text = fmt.Sprintf("%s · %s 当前 %s，不足以执行每日定投；补足后请到每日定投手动恢复", a.AccountName, sym, a.Balance)
				}
			}
			alerts = append(alerts, dcaNotificationAlert{
				ID:       "crypto:" + a.AccountId + ":" + a.PaymentInstrumentId,
				Kind:     "crypto",
				Severity: severity,
				Title:    title,
				Text:     text,
				Route:    "/crypto/dca?accountId=" + a.AccountId,
				Day:      day,
			})
		}
	}

	for _, a := range fundAlerts {
		names := joinPlanNames(a.PlanNames)
		severity := "low"
		title := "基金定投余额即将不足"
		route := "/investments/plans"
		text := fmt.Sprintf("%s 当前 %s 元，未来 3 天定投（%s）需 %s 元，请及时补充资金", a.CashAccountName, a.Balance, names, a.Required)
		if a.InsufficientNow {
			severity = "due"
			title = "基金定投余额不足待处理"
			route = "/investments/plans?tab=orders"
			text = fmt.Sprintf("%s 当前 %s 元，待扣款/近期定投（%s）需 %s 元，请补充资金后确认", a.CashAccountName, a.Balance, names, a.Required)
		}
		alerts = append(alerts, dcaNotificationAlert{
			ID:       "fund:" + a.CashAccountId,
			Kind:     "fund",
			Severity: severity,
			Title:    title,
			Text:     text,
			Route:    route,
			Day:      day,
		})
	}

	raw, err := json.Marshal(dcaCheckPayload{Alerts: alerts, NextAt: nextAt})
	if err != nil {
		return empty
	}
	return string(raw)
}

//export CYLedgerCheckDCA
func CYLedgerCheckDCA(sync C.int) *C.char {
	return C.CString(checkDCANotifications(sync != 0))
}
