import { describe, expect, it } from 'vitest';
import { groupAccountTransferFees, ledgerAccountFlowTotals } from '../ledger-display.ts';

const parent = { id:'p', type:4, day:'2026-11-14', accountId:'a', currency:'CNY', amount:'2', hidden:false, transferDirection:'out' as const, transferFeeAmount:'0.30' };
const fee = { id:'f', type:3, day:'2026-11-14', accountId:'a', currency:'CNY', amount:'0.30', hidden:false, transferFeeParentId:'p' };

describe('账户转账与手续费投影', () => {
    it('将已完整加载的手续费并入转出，并保持净流出', () => {
        const projected = groupAccountTransferFees([fee, parent]);
        expect(projected).toHaveLength(1);
        expect(projected[0]).toMatchObject({ id:'p', amount:'2.3', accountFee:'0.3' });
        expect(ledgerAccountFlowTotals(projected)).toMatchObject({ income:'0', expense:'2.3' });
    });
    it('优惠抵减转出，而不是当作账户额外收入', () => {
        const projected = groupAccountTransferFees([{...fee,type:2}, {...parent,transferFeeAmount:'-0.30'}]);
        expect(projected[0]).toMatchObject({ amount:'1.7', accountFee:'-0.3' });
        expect(ledgerAccountFlowTotals(projected)).toMatchObject({ income:'0', expense:'1.7' });
    });
    it('分页未齐、重复和不一致时保留事实行，不猜测或丢失金额', () => {
        expect(groupAccountTransferFees([fee])).toEqual([fee]);
        expect(groupAccountTransferFees([parent])).toEqual([parent]);
        expect(groupAccountTransferFees([fee, {...fee,id:'f2'}, parent])).toHaveLength(3);
        expect(groupAccountTransferFees([{...fee,currency:'USD'}, parent])).toHaveLength(2);
        expect(groupAccountTransferFees([{...fee,amount:'0.31'}, parent])).toHaveLength(2);
        expect(groupAccountTransferFees([{...fee,type:2,amount:'2.30'}, {...parent,transferFeeAmount:'-2.30'}])).toHaveLength(2);
    });
    it('一方隐藏金额时合并行仍隐藏', () => {
        expect(groupAccountTransferFees([{...fee,hidden:true}, parent])[0]?.hidden).toBe(true);
    });
});
