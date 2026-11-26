<template>
 <f7-page class="cy-main-page cy-mobile-surface" ptr @ptr:refresh="load" @page:afterin="activate" @page:beforeout="valuationRefresh.stop">
  <f7-navbar :title="account?.name || '加密资产'" back-link="资产"><f7-nav-right><f7-link :disabled="loading" @click="load()">{{ loading ? '刷新中…' : '刷新' }}</f7-link><AccountOptionsMenu v-if="account" :edit-href="`/crypto/add?id=${id}&kind=${account.kind}`" :target="{id,name:account.name,kind:'portfolio',href:`/crypto/account?id=${id}`}" @deleted="f7router.navigate('/investments',{reloadAll:true})" /></f7-nav-right></f7-navbar>
  <main class="cy-page-body crypto-account">
   <p v-if="error" class="cy-message" role="alert">{{ error }}</p>
   <template v-if="account">
    <section class="cy-panel"><div class="brand"><img v-if="platformIcon(account.platform, account.kind)" :src="platformIcon(account.platform, account.kind)" alt="" /><div><strong>{{ account.name }}</strong><p>{{ platformName(account.platform) || (account.kind==='WALLET'?'加密钱包':'加密货币交易所') }}</p></div></div><p class="hint">{{ totalAmount?.missing?'已知参考市值':'当前参考市值' }}（{{currency==='USD'?'美元':'人民币'}}）</p><strong class="total">{{ currencyMoney(total,currency) }}</strong><p v-if="currency==='USD'" class="hint">参考汇率 {{usdFX(summary)?.rate||'未知'}} CNY / USD · {{usdFX(summary)?.date||'暂无汇率'}}</p><p v-if="totalAmount?.missing" class="hint">{{ totalAmount.missing }} 项持仓暂无报价或汇率，参考市值仅汇总已知部分。</p><p v-if="rows.some(r=>!r.position?.costKnown && r.position)" class="hint">部分持仓成本未录入。</p></section>
    <template v-if="account.kind==='EXCHANGE'">
     <nav class="actions"><f7-link :href="convertLink('cash')">人民币买币</f7-link><f7-link :href="convertLink('coin')">币币兑换</f7-link><f7-link :href="convertLink('transfer')">转移</f7-link><f7-link :href="convertLink('redeem')">卖币到账</f7-link></nav>
     <nav class="wallet-actions"><f7-link :href="entryLink('EXPENSE')">记支出</f7-link><f7-link :href="entryLink('INCOME')">记收入</f7-link></nav>
    </template>
    <template v-else>
     <nav class="wallet-actions"><f7-link :href="entryLink('EXPENSE')">记支出</f7-link><f7-link :href="entryLink('INCOME')">记收入</f7-link></nav>
     <nav class="actions"><f7-link :href="convertLink('cash')">人民币买币</f7-link><f7-link :href="convertLink('coin')">币币兑换</f7-link><f7-link :href="convertLink('transfer')">转移</f7-link><f7-link :href="convertLink('redeem')">卖币到账</f7-link></nav>
    </template>
    <section v-if="account.kind==='EXCHANGE'" class="cy-panel"><div class="section-head"><h2>每日定投</h2><f7-link :href="`/crypto/dca?accountId=${id}`">管理定投 →</f7-link></div><p class="hint">设置每日投入与时间，自动记录买入、成本与收益；余额不足时暂停。</p></section>
    <section class="cy-panel"><div class="section-head"><h2>持有币种</h2><f7-link :href="convertLink('opening')">添加已有持仓</f7-link></div><p v-if="!rows.length" class="hint">还没有持仓。可录入已有数量，或从其他账户转入。</p><f7-link v-for="row in rows" :key="row.coin.id" :href="positionLink(row.coin.id)" class="holding"><div><strong>{{ row.coin.symbol }}</strong><small>{{ row.coin.name }}</small></div><div><strong :title="assetText(row.position?.quantity || '0')">{{ displayQuantity(row.position?.quantity || '0') }}</strong><small>{{ currencyMoney(positionValue(row.position,currency,summary),currency) }}</small></div></f7-link></section>
    <section class="cy-panel"><div class="section-head"><h2>交易记录</h2><span>{{ transactions.length }} 笔</span></div><p v-if="!transactions.length" class="hint">兑换与持仓记录会显示在这里。</p><button v-for="event in transactions" :key="event.id" type="button" class="event event-btn" @click="selectedEvent=event"><div><strong>{{ event.dca ? '每日定投' : eventNames[event.type] }}<small v-if="event.voided"> · 已撤销</small></strong><span>{{ date(event.occurredAt) }}</span></div><p>{{ eventDescription(event) }}</p><p v-if="event.conversion" class="hint">行情换算 · {{ date(event.conversion.observedAt) }}<template v-if="event.conversion.toQuantity !== (event.type==='SELL'?event.amount:event.quantity)"> · 已校正到账数量</template></p></button></section>
   </template>
  </main>
  <f7-sheet class="crypto-event-sheet cy-mobile-surface" :opened="!!selectedEvent" @sheet:closed="selectedEvent=undefined" swipe-to-close backdrop>
   <template v-if="selectedEvent">
    <div class="sheet-head"><strong>{{ selectedEvent.dca ? '每日定投' : eventNames[selectedEvent.type] }}{{ selectedEvent.voided ? ' · 已撤销' : '' }}</strong><f7-link sheet-close>关闭</f7-link></div>
    <div class="sheet-body">
     <p class="hint">{{ date(selectedEvent.occurredAt) }}</p>
     <p class="sheet-desc">{{ eventDescription(selectedEvent) }}</p>
     <p v-if="selectedEvent.fee && !new LedgerDecimal(selectedEvent.fee).isZero()" class="hint">手续费：{{ assetText(selectedEvent.fee) }}</p>
     <p v-if="selectedEvent.note" class="hint">备注：{{ selectedEvent.note }}</p>
     <div class="sheet-actions">
      <f7-link v-if="!selectedEvent.voided" class="cy-button cy-primary" :href="editEventLink(selectedEvent)" sheet-close>编辑记录</f7-link>
      <f7-link v-if="!selectedEvent.voided" class="cy-button cy-danger" :href="voidEventLink(selectedEvent)" sheet-close>撤销记录</f7-link>
      <f7-link class="cy-button" :href="positionLink(selectedEvent.instrumentId)" sheet-close>查看 {{ symbol(selectedEvent.instrumentId) }} 持仓</f7-link>
     </div>
    </div>
   </template>
  </f7-sheet>
 </f7-page>
