<template>
 <f7-popup :opened="opened" class="cy-mobile-surface" @popup:closed="emit('update:opened',false)"><f7-page class="cy-mobile-surface cy-asset-surface"><f7-navbar title="校准余额"><f7-nav-left><f7-link icon-f7="xmark" :disabled="busy" @click="emit('update:opened',false)"/></f7-nav-left><f7-nav-right><f7-link icon-f7="checkmark" aria-label="保存余额校准" :disabled="busy" @click="save"/></f7-nav-right></f7-navbar><main class="cy-page-body"><p v-if="error" class="cy-message" role="alert">{{error}}</p><section class="cy-panel balance-form"><p>{{account.name}}</p><label>{{account.isLiability?'实际欠款':'实际余额'}}<input v-model="target" inputmode="decimal" aria-label="实际余额" maxlength="17"/></label><div>当前{{account.isLiability?'欠款':'余额'}}<span>{{ledgerCurrencyMoney(displayBalance,account.currency)}}</span></div><div v-if="deltaPreview"><span>{{deltaPreview.label}}</span><strong :class="deltaPreview.cls">{{deltaPreview.text}}</strong></div></section><section class="cy-panel balance-options"><label><span>生成差额收支账单</span><f7-toggle :checked="countInStatistics" @toggle:change="countInStatistics=$event"/></label><p>{{countInStatistics?'差额计入收入或支出。':'保存校准记录，差额不计入收入或支出。'}}</p><balance-adjustment-details v-if="countInStatistics" v-model="details" :income="adjustmentIsIncome"/></section></main></f7-page></f7-popup>
</template>
<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import type {Account} from '@/models/account.ts';
import {assetTools,defaultBalanceAdjustmentDetails} from '@/lib/asset-tools.ts';
import {LedgerDecimal,ledgerCurrencyMoney} from '@/lib/ledger-display.ts';
import {investmentError} from '@/lib/investments.ts';
import {generateRandomUUID} from '@/lib/misc.ts';
import {useI18nUIComponents} from '@/lib/ui/mobile.ts';
import {useLedgerScopeStore} from '@/stores/ledgerScope.ts';
import {useTransactionsStore} from '@/stores/transaction.ts';
import {useTransactionCategoriesStore} from '@/stores/transactionCategory.ts';
import BalanceAdjustmentDetails from './BalanceAdjustmentDetails.vue';
const props=defineProps<{account:Account;opened:boolean}>();
const emit=defineEmits<{'update:opened':[value:boolean];saved:[]}>();
const {showToast}=useI18nUIComponents();
const scope=useLedgerScopeStore(),target=ref(''),expected=ref(''),error=ref(''),busy=ref(false),countInStatistics=ref(false);let requestId=generateRandomUUID();
const details=ref(defaultBalanceAdjustmentDetails());
const displayBalance=computed(()=>new LedgerDecimal(props.account.balance).div(100).mul(props.account.isLiability?-1:1).toString());
const validTarget=computed(()=>/^-?(0|[1-9]\d{0,12})(\.\d{1,2})?$/.test(target.value));
const deltaDecimal=computed(()=>validTarget.value?new LedgerDecimal(target.value).mul(props.account.isLiability?-1:1).minus(expected.value||'0'):null);
const adjustmentIsIncome=computed(()=>!!deltaDecimal.value&&deltaDecimal.value.gt(0));
const deltaPreview=computed(()=>{
 if(!deltaDecimal.value)return null;
 if(deltaDecimal.value.isZero())return {label:'校准差额',text:'无变化',cls:''};
 const inc=deltaDecimal.value.gt(0);
 return {label:props.account.isLiability?(inc?'欠款减少':'欠款增加'):(inc?'余额增加':'余额减少'),text:ledgerCurrencyMoney(deltaDecimal.value.toString(),props.account.currency,true),cls:inc?'cy-income':'cy-expense'};
});
watch(()=>props.opened,open=>{if(open){target.value=displayBalance.value;expected.value=new LedgerDecimal(props.account.balance).div(100).toString();error.value='';countInStatistics.value=false;details.value=defaultBalanceAdjustmentDetails();requestId=generateRandomUUID();}},{immediate:true});
async function save():Promise<void>{if(busy.value)return;busy.value=true;error.value='';try{if(!validTarget.value)throw Error('请输入最多两位小数的金额');await assetTools.adjustBalance({accountId:props.account.id,balance:new LedgerDecimal(target.value).mul(props.account.isLiability?-1:1).toString(),expectedBalance:expected.value,bookId:countInStatistics.value?details.value.bookId:'',countInStatistics:countInStatistics.value,...(countInStatistics.value?{categoryId:details.value.categoryId,comment:details.value.comment,tagIds:details.value.tagIds}:{}),timeZone:scope.timeZone,requestId});useTransactionCategoriesStore().updateTransactionCategoryListInvalidState(true);useTransactionsStore().updateStoreInvalidState({accountList:true,transactionList:true,overview:true,statistics:true,explorer:true,reconciliationStatement:true});showToast('余额已校准');emit('update:opened',false);emit('saved');}catch(e){error.value=investmentError(e);}finally{busy.value=false;}}
</script>
<style scoped>
.balance-form,.balance-options{border:0!important;border-radius:12px!important}.balance-form p{font-size:14px;color:var(--cy-muted);margin-bottom:15px}.balance-form label,.balance-form>div,.balance-options label{display:flex;justify-content:space-between;align-items:center;gap:15px;min-height:44px;font-size:15px}.balance-form input{min-width:0;width:55%;background:none;border:0;color:var(--cy-accent);text-align:right;font-size:23px}.balance-form>div,.balance-options p{font-size:12px;color:var(--cy-muted)}.balance-options p{margin-top:13px;line-height:1.7}
</style>
