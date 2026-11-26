import { describe, expect, it } from 'vitest';
import { projectRefundEntry } from '../refund-ledger.ts';
import { ledgerTotals } from '../ledger-display.ts';
import { budgetReports, groupReport } from '../statistics-report.ts';
import { filterStatisticsEntries } from '../statistics-links.ts';
import type { LedgerEntry } from '../mobile-ledger.ts';
import type { RefundProjection } from '@/models/transaction.ts';

const entry = (patch: Partial<LedgerEntry> = {}): LedgerEntry => ({
    id:'1', type:3, day:'2026-11-06', time:1793952000, title:'午餐', account:'现金', currency:'CNY', amount:'12', cny:'12', hidden:false,
    categoryId:'lunch', primaryCategoryId:'food', primaryCategory:'餐饮', comment:'', tags:[], bookId:'book-a', accountId:'cash',
    destinationAccountId:'', tagIds:[], investment:false, ...patch
});
const categories = { income: {id:'income',name:'其他收入',parentId:'income-parent'}, 'income-parent': {id:'income-parent',name:'收入',parentId:'0'} };
const excess: RefundProjection = {kind:'cash',expenseId:'1',refundId:'r',offsetAmount:'12.00',excessAmount:'1.00',excess:{categoryId:'income',bookId:'book-b',comment:'退款差额',tagIds:['tag'],excludeFromStatistics:false,excludeFromBudget:false}};

describe('refund reporting projections', () => {
    it('keeps the original day and independent budget flag while preserving the cash fact', () => {
        const raw = entry({excludeFromStatistics:true, excludeFromBudget:false});
        const [net] = projectRefundEntry(raw, {kind:'expense',expenseId:'1',netExpense:'7.00',refundedAmount:'5.00'}, categories, {}, {});
        expect(net).toMatchObject({amount:'7',cny:'7',originalAmount:'12',refundedAmount:'5',day:'2026-11-06'});
        expect(raw.amount).toBe('12');
        expect(ledgerTotals([net!])).toMatchObject({expense:'0'});
        const rule = {id:'b',bookId:'book-a',categoryId:'0',name:'预算',amount:'100',startDate:'2026-11-01',endDate:'2026-11-30',kind:'monthly' as const,repeat:false,revision:'1'};
        expect(budgetReports([rule],[net!],'2026-11',{carrySurplus:false,carryDeficit:false})[0]).toMatchObject({spent:'7',remaining:'93'});
    });
    it('places excess on the actual refund day, uses its own book and flags, and does not add a cash account', () => {
        const cash = entry({id:'2',day:'2026-12-02',time:1796212800,amount:'-13',cny:'-13',sourceAccountName:'真实到账账户',excludeFromStatistics:true,excludeFromBudget:true});
        const [income] = projectRefundEntry(cash,excess,categories,{tag:'标签'},{'book-b':'旅行'});
        expect(income).toMatchObject({type:2,amount:'1',cny:'1',day:'2026-12-02',bookId:'book-b',account:'无账户',accountId:'refund_unassigned',tagIds:['tag'],tags:['标签'],refundExpenseId:'1',excludeFromStatistics:false});
        expect(ledgerTotals([income!])).toMatchObject({income:'1'});
        expect(groupReport([income!],'category')[0]).toMatchObject({id:'income-parent',income:'1'});
        expect(filterStatisticsEntries([income!,entry()],{accountId:'refund_unassigned'},{}).map(row=>row.id)).toEqual(['2']);
        expect(cash.amount).toBe('-13');
        expect(projectRefundEntry(cash,{...excess,excessAmount:'0.00'},categories,{},{})).toEqual([]);
    });
    it('preserves unknown FX and rejects incomplete or inconsistent projections', () => {
        const foreign = entry({currency:'USD',cny:null});
        expect(projectRefundEntry(foreign,{kind:'expense',expenseId:'1',netExpense:'7.00',refundedAmount:'5.00'},categories,{}, {})[0]?.cny).toBeNull();
        expect(projectRefundEntry({...foreign,amount:'-13'},excess,categories,{}, {})[0]?.cny).toBeNull();
        expect(() => projectRefundEntry(entry(),{kind:'expense',expenseId:'1',netExpense:'13',refundedAmount:'1'},categories,{},{})).toThrow();
        expect(() => projectRefundEntry(entry({amount:'-13'}),{...excess,excess:undefined},categories,{},{})).toThrow();
    });
});
