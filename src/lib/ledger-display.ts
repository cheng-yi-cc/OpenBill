import Decimal from 'decimal.js';

// Keep financial aggregation in decimal arithmetic; convert only chart coordinates.
export const LedgerDecimal = Decimal.clone({ precision: 40 });

export const CURRENCY_SYMBOLS: Record<string, string> = {
    CNY: '¥',
    USD: '$',
    EUR: '€',
    HKD: 'HK$',
    JPY: 'JP¥',
    GBP: '£'
};

export function currencyPrefix(currency = 'CNY'): string {
    return CURRENCY_SYMBOLS[currency] || (currency ? `${currency} ` : '');
}

export function ledgerMoney(value: string | null | undefined, symbol = true): string {
    if (value == null || value.trim() === '') return '—';
    const dec = new LedgerDecimal(value);
    const negative = dec.lt(0);
    const [whole, fraction] = dec.abs().toFixed(2).split('.');
    const formatted = `${whole!.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}.${fraction}`;
    return `${negative ? '-' : ''}${symbol ? '¥' : ''}${formatted}`;
}

export function ledgerCurrencyMoney(value: string | null | undefined, currency = 'CNY', signed = false): string {
    if (value === null || value === undefined || value === '') return '—';
    const dec = new LedgerDecimal(value);
    const negative = dec.lt(0);
    const [whole, fraction] = dec.abs().toFixed(2).split('.');
    const formatted = `${whole!.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}.${fraction}`;
    const sign = negative ? '-' : (signed && dec.gt(0) ? '+' : '');
    return `${sign}${currencyPrefix(currency)}${formatted}`;
}

export function ledgerSignedAmount(amount: string, type: number): string {
    if (type !== 2 && type !== 3) return ledgerMoney(amount,false);
    const value = new LedgerDecimal(amount).mul(type === 3 ? -1 : 1);
    return `${value.gt(0) ? '+' : ''}${ledgerMoney(value.toString(),false)}`;
}

export function ledgerTotals(items: {type: number; cny: string | null; excludeFromStatistics?: boolean}[], respectStatisticsExclusion = true) {
    let income = new LedgerDecimal(0), expense = new LedgerDecimal(0);
    let complete = true;
    for (const item of items) {
        if (respectStatisticsExclusion && item.excludeFromStatistics) continue;
        if (item.type !== 2 && item.type !== 3) continue;
        if (item.cny === null) { complete = false; continue; }
        if (item.type === 2) income = income.plus(item.cny);
        else expense = expense.plus(item.cny);
    }
    return { income: income.toString(), expense: expense.toString(), balance: income.minus(expense).toString(), complete };
}

// Account activity reports actual movements in the account's own currency.
export function ledgerAccountFlowTotals(items: {type:number; amount:string; transferDirection?:'in'|'out'}[]) {
    return ledgerTotals(items.map(item => ({...item, type:item.type===4 && item.transferDirection ? item.transferDirection==='in' ? 2 : 3 : item.type, cny:item.amount})), false);
}

// Merge only a fully loaded, internally consistent pair. A fee on a different
// page remains visible until its transfer arrives, so no cash flow is invented.
export function groupAccountTransferFees<T extends {id:string; type:number; day:string; accountId:string; currency:string; amount:string; hidden:boolean; transferDirection?:'in'|'out'; transferFeeAmount?:string; transferFeeParentId?:string}>(items:T[]): (T & {accountFee?:string})[] {
    const parents = new Map(items.filter(item => item.type === 4 && item.transferDirection === 'out').map(item => [item.id, item]));
    const candidates = new Map<string,T[]>();
    for (const item of items) {
        if (!item.transferFeeParentId || item.transferFeeParentId === '0') continue;
        const list = candidates.get(item.transferFeeParentId) || [];
        list.push(item); candidates.set(item.transferFeeParentId, list);
    }
    const matches = new Map<string,{fee:T; signed:string}>();
    for (const [id, fees] of candidates) {
        const parent = parents.get(id), fee = fees[0];
        if (!parent || fees.length !== 1 || !fee || (fee.type !== 2 && fee.type !== 3) || parent.day !== fee.day || parent.accountId !== fee.accountId || parent.currency !== fee.currency || !parent.transferFeeAmount) continue;
        try {
            const signed = new LedgerDecimal(fee.amount).mul(fee.type === 3 ? 1 : -1);
            if (signed.isZero() || !signed.eq(new LedgerDecimal(parent.transferFeeAmount)) || new LedgerDecimal(parent.amount).plus(signed).lte(0)) continue;
            matches.set(id, { fee, signed:signed.toString() });
        } catch { /* Keep inconsistent legacy rows separate. */ }
    }
    return items.flatMap(item => {
        if (item.transferFeeParentId && matches.get(item.transferFeeParentId)?.fee === item) return [];
        const match = matches.get(item.id);
        if (!match) return [item];
        return [{ ...item, amount:new LedgerDecimal(item.amount).plus(match.signed).toString(), hidden:item.hidden || match.fee.hidden, accountFee:match.signed }];
    });
}

export function ledgerCategoryTotals(items: { type: number; cny: string | null; categoryId: string; primaryCategoryId: string; primaryCategory: string; title: string; excludeFromStatistics?: boolean }[], primary: boolean, type: number) {
    const groups = new Map<string, { id: string; name: string; amount: InstanceType<typeof LedgerDecimal> }>();
    for (const item of items) {
        if (item.excludeFromStatistics) continue;
        if (item.type !== type || item.cny === null) continue;
        const id = primary ? item.primaryCategoryId : item.categoryId;
        const name = primary ? item.primaryCategory : item.primaryCategoryId === item.categoryId ? item.title : `${item.primaryCategory} / ${item.title}`;
        const group = groups.get(id) || { id, name, amount: new LedgerDecimal(0) };
        group.amount = group.amount.plus(item.cny);
        groups.set(id, group);
    }
    const maximum = [...groups.values()].reduce((max, item) => LedgerDecimal.max(max, item.amount.abs()), new LedgerDecimal(1));
    return [...groups.values()].sort((a,b) => b.amount.abs().comparedTo(a.amount.abs())).map(item => ({ id: item.id, name: item.name, amount: item.amount.toString(), width: item.amount.abs().div(maximum).mul(100).toNumber() }));
}
