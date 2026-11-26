import { describe, expect, it } from 'vitest';
import { ordinaryTransferCategory } from '../ledger-debt.ts';

describe('债务切回普通转账', () => {
    const categories = [{ id: '1', name: '借入' }, { id: '2', name: '账户互转' }, { id: '3', name: '还款' }, { id: '4', name: '其他转账' }];
    it('保留原普通分类，不沿用债务分类', () => {
        expect(ordinaryTransferCategory(categories, '4')).toBe('4');
        expect(ordinaryTransferCategory(categories, '1')).toBe('2');
        expect(ordinaryTransferCategory(categories, 'gone')).toBe('2');
    });
    it('没有可用普通分类时不伪造分类', () => {
        expect(ordinaryTransferCategory([{ id: '1', name: '借入' }], '')).toBe('');
    });
});
