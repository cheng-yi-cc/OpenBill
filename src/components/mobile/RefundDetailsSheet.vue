<template>
 <StatisticsSheet :open="open" :title="form==='excess'?'退款差额':form==='new'?'添加退款':form==='edit'?'修改退款':'退款明细'" @update:open="close">
  <p v-if="loading" class="refund-message">正在加载退款明细…</p>
  <p v-else-if="error && !summary" class="refund-message" role="alert">{{error}} <button @click="load">重试</button></p>
  <template v-else-if="summary">
   <template v-if="!form">
    <div class="refund-total"><span>原支出 {{money(summary.originalAmount)}}</span><strong>已退 {{money(summary.refundedAmount)}}</strong><span>净支出 {{money(summary.netExpense)}}</span></div>
    <p v-if="!summary.refunds.length" class="refund-message">暂无退款记录</p>
    <section v-for="item in summary.refunds" :key="item.id" class="refund-record">
     <div><strong>{{accounts.allAccountsMap[item.accountId]?.name || '账户已删除'}}</strong><b>+{{money(item.amount)}}</b></div>
     <small>{{moment.unix(item.time).tz(timeZone).format('YYYY-MM-DD HH:mm:ss')}}{{item.comment?' · '+item.comment:''}}{{item.installmentMode==='reduce'?' · 缩减分期本金':item.installmentMode==='offset'?' · 冲抵应还':''}}</small>
     <p v-if="positive(item.excessAmount)">其中 {{money(item.excessAmount)}} 为退款差额收入</p>
     <div v-if="editable" class="refund-actions"><button @click="edit(item)">编辑退款</button><button v-if="positive(item.excessAmount)" @click="editExcess(item)">差额设置</button><button class="danger" @click="remove(item)">删除退款</button></div>
    </section>
    <p v-if="error" class="refund-error" role="alert">{{error}}</p>
    <button v-if="editable" class="refund-primary" :disabled="busy" @click="startNew">添加退款</button>
   </template>
   <template v-else-if="form==='new'||form==='edit'">
    <div class="refund-form">
     <label>金额（{{currency}}）<span class="refund-amount-input"><input v-model="draft.amount" inputmode="decimal" maxlength="16" placeholder="0.00" aria-label="退款金额" /><button v-if="form==='new'&&positive(summary.netExpense)" type="button" class="refund-full-btn" @click="draft.amount=summary.netExpense">全额退</button></span></label>
     <label>退款账户<select v-model="draft.accountId" aria-label="退款账户"><option value="">请选择账户</option><option v-for="account in cashAccounts" :key="account.id" :value="account.id">{{account.name}}</option></select></label>
     <label v-if="showInstallmentMode">分期处理方式
      <select v-model="draft.installmentMode" aria-label="分期处理方式">
       <option value="offset">冲减当期应还（保留原分期）</option>
       <option v-if="summary.supportsInstallmentReduce || draft.installmentMode==='reduce'" value="reduce">缩减分期本金</option>
      </select>
     </label>
     <label>退款日期<input v-model="draft.time" type="datetime-local" step="1" aria-label="退款日期" /></label>
     <label>备注<input v-model="draft.comment" maxlength="200" placeholder="可选" aria-label="退款备注" /></label>
    </div>
    <p v-if="showInstallmentMode" class="refund-hint">
     {{summary.supportsInstallmentReduce ? `当前分期本金 ${money(summary.installmentPrincipal||'0')}（未入账 ${money(summary.installmentUnaccruedPrincipal||'0')}）：冲减应还保留原计划；缩减本金将重算未到期各期。` : '当前为账单分期或暂无未入账本金，仅支持冲减应还。'}}
    </p>
    <p v-else-if="summary.hasActiveInstallment && (!selected || !!selected.installmentMode) && draft.accountId !== sourceAccountId" class="refund-hint">退回其他账户默认保留原信用卡分期计划不变。</p>
    <p class="refund-hint">退款会在所选日期转入退款账户，并自动冲减原支出的统计与预算金额。</p>
    <p v-if="error" class="refund-error" role="alert">{{error}}</p>
    <div class="refund-form-actions"><button :disabled="busy" @click="form=''">返回</button><button class="refund-primary" :disabled="busy" @click="save">{{busy?'保存中…':'保存退款'}}</button></div>
   </template>
   <template v-else>
    <div class="refund-form">
     <label>所属账本<select v-model="excessDraft.bookId"><option v-for="book in books.allBooks.filter(b=>!b.archived||b.id===excessDraft.bookId)" :key="book.id" :value="book.id">{{book.name}}</option></select></label>
     <label>收入分类<select v-model="excessDraft.categoryId"><option value="">请选择分类</option><option v-for="category in incomeCategories" :key="category.id" :value="category.id">{{category.name}}</option></select></label>
     <label>备注<input v-model="excessDraft.comment" maxlength="200" /></label>
     <label>计入收支统计<input :checked="!excessDraft.excludeFromStatistics" type="checkbox" @change="excessDraft.excludeFromStatistics=!($event.target as HTMLInputElement).checked" /></label>
    </div>
    <h3 class="refund-label">标签</h3><div class="refund-tags"><label v-for="tag in visibleTags" :key="tag.id"><input v-model="excessDraft.tagIds" type="checkbox" :value="tag.id" />{{tag.name}}</label><span v-if="!visibleTags.length">暂无标签</span></div>
    <p class="refund-hint">超出原支出的部分会单独记为一笔收入，不会重复增加账户余额。</p>
    <p v-if="error" class="refund-error" role="alert">{{error}}</p>
    <div class="refund-form-actions"><button :disabled="busy" @click="form=''">返回</button><button class="refund-primary" :disabled="busy" @click="saveExcess">{{busy?'保存中…':'保存差额设置'}}</button></div>
   </template>
  </template>
 </StatisticsSheet>
