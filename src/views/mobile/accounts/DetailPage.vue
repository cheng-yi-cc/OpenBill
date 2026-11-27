<template>
    <f7-page class="cy-mobile-surface cy-account-detail" ptr @ptr:refresh="refresh" @page:afterin="refresh">
        <f7-navbar :title="account?.name || '账户'" back-link="资产"><f7-nav-right><f7-link @click="statistics">统计</f7-link><AccountOptionsMenu v-if="account" :edit-href="`/account/edit?id=${id}`" :items="menuItems" :target="{id,name:account.name,kind:'cash',href:`/transaction/list?accountIds=${id}`}" @action="handleAction" @cleared="refresh" @deleted="deleted" /></f7-nav-right></f7-navbar>
        <main class="cy-page-body">
            <p v-if="error" class="cy-message" role="alert">{{ error }} <button @click="refresh()">重试</button></p>
            <template v-if="account">
                <section class="cy-panel cy-account-balance"><button @click="showBalance=true"><span class="balance-label-row">{{ creditSurplus ? '溢缴款' : account.isLiability ? '当前欠款' : '账户余额' }}<span class="balance-edit-pill">校准 ✎</span></span><strong>{{ assetCurrencyMoney(creditSurplus ? new LedgerDecimal(balance).abs().toString() : balance, account.currency) }}</strong></button><div v-if="account.category===3" class="credit-balance-info"><span>可用额度 {{hasCreditLimit ? assetCurrencyMoney(availableCredit, account.currency) : '未设置'}}</span><span>总额度 {{hasCreditLimit ? assetCurrencyMoney(creditLimit, account.currency) : '未设置'}}</span></div><div v-if="depositSummary" class="deposit-balance-info">活期 {{assetCurrencyMoney(depositSummary.available, account.currency)}} · 定存 {{assetCurrencyMoney(depositSummary.principal, account.currency)}}</div></section>
                <p v-if="depositError || depositIssue" class="cy-message" role="alert">{{depositError || depositIssue}} <button @click="refresh()">重试</button></p>
                <f7-link v-if="depositSummary" class="cy-panel deposit-summary" :href="`/assets/tool?tool=deposits&accountId=${encodeURIComponent(id)}`"><span>定期存款<strong>{{assetCurrencyMoney(depositSummary.principal, account.currency)}}</strong></span><span>预计总收益<strong>{{assetCurrencyMoney(depositSummary.interest, account.currency)}}</strong></span><span>{{depositSummary.nextDate.slice(5)}} 到期<strong>{{assetCurrencyMoney(depositSummary.nextInterest, account.currency, true)}}</strong></span></f7-link>
                <nav v-if="account.category===3" class="cy-panel cy-account-entry cy-credit-entry"><f7-link :href="entryLink(3)"><f7-icon f7="creditcard" /><span>记消费</span></f7-link><f7-link :href="repayLink"><f7-icon f7="arrow_right_arrow_left" /><span>去还款</span></f7-link><f7-link :href="`/account/credit?id=${encodeURIComponent(id)}`"><f7-icon f7="doc_text" /><span>每期账单</span></f7-link></nav>
                <nav v-else class="cy-panel cy-account-entry"><f7-link :href="entryLink(3)"><f7-icon f7="doc_badge_plus" /><span>记一笔</span></f7-link><f7-link :href="entryLink(4)"><f7-icon f7="tray_arrow_up" /><span>转账</span></f7-link></nav>
                <p v-if="loading && !entries.length" class="cy-empty" role="status">正在读取流水…</p>
                <p v-else-if="!entries.length && !error" class="cy-empty">还没有流水</p>
                <LedgerDayList :entries="entries" account-view :partial-last-day="hasMore" />
                <button v-if="hasMore" class="cy-load-more" :disabled="loading" @click="loadAccount(true)">{{ loading ? '加载中…' : '查看更多流水' }}</button>
            </template>
        </main>
        <AssetBalanceSheet v-if="account" v-model:opened="showBalance" :account="account" @saved="refresh" />
    </f7-page>
