import { LedgerDecimal } from '@/lib/ledger-display.ts';

// 仅用于展示汇总；未知明细仍保持未知，并随已知小计传递缺失数量。
export interface AmountSummary {
    value: string | null;
    known: number;
    missing: number;
}

export function summarizeAmounts(values: (string | null | undefined)[]): AmountSummary {
    let total = new LedgerDecimal(0), known = 0, missing = 0;
    for (const value of values) {
        if (value == null || value.trim() === '') {
            missing++;
        } else {
            total = total.plus(value);
            known++;
        }
    }
    return { value: known || !missing ? total.toString() : null, known, missing };
}

export function combineAmountSummaries(summaries: AmountSummary[]): AmountSummary {
    let total = new LedgerDecimal(0), known = 0, missing = 0;
    for (const summary of summaries) {
        if (summary.value != null) total = total.plus(summary.value);
        known += summary.known;
        missing += summary.missing;
    }
    return { value: known || !missing ? total.toString() : null, known, missing };
}
