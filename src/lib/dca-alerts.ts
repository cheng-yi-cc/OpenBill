import moment from 'moment-timezone';
import { dcaSymbols, type CryptoDCAAlert } from '@/lib/crypto-dca.ts';
import type { FundDCAAlert } from '@/lib/investments.ts';
import { notificationStatus } from '@/lib/asset-tools.ts';

export interface UnifiedDCAAlert {
    id: string;
    kind: 'crypto' | 'fund';
    severity: 'paused' | 'due' | 'low';
    title: string;
    text: string;
    route: string;
    day: string;
    heading: string;
    detail: string;
}

function joinPlanNames(names: string[]): string {
    if (!names.length) return '基金定投';
    if (names.length <= 2) return names.join('、');
    return names.slice(0, 2).join('、') + '等';
}

export function buildUnifiedDCAAlerts(
    cryptoAlerts: CryptoDCAAlert[],
    fundAlerts: FundDCAAlert[],
    timeZone?: string
): UnifiedDCAAlert[] {
    const day = timeZone ? moment().tz(timeZone).format('YYYY-MM-DD') : moment().format('YYYY-MM-DD');
    const out: UnifiedDCAAlert[] = [];

    for (const a of cryptoAlerts) {
        const sym = dcaSymbols[a.paymentInstrumentId] || a.paymentInstrumentId;
        const paused = a.paused;
        const title = paused ? '定投因余额不足已暂停' : '定投稳定币余额即将不足';
        const text = paused
            ? (a.required && a.required !== '0'
                ? `${a.accountName} · ${sym} 当前 ${a.balance}，部分定投已暂停，其余未来 3 天需 ${a.required}；补足后请手动恢复`
                : `${a.accountName} · ${sym} 当前 ${a.balance}，不足以执行每日定投；补足后请到每日定投手动恢复`)
            : `${a.accountName} · ${sym} 当前 ${a.balance}，未来 3 天定投需 ${a.required}，请及时补充`;
        const detail = paused
            ? (a.required && a.required !== '0'
                ? `当前 ${a.balance} ${sym}，部分定投已暂停，其余未来 3 天需 ${a.required}；补足后请手动恢复`
                : `当前 ${a.balance} ${sym}，不足单次定投已暂停；补足后请手动恢复`)
            : `当前 ${a.balance} ${sym}，未来 3 天定投需 ${a.required}，请及时补充`;
        out.push({
            id: `crypto:${a.accountId}:${a.paymentInstrumentId}`,
            kind: 'crypto',
            severity: paused ? 'paused' : 'low',
            title,
            text,
            route: `/crypto/dca?accountId=${a.accountId}`,
            day,
            heading: `${a.accountName} · ${sym}`,
            detail,
        });
    }

    for (const a of fundAlerts) {
        const names = joinPlanNames(a.planNames || []);
        const due = a.insufficientNow;
        const title = due ? '基金定投余额不足待处理' : '基金定投余额即将不足';
        const text = due
            ? `${a.cashAccountName} 当前 ¥${a.balance}，待扣款/近期定投（${names}）需 ¥${a.required}，请补充资金后确认`
            : `${a.cashAccountName} 当前 ¥${a.balance}，未来 3 天定投（${names}）需 ¥${a.required}，请及时补充资金`;
        const detail = due
            ? `当前余额 ¥${a.balance}，待扣款定投需 ¥${a.required}，请补充资金后确认`
            : `当前余额 ¥${a.balance}，未来 3 天定投需 ¥${a.required}，请及时补充`;
        out.push({
            id: `fund:${a.cashAccountId}`,
            kind: 'fund',
            severity: due ? 'due' : 'low',
            title,
            text,
            route: due ? '/investments/plans?tab=orders' : '/investments/plans',
            day,
            heading: `${a.cashAccountName} · ${names}`,
            detail,
        });
    }

    return out;
}

export function dispatchNativeDCAAlerts(alerts: UnifiedDCAAlert[]): boolean {
    if (notificationStatus() !== 'granted' || !window.CYLedgerReminders?.notifyDca) {
        return false;
    }
    const payload = alerts.map(a => ({
        id: a.id,
        kind: a.kind,
        severity: a.severity,
        title: a.title,
        text: a.text,
        route: a.route,
        day: a.day,
    }));
    window.CYLedgerReminders.notifyDca(JSON.stringify(payload));
    return true;
}
