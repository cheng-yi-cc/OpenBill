import { describe, expect, it } from 'vitest';
import { convertEntryAmount, resolveUsdCnyRate, toggleAccountCurrency } from '../currency-conversion.ts';

describe('cross-currency entry conversion', () => {
    it('toggles strictly between CNY and USD and converts with decimal precision', () => {
        expect(toggleAccountCurrency('CNY')).toBe('USD');
        expect(toggleAccountCurrency('USD')).toBe('CNY');

        // Exact terminating product stays exact
        const usdToCny = convertEntryAmount('10', 'USD', 'CNY', '7.25');
        expect(usdToCny).toEqual({
            accountCents: 7250,
            accountAmount: '72.5',
            originalAmount: '10'
        });

        // CNY -> USD uses high-precision division before rounding to integer cents
        const cnyToUsd = convertEntryAmount('100', 'CNY', 'USD', '7.25');
        expect(cnyToUsd).toEqual({
            accountCents: 1379,
            accountAmount: '13.79',
            originalAmount: '100'
        });
    });

    it('resolves USD/CNY rate from WealthSummary first, falling back to exchangeRatesStore without rounding exact ratios', () => {
        const fromWealth = resolveUsdCnyRate({
            fxRates: [{ base: 'USD', quote: 'CNY', rate: '7.2435', source: 'ECB', date: '2026-11-27', state: 'live', receivedAt: 1700000000 }],
            baseCurrency: 'CNY',
            netAssets: null,
            valuedAssets: '0',
            cashAssets: '0',
            investmentValue: '0',
            liabilities: '0',
            missingPrices: 0,
            stalePrices: 0,
            unrealizedPnl: null,
            realizedPnl: null,
            costComplete: true,
            cashAccounts: [],
            positions: []
        });
        expect(fromWealth).toEqual({ rate: '7.2435', date: '2026-11-27', source: 'ECB' });

        // Exact ratio from EUR-based ECB table (e.g. 7.8 / 1.04 = 7.5 exact) must not carry trailing zeros
        const fromEcb = resolveUsdCnyRate(undefined, {
            dataSource: 'euro_central_bank',
            referenceUrl: '',
            updateTime: 1700000000,
            baseCurrency: 'EUR',
            exchangeRates: [
                { currency: 'USD', rate: '1.04' },
                { currency: 'CNY', rate: '7.8' }
            ]
        }, 'Asia/Shanghai');
        expect(fromEcb?.rate).toBe('7.5');
        expect(fromEcb?.source).toBe('euro_central_bank');

        expect(resolveUsdCnyRate(undefined, undefined)).toBeNull();
    });
});
