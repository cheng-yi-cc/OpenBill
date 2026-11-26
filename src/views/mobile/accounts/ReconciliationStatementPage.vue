<template>
    <f7-page class="cy-mobile-surface cy-asset-surface" @page:afterin="onPageAfterIn">
        <f7-navbar>
            <f7-nav-left :class="{ 'disabled': loading || updatingLastReconciledTime }" back-link="返回"></f7-nav-left>
            <f7-nav-title>
                <f7-link popover-open=".display-mode-popover-menu" class="reconcile-nav-trigger" :class="{ 'disabled': loading || updatingLastReconciledTime }">
                    <span class="reconcile-nav-text">{{ currentAccount?.name ? `${currentAccount.name} · 对账` : '流水对账' }}</span>
                    <f7-icon class="reconcile-nav-caret" f7="chevron_down_circle_fill"></f7-icon>
                </f7-link>
            </f7-nav-title>
            <f7-nav-right :class="{ 'navbar-compact-icons': true, 'disabled': loading || updatingLastReconciledTime }">
                <f7-link icon-f7="ellipsis" :class="{ 'disabled': loading || updatingLastReconciledTime }" aria-label="更多操作" @click="showMoreActionSheet = true"></f7-link>
            </f7-nav-right>
        </f7-navbar>

        <f7-popover class="display-mode-popover-menu">
            <f7-list dividers>
                <f7-list-item link="#" no-chevron popover-close
                              title="对账流水"
                              :class="{ 'list-item-selected': !showAccountBalanceTrendsCharts }"
                              @click="showAccountBalanceTrendsCharts = false">
                    <template #after>
                        <f7-icon class="list-item-checked-icon" f7="checkmark_alt" v-if="!showAccountBalanceTrendsCharts"></f7-icon>
                    </template>
                </f7-list-item>
                <f7-list-item link="#" no-chevron popover-close
                              title="余额走势"
                              :class="{ 'list-item-selected': showAccountBalanceTrendsCharts, 'disabled': !reconciliationStatements?.transactions?.length }"
                              @click="showAccountBalanceTrendsCharts = true">
                    <template #after>
                        <f7-icon class="list-item-checked-icon" f7="checkmark_alt" v-if="showAccountBalanceTrendsCharts"></f7-icon>
                    </template>
                </f7-list-item>
            </f7-list>
        </f7-popover>

        <div class="reconcile-range-bar">
            <button v-for="preset in quickDateRanges" :key="preset.type" type="button" :aria-pressed="queryDateRangeType === preset.type" :disabled="loading" @click="selectDateRangeAndReload(preset.type)">{{ preset.label }}</button>
            <button type="button" :aria-pressed="isCustomRangeActive" :disabled="loading" @click="showCustomDateRangeSheet = true">自定义</button>
        </div>

        <f7-list strong inset dividers class="margin-vertical-half" v-if="startTime || endTime">
            <f7-list-item link="#" title="对账时段" :after="compactRangeLabel" @click="showCustomDateRangeSheet = true"></f7-list-item>
        </f7-list>

        <p v-if="loading" class="cy-empty" role="status">正在核算对账流水…</p>

        <f7-list strong inset dividers class="margin-vertical" v-if="!loading && reconciliationStatements">
            <f7-list-item title="账单笔数"
                          :after="`${formatNumberToLocalizedNumerals(reconciliationStatements.transactions.length)} 笔`"
                          v-if="reconciliationStatements.transactions"></f7-list-item>
            <f7-list-item title="本期流入" :after="masked(displayTotalInflows)"></f7-list-item>
            <f7-list-item title="本期流出" :after="masked(displayTotalOutflows)"></f7-list-item>
            <f7-list-item title="净流入" :after="masked(displayTotalBalance)"></f7-list-item>
            <f7-list-item :title="isCurrentLiabilityAccount ? '期初欠款' : '期初余额'" :after="masked(displayOpeningBalance)"></f7-list-item>
            <f7-list-item :title="isCurrentLiabilityAccount ? '期末欠款' : '期末余额'" :after="masked(displayClosingBalance)"></f7-list-item>
            <f7-list-item v-if="currentAccount && !currentAccount.subAccounts?.length" link="#" title="校准账户余额" after="去校准" @click="showBalanceAdjustSheet = true"></f7-list-item>
        </f7-list>

        <f7-list strong inset dividers class="margin-vertical"
                 v-if="!showAccountBalanceTrendsCharts && !loading && (!allReconciliationStatementVirtualListItems || !allReconciliationStatementVirtualListItems.length)">
            <f7-list-item title="所选时段内暂无流水"></f7-list-item>
        </f7-list>

        <f7-list strong inset dividers media-list virtual-list
                 class="margin-vertical transaction-info-list reconciliation-statement-list"
                 :virtual-list-params="{ items: allReconciliationStatementVirtualListItems, renderExternal, height: 'auto' }"
                 v-if="!showAccountBalanceTrendsCharts && !loading && allReconciliationStatementVirtualListItems && allReconciliationStatementVirtualListItems.length">
            <ul>
                <f7-list-item chevron-center
                              :key="item.index"
                              :id="item.transaction ? getTransactionDomId(item.transaction) : undefined"
                              :class="{ 'transaction-info': item.type == 'transaction', 'last-transaction-of-day': allReconciliationStatementVirtualListItems[item.index + 1] && allReconciliationStatementVirtualListItems[item.index + 1]!.type === 'date', 'reconciliation-statement-transaction-date': item.type == 'date' }"
                              :style="`top: ${virtualDataItems.topPosition}px`"
                              :virtual-list-index="item.index"
                              :swipeout="item.type === 'transaction' && !!item.transaction && item.transaction.refund?.kind !== 'cash'"
                              :accordion-item="item.type === 'transaction' && !!item.transaction"
                              :link="item.type == 'transaction' && item.transaction && item.transaction.type !== TransactionType.ModifyBalance ? `/transaction/detail?id=${item.transaction.refund?.kind === 'cash' ? item.transaction.refund.expenseId : item.transaction.id}&type=${item.transaction.refund?.kind === 'cash' ? TransactionType.Expense : item.transaction.type}` : null"
                              v-for="item in virtualDataItems.items"
                >
                    <template #inner>
                        <div class="display-flex no-padding-horizontal" v-if="item.type == 'date' && item.displayDate">
                            <div class="actual-item-inner">
                                <div class="item-title-row">
                                    <div class="item-title">
                                        <small>{{ item.displayDate }}</small>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="display-flex no-padding-horizontal" v-if="item.type == 'transaction' && item.transaction">
                            <div class="item-media">
                                <div class="transaction-icon display-flex align-items-center">
                                    <ItemIcon :icon-type="getCategoryIconType(item.transaction.category?.iconType)"
                                              :icon-id="item.transaction.category?.icon"
                                              :color="item.transaction.category?.color"
                                              v-if="item.transaction.category && item.transaction.category?.color"></ItemIcon>
                                    <f7-icon v-else-if="!item.transaction.category || !item.transaction.category?.color"
                                             f7="pencil_ellipsis_rectangle">
                                    </f7-icon>
                                </div>
                            </div>
                            <div class="actual-item-inner">
                                <div class="item-title-row">
                                    <div class="item-title">
                                        <div class="transaction-category-name no-padding">
                                        <span v-if="item.transaction.refund?.kind === 'cash'">退款</span>
                                        <span v-else-if="item.transaction.type === TransactionType.ModifyBalance">
                                            余额校准
                                        </span>
                                            <span v-else-if="item.transaction.type !== TransactionType.ModifyBalance && item.transaction.category">
                                            {{ item.transaction.category.name }}
                                        </span>
                                        </div>
                                    </div>
                                    <div class="item-after">
                                        <div class="transaction-amount"
                                             :class="{ 'text-expense': item.transaction.type === TransactionType.Expense && item.transaction.refund?.kind !== 'cash', 'text-income': item.transaction.type === TransactionType.Income || item.transaction.refund?.kind === 'cash' }">
                                            <span v-if="item.transaction.type === TransactionType.Transfer && item.transaction.destinationAccountId === accountId">{{ masked(getDisplayDestinationAmount(item.transaction)) }}</span>
                                            <span v-else-if="item.transaction.type !== TransactionType.Transfer || item.transaction.destinationAccountId !== accountId">{{ masked(getDisplaySourceAmount(item.transaction)) }}</span>
                                        </div>
                                    </div>
                                </div>
                                <div class="item-text">
                                    <div class="transaction-description" v-if="item.transaction.comment">
                                        <span>{{ item.transaction.comment }}</span>
                                    </div>
                                </div>
                                <div class="item-footer">
                                    <div class="transaction-footer display-flex justify-content-space-between">
                                        <div class="flex-shrink-0">
                                            <span>{{ getDisplayTime(item.transaction) }}</span>
                                            <span style="margin-inline-start: 4px" v-if="!isSameAsDefaultTimezoneOffsetMinutes(item.transaction)">{{ `(${getDisplayTimezone(item.transaction)})` }}</span>
                                        </div>
                                        <div class="account-balance flex-shrink-1">
                                            <span>{{ isCurrentLiabilityAccount ? '当前欠款' : '余额' }}</span>
                                            <span style="margin-inline-start: 4px">{{ masked(getDisplayAccountBalance(item.transaction)) }}</span>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </template>
                    <f7-swipeout-actions :left="textDirection === TextDirection.RTL"
                                         :right="textDirection === TextDirection.LTR"
                                         v-if="item.type == 'transaction' && item.transaction && item.transaction.refund?.kind !== 'cash'">
                        <f7-swipeout-button color="primary" close
                                            text="再记"
                                            v-if="item.transaction.type !== TransactionType.ModifyBalance"
                                            @click="duplicateTransaction(item.transaction)"></f7-swipeout-button>
                        <f7-swipeout-button color="orange" close
                                            text="编辑"
                                            v-if="item.transaction.editable && item.transaction.type !== TransactionType.ModifyBalance"
                                            @click="editTransaction(item.transaction)"></f7-swipeout-button>
                        <f7-swipeout-button color="red" class="padding-horizontal" aria-label="删除"
                                            v-if="item.transaction.editable"
                                            @click="removeTransaction(item.transaction, false)">
                            <f7-icon f7="trash"></f7-icon>
                        </f7-swipeout-button>
                    </f7-swipeout-actions>
                </f7-list-item>
            </ul>
        </f7-list>

        <f7-card v-if="showAccountBalanceTrendsCharts">
            <f7-card-header class="no-border display-block">
                <div class="statistics-chart-header display-flex width-100 justify-content-space-between">
                    <div></div>
                    <div class="align-self-flex-end">
                        <span style="margin-inline-end: 4px;">统计粒度</span>
                        <f7-link :class="{ 'disabled': loading || updatingLastReconciledTime }" href="#" popover-open=".chart-data-date-aggregation-type-popover-menu">{{ chartDataDateAggregationTypeDisplayName }}</f7-link>
                    </div>
                </div>
            </f7-card-header>
            <f7-card-content style="margin-top: -14px" :padding="false">
                <account-balance-trends-bar-chart
                    :loading="loading"
                    :date-aggregation-type="chartDataDateAggregationType"
                    :timezone-used-for-date-range="timezoneUsedForDateRange"
                    :fiscal-year-start="fiscalYearStart"
                    :items="reconciliationStatements?.transactions"
                    :account="currentAccount"
                    :statement-date="currentAccountStatementDate"
                />
            </f7-card-content>
        </f7-card>

        <f7-popover class="chart-data-date-aggregation-type-popover-menu">
            <f7-list dividers>
                <f7-list-item group-title>
                    <small>统计粒度</small>
                </f7-list-item>
                <f7-list-item link="#" no-chevron popover-close
                              :title="dateAggregationType.displayName"
                              :class="{ 'list-item-selected': chartDataDateAggregationType === dateAggregationType.type }"
                              :key="dateAggregationType.type"
                              v-for="dateAggregationType in allDateAggregationTypes"
                              @click="setChartDataDateAggregationType(dateAggregationType.type)">
                    <template #after>
                        <f7-icon class="list-item-checked-icon" f7="checkmark_alt" v-if="chartDataDateAggregationType === dateAggregationType.type"></f7-icon>
                    </template>
                </f7-list-item>
            </f7-list>
        </f7-popover>

        <date-range-selection-sheet title="自定义对账时段"
                                    :min-time="startTime"
                                    :max-time="endTime"
                                    v-model:show="showCustomDateRangeSheet"
                                    @dateRange:change="changeCustomDateFilter">
        </date-range-selection-sheet>

        <AssetBalanceSheet v-if="currentAccount" v-model:opened="showBalanceAdjustSheet" :account="currentAccount" @saved="onBalanceAdjusted" />

        <f7-actions close-by-outside-click close-on-escape :opened="showMoreActionSheet" @actions:closed="showMoreActionSheet = false">
            <f7-actions-group>
                <f7-actions-button :class="{ 'disabled': loading || updatingLastReconciledTime }" @click="addTransaction()">记一笔</f7-actions-button>
                <f7-actions-button :class="{ 'disabled': loading || updatingLastReconciledTime }" @click="openBalanceAdjust()" v-if="currentAccount && currentAccount.type === AccountType.SingleAccount.type">校准余额</f7-actions-button>
                <f7-actions-button :class="{ 'disabled': loading || updatingLastReconciledTime }" @click="updateLastReconciledTime()" v-if="newLastReconciledTime">标记已对账</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button :class="{ 'disabled': loading || updatingLastReconciledTime }" @click="reload(true)">刷新对账</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button bold close>取消</f7-actions-button>
            </f7-actions-group>
        </f7-actions>

        <f7-actions close-by-outside-click close-on-escape :opened="showDeleteActionSheet" @actions:closed="showDeleteActionSheet = false">
            <f7-actions-group>
                <f7-actions-label>确定删除这条账单？账户余额将相应恢复。</f7-actions-label>
                <f7-actions-button color="red" @click="removeTransaction(transactionToDelete, true)">删除</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button bold close>取消</f7-actions-button>
            </f7-actions-group>
        </f7-actions>
    </f7-page>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import type { Router } from 'framework7/types';

