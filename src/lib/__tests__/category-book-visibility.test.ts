import { describe, expect, it } from 'vitest';
import { categoryAvailableInBook } from '../category-book-visibility.ts';
import type { LedgerBook } from '@/models/book.ts';

const book = (id: string, hiddenCategoryIds?: string[]): LedgerBook => ({ id, name: id, icon: '', displayOrder: 0, isDefault: false, archived: false, hidden: false, showTransfers: true, showInvestments: true, hiddenCategoryIds });
const category = (id: string, bookIds: string[] = [], hidden = false) => ({ id, bookIds, hidden });

describe('账本分类显示范围', () => {
    it('独立隐藏不改变其他账本、之后新增分类或旧分类白名单', () => {
        const first = book('a', ['old']), other = book('b');
        expect(categoryAvailableInBook(category('old'), 'a', first)).toBe(false);
        expect(categoryAvailableInBook(category('old'), 'b', other)).toBe(true);
        expect(categoryAvailableInBook(category('new'), 'a', first)).toBe(true);
        expect(categoryAvailableInBook(category('limited', ['b']), 'a', first)).toBe(false);
        expect(categoryAvailableInBook(category('limited', ['b']), 'b', other)).toBe(true);
        expect(categoryAvailableInBook(category('global-hidden', [], true), 'b', other)).toBe(false);
    });
    it('父子分类分别保存状态，旧账本没有新字段时沿用原范围', () => {
        const selected = book('a', ['parent']);
        expect(categoryAvailableInBook(category('parent'), 'a', selected)).toBe(false);
        expect(categoryAvailableInBook(category('child'), 'a', selected)).toBe(true);
        expect(categoryAvailableInBook(category('child'), 'a', book('a', ['child']))).toBe(false);
        expect(categoryAvailableInBook(category('parent'), 'a', book('a'))).toBe(true);
    });
});
