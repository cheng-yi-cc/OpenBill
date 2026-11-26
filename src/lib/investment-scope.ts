import type {HoldingProfile} from '@/models/investment.ts';
import type {AssetPreferences} from '@/lib/asset-tools.ts';
import type {HoldingRow} from '@/lib/investment-mobile.ts';
import {summarizeAmounts} from '@/lib/amount-summary.ts';

export function accountInBooks(preferences: AssetPreferences, kind: 'cash' | 'portfolio', id: string, bookIds: string[]): boolean {
    const disabled = preferences.rules[`${kind}:${id}`]?.disabledBooks || [];
    return !bookIds.length || bookIds.some(book => !disabled.includes(book));
}

// 隐藏只参与列表可见性；账本和计入开关决定汇总范围。
export function holdingInBooks(profile: Pick<HoldingProfile,'accountId'|'bookIds'>, preferences: AssetPreferences, bookIds: string[]): boolean {
    const eligible = bookIds.filter(id => !profile.bookIds.length || profile.bookIds.includes(id));
    return !bookIds.length || eligible.length > 0 && accountInBooks(preferences, 'portfolio', profile.accountId, eligible);
}

export function holdingVisible(profile: HoldingProfile, preferences: AssetPreferences): boolean {
    return !profile.hidden && !preferences.rules[`portfolio:${profile.accountId}`]?.hidden;
}

export function investmentAmounts(rows: HoldingRow[], monetary: {value:string|null;excluded:boolean;binding:{totalIncome?:string}}[]) {
    const included=rows.filter(r=>!r.profile.excludeFromTotal),cash=monetary.filter(r=>!r.excluded);
    const netValue=summarizeAmounts(included.map(r=>r.profile.excludeProfit?r.position.cost:r.position.marketValue).concat(cash.map(r=>r.value)));
    const marketValue=summarizeAmounts(included.map(r=>r.position.marketValue).concat(cash.map(r=>r.value)));
    const holdingProfit=summarizeAmounts(included.map(r=>r.position.unrealizedPnl));
    const totalProfit=summarizeAmounts(included.map(r=>r.profit).concat(cash.map(r=>r.binding.totalIncome??null)));
    return {
        netValue:netValue.value, marketValue:marketValue.value,
        holdingProfit:holdingProfit.value, totalProfit:totalProfit.value,
        missing:{netValue:netValue.missing,marketValue:marketValue.missing,holdingProfit:holdingProfit.missing,totalProfit:totalProfit.missing}
    };
}