import { useI18n } from '@/locales/helpers.ts';
import { useI18nUIComponents, showLoading, hideLoading, onSwipeoutDeleted } from '@/lib/ui/mobile.ts';
import { useReconciliationStatementPageBase } from '@/views/base/accounts/ReconciliationStatementPageBase.ts';
import { assetAmountsVisible } from '@/lib/asset-visibility.ts';
import AssetBalanceSheet from '@/components/mobile/AssetBalanceSheet.vue';

import { useSettingsStore } from '@/stores/setting.ts';
import { useUserStore } from '@/stores/user.ts';
import { useAccountsStore } from '@/stores/account.ts';
import { useTransactionCategoriesStore } from '@/stores/transactionCategory.ts';
import { useTransactionsStore } from '@/stores/transaction.ts';

import { TextDirection } from '@/core/text.ts';
import { type TimeRangeAndDateType, DateRange, DateRangeScene } from '@/core/datetime.ts';
import { AccountType } from '@/core/account.ts';
import { TransactionType } from '@/core/transaction.ts';
import { DEFAULT_RECONCILIATION_STATEMENT_DATE_RANGE_IN_MOBILE } from '@/core/statistics.ts';
import { type TransactionReconciliationStatementResponseItemWithInfo } from '@/models/transaction.ts';

