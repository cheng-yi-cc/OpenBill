import type { CalendarEvent } from '@/lib/calendar-events.ts';
import type { CreditReport } from '@/lib/credit-accounts.ts';
import { LedgerDecimal } from '@/lib/ledger-display.ts';

// Calendar rows are projections of existing statements, not new ledger facts.
export function calendarCreditEvents(reports: CreditReport[], month: string): CalendarEvent[] {
    return reports.flatMap(report => report.statements.flatMap(row => {
        const amount = row.projectedRemaining ?? row.remaining;
        const hasProjectedFee = new LedgerDecimal(amount).gt(new LedgerDecimal(row.remaining));
        if (!row.dueDate.startsWith(`${month}-`) || !new LedgerDecimal(amount).gt(0)) return [];
        return [{
            id: `credit:${report.accountId}:${row.month}`,
            accountId: report.accountId,
            accountName: report.accountName,
            bookId: '',
            currency: report.currency,
            kind: 'repayment' as const,
            date: row.dueDate,
            amount,
            note: `${row.endDate.slice(0, 7).replace('-', '年')}月账单${row.unbilled ? ' · 未出账' : ''}${row.projectedRemaining !== undefined && hasProjectedFee ? ' · 预计待还，含未入账分期费用' : ''}`,
            completed: false,
        }];
    }));
}
