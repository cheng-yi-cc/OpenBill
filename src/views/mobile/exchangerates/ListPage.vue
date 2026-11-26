<template>
    <f7-page class="cy-mobile-surface" ptr @ptr:refresh="reload">
        <f7-navbar>
            <f7-nav-left :class="{ 'disabled': loading }" back-link="返回"></f7-nav-left>
            <f7-nav-title title="汇率数据"></f7-nav-title>
            <f7-nav-right :class="{ 'disabled': loading }">
                <f7-link icon-f7="ellipsis" aria-label="更多" @click="showMoreActionSheet = true"></f7-link>
            </f7-nav-right>
        </f7-navbar>

        <f7-list strong inset dividers class="margin-vertical-half" v-if="exchangeRatesData && exchangeRatesData.exchangeRates && exchangeRatesData.exchangeRates.length">
            <f7-list-item
                class="list-item-with-header-and-title list-item-no-item-after"
                link="#"
                header="基准货币"
                @click="showBaseCurrencyPopup = true"
            >
                <template #title>
                    <div class="no-padding no-margin">
                        <span>{{ getCurrencyName(baseCurrency) }}&nbsp;</span>
                        <small class="smaller">{{ baseCurrency }}</small>
                    </div>
                </template>
                <list-item-selection-popup value-type="item"
                                           key-field="currencyCode" value-field="currencyCode"
                                           title-field="currencyDisplayName" after-field="currencyCode"
                                           title="基准货币"
                                           :enable-filter="true"
                                           filter-placeholder="搜索币种或代码"
                                           filter-no-items-text="没有匹配的货币"
                                           :items="sortedExchangeRates"
                                           v-model:show="showBaseCurrencyPopup"
                                           v-model="baseCurrency">
                </list-item-selection-popup>
            </f7-list-item>
            <f7-list-item
                class="currency-base-amount"
                link="#" no-chevron
                :class="baseAmountFontSizeClass"
                header="基准金额"
                :title="displayBaseAmount"
                @click="showBaseAmountSheet = true"
            >
                <number-pad-sheet :min-value="TRANSACTION_MIN_AMOUNT"
                                  :max-value="TRANSACTION_MAX_AMOUNT"
                                  :currency="baseCurrency"
                                  v-model:show="showBaseAmountSheet"
                                  v-model="baseAmount"
                ></number-pad-sheet>
            </f7-list-item>
        </f7-list>

        <div v-if="exchangeRatesData && exchangeRatesData.exchangeRates && exchangeRatesData.exchangeRates.length" class="fx-search-wrap">
            <label class="fx-search-box">
                <f7-icon f7="search" />
                <input v-model="searchQuery" type="search" placeholder="搜索币种或代码（如 USD、日元）" aria-label="搜索汇率货币" />
                <button v-if="searchQuery" type="button" aria-label="清空搜索" @click="searchQuery = ''"><f7-icon f7="xmark_circle_fill" /></button>
            </label>
            <p class="cy-muted fx-hint">常用及已使用币种已置顶，点按任意币种可设为基准货币</p>
        </div>

        <f7-list strong inset dividers class="margin-vertical-half" v-if="!exchangeRatesData || !exchangeRatesData.exchangeRates || !exchangeRatesData.exchangeRates.length">
            <f7-list-item title="暂无汇率数据"></f7-list-item>
        </f7-list>

        <f7-list strong inset dividers class="margin-vertical-half" v-if="filteredExchangeRates.length">
            <f7-list-item swipeout
                          link="#" no-chevron
                          :id="getExchangeRateDomId(exchangeRate)"
                          :after="getFinalConvertedAmount(exchangeRate, true)"
                          :key="baseCurrencyChangedTime + '_' + exchangeRate.currencyCode" v-for="exchangeRate in filteredExchangeRates"
                          @click="tapCurrency(exchangeRate)"
                          @swipeout:closed="onExchangeRateSwipeoutClosed()">
                <template #title>
                    <div class="no-padding no-margin">
                        <span style="margin-inline-end: 5px">{{ exchangeRate.currencyDisplayName }}</span>
                        <small class="smaller">{{ exchangeRate.currencyCode }}</small>
                        <small v-if="exchangeRate.currencyCode === baseCurrency" class="fx-base-badge">基准</small>
                    </div>
                </template>
                <f7-swipeout-actions :left="textDirection === TextDirection.RTL"
                                     :right="textDirection === TextDirection.LTR"
                                     v-if="exchangeRate.currencyCode !== baseCurrency || (exchangeRate.currencyCode !== defaultCurrency && isUserCustomExchangeRates)">
                    <f7-swipeout-button color="primary" close
                                        text="设为基准"
                                        :class="{ 'disabled': exchangeRate.currencyCode === baseCurrency }"
                                        @click="tapCurrency(exchangeRate); settingBaseLine = true"
                                        v-if="settingBaseLine || exchangeRate.currencyCode !== baseCurrency"></f7-swipeout-button>
                    <f7-swipeout-button color="red" class="padding-horizontal" aria-label="删除"
                                        @click="remove(exchangeRate, false)"
                                        v-if="exchangeRate.currencyCode !== defaultCurrency && isUserCustomExchangeRates">
                        <f7-icon f7="trash"></f7-icon>
                    </f7-swipeout-button>
                </f7-swipeout-actions>
            </f7-list-item>
        </f7-list>
        <p v-else-if="searchQuery && exchangeRatesData?.exchangeRates?.length" class="cy-empty">没有找到匹配「{{ searchQuery }}」的币种</p>

        <f7-list strong inset dividers class="margin-vertical" v-if="exchangeRatesData && exchangeRatesData.exchangeRates && exchangeRatesData.exchangeRates.length">
            <f7-list-item v-if="exchangeRatesDataUpdateTime">
                <small>{{ tt('Last Updated') }}</small>
                <small>{{ exchangeRatesDataUpdateTime }}</small>
            </f7-list-item>
            <f7-list-item>
                <small>{{ tt('Data source') }}</small>
                <small>
                    <f7-link @click="openExternalUrl(exchangeRatesData.referenceUrl)" v-if="!isUserCustomExchangeRates && exchangeRatesData.referenceUrl">{{ exchangeRatesData.dataSource }}</f7-link>
                    <span v-else-if="!isUserCustomExchangeRates && !exchangeRatesData.referenceUrl">{{ exchangeRatesData.dataSource }}</span>
                    <span v-else-if="isUserCustomExchangeRates">{{ tt('User Custom') }}</span>
                </small>
            </f7-list-item>
        </f7-list>

        <f7-actions close-by-outside-click close-on-escape :opened="showMoreActionSheet" @actions:closed="showMoreActionSheet = false">
            <f7-actions-group v-if="isUserCustomExchangeRates">
                <f7-actions-button :class="{ 'disabled': loading }" @click="update()">
                    <span>{{ tt('Update') }}</span>
                </f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button :class="{ 'disabled': loading }" @click="reload(undefined)">
                    <span>{{ tt('Refresh') }}</span>
                </f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button bold close>{{ tt('Cancel') }}</f7-actions-button>
            </f7-actions-group>
        </f7-actions>

        <f7-actions close-by-outside-click close-on-escape :opened="showDeleteActionSheet" @actions:closed="showDeleteActionSheet = false">
            <f7-actions-group>
                <f7-actions-label>{{ tt('Are you sure you want to delete this user custom exchange rate?') }}</f7-actions-label>
                <f7-actions-button color="red" @click="remove(customExchangeRateToDelete, true)">{{ tt('Delete') }}</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button bold close>{{ tt('Cancel') }}</f7-actions-button>
            </f7-actions-group>
        </f7-actions>
    </f7-page>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import type { Router } from 'framework7/types';