</template>
<script setup lang="ts">
import {assetCurrencyMoney} from '@/lib/asset-visibility.ts';
import {computed,ref} from 'vue';
import {investmentError} from '@/lib/investments.ts';
import type {Router} from 'framework7/types';
import {useI18nUIComponents} from '@/lib/ui/mobile.ts';
import {useAccountsStore} from '@/stores/account.ts';
import {useLedgerScopeStore} from '@/stores/ledgerScope.ts';
import {LedgerDecimal,useMobileLedger} from '@/lib/mobile-ledger.ts';
import {assetTools,type FixedDeposit} from '@/lib/asset-tools.ts';
import {depositAccountSummary} from '@/lib/deposit-account-summary.ts';
import LedgerDayList from '@/components/mobile/LedgerDayList.vue';
import AccountOptionsMenu from '@/components/mobile/AccountOptionsMenu.vue';
import AssetBalanceSheet from '@/components/mobile/AssetBalanceSheet.vue';
const props=defineProps<{f7route:Router.Route;f7router:Router.Router}>();
const {showToast,showConfirm}=useI18nUIComponents();
const id=computed(()=>props.f7route.query['id']||''),accounts=useAccountsStore();
const account=computed(()=>accounts.allAccountsMap[id.value]);
const showBalance=ref(false);
const creditMain=computed(()=>accounts.allAccountsMap[account.value?.assetProfile.sharedLimitAccount||'']||account.value);
const creditLimit=computed(()=>new LedgerDecimal(creditMain.value?.creditCardLimit||0).div(100).toString());
const hasCreditLimit=computed(()=>new LedgerDecimal(creditLimit.value).gt(0));
const availableCredit=computed(()=>Object.values(accounts.allAccountsMap).filter(a=>a.id===creditMain.value?.id||a.assetProfile.sharedLimitAccount===creditMain.value?.id).reduce((s,a)=>s.plus(a.balance),new LedgerDecimal(0)).div(100).plus(creditLimit.value).toString());
const repayLink=computed(()=>{
 const source=accounts.allVisiblePlainAccounts.find(a=>[1,2,4,8].includes(a.category)&&!a.assetProfile.kind&&a.currency===account.value?.currency);
 const amount=new LedgerDecimal(balance.value).gt(0)?`&amount=${new LedgerDecimal(balance.value).mul(100).toFixed(0)}`:'';
 return `/transaction/add?type=4&accountId=${source?.id||''}&destinationAccountId=${encodeURIComponent(id.value)}${amount}&noTransactionDraft=true`;
});
const menuItems=computed(()=>[
 {title:'账单明细与导出',href:`/account/activity?id=${id.value}&mode=flow`},
 {title:'流水对账',href:`/account/reconciliation_statements?accountId=${id.value}`},
 ...(account.value?.category===3?[
  {title:'每期还款账单',href:`/account/credit?id=${id.value}`},
  {title:'分期管理',href:`/account/installments?id=${id.value}`},
  {title:'还款记录',href:`/account/activity?id=${id.value}&mode=repayments`}
 ]:[{title:'定期存款',href:`/assets/tool?tool=deposits&accountId=${id.value}`},{title:'转为信贷账户',action:'credit'}])
]);
function handleAction(action:string):void{if(action!=='credit'||!account.value)return;showConfirm('转换后将以欠款和可用额度展示，现有余额与历史流水完整保留。',async()=>{try{const a=account.value!.cloneSelf();a.category=3;a.assetProfile.group='信贷账户';await accounts.saveAccount({account:a,subAccounts:[],isEdit:true,clientSessionId:''});showToast('已转为信贷账户');await refresh();}catch(e){error.value=investmentError(e);}},undefined,{title:'转为信贷账户',confirmText:'确认转换',raw:true});}
const balance=computed(()=>new LedgerDecimal(account.value?.balance||'0').div(100).mul(account.value?.isLiability?-1:1).toString());
const creditSurplus=computed(()=>account.value?.category===3&&new LedgerDecimal(balance.value).lt(0));
const deposits=ref<FixedDeposit[]>([]), depositError=ref('');
const depositToday=computed(()=>useLedgerScopeStore().currentDay);
const depositIssue=computed(()=>account.value && deposits.value.some(d=>d.accountId===id.value&&!d.closed&&!d.settled&&d.startDate<=depositToday.value&&d.maturityDate>depositToday.value&&d.currency!==account.value!.currency)?'定存币种与账户不一致，请检查定存记录':'');
const depositSummary=computed(()=>{if(!account.value || account.value.isLiability || depositError.value || depositIssue.value)return null;return depositAccountSummary(deposits.value,id.value,account.value.currency,balance.value,depositToday.value);});
const {entries,loading,error,loadAccount,hasMore}=useMobileLedger({applyFilters:()=>false,accountId:()=>id.value});
let refreshGeneration=0;
async function refresh(done?:()=>void):Promise<void>{const version=++refreshGeneration;const accountId=id.value;depositError.value='';try{await Promise.all([loadAccount(),assetTools.deposits().then(rows=>{if(version===refreshGeneration&&id.value===accountId)deposits.value=rows;}).catch(cause=>{if(version===refreshGeneration){deposits.value=[];depositError.value=investmentError(cause);}})]);}finally{if(typeof done==='function')done();}}
function entryLink(type:number):string{return `/transaction/add?type=${type}&accountId=${encodeURIComponent(id.value)}`;}
function statistics():void{useLedgerScopeStore().accountId=id.value;props.f7router.navigate('/statistics');}
function deleted():void{props.f7router.navigate('/investments',{reloadAll:true});}
</script>
<style scoped>
.cy-account-detail :deep(.page-content){padding-bottom:30px}.cy-account-balance{padding:24px;margin-bottom:16px}.cy-account-balance p{color:var(--cy-muted);font-size:15px;margin:0 0 10px}.cy-account-balance strong{font-size:32px;font-weight:600;overflow-wrap:anywhere}.cy-account-entry{display:flex;padding:0;margin-bottom:22px;gap:0}.cy-account-entry :deep(a.link){display:flex;align-items:center;justify-content:center;gap:12px;min-height:56px;padding:0 12px;color:var(--cy-ink);font-size:14px;flex:1}.cy-account-entry:not(.cy-credit-entry) :deep(a.link:first-child){flex:1.6}.cy-account-entry :deep(.icon){font-size:20px;color:var(--cy-accent);width:auto;min-width:24px;height:24px;line-height:24px;padding-right:4px;flex:0 0 auto;text-align:center;overflow:visible}.cy-account-entry :deep(a.link i + span){margin-left:0}.cy-account-entry :deep(span){display:block;min-width:0;white-space:normal;line-height:1.4}.cy-load-more{display:block;width:100%;border:1px solid var(--cy-line);background:var(--cy-card);border-radius:12px;min-height:46px;font:inherit;color:var(--cy-accent)}
.cy-account-detail .cy-account-balance{padding:18px 16px;border:0;border-radius:12px;margin-bottom:14px;background:var(--cy-accent);color:#fff}.cy-account-balance button{display:block;width:100%;text-align:left;background:none;border:0;padding:0;color:#fff}.balance-label-row{display:flex!important;align-items:center;justify-content:space-between;font-size:14px;color:rgba(255,255,255,.88)}.balance-edit-pill{font-size:12px;padding:3px 10px;border-radius:999px;background:rgba(255,255,255,.18);color:#fff}.cy-account-balance button strong{display:block;font-size:27px;font-weight:500;margin-top:8px}.credit-balance-info{display:flex;justify-content:space-between;margin-top:16px;color:rgba(255,255,255,.9);font-size:12px}.cy-account-entry{border:0!important;border-radius:12px!important;margin-bottom:18px}.cy-account-detail :deep(.cy-day-card){border:0;border-radius:12px}.deposit-balance-info{margin-top:14px;font-size:12px;color:rgba(255,255,255,.9)}.deposit-summary{display:flex;justify-content:space-around;gap:8px;text-align:center;color:var(--cy-ink);border:0;margin-bottom:14px}.deposit-summary span{min-width:0;font-size:12px}.deposit-summary strong{display:block;font-size:16px;font-weight:500;margin-top:8px;overflow-wrap:anywhere}
</style>
