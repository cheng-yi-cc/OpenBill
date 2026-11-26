<template>
    <f7-page class="cy-main-page cy-mobile-surface" :class="{'cy-calendar-page': isCalendar, 'cy-home-page': !isCalendar}" :ptr="isCalendar || experience.preferences.pullAction !== 'none'" :ptr-preloader="isCalendar || experience.preferences.pullAction !== 'add'" @ptr:refresh="pullRefresh" @page:afterin="onPageAfterIn" @page:beforeout="active = false">
        <div v-if="!isCalendar && experience.preferences.pullAction === 'add'" class="ptr-preloader cy-ptr-add"><span class="cy-ptr-arrow" aria-hidden="true">↓</span><span class="cy-ptr-text cy-ptr-pull">下拉记一笔</span><span class="cy-ptr-text cy-ptr-release">松手记一笔</span></div>
        <f7-navbar :class="{'cy-calendar-navbar': isCalendar}">
            <f7-nav-left v-if="isCalendar" class="cy-calendar-nav-left"><div class="cy-calendar-caption"><button aria-label="选择月份" @click="showMonth = true">{{ date(month).format('YYYY年M月') }}<small>⌄</small></button><div class="cy-calendar-totals"><span>收 <b class="cy-income">{{ display(totals.income) }}</b></span><span>支 <b class="cy-expense">{{ display(totals.expense) }}</b></span><span>余 {{ display(totals.balance) }}</span></div></div></f7-nav-left>
            <f7-nav-left v-else class="cy-home-nav-left">
                <button id="cy-home-book-menu" class="cy-book-trigger" aria-label="切换或添加账本" aria-haspopup="dialog" :aria-expanded="showBooks" @click="showBooks = true">
                    <f7-icon f7="book_closed_fill" /><span>{{ books.scopeName }}</span><f7-icon class="cy-book-chevron" f7="chevron_down" />
                </button>
            </f7-nav-left>
            <f7-nav-right v-if="isCalendar" class="cy-calendar-tools"><button aria-label="回到今天" @click="today">今</button><f7-link :href="addLink" aria-label="添加记账"><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="mdiNotePlusOutline" /></svg></f7-link><button id="cy-calendar-menu" aria-label="日历菜单" @click="showMenu = true"><f7-icon f7="ellipsis_vertical" /></button></f7-nav-right>
            <f7-nav-right v-else class="cy-home-tools"><f7-link v-if="experience.preferences.hideAdd" :href="addLink" icon-f7="plus" aria-label="添加记账" /><f7-link href="/transaction/list?search=true" icon-f7="search" aria-label="搜索账单" /><f7-link id="cy-home-menu" icon-f7="ellipsis_vertical" aria-label="首页更多功能" @click="showMenu = true" /></f7-nav-right>
        </f7-navbar>
        <main class="cy-page-body" :class="{'cy-calendar-body': isCalendar}">
            <p v-if="error" class="cy-message" role="alert">{{ error }} <button @click="refresh()">重试</button></p>
            <template v-if="!error">
                <section v-if="!isCalendar && !experience.preferences.hideHero" class="cy-home-hero" aria-label="月度收支概览">
                    <div class="cy-hero-stats"><f7-link :href="homeBillsLink(2)"><p>{{ homePeriodLabel }}收入</p><strong>{{ display(totals.income) }}</strong></f7-link><f7-link :href="homeBillsLink(0)"><p>{{ homePeriodLabel }}结余</p><strong>{{ display(totals.balance) }}</strong></f7-link><f7-link :href="budgetLink"><p>剩余预算</p><strong>{{ loading ? '—' : homeBudget === undefined ? '点击设置' : ledgerMoney(homeBudget,false) }}</strong></f7-link></div>
                    <f7-link :href="homeBillsLink(3)"><p>{{ homePeriodLabel }}支出(元)</p><strong class="cy-major">{{ display(totals.expense) }}</strong></f7-link>
                </section>
                <section v-else-if="isCalendar" class="cy-calendar-panel">
                    <div ref="calendarGridEl" class="cy-calendar-grid" role="group" aria-label="按日期查看账单" @touchstart.passive="touchStart" @touchend.passive="touchEnd">
                        <span v-for="weekday in weekDays" :key="weekday" class="cy-weekday">{{ weekday }}</span>
                        <span v-for="blank in firstWeekday" :key="`blank-${blank}`" aria-hidden="true" />
                        <button v-for="day in days" :key="day.key" :disabled="loading" :aria-label="dayLabel(day)" :aria-pressed="selectedDay === day.key" :class="{ 'cy-today': day.key === currentDay }" :style="heatStyle(day)" @click="selectCalendarDay(day.key)">
                            <strong>{{ day.key === currentDay && day.key !== selectedDay ? '今' : day.number }}</strong>
                            <template v-if="day.hasFlows"><small class="cy-expense">{{ day.totals.complete ? compact(day.totals.expense,-1) : '待换算' }}</small><small class="cy-income">{{ day.totals.complete ? compact(day.totals.income,1) : '' }}</small></template>
                            <small v-else-if="preferences.calendarShowLunar" class="cy-lunar">{{ lunar(day.key) }}</small>
                            <span v-if="day.due.length" class="cy-due-dots"><i v-if="day.due.some(item => item.kind === 'repayment')" class="cy-repay-dot" /><i v-if="day.due.some(item => item.kind === 'deposit')" class="cy-deposit-dot" /></span>
                        </button>
                    </div>
                </section>
                <p v-if="isCalendar && dueError" class="cy-message" role="alert">{{ dueError }} <button @click="loadDue">重试</button></p>
                <header v-if="isCalendar && (loading || visibleEntries.length || selectedDue.length)" class="cy-selected-heading"><h2>{{ selectedDayLabel }} <small>{{ lunar(selectedDay) }}</small></h2><span><span>收 <b class="cy-income">{{ display(dayTotals.income,dayTotals.complete) }}</b></span><span>支 <b class="cy-expense">{{ display(dayTotals.expense,dayTotals.complete) }}</b></span><span>余 {{ display(dayTotals.balance,dayTotals.complete) }}</span></span></header>
                <section v-if="isCalendar && selectedDue.length" class="cy-due-card"><f7-link v-for="item in selectedDue" :key="item.id" :href="dueLink(item)"><span><strong>{{ dueTitle(item) }} · {{ item.accountName }}</strong><small>{{ item.note || (item.date < currentDay ? '已逾期 · 点击处理' : '点击查看或处理') }}</small></span><b>{{ ledgerMoney(item.amount,false) }} <small>{{ item.currency }}</small></b></f7-link></section>
                <p v-if="!loading && !totals.complete" class="cy-message" role="status">含未确认汇率的外币账单，人民币合计暂不显示（明细仍显示原币）</p>
                <f7-link v-if="!isCalendar && !experience.preferences.hideAdd" class="cy-primary-action" :href="addLink" @taphold="openTemplateQuickSheet"><svg class="cy-add-icon" viewBox="0 0 24 24" aria-hidden="true"><path :d="mdiNotePlusOutline" /></svg><span>添加一条新记账</span></f7-link>
                <LedgerHomeCards v-if="!isCalendar && experience.preferences.cards.length" :entries="entries" :month="homeMonth" />
                <div v-if="!isCalendar && experience.preferences.range === 'month'" class="cy-home-month"><button aria-label="上个月" @click="shiftHome(-1)">‹</button><input v-model="homeMonth" type="month" aria-label="账单月份" /><button aria-label="下个月" @click="shiftHome(1)">›</button></div>
                <div ref="ledgerListEl" class="cy-ledger-flow-stage">
                <p v-if="loading" class="cy-empty" role="status">正在加载账单…</p>
                <template v-else><LedgerDayList :entries="isCalendar ? visibleEntries : visibleEntries.slice(0,homeLimit)" :show-heading="!isCalendar" @hold="openBillActions" /><button v-if="!isCalendar && homeLimit < visibleEntries.length" class="cy-home-load" @click="homeLimit += 100">加载更多账单</button><section v-if="!visibleEntries.length && (!isCalendar || !selectedDue.length)" class="cy-panel cy-empty"><p>{{ isCalendar ? `${selectedDay} 还没有账单` : '还没有账单，记一笔吧' }}</p><f7-link v-if="isCalendar" :href="addLink">记一笔</f7-link></section></template>
                </div>
            </template>
        </main>
        <template #fixed><f7-link v-if="!isCalendar && experience.preferences.floatPosition !== 'hidden'" :class="['cy-fab',{'cy-home-fab-left':experience.preferences.floatPosition === 'left'}]" :href="experience.preferences.floatAction === 'add' ? addLink : '/user/import'" :aria-label="experience.preferences.floatAction === 'add' ? '快捷记账' : '导入账单'"><f7-icon :f7="experience.preferences.floatAction === 'add' ? 'plus' : 'tray_arrow_down'" /></f7-link><LedgerNavigation :active="isCalendar ? 'calendar' : 'home'" /></template>
        <f7-popover v-if="!isCalendar" target-el="#cy-home-menu" v-model:opened="showMenu" class="cy-mobile-surface cy-home-popover"><div class="cy-home-menu-content"><f7-link href="/settings/cards" popover-close><f7-icon f7="square_grid_2x2" />数据小卡片</f7-link><f7-link href="/template/list" popover-close><f7-icon f7="doc_on_doc" />模板记账</f7-link><f7-link :href="budgetLink" popover-close><f7-icon f7="chart_pie" />预算管理</f7-link><p>视图</p><div><button :aria-pressed="experience.preferences.view === 'simple'" @click="experience.set('view','simple'); showMenu = false"><f7-icon f7="list_bullet" />简约</button><button :aria-pressed="experience.preferences.view === 'detail'" @click="experience.set('view','detail'); showMenu = false"><f7-icon f7="list_bullet_below_rectangle" />详细</button><button :aria-pressed="experience.preferences.range === 'all'" @click="experience.set('range','all'); showMenu = false">全部</button><button :aria-pressed="experience.preferences.range === 'month'" @click="experience.set('range','month'); showMenu = false">按月</button></div><p>截图导入</p><div><f7-link href="/user/import?source=wechat-screenshot" popover-close><f7-icon f7="bubble_left_bubble_right" />微信</f7-link><f7-link href="/user/import?source=alipay-screenshot" popover-close><f7-icon f7="creditcard" />支付宝</f7-link></div></div></f7-popover>
        <f7-popover v-if="isCalendar" target-el="#cy-calendar-menu" v-model:opened="showMenu" class="cy-calendar-popover"><f7-list>
            <f7-list-item title="本月账单" :link="monthBillsLink" popover-close><template #media><f7-icon f7="calendar" /></template></f7-list-item>
            <f7-list-item title="多选账本" link="#" popover-close @click="openBookSelector"><template #media><f7-icon f7="checkmark_square" /></template></f7-list-item>
            <f7-list-item :title="preferences.calendarHeatmap ? '隐藏热力背景' : '显示热力背景'" link="#" popover-close @click="settings.setCalendarPreference('calendarHeatmap',!preferences.calendarHeatmap)"><template #media><f7-icon f7="square_stack_3d_up" /></template></f7-list-item>
            <f7-list-item title="更多设置" link="/calendar/settings" popover-close><template #media><f7-icon f7="ellipsis" /></template></f7-list-item>
        </f7-list></f7-popover>
        <f7-sheet v-if="isCalendar" v-model:opened="showMonth" class="cy-calendar-sheet" backdrop swipe-to-close><f7-toolbar><div class="left">选择月份</div><div class="right"><f7-link sheet-close>完成</f7-link></div></f7-toolbar><div class="cy-month-choice"><button class="cy-button" aria-label="上个月" @click="shift(-1)">‹</button><input v-model="month" type="month" min="1900-01" max="9999-12" aria-label="账单月份" @change="monthChanged" /><button class="cy-button" aria-label="下个月" @click="shift(1)">›</button></div></f7-sheet>
        <f7-sheet v-if="isCalendar" v-model:opened="showBookSelector" class="cy-calendar-sheet" backdrop swipe-to-close><f7-toolbar><div class="left"><f7-link sheet-close>取消</f7-link></div><div class="title">选择账本范围</div><div class="right"><f7-link @click="applyBooks">完成</f7-link></div></f7-toolbar><div class="cy-book-options"><p v-if="!calendarBookIds.length && books.allBooks.some(book => book.hidden)" class="cy-muted">将仅统计所选可见账本。</p><label><input type="checkbox" :checked="allVisibleDraftBooks" :disabled="!books.visibleBooks.length" @change="draftBooks = books.visibleBooks.map(book => book.id)" />全部可见账本</label><label v-for="book in books.visibleBooks" :key="book.id"><input type="checkbox" :checked="draftBooks.includes(book.id)" @change="draftBooks = toggleVisibleCalendarBook(draftBooks, books.allBooks, book.id, ($event.target as HTMLInputElement).checked)" />{{ book.icon }} {{ book.name }}{{ book.archived ? ' · 已归档' : '' }}</label><p v-if="bookSelectorError" class="cy-message" role="alert">{{ bookSelectorError }}</p></div></f7-sheet>
        <f7-popover v-if="!isCalendar" target-el="#cy-home-book-menu" v-model:opened="showBooks" class="cy-home-books">
            <f7-list>
                <f7-list-item title="全部账本" link="#" popover-close :no-chevron="true" @click="selectBook('')"><template #after><f7-icon v-if="!books.selectedBookIds.length" f7="checkmark_alt" /></template></f7-list-item>
                <f7-list-item v-for="book in books.visibleBooks" :key="book.id" :title="`${book.icon} ${book.name}${book.archived ? ' · 已归档' : ''}`" link="#" popover-close :no-chevron="true" @click="selectBook(book.id)"><template #after><f7-icon v-if="books.selectedBookIds.includes(book.id)" f7="checkmark_alt" /></template></f7-list-item>
                <f7-list-item title="添加账本" link="/books" popover-close><template #media><f7-icon f7="plus" /></template></f7-list-item>
            </f7-list>
        </f7-popover>
    </f7-page>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { f7 } from 'framework7-vue';