import { findDisplayNameByType } from '@/lib/common.ts';
import {
    getDateTypeByDateRange,
    getDateTypeByBillingCycleDateRange,
    getDateRangeByDateType,
    getDateRangeByBillingCycleDateType,
    getDateRangeByLastReconciledTimeRangeDateType
} from '@/lib/datetime.ts';
import { getCategoryIconType } from '@/lib/icon.ts';

interface ReconciliationStatementVirtualListData {
    items: ReconciliationStatementVirtualListItem[],
    topPosition: number
}

interface ReconciliationStatementVirtualListItem {
    index: number;
    type: ReconciliationStatementVirtualListItemType;
    displayDate?: string;
    transaction?: TransactionReconciliationStatementResponseItemWithInfo;
}

type ReconciliationStatementVirtualListItemType = 'transaction' | 'date';

const props = defineProps<{
    f7route: Router.Route;
    f7router: Router.Router;
}>();

const {
    tt,
    getCurrentLanguageTextDirection,
    formatNumberToLocalizedNumerals
} = useI18n();

const { showAlert, showToast, routeBackOnError } = useI18nUIComponents();

const {
    accountId,
    startTime,
    endTime,
    reconciliationStatements,
    chartDataDateAggregationType,
    timezoneUsedForDateRange,
    firstDayOfWeek,
    fiscalYearStart,
    allDateAggregationTypes,
    isCurrentLiabilityAccount,
    newLastReconciledTime,
    currentAccount,
    currentAccountCurrency,
    currentAccountStatementDate,
    currentAccountLastReconciledTime,
    displayStartDateTime,
    displayEndDateTime,
    displayTotalInflows,
    displayTotalOutflows,
    displayTotalBalance,
    displayOpeningBalance,
    displayClosingBalance,
    updatePageOpenTime,
    isReconciliationStatementsResponseEquals,
    setReconciliationStatements,
    getDisplayDate,
    getDisplayTime,
    isSameAsDefaultTimezoneOffsetMinutes,
    getDisplayTimezone,
    getDisplaySourceAmount,
    getDisplayDestinationAmount,
    getDisplayAccountBalance
} = useReconciliationStatementPageBase();