</template>
<script setup lang="ts">
import { computed,ref,onMounted,onUnmounted } from 'vue';
import { walletCurrency, positionValue, currencyMoney as rawCurrencyMoney, usdFX } from '@/lib/wallet-entry.ts';
import type { Router } from 'framework7/types';
import moment from 'moment-timezone';
import { investments, investmentError } from '@/lib/investments.ts';
import { summarizeAmounts } from '@/lib/amount-summary.ts';
import { createValuationRefresh } from '@/lib/valuation-refresh.ts';
import { platformIcon,platformName } from '@/lib/crypto-platforms.ts';
import { LedgerDecimal } from '@/lib/ledger-display.ts';
import {assetAmountsVisible,assetText} from '@/lib/asset-visibility.ts';
function currencyMoney(value:string|null|undefined,currency:string):string{return assetAmountsVisible.value?rawCurrencyMoney(value,currency):'••••';}
import type { Instrument,InvestmentAccount,InvestmentEvent,WealthSummary } from '@/models/investment.ts';
import AccountOptionsMenu from '@/components/mobile/AccountOptionsMenu.vue';
const props=defineProps<{f7route:Router.Route;f7router:Router.Router}>();
const account=ref<InvestmentAccount>(),coins=ref<Instrument[]>([]),summary=ref<WealthSummary>(),events=ref<InvestmentEvent[]>([]),selectedEvent=ref<InvestmentEvent>(),loading=ref(false),error=ref(''),zone=ref('Asia/Shanghai');
const id=computed(()=>props.f7route.query['id']||'');
const valuationRefresh=createValuationRefresh(value=>{summary.value=value;});
function activate():void{void load();valuationRefresh.start();}
onMounted(()=>window.addEventListener('cy-dca-updated',()=>void load()));
onUnmounted(()=>{valuationRefresh.stop();});
const rows=computed(()=>coins.value.filter(c=>(account.value?.instruments||[]).includes(c.id)||summary.value?.positions.some(p=>p.accountId===id.value&&p.instrumentId===c.id)).map(coin=>({coin,position:summary.value?.positions.find(p=>p.accountId===id.value&&p.instrumentId===coin.id)})));
const currency=computed(()=>walletCurrency(account.value));
const totalAmount=computed(()=>summary.value?summarizeAmounts(summary.value.positions.filter(p=>p.accountId===id.value).map(p=>positionValue(p,currency.value,summary.value))):undefined);
const total=computed(()=>totalAmount.value?.value);
const transactions=computed(()=>events.value.filter(e=>[e.accountId,e.toAccountId,e.settlementAccountId].includes(id.value)).sort((a,b)=>b.occurredAt-a.occurredAt||b.id.localeCompare(a.id)));
const eventNames={OPENING:'已有持仓',BUY:'兑换买入',SELL:'卖币到账',TRANSFER:'账户间转移',INCOME:'收入',EXPENSE:'支出',ADJUST:'持仓校准'};
function displayQuantity(value:string):string{if(!assetAmountsVisible.value)return '••••';const original=new LedgerDecimal(value),rounded=original.toSignificantDigits(10);return `${rounded.eq(original)?'':'≈ '}${rounded.toFixed()}`;}
function symbol(id:string):string{return coins.value.find(c=>c.id===id)?.symbol||id;}
function eventDescription(e:InvestmentEvent):string{if(!assetAmountsVisible.value)return '••••';if(e.wallet)return `${e.amount} ${e.wallet.currency} · ${[{instrumentId:e.instrumentId,quantity:e.quantity},...(e.additionalMovements||[])].map(m=>`${m.quantity} ${symbol(m.instrumentId)}`).join(' + ')}`;if(e.type==='OPENING'||e.type==='ADJUST')return `${e.quantity} ${symbol(e.instrumentId)}`;if(e.type==='TRANSFER')return `${e.quantity} ${symbol(e.instrumentId)} · ${e.accountId===id.value?'转出':'转入'}`;const payment=`${e.amount} ${e.settlementInstrumentId?symbol(e.settlementInstrumentId):'CNY'}`;const acquired=`${e.quantity} ${symbol(e.instrumentId)}`;return e.type==='BUY'?`${payment} → ${acquired}`:`${acquired} → ${payment}`;}
function entryLink(type:string):string{return '/crypto/entry?'+new URLSearchParams({accountId:id.value,type}).toString();}
function convertLink(mode:string):string{return '/crypto/convert?'+new URLSearchParams({accountId:id.value,mode}).toString();}
function positionLink(instrumentId:string,accountId=id.value):string{return '/investments/position?'+new URLSearchParams({accountId,instrumentId}).toString();}
function editEventLink(e:InvestmentEvent):string{if(e.wallet)return `/crypto/entry?eventId=${encodeURIComponent(e.id)}&action=revise`;return `/investments/record?action=revise&eventId=${encodeURIComponent(e.id)}&accountId=${encodeURIComponent(e.accountId)}&instrumentId=${encodeURIComponent(e.instrumentId)}`;}
function voidEventLink(e:InvestmentEvent):string{if(e.wallet)return `/crypto/entry?eventId=${encodeURIComponent(e.id)}&action=void`;return `/investments/record?action=void&eventId=${encodeURIComponent(e.id)}&accountId=${encodeURIComponent(e.accountId)}&instrumentId=${encodeURIComponent(e.instrumentId)}`;}
function date(at:number):string{return moment.unix(at).tz(zone.value).format('MM-DD HH:mm');}
async function load(done?:()=>void):Promise<void>{loading.value=true;error.value='';try{const[a,c,s,e,t]=await Promise.all([investments.accounts(),investments.instruments(),investments.summary(),investments.events(),investments.settings()]);account.value=a.find(x=>x.id===id.value);coins.value=c.filter(x=>x.type==='CRYPTO');summary.value=s;events.value=e;zone.value=t.timeZone||zone.value;if(!account.value)throw Error('找不到这个账户');}catch(e){error.value=investmentError(e);}finally{loading.value=false;if(typeof done==='function')done();}}
</script>
<style scoped>
.crypto-account{max-width:640px;margin:auto;padding-bottom:35px}.brand{display:flex;align-items:center;gap:13px;margin-bottom:24px}.brand img{width:44px;height:44px;border-radius:12px}.brand strong{font-size:20px}.brand p,.hint{font-size:12px;color:var(--cy-muted);line-height:1.8}.brand p{margin:5px 0 0}.total{font-size:32px;display:block;margin:10px 0 16px}.wallet-actions{display:flex;gap:10px;margin-bottom:14px}.wallet-actions a{flex:1;padding:15px;text-align:center;border-radius:12px;background:var(--cy-soft);color:var(--cy-accent)}.actions{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:14px}.actions a{padding:15px 8px;text-align:center;border:1px solid var(--cy-line);border-radius:12px;background:var(--cy-card);color:var(--cy-accent)}.section-head{display:flex;justify-content:space-between;align-items:center;gap:10px;margin-bottom:12px}.section-head h2{font-size:17px;margin:0}.section-head a,.section-head>span{font-size:12px;color:var(--cy-accent)}.holding{display:flex;justify-content:space-between;gap:12px;color:inherit;width:100%;padding:18px 0;border-bottom:1px solid var(--cy-line)}.holding:last-child{border:0}.holding>div:last-child{text-align:right;max-width:60%;overflow-wrap:anywhere}.holding strong{font-size:16px}.holding small{display:block;font-size:12px;color:var(--cy-muted);margin-top:6px}.event-btn{display:block;width:100%;text-align:left;background:none;border:0;border-bottom:1px solid var(--cy-line);padding:15px 0;color:var(--cy-ink)}.event-btn:last-child{border:0}.event-btn>div{display:flex;justify-content:space-between;font-size:14px}.event-btn>div>span{font-size:11px;color:var(--cy-muted)}.event-btn>p{font-size:13px;overflow-wrap:anywhere;margin-top:4px}.crypto-event-sheet{height:auto;max-height:78dvh;border-radius:18px 18px 0 0}.sheet-head{display:flex;justify-content:space-between;align-items:center;padding:16px 18px;border-bottom:1px solid var(--cy-line)}.sheet-body{padding:14px 18px 26px}.sheet-desc{font-size:16px;font-weight:500;margin:8px 0 10px}.sheet-actions{display:grid;gap:10px;margin-top:16px}
</style>
