<template>
  <StatisticsSheet v-if="field && field!=='date'" :open="!!field" :title="title" @update:open="!$event && close()">
   <template #action><button v-if="field!=='links' || linking" :disabled="busy" @click="save">{{ busy?'保存中':'保存' }}</button></template>
   <div v-if="draft" class="detail-form">
    <p v-if="error" class="detail-error" role="alert">{{ error }}</p>
    <template v-if="field==='comment'"><label>备注<textarea v-model="draft.comment" maxlength="255" rows="5" placeholder="填写备注" /></label><button @click="draft.comment=''">清空备注</button></template>
    <template v-else-if="field==='location'"><label>地点名称<input v-model="draft.locationName" maxlength="200" placeholder="公司、家、商场" /></label><label>地址信息<textarea v-model="draft.locationAddress" maxlength="500" rows="3" placeholder="详细地址" /></label><p v-if="draft.geoLocation">坐标 {{ draft.geoLocation.latitude }}, {{ draft.geoLocation.longitude }}</p><button @click="locate">使用当前位置</button><button @click="draft.locationName='';draft.locationAddress='';draft.removeGeoLocation()">清空地点</button></template>
    <template v-else-if="field==='tags'"><button @click="tagsOpen=true">选择标签</button><p>{{ draft.tagIds.map(id=>tags.allTransactionTagsMap[id]?.name).join('、') || '无标签' }}</p></template>
    <template v-else-if="field==='book'"><button v-for="book in books.allBooks.filter(b=>!b.archived||b.id===draft?.bookId)" :key="book.id" :aria-pressed="draft.bookId===book.id" @click="draft.bookId=book.id">{{ book.icon }} {{ book.name }} <span v-if="draft.bookId===book.id">✓</span></button></template>
    <template v-else-if="field==='source'||field==='destination'||field==='reimbursement'"><input v-model="accountSearch" placeholder="搜索账户" aria-label="搜索账户" /><button v-if="field==='reimbursement'" @click="draft.reimbursementAccountId='0'">不报销 <span v-if="draft.reimbursementAccountId==='0'">✓</span></button><button v-for="account in selectableAccounts" :key="account.id" :aria-pressed="selectedAccount===account.id" @click="selectAccount(account.id)">{{ account.name }} <small>{{ account.currency }}</small><span v-if="selectedAccount===account.id">✓</span></button></template>
    <template v-else-if="field==='pictures'"><div v-for="picture in draft.pictures" :key="picture.pictureId" class="detail-picture"><img :src="store.getTransactionPictureUrl(picture)" alt="账单附件" /><button aria-label="移除此附件" @click="draft.removePicture(picture)">移除</button></div><input ref="fileInput" type="file" accept="image/*" hidden @change="upload" /><button :disabled="busy || (draft.pictures?.length||0)>=10" @click="fileInput?.click()">添加附件</button><p>保存后应用附件变更。</p></template>
    <template v-else-if="field==='debt'"><label>约定还款日期<input type="date" v-model="draft.debtDueDate" /></label><button @click="draft.debtDueDate=''">清除日期</button></template>
    <template v-else-if="field==='discount'"><label>优惠金额（{{ currency }}）<input v-model="draft.discountAmount" inputmode="decimal" maxlength="16" /></label><p>当前实收付金额保持 {{ money(draft.sourceAmount) }} {{ currency }}，如需调整金额，请进入完整编辑。</p></template>
    <template v-else-if="field==='destinationAmount'"><label>转入金额<input v-model="amount" inputmode="decimal" maxlength="16" /></label></template>
    <template v-else-if="field==='fee'"><label>手续费（{{ currency }}）<input v-model="draft.transferFeeAmount" inputmode="decimal" maxlength="17" /></label><label>手续费分类<select v-model="draft.transferFeeCategoryId"><option value="0">请选择</option><option v-for="c in feeCategories" :key="c.id" :value="c.id">{{ c.name }}</option></select></label><p>正数为手续费，负数为转账优惠。</p></template>
    <template v-else-if="field==='links'">
      <button v-if="!linking && mutable" @click="linking=true;searchLinks()">＋ 关联已有账单</button>
      <template v-if="linking">
        <input v-model="keyword" placeholder="搜索备注" aria-label="搜索关联账单" @keyup.enter="searchLinks()" />
        <button @click="searchLinks()">搜索</button>
        <label v-for="item in candidates" :key="item.id" class="detail-choice"><input type="checkbox" :value="item.id" v-model="draft.linkedTransactionIds" />{{ item.comment || item.category?.name || '账单' }} · {{ dateText(item.time) }} · {{ item.hideAmount?'••••':money(item.sourceAmount) }}</label>
        <button v-if="more" @click="searchLinks(true)">加载更多</button>
      </template>
      <template v-else>
        <p v-if="!related.length">暂无关联账单</p>
        <div v-for="item in related" :key="item.id" class="detail-related">
          <button @click="emit('navigate',`/transaction/detail?id=${item.id}&type=${item.type}`);close()">{{ item.refund?.kind==='cash'?'退款 · ':'' }}{{ item.comment || item.category?.name || '账单' }}<small>{{ dateText(item.time) }} · {{ item.hideAmount?'••••':money(item.sourceAmount) }}</small></button>
          <button v-if="mutable && draft.linkedTransactionIds.includes(item.id)" @click="draft.linkedTransactionIds=draft.linkedTransactionIds.filter(id=>id!==item.id);save()">解除关联</button>
        </div>
      </template>
    </template>
   </div>
  </StatisticsSheet>
  <bill-detail-date-sheet v-if="draft" :show="dateOpen" :model-value="draft.time" :time-zone="props.timeZone" :busy="busy" :error="dateError" @update:show="onDateSheetClosed" @confirm="onDateConfirm" />
  <transaction-tag-selection-sheet v-if="draft" :allow-add-new-tag="true" :enable-filter="true" v-model:show="tagsOpen" v-model="draft.tagIds" />
