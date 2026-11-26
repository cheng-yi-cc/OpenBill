import { effectScope, nextTick, ref } from 'vue';
import { describe, expect, it, vi } from 'vitest';
import { useMobileLedger } from '@/lib/mobile-ledger.ts';
import { calendarCreditEvents } from '@/lib/calendar-credit.ts';
import type { CreditReport, CreditStatement } from '@/lib/credit-accounts.ts';

const mocks = vi.hoisted(() => ({ getAllTransactions: vi.fn() }));
vi.mock('@/lib/services.ts', () => ({ default: { getAllTransactions: mocks.getAllTransactions } }));
vi.mock('@/stores/account.ts', () => ({ useAccountsStore: () => ({ allAccountsMap: {}, loadAllAccounts: async () => {} }) }));
vi.mock('@/stores/transactionCategory.ts', () => ({ useTransactionCategoriesStore: () => ({ allTransactionCategoriesMap: {}, loadAllCategories: async () => {} }) }));
vi.mock('@/stores/transactionTag.ts', () => ({ useTransactionTagsStore: () => ({ allTransactionTagsMap: {}, loadAllTags: async () => {} }) }));
vi.mock('@/stores/books.ts', () => ({ useBooksStore: () => ({ allBooks: [], selectedBookIds: ['home-book'], loadBooks: async () => {}, captureReportScope: () => () => true }) }));
vi.mock('@/stores/ledgerScope.ts', () => ({ useLedgerScopeStore: () => ({ timeZone: 'UTC', accountId: '', categoryId: '', tagId: '', type: 0 }) }));
vi.mock('@/stores/user.ts', () => ({ useUserStore: () => ({ currentUserBasicInfo: { username: 'qa' } }) }));

function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (reason: Error) => void;
    const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
    return { promise, resolve, reject };
}
const response = (items: unknown[]) => ({ data: { success: true, result: items } });

describe('calendar credit statement projections', () => {
    it('shows a separately labelled forecast even when principal is already repaid', () => {
        const statement: CreditStatement = { month: '2026-12', startDate: '2026-11-01', endDate: '2026-11-30', statementDate: '2026-12-01', dueDate: '2026-12-09', charges: '1.00', fees: '0', futureFees: '0.10', remaining: '0.00', projectedRemaining: '0.10', unbilled: true, transactionIds: [] };
        const report: CreditReport = { accountId: 'card', accountName: '测试信用卡', currency: 'CNY', outstanding: '0.00', statements: [statement], annualFee: '0', annualDate: '', annualSpend: '0', annualCount: 0, annualWaived: false };
        expect(calendarCreditEvents([report], '2026-12')).toMatchObject([{ amount: '0.10', note: '2026年11月账单 · 未出账 · 预计待还，含未入账分期费用' }]);
        statement.projectedRemaining = '0.00';
        expect(calendarCreditEvents([report], '2026-12')).toEqual([]);
    });
    it('shows remaining debt in the due month, including an unbilled statement, and removes it after repayment', () => {
        const statement: CreditStatement = { month: '2026-12', startDate: '2026-11-01', endDate: '2026-11-30', statementDate: '2026-12-01', dueDate: '2026-12-09', charges: '3.00', fees: '0', futureFees: '2.00', remaining: '3.00', unbilled: true, transactionIds: ['expense'] };
        const report: CreditReport = { accountId: 'card', accountName: '测试信用卡', currency: 'CNY', outstanding: '3.00', statements: [statement], annualFee: '0', annualDate: '', annualSpend: '0', annualCount: 0, annualWaived: false };
        expect(calendarCreditEvents([report], '2026-11')).toEqual([]);
        expect(calendarCreditEvents([report], '2026-12')).toMatchObject([{ accountId: 'card', date: '2026-12-09', amount: '3.00', note: '2026年11月账单 · 未出账' }]);
        statement.remaining = '0.00';
        expect(calendarCreditEvents([report], '2026-12')).toEqual([]);
    });
});

describe('independent calendar book scope', () => {
    it('filters actual ledger entries by the explicit scope, independently of the home scope', async () => {
        const bill = (id: string, bookId: string, amount: number) => ({
            id, bookId, type: 3, sourceAmount: amount, time: 100, sourceAccountId: 'cash',
            sourceAccount: { name: '现金', currency: 'CNY' }, categoryId: 'category', tagIds: [], comment: ''
        });
        mocks.getAllTransactions.mockReset().mockResolvedValue(response([bill('1', 'home-book', 1200), bill('2', 'calendar-book', 500)]));
        const selected = ref(['calendar-book']);
        const scope = effectScope();
        const ledger = scope.run(() => useMobileLedger({ applyFilters: () => false, bookIds: () => selected.value }))!;
        await ledger.load(0, 100);
        expect(ledger.entries.value.map(item => [item.bookId, item.amount])).toEqual([['calendar-book', '5']]);
        selected.value = [];
        await ledger.load(0, 100);
        expect(ledger.entries.value.map(item => item.bookId).sort()).toEqual(['calendar-book', 'home-book']);
        scope.stop();
    });

    it('invalidates an old request immediately when the selected books change', async () => {
        const oldResponse = deferred<ReturnType<typeof response>>();
        mocks.getAllTransactions.mockReset().mockReturnValueOnce(oldResponse.promise);
        const selected = ref(['book-a']);
        const scope = effectScope();
        const ledger = scope.run(() => useMobileLedger({ applyFilters: () => false, bookIds: () => selected.value }))!;
        const old = ledger.load(0, 100);
        selected.value = ['book-b'];
        oldResponse.resolve(response([{}]));
        await old;
        expect(ledger.error.value).toBe('');
        expect(ledger.entries.value).toEqual([]);
        expect(ledger.loading.value).toBe(true);
        scope.stop();
    });

    it('discards an old response after switching away and back while keeping the latest result', async () => {
        const first = deferred<ReturnType<typeof response>>();
        const second = deferred<ReturnType<typeof response>>();
        const third = deferred<ReturnType<typeof response>>();
        mocks.getAllTransactions.mockReset().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockReturnValueOnce(third.promise);
        const selected = ref(['book-a']);
        const scope = effectScope();
        const ledger = scope.run(() => useMobileLedger({ applyFilters: () => false, bookIds: () => selected.value }))!;
        const old = ledger.load(0, 100);
        selected.value = ['book-b'];
        const middle = ledger.load(0, 100);
        selected.value = ['book-a'];
        const current = ledger.load(0, 100);
        third.resolve(response([]));
        await current;
        expect(ledger.loading.value).toBe(false);
        expect(ledger.entries.value).toEqual([]);
        first.resolve(response([{}]));
        second.reject(new Error('obsolete'));
        await old;
        await middle;
        await nextTick();
        expect(ledger.entries.value).toEqual([]);
        expect(ledger.error.value).toBe('');
        expect(ledger.loading.value).toBe(false);
        scope.stop();
    });
});
