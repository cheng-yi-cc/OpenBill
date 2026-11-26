<template>
 <div class="adjustment-details">
  <p v-if="error" role="alert">{{error}}</p>
  <label v-if="books.activeBooks.length > 1"><span>账本</span><select :value="effectiveBook" @change="setBook(($event.target as HTMLSelectElement).value)"><option v-for="book in books.activeBooks" :key="book.id" :value="book.id">{{book.name}}{{book.isDefault?'（默认）':''}}</option></select></label>
  <label><span>{{income?'收入分类':'支出分类'}}</span><select :value="modelValue.categoryId" @change="update({categoryId:($event.target as HTMLSelectElement).value})"><option value="">{{income?'其他收入（默认）':'其他（默认）'}}</option><option v-for="category in availableCategories" :key="category.id" :value="category.id">{{categories.allTransactionCategoriesMap[category.parentId]?.name}} · {{category.name}}</option></select></label>
  <label><span>备注</span><input :value="modelValue.comment" maxlength="200" aria-label="差额账单备注" @input="update({comment:($event.target as HTMLInputElement).value})"/></label>
  <button type="button" @click="showTags=true"><span>标签</span><span>{{tagLabel}}</span></button>
  <transaction-tag-selection-sheet :allow-add-new-tag="true" :enable-filter="true" v-model:show="showTags" :model-value="modelValue.tagIds" @update:model-value="update({tagIds:$event})"/>
 </div>
</template>
<script setup lang="ts">
import {computed,onMounted,ref,watch} from 'vue';
import type {BalanceAdjustmentDetails} from '@/lib/asset-tools.ts';
import {investmentError} from '@/lib/investments.ts';
import {categoryAvailableInBook} from '@/lib/category-book-visibility.ts';
import {useBooksStore} from '@/stores/books.ts';
import {useTransactionCategoriesStore} from '@/stores/transactionCategory.ts';
import {useTransactionTagsStore} from '@/stores/transactionTag.ts';
import TransactionTagSelectionSheet from './TransactionTagSelectionSheet.vue';
const props=defineProps<{modelValue:BalanceAdjustmentDetails;income:boolean}>();
const emit=defineEmits<{'update:modelValue':[value:BalanceAdjustmentDetails]}>();
const books=useBooksStore(),categories=useTransactionCategoriesStore(),tags=useTransactionTagsStore(),showTags=ref(false),error=ref('');
const effectiveBook=computed(()=>props.modelValue.bookId||books.activeBooks.find(b=>b.isDefault)?.id||'');
const availableCategories=computed(()=>Object.values(categories.allTransactionCategoriesMap).filter(c=>{const parent=categories.allTransactionCategoriesMap[c.parentId],book=books.allBooks.find(b=>b.id===effectiveBook.value);return c.type===(props.income?1:2)&&c.parentId!=='0'&&!!parent&&categoryAvailableInBook(c,effectiveBook.value,book)&&categoryAvailableInBook(parent,effectiveBook.value,book);}));
const tagLabel=computed(()=>props.modelValue.tagIds.map(id=>tags.allTransactionTagsMap[id]?.name).filter(Boolean).join('、')||'选择标签');
function update(value:Partial<BalanceAdjustmentDetails>){emit('update:modelValue',{...props.modelValue,...value});}
function setBook(bookId:string){update({bookId,categoryId:''});}
watch(()=>props.income,()=>{if(props.modelValue.categoryId)update({categoryId:''});});
onMounted(()=>{void Promise.all([books.loadBooks(),categories.loadAllCategories({force:false}),tags.loadAllTags({force:false})]).catch(cause=>{error.value=investmentError(cause);});});
</script>
<style scoped>
.adjustment-details{padding:0 14px}.adjustment-details label,.adjustment-details button{display:flex;align-items:center;justify-content:space-between;gap:12px;min-height:48px;width:100%;border:0;border-top:1px solid var(--cy-line);background:none;color:var(--cy-ink);font:inherit;text-align:left}.adjustment-details select,.adjustment-details input{width:65%;min-width:0;border:0;background:none;text-align:right;color:var(--cy-ink);font:inherit}.adjustment-details button span:last-child,.adjustment-details p{color:var(--cy-muted)}.adjustment-details p{font-size:12px}
</style>