</template>
<script setup lang="ts">
import {computed,ref,useTemplateRef} from 'vue';
import moment from 'moment-timezone';
import axios from 'axios';
import StatisticsSheet from './StatisticsSheet.vue';
import BillDetailDateSheet from './BillDetailDateSheet.vue';
import {Transaction,type TransactionInfoResponse} from '@/models/transaction.ts';
import {useTransactionsStore} from '@/stores/transaction.ts';
import {useAccountsStore} from '@/stores/account.ts';
import {useTransactionCategoriesStore} from '@/stores/transactionCategory.ts';
import {useTransactionTagsStore} from '@/stores/transactionTag.ts';
import {useBooksStore} from '@/stores/books.ts';
import {LedgerDecimal} from '@/lib/ledger-display.ts';
import services from '@/lib/services.ts';
import {generateRandomUUID} from '@/lib/misc.ts';

const props=defineProps<{transaction:Transaction;timeZone:string}>();
const emit=defineEmits<{saved:[transaction:Transaction];navigate:[path:string]}>();
const store=useTransactionsStore(),accounts=useAccountsStore(),tags=useTransactionTagsStore(),books=useBooksStore(),categories=useTransactionCategoriesStore();
const field=ref(''),draft=ref<Transaction>(),busy=ref(false),error=ref(''),dateError=ref(''),dateOpen=ref(false),tagsOpen=ref(false),amount=ref(''),accountSearch=ref(''),keyword=ref(''),linking=ref(false),more=ref(false),page=ref(1),key=ref('');
const related=ref<TransactionInfoResponse[]>([]),candidates=ref<TransactionInfoResponse[]>([]),fileInput=useTemplateRef<HTMLInputElement>('fileInput');
const title=computed(()=>({comment:'备注',location:'账单地点',date:'账单日期',tags:'标签',book:'所属账本',source:'收支账户',destination:'转入账户',reimbursement:'报销账户',pictures:'附件',debt:'约定还款日期',discount:'优惠金额',fee:'转账手续费',destinationAmount:'转入金额',links:'关联账单'}[field.value]||'账单属性'));
const mutable=computed(()=>props.transaction.editable&&!props.transaction.investmentEventId&&props.transaction.transferFeeParentId==='0'&&!props.transaction.reimbursementReceiptId);
const currency=computed(()=>accounts.allAccountsMap[props.transaction.sourceAccountId]?.currency||'CNY');
const ordinaryAccounts=computed(()=>Object.values(accounts.allAccountsMap).filter(a=>a.visible&&a.type!==2));
const selectableAccounts=computed(()=>ordinaryAccounts.value.filter(a=>(field.value==='reimbursement'?a.assetProfile.kind==='reimbursement'&&a.currency===currency.value:a.assetProfile.kind!=='reimbursement'&&(!props.transaction.originalCurrency||field.value!=='source'||a.currency===currency.value))&&a.name.includes(accountSearch.value)));
const selectedAccount=computed(()=>field.value==='destination'?draft.value?.destinationAccountId:field.value==='reimbursement'?draft.value?.reimbursementAccountId:draft.value?.sourceAccountId);
const feeCategories=computed(()=>Object.values(categories.allTransactionCategoriesMap).filter(c=>c.parentId!=='0'&&!c.hidden&&c.type===(draft.value?.transferFeeAmount.startsWith('-')?1:2)));
function dateText(time:number){return moment.unix(time).tz(props.timeZone).format('YYYY年MM月DD日 HH:mm');}
function money(minor:number){return new LedgerDecimal(minor).div(100).toFixed(2);}
function close(){if(busy.value)return;field.value='';dateOpen.value=false;tagsOpen.value=false;dateError.value='';}
function selectAccount(id:string){if(!draft.value)return;if(field.value==='destination')draft.value.destinationAccountId=id;else if(field.value==='reimbursement')draft.value.reimbursementAccountId=id;else draft.value.sourceAccountId=id;}
async function open(name:string){
 if(busy.value)return;draft.value=Transaction.of(props.transaction);field.value=name;error.value='';accountSearch.value='';linking.value=false;keyword.value='';related.value=[];key.value=generateRandomUUID();amount.value=name==='destinationAmount'?money(props.transaction.destinationAmount):'';
 if(name==='date')dateOpen.value=true;if(name==='tags')tagsOpen.value=true;
 dateError.value='';
 if(name==='links') {busy.value=true;try{const response=await axios.get<{success:boolean;result:{ids:string[]}}>(`v1/ledger/relations?id=${props.transaction.id}`);if(!response.data.success)throw new Error('无法读取关联账单');
  const results=await Promise.allSettled(response.data.result.ids.map(id=>services.getTransaction({id,withPictures:false})));related.value=results.flatMap(r=>r.status==='fulfilled'&&r.value.data.success?[r.value.data.result]:[]);
 }catch(cause){error.value=message(cause)}finally{busy.value=false}}
}
function message(cause:unknown){const e=cause as {message?:string;error?:{errorMessage?:string};response?:{data?:{errorMessage?:string}}};return e.response?.data?.errorMessage||e.error?.errorMessage||e.message||'保存失败，请重试';}
function onDateSheetClosed(open:boolean):void{
  if(open||!draft.value)return;
  dateOpen.value=false;
  if(field.value==='date'){field.value='';draft.value=undefined;dateError.value='';}
  else dateError.value='';
}
async function onDateConfirm(value:number):Promise<void>{
  if(!draft.value||busy.value)return;
  if(value===props.transaction.time){dateOpen.value=false;return;}
  draft.value.time=value;
  draft.value.utcOffset=moment.unix(value).tz(props.timeZone).utcOffset();
  if(field.value!=='date'){dateOpen.value=false;return;}
  dateError.value='';
  busy.value=true;
  try{
    const saved=await store.saveTransaction({transaction:draft.value,defaultCurrency:'CNY',isEdit:true,clientSessionId:key.value});
    emit('saved',saved);busy.value=false;close();
  }catch(cause){dateError.value=message(cause);busy.value=false;}
}
async function save(){
 if(!draft.value||busy.value)return;error.value='';const d=draft.value;
 if(field.value==='destinationAmount'){if(!/^(0|[1-9]\d{0,12})(\.\d{1,2})?$/.test(amount.value)){error.value='请填写有效金额，最多两位小数';return}const n=new LedgerDecimal(amount.value);d.destinationAmount=n.mul(100).toNumber();}
 if(field.value==='date')d.utcOffset=moment.unix(d.time).tz(props.timeZone).utcOffset();
 if(d.type===4&&accounts.allAccountsMap[d.sourceAccountId]?.currency===accounts.allAccountsMap[d.destinationAccountId]?.currency){if(field.value==='destinationAmount')d.sourceAmount=d.destinationAmount;else d.destinationAmount=d.sourceAmount;}
 busy.value=true;try{const saved=await store.saveTransaction({transaction:d,defaultCurrency:'CNY',isEdit:true,clientSessionId:key.value});emit('saved',saved);busy.value=false;close();}catch(cause){error.value=message(cause)}finally{busy.value=false}
}
async function searchLinks(append=false){if(busy.value||!draft.value)return;busy.value=true;error.value='';if(!append){page.value=1;candidates.value=[]}try{const response=await services.getTransactions({maxTime:(moment().unix()+86400)*1000,minTime:0,count:50,page:page.value,withCount:false,type:0,categoryIds:'',accountIds:'',tagFilter:'',amountFilter:'',keyword:keyword.value,matchMode:0,bookIds:[]});if(!response.data.success)throw new Error('无法读取账单');candidates.value.push(...response.data.result.items.filter(item=>item.id!==props.transaction.id));more.value=response.data.result.items.length===50;page.value++;}catch(cause){error.value=message(cause)}finally{busy.value=false}}
async function upload(event:Event){const input=event.target as HTMLInputElement,file=input.files?.[0];input.value='';if(!file||!draft.value)return;busy.value=true;try{draft.value.addPicture(await store.uploadTransactionPicture({pictureFile:file}));}catch(cause){error.value=message(cause)}finally{busy.value=false}}
function locate(){if(!navigator.geolocation){error.value='设备不支持定位';return}navigator.geolocation.getCurrentPosition(p=>draft.value?.setLatitudeAndLongitude(p.coords.latitude,p.coords.longitude),()=>{error.value='无法获取位置，可手动填写地点';});}
defineExpose({open,busy});
</script>
<style scoped>
.detail-form{display:grid;gap:16px;font-size:14px}.detail-form label{display:grid;gap:10px}.detail-form input,.detail-form textarea,.detail-form select{box-sizing:border-box;width:100%;padding:12px;border:1px solid var(--cy-line);border-radius:8px;background:var(--cy-soft);color:var(--cy-ink);font:inherit}.detail-form button{display:flex;align-items:center;justify-content:space-between;gap:12px;border:0;background:var(--cy-soft);color:var(--cy-ink);padding:14px;border-radius:8px;font:inherit;text-align:left}.detail-form button[aria-pressed=true]{color:var(--cy-accent)}.detail-form button:disabled{opacity:.5}.detail-form p,.detail-form small{color:var(--cy-muted);line-height:1.6;margin:0}.detail-form .detail-error{color:var(--cy-expense)}.detail-picture{display:flex;align-items:center;gap:18px}.detail-picture img{width:100px;height:100px;object-fit:cover;border-radius:8px}.detail-form .detail-choice{display:flex;align-items:center;line-height:1.6}.detail-choice input{width:20px;height:20px;flex:none}.detail-related{display:flex;gap:8px}.detail-related>button:first-child{flex:1;display:grid;min-width:0;overflow-wrap:anywhere}
</style>
