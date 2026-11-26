package services

import (
	"sort"
	"time"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/models"
)

// NextDCAScheduleTimes returns deduplicated upcoming Unix millisecond timestamps
// when background DCA execution or retry should wake up.
func (s *InvestmentService) NextDCAScheduleTimes(c core.Context, uid int64, now time.Time) ([]int64, error) {
	sess := s.UserDataDB(uid).NewSession(c)
	defer sess.Close()
	var times []int64
	limit := now.Add(7 * 24 * time.Hour)
	add := func(at time.Time) {
		if at.After(now) && !at.After(limit) {
			times = append(times, at.UnixMilli())
		}
	}

	var cryptoPlans []models.CryptoDCAPlan
	if err := sess.Where("uid=? AND enabled=?", uid, true).Find(&cryptoPlans); err != nil {
		return nil, err
	}
	activeCrypto := map[string]bool{}
	for _, p := range cryptoPlans {
		activeCrypto[p.Id] = true
		if at, err := cryptoDCAScheduled(p, p.NextDate); err == nil {
			add(at.Add(75 * time.Second))
		}
	}

	var cryptoDays []models.CryptoDCADay
	if err := sess.Where("uid=? AND status=?", uid, "pending").Find(&cryptoDays); err != nil {
		return nil, err
	}
	for _, d := range cryptoDays {
		if !activeCrypto[d.PlanId] {
			continue
		}
		retry := time.Unix(d.LastAttempt, 0).Add(20 * time.Minute)
		if minRetry := now.Add(5 * time.Minute); retry.Before(minRetry) {
			retry = minRetry
		}
		add(retry)
	}

	var fundPlans []models.InvestmentPlan
	if err := sess.Where("uid=? AND paused=? AND deleted=?", uid, false, false).Find(&fundPlans); err != nil {
		return nil, err
	}
	activeFund := map[string]bool{}
	for _, p := range fundPlans {
		activeFund[p.Id] = true
		zone, err := time.LoadLocation(p.TimeZone)
		if err != nil {
			continue
		}
		date := p.NextDate
		if date == "" {
			date = p.StartDate
		}
		if date == "" || (p.EndDate != "" && date > p.EndDate) {
			continue
		}
		if at, err := time.ParseInLocation("2006-01-02 15:04", date+" "+p.Time, zone); err == nil {
			add(at.Add(75 * time.Second))
		}
	}

	var fundOrders []models.InvestmentOrder
	if err := sess.Where("uid=? AND status=?", uid, "pending").Find(&fundOrders); err != nil {
		return nil, err
	}
	for _, o := range fundOrders {
		if !activeFund[o.PlanId] {
			continue
		}
		retry := time.Unix(o.LastAttempt, 0).Add(65 * time.Minute)
		if minRetry := now.Add(5 * time.Minute); retry.Before(minRetry) {
			retry = minRetry
		}
		add(retry)
	}

	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	out := make([]int64, 0, len(times))
	for _, ts := range times {
		if len(out) == 0 || ts-out[len(out)-1] >= 120_000 {
			out = append(out, ts)
			if len(out) >= 8 {
				break
			}
		}
	}
	return out, nil
}