const settingsStore = useSettingsStore();
const userStore = useUserStore();
const accountsStore = useAccountsStore();
const transactionCategoriesStore = useTransactionCategoriesStore();
const transactionsStore = useTransactionsStore();

const loading = ref<boolean>(false);
const loadingError = ref<unknown | null>(null);
const queryDateRangeType = ref<number>(DateRange.ThisMonth.type);
const showAccountBalanceTrendsCharts = ref<boolean>(false);
const updatingLastReconciledTime = ref<boolean>(false);
const transactionToDelete = ref<TransactionReconciliationStatementResponseItemWithInfo | null>(null);
const showCustomDateRangeSheet = ref<boolean>(false);
const showBalanceAdjustSheet = ref<boolean>(false);
const showMoreActionSheet = ref<boolean>(false);
const showDeleteActionSheet = ref<boolean>(false);
const virtualDataItems = ref<ReconciliationStatementVirtualListData>({
    items: [],
    topPosition: 0
});

const textDirection = computed<TextDirection>(() => getCurrentLanguageTextDirection());
const quickDateRanges = computed(() => [
    { type: DateRange.ThisMonth.type, label: '本月' },
    { type: DateRange.LastMonth.type, label: '上月' },
    { type: DateRange.LastThirtyDays.type, label: '近30天' },
    { type: DateRange.ThisYear.type, label: '今年' },
    { type: DateRange.All.type, label: '全部' }
]);
const isCustomRangeActive = computed(() => !quickDateRanges.value.some(item => item.type === queryDateRangeType.value));
const compactRangeLabel = computed(() => {
    const clean = (s: string) => s.split(' ')[0]?.replace(/年|月/g, '/').replace(/日/g, '') || '';
    return `${clean(displayStartDateTime.value)} — ${clean(displayEndDateTime.value)}`;
});

