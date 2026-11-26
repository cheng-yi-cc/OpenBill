<template>
  <f7-page class="cy-mobile-surface cy-investment-page" ptr @ptr:refresh="refresh" @page:afterin="activate" @page:beforeout="live.stop">
    <f7-navbar :title="selectedAccount?.name || '理财'" back-link="资产">
      <f7-nav-right>
        <f7-link class="inv-nav-icon" :href="`/investments/statistics?accountId=${accountId}`" aria-label="理财统计"><f7-icon f7="chart_pie" /></f7-link>
        <f7-link class="inv-nav-icon" aria-label="搜索理财" @click="showSearch=!showSearch"><f7-icon f7="search" /></f7-link>
        <f7-link @click="showAdd=true">新增</f7-link>
      </f7-nav-right>
    </f7-navbar>
    <main class="inv-body">
      <p v-if="error" role="alert" class="cy-message">{{ error }} <button @click="refresh()">重试</button></p>
      <Transition name="cy-collapse"><div v-if="showSearch"><div class="cy-collapse-inner"><div class="inv-search"><input v-model="search" aria-label="搜索理财" placeholder="搜索名称或代码" /><button class="inv-link" @click="search='';showSearch=false">取消</button></div></div></div></Transition>
      <section class="inv-card inv-summary">
        <div><span>{{ amounts?.missing.netValue?'已知净资产贡献':'计入净资产' }}（元） <button class="inv-link" :aria-label="assetAmountsVisible?'隐藏资产金额':'显示资产金额'" @click="assetAmountsVisible=!assetAmountsVisible"><f7-icon :f7="assetAmountsVisible?'eye':'eye_slash'" /></button></span><strong>{{ money(total) }}</strong></div>
        <f7-link :href="`/investments/statistics?accountId=${accountId}`"><div><span>{{ amounts?.missing.totalProfit?'已知累计收益':'累计收益' }}（元）</span><strong :class="profitClass(profit)">{{ money(profit) }}</strong></div></f7-link>
      </section>
      <p v-if="amounts?.missing.netValue || amounts?.missing.totalProfit" class="inv-caption" role="status">汇总仅包含已知部分。<template v-if="amounts.missing.netValue">{{ amounts.missing.netValue }} 项估值或成本未知，未计入金额。</template><template v-if="amounts.missing.totalProfit">{{ amounts.missing.totalProfit }} 项收益未知，未计入累计收益。</template></p>
      <f7-link v-if="pending.length" class="inv-card inv-list-button" :href="`/investments/plans?accountId=${accountId}&tab=orders`"><span>{{ pending.length }} 笔待确认</span><f7-icon f7="chevron_right" /></f7-link>
      <p v-if="loading&&!wealth" class="cy-empty">正在加载理财…</p>
      <details v-for="group in grouped" :key="group.name" class="inv-card" open>
        <summary class="inv-group-head"><span>{{ group.name }}</span><span>{{ group.missing?'已知市值':'市值' }} {{ money(group.value) }}</span><f7-icon f7="chevron_down" /></summary>
        <p v-if="group.missing" class="inv-caption">{{ group.missing }} 项估值未知，未计入分组金额。</p>
        <template v-for="row in group.rows" :key="row.key">
          <details v-if="row.members.length>1" class="inv-merged"><summary class="inv-holding"><InvestmentHoldingSummary :row="row" :members="row.members.length" /></summary><f7-link v-for="member in row.members" :key="member.key" class="inv-holding inv-member" :href="holdingLink(member)"><InvestmentHoldingSummary :row="member" :members="1" /></f7-link></details>
          <f7-link v-else class="inv-holding" :href="holdingLink(row)"><InvestmentHoldingSummary :row="row" :members="1" /></f7-link>
        </template>
      </details>
      <details v-if="monetaryRows.length" class="inv-card" open><summary class="inv-group-head"><span>货币基金</span><span>{{ monetaryAmount.missing?'已知 ':'' }}{{ money(monetaryAmount.value) }}</span><f7-icon f7="chevron_down" /></summary><p v-if="monetaryAmount.missing" class="inv-muted">{{ monetaryAmount.missing }} 项估值未知，未计入分组汇总。</p><f7-link v-for="r in monetaryRows" :key="r.id" class="inv-holding" :href="`/account/detail?id=${r.id}`"><div class="inv-holding-top"><span>{{ r.binding.name }}</span><small class="inv-code">{{ r.binding.code }}</small><strong>{{ money(r.value) }}</strong></div><div class="inv-holding-info"><div><span class="inv-muted">已记录收益</span><span :class="profitClass(r.binding.totalIncome)">{{ money(r.binding.totalIncome) }}</span><span class="inv-muted">万份收益</span><span>{{ r.binding.lastPerTenThousand || '—' }}</span></div><div class="inv-muted">{{ r.name }} · {{ r.binding.enabled?'自动收益已启用':'自动收益已暂停' }}</div></div></f7-link></details>
      <section v-if="!loading&&!grouped.length&&!monetaryRows.length" class="inv-card cy-empty"><p>{{ search?'没有匹配的理财':'还没有理财记录' }}</p><button class="inv-link" @click="showAdd=true">新增一项理财</button></section>
      <nav class="inv-toolbar"><button class="inv-link" @click="showClosed=!showClosed">{{ showClosed?'收起已清仓':'查看已清仓' }}</button><f7-link href="/investments/plans">理财定投</f7-link><f7-link href="/investments/manage">管理</f7-link></nav>
    </main>
    <f7-sheet class="inv-sheet cy-mobile-surface cy-investment-page" v-model:opened="showAdd" swipe-to-close backdrop>
      <div class="inv-group-head"><span>新增理财</span><button class="inv-link" @click="showAdd=false">取消</button></div>
      <f7-link class="inv-list-button" :href="addLink('FUND')" sheet-close>基金<f7-icon f7="chevron_right" /></f7-link>
      <f7-link class="inv-list-button" href="/investments/add?type=MONETARY" sheet-close>基金（货币型）<f7-icon f7="chevron_right" /></f7-link>
      <f7-link v-for="type in investmentTypes.filter(t=>t.key!=='FUND')" :key="type.key" class="inv-list-button" :href="addLink(type.key)" sheet-close>{{ type.name }}<f7-icon f7="chevron_right" /></f7-link>
    </f7-sheet>
  </f7-page>
