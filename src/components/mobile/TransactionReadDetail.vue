<template>
 <main class="bill-detail">
  <section class="bill-card bill-hero" role="button" tabindex="0" @click="emit('edit')" @keydown.enter="emit('edit')">
   <span class="bill-category-icon"><ItemIcon v-if="category?.icon" :icon-type="getCategoryIconType(category.iconType)" :icon-id="category.icon" /><f7-icon v-else :f7="transaction.type===4?'arrow_right_arrow_left':transaction.type===2?'money_dollar_circle':'cart'" /></span>
   <div class="bill-category" role="button" tabindex="0" @click.stop="emit('category')" @keydown.enter.stop="emit('category')"><strong>{{ typeName }}{{ category?.name && category.name!==typeName ? '-'+category.name : '' }}</strong></div>
   <div class="bill-amount" :class="transaction.type===2||transaction.refund?.kind==='cash'?'cy-income':transaction.type===3?'cy-expense':''"><strong>{{ amount }}</strong><small v-if="(transaction.wallet?.currency || currency)!=='CNY'">{{ transaction.wallet?.currency || currency }}</small></div>
  </section>
  <section class="bill-card">
   <button v-if="transaction.refund?.kind==='expense'" class="bill-row bill-refund" @click="emit('refunds')"><span>退款金额</span><b>{{ transaction.hideAmount?'••••':ledgerMoney(transaction.refund.refundedAmount,false) }} <f7-icon f7="chevron_right" /></b></button>
   <f7-link v-if="transaction.refund?.kind==='cash'" class="bill-row" :href="`/transaction/detail?id=${transaction.refund.expenseId}&type=3`"><span>关联支出</span><b>查看原账单 <f7-icon f7="chevron_right" /></b></f7-link>
   <div class="bill-row" role="button" tabindex="0" @click="emit('field','date')" @keydown.enter="emit('field','date')"><span>账单日期</span><b>{{ date(transaction.time) }}</b></div>
   <f7-link v-if="installmentEligible && installmentStatus" class="bill-row" :href="installmentLink"><span>分期信息</span><b>{{ installmentStatus }} <f7-icon f7="chevron_right" /></b></f7-link>
   <button v-else-if="installmentEligible && installmentError" class="bill-row bill-refund" @click="loadInstallments"><span>分期信息</span><b>读取失败 · 重试 <f7-icon f7="chevron_right" /></b></button>
   <div class="bill-row" role="button" tabindex="0" @click="emit('field','source')" @keydown.enter="emit('field','source')"><span>{{ transaction.type===4?'转出账户':'收支账户' }}</span><b role="button" tabindex="0" @click.stop="emit('account',transaction.wallet?.accountId || transaction.sourceAccountId)" @keydown.enter.stop="emit('account',transaction.wallet?.accountId || transaction.sourceAccountId)"><ItemIcon v-if="source?.icon" :icon-type="getAccountIconType(source.iconType)" :icon-id="source.icon" />{{ transaction.wallet?.accountName || source?.name || '账户已删除' }}</b></div>
   <template v-if="transaction.type===4"><div class="bill-row" role="button" tabindex="0" @click="emit('field','destination')" @keydown.enter="emit('field','destination')"><span>转入账户</span><b role="button" tabindex="0" @click.stop="emit('account',transaction.destinationAccountId)" @keydown.enter.stop="emit('account',transaction.destinationAccountId)"><ItemIcon v-if="destination?.icon" :icon-type="getAccountIconType(destination.iconType)" :icon-id="destination.icon" />{{ destination?.name || '账户已删除' }}</b></div><div class="bill-row" role="button" tabindex="0" @click="emit('field','destinationAmount')" @keydown.enter="emit('field','destinationAmount')"><span>转入金额</span><b>{{ transaction.hideAmount?'••••':money(transaction.destinationAmount,destination?.currency) }}</b></div><div v-if="nonzero(transaction.transferFeeAmount)" class="bill-row" role="button" tabindex="0" @click="emit('field','fee')" @keydown.enter="emit('field','fee')"><span>{{ isTransferDiscount ? '转账优惠' : '手续费' }}</span><b>{{ transaction.hideAmount ? '••••' : formatDecimalMoney(transaction.transferFeeAmount, currency, true) }}</b></div></template>
   <div v-if="nonzero(transaction.discountAmount)" class="bill-row" role="button" tabindex="0" @click="emit('field','discount')" @keydown.enter="emit('field','discount')"><span>优惠金额</span><b>{{ transaction.hideAmount ? '••••' : formatDecimalMoney(transaction.discountAmount, currency) }}</b></div>
   <template v-if="!transaction.wallet && transaction.originalCurrency && transaction.originalAmount && transaction.originalExchangeRate">
    <div class="bill-row"><span>原币金额</span><b>{{ transaction.hideAmount ? '••••' : formatDecimalMoney(transaction.originalAmount, transaction.originalCurrency) }}</b></div>
    <div class="bill-row"><span>记账汇率</span><b>1 USD = {{ transaction.originalExchangeRate }} CNY<template v-if="transaction.originalFXDate"> · {{ transaction.originalFXDate }}</template></b></div>
   </template>
  </section>
  <section class="bill-card">
   <div class="bill-row bill-note" role="button" tabindex="0" @click="emit('field','comment')" @keydown.enter="emit('field','comment')"><span>备注</span><b>{{ transaction.comment || '无备注' }}</b></div>
   <div class="bill-row" role="button" tabindex="0" @click="emit('field','tags')" @keydown.enter="emit('field','tags')"><span>标签</span><b class="bill-tags"><button v-for="tag in tags" :key="tag.id" type="button" @click.stop="emit('tag',tag.id)">{{ tag.name }}</button><template v-if="!tags.length">无标签</template></b></div>
   <div class="bill-row" role="button" tabindex="0" @click="emit('field','pictures')" @keydown.enter="emit('field','pictures')"><span>附件</span><b>{{ transaction.pictures?.length ? transaction.pictures.length+' 张' : '无附件' }}</b></div>
   <div v-if="transaction.pictures?.length" class="bill-pictures"><button v-for="(picture, idx) in transaction.pictures" :key="picture.pictureId" aria-label="查看附件" @click="emit('picture', picture, idx)"><img :src="pictureUrl(picture)" alt="账单附件" /></button></div>
   <div v-if="[2,3].includes(transaction.type)" class="bill-row"><span>计入收支</span><f7-toggle v-if="canEditExclusion" :disabled="savingFlag" :checked="!excluded" @toggle:change="emit('toggle-flag', 'statistics', $event)" /><b v-else>{{ excluded ? '不计入' : '计入' }}</b></div>
   <div v-if="transaction.type === 3" class="bill-row"><span>计入预算</span><f7-toggle v-if="canEditExclusion" :disabled="savingFlag" :checked="!budgetExcluded" @toggle:change="emit('toggle-flag', 'budget', $event)" /><b v-else>{{ budgetExcluded ? '不计入' : '计入' }}</b></div>
   <div v-if="transaction.reimbursementAccountId && transaction.reimbursementAccountId!=='0'" class="bill-row" role="button" tabindex="0" @click="emit('field','reimbursement')" @keydown.enter="emit('field','reimbursement')"><span>报销账户</span><b>{{ reimbursementName || '账户已删除' }}</b></div>
  </section>
  <details v-if="transaction.monetaryIncome" class="bill-card bill-settlement"><summary class="bill-row"><span>自动收益结算依据</span><f7-icon f7="chevron_down" /></summary>
   <div class="bill-row"><span>收益所属日期</span><b>{{ transaction.monetaryIncome.date }}</b></div>
   <div class="bill-row"><span>货币基金</span><b>{{ transaction.monetaryIncome.code }}</b></div>
   <div class="bill-row"><span>万份收益（元）</span><b>{{ transaction.monetaryIncome.perTenThousand }}</b></div>
   <div class="bill-row"><span>原始计息本金（元）</span><b>{{ transaction.hideAmount?'••••':transaction.monetaryIncome.principal }}</b></div>
  </details>
  <section class="bill-card">
   <div class="bill-row" role="button" tabindex="0" @click="emit('field','book')" @keydown.enter="emit('field','book')"><span>所属账本</span><b>{{ bookName || '默认账本' }}</b></div>
   <div class="bill-row bill-note" role="button" tabindex="0" @click="emit('field','location')" @keydown.enter="emit('field','location')"><span>地点信息</span><b>{{ [transaction.locationName,transaction.locationAddress].filter(Boolean).join(' · ') || (transaction.geoLocation ? `${transaction.geoLocation.latitude.toFixed(5)}, ${transaction.geoLocation.longitude.toFixed(5)}` : '无地点') }}</b></div>
   <div class="bill-row"><span>记录时间</span><b>{{ transaction.createdAt ? date(transaction.createdAt) : '历史记录未保存时间' }}</b></div>
   <div v-if="sourceLabel" class="bill-row"><span>记录方式</span><b>{{ sourceLabel }}</b></div>
   <f7-link v-if="transaction.transferFeeParentId && transaction.transferFeeParentId!=='0'" class="bill-row" :href="`/transaction/detail?id=${transaction.transferFeeParentId}&type=4`"><span>关联账单</span><b>原转账 <f7-icon f7="chevron_right" /></b></f7-link>
   <div class="bill-row" role="button" tabindex="0" @click="emit('field','links')" @keydown.enter="emit('field','links')"><span>关联账单</span><b>查看与添加 <f7-icon f7="chevron_right" /></b></div>
   <div v-if="transaction.debtDueDate" class="bill-row" role="button" tabindex="0" @click="emit('field','debt')" @keydown.enter="emit('field','debt')"><span>约定还款日期</span><b>{{ transaction.debtDueDate }}</b></div>
  </section>
 </main>
