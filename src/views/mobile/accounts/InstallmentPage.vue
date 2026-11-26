<template>
 <f7-page class="cy-mobile-surface cy-asset-surface cy-installments" @page:afterin="load">
  <f7-navbar :title="editing?(preview?'分期预览':editing.id?'编辑分期':'新增分期'):'分期管理'" :back-link="editing?undefined:'账户'">
   <f7-nav-left v-if="editing"><f7-link icon-f7="xmark" :disabled="busy" @click="preview?preview=false:editing=null"/></f7-nav-left>
   <f7-nav-right><f7-link v-if="editing" icon-f7="checkmark" :aria-label="preview?'确认分期':'预览分期'" :disabled="busy||sourceLoading" @click="preview?save():calculate()"/><f7-link v-else icon-f7="plus" aria-label="新增分期" @click="create()"/></f7-nav-right>
  </f7-navbar>
  <main class="cy-page-body">
   <p v-if="error" class="cy-message" role="alert">{{error}}</p>
   <template v-if="editing">
    <template v-if="!preview">
     <section class="installment-fields">
      <label>原始账单<select :disabled="!!editing.id" v-model="sourceKey" aria-label="选择分期账单" @change="selectSource"><option value="">请选择</option><optgroup label="每月账单"><option v-for="s in availableStatements" :key="s.month" :value="`month:${s.month}`">{{s.endDate.slice(0,7)}} · 待还 {{ledgerMoney(s.remaining,false)}}</option></optgroup><optgroup label="单笔消费 / 借入"><option v-for="t in expenses" :key="t.id" :value="`expense:${t.id}`">{{moment.unix(t.time).tz(scope.timeZone).format('MM-DD')}} · {{t.comment||'账单'}} {{t.refund?.kind==='expense'?'净支出 ':''}}{{t.refund?.kind==='expense'&&!t.refund.netExpense?'待核对':ledgerMoney(t.refund?.kind==='expense' ? t.refund.netExpense! : new LedgerDecimal(t.sourceAmount).div(100).toString(),false)}}</option></optgroup></select></label>
      <label>分期本金<input v-model="editing.principal" inputmode="decimal" maxlength="16" aria-label="分期本金" placeholder="输入分期金额"/></label>
      <label>总服务费<input v-model="editing.totalFee" inputmode="decimal" maxlength="16" aria-label="分期服务费" placeholder="0.00"/></label>
      <label>分期期数<input v-model.number="editing.periods" type="number" min="1" max="500" aria-label="分期期数"/></label>
      <div class="period-presets"><button v-for="n in [3,6,12,24]" :key="n" type="button" :aria-pressed="editing.periods===n" @click="editing.periods=n">{{n}}期</button></div>
      <label>第一期日期<input v-model="editing.firstDate" type="date" aria-label="第一期日期"/></label>
     </section>
     <section class="installment-fields">
      <label>分期方式<select v-model="editing.method" aria-label="分期方式"><option value="monthly">本金与服务费按月分摊</option><option value="first_fee">服务费首期收取</option><option value="immediate_fee">服务费立即收取</option><option value="balloon">本金最后一期偿还</option></select></label>
      <details class="installment-advanced"><summary>高级规则（尾差处理）</summary><label>尾差处理<select v-model="editing.remainder" aria-label="尾差处理"><option value="first">尾差计入首期</option><option value="last">尾差计入末期</option><option value="except_first">除首期外均分</option><option value="except_last">除末期外均分</option></select></label></details>
      <label>备注<input v-model="editing.note" maxlength="200" aria-label="分期备注" placeholder="选填"/></label>
     </section>
     <p class="installment-hint">本金沿用原账单；确认分期后，服务费将在约定日期入账。实际还款时选择资金账户完成转账。</p>
     <p v-if="sourceLoading" class="installment-hint">正在读取退款明细…</p>
     <p v-if="editing.allowRefundedExpense" class="installment-hint">退回本卡的金额从可分期本金中扣除；退款到其他账户不减少本卡欠款。进行中的分期会保护原退款，需修改或删除退款时请先结束分期。</p>
    </template>
    <template v-else>
     <section class="installment-summary"><span>本金<strong>{{assetCurrencyMoney(editing.principal,account?.currency||'CNY')}}</strong></span><span>总服务费<strong>{{assetCurrencyMoney(editing.totalFee,account?.currency||'CNY')}}</strong></span><span>期数<strong>{{editing.periods}}期</strong></span></section>
     <p v-if="previewDiffMessage" class="cy-message" role="alert">{{previewDiffMessage}}</p>
     <p v-else class="installment-hint">核对每期日期与金额。可调整未到期项目，合计须与本金和总服务费一致。</p>
     <section class="installment-payments"><header><span>日期</span><span>本金</span><span>服务费</span></header><div v-for="(p,i) in editing.payments" :key="i"><input v-model="p.date" type="date" :disabled="p.accrued" :aria-label="`第${i+1}期日期`"/><input v-model="p.principal" inputmode="decimal" :disabled="p.accrued" maxlength="16" :aria-label="`第${i+1}期本金`"/><input v-model="p.fee" inputmode="decimal" :disabled="p.accrued" maxlength="16" :aria-label="`第${i+1}期服务费`"/></div></section>
     <button class="installment-save" :disabled="busy || !!previewDiffMessage" @click="save">{{busy?'正在保存…':'确认分期计划'}}</button>
    </template>
   </template>
   <template v-else>
    <p v-if="requestedExpense" class="installment-hint">显示此账单的分期以及本账户的月账单分期；月账单分期可能包含其他消费。</p>
    <div class="installment-tabs"><button :class="{active:!closed}" @click="closed=false">进行中</button><button :class="{active:closed}" @click="closed=true">已结束</button></div>
    <p v-if="loading" class="cy-empty">正在读取分期…</p><p v-else-if="!shown.length" class="cy-empty">暂无分期记录</p>
    <section v-for="plan in shown" :key="plan.id" class="installment-record">
     <header><strong>{{plan.data.note||`${statementLabel(plan.statementMonth)}分期`}}</strong><span>{{plan.data.periods}}期</span></header>
     <div class="installment-summary"><span>本金<strong>{{ledgerMoney(plan.data.principal,false)}}</strong></span><span>总服务费<strong>{{ledgerMoney(plan.data.totalFee,false)}}</strong></span><span>已到期<strong>{{plan.data.payments.filter(p=>p.accrued).length}}期</strong></span></div>
     <button class="installment-expand" @click="expanded=expanded===plan.id?'':plan.id">{{expanded===plan.id?'收起':'查看每期计划'}}<f7-icon f7="chevron_down"/></button>
     <template v-if="expanded===plan.id"><div v-for="(p,i) in plan.data.payments" :key="i" class="installment-payment"><span>{{p.date}}<small>{{p.accrued?'已到期':'未到期'}}<f7-link v-if="p.feeTransactionId" :href="`/transaction/detail?id=${p.feeTransactionId}&type=3`"> · 服务费流水</f7-link></small></span><strong>{{ledgerMoney(new LedgerDecimal(p.principal).plus(p.fee).toString(),false)}}<small>本金 {{ledgerMoney(p.principal)}} + 服务费 {{ledgerMoney(p.fee)}}</small></strong></div></template>
     <footer v-if="!plan.closed"><button @click="edit(plan)">编辑</button><button @click="close(plan)">结束分期</button><f7-link :href="repay(plan)">还款</f7-link></footer>
    </section>
   </template>
  </main>
 </f7-page>