</template>
<script setup lang="ts">
import {computed,onUnmounted,ref} from 'vue';
import type {Router} from 'framework7/types';
import {LedgerDecimal} from '@/lib/ledger-display.ts';
import {assetMoney as money,assetAmountsVisible} from '@/lib/asset-visibility.ts';
import {investmentAmounts} from '@/lib/investment-scope.ts';
import {summarizeAmounts} from '@/lib/amount-summary.ts';
import InvestmentHoldingSummary from '@/components/mobile/InvestmentHoldingSummary.vue';
import {groupInvestmentHoldings,type HoldingGroupRow} from '@/lib/investment-groups.ts';
import {createValuationRefresh} from '@/lib/valuation-refresh.ts';
import {useInvestmentData,investmentTypes,profitClass,type HoldingRow} from '@/lib/investment-mobile.ts';
const props=defineProps<{f7route:Router.Route}>();
const {monetaryRows:allMonetary,wealth,accounts,orders,events,includedRows,visibleRows,loading,error,summaryReady,load}=useInvestmentData();
const accountId=computed(()=>props.f7route.query['accountId']||''),selectedAccount=computed(()=>accounts.value.find(a=>a.id===accountId.value));
const showAdd=ref(false),showSearch=ref(false),showClosed=ref(false),search=ref('');
const inScope=computed(()=>includedRows.value.filter(r=>!accountId.value||r.position.accountId===accountId.value));
const scopedMonetary=computed(()=>accountId.value?[]:allMonetary.value.filter(r=>!r.excluded));
const monetaryRows=computed(()=>(accountId.value?[]:allMonetary.value).filter(r=>!r.hidden&&(!search.value||`${r.name} ${r.binding.name} ${r.binding.code}`.toLowerCase().includes(search.value.toLowerCase()))));
const amounts=computed(()=>summaryReady.value?investmentAmounts(inScope.value,scopedMonetary.value):undefined);
const total=computed(()=>amounts.value?.netValue),profit=computed(()=>amounts.value?.totalProfit);
const grouped=computed(()=>{const groups=new Map<string,HoldingGroupRow[]>();for(const r of groupInvestmentHoldings(visibleRows.value.filter(r=>!accountId.value||r.position.accountId===accountId.value))){if(!showClosed.value&&new LedgerDecimal(r.position.quantity).isZero()&&events.value.some(e=>!e.voided&&e.accountId===r.position.accountId&&e.instrumentId===r.position.instrumentId))continue;if(search.value&&!`${r.name} ${r.asset?.symbol}`.toLowerCase().includes(search.value.toLowerCase()))continue;groups.set(r.group,[...(groups.get(r.group)||[]),r]);}return [...groups].map(([name,rs])=>({name,rows:rs,...summarizeAmounts(rs.flatMap(r=>r.members).map(r=>r.position.marketValue))}));});
const pending=computed(()=>orders.value.filter(o=>o.status==='pending'&&(!accountId.value||o.accountId===accountId.value)));
function holdingLink(row:HoldingRow):string{return '/investments/position?'+new URLSearchParams({accountId:row.position.accountId,instrumentId:row.position.instrumentId});}
function addLink(type:string):string{return '/investments/add?'+new URLSearchParams({type,accountId:accountId.value});}

async function refresh(done?:()=>void):Promise<void>{try{await load();}finally{if(typeof done==='function')done();}}
const live=createValuationRefresh(summary=>{wealth.value=summary;});
function activate():void{void load();live.start();}
onUnmounted(live.stop);
const monetaryAmount=computed(()=>summarizeAmounts(monetaryRows.value.map(r=>r.value)));
</script>

<style scoped>
.inv-merged>summary{list-style:none;cursor:pointer}.inv-merged>summary::-webkit-details-marker{display:none}.inv-member{padding-left:26px!important;background:var(--cy-soft);border-left:3px solid var(--cy-accent)}
</style>
