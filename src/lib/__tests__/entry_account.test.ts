import { describe, expect, it } from 'vitest';
import { resolveEntryAccount, resolveRouteEntryAccount } from '../entry-account';

describe('entry account precedence', () => {
    const accounts = [{ id: '1', currency: 'CNY' }, { id: '2', currency: 'CNY' }, { id: '3', currency: 'USD' }];
    it('requires the route account to exist instead of silently falling back to another account', () => {
        expect(resolveRouteEntryAccount('2', accounts)).toBe('2');
        expect(() => resolveRouteEntryAccount('404', accounts)).toThrow('指定账户不可用');
    });
    it('retains an explicitly chosen account when a different category suggests another', () => {
        expect(resolveEntryAccount('1', true, 'category', 'food', { food: '2' }, '2', accounts, false)).toBe('1');
        expect(resolveEntryAccount('2', true, 'last', 'food', {}, '1', accounts, false)).toBe('2');
    });
    it('uses category and last-account suggestions when no account was explicitly chosen', () => {
        expect(resolveEntryAccount('1', false, 'category', 'food', { food: '2' }, '', accounts, false)).toBe('2');
        expect(resolveEntryAccount('1', false, 'last', 'food', {}, '2', accounts, false)).toBe('2');
        expect(resolveEntryAccount('1', false, 'category', 'food', { food: '3' }, '', accounts, false)).toBe('1');
        expect(resolveEntryAccount('1', false, 'category', 'food', { food: '3' }, '', accounts, true)).toBe('3');
        expect(resolveEntryAccount('1', false, 'category', 'food', { food: '404' }, '', accounts, true)).toBe('1');
    });
});
