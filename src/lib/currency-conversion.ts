import Decimal from 'decimal.js';
import moment from 'moment-timezone';
import { TRANSACTION_MAX_AMOUNT, TRANSACTION_MIN_AMOUNT } from '@/consts/transaction.ts';
import type { LatestExchangeRateResponse } from '@/models/exchange_rate.ts';
import type { WealthSummary } from '@/models/investment.ts';
import { usdFX } from '@/lib/wallet-entry.ts';

export const EntryMoney = Decimal.clone({ precision: 40 });
export type SupportedEntryCurrency = 'CNY' | 'USD';

export interface ResolvedUsdCnyRate {
    rate: string;
    date: string;
    source: string;
}

export function isToggleableAccountCurrency(currency: string): currency is SupportedEntryCurrency {
    return currency === 'CNY' || currency === 'USD';
}

export function toggleAccountCurrency(currency: SupportedEntryCurrency): SupportedEntryCurrency {
    return currency === 'CNY' ? 'USD' : 'CNY';
}

export function formatEntryCurrencySymbol(currency: string): string {
    if (currency === 'CNY') return '¥';
    if (currency === 'USD') return '$';
    return currency;
}

export function sanitizeDecimalInput(raw: string): string {
    const trimmed = raw.trim().replace(/\.$/, '');
    if (!trimmed || trimmed === '-') return '0';
    return trimmed;
}

function formatExactOrHighPrecisionRate(ratio: InstanceType<typeof EntryMoney>, numerator: InstanceType<typeof EntryMoney>, denominator: InstanceType<typeof EntryMoney>): string {
    for (let places = 0; places <= 12; places++) {
        const candidate = ratio.toDecimalPlaces(places, Decimal.ROUND_HALF_UP);
        if (candidate.mul(denominator).eq(numerator)) {
            return candidate.toFixed();
        }
    }
    return ratio.toDecimalPlaces(12, Decimal.ROUND_HALF_UP).toString();
}

export function resolveUsdCnyRate(summary?: WealthSummary, exchangeRates?: LatestExchangeRateResponse, timeZone = 'Asia/Shanghai'): ResolvedUsdCnyRate | null {
    const marketFx = usdFX(summary);
    if (marketFx) {
        return {
            rate: marketFx.rate,
            date: marketFx.date || '',
            source: marketFx.source || 'marketquotes'
        };
    }

    if (!exchangeRates?.exchangeRates?.length) {
        return null;
    }

    const byCurrency: Record<string, string> = {};
    if (exchangeRates.baseCurrency) {
        byCurrency[exchangeRates.baseCurrency] = '1';
    }
    for (const item of exchangeRates.exchangeRates) {
        if (item.currency && item.rate) {
            byCurrency[item.currency] = item.rate;
        }
    }

    const usdRaw = byCurrency['USD'];
    const cnyRaw = byCurrency['CNY'];
    if (!usdRaw || !cnyRaw) {
        return null;
    }

    try {
        const usdDec = new EntryMoney(usdRaw);
        const cnyDec = new EntryMoney(cnyRaw);
        if (!usdDec.isFinite() || !cnyDec.isFinite() || usdDec.lte(0) || cnyDec.lte(0)) {
            return null;
        }
        const rate = exchangeRates.baseCurrency === 'USD'
            ? cnyDec.toFixed()
            : formatExactOrHighPrecisionRate(cnyDec.div(usdDec), cnyDec, usdDec);
        const date = exchangeRates.updateTime
            ? moment.unix(exchangeRates.updateTime).tz(timeZone).format('YYYY-MM-DD')
            : '';
        return {
            rate,
            date,
            source: exchangeRates.dataSource || 'exchange_rates'
        };
    } catch {
        return null;
    }
}

export function convertEntryAmount(
    rawAmount: string,
    entryCurrency: SupportedEntryCurrency,
    accountCurrency: SupportedEntryCurrency,
    usdCnyRate: string
): { accountCents: number; accountAmount: string; originalAmount: string } | null {
    const cleanAmount = sanitizeDecimalInput(rawAmount);
    const cleanRate = usdCnyRate.trim();
    if (!cleanRate) return null;

    try {
        const amountDec = new EntryMoney(cleanAmount);
        const rateDec = new EntryMoney(cleanRate);
        if (!amountDec.isFinite() || !rateDec.isFinite() || rateDec.lte(0)) {
            return null;
        }
        if (amountDec.isZero()) {
            return { accountCents: 0, accountAmount: '0', originalAmount: '0' };
        }
        if (entryCurrency !== accountCurrency && amountDec.lt(0)) {
            return null;
        }

        const rawAccountDec = entryCurrency === accountCurrency
            ? amountDec
            : entryCurrency === 'USD'
                ? amountDec.mul(rateDec)
                : amountDec.div(rateDec);

        const roundedAccountDec = rawAccountDec.toDecimalPlaces(2, Decimal.ROUND_HALF_UP);
        const accountCentsDec = roundedAccountDec.mul(100);
        if (accountCentsDec.lt(TRANSACTION_MIN_AMOUNT) || accountCentsDec.gt(TRANSACTION_MAX_AMOUNT)) {
            return null;
        }

        return {
            accountCents: accountCentsDec.toNumber(),
            accountAmount: roundedAccountDec.toString(),
            originalAmount: amountDec.toDecimalPlaces(2, Decimal.ROUND_HALF_UP).toString()
        };
    } catch {
        return null;
    }
}
