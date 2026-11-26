import { describe, expect, it } from 'vitest';
import { depositAccountSummary } from '@/lib/deposit-account-summary.ts';
import type { FixedDeposit } from '@/lib/asset-tools.ts';

const deposit = (patch: Partial<FixedDeposit> = {}): FixedDeposit => ({id:'d1',accountId:'1',bookId:'b',categoryId:'c',currency:'CNY',principal:'100',annualRate:'2',term:2,unit:'day',startDate:'2026-11-10',maturityDate:'2026-11-12',expectedInterest:'0.01',timeZone:'Asia/Shanghai',note:'',settled:false,transactionId:'',closed:false,...patch});

describe('账户定存汇总', () => {
    it('按有效定存和最近到期日汇总，保留负活期且不增加本金', () => {
        const rows = [deposit(), deposit({id:'d2',principal:'50',expectedInterest:'0.02'}), deposit({id:'d3',principal:'20',expectedInterest:'0.03',maturityDate:'2026-11-20'}), deposit({id:'closed',principal:'999',closed:true}), deposit({id:'future',principal:'999',startDate:'2026-11-11'}), deposit({id:'other',accountId:'2',principal:'999'})];
        expect(depositAccountSummary(rows, '1', 'CNY', '100', '2026-11-10')).toEqual({principal:'170',available:'-70',interest:'0.06',nextDate:'2026-11-12',nextInterest:'0.03'});
        expect(depositAccountSummary([deposit({settled:true})], '1', 'CNY', '100', '2026-11-10')).toBeNull();
        expect(() => depositAccountSummary([deposit({currency:'USD'})], '1', 'CNY', '100', '2026-11-10')).toThrow();
    });
});