function masked(value: string): string {
    if (!assetAmountsVisible.value) return '••••';
    return value.replace(/^([¥$€£])\s*-/, '-$1').replace(/^([¥$€£])\s+/, '$1');
}

const allReconciliationStatementVirtualListItems = computed<ReconciliationStatementVirtualListItem[]>(() => {
    const ret: ReconciliationStatementVirtualListItem[] = [];

    if (!reconciliationStatements.value || !reconciliationStatements.value.transactions || reconciliationStatements.value.transactions.length < 1) {
        return ret;
    }

    let index = 0;
    let lastDisplayDate: string | null = null;

    for (const transaction of reconciliationStatements.value.transactions) {
        const displayDate = getDisplayDate(transaction);

        if (lastDisplayDate !== displayDate) {
            lastDisplayDate = displayDate;
            ret.push({
                index: index++,
                type: 'date',
                displayDate: displayDate
            });
        }

        ret.push({
            index: index++,
            type: 'transaction',
            transaction: transaction
        });
    }

    return ret;
});

const chartDataDateAggregationTypeDisplayName = computed<string>(() => {
    return findDisplayNameByType(allDateAggregationTypes.value, chartDataDateAggregationType.value) || tt('Unknown');
});

function getTransactionDomId(transaction: TransactionReconciliationStatementResponseItemWithInfo): string {
    return 'transaction_' + transaction.id;
}

function init(): void {
    const query = props.f7route.query;
    updatePageOpenTime();
    loading.value = true;
    accountId.value = query['accountId'] || '';
    reconciliationStatements.value = undefined;

    Promise.all([
        accountsStore.loadAllAccounts({ force: false }),
        transactionCategoriesStore.loadAllCategories({ force: false })
    ]).then(() => {
        initDateFilter();
        reload(false);
    }).catch(error => {
        loading.value = false;
        loadingError.value = error;
        showToast(error.message || error);
    });
}

