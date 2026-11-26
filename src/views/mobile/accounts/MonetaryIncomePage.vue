<template>
    <f7-page class="cy-main-page cy-mobile-surface cy-fund-binding" @page:beforein="load">
        <f7-navbar title="绑定货币型基金">
            <f7-nav-left><f7-link aria-label="取消" :disabled="working" @click="close"><f7-icon f7="xmark" /></f7-link></f7-nav-left>
            <f7-nav-right><f7-link aria-label="保存基金绑定" :disabled="working || loading" @click="save"><f7-preloader v-if="working" /><f7-icon v-else f7="checkmark" /></f7-link></f7-nav-right>
        </f7-navbar>
        <main class="cy-fund-body">
            <p v-if="error" class="cy-fund-error" role="alert">{{ error }} <button v-if="searchQuery && !selected" type="button" :disabled="searching || working" @click="searchLater(searchQuery)">重新查询</button></p>
            <div v-if="loading" class="cy-fund-loading"><f7-preloader /></div>
            <form v-else @submit.prevent="save">
                <section v-if="selected" class="cy-fund-card cy-fund-selected">
                    <div><small>已选货币基金</small><strong>{{ selected.name }}</strong><span>代码 {{ selected.symbol }}</span></div>
                    <button type="button" :disabled="working" @click="clearSelected">重选</button>
                </section>
                <section v-else class="cy-fund-card">
                    <label><span>搜索基金</span><input v-model="keyword" placeholder="输入基金名称或 6 位代码" aria-label="搜索货币基金名称或代码" maxlength="50" :disabled="working" @input="searchLater(keyword)" /></label>
                </section>
                <section v-if="!selected && (searching || candidates.length)" class="cy-fund-card cy-fund-results" aria-live="polite">
                    <p v-if="searching">正在查询货币基金…</p>
                    <button v-for="fund in candidates" :key="fund.providerId" type="button" @click="choose(fund)"><span>{{ fund.name }}</span><small>{{ fund.symbol }}</small></button>
                </section>
                <section class="cy-fund-card cy-fund-date">
                    <label><span>开始计息日期</span><input type="date" v-model="startDate" aria-label="开始计息日期" min="2025-01-01" :max="today" :disabled="working" required /><f7-icon f7="chevron_right" /></label>
                </section>
                <div class="cy-fund-hint">
                    <p>说明：</p>
                    <p>按绑定基金公开的每万份收益，每日自动结算一笔收益账单。</p>
                    <p>编辑最近一条自动收益账单后，后续收益会自动沿用其分类、账本和标签。</p>
                    <p>余额宝可在左上角查看对应基金名称；零钱通可在「七日年化收益率」下方查看。</p>
                </div>
                <button v-if="saved?.enabled" class="cy-fund-unbind" type="button" :disabled="working" @click="unbind">解除基金绑定（保留历史收益）</button>
            </form>
        </main>
    </f7-page>
