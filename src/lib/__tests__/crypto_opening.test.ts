import { describe, expect, it } from 'vitest';
import { cryptoOpeningInput, emptyCryptoOpening, updateCryptoOpening } from '../crypto-opening.ts';

describe('initial crypto holding costs', () => {
    it('retains the total when average division repeats and quantity changes', () => {
        const draft = emptyCryptoOpening();
        updateCryptoOpening(draft, 'quantity', '3');
        updateCryptoOpening(draft, 'total', '1');
        expect(draft.average).toBe('0.333333333333333333');
        updateCryptoOpening(draft, 'quantity', '7');
        expect(cryptoOpeningInput('btc', draft, 'CNY').cost).toBe('1');
        updateCryptoOpening(draft, 'average', '0.123456789123456789');
        expect(draft.total).toBe('0.864197523864197523');
        updateCryptoOpening(draft, 'quantity', '10');
        expect(cryptoOpeningInput('btc', draft, 'CNY').cost).toBe('1.23456789123456789');
    });
    it('keeps unknown distinct from explicit zero, and requires USD cost FX', () => {
        const draft = emptyCryptoOpening();
        updateCryptoOpening(draft, 'quantity', '0.5');
        expect(cryptoOpeningInput('btc', draft, 'USD').cost).toBeNull();
        updateCryptoOpening(draft, 'total', '0');
        expect(() => cryptoOpeningInput('btc', draft, 'USD')).toThrow();
        draft.exchangeRate = '7.123456789';
        expect(cryptoOpeningInput('btc', draft, 'USD')).toMatchObject({ cost: '0', costExchangeRate: '7.123456789' });
        updateCryptoOpening(draft, 'average', '');
        expect(draft.total).toBe('');
        expect(cryptoOpeningInput('btc', draft, 'USD').cost).toBeNull();
    });
    it('rejects invalid or absent quantities and hostile decimal inputs without treating them as unknown', () => {
        for (const value of ['1e9', '-1', '9'.repeat(129), '0.0000000000000000001']) {
            const draft = emptyCryptoOpening();
            updateCryptoOpening(draft, 'quantity', '1');
            updateCryptoOpening(draft, 'average', value);
            expect(() => cryptoOpeningInput('btc', draft, 'CNY')).toThrow();
        }
        const draft = emptyCryptoOpening();
        updateCryptoOpening(draft, 'total', '20');
        expect(() => cryptoOpeningInput('btc', draft, 'CNY')).toThrow();
    });
});