import { mdiNotePlusOutline } from '@mdi/js';
import { storeToRefs } from 'pinia';
import { useBooksStore } from '@/stores/books.ts';
import { useLedgerScopeStore } from '@/stores/ledgerScope.ts';
import { useTransactionsStore } from '@/stores/transaction.ts';
import { useTransactionTemplatesStore } from '@/stores/transactionTemplate.ts';
import { TemplateType } from '@/core/template.ts';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import services from '@/lib/services.ts';
import moment from 'moment-timezone';
import type { Router } from 'framework7/types';
import LedgerNavigation from '@/components/mobile/LedgerNavigation.vue';
import LedgerDayList from '@/components/mobile/LedgerDayList.vue';
import LedgerHomeCards from '@/components/mobile/LedgerHomeCards.vue';
import { LedgerDecimal, ledgerMoney, ledgerTotals, useMobileLedger, type LedgerEntry } from '@/lib/mobile-ledger.ts';
import { useLedgerExperienceStore } from '@/stores/ledgerExperience.ts';
import { useStatisticsWorkspaceStore } from '@/stores/statisticsWorkspace.ts';
import { budgetReports } from '@/lib/statistics-report.ts';
import { ledgerMonthRange, ledgerAccountingMonth } from '@/lib/ledger-preferences.ts';
import { useSettingsStore } from '@/stores/setting.ts';
import { calendarEvents, type CalendarEvent } from '@/lib/calendar-events.ts';
import { calendarCreditEvents } from '@/lib/calendar-credit.ts';
import { calendarBookDraft, calendarBookScope, toggleVisibleCalendarBook } from '@/lib/calendar-book-scope.ts';
import { creditAccounts } from '@/lib/credit-accounts.ts';
import { debtAccounts } from '@/lib/debt-accounts.ts';
import { assetTools, mergeDebtDue } from '@/lib/asset-tools.ts';
import { useAccountsStore } from '@/stores/account.ts';
import { investmentError } from '@/lib/investments.ts';
import { monetaryIncomeRevision } from '@/lib/monetary-income.ts';
import { getChineseYearMonthDayInfo, getChineseCalendarAlternateDisplayDate } from '@/lib/calendar/chinese_calendar.ts';
import { DEFAULT_CONTENT } from '@/locales/calendar/chinese/index.ts';
import { animateSettle } from '@/lib/motion.ts';
const calendarGridEl = ref<HTMLElement>(), ledgerListEl = ref<HTMLElement>();
const props = defineProps<{ f7route: Router.Route; f7router: Router.Router }>();
const isCalendar = computed(() => props.f7route.path === '/calendar');
const scope = useLedgerScopeStore(), books = useBooksStore(), accounts = useAccountsStore(), transactionsStore = useTransactionsStore(), templatesStore = useTransactionTemplatesStore();
const { showConfirm, showToast } = useI18nUIComponents();
const calendarBookIds = computed(() => scope.calendarBookIds ?? books.selectedBookIds);
const { entries, displayEntries, loading, error, load } = useMobileLedger({ applyFilters: () => false, bookIds: () => isCalendar.value ? calendarBookIds.value : books.selectedBookIds });
const settings = useSettingsStore();
const experience = useLedgerExperienceStore(), workspace = useStatisticsWorkspaceStore();
const preferences = computed(() => settings.appSettings);
const showMenu = ref(false), showMonth = ref(false), showBookSelector = ref(false), draftBooks = ref<string[]>([]);
const bookSelectorError = ref('');
const allVisibleDraftBooks = computed(() => books.visibleBooks.length > 0 && draftBooks.value.length === books.visibleBooks.length && books.visibleBooks.every(book => draftBooks.value.includes(book.id)));
const dueItems = ref<CalendarEvent[]>([]), dueError = ref('');
let dueGeneration = 0;
const visibleDue = computed(() => dueItems.value.filter(item => !item.completed && (item.id.startsWith('credit:') || item.id.startsWith('debt:') || !calendarBookIds.value.length || calendarBookIds.value.includes(item.bookId)) && (item.kind === 'repayment' ? preferences.value.calendarShowRepayments : preferences.value.calendarShowDeposits)));
const selectedDue = computed(() => visibleDue.value.filter(item => item.date === selectedDay.value));
const dayTotals = computed(() => ledgerTotals(entries.value.filter(item => item.day === selectedDay.value)));
const monthBillsLink = computed(() => `/transaction/list?dateType=255&minTime=${date(month.value).startOf('month').unix()}&maxTime=${date(month.value).endOf('month').unix()}&bookIds=${encodeURIComponent(calendarBookIds.value.join(','))}`);
const homeMonth = ref(ledgerAccountingMonth(moment().tz(scope.timeZone),experience.preferences.monthStart)), homeLimit = ref(100);
const currentAccountingMonth = computed(() => ledgerAccountingMonth(moment.tz(scope.currentDay, 'YYYY-MM-DD', scope.timeZone), experience.preferences.monthStart));
const homePeriodLabel = computed(() => experience.preferences.range === 'month' && homeMonth.value !== currentAccountingMonth.value ? date(homeMonth.value).format('M月') : '本月');
const homeRange = computed(() => ledgerMonthRange(homeMonth.value,scope.timeZone,experience.preferences.monthStart));
const homeEntries = computed(() => entries.value.filter(item => item.time >= homeRange.value.start.unix() && item.time <= homeRange.value.end.unix()));
const homeBudget = computed(() => { const reports = budgetReports(workspace.budgets.filter(item => !books.selectedBookIds.length || books.selectedBookIds.includes(item.bookId)),entries.value,homeMonth.value,workspace.preferences).filter(item => item.budget.kind === 'monthly' && item.budget.categoryId === '0'); return reports.length ? reports.some(item => item.remaining === null) ? null : reports.reduce((sum,item) => sum.plus(item.remaining!),new LedgerDecimal(0)).toString() : undefined; });
const budgetLink = computed(() => `/statistics/budgets?bookId=${books.defaultBookId}`);
function homeBillsLink(type:number) { return `/transaction/list?minTime=${homeRange.value.start.unix()}&maxTime=${homeRange.value.end.unix()}&type=${type}&bookIds=${books.selectedBookIds.join(',')}`; }
function shiftHome(amount:number) { homeMonth.value = moment(homeMonth.value,'YYYY-MM').add(amount,'month').format('YYYY-MM'); homeLimit.value = 100; animateSettle(ledgerListEl.value, { x: amount * 14, duration: 170 }); }
async function openTemplateQuickSheet(): Promise<void> {
    try { await templatesStore.loadAllTemplates({ templateType: TemplateType.Normal.type, force: false }); } catch { /* ignore */ }
    const list = templatesStore.allVisibleTemplates[TemplateType.Normal.type] || [];
    if (!list.length) {
        props.f7router.navigate('/template/list');
        return;
    }
    f7.actions.create({
        buttons: [
            [{ text: '选择模板快速记账', label: true }, ...list.slice(0, 8).map(t => ({ text: t.name, onClick: () => props.f7router.navigate(`/transaction/add?templateId=${t.id}`) })), { text: '管理全部模板', onClick: () => props.f7router.navigate('/template/list') }],
            [{ text: '取消', color: 'gray' }]
        ]
    }).open();
}
function openBillActions(item: LedgerEntry): void {
    const targetId = item.transferFeeParentId && item.transferFeeParentId !== '0' ? item.transferFeeParentId : item.id;
    const isOrdinary = !item.investment && !item.refundExcess && !item.wallet;
    const primaryButtons: { text: string; color?: string; label?: boolean; onClick?: () => void }[] = [
        { text: `${item.title} · ${item.hidden ? '••••' : ledgerMoney(item.amount, false)}`, label: true }
    ];
    if (item.investment) {
        primaryButtons.push({ text: '前往投资记录处理', onClick: () => props.f7router.navigate(`/investments/record?action=revise&eventId=${encodeURIComponent(item.investmentEventId || '')}`) });
    } else if (item.wallet) {
        primaryButtons.push({ text: '编辑钱包收支', onClick: () => props.f7router.navigate(`/crypto/entry?eventId=${encodeURIComponent(item.investmentEventId || '')}&action=revise`) });
    } else if (item.refundExcess && item.refundExpenseId) {
        primaryButtons.push({ text: '查看原支出与退款', onClick: () => props.f7router.navigate(`/transaction/detail?id=${item.refundExpenseId}&type=3`) });
    } else {
        primaryButtons.push(
            { text: '编辑账单', onClick: () => props.f7router.navigate(`/transaction/edit?id=${targetId}&type=${item.type}`) },
            { text: '再记一笔', onClick: () => props.f7router.navigate(`/transaction/add?id=${targetId}&type=${item.type}&withTime=false&withGeoLocation=false`) },
            { text: '批量管理', onClick: () => props.f7router.navigate(`/transaction/list?selectId=${targetId}`) }
        );
    }
    if (isOrdinary) {
        primaryButtons.push({
            text: '删除账单',
            color: 'red',
            onClick: () => {
                const refundMsg = item.refundedAmount ? '删除原账单将同时删除关联退款，并撤销退款到账。' : '删除后账户余额将相应恢复。';
                showConfirm(`确定删除这笔「${item.title}」账单？${refundMsg}`, async () => {
                    try {
                        const result = await services.deleteTransaction({ id: targetId });
                        if (!result.data.success || !result.data.result) throw new Error('删除失败');
                        transactionsStore.updateStoreInvalidState({ accountList: true, transactionList: true, overview: true, statistics: true, explorer: true, reconciliationStatement: true });
                        showToast('账单已删除');
                        await refresh();
                    } catch (cause) {
                        showToast(investmentError(cause));
                    }
                }, undefined, { title: '删除账单', confirmText: '删除', destructive: true, raw: true });
            }
        });
    }
    f7.actions.create({ buttons: [primaryButtons, [{ text: '取消', color: 'gray' }]] }).open();
}
const { month, selectedDay } = storeToRefs(scope);
const { currentDay } = storeToRefs(scope);
const selectedDayLabel = computed(() => selectedDay.value === currentDay.value ? '今天' : selectedDay.value === date(currentDay.value,'YYYY-MM-DD').subtract(1,'day').format('YYYY-MM-DD') ? '昨天' : date(selectedDay.value,'YYYY-MM-DD').format('M月D日'));
const showBooks = ref(false), active = ref(false);
function date(value: string, format = 'YYYY-MM'): moment.Moment { return moment.tz(value,format,true,scope.timeZone); }
watch(() => (isCalendar.value ? calendarBookIds.value : books.selectedBookIds).join(','), () => { if (active.value) void refresh(); });
watch(() => [scope.timeZone, currentDay.value], () => { if (active.value) void refresh(); });
watch(monetaryIncomeRevision, () => { if (active.value) void refresh(); });
const addLink = computed(() => isCalendar.value ? `/transaction/add?time=${date(selectedDay.value,'YYYY-MM-DD').hour(12).unix()}&noTransactionDraft=true` : '/transaction/add');
const totals = computed(() => ledgerTotals(isCalendar.value ? entries.value : homeEntries.value));
const visibleEntries = computed(() => isCalendar.value ? displayEntries.value.filter(item => item.day === selectedDay.value) : experience.preferences.range === 'all' ? displayEntries.value : displayEntries.value.filter(item => item.time >= homeRange.value.start.unix() && item.time <= homeRange.value.end.unix()));
const weekStart = computed(() => preferences.value.calendarWeekStart);
const weekDays = computed(() => Array.from({ length: 7 }, (_, index) => ['日','一','二','三','四','五','六'][(index + weekStart.value) % 7]));
const firstWeekday = computed(() => (date(month.value).day() - weekStart.value + 7) % 7);
const days = computed(() => Array.from({ length: date(month.value).daysInMonth() }, (_, index) => {
    const key = `${month.value}-${String(index + 1).padStart(2,'0')}`;
    const items = entries.value.filter(item => item.day === key);
    return { key, number: index + 1, hasFlows: items.some(item => item.type === 2 || item.type === 3), totals: ledgerTotals(items), due: visibleDue.value.filter(item => item.date === key) };
}));
function display(value: string, complete = totals.value.complete): string { return loading.value || !complete ? '—' : ledgerMoney(value, false); }
function compact(value: string, direction: number): string { const amount = new LedgerDecimal(value).mul(direction); return `${direction > 0 ? '+' : amount.isZero() ? '-' : ''}${amount.abs().gte(10000) ? `${amount.div(10000).toFixed(1)}万` : amount.toFixed(amount.isInteger() ? 0 : 2)}`; }
async function refresh(done?: () => void): Promise<void> {
    let requestedDate = date(isCalendar.value ? month.value : currentDay.value.slice(0,7));
    if (!requestedDate.isValid()) { requestedDate = moment().tz(scope.timeZone); month.value = requestedDate.format('YYYY-MM'); selectedDay.value = requestedDate.format('YYYY-MM-DD'); }
    try { await Promise.all([load(isCalendar.value ? requestedDate.clone().startOf('month').unix() : 0, isCalendar.value ? requestedDate.clone().endOf('month').unix() : moment().tz(scope.timeZone).add(100,'years').unix()), ...(isCalendar.value ? [loadDue()] : [workspace.load(true)])]); }
    catch(cause) { error.value = investmentError(cause); }
    finally { if (typeof done === 'function') done(); }
}
function pullRefresh(done?: () => void):void { if (!isCalendar.value && experience.preferences.pullAction === 'add') { done?.(); props.f7router.navigate(addLink.value); } else void refresh(done); }
function selectBook(id: string): void { books.selectedBookIds = id ? [id] : []; }
async function onPageAfterIn(): Promise<void> {
    scope.syncClock(); active.value = true;
    if (isCalendar.value && scope.calendarBookIds === null) {
        try { await books.loadBooks(); }
        catch (cause) { error.value = investmentError(cause); return; }
        if (!active.value) return;
        const saved = experience.preferences.calendarRememberedBooks;
        const valid = saved.filter(id => books.allBooks.some(book => book.id === id));
        scope.calendarBookIds = experience.preferences.calendarRememberBooks && (!saved.length || valid.length) ? valid : [...books.selectedBookIds];
    }
    if (active.value) await refresh();
}
function selectCalendarDay(dayKey: string): void {
    if (selectedDay.value === dayKey) return;
    selectedDay.value = dayKey;
    animateSettle(ledgerListEl.value, { y: 5, duration: 150 });
}
function monthChanged(): void { if (!date(month.value).isValid()) return; selectedDay.value = month.value === currentDay.value.slice(0,7) ? currentDay.value : `${month.value}-01`; void refresh(); }
function shift(direction: number): void { const next = date(month.value).add(direction,'month'); if (next.year() < 1900 || next.year() > 9999) return; month.value = next.format('YYYY-MM'); animateSettle(calendarGridEl.value, { x: direction * 18, duration: 180 }); monthChanged(); }
function today(): void {
    const prev = month.value;
    const nowMonth = moment().tz(scope.timeZone).format('YYYY-MM');
    month.value = nowMonth;
    selectedDay.value = moment().tz(scope.timeZone).format('YYYY-MM-DD');
    if (prev !== nowMonth) animateSettle(calendarGridEl.value, { x: nowMonth > prev ? 18 : -18, duration: 180 });
    else animateSettle(ledgerListEl.value, { y: 5, duration: 150 });
    void refresh();
}
function lunar(key: string): string {
    const [year,month,day] = key.split('-').map(Number);
    const info = getChineseYearMonthDayInfo({ year: year!, month: month!, day: day! }, DEFAULT_CONTENT);
    if (!info) return '';
    const holidays: Record<string,string> = { '1-1': '春节', '1-15': '元宵', '5-5': '端午', '7-7': '七夕', '8-15': '中秋', '9-9': '重阳', '12-8': '腊八' };
    return (!info.isLeapMonth && holidays[`${info.month}-${info.day}`]) || getChineseCalendarAlternateDisplayDate(info).displayDate;
}
type CalendarDay = typeof days.value[number];
function dayLabel(day: CalendarDay): string { return `${day.key}，${day.totals.complete ? `收入${ledgerMoney(day.totals.income,false)}，支出${ledgerMoney(day.totals.expense,false)}` : '部分金额待换算'}${day.due.length ? `，${day.due.length}项待处理` : ''}`; }
const maxDailyAmount = computed(() => days.value.reduce((max,day) => LedgerDecimal.max(max,new LedgerDecimal(day.totals.income).abs().plus(new LedgerDecimal(day.totals.expense).abs())),new LedgerDecimal(1)));
function heatStyle(day: CalendarDay): Record<string,string> {
    if (!preferences.value.calendarHeatmap || !day.hasFlows || !day.totals.complete || day.key === selectedDay.value) return {};
    const ratio = new LedgerDecimal(day.totals.income).abs().plus(new LedgerDecimal(day.totals.expense).abs()).div(maxDailyAmount.value);
    return { backgroundColor: `color-mix(in srgb, var(--cy-accent) ${ratio.mul(28).plus(4).toFixed(1)}%, transparent)` };
}
function openBookSelector(): void { draftBooks.value = calendarBookDraft(calendarBookIds.value, books.allBooks); bookSelectorError.value = ''; showBookSelector.value = true; }
function applyBooks(): void {
    const selected = calendarBookScope(draftBooks.value, books.allBooks);
    if (selected === null) { bookSelectorError.value = draftBooks.value.length ? '最多可同时选择 100 个账本' : '请至少选择一个账本'; return; }
    scope.calendarBookIds = selected;
    experience.set('calendarRememberBooks', true);
    experience.set('calendarRememberedBooks', [...selected]);
    showBookSelector.value = false;
}
async function loadDue(): Promise<void> {
    const version = ++dueGeneration, requestedMonth = month.value; dueItems.value = []; dueError.value = '';
    try {
        const [manual, credit, debts] = await Promise.all([calendarEvents.list(requestedMonth), creditAccounts.reports(scope.timeZone), debtAccounts.reports(), accounts.loadAllAccounts({ force: false }).catch(() => undefined)]);
        if (version !== dueGeneration) return;
        dueItems.value = mergeDebtDue([...manual, ...calendarCreditEvents(credit, requestedMonth)], debts, accounts.allAccountsMap).filter(item => item.date.startsWith(requestedMonth));
        if (manual.some(item => item.id.startsWith('deposit-'))) {
            try {
                const deposits = await assetTools.deposits();
                if (version !== dueGeneration) return;
                const byId = new Map(deposits.map(item => [item.id, item]));
                dueItems.value = dueItems.value.map(item => {
                    const deposit = byId.get(item.id);
                    if (!deposit) return item;
                    const interest = `预计收益 ${ledgerMoney(deposit.expectedInterest, false)} ${deposit.currency}`;
                    return { ...item, note: item.note ? `${item.note} · ${interest}` : interest };
                });
            } catch {
                if (version === dueGeneration) dueError.value = '定存预计收益暂不可用';
            }
        }
    }
    catch(cause) { if (version === dueGeneration) dueError.value = investmentError(cause); }
}
function dueLink(item: CalendarEvent): string {
    if (item.id.startsWith('credit:')) return `/account/credit?id=${encodeURIComponent(item.accountId)}&month=${encodeURIComponent(item.id.slice(item.id.lastIndexOf(':') + 1))}`;
    if (item.id.startsWith('debt:')) return `/account/debt?id=${encodeURIComponent(item.accountId)}`;
    if (item.id.startsWith('deposit-')) return `/assets/tool?tool=deposits&accountId=${encodeURIComponent(item.accountId)}`;
    return `/calendar/due?date=${item.date}&id=${encodeURIComponent(item.id)}&bookIds=${encodeURIComponent(calendarBookIds.value.join(','))}`;
}
function dueTitle(item: CalendarEvent): string {
    if (item.id.startsWith('credit:')) return '信用卡待还';
    if (item.id.startsWith('debt:')) return '债务待还';
    if (item.id.startsWith('deposit-')) return '定存到期';
    if (item.transactionId && item.transactionId !== '0') return '借款约定还款';
    return item.kind === 'repayment' ? '手动还款提醒' : '手动定存提醒';
}
let touchX = 0, touchY = 0;
function touchStart(event: TouchEvent): void { touchX = event.changedTouches[0]?.clientX || 0; touchY = event.changedTouches[0]?.clientY || 0; }
function touchEnd(event: TouchEvent): void { const touch = event.changedTouches[0]; if (touch && Math.abs(touch.clientX-touchX)>70 && Math.abs(touch.clientY-touchY)<45) shift(touch.clientX>touchX ? -1 : 1); }
</script>
<style scoped>
.cy-home-page{--f7-navbar-height:50px}.cy-home-page .cy-page-body{padding:6px 15px 22px}.cy-home-page .cy-primary-action{min-height:45px;margin-bottom:14px;border-radius:9px;font-weight:400;padding:9px 12px}.cy-home-page .cy-primary-action .cy-add-icon{width:25px;height:25px;flex-basis:25px}.cy-home-hero{background:#d6ebe4;border-radius:11px;min-height:145px;box-sizing:border-box;padding:13px 15px;margin-bottom:15px;color:#365651}.cy-home-hero a{display:block;color:inherit}.cy-home-hero p{font-size:13px;line-height:1.4}.cy-home-hero .cy-hero-stats{gap:14px;margin-bottom:12px}.cy-home-hero .cy-hero-stats strong{font-size:18px;font-weight:550;margin-top:3px;line-height:1.3}.cy-home-hero .cy-major{font-size:29px;font-weight:600;line-height:1.3;margin-top:1px;display:block}.cy-home-tools{gap:0}.cy-home-tools .link{width:37px;min-width:37px;padding:0}.cy-home-tools .icon{font-size:24px}.cy-home-nav-left{max-width:calc(100% - 100px)!important}.cy-home-page .cy-book-trigger{font-size:18px;font-weight:500;gap:9px}.cy-home-page :deep(.navbar-inner){padding-inline:13px}.cy-home-page :deep(.navbar .left),.cy-home-page :deep(.navbar .right){background:transparent;box-shadow:none;border-radius:0;backdrop-filter:none}.cy-home-month{display:flex;align-items:center;justify-content:center;margin:12px 0;gap:10px}.cy-home-month button{border:0;background:transparent;font-size:25px;color:var(--cy-muted);padding:5px 15px}.cy-home-month input{width:128px;background:transparent;border:0;font:inherit;font-size:15px}.cy-home-load{width:100%;background:var(--cy-card);border:0;border-radius:9px;color:var(--cy-muted);padding:13px;font-size:12px;margin-bottom:12px}.cy-home-fab-left{left:20px;right:auto}.dark .cy-home-hero{background:#25433f;color:#d9e9e3}
.cy-home-popover{width:250px;--f7-list-bg-color:var(--cy-card);background:var(--cy-card);border-radius:10px}.cy-home-menu-content{padding:7px 13px 13px;color:var(--cy-ink)}.cy-home-menu-content>a{display:flex;align-items:center;gap:14px;color:inherit;min-height:46px;font-size:15px;padding-inline:7px}.cy-home-menu-content>a .icon{font-size:21px;color:var(--cy-muted)}.cy-home-menu-content>p{font-size:11px;color:var(--cy-muted);padding:9px 0}.cy-home-menu-content>div{display:grid;grid-template-columns:1fr 1fr;gap:8px}.cy-home-menu-content>div>button,.cy-home-menu-content>div>a{display:flex;align-items:center;justify-content:center;gap:6px;background:var(--cy-bg);color:var(--cy-ink);border:0;border-radius:7px;padding:10px 3px;font-size:13px}.cy-home-menu-content>div .icon{font-size:16px}.cy-home-menu-content button[aria-pressed=true]{color:var(--cy-accent);background:var(--cy-soft)}
.cy-home-nav-left{min-width:0;max-width:calc(100% - 60px)}
.cy-book-trigger{display:flex;align-items:center;gap:10px;min-width:0;max-width:100%;min-height:44px;padding:0 4px;background:transparent;border:0;color:var(--cy-ink);font:inherit;font-size:21px;font-weight:650;text-align:left;cursor:pointer}
.cy-book-trigger span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.cy-book-trigger .icon{flex:none;font-size:23px}.cy-book-trigger .cy-book-chevron{font-size:12px;color:var(--cy-muted)}
.cy-primary-action{box-sizing:border-box;height:auto;min-height:56px;padding:14px 18px;line-height:1.5;white-space:normal;text-align:center}
.cy-primary-action .cy-add-icon{display:block;flex:0 0 26px;width:26px;height:26px;fill:currentColor}
.cy-primary-action span{position:static;flex:0 1 auto;min-width:0;margin:0;line-height:1.5}
.cy-hero-stats small{font-size:11px;font-weight:400}
.cy-calendar-page,.cy-calendar-navbar{--f7-navbar-height:82px}.cy-calendar-nav-left{flex:1;min-width:0}.cy-calendar-caption{min-width:0;width:100%}.cy-calendar-caption>button{background:transparent;border:0;padding:0;color:var(--cy-ink);font:inherit;font-size:23px;font-weight:600;line-height:1.5}.cy-calendar-caption>button small{font-size:12px;margin-left:7px;color:var(--cy-muted)}.cy-calendar-totals{display:flex;gap:9px;font-size:11px;line-height:1.7;color:var(--cy-muted);flex-wrap:wrap}.cy-calendar-totals b,.cy-selected-heading b{font-weight:500}.cy-calendar-tools{gap:8px;margin-left:4px!important}.cy-calendar-tools button{display:flex;align-items:center;justify-content:center;border:0;background:transparent;color:var(--cy-ink);font:inherit;font-size:23px;min-width:30px;height:44px;padding:0}.cy-calendar-tools .link{padding:0;width:32px;min-width:32px}.cy-calendar-tools svg{width:25px;height:25px;fill:currentColor}.cy-calendar-tools .icon{font-size:23px}
.cy-calendar-body{padding:8px 12px 110px}.cy-calendar-panel{margin-bottom:20px}.cy-calendar-grid{display:grid;grid-template-columns:repeat(7,minmax(0,1fr));gap:4px 2px;touch-action:pan-y}.cy-weekday{text-align:center;color:var(--cy-muted);font-size:14px;padding:11px 0 15px}.cy-calendar-grid button{position:relative;background:transparent;color:var(--cy-ink);border:0;border-radius:7px;height:73px;padding:6px 0 9px;display:flex;flex-direction:column;align-items:center;min-width:0;gap:1px}.cy-calendar-grid button strong{font-size:20px;font-weight:550;line-height:25px}.cy-calendar-grid button small{display:block;font-size:10px;line-height:14px;max-width:100%;overflow:hidden;white-space:nowrap}.cy-calendar-grid button .cy-lunar{margin-top:6px;color:var(--cy-muted);font-size:11px}.cy-calendar-grid .cy-today strong{color:var(--cy-accent);font-weight:750}.cy-calendar-grid button[aria-pressed=true]{background:var(--cy-accent);color:white}.cy-calendar-grid button[aria-pressed=true] *{color:white!important}.cy-due-dots{display:flex;gap:3px;position:absolute;bottom:3px}.cy-due-dots i{width:4px;height:4px;border-radius:50%}.cy-repay-dot{background:var(--cy-expense)}.cy-deposit-dot{background:var(--cy-accent)}.cy-calendar-grid button[aria-pressed=true] i{background:white}
.cy-selected-heading{display:flex;align-items:center;justify-content:space-between;gap:10px;margin:4px 0 10px;flex-wrap:wrap}.cy-selected-heading h2{font-size:14px;font-weight:500;margin:0}.cy-selected-heading h2 small{font-size:12px;color:var(--cy-muted);margin-left:5px}.cy-selected-heading>span{display:flex;gap:9px;font-size:11px;flex-wrap:wrap}.cy-due-card{border:1px solid var(--cy-line);border-radius:14px;background:var(--cy-card);padding:0 13px;margin-bottom:12px}.cy-due-card>a{display:flex;align-items:center;gap:12px;padding:14px 0;border-bottom:1px solid var(--cy-line);color:var(--cy-ink)}.cy-due-card>a:last-child{border:0}.cy-due-card>a>span{flex:1;min-width:0}.cy-due-card strong{font-size:13px;font-weight:500;overflow-wrap:anywhere}.cy-due-card small{display:block;font-size:11px;color:var(--cy-muted);margin-top:4px}.cy-due-card b{font-size:15px;font-weight:500;text-align:right}
.cy-calendar-sheet{height:auto;max-height:75vh;background:var(--cy-card);color:var(--cy-ink)}.cy-calendar-sheet :deep(.sheet-modal-inner){padding-top:var(--f7-toolbar-height);max-height:70vh;overflow:auto}.cy-month-choice{display:flex;align-items:center;justify-content:space-between;gap:15px;padding:30px 20px 50px}.cy-month-choice button{width:42px!important;flex:0 0 42px;height:42px;border:1px solid var(--cy-line);border-radius:9px;background:var(--cy-card);color:var(--cy-ink);font-size:24px;padding:0}.cy-month-choice input{flex:1;width:160px;text-align:center;font:inherit;border:0;background:transparent;color:inherit;min-width:0}.cy-book-options{padding:14px 20px 30px}.cy-book-options label{display:flex;align-items:center;gap:12px;padding:13px 0;font-size:16px}.cy-book-options input{accent-color:var(--cy-accent);width:18px;height:18px}
.cy-calendar-panel{margin-bottom:14px}.cy-weekday{font-size:12px;padding:7px 0 10px}.cy-calendar-grid button{height:58px;padding:3px 0 5px}.cy-calendar-grid button strong{font-size:16px;line-height:21px}.cy-calendar-grid button small{line-height:12px}.cy-calendar-grid button .cy-lunar{margin-top:4px;font-size:10px}
.cy-calendar-navbar :deep(.left),.cy-calendar-navbar :deep(.right){box-shadow:none!important;backdrop-filter:none!important;background:transparent!important;border-radius:0!important}.cy-calendar-navbar :deep(.navbar-inner){padding-inline:14px}
.cy-ptr-add{display:flex;align-items:center;justify-content:center;gap:6px;width:100%!important;left:0!important;margin-left:0!important;font-size:13px;color:var(--cy-accent);font-weight:500;opacity:0;pointer-events:none;transition:opacity var(--cy-dur-fast,150ms) var(--cy-ease-out,ease)}.cy-ptr-arrow{display:inline-block;transition:transform 180ms var(--cy-ease-spring,cubic-bezier(0.34,1.56,0.64,1))}.cy-ptr-text{transition:opacity var(--cy-dur-fast,150ms) var(--cy-ease-out,ease)}.cy-ptr-add .cy-ptr-release{position:absolute;opacity:0}:deep(.ptr-pull-down) .cy-ptr-add,:deep(.ptr-pull-up) .cy-ptr-add,:deep(.ptr-refreshing) .cy-ptr-add{opacity:1}:deep(.ptr-pull-up) .cy-ptr-arrow,:deep(.ptr-refreshing) .cy-ptr-arrow{transform:rotate(180deg)}:deep(.ptr-pull-up) .cy-ptr-add .cy-ptr-pull,:deep(.ptr-refreshing) .cy-ptr-add .cy-ptr-pull{opacity:0}:deep(.ptr-pull-up) .cy-ptr-add .cy-ptr-release,:deep(.ptr-refreshing) .cy-ptr-add .cy-ptr-release{position:static;opacity:1}
</style>