</template>
<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue';
import type { Router } from 'framework7/types';
import moment from 'moment-timezone';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import { monetaryIncome, type MonetaryBinding, type MonetaryFund, type MonetaryDraft } from '@/lib/monetary-income.ts';
import type { Account } from '@/models/account.ts';
import { investmentError } from '@/lib/investments.ts';
import { getTimeZone } from '@/lib/settings.ts';
import { getBrowserTimezoneName } from '@/lib/datetime.ts';
import { useAccountsStore } from '@/stores/account.ts';
const props = defineProps<{ f7route?: Router.Route; f7router?: Router.Router; draftAccount?: Account; draft?: MonetaryDraft }>();
const emit = defineEmits<{ configured: [draft: MonetaryDraft]; cancel: [] }>();
const { showToast, showConfirm } = useI18nUIComponents();
const accounts = useAccountsStore();
const accountId = computed(() => props.f7route?.query['id'] || '');
const account = computed(() => props.draftAccount || accounts.allAccountsMap[accountId.value]);
const saved = ref<MonetaryBinding>(), selected = ref<MonetaryFund>(), candidates = ref<MonetaryFund[]>([]);
const keyword = ref(''), startDate = ref(''), error = ref(''), searchQuery = ref('');
const loading = ref(false), working = ref(false), searching = ref(false);
const zone = getTimeZone() || getBrowserTimezoneName();
const today = moment().tz(zone).format('YYYY-MM-DD');
let timer: ReturnType<typeof setTimeout> | undefined, searchVersion = 0;
onMounted(() => { if (props.draftAccount) void load(); });
onBeforeUnmount(() => { clearTimeout(timer); searchVersion++; });
function close(): void { if (props.draftAccount) emit('cancel'); else props.f7router?.back(); }
async function load(): Promise<void> {
    if (loading.value || working.value) return;
    loading.value = true; error.value = '';
    try {
        const [, items] = await Promise.all([accounts.loadAllAccounts({ force: false }).catch(e => { if (!e?.isUpToDate) throw e; }), props.draftAccount ? Promise.resolve([]) : monetaryIncome.list()]);
        if (!account.value || account.value.currency !== 'CNY' || ![1, 2, 4, 8].includes(account.value.category)) throw Error('请选择人民币资金账户');
        saved.value = items.find(b => b.accountId === accountId.value);
        const b = saved.value;
        const fund = props.draft?.fund || (b ? { name: b.name, symbol: b.code, providerId: b.code } : undefined);
        if (fund) choose(fund);
        startDate.value = props.draft?.startDate || b?.startDate || moment().tz(zone).subtract(1, 'day').format('YYYY-MM-DD');
    } catch (cause) { error.value = investmentError(cause); }
    finally { loading.value = false; }
}
function clearSelected(): void {
    selected.value = undefined;
    keyword.value = '';
    candidates.value = [];
    error.value = '';
}
function searchLater(query: string): void {
    searchQuery.value = query.trim();
    clearTimeout(timer); selected.value = undefined; candidates.value = []; error.value = '';
    const version = ++searchVersion;
    if (query.trim().length < 2) { searching.value = false; return; }
    searching.value = true;
    timer = setTimeout(async () => {
        try { const list = await monetaryIncome.search(query.trim()); if (version !== searchVersion) return; candidates.value = list; if (!list.length) error.value = '没有找到匹配的货币基金，请核对名称或 6 位代码'; }
        catch (cause) { if (version === searchVersion) error.value = investmentError(cause); }
        finally { if (version === searchVersion) searching.value = false; }
    }, 350);
}
function choose(fund: MonetaryFund): void {
    clearTimeout(timer); searchVersion++; searching.value = false;
    selected.value = fund; keyword.value = `${fund.name} (${fund.symbol})`; candidates.value = []; error.value = '';
}
async function save(): Promise<void> {
    if (working.value || loading.value) return;
    if (!selected.value) { error.value = '请先输入并从查询结果中选择一只货币基金'; return; }
    const draft: MonetaryDraft = { fund: selected.value, code: selected.value.providerId, startDate: startDate.value, bookId: props.draft?.bookId || saved.value?.bookId || '', categoryId: props.draft?.categoryId || saved.value?.categoryId || '', timeZone: zone, enabled: true };
    if (props.draftAccount) { emit('configured', draft); return; }
    working.value = true; error.value = '';
    try {
        saved.value = await monetaryIncome.save({ accountId: accountId.value, ...draft });
        void monetaryIncome.sync(accountId.value, true).catch(() => undefined);
        showToast('已绑定货币基金收益');
        close();
    } catch (cause) { error.value = investmentError(cause); }
    finally { working.value = false; }
}
function unbind(): void {
    if (working.value) return;
    showConfirm('解除后停止自动生成收益，已有收益账单会完整保留。', async () => {
        working.value = true; error.value = '';
        try { await monetaryIncome.pause(accountId.value); showToast('已解除基金绑定'); close(); }
        catch (cause) { error.value = investmentError(cause); }
        finally { working.value = false; }
    }, undefined, { title: '解除基金绑定', confirmText: '解除绑定', destructive: true, raw: true });
}
</script>
<style scoped>
.cy-fund-body{max-width:640px;margin:auto;padding:14px 14px 40px}.cy-fund-card{background:var(--cy-card);border-radius:12px;overflow:hidden;margin-bottom:15px}.cy-fund-card label{display:flex;align-items:center;gap:14px;min-height:53px;padding:0 15px;font-size:16px}.cy-fund-card label>span{flex:0 0 auto;letter-spacing:1px}.cy-fund-card input{min-width:0;width:100%;border:0;outline:0;background:transparent;color:var(--cy-ink);font:inherit;caret-color:var(--cy-accent);padding:14px 0}.cy-fund-card input::placeholder{color:var(--cy-muted)}.cy-fund-selected{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:14px 16px}.cy-fund-selected small{display:block;font-size:11px;color:var(--cy-muted)}.cy-fund-selected strong{display:block;font-size:16px;font-weight:500;margin:4px 0 2px}.cy-fund-selected span{font-size:12px;color:var(--cy-muted)}.cy-fund-selected button{border:1px solid var(--cy-line);background:var(--cy-bg);color:var(--cy-accent);border-radius:8px;padding:7px 14px;font-size:13px;flex:none}.cy-fund-date{margin-top:16px}.cy-fund-date input{text-align:right;font-size:15px}.cy-fund-date .icon{font-size:16px;color:var(--cy-muted);margin-left:-7px}.cy-fund-hint{color:var(--cy-muted);font-size:13px;line-height:1.85;margin-top:14px}.cy-fund-hint p{margin:0 0 4px}.cy-fund-results button{display:flex;justify-content:space-between;align-items:center;width:100%;text-align:left;gap:12px;min-height:48px;padding:12px 15px;background:transparent;border:0;color:var(--cy-ink);font:inherit}.cy-fund-results small{color:var(--cy-muted)}.cy-fund-results p{padding:12px 15px;color:var(--cy-muted)}.cy-fund-unbind{display:block;width:100%;background:var(--cy-card);border:1px solid color-mix(in srgb,var(--cy-expense) 35%,var(--cy-line));border-radius:11px;color:var(--cy-expense);padding:13px;margin:24px 0 0;font-size:14px}.cy-fund-error{color:var(--cy-expense);font-size:14px;margin-bottom:10px}.cy-fund-loading{padding:30px;text-align:center}
.cy-fund-error button{border:0;background:none;color:var(--cy-accent);font:inherit;padding:4px 6px;text-decoration:underline}
</style>