import { useI18n } from '@/locales/helpers.ts';
import { useI18nUIComponents, showLoading, hideLoading, onSwipeoutDeleted } from '@/lib/ui/mobile.ts';
import { useExchangeRatesPageBase } from '@/views/base/ExchangeRatesPageBase.ts';

import { useExchangeRatesStore } from '@/stores/exchangeRates.ts';
import { useAccountsStore } from '@/stores/account.ts';

import { TextDirection } from '@/core/text.ts';
import { NumeralSystem } from '@/core/numeral.ts';
import { AMOUNT_FACTOR } from '@/consts/numeral.ts';
import { TRANSACTION_MIN_AMOUNT, TRANSACTION_MAX_AMOUNT } from '@/consts/transaction.ts';

import type { LocalizedLatestExchangeRate } from '@/models/exchange_rate.ts';

import { parseBigDecimal} from '@/lib/numeral.ts';
import { getCurrentUnixTime } from '@/lib/datetime.ts';

const props = defineProps<{
    f7router: Router.Router;
}>();

const {
    tt,
    getCurrentLanguageTextDirection,
    getCurrentNumeralSystemType,
    getCurrencyName,
    formatAmountToLocalizedNumerals,
    formatExchangeRateAmountToWesternArabicNumerals
} = useI18n();