</template>
<script setup lang="ts">
import {computed,onUnmounted,ref,watch} from 'vue';
import moment from 'moment-timezone';
import type {Transaction} from '@/models/transaction.ts';
import type {Account} from '@/models/account.ts';
import type {TransactionCategory} from '@/models/transaction_category.ts';
import type {TransactionPictureInfoBasicResponse} from '@/models/transaction_picture_info.ts';
import {LedgerDecimal,ledgerMoney} from '@/lib/ledger-display.ts';
import {getAccountIconType,getCategoryIconType} from '@/lib/icon.ts';
import {creditAccounts} from '@/lib/credit-accounts.ts';
import ItemIcon from './ItemIcon.vue';

const props=defineProps<{transaction:Transaction;source?:Account;destination?:Account;category?:TransactionCategory;tags:{id:string;name:string}[];bookName?:string;reimbursementName?:string;timeZone:string;pictureUrl:(p:TransactionPictureInfoBasicResponse)=>string|undefined;refreshKey?:number;savingFlag?:boolean}>();
const emit=defineEmits<{picture:[p:TransactionPictureInfoBasicResponse,idx:number];edit:[];refunds:[];category:[];account:[id:string];tag:[id:string];field:[name:string];'toggle-flag':[kind:'statistics'|'budget',included:boolean]}>();
const isTransferDiscount = computed(() => !!props.transaction.transferFeeAmount && new LedgerDecimal(props.transaction.transferFeeAmount).lt(0));
const sourceLabel = computed(() => props.transaction.monetaryIncome ? '货币基金自动收益' : props.transaction.wallet ? '钱包收支' : props.transaction.investmentEventId ? '投资关联账单' : props.transaction.scheduledCreated ? '周期记账' : '');
const typeName=computed(()=>props.transaction.refund?.kind==='cash'?'退款':({1:'余额调整',2:'收入',3:'支出',4:'转账'} as Record<number,string>)[props.transaction.type]);
const currency=computed(()=>props.source?.currency||'CNY');
const amount=computed(()=>{const t=props.transaction;if(t.hideAmount)return '••••';const refundCash=t.refund?.kind==='cash';const value=t.refund?.kind==='expense'?t.refund.netExpense:refundCash?new LedgerDecimal(t.sourceAmount).abs().div(100).toString():t.wallet?.amount||new LedgerDecimal(t.sourceAmount).div(100).toString();return (t.type===2||refundCash?'+':t.type===3?'-':'')+ledgerMoney(value,false);});
const excluded=computed(()=>props.transaction.excludeFromStatistics||!!props.transaction.reimbursementReceiptId||!!props.transaction.reimbursementAccountId&&props.transaction.reimbursementAccountId!=='0');
const budgetExcluded=computed(()=>props.transaction.excludeFromBudget||!!props.transaction.reimbursementReceiptId||!!props.transaction.reimbursementAccountId&&props.transaction.reimbursementAccountId!=='0');
const canEditExclusion=computed(()=>props.transaction.editable && !props.transaction.investmentEventId && !props.transaction.reimbursementReceiptId && (!props.transaction.reimbursementAccountId || props.transaction.reimbursementAccountId==='0') && props.transaction.transferFeeParentId==='0');
const installmentEligible=computed(()=>props.transaction.type===3 && props.source?.category===3 && props.transaction.sourceAmount>0 && !props.transaction.investmentEventId && props.transaction.refund?.kind!=='cash');
const installmentLink=computed(()=>`/account/installments?id=${encodeURIComponent(props.transaction.sourceAccountId)}&expenseId=${encodeURIComponent(props.transaction.id)}`);
const installmentStatus=ref(''),installmentError=ref(false);
let installmentGeneration=0;
async function loadInstallments():Promise<void>{
 const generation=++installmentGeneration;installmentStatus.value='';installmentError.value=false;
 if(!installmentEligible.value || !props.transaction.id)return;
 const accountId=props.transaction.sourceAccountId,expenseId=props.transaction.id;
 try{const plans=await creditAccounts.installments();if(generation!==installmentGeneration)return;const active=plans.filter(p=>!p.closed&&p.accountId===accountId);const direct=active.filter(p=>p.expenseId===expenseId);installmentStatus.value=direct.length?`已分期（${direct.length}项）`:active.some(p=>!!p.statementMonth)?'查看或新增':'未分期';}
 catch{if(generation===installmentGeneration)installmentError.value=true;}
}
watch(()=>[props.transaction.id,props.transaction.sourceAccountId,props.transaction.type,props.transaction.sourceAmount,props.transaction.refund?.kind,props.transaction.investmentEventId,props.source?.category,props.refreshKey],()=>{void loadInstallments();},{immediate:true});
onUnmounted(()=>{installmentGeneration++;});
function date(at:number):string{return moment.unix(at).tz(props.timeZone).format('YYYY年MM月DD日 HH:mm');}
function money(amount:number,currency='CNY'):string{const formatted=ledgerMoney(new LedgerDecimal(amount).div(100).toString(),false);return currency==='CNY'?`¥${formatted}`:`${formatted} ${currency}`;}
function formatDecimalMoney(raw?:string,curr='CNY',abs=false):string{if(!raw)return '';const dec=new LedgerDecimal(raw);const formatted=ledgerMoney((abs?dec.abs():dec).toString(),false);return curr==='CNY'?`¥${formatted}`:curr==='USD'?`$${formatted}`:`${formatted} ${curr}`;}
function nonzero(value?:string):boolean{return !!value&&!new LedgerDecimal(value).isZero();}
</script>
<style scoped>
.bill-detail{padding:14px 14px 100px;max-width:650px;margin:auto;font-size:14px}.bill-card{background:var(--cy-card);border-radius:11px;margin-bottom:12px;padding:4px 16px;overflow:hidden}.bill-hero{display:flex;align-items:center;gap:12px;padding:18px 16px}.bill-category-icon{width:36px;height:36px;display:grid;place-items:center;border-radius:8px;background:var(--cy-soft);color:var(--cy-accent);flex-shrink:0}.bill-category-icon :deep(.icon),.bill-category-icon :deep(svg){width:25px;height:25px;font-size:25px}.bill-category{min-width:0;display:grid;gap:5px}.bill-category>span{font-size:12px;color:var(--cy-muted)}.bill-category>strong{font-size:15px;font-weight:500;overflow-wrap:anywhere}.bill-amount{margin-left:auto;text-align:right;flex-shrink:0;max-width:62%;overflow-wrap:anywhere}.bill-amount strong{font-size:23px;line-height:1.2;font-weight:600;font-family:Arial,"Microsoft YaHei",sans-serif;letter-spacing:-.7px}.bill-amount small{display:block;font-size:10px;color:var(--cy-muted);margin-top:6px}.bill-row{min-height:48px;display:flex;align-items:center;justify-content:space-between;gap:18px;line-height:1.5;color:inherit;width:100%;box-sizing:border-box}.bill-row>span:first-child{flex-shrink:0}.bill-row>b{font-weight:400;text-align:right;min-width:0;overflow-wrap:anywhere;color:var(--cy-muted);display:flex;justify-content:flex-end;align-items:center;gap:6px}.bill-row>b :deep(.icon),.bill-row>b :deep(svg){width:18px;height:18px;font-size:18px;flex-shrink:0}.bill-note{align-items:flex-start;padding:15px 0}.bill-note>b{white-space:pre-wrap}.bill-tags{flex-wrap:wrap}.bill-tags button{border:0;color:inherit;font-style:normal;font-size:12px;background:var(--cy-soft);padding:3px 7px;border-radius:4px}.bill-switch{height:24px;width:41px;flex-shrink:0;background:var(--cy-line);border-radius:20px;padding:3px;box-sizing:border-box}.bill-switch i{display:block;width:18px;height:18px;border-radius:50%;background:var(--cy-muted)}.bill-switch.on{background:var(--cy-accent)}.bill-switch.on i{background:var(--cy-card);transform:translateX(17px)}.bill-pictures{display:flex;gap:8px;padding:0 0 14px;overflow-x:auto}.bill-pictures button{border:0;background:none;padding:0;flex-shrink:0}.bill-pictures img{width:70px;height:70px;object-fit:cover;border-radius:6px}@media(max-width:360px){.bill-detail{padding:10px 10px 100px}.bill-card{padding-inline:12px}.bill-amount strong{font-size:24px}.bill-row{gap:10px;font-size:13px}}
.bill-settlement summary{list-style:none;cursor:pointer}.bill-settlement summary::-webkit-details-marker{display:none}.bill-settlement summary>.icon{font-size:14px;color:var(--cy-muted)}.bill-settlement[open] summary>.icon{transform:rotate(180deg)}
.bill-setting{border:0;background:none;padding:10px 0;color:var(--cy-accent);font:inherit;cursor:pointer}
.bill-refund{border:0;padding:0;background:none;font:inherit;text-align:left;cursor:pointer}.bill-refund .icon{font-size:14px}
[role=button]{cursor:pointer}.bill-row[role=button]:active{background:var(--cy-soft)}
</style>