</template>
<script setup lang="ts">
import {assetMoney as ledgerMoney, assetCurrencyMoney} from '@/lib/asset-visibility.ts';
import {computed,ref} from 'vue';
import type {Router} from 'framework7/types';
import moment from 'moment-timezone';
import {useI18nUIComponents} from '@/lib/ui/mobile.ts';
import {creditAccounts,type CreditInstallment,type InstallmentInput,type CreditStatement} from '@/lib/credit-accounts.ts';
import {LedgerDecimal,keepUpToDate} from '@/lib/mobile-ledger.ts';
import {investmentError} from '@/lib/investments.ts';
import {loadRefunds} from '@/lib/refunds.ts';
import services from '@/lib/services.ts';
import type {TransactionInfoResponse} from '@/models/transaction.ts';
import {useAccountsStore} from '@/stores/account.ts';
import {useLedgerScopeStore} from '@/stores/ledgerScope.ts';
import {useBooksStore} from '@/stores/books.ts';
const props=defineProps<{f7route:Router.Route}>(),accounts=useAccountsStore(),scope=useLedgerScopeStore(),books=useBooksStore();
const {showToast,showConfirm}=useI18nUIComponents();
const id=computed(()=>props.f7route.query['id']||''),account=computed(()=>accounts.allAccountsMap[id.value]);
const plans=ref<CreditInstallment[]>([]),statements=ref<CreditStatement[]>([]),expenses=ref<TransactionInfoResponse[]>([]),editing=ref<InstallmentInput|null>(null),preview=ref(false),sourceKey=ref(''),closed=ref(false),expanded=ref(''),busy=ref(false),loading=ref(false),error=ref('');
const sourceLoading=ref(false);
let sourceGeneration=0;
const requestedExpense=computed(()=>String(props.f7route.query['expenseId']||''));
const shown=computed(()=>plans.value.filter(p=>p.accountId===id.value&&p.closed===closed.value&&(!requestedExpense.value||p.expenseId===requestedExpense.value||!!p.statementMonth)));
const availableStatements=computed(()=>statements.value.filter(s=>new LedgerDecimal(s.remaining).gt(0)));
function statementLabel(month:string):string{return month?(statements.value.find(s=>s.month===month)?.endDate.slice(0,7)||month):'单笔账单';}
let initialized=false;
async function load():Promise<void>{loading.value=true;error.value='';try{await creditAccounts.sync();await Promise.all([accounts.loadAllAccounts({force:true}).catch(keepUpToDate),books.loadBooks()]);const[p,r,t]=await Promise.all([creditAccounts.installments(),creditAccounts.reports(scope.timeZone,id.value),services.getAllTransactions({accountIds:id.value,startTime:0,endTime:0,bookIds:[]})]);plans.value=p;statements.value=r[0]?.statements||[];if(!t.data.success)throw Error('原始账单读取失败');expenses.value=t.data.result.filter(tx=>tx.sourceAccountId===id.value&&[3,4].includes(tx.type)&&tx.refund?.kind!=='cash'&&new LedgerDecimal(tx.sourceAmount).gt(0));if(!initialized&&requestedExpense.value){if(!expenses.value.some(t=>t.id===requestedExpense.value))throw Error('原账单不存在或不属于当前账户');const related=p.filter(plan=>!plan.closed&&plan.accountId===id.value&&(plan.expenseId===requestedExpense.value||!!plan.statementMonth));if(!related.length){create();}else if(related.length===1&&related[0]?.expenseId===requestedExpense.value)expanded.value=related[0].id;}else if(!initialized&&props.f7route.query['month']){create();sourceKey.value=`month:${props.f7route.query['month']}`;await selectSource();}initialized=true;}catch(e){error.value=investmentError(e);}finally{loading.value=false;}}
function create():void{sourceGeneration++;sourceLoading.value=false;error.value='';preview.value=false;sourceKey.value=requestedExpense.value&&expenses.value.some(t=>t.id===requestedExpense.value)?`expense:${requestedExpense.value}`:'';editing.value={id:'',version:'',accountId:id.value,expenseId:'',statementMonth:'',principal:'',totalFee:'0',periods:12,firstDate:moment().tz(scope.timeZone).add(1,'month').format('YYYY-MM-DD'),method:'monthly',remainder:'first',bookId:books.defaultBookId,timeZone:scope.timeZone,note:'',requestId:crypto.randomUUID(),payments:[]};if(sourceKey.value)void selectSource();}
async function selectSource():Promise<void>{const e=editing.value;if(!e)return;const generation=++sourceGeneration;sourceLoading.value=false;error.value='';const[k,v='']=sourceKey.value.split(':');e.expenseId=k==='expense'?v:'';e.statementMonth=k==='month'?v:'';e.payments=[];e.principal='';e.allowRefundedExpense=false;e.allowOtherAccountRefund=false;if(k==='month'){const s=statements.value.find(s=>s.month===v);e.principal=s?.remaining||'';if(s?.dueDate)e.firstDate=s.dueDate;e.allowRefundedExpense=true;e.allowOtherAccountRefund=true;return;}const t=expenses.value.find(t=>t.id===v);if(!t)return;e.bookId=t.bookId||books.defaultBookId;if(t.refund?.kind!=='expense'){e.principal=new LedgerDecimal(t.sourceAmount).div(100).toString();return;}sourceLoading.value=true;try{const summary=await loadRefunds(t.id);if(generation!==sourceGeneration||editing.value!==e)return;const returned=summary.refunds.filter(r=>r.accountId===id.value&&(r.installmentMode!=='offset'||!summary.hasActiveInstallment)).reduce((amount,r)=>amount.plus(r.amount),new LedgerDecimal(0));e.principal=LedgerDecimal.max(0,new LedgerDecimal(summary.originalAmount).minus(returned)).toString();e.allowRefundedExpense=true;e.allowOtherAccountRefund=true;}catch(err){if(generation===sourceGeneration&&editing.value===e)error.value=investmentError(err);}finally{if(generation===sourceGeneration)sourceLoading.value=false;}}
function edit(p:CreditInstallment):void{sourceGeneration++;sourceLoading.value=false;error.value='';const refunded=p.expenseId&&p.expenseId!=='0'?expenses.value.find(t=>t.id===p.expenseId)?.refund?.kind==='expense':true;editing.value={...JSON.parse(JSON.stringify(p.data)),id:p.id,version:p.version,accountId:p.accountId,expenseId:p.expenseId,statementMonth:p.statementMonth,requestId:crypto.randomUUID(),allowRefundedExpense:refunded,allowOtherAccountRefund:refunded};sourceKey.value=p.expenseId&&p.expenseId!=='0'?`expense:${p.expenseId}`:`month:${p.statementMonth}`;preview.value=true;}
async function calculate():Promise<void>{if(!editing.value||busy.value||sourceLoading.value)return;busy.value=true;error.value='';try{const d=await creditAccounts.preview({...editing.value,payments:[]});editing.value={...editing.value,...d};preview.value=true;}catch(e){error.value=investmentError(e);}finally{busy.value=false;}}
const previewDiffMessage=computed(()=>{
 const e=editing.value; if(!e||!preview.value||!e.payments.length)return '';
 const valid=/^(0|[1-9]\d{0,12})(\.\d{1,2})?$/;
 if(e.payments.some(p=>!valid.test(p.principal)||!valid.test(p.fee)))return '每期本金和服务费需为有效数字（最多两位小数）';
 const pSum=e.payments.reduce((s,p)=>s.plus(p.principal),new LedgerDecimal(0));
 const fSum=e.payments.reduce((s,p)=>s.plus(p.fee),new LedgerDecimal(0));
 const pTarget=new LedgerDecimal(e.principal||'0'),fTarget=new LedgerDecimal(e.totalFee||'0');
 const issues:string[]=[];
 if(!pSum.eq(pTarget))issues.push(`本金合计差 ${pSum.minus(pTarget).toFixed(2)}`);
 if(!fSum.eq(fTarget))issues.push(`服务费合计差 ${fSum.minus(fTarget).toFixed(2)}`);
 return issues.length?`各期之和与总额不一致（${issues.join('，')}），请核对后再确认。`:'';
});
async function save():Promise<void>{if(!editing.value||busy.value||previewDiffMessage.value)return;busy.value=true;error.value='';try{await creditAccounts.save(editing.value);editing.value=null;await load();showToast('分期计划已保存');}catch(e){error.value=investmentError(e);}finally{busy.value=false;}}
function close(p:CreditInstallment):void{showConfirm('结束后将取消后续计划和未入账服务费，已入账的本金与服务费完整保留。',async()=>{try{await creditAccounts.close(p.id,p.version);showToast('已结束分期');await load();}catch(e){error.value=investmentError(e);}},undefined,{title:'结束分期',confirmText:'结束分期',destructive:true,raw:true});}
function repay(p:CreditInstallment):string{const source=accounts.allVisiblePlainAccounts.find(a=>[1,2,4,8].includes(a.category)&&!a.assetProfile.kind&&a.currency===account.value?.currency),next=p.data.payments.find(x=>x.date>=moment().tz(scope.timeZone).format('YYYY-MM-DD'))||p.data.payments[p.data.payments.length-1],amount=new LedgerDecimal(next?.principal||0).plus(next?.fee||0).mul(100).toFixed(0);return `/transaction/add?type=4&accountId=${source?.id||''}&destinationAccountId=${id.value}&amount=${amount}&noTransactionDraft=true`;}
</script>
<style scoped>
.installment-fields,.installment-record,.installment-payments{background:var(--cy-card);border-radius:12px;padding:0 16px;margin-bottom:14px}.installment-fields label{display:flex;align-items:center;justify-content:space-between;gap:15px;min-height:53px;font-size:14px}.installment-fields input,.installment-fields select{min-width:0;flex:1;width:60%;text-align:right;background:none;border:0;color:var(--cy-ink);font:inherit}.period-presets{display:flex;gap:8px;padding:0 0 12px}.period-presets button{flex:1;padding:7px 0;border:1px solid var(--cy-line);border-radius:8px;background:var(--cy-bg);color:var(--cy-muted);font-size:13px}.period-presets button[aria-pressed=true]{background:var(--cy-soft);color:var(--cy-accent);border-color:var(--cy-accent);font-weight:500}.installment-advanced summary{padding:14px 0;font-size:13px;color:var(--cy-muted);cursor:pointer}.installment-hint{font-size:12px;color:var(--cy-muted);line-height:1.8;margin:18px 6px}.installment-summary{display:flex;justify-content:space-between;gap:10px;padding:20px 0}.installment-summary span{font-size:12px;color:var(--cy-muted)}.installment-summary strong{display:block;color:var(--cy-ink);font-size:20px;font-weight:500;margin-top:8px}.installment-tabs{display:flex;gap:24px;padding:0 3px 18px}.installment-tabs button,.installment-record button{background:none;border:0;color:var(--cy-muted);font:inherit;padding:6px 0}.installment-tabs .active{color:var(--cy-ink);border-bottom:2px solid var(--cy-accent)}.installment-record{padding:17px}.installment-record header{display:flex;justify-content:space-between;gap:10px;font-size:14px}.installment-record header strong{font-weight:500}.installment-record header>span{color:var(--cy-muted)}.installment-record footer{display:flex;gap:24px;justify-content:flex-end;align-items:center;padding-top:12px;font-size:13px}.installment-record footer button{color:var(--cy-accent)}.installment-expand{width:100%;display:flex;justify-content:space-between;font-size:12px!important}.installment-expand .icon{font-size:12px}.installment-payment{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:14px 0;font-size:13px}.installment-payment small{display:block;color:var(--cy-muted);font-size:10px;margin-top:6px}.installment-payment strong{text-align:right;font-weight:400}.installment-payments>header,.installment-payments>div{display:grid;grid-template-columns:1.5fr 1fr 1fr;gap:12px;padding:13px 0;font-size:12px}.installment-payments header{color:var(--cy-muted)}.installment-payments input{width:100%;min-width:0;border:0;background:none;color:var(--cy-ink);font:inherit}.installment-payments input:disabled{opacity:.5}.installment-save{width:100%;border:0;background:var(--cy-accent);color:var(--cy-card);padding:14px;border-radius:10px;font:inherit}.installment-save:disabled{opacity:.5}
</style>