function initDateFilter(): void {
    let defaultDateRangeType = settingsStore.appSettings.reconciliationStatementPageDefaultDateRangeTypeInMobile;
    const defaultDateRange = DateRange.valueOf(defaultDateRangeType);

    if (!defaultDateRange || defaultDateRange.type === DateRange.All.type) {
        defaultDateRangeType = DateRange.ThisMonth.type;
    } else if (defaultDateRange.isBillingCycle && !accountsStore.getAccountStatementDate(accountId.value)) {
        defaultDateRangeType = DateRange.ThisMonth.type;
    } else if (defaultDateRange.isLastReconciledTimeRange && (!userStore.currentUserUseLastReconciledTime || !currentAccount.value || !currentAccount.value.lastReconciledTime)) {
        defaultDateRangeType = DateRange.ThisMonth.type;
    } else {
        defaultDateRangeType = defaultDateRange.type;
    }

    changeDateFilter(defaultDateRangeType || DEFAULT_RECONCILIATION_STATEMENT_DATE_RANGE_IN_MOBILE.type);
}

function changeDateFilter(dateRangeType: number): void {
    if (dateRangeType === DateRange.Custom.type) {
        showCustomDateRangeSheet.value = true;
        return;
    }

    let dateRange: TimeRangeAndDateType | null = null;

    if (DateRange.isBillingCycle(dateRangeType)) {
        dateRange = getDateRangeByBillingCycleDateType(dateRangeType, firstDayOfWeek.value, fiscalYearStart.value, accountsStore.getAccountStatementDate(accountId.value));
    } else if (DateRange.isLastReconciledTimeRange(dateRangeType)) {
        dateRange = getDateRangeByLastReconciledTimeRangeDateType(dateRangeType, currentAccountLastReconciledTime.value);
    } else {
        dateRange = getDateRangeByDateType(dateRangeType, firstDayOfWeek.value, fiscalYearStart.value);
    }

    if (!dateRange) {
        return;
    }

    queryDateRangeType.value = dateRange.dateType;
    startTime.value = dateRange.minTime;
    endTime.value = dateRange.maxTime;
}

function selectDateRangeAndReload(dateRangeType: number): void {
    changeDateFilter(dateRangeType);
    reload(false);
}

function changeCustomDateFilter(minTime: number, maxTime: number): void {
    if (!minTime || !maxTime) {
        return;
    }

    let dateType: number | null = getDateTypeByBillingCycleDateRange(minTime, maxTime, firstDayOfWeek.value, fiscalYearStart.value, DateRangeScene.Normal, accountsStore.getAccountStatementDate(accountId.value));

    if (!dateType) {
        dateType = getDateTypeByDateRange(minTime, maxTime, firstDayOfWeek.value, fiscalYearStart.value, DateRangeScene.Normal);
    }

    queryDateRangeType.value = dateType;
    startTime.value = minTime;
    endTime.value = maxTime;
    showCustomDateRangeSheet.value = false;
    reload(false);
}

