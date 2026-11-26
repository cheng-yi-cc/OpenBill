import { describe, expect, it } from 'vitest';
import { calendarBookDraft, calendarBookScope, toggleVisibleCalendarBook } from '../calendar-book-scope.ts';
import type { LedgerBook } from '@/models/book.ts';

const book = (id: string, hidden = false): LedgerBook => ({ id, hidden, name: id, icon: '', displayOrder: 0, isDefault: false, archived: false, showTransfers: true, showInvestments: true });

describe('日历账本范围', () => {
    it('保留当前隐藏账本，但全选可见账本不会扩大为无筛选', () => {
        const books = [book('visible'), book('hidden', true)];
        expect(calendarBookDraft(['hidden'], books)).toEqual(['hidden']);
        expect(calendarBookScope(['hidden'], books)).toEqual(['hidden']);
        expect(calendarBookDraft([], books)).toEqual(['visible']);
        expect(calendarBookScope(['visible'], books)).toEqual(['visible']);
        expect(calendarBookScope([], books)).toBeNull();
        expect(toggleVisibleCalendarBook(['hidden'], books, 'visible', true)).toEqual(['visible']);
        expect(toggleVisibleCalendarBook(['hidden', 'visible'], books, 'visible', false)).toEqual([]);
    });
    it('全部账本可用时保持无筛选语义，拒绝无效和超限的筛选', () => {
        const books = [book('one'), book('two')];
        expect(calendarBookScope(['two', 'one'], books)).toEqual([]);
        expect(calendarBookScope(['missing'], books)).toBeNull();
        expect(calendarBookScope([], [book('hidden', true)])).toBeNull();
        expect(calendarBookScope(['hidden'], [book('hidden', true)])).toEqual(['hidden']);
        const many = Array.from({ length: 102 }, (_, index) => book(String(index), index === 101));
        expect(calendarBookScope(many.slice(0, 101).map(item => item.id), many)).toBeNull();
    });
});
