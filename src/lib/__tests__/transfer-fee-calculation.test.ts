import { describe, expect, it } from 'vitest';
import { transferPrincipalFromTotal } from '../transfer-fee-calculation.ts';

describe('含手续费扣款总额', () => {
    it('使用十进制精确计算转账本金，手续费和优惠均适用', () => {
        expect(transferPrincipalFromTotal('2.00', '0.30')).toBe(170);
        expect(transferPrincipalFromTotal('2.00', '-0.30')).toBe(230);
        expect(transferPrincipalFromTotal('0.30', '0.10')).toBe(20);
        expect(transferPrincipalFromTotal('9999999999999.99', '0.01')).toBe(999999999999998);
    });
    it('拒绝超限、非正本金、非正扣款与不合法精度', () => {
        for (const [total, fee] of [['2', '2'], ['2', '2.01'], ['0', '-1'], ['9999999999999.99', '-0.01'], ['1.001', '0'], ['1', '0.001'], ['1e2', '0'], ['', '0']]) {
            expect(() => transferPrincipalFromTotal(total!, fee!)).toThrow();
        }
    });
});
