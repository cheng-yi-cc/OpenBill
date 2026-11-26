import { LedgerDecimal } from '@/lib/ledger-display.ts';
import type { FixedDeposit } from '@/lib/asset-tools.ts';

// Display projection only: principal is already included in the account balance.
export function depositAccountSummary(deposits: FixedDeposit[], accountId: string, currency: string, balance: string, today: string) {
    const active = deposits.filter(d => d.accountId === accountId && !d.closed && !d.settled && d.startDate <= today && d.maturityDate > today);
    if (active.some(d => d.currency !== currency)) throw new Error('定存币种与账户不一致，请检查定存记录');
    if (!active.length) return null;
    const principal = active.reduce((sum, d) => sum.plus(d.principal), new LedgerDecimal(0));
    const interest = active.reduce((sum, d) => sum.plus(d.expectedInterest), new LedgerDecimal(0));
    const nextDate = active.map(d => d.maturityDate).sort()[0]!;
    const nextInterest = active.filter(d => d.maturityDate === nextDate).reduce((sum, d) => sum.plus(d.expectedInterest), new LedgerDecimal(0));
    return { principal: principal.toString(), available: new LedgerDecimal(balance).minus(principal).toString(), interest: interest.toString(), nextDate, nextInterest: nextInterest.toString() };
}