</template>
<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import moment from 'moment-timezone';
import StatisticsSheet from './StatisticsSheet.vue';
import {loadRefunds,refundActions,type RefundRecord,type RefundSummary} from '@/lib/refunds.ts';
import {LedgerDecimal,ledgerMoney} from '@/lib/ledger-display.ts';
import {investmentError} from '@/lib/investments.ts';
import {categoryAvailableInBook} from '@/lib/category-book-visibility.ts';
import {generateRandomUUID} from '@/lib/misc.ts';
import {useI18nUIComponents} from '@/lib/ui/mobile.ts';
import {useAccountsStore} from '@/stores/account.ts';
import {useBooksStore} from '@/stores/books.ts';
import {useTransactionCategoriesStore} from '@/stores/transactionCategory.ts';
import {useTransactionTagsStore} from '@/stores/transactionTag.ts';
const props=defineProps<{open:boolean;expenseId:string;sourceAccountId:string;currency:string;timeZone:string;hideAmount:boolean;editable:boolean}>();
const emit=defineEmits<{ 'update:open':[value:boolean]; changed:[summary:RefundSummary] }>();
const accounts=useAccountsStore(),books=useBooksStore(),categories=useTransactionCategoriesStore(),tags=useTransactionTagsStore(),{showConfirm,showToast}=useI18nUIComponents();
const summary=ref<RefundSummary>(),loading=ref(false),busy=ref(false),error=ref(''),form=ref<''|'new'|'edit'|'excess'>('');
const selected=ref<RefundRecord>(),draft=ref({amount:'',accountId:'',time:'',comment:'',installmentMode:'offset'});
const excessDraft=ref({categoryId:'',bookId:'',comment:'',tagIds:[] as string[],excludeFromStatistics:false,excludeFromBudget:false});
const cashAccounts=computed(()=>accounts.allVisiblePlainAccounts.filter(a=>a.assetProfile.kind!=='reimbursement'&&a.currency===props.currency));
const showInstallmentMode=computed(()=>Boolean(summary.value?.hasActiveInstallment && (!selected.value || !!selected.value.installmentMode) && draft.value.accountId===props.sourceAccountId));
const incomeCategories=computed(()=>Object.values(categories.allTransactionCategoriesMap).filter(c=>{const id=excessDraft.value.bookId,book=books.allBooks.find(b=>b.id===id),parent=categories.allTransactionCategoriesMap[c.parentId];return c.type===1&&c.parentId!=='0'&&((c.id===selected.value?.excess?.categoryId&&id===selected.value?.excess?.bookId)||!!parent&&categoryAvailableInBook(c,id,book)&&categoryAvailableInBook(parent,id,book));}));
const visibleTags=computed(()=>Object.values(tags.allTransactionTagsMap).filter(t=>!t.hidden || excessDraft.value.tagIds.includes(t.id)));
const money=(amount:string)=>props.hideAmount?'••••':ledgerMoney(amount,false);
const positive=(amount:string)=>new LedgerDecimal(amount).gt(0);
let generation=0,requestId='';
function close(value:boolean){emit('update:open',value);}
async function load(){const request=++generation;if(!props.open||!props.expenseId)return;loading.value=true;error.value='';summary.value=undefined;form.value='';try{const result=await loadRefunds(props.expenseId);if(request===generation)summary.value=result;}catch(cause){if(request===generation)error.value=investmentError(cause);}finally{if(request===generation)loading.value=false;}}
watch(()=>[props.open,props.expenseId] as const,([open])=>{if(open)void load();else generation++;},{immediate:true});
function startNew(){if(!summary.value)return;selected.value=undefined;requestId=generateRandomUUID();draft.value={amount:positive(summary.value.netExpense)?summary.value.netExpense:'',accountId:cashAccounts.value.some(a=>a.id===props.sourceAccountId)?props.sourceAccountId:cashAccounts.value[0]?.id||'',time:moment().tz(props.timeZone).format('YYYY-MM-DDTHH:mm:ss'),comment:'',installmentMode:'offset'};error.value='';form.value='new';}
function edit(item:RefundRecord){selected.value=item;draft.value={amount:item.amount,accountId:item.accountId,time:moment.unix(item.time).tz(props.timeZone).format('YYYY-MM-DDTHH:mm:ss'),comment:item.comment,installmentMode:item.installmentMode==='reduce'?'reduce':'offset'};error.value='';form.value='edit';}
function editExcess(item:RefundRecord){if(!item.excess)return;selected.value=item;excessDraft.value={...item.excess,tagIds:[...item.excess.tagIds]};error.value='';form.value='excess';}
function changed(result:RefundSummary){summary.value=result;form.value='';error.value='';emit('changed',result);}
function save(){
 if(busy.value||!summary.value)return;
 const amount=draft.value.amount;
 if(!/^(0|[1-9]\d{0,12})(\.\d{1,2})?$/.test(amount)||!positive(amount)){error.value='请输入大于零、最多两位小数的退款金额';return;}
 const at=moment.tz(draft.value.time,['YYYY-MM-DDTHH:mm:ss','YYYY-MM-DDTHH:mm'],true,props.timeZone);
 if(!at.isValid()||!draft.value.accountId){error.value='请选择有效退款账户和日期';return;}
 const current=summary.value,previous=selected.value;
 const installmentMode=current.hasActiveInstallment && (!previous || !!previous.installmentMode)
  ? (draft.value.accountId===props.sourceAccountId ? (draft.value.installmentMode || 'offset') : 'offset')
  : '';
 const total=new LedgerDecimal(current.refundedAmount).minus(previous?.amount||'0').plus(amount);
 const excess=LedgerDecimal.max(0,total.minus(current.originalAmount));
 const commit=async()=>{busy.value=true;error.value='';try{const common={accountId:draft.value.accountId,amount,time:at.unix(),timeZone:props.timeZone,comment:draft.value.comment,installmentMode};const result=previous?await refundActions.update({...common,id:previous.id,version:previous.version}):await refundActions.create({...common,expenseId:props.expenseId,requestId,expectedOriginalAmount:current.originalAmount,expectedRefundedAmount:current.refundedAmount});changed(result);showToast(previous?'退款已修改':'已添加退款');}catch(cause){error.value=investmentError(cause);}finally{busy.value=false;}};
 if(excess.gt(current.excessIncome)){busy.value=true;showConfirm(`退款将超出原支出 ${money(excess.toString())}，超出部分将单独记为一笔收入。`,()=>{busy.value=false;void commit();},()=>{busy.value=false;},{title:'退款超出原支出',confirmText:'继续保存'});}else void commit();
}
async function saveExcess(){if(busy.value||!selected.value)return;busy.value=true;error.value='';try{changed(await refundActions.updateExcess({...excessDraft.value,id:selected.value.id,version:selected.value.version}));showToast('差额设置已保存');}catch(cause){error.value=investmentError(cause);}finally{busy.value=false;}}
function remove(item:RefundRecord){if(busy.value)return;busy.value=true;showConfirm('删除后将撤销这笔退款到账，并重新计算原账单金额。',async()=>{error.value='';try{changed(await refundActions.delete(item.id,item.version));showToast('已删除退款');}catch(cause){error.value=investmentError(cause);}finally{busy.value=false;}},()=>{busy.value=false;},{title:'删除退款',confirmText:'删除',destructive:true});}
</script>
<style scoped>
.refund-message{color:var(--cy-muted);text-align:center;padding:24px 0}.refund-total{display:grid;gap:9px;padding:18px;border-radius:12px;background:var(--cy-soft);font-size:13px;color:var(--cy-muted)}.refund-total strong{font-size:20px;color:var(--cy-ink)}.refund-record{padding:18px 0;border-bottom:1px solid var(--cy-line)}.refund-record>div:first-child{display:flex;justify-content:space-between;gap:12px}.refund-record strong,.refund-record b{font-weight:500}.refund-record b{color:var(--cy-income)}.refund-record small,.refund-record p{display:block;margin:8px 0 0;color:var(--cy-muted);font-size:12px}.refund-actions{display:flex;gap:16px;margin-top:12px}.refund-actions button,.refund-form-actions>button:first-child{border:0;background:none;color:var(--cy-accent);padding:8px 0}.refund-actions .danger{color:var(--cy-expense)}.refund-primary{display:block;width:100%;border:0;border-radius:10px;background:var(--cy-accent);color:var(--cy-card);padding:14px;margin-top:20px;font:inherit}.refund-primary:disabled{opacity:.5}.refund-form{background:var(--cy-bg);border-radius:12px;padding:0 14px}.refund-form label{min-height:56px;display:flex;align-items:center;justify-content:space-between;gap:16px;border-bottom:1px solid var(--cy-line)}.refund-form label:last-child{border:0}.refund-form input:not([type=checkbox]),.refund-form select{width:64%;min-width:0;background:none;border:0;text-align:right;color:var(--cy-ink);font:inherit}.refund-amount-input{display:flex;align-items:center;justify-content:flex-end;gap:8px;flex:1;min-width:0}.refund-amount-input input{width:auto!important;flex:1}.refund-full-btn{flex:none;border:1px solid var(--cy-accent);background:var(--cy-soft);color:var(--cy-accent);border-radius:12px;padding:3px 9px;font-size:11px;white-space:nowrap}.refund-form input[type=checkbox]{width:20px;height:20px;accent-color:var(--cy-accent)}.refund-hint,.refund-error,.refund-label{font-size:12px;line-height:1.6;color:var(--cy-muted);margin:14px 0}.refund-error{color:var(--cy-expense)}.refund-tags{display:flex;flex-wrap:wrap;gap:12px;font-size:13px}.refund-tags label{display:flex;align-items:center;gap:5px}.refund-form-actions{display:flex;align-items:center;gap:20px}.refund-form-actions .refund-primary{flex:1}.refund-form-actions>button:first-child{min-width:50px}
</style>