const { showAlert, showToast, openExternalUrl } = useI18nUIComponents();

const {
    baseCurrency,
    baseAmount,
    defaultCurrency,
    exchangeRatesData,
    isUserCustomExchangeRates,
    exchangeRatesDataUpdateTime,
    availableExchangeRates,
    getConvertedAmount,
    setAsBaseline
} = useExchangeRatesPageBase();

const exchangeRatesStore = useExchangeRatesStore();
const accountsStore = useAccountsStore();
const MAJOR_CURRENCIES = ['CNY', 'USD', 'HKD', 'EUR', 'JPY', 'GBP', 'SGD', 'AUD', 'CAD', 'KRW'];

const loading = ref<boolean>(false);
const searchQuery = ref<string>('');
const baseCurrencyChangedTime = ref<number>(getCurrentUnixTime());
const settingBaseLine = ref<boolean>(false);
const showMoreActionSheet = ref<boolean>(false);
const showBaseCurrencyPopup = ref<boolean>(false);
const showBaseAmountSheet = ref<boolean>(false);
const customExchangeRateToDelete = ref<LocalizedLatestExchangeRate | null>(null);
const showDeleteActionSheet = ref<boolean>(false);

const sortedExchangeRates = computed<LocalizedLatestExchangeRate[]>(() => {
    const activeCodes = new Set(accountsStore.allPlainAccounts.map(a => a.currency));
    const rank = (code: string): number => {
        if (code === baseCurrency.value) return 0;
        if (activeCodes.has(code)) return 1;
        const idx = MAJOR_CURRENCIES.indexOf(code);
        return idx >= 0 ? 10 + idx : 1000;
    };
    return [...availableExchangeRates.value].sort((a, b) => {
        const diff = rank(a.currencyCode) - rank(b.currencyCode);
        return diff !== 0 ? diff : a.currencyCode.localeCompare(b.currencyCode);
    });
});

const filteredExchangeRates = computed<LocalizedLatestExchangeRate[]>(() => {
    const q = searchQuery.value.trim().toLowerCase();
    if (!q) return sortedExchangeRates.value;
    return sortedExchangeRates.value.filter(item =>
        item.currencyCode.toLowerCase().includes(q) || item.currencyDisplayName.toLowerCase().includes(q)
    );
});

function tapCurrency(exchangeRate: LocalizedLatestExchangeRate): void {
    if (exchangeRate.currencyCode === baseCurrency.value) return;
    setAsBaseline(exchangeRate.currencyCode, getFinalConvertedAmount(exchangeRate, false));
    baseCurrencyChangedTime.value = getCurrentUnixTime();
    showToast(`已将 ${exchangeRate.currencyDisplayName}（${exchangeRate.currencyCode}）设为基准货币`);
}

const textDirection = computed<TextDirection>(() => getCurrentLanguageTextDirection());
const numeralSystem = computed<NumeralSystem>(() => getCurrentNumeralSystemType());
const displayBaseAmount = computed<string>(() => formatAmountToLocalizedNumerals(parseBigDecimal(baseAmount.value), baseCurrency.value));
const baseAmountFontSizeClass = computed<string>(() => {
    if (baseAmount.value >= 10000000000 || baseAmount.value <= -10000000000) {
        return 'ebk-extra-small-amount';
    } else if (baseAmount.value >= 100000000 || baseAmount.value <= -100000000) {
        return 'ebk-small-amount';
    } else if (baseAmount.value >= 1000000 || baseAmount.value <= -1000000) {
        return 'ebk-normal-amount';
    } else {
        return 'ebk-large-amount';
    }
});

function getExchangeRateDomId(exchangeRate: LocalizedLatestExchangeRate): string {
    return 'exchangeRate_' + exchangeRate.currencyCode;
}

