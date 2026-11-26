import { LedgerDecimal } from '@/lib/ledger-display.ts';

const CostDecimal = LedgerDecimal.clone({ precision: 160 });
export type CostEntry = 'total' | 'average';
export interface CryptoOpeningDraft { quantity: string; total: string; average: string; exchangeRate: string; source: CostEntry }
export function emptyCryptoOpening(): CryptoOpeningDraft {
    return { quantity: '', total: '', average: '', exchangeRate: '', source: 'total' };
}
function nonnegative(value: string): string {
    let text = value.trim();
    if (text.startsWith('.')) text = '0' + text;
    if (text.endsWith('.')) text = text.slice(0, -1);
    if (text.length > 60 || !/^\d+(\.\d{1,18})?$/.test(text)) throw Error('金额或数量须为普通数字，最多支持 18 位小数');
    return new CostDecimal(text).toFixed();
}
/** Keep the last edited cost field authoritative so repeating division never changes total cost. */
export function updateCryptoOpening(draft: CryptoOpeningDraft, field: 'quantity' | CostEntry, value: string): void {
    draft[field === 'total' ? 'total' : field === 'average' ? 'average' : 'quantity'] = value;
    if (field !== 'quantity') draft.source = field;
    const other = draft.source === 'total' ? 'average' : 'total';
    draft[other] = '';
    if (!draft.quantity.trim() || !draft[draft.source].trim()) return;
    try {
        const quantity = new CostDecimal(nonnegative(draft.quantity));
        if (!quantity.gt(0)) return;
        const cost = new CostDecimal(nonnegative(draft[draft.source]));
        draft[other] = (draft.source === 'total' ? cost.div(quantity) : cost.mul(quantity)).toDecimalPlaces(18).toFixed();
    } catch { /* Incomplete keypad input stays editable; submission performs validation. */ }
}
export function cryptoOpeningInput(instrumentId: string, draft: CryptoOpeningDraft, currency: 'CNY' | 'USD') {
    const quantity = nonnegative(draft.quantity.trim() || '0');
    const entered = draft[draft.source].trim();
    if (!entered) return { instrumentId, quantity, cost: null };
    if (!new CostDecimal(quantity).gt(0)) throw Error('填写持有成本时，持有数量必须大于零');
    const source = nonnegative(entered);
    const cost = draft.source === 'total' ? source : new CostDecimal(source).mul(quantity).toDecimalPlaces(18).toFixed();
    if (new CostDecimal(source).gt(0) && new CostDecimal(cost).isZero()) throw Error('计算后的成本小于支持的最小精度');
    nonnegative(cost);
    if (currency === 'USD' && !draft.exchangeRate.trim()) throw Error('请填写美元成本汇率');
    const costExchangeRate = currency === 'CNY' ? '1' : nonnegative(draft.exchangeRate);
    if (!new CostDecimal(costExchangeRate).gt(0)) throw Error('请填写大于零的美元成本汇率');
    return { instrumentId, quantity, cost, costExchangeRate };
}
