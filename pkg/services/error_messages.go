package services

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/mayswind/ezbookkeeping/pkg/investments"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"xorm.io/xorm"
)

var replayInsufficientPattern = regexp.MustCompile(`^insufficient holding for account "([^"]+)", instrument "([^"]+)": available ([^,]+), required (.+)$`)

func containsChinese(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// localizeReplayError translates investment replay engine errors into natural
// Chinese without exposing internal event/account/instrument IDs.
func localizeReplayError(sess *xorm.Session, uid int64, err error) string {
	if err == nil {
		return ""
	}
	var replayErr *investments.ReplayError
	if !errors.As(err, &replayErr) {
		msg := strings.TrimSpace(err.Error())
		if containsChinese(msg) {
			return msg
		}
		return "投资记录与历史持仓冲突，请核对时间、账户和数量"
	}

	msg := strings.TrimSpace(replayErr.Message)
	if m := replayInsufficientPattern.FindStringSubmatch(msg); len(m) == 5 {
		accountName := resolvePortfolioAccountName(sess, uid, m[1])
		instrumentName := resolveInstrumentLabel(sess, uid, m[2])
		if accountName != "" && instrumentName != "" {
			return fmt.Sprintf("「%s」中的 %s 可用数量不足（可用 %s，需要 %s），请核对数量或先补充持仓", accountName, instrumentName, m[3], m[4])
		}
		if instrumentName != "" {
			return fmt.Sprintf("%s 可用数量不足（可用 %s，需要 %s），请核对数量或先补充持仓", instrumentName, m[3], m[4])
		}
		return fmt.Sprintf("对应账户持仓数量不足（可用 %s，需要 %s），请核对数量或先补充持仓", m[3], m[4])
	}

	switch {
	case strings.Contains(msg, "must not exceed sell proceeds"):
		return "手续费不能大于卖出成交金额"
	case strings.Contains(msg, "must differ from the traded instrument"):
		return "结算币种不能与买入或卖出币种相同，同币种请使用转移"
	case strings.Contains(msg, "must identify a different investment account"):
		return "转入账户不能与转出账户相同"
	case strings.Contains(msg, "must be greater than zero"):
		return "数量或金额必须大于 0"
	case strings.Contains(msg, "must not be negative"):
		return "数量或金额不能为负数"
	case strings.Contains(msg, "plain decimal string"):
		return "请输入有效的数字（最多 18 位小数）"
	case strings.Contains(msg, "opening holdings do not have a settlement or fee"):
		return "已有持仓记录不需要填写结算金额或手续费"
	case strings.Contains(msg, "transfers only support a same-asset quantity and network fee"):
		return "同资产转移只需填写转出数量与网络手续费"
	case strings.Contains(msg, "duplicate active event"):
		return "该记录已存在，请刷新页面后重试"
	case strings.Contains(msg, "explicit cost is only accepted for opening holdings"):
		return "仅「已有持仓」记录可直接填写总成本"
	case containsChinese(msg):
		return msg
	default:
		return "本次操作会导致历史持仓数量不足或记录冲突，请核对时间、账户与数量"
	}
}

func resolvePortfolioAccountName(sess *xorm.Session, uid int64, accountID string) string {
	if sess == nil || uid <= 0 || accountID == "" {
		return ""
	}
	var account models.PortfolioAccount
	has, err := sess.Where("uid=? AND id=?", uid, accountID).Get(&account)
	if err == nil && has && strings.TrimSpace(account.Name) != "" {
		return strings.TrimSpace(account.Name)
	}
	return ""
}

func resolveInstrumentLabel(sess *xorm.Session, uid int64, instrumentID string) string {
	if instrumentID == "" {
		return ""
	}
	for _, preset := range investmentPresets {
		if preset.Id == instrumentID {
			if preset.Symbol != "" {
				return preset.Symbol
			}
			return preset.Name
		}
	}
	if sess == nil || uid <= 0 {
		return ""
	}
	var inst models.InvestmentInstrument
	has, err := sess.Where("uid=? AND id=?", uid, instrumentID).Get(&inst)
	if err == nil && has {
		if strings.TrimSpace(inst.Name) != "" {
			return strings.TrimSpace(inst.Name)
		}
		if strings.TrimSpace(inst.Symbol) != "" {
			return strings.TrimSpace(inst.Symbol)
		}
	}
	return ""
}

// localizeMarketQuoteError converts upstream market quote failures into clear
// Chinese user messages while avoiding raw HTTP/provider jargon.
func localizeMarketQuoteError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	switch msg {
	case "供应商报价无效或已经过期":
		return "暂未获取到最新有效行情，请稍后重试"
	case "加密现货产品未通过来源核验":
		return "暂不支持核验该币种行情，请改用手动填价"
	case "缺少对应现货产品的成交报价":
		return "暂未获取到该币种的成交行情，请稍后重试"
	}
	if containsChinese(msg) {
		return msg
	}
	return "行情服务暂时无响应，请稍后重试或改用手动填价"
}
