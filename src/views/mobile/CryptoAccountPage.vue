<template>
 <f7-page class="cy-main-page cy-mobile-surface" @page:beforein="load" @page:afterin="openInitialPicker">
  <f7-navbar :title="editId?'编辑账户':kind==='WALLET'?'添加加密钱包':'添加加密货币交易所'" back-link="返回" />
  <main class="cy-page-body crypto-page">
   <p v-if="error" ref="errorElement" class="crypto-error" role="alert">{{ error }}</p>
   <form v-if="selectionMade" @submit.prevent="save"><fieldset :disabled="busy">
    <section class="cy-panel"><h2>{{ kind==='WALLET'?'钱包品牌':'交易所' }}</h2><button type="button" class="platform-choice" @click="showPlatforms=true"><img v-if="platformIcon(platform,kind)" :src="platformIcon(platform,kind)" alt="" /><span>{{platformName(platform)||(kind==='WALLET'?'自定义钱包':'自定义交易所')}}</span><span class="change-platform">更换 ›</span></button><label>账户名称<input v-model="name" required maxlength="64" placeholder="可自定义名称" /></label><label>货币单位<select v-model="currency" aria-label="账户货币单位"><option value="CNY">人民币（CNY）</option><option value="USD">美元（USD）</option></select></label><p class="hint">账户市值与新增收支默认使用这个单位，总资产仍折算人民币；切换单位不会更改已有交易。</p></section>
    <section class="cy-panel"><h2>持有的加密货币</h2><p v-if="editId" class="hint">持有中的币种保留；数量和成本在交易记录中修改。</p><p v-else class="hint">选择币种并填写已有数量。总成本与每枚均价可以互相换算，不知道成本可留空。</p>
     <div v-for="coin in coins" :key="coin.id" class="coin-row">
      <label class="coin-choice"><input v-model="selected" type="checkbox" :value="coin.id" :disabled="held.includes(coin.id)" /><span><strong>{{ coin.symbol }}</strong><small>{{ coin.name }}</small></span></label>
      <div v-if="!editId && selected.includes(coin.id)" class="opening-fields">
       <label>持有数量<input :value="opening(coin.id).quantity" inputmode="decimal" maxlength="60" :aria-label="`${coin.symbol} 持有数量`" placeholder="没有可留空" @input="editOpening(coin.id,'quantity',$event)" /></label>
       <div class="cost-grid">
        <label>总成本（{{currency}}）<input :value="opening(coin.id).total" inputmode="decimal" maxlength="60" :aria-label="`${coin.symbol} 总成本（${currency}）`" placeholder="成本未知可留空" @input="editOpening(coin.id,'total',$event)" /></label>
        <label>每枚均价（{{currency}}）<input :value="opening(coin.id).average" inputmode="decimal" maxlength="60" :aria-label="`${coin.symbol} 每枚均价（${currency}）`" placeholder="填写后自动计算总成本" @input="editOpening(coin.id,'average',$event)" /></label>
       </div>
       <label v-if="currency==='USD' && opening(coin.id)[opening(coin.id).source].trim()">成本汇率（1 USD = 人民币）<input v-model="opening(coin.id).exchangeRate" inputmode="decimal" maxlength="60" :aria-label="`${coin.symbol} 成本汇率`" placeholder="填写取得这些持仓时的成本汇率" /></label>
      </div>
     </div><CryptoCoinSearch :instruments="coins" @selected="addCoin" />
    </section>
    <section class="cy-panel"><h2>收支默认币种</h2><label>优先扣款 / 默认收款<select v-model="primaryCoin" aria-label="优先收支币种"><option value="">每次记账时选择</option><option v-for="coin in selectedCoins" :key="coin.id" :value="coin.id">{{coin.symbol}} · {{coin.name}}</option></select></label><label>余额不足时使用<select v-model="backupCoin" :disabled="!primaryCoin" aria-label="备用支出币种"><option value="">不自动分配</option><option v-for="coin in selectedCoins.filter(c=>c.id!==primaryCoin)" :key="coin.id" :value="coin.id">{{coin.symbol}} · {{coin.name}}</option></select></label><p class="hint">美元稳定币默认按 1 枚约 1 美元预填数量，保存收支前可核对和修改实际数量。</p></section>
    <section v-if="!editId && kind==='EXCHANGE'" class="cy-panel">
     <div class="dca-heading"><h2>每日定投（可选）</h2><button type="button" class="text-button" :disabled="dcaPlans.length>=100" @click="addDCAPlan">＋ 添加计划</button></div>
     <p class="hint">可直接配置与交易所一致的定投，保存账户时一并启用。按公开参考价记账，不向交易所下单。</p>
     <p v-if="!dcaPlans.length" class="hint">暂不配置也可以，之后可在账户详情中添加。</p>
     <article v-for="(plan,index) in dcaPlans" :key="plan.key" class="setup-plan">
      <div class="dca-heading"><strong>计划 {{index+1}}</strong><button type="button" class="text-button" @click="dcaPlans.splice(index,1)">移除</button></div>
      <div class="cost-grid">
       <label>买入币种<select v-model="plan.instrumentId" @change="selectPlanCoins(plan)"><option value="crypto:bitcoin">比特币 BTC</option><option value="crypto:ethereum">以太坊 ETH</option><option value="crypto:solana">Solana SOL</option></select></label>
       <label>支付币种<select v-model="plan.paymentInstrumentId" @change="selectPlanCoins(plan)"><option value="crypto:tether">USDT</option><option value="crypto:usd-coin">USDC</option></select></label>
       <label>每日投入（{{dcaSymbols[plan.paymentInstrumentId]}}）<input v-model.trim="plan.amount" inputmode="decimal" maxlength="60" required placeholder="例如 10" /></label>
       <label>每天时间<input v-model="plan.dailyTime" type="time" step="60" required /></label>
       <label>开始日期<input v-model="plan.startDate" type="date" :min="today" required /></label>
      </div>
     </article>
     <p v-if="dcaPlans.length" class="hint">时区：{{ledgerScope.timeZone}}。从保存后的下一次计划时间开始，过去买入请计入上方初始持仓。请填写实际持有的支付币数量；不足一次投入时会自动暂停。</p>
    </section>
    <section v-if="!editId" class="cy-panel"><BookPicker v-model="bookId" /><p class="hint">已有持仓不扣其他账户余额；未录入成本时不计算盈亏。<template v-if="kind==='EXCHANGE'">定投记录使用此账本。</template></p><p v-if="currency==='USD'" class="hint">美元成本按填写的成本汇率折算人民币，不使用当前汇率替代。切换货币单位后请核对已填写的成本金额。</p></section>
    <button class="primary" :disabled="(kind!=='WALLET' && !platform) || !name.trim() || busy">{{ busy?'正在保存…':editId?'保存账户':dcaPlans.length?'保存账户并启用定投':'保存账户与持仓' }}</button>
   </fieldset></form>
  </main>
  <CryptoPlatformSelectionSheet v-model:opened="showPlatforms" :kind="kind" @select="selectPlatform" @closed="pickerClosed" />
 </f7-page>