function reload(done?: () => void): void {
    if (loading.value) {
        done?.();
        return;
    }

    loading.value = true;

    if (!done) {
        showLoading();
    }

    exchangeRatesStore.getLatestExchangeRates({
        silent: false,
        force: true
    }).then(() => {
        done?.();

        loading.value = false;
        hideLoading();

        showToast('汇率数据已更新');
    }).catch(error => {
        done?.();

        loading.value = false;
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function update(): void {
    props.f7router.navigate('/exchange_rates/update');
}

function remove(customExchangeRate: LocalizedLatestExchangeRate | null, confirm: boolean): void {
    if (!customExchangeRate) {
        showAlert('An error occurred');
        return;
    }

    if (!confirm) {
        customExchangeRateToDelete.value = customExchangeRate;
        showDeleteActionSheet.value = true;
        return;
    }

    showDeleteActionSheet.value = false;
    customExchangeRateToDelete.value = null;
    showLoading();

    exchangeRatesStore.deleteUserCustomExchangeRate({
        currency: customExchangeRate.currencyCode,
        beforeResolve: (done) => {
            onSwipeoutDeleted(getExchangeRateDomId(customExchangeRate), done);
        }
    }).then(() => {
        if (customExchangeRate.currencyCode === baseCurrency.value) {
            baseCurrency.value = defaultCurrency.value;
        }

        hideLoading();
    }).catch(error => {
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function getFinalConvertedAmount(toExchangeRate: LocalizedLatestExchangeRate, displayLocalizedDigits: boolean): string {
    const fromExchangeRate = exchangeRatesStore.latestExchangeRateMap[baseCurrency.value];
    const exchangeRateAmount = getConvertedAmount(parseBigDecimal(baseAmount.value).divide(AMOUNT_FACTOR), fromExchangeRate, toExchangeRate);

    if (!exchangeRateAmount) {
        if (displayLocalizedDigits) {
            return numeralSystem.value.digitZero;
        } else {
            return NumeralSystem.WesternArabicNumerals.digitZero;
        }
    }

    let ret = formatExchangeRateAmountToWesternArabicNumerals(exchangeRateAmount);

    if (displayLocalizedDigits) {
        ret = numeralSystem.value.replaceWesternArabicDigitsToLocalizedDigits(ret);
    }

    return ret;
}

function onExchangeRateSwipeoutClosed(): void {
    baseCurrencyChangedTime.value = getCurrentUnixTime();
    settingBaseLine.value = false;
}

exchangeRatesStore.getLatestExchangeRates({
    silent: true,
    force: false
}).then(() => {
    if (exchangeRatesData.value && exchangeRatesData.value.exchangeRates) {
        const exchangeRates = exchangeRatesData.value.exchangeRates;
        let hasBaseCurrency = false;

        for (const exchangeRate of exchangeRates) {
            if (exchangeRate.currency === baseCurrency.value) {
                hasBaseCurrency = true;
                break;
            }
        }

        if (!hasBaseCurrency) {
            showToast('There is no exchange rates data for your default currency');
        }
    }
});
</script>

<style>
.currency-base-amount {
    line-height: 53px;
}

.currency-base-amount .item-header {
    padding-top: calc(var(--f7-typography-padding) / 2);
}

.fx-search-wrap {
    padding: 4px 16px 2px;
}

.fx-search-box {
    display: flex;
    align-items: center;
    gap: 8px;
    background: var(--cy-card);
    border: 1px solid var(--cy-line);
    border-radius: 10px;
    padding: 0 12px;
    min-height: 42px;
}

.fx-search-box .icon {
    font-size: 16px;
    color: var(--cy-muted);
}

.fx-search-box input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: transparent;
    color: var(--cy-ink);
    font-size: 14px;
}

.fx-search-box button {
    border: 0;
    background: transparent;
    color: var(--cy-muted);
    padding: 4px;
    display: grid;
    place-items: center;
}

.fx-hint {
    margin: 6px 4px 0 !important;
}

.fx-base-badge {
    margin-left: 6px;
    padding: 1px 6px;
    border-radius: 8px;
    background: var(--cy-soft);
    color: var(--cy-accent);
    font-size: 11px;
}
</style>