function reload(force: boolean): void {
    if (!accountId.value) return;
    loading.value = true;

    transactionsStore.getReconciliationStatements({
        accountId: accountId.value,
        startTime: startTime.value,
        endTime: endTime.value
    }).then(result => {
        if (force) {
            if (isReconciliationStatementsResponseEquals(result)) {
                showToast('对账数据已是最新');
            } else {
                showToast('对账数据已更新');
            }
        }

        loading.value = false;
        setReconciliationStatements(result);
    }).catch(error => {
        loading.value = false;

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function addTransaction(): void {
    props.f7router.navigate(`/transaction/add?accountId=${accountId.value}`);
}

function duplicateTransaction(transaction: TransactionReconciliationStatementResponseItemWithInfo): void {
    props.f7router.navigate(`/transaction/add?id=${transaction.id}&type=${transaction.type}`);
}

function editTransaction(transaction: TransactionReconciliationStatementResponseItemWithInfo): void {
    props.f7router.navigate(`/transaction/edit?id=${transaction.id}&type=${transaction.type}`);
}

function openBalanceAdjust(): void {
    showBalanceAdjustSheet.value = true;
}

function onBalanceAdjusted(): void {
    void accountsStore.loadAllAccounts({ force: true }).finally(() => reload(false));
}

function updateLastReconciledTime(): void {
    if (!newLastReconciledTime.value) {
        return;
    }

    updatingLastReconciledTime.value = true;
    showLoading(() => updatingLastReconciledTime.value);

    accountsStore.updateAccountLastReconciledTime(accountId.value, newLastReconciledTime.value).then(() => {
        updatingLastReconciledTime.value = false;
        hideLoading();
        showToast('已标记最新对账时间');
    }).catch(error => {
        updatingLastReconciledTime.value = false;
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function removeTransaction(transaction: TransactionReconciliationStatementResponseItemWithInfo | null, confirm: boolean): void {
    if (!transaction) {
        showAlert('发生错误');
        return;
    }

    if (!confirm) {
        transactionToDelete.value = transaction;
        showDeleteActionSheet.value = true;
        return;
    }

    showDeleteActionSheet.value = false;
    transactionToDelete.value = null;
    showLoading();

    transactionsStore.deleteTransaction({
        transaction: transaction,
        defaultCurrency: currentAccountCurrency.value,
        beforeResolve: (done) => {
            onSwipeoutDeleted(getTransactionDomId(transaction), done);
        }
    }).then(() => {
        hideLoading();
        showToast('账单已删除');
        reload(false);
    }).catch(error => {
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function setChartDataDateAggregationType(type: number): void {
    chartDataDateAggregationType.value = type;
}

function renderExternal(vl: unknown, vlData: ReconciliationStatementVirtualListData): void {
    virtualDataItems.value = vlData;
}

function onPageAfterIn(): void {
    if (transactionsStore.transactionReconciliationStatementStateInvalid) {
        reload(false);
    }

    routeBackOnError(props.f7router, loadingError);
}

init();
</script>

<style>
.reconcile-nav-trigger {
    display: inline-flex !important;
    align-items: center;
    gap: 5px;
    max-width: 100%;
    min-width: 0;
    color: var(--cy-ink) !important;
    padding: 0 !important;
}
.reconcile-nav-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.reconcile-nav-caret {
    flex: none;
    font-size: 16px !important;
    opacity: 0.6;
}
.reconcile-range-bar {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding: 10px 16px 4px;
}
.reconcile-range-bar button {
    flex: 0 0 auto;
    padding: 6px 11px;
    border-radius: 999px;
    border: 1px solid var(--cy-line);
    background: var(--cy-card);
    color: var(--cy-muted);
    font-size: 12px;
}
.reconcile-range-bar button[aria-pressed=true] {
    background: var(--cy-accent);
    border-color: var(--cy-accent);
    color: #fff;
    font-weight: 500;
}
.dark .reconcile-range-bar button[aria-pressed=true] {
    color: #102725;
}
.list.reconciliation-statement-list li.reconciliation-statement-transaction-date:first-child {
    border-radius: var(--f7-list-inset-border-radius) var(--f7-list-inset-border-radius) 0 0;
}

.list.reconciliation-statement-list li.reconciliation-statement-transaction-date {
    display: flex;
    align-items: center;
    align-content: center;
    padding-top: 0;
    padding-bottom: 0;
    height: var(--f7-list-group-title-height);
    line-height: var(--f7-list-group-title-height);
    background-color: var(--f7-list-group-title-bg-color);
}

.list.reconciliation-statement-list li.reconciliation-statement-transaction-date > .item-content {
    padding-inline-start: 0 !important;
}

.list.reconciliation-statement-list li.reconciliation-statement-transaction-date > .item-content > .item-inner {
    padding-inline-start: calc(var(--f7-list-item-padding-horizontal) + var(--f7-safe-area-left));
}

.list.reconciliation-statement-list li.reconciliation-statement-transaction-date > .item-content > .item-inner:after {
    background-color: inherit;
}

.list.reconciliation-statement-list li.transaction-info.last-transaction-of-day > .item-link > .item-content > .item-inner:after,
.list.reconciliation-statement-list li.transaction-info.last-transaction-of-day > .swipeout-content > .item-link > .item-content > .item-inner:after {
    background-color: inherit;
}

.list.reconciliation-statement-list li.transaction-info .account-balance {
    margin-inline-start: 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
</style>