</template>
<script setup lang="ts">
import { computed, ref, reactive, watch, nextTick } from 'vue';
import { useLedgerScopeStore } from '@/stores/ledgerScope.ts';
import { cryptoInput } from '@/lib/crypto-entry.ts';
import { dcaSymbols, type CryptoDCASetupInput } from '@/lib/crypto-dca.ts';
import { suggestedPaymentCoins } from '@/lib/wallet-entry.ts';
import type { Router } from 'framework7/types';
import { investments, investmentError } from '@/lib/investments.ts';
import { emptyCryptoOpening, updateCryptoOpening, cryptoOpeningInput, type CryptoOpeningDraft, type CostEntry } from '@/lib/crypto-opening.ts';
import { LedgerDecimal } from '@/lib/ledger-display.ts';
import { cryptoPlatforms, platformIcon, platformName, type CryptoPlatform } from '@/lib/crypto-platforms.ts';
import { generateRandomUUID } from '@/lib/misc.ts';
import { useBooksStore } from '@/stores/books.ts';
import { useTransactionsStore } from '@/stores/transaction.ts';
import type { Instrument } from '@/models/investment.ts';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import BookPicker from '@/components/mobile/BookPicker.vue';
import CryptoPlatformSelectionSheet from '@/components/mobile/CryptoPlatformSelectionSheet.vue';
import CryptoCoinSearch from '@/components/mobile/CryptoCoinSearch.vue';
const props=defineProps<{f7route:Router.Route;f7router:Router.Router}>();
const { showToast } = useI18nUIComponents();
const editId=computed(()=>props.f7route.query['id']||''),held=ref<string[]>([]);
const kind=computed(()=>props.f7route.query['kind']==='WALLET'?'WALLET':'EXCHANGE');
const showPlatforms=ref(false),selectionMade=ref(!!editId.value);
let pickerShown=false;
function openInitialPicker():void{if(!editId.value&&!pickerShown){pickerShown=true;showPlatforms.value=true;}}
function pickerClosed():void{if(!selectionMade.value)props.f7router.back();}
const platform=ref(''),name=ref(''),coins=ref<Instrument[]>([]),selected=ref<string[]>([]),openings=reactive<Record<string,CryptoOpeningDraft>>({}),error=ref(''),busy=ref(false),bookId=ref('');
function opening(id:string):CryptoOpeningDraft { return openings[id] ||= emptyCryptoOpening(); }
function editOpening(id:string,field:'quantity'|CostEntry,event:Event):void { updateCryptoOpening(opening(id),field,(event.target as HTMLInputElement).value); }
const currency=ref<'CNY'|'USD'>('CNY'),primaryCoin=ref(''),backupCoin=ref('');
const selectedCoins=computed(()=>coins.value.filter(c=>selected.value.includes(c.id)));
watch(primaryCoin,()=>{if(!primaryCoin.value||backupCoin.value===primaryCoin.value)backupCoin.value='';});
watch(selected,ids=>{
 if(primaryCoin.value && !ids.includes(primaryCoin.value))primaryCoin.value='';
 if(backupCoin.value && !ids.includes(backupCoin.value))backupCoin.value='';
},{deep:true});
const books=useBooksStore();let requestKey=generateRandomUUID(),lastPayload='';
const ledgerScope=useLedgerScopeStore();
const today=computed(()=>ledgerScope.currentDay);
const errorElement=ref<HTMLElement>();
const dcaPlans=ref<(CryptoDCASetupInput & {key:string})[]>([]);
function selectPlanCoins(plan:CryptoDCASetupInput):void { for(const id of [plan.instrumentId,plan.paymentInstrumentId]) if(!selected.value.includes(id))selected.value.push(id); }
function addDCAPlan():void {
 const plan={key:generateRandomUUID(),instrumentId:'crypto:bitcoin',paymentInstrumentId:'crypto:tether',amount:'',dailyTime:'09:00',startDate:today.value,timeZone:ledgerScope.timeZone};
 dcaPlans.value.push(plan);selectPlanCoins(plan);
}
function selectPlatform(p:CryptoPlatform):void{const previous=cryptoPlatforms.find(x=>x.id===platform.value)?.name;if(!name.value||name.value===previous)name.value=p.name;platform.value=p.id;selectionMade.value=true;}
function addCoin(coin:Instrument):void{if(!coins.value.some(c=>c.id===coin.id))coins.value.push(coin);if(!selected.value.includes(coin.id))selected.value.push(coin.id);}
async function load():Promise<void>{busy.value=true;error.value='';try{const [items]=await Promise.all([investments.instruments(),books.loadBooks()]);coins.value=items.filter(i=>i.type==='CRYPTO');bookId.value ||= books.defaultBookId;if(editId.value){const[a,s]=await Promise.all([investments.accounts(),investments.summary()]);const existing=a.find(a=>a.id===editId.value);if(!existing || existing.kind!==kind.value)throw Error('找不到这个账户');platform.value=existing.platform||'';name.value=existing.name;currency.value=existing.currency||'CNY';const defaults=suggestedPaymentCoins(existing,coins.value);primaryCoin.value=defaults[0]||'';backupCoin.value=defaults[1]||'';held.value=s.positions.filter(p=>p.accountId===editId.value && new LedgerDecimal(p.quantity).gt(0)).map(p=>p.instrumentId);selected.value=[...new Set([...(existing.instruments||[]),...held.value])];}}catch(e){error.value=investmentError(e);}finally{busy.value=false;}}
async function save():Promise<void>{if(busy.value)return;busy.value=true;error.value='';try{const paymentInstruments=[primaryCoin.value,backupCoin.value].filter(Boolean);if(paymentInstruments.some(id=>!selected.value.includes(id)))throw Error('收支默认币种必须保留在持有币种中');const initialPlans=!editId.value && kind.value==='EXCHANGE'?dcaPlans.value.map(({key,...plan})=>{if(!selected.value.includes(plan.instrumentId)||!selected.value.includes(plan.paymentInstrumentId))throw Error('请在持有币种中保留定投的买入和支付币种');return {...plan,amount:cryptoInput(plan.amount),timeZone:ledgerScope.timeZone};}):[];const input={name:name.value,kind:kind.value,platform:platform.value,bookId:bookId.value,currency:currency.value,paymentInstruments,holdings:editId.value?[]:selected.value.map(id=>cryptoOpeningInput(id,opening(id),currency.value)),dcaPlans:initialPlans};const payload=JSON.stringify(input);if(payload!==lastPayload){requestKey=generateRandomUUID();lastPayload=payload;}const account=editId.value?await investments.updateAccount({id:editId.value,name:name.value,kind:kind.value,platform:platform.value,instruments:selected.value,currency:currency.value,paymentInstruments}):await investments.createCryptoAccount(input,requestKey);useTransactionsStore().updateStoreInvalidState({accountList:true,transactionList:true,overview:true,statistics:true});if(initialPlans.length)window.CYLedgerReminders?.scheduleDca?.(false);showToast(editId.value?'账户已保存':initialPlans.length?'账户与定投计划已保存':'账户与持仓已保存');if(editId.value)props.f7router.back();else props.f7router.navigate(`/crypto/account?id=${encodeURIComponent(account.id)}`,{reloadCurrent:true});}catch(e){error.value=investmentError(e);await nextTick();errorElement.value?.scrollIntoView({block:'center',behavior:'smooth'});}finally{busy.value=false;}}
</script>
<style scoped>
.crypto-page{max-width:640px;margin:auto;padding-bottom:40px}fieldset{border:0;margin:0;padding:0;min-width:0}h2{font-size:17px;margin:0 0 17px}.platform-choice{display:flex;align-items:center;gap:12px;width:100%;min-height:48px;border:0;background:none;color:inherit;font:inherit;text-align:left}.platform-choice img{width:32px;height:32px;object-fit:contain}.change-platform{margin-left:auto;color:var(--cy-accent);font-size:13px}label:not(.coin-choice){display:grid;gap:10px;margin-top:20px;font-size:13px}input:not([type=checkbox]),select{box-sizing:border-box;width:100%;min-height:46px;padding:12px;border:1px solid var(--cy-line);border-radius:10px;background:var(--cy-card);color:inherit;font:inherit}.hint{font-size:12px;color:var(--cy-muted);line-height:1.8}.coin-row{display:flex;flex-direction:column;gap:12px;align-items:stretch;min-height:70px;border-bottom:1px solid var(--cy-line);padding:10px 0}.coin-choice{display:flex;align-items:center;gap:10px;flex:1;min-width:0}.coin-choice input{width:18px;height:18px;accent-color:var(--cy-accent)}.coin-choice strong{font-size:15px}.coin-choice small{display:block;color:var(--cy-muted);font-size:11px;overflow-wrap:anywhere;margin-top:5px}.opening-fields{padding:0 0 12px 28px}.opening-fields label{margin-top:10px;font-size:12px}.cost-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.opening-fields input{min-width:0;font-size:13px}@media(max-width:360px){.cost-grid{grid-template-columns:1fr}}.primary{width:100%;padding:15px;border:0;border-radius:12px;background:var(--cy-accent);color:white;font:inherit}.primary:disabled{opacity:.5}.dca-heading{display:flex;align-items:center;justify-content:space-between;gap:12px}.dca-heading h2{margin:0}.text-button{padding:8px 0;border:0;background:none;color:var(--cy-accent);font:inherit;font-size:13px}.setup-plan{margin-top:18px;padding-top:18px;border-top:1px solid var(--cy-line)}.setup-plan strong{font-size:14px}.crypto-error{padding:12px;color:var(--cy-expense);font-size:13px}
</style>
