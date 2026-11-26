<template>
 <f7-page class="cy-main-page cy-mobile-surface cy-statistics cy-budget-page" ptr @ptr:refresh="onPullRefresh" @page:afterin="refresh">
  <f7-navbar title="预算管理" back-link="返回"><f7-nav-right><f7-link aria-label="预算更多操作" @click="sheet = 'menu'">⋮</f7-link></f7-nav-right></f7-navbar>
  <main ref="budgetMainEl" class="cy-page-body">
   <div class="cy-segments budget-mode-switch" role="group" aria-label="预算类型"><button type="button" :aria-pressed="!custom" @click="custom = false">月度预算</button><button type="button" :aria-pressed="custom" @click="custom = true">自定义预算</button></div>
   <div class="budget-scope"><select v-if="visibleBooks.length > 1" v-model="bookId" aria-label="预算账本"><option v-for="book in visibleBooks" :key="book.id" :value="book.id">{{ book.name }}</option></select><div v-if="!custom" class="budget-month"><button aria-label="上月预算" @click="shift(-1)">‹</button><input v-model="month" type="month" aria-label="预算月份" /><button aria-label="下月预算" @click="shift(1)">›</button></div></div>
   <p v-if="error" role="alert" class="cy-message">{{ error }} <button @click="refresh">重试</button></p><p v-else-if="loading" class="cy-empty">正在加载预算…</p>
   <template v-else>
    <section v-if="!custom" class="cy-panel budget-overview">
     <div class="stat-heading"><button class="stat-small" @click="editTotal">总预算 {{ total ? ledgerMoney(total.available,false) : '未设置' }} ✎</button><button v-if="total" class="stat-small" @click="openDetail(total)">查看详情 ›</button></div>
     <button class="budget-ring" :style="{ '--cy-ring-progress': total ? ringProgress(total) : 0, '--cy-ring-color': total && isOverBudget(total) ? 'var(--cy-expense)' : 'var(--cy-accent)' }" @click="total ? openDetail(total) : editTotal()"><span><small>{{ total && isOverBudget(total) ? '已超支' : workspace.preferences.budgetProgress === 'remaining' ? '剩余预算' : '已用预算' }}</small><strong :class="{ 'cy-expense': !!total && isOverBudget(total) }">{{ total ? ringAmount(total) : '—' }}</strong><small>{{ total && total.available !== null ? `预算 ¥${ledgerMoney(total.available,false)}` : '点击设置预算' }}</small></span></button>
     <p v-if="total && !total.complete" class="cy-message">部分外币账单缺少历史汇率，预算支出与剩余暂显示“—”。</p>
     <div v-if="total" class="budget-stats" :style="{ gridTemplateColumns: `repeat(${isCurrentMonth ? 4 : 3}, 1fr)` }"><f7-link :href="bills(total)"><span>预算支出</span><strong>{{ ledgerMoney(total.complete ? total.spent : null,false) }}</strong></f7-link><div><span>日均预算</span><strong>{{ ledgerMoney(daily(total).daily,false) }}</strong></div><div><span>日均支出</span><strong>{{ ledgerMoney(daily(total).average,false) }}</strong></div><div v-if="isCurrentMonth"><span>今日剩余</span><strong>{{ ledgerMoney(daily(total).todayRemaining,false) }}</strong></div></div>
     <p v-if="total && total.carry !== '0'" class="cy-muted">上期结转 {{ ledgerMoney(total.carry) }} · 本期基础预算 {{ ledgerMoney(total.base) }}</p>
    </section>
    <section v-if="total && !custom" class="cy-panel"><div class="stat-heading"><h2>预算趋势</h2><span class="cy-muted">支出 / 剩余</span></div><StatisticsChart :option="trend(total)" :height="220" /></section>
    <section class="cy-panel"><div class="stat-heading"><h2>{{ custom ? '自定义预算' : '分类预算' }}</h2><button class="stat-small" @click="newBudget(custom ? 'custom' : 'monthly', !custom)">＋ 添加</button></div><p v-if="!listed.length" class="cy-empty">{{ custom ? '为旅行、装修等计划设置独立预算' : '设置分类预算，查看各类支出进度' }}</p><button v-for="row in listed" :key="row.budget.id" class="budget-list-row" @click="openDetail(row)"><div><span>{{ row.budget.name || categoryName(row.budget.categoryId) }}</span><strong :class="{ 'cy-expense': isOverBudget(row) }">{{ isOverBudget(row) ? `超支 ¥${ledgerMoney(new LedgerDecimal(row.remaining!).abs().toString(), false)}` : ledgerMoney(row.remaining, false) }}<small> / {{ ledgerMoney(row.available,false) }}</small></strong></div><div class="stat-progress"><i :style="{width:ringProgress(row)+'%', background: isOverBudget(row) ? 'var(--cy-expense)' : 'var(--cy-accent)'}" /></div><div class="cy-muted"><span>{{ custom ? `${row.from} 至 ${row.to}` : `支出 ${ledgerMoney(row.complete ? row.spent : null,false)}` }}</span><span :class="{ 'cy-expense': isOverBudget(row) }">{{ isOverBudget(row) ? '已超支' : '剩余预算' }} ›</span></div></button></section>
    <p class="cy-muted stat-center">预算按人民币统计，退款冲减支出。外币缺少历史汇率时显示“—”。</p>
   </template>
  </main>
  <StatisticsSheet :open="!!sheet" :title="titles[sheet] || '预算'" :compact="sheet === 'menu' || sheet === 'settings'" :tall="sheet === 'detail'" :dirty="sheetDirty" @update:open="v => { if (!v) sheet = ''; }">
   <template #action><button v-if="sheet === 'edit'" :disabled="saving" @click="saveBudget">{{ saving ? '保存中…' : '保存' }}</button><button v-if="sheet === 'settings'" :disabled="saving" @click="saveSettings">{{ saving ? '保存中…' : '保存' }}</button></template>
   <p v-if="actionError" class="cy-message" role="alert">{{ actionError }} <button v-if="conflictDetected" type="button" @click="syncAfterConflict">同步最新</button><button v-else-if="duplicateExisting" type="button" @click="openDuplicateExisting">打开已有预算</button></p>
   <template v-if="sheet === 'menu'"><button class="stat-menu-item" @click="openSettings">预算设置<span>›</span></button><f7-link v-if="total" class="stat-menu-item" :href="bills(total)" @click="sheet = ''">查看预算支出<span>›</span></f7-link><button class="stat-menu-item" @click="sheet = 'help'">预算规则说明<span>›</span></button></template>
   <form v-else-if="sheet === 'edit'" class="budget-form" @submit.prevent="saveBudget">
    <p v-if="inheritedSourceMonth" class="cy-message">当前沿用 {{ inheritedSourceMonth }} 的预算规则，保存后将从本月起使用新金额。<button type="button" @click="openInheritedSource">管理源规则</button></p>
    <template v-if="draft.id"><p class="cy-muted">{{ categoryName(draft.categoryId) }} · {{ draft.kind === 'custom' ? `${draft.startDate} 至 ${draft.endDate}（周期不可改，如需调整请新建）` : draft.startDate.slice(0,7) }}</p></template>
    <template v-else>
     <label v-if="draft.kind === 'custom'">计划名称<input v-model="draft.name" maxlength="64" placeholder="如：日本旅行、装修计划" /></label>
     <label v-if="!editingTotalOnly">分类<select v-model="draft.categoryId"><option v-if="draft.kind === 'custom'" value="0">全部支出</option><option v-for="category in availableCategoriesForDraft" :key="category.id" :value="category.id">{{ category.name }}</option></select></label>
    </template>
    <label>预算金额（元）<input v-model="draft.amount" inputmode="decimal" placeholder="0.00" maxlength="16" required /></label>
    <template v-if="draft.kind === 'custom' && !draft.id"><label>开始日期<input v-model="draft.startDate" type="date" required /></label><label>结束日期<input v-model="draft.endDate" type="date" required /></label></template>
    <template v-else-if="draft.kind === 'monthly'"><label class="budget-toggle"><span>后续月份沿用此金额</span><input v-model="draft.repeat" type="checkbox" /></label></template>
    <p class="cy-muted">总预算与分类预算分开统计，分类预算不会重复叠加到总预算。</p>
    <button v-if="draft.id" type="button" class="cy-button cy-danger" :disabled="saving" @click="confirmDelete = true">删除此预算规则</button>
    <div v-if="confirmDelete" class="cy-message"><p>删除此规则后，预算与后续结转会重新计算，账单本身保留。</p><div class="cy-actions"><button type="button" class="cy-button" @click="confirmDelete = false">取消</button><button type="button" class="cy-button cy-danger" :disabled="saving" @click="deleteBudget">确认删除</button></div></div>
   </form>
   <template v-else-if="sheet === 'detail' && selected"><div class="stat-heading"><h2>{{ selected.budget.name || categoryName(selected.budget.categoryId) }}</h2><button class="stat-small" @click="editRule(selected)">编辑</button></div><p class="cy-muted">{{ selected.from }} 至 {{ selected.to }}</p><div class="budget-detail-values"><div><span>可用预算</span><strong>{{ ledgerMoney(selected.available,false) }}</strong></div><div><span>预算支出</span><strong>{{ ledgerMoney(selected.complete ? selected.spent : null,false) }}</strong></div><div><span>{{ isOverBudget(selected) ? '已超支' : '剩余预算' }}</span><strong :class="{ 'cy-expense': isOverBudget(selected) }">{{ isOverBudget(selected) ? ledgerMoney(new LedgerDecimal(selected.remaining!).abs().toString(), false) : ledgerMoney(selected.remaining, false) }}</strong></div></div><StatisticsChart :option="trend(selected)" :height="280" /><f7-link class="cy-button cy-primary" :href="bills(selected)" @click="sheet = ''">查看预算支出账单</f7-link><p v-if="selected.budget.startDate !== selected.from && selected.budget.kind === 'monthly'" class="cy-muted">沿用 {{ selected.budget.startDate.slice(0,7) }} 的规则。点「编辑」可修改本月预算或管理源规则。</p></template>
   <template v-else-if="sheet === 'settings'">
    <label class="budget-toggle"><span>多余预算累加到下月<small>当月未花完的预算自动结转到下月</small></span><input v-model="preferencesDraft.carrySurplus" type="checkbox" /></label>
    <label class="budget-toggle"><span>超额支出从下月扣除<small>当月超支金额会从下月可用预算中扣除</small></span><input v-model="preferencesDraft.carryDeficit" type="checkbox" /></label>
    <div class="budget-form">
     <label>日均预算计算<select v-model="preferencesDraft.dailyBudgetMode"><option value="remaining">剩余均摊（按剩余天数平摊）</option><option value="fixed">固定均摊（按整月天数平摊）</option></select></label>
     <label>圆环优先显示<select v-model="preferencesDraft.budgetProgress"><option value="remaining">剩余预算</option><option value="spent">已用预算</option></select></label>
    </div>
   </template>
   <template v-else-if="sheet === 'help'"><div class="budget-help"><p>月预算按账本和分类独立设置。开启沿用后，后续月份会使用最近一次规则；某月另设预算时，该月规则优先。</p><p>结转从最早设置的月份逐月计算。修改历史流水、预算或结转开关，会重新计算后续可用预算；这些操作不会改变账户余额。</p><p>剩余均摊：先扣除今天以前的支出，再按包含今天的剩余天数均摊。固定均摊：本期可用预算除以本期天数。</p><p>退款冲减支出。转账、投资结算、报销支出与被标记为不计入预算的账单不消耗预算。缺少历史汇率时，不推算人民币金额。</p><p>自定义预算使用指定日期范围，不自动结转；与月预算重叠时，各自独立展示。</p></div></template>
  </StatisticsSheet>
 </f7-page>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import moment from 'moment-timezone';
import type { Router } from 'framework7/types';
import type { EChartsOption } from 'echarts';
import StatisticsChart from '@/components/mobile/StatisticsChart.vue';
import StatisticsSheet from '@/components/mobile/StatisticsSheet.vue';
import { useBooksStore } from '@/stores/books.ts';
import { useLedgerScopeStore } from '@/stores/ledgerScope.ts';
import { useStatisticsWorkspaceStore } from '@/stores/statisticsWorkspace.ts';
import { useTransactionCategoriesStore } from '@/stores/transactionCategory.ts';
import { useMobileLedger, ledgerMoney, LedgerDecimal } from '@/lib/mobile-ledger.ts';
import { budgetReports, budgetDaily, reportBuckets, type BudgetReport } from '@/lib/statistics-report.ts';
import { defaultStatisticsPreferences, type StatisticsBudget } from '@/lib/statistics-workspace.ts';
import { statisticsLink } from '@/lib/statistics-links.ts';
import { investmentError } from '@/lib/investments.ts';
import { isRevisionConflict } from '@/lib/error-message.ts';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import { animateSettle } from '@/lib/motion.ts';
const props=defineProps<{f7route:Router.Route}>();
const budgetMainEl = ref<HTMLElement>();
const { showToast } = useI18nUIComponents();
const books=useBooksStore(),scope=useLedgerScopeStore(),workspace=useStatisticsWorkspaceStore(),categories=useTransactionCategoriesStore();
const now=()=>moment().tz(scope.timeZone),month=ref(now().format('YYYY-MM')),bookId=ref(props.f7route.query['bookId']||books.defaultBookId),custom=ref(false);
const {entries,load,loading,error:ledgerError}=useMobileLedger({applyFilters:()=>false,bookIds:()=>bookId.value?[bookId.value]:[]});
const error=ref(''),sheet=ref(''),saving=ref(false),actionError=ref(''),confirmDelete=ref(false),selectedId=ref('');
const conflictDetected=ref(false),duplicateExisting=ref<BudgetReport|null>(null),inheritedSourceMonth=ref(''),inheritedSourceBudget=ref<StatisticsBudget|null>(null),editingTotalOnly=ref(false);
const initialDraftSnapshot=ref('');
const titles:Record<string,string>={menu:'预算管理',edit:'设置预算',detail:'预算详情',settings:'预算设置',help:'预算规则说明'};
const visibleBooks=computed(()=>books.allBooks.filter(item=>!item.hidden||item.id===bookId.value));
const isCurrentMonth=computed(()=>month.value===now().format('YYYY-MM'));
const expenseCategories=computed(()=>Object.values(categories.allTransactionCategoriesMap).filter(c=>c.type===2&&(!c.parentId||c.parentId==='0')&&!c.hidden));
const availableCategoriesForDraft=computed(()=>{
 if(draft.value.kind==='custom'||draft.value.id)return expenseCategories.value;
 const used=new Set(workspace.budgets.filter(b=>b.bookId===bookId.value&&b.kind==='monthly'&&b.startDate===draft.value.startDate&&b.categoryId!=='0').map(b=>b.categoryId));
 return expenseCategories.value.filter(c=>!used.has(c.id));
});
function categoryName(id:string){return id==='0'?'总预算':categories.allTransactionCategoriesMap[id]?.name||'已删除分类';}
const reports=computed(()=>budgetReports(workspace.budgets.filter(b=>b.bookId===bookId.value),entries.value,month.value,workspace.preferences));
const customReports=computed(()=>workspace.budgets.filter(b=>b.bookId===bookId.value&&b.kind==='custom').flatMap(b=>budgetReports([b],entries.value,b.startDate.slice(0,7),workspace.preferences)));
const total=computed(()=>reports.value.find(r=>r.budget.kind==='monthly'&&r.budget.categoryId==='0'));
const listed=computed(()=>custom.value?customReports.value:reports.value.filter(r=>r.budget.kind==='monthly'&&r.budget.categoryId!=='0'));
const selected=computed(()=>[...reports.value,...customReports.value].find(r=>r.budget.id===selectedId.value));
function isOverBudget(row:BudgetReport):boolean{return row.remaining!==null && new LedgerDecimal(row.remaining).lt(0);}
function daily(row:BudgetReport){return budgetDaily(row,entries.value,now(),workspace.preferences.dailyBudgetMode);}
function ringProgress(row:BudgetReport){if(isOverBudget(row))return 100;if(row.available===null||new LedgerDecimal(row.available).lte(0))return 0;const amount=workspace.preferences.budgetProgress==='remaining'?row.remaining:row.spent;return amount===null?0:LedgerDecimal.max(0,LedgerDecimal.min(100,new LedgerDecimal(amount).div(row.available).mul(100))).toNumber();}
function ringAmount(row:BudgetReport){if(isOverBudget(row))return `¥${ledgerMoney(new LedgerDecimal(row.remaining!).abs().toString(),false)}`;const val=workspace.preferences.budgetProgress==='remaining'?row.remaining:row.complete?row.spent:null;return val===null?'—':`¥${ledgerMoney(val,false)}`;}
function bills(row:BudgetReport){return statisticsLink(moment.tz(row.from,scope.timeZone).unix(),moment.tz(row.to,scope.timeZone).endOf('day').unix(),{bookIds:row.budget.bookId,categoryId:row.budget.categoryId==='0'?'':row.budget.categoryId,primary:'true',type:'3',aux:'budget',title:`${row.budget.name||categoryName(row.budget.categoryId)}·预算支出`});}
function trend(row:BudgetReport):EChartsOption{const from=moment.tz(row.from,scope.timeZone),rawTo=moment.tz(row.to,scope.timeZone).endOf('day'),todayEnd=now().endOf('day'),to=from.isSameOrBefore(todayEnd)&&rawTo.isAfter(todayEnd)?todayEnd:rawTo,data=reportBuckets(entries.value.filter(e=>e.type===3&&e.bookId===row.budget.bookId&&(row.budget.categoryId==='0'||e.categoryId===row.budget.categoryId||e.primaryCategoryId===row.budget.categoryId)),from,to,to.diff(from,'days')>366?'month':'day',true);let remaining=row.available===null?null:new LedgerDecimal(row.available);const balances=data.map(d=>{if(!d.complete)remaining=null;else if(remaining!==null)remaining=remaining.minus(d.expense);return remaining?.toNumber()??null;});return {grid:{left:12,right:15,top:35,bottom:28,containLabel:true},legend:{top:0,data:['支出','剩余'],textStyle:{color:'#89979d',fontSize:11}},tooltip:{trigger:'axis',confine:true},xAxis:{type:'category',data:data.map(d=>d.label),axisLabel:{color:'#89979d',fontSize:10}},yAxis:{type:'value',splitNumber:3,axisLabel:{color:'#89979d',fontSize:10},splitLine:{lineStyle:{color:'#71808722'}}},series:[{name:'支出',type:'line',data:data.map(d=>d.complete?new LedgerDecimal(d.expense).toNumber():null),itemStyle:{color:'#ff626a'},connectNulls:false,symbolSize:3},{name:'剩余',type:'line',data:balances,itemStyle:{color:'#48b69d'},connectNulls:false,symbolSize:3}]};}
function emptyBudget(kind:'monthly'|'custom',categoryId='0'):StatisticsBudget{const date=moment.tz(`${month.value}-01`,scope.timeZone);return {id:'',revision:'0',bookId:bookId.value,categoryId,name:'',amount:'',kind,repeat:kind==='monthly',startDate:date.format('YYYY-MM-DD'),endDate:date.clone().endOf('month').format('YYYY-MM-DD')};}
const draft=ref<StatisticsBudget>(emptyBudget('monthly')),preferencesDraft=ref(defaultStatisticsPreferences());
const sheetDirty=computed(()=>{
 if(sheet.value==='edit')return JSON.stringify(draft.value)!==initialDraftSnapshot.value;
 if(sheet.value==='settings')return JSON.stringify(preferencesDraft.value)!==initialDraftSnapshot.value;
 return false;
});
watch(sheet,()=>{actionError.value='';confirmDelete.value=false;conflictDetected.value=false;duplicateExisting.value=null;});
function newBudget(kind:'monthly'|'custom',forCategory=false){
 inheritedSourceMonth.value='';inheritedSourceBudget.value=null;editingTotalOnly.value=kind==='monthly'&&!forCategory;
 let defaultCat='0';
 if(kind==='monthly'&&forCategory){
  const startDate=`${month.value}-01`;
  const used=new Set(workspace.budgets.filter(b=>b.bookId===bookId.value&&b.kind==='monthly'&&b.startDate===startDate&&b.categoryId!=='0').map(b=>b.categoryId));
  const first=expenseCategories.value.find(c=>!used.has(c.id));
  if(!first){showToast('本月全部一级支出分类均已设置预算');return;}
  defaultCat=first.id;
 }
 draft.value=emptyBudget(kind,defaultCat);
 initialDraftSnapshot.value=JSON.stringify(draft.value);
 sheet.value='edit';
}
function editTotal(){if(total.value)editRule(total.value);else newBudget('monthly',false);}
function openDetail(row:BudgetReport){selectedId.value=row.budget.id;sheet.value='detail';}
function editRule(row:BudgetReport){
 editingTotalOnly.value=row.budget.kind==='monthly'&&row.budget.categoryId==='0';
 draft.value={...row.budget};
 if(row.budget.kind==='monthly'&&row.from!==row.budget.startDate){
  inheritedSourceMonth.value=row.budget.startDate.slice(0,7);
  inheritedSourceBudget.value={...row.budget};
  draft.value.id='';draft.value.revision='0';draft.value.startDate=row.from;draft.value.endDate=row.to;
 }else{
  inheritedSourceMonth.value='';inheritedSourceBudget.value=null;
 }
 initialDraftSnapshot.value=JSON.stringify(draft.value);
 sheet.value='edit';
}
function openInheritedSource(){
 if(!inheritedSourceBudget.value)return;
 const src=inheritedSourceBudget.value;
 month.value=src.startDate.slice(0,7);
 draft.value={...src};
 inheritedSourceMonth.value='';inheritedSourceBudget.value=null;
 initialDraftSnapshot.value=JSON.stringify(draft.value);
 showToast(`已切换到源规则月份 ${month.value}`);
}
function openDuplicateExisting(){
 if(!duplicateExisting.value)return;
 editRule(duplicateExisting.value);
}
async function syncAfterConflict(){
 saving.value=true;actionError.value='';conflictDetected.value=false;
 try{
  await workspace.load(true);
  if(sheet.value==='edit' && draft.value.id){
   const latest=workspace.budgets.find(b=>b.id===draft.value.id);
   if(latest){draft.value.revision=latest.revision;showToast('已同步最新版本，请核对后重新保存');}
   else{draft.value.id='';draft.value.revision='0';showToast('原规则已不存在，可直接保存为新预算');}
  }else if(sheet.value==='settings'){
   preferencesDraft.value={...preferencesDraft.value,revision:workspace.preferences.revision};
   showToast('已同步最新设置版本，请重新保存');
  }
 }catch(cause){actionError.value=investmentError(cause);}finally{saving.value=false;}
}
async function saveBudget(){
 conflictDetected.value=false;duplicateExisting.value=null;
 const amountText=draft.value.amount.trim();
 if(!/^(0|[1-9]\d{0,12})(\.\d{1,2})?$/.test(amountText)||new LedgerDecimal(amountText).lte(0)){actionError.value='请输入大于 0 且最多两位小数的预算金额。';return;}
 draft.value.amount=amountText;
 if(!bookId.value){actionError.value='请先选择账本。';return;}
 if(draft.value.kind==='custom' && draft.value.endDate < draft.value.startDate){actionError.value='结束日期不能早于开始日期。';return;}
 if(!draft.value.id){
  const dup=workspace.budgets.find(b=>b.bookId===bookId.value&&b.categoryId===draft.value.categoryId&&b.startDate===draft.value.startDate&&b.endDate===draft.value.endDate&&b.kind===draft.value.kind);
  if(dup){
   duplicateExisting.value=[...reports.value,...customReports.value].find(r=>r.budget.id===dup.id)||null;
   actionError.value='这个周期已有同分类预算，请直接编辑已有预算。';return;
  }
 }
 saving.value=true;actionError.value='';
 try{await workspace.saveBudget({...draft.value,bookId:bookId.value});initialDraftSnapshot.value=JSON.stringify(draft.value);sheet.value='';showToast('预算已保存');await refresh();}
 catch(cause){conflictDetected.value=isRevisionConflict(cause);actionError.value=conflictDetected.value?'该预算刚被更新，点击右侧「同步最新」后即可继续保存（已保留你的输入）。':investmentError(cause);}
 finally{saving.value=false;}
}
async function deleteBudget(){saving.value=true;actionError.value='';conflictDetected.value=false;try{await workspace.deleteBudget(draft.value);sheet.value='';showToast('预算规则已删除');await refresh();}catch(cause){conflictDetected.value=isRevisionConflict(cause);actionError.value=investmentError(cause);}finally{saving.value=false;}}
function openSettings(){preferencesDraft.value=JSON.parse(JSON.stringify(workspace.preferences));initialDraftSnapshot.value=JSON.stringify(preferencesDraft.value);sheet.value='settings';}
async function saveSettings(){saving.value=true;actionError.value='';conflictDetected.value=false;try{await workspace.savePreferences(preferencesDraft.value);initialDraftSnapshot.value=JSON.stringify(preferencesDraft.value);sheet.value='';showToast('预算设置已保存');}catch(cause){conflictDetected.value=isRevisionConflict(cause);actionError.value=conflictDetected.value?'设置刚被更新，请先同步最新版本再保存。':investmentError(cause);}finally{saving.value=false;}}
function shift(amount:number){month.value=moment(month.value,'YYYY-MM').add(amount,'month').format('YYYY-MM');animateSettle(budgetMainEl.value,{x:amount*14,duration:170});}
watch(custom, () => animateSettle(budgetMainEl.value, { y: 5, duration: 155 }));
let generation=0;
async function onPullRefresh(done?:()=>void){try{await refresh();}finally{done?.();}}
async function refresh(){const version=++generation;error.value='';try{await Promise.all([workspace.load(true),books.loadBooks()]);if(!bookId.value)bookId.value=books.defaultBookId;if(version!==generation)return;const budgets=workspace.budgets.filter(b=>b.bookId===bookId.value);const from=moment.min(moment.tz(`${month.value}-01`,scope.timeZone),...budgets.map(b=>moment.tz(b.startDate,scope.timeZone))),to=moment.max(moment.tz(`${month.value}-01`,scope.timeZone).endOf('month'),...budgets.map(b=>moment.tz(b.endDate,scope.timeZone).endOf('day')));await load(from.unix(),to.unix());if(version===generation)error.value=ledgerError.value;}catch(cause){if(version===generation)error.value=investmentError(cause,'read');}}
watch([month,bookId],refresh);
</script>
<style scoped src="@/styles/mobile/statistics.css"></style>
<style scoped>
.budget-mode-switch{margin-bottom:14px}.budget-mode-switch button{flex:1}.budget-scope{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:16px}.budget-scope>select{max-width:45%;font-size:13px}.budget-month{display:flex;align-items:center;gap:4px}.budget-month input{width:125px;min-width:0;border:0;background:none;font-size:14px}.budget-month button{border:0;background:none;font-size:23px;padding:0 9px;color:var(--cy-muted)}.budget-overview{text-align:center}.budget-ring{--cy-ring-progress:0;--cy-ring-color:var(--cy-accent);background:conic-gradient(var(--cy-ring-color) calc(var(--cy-ring-progress) * 1%),var(--cy-soft) 0);transition:--cy-ring-progress var(--cy-dur-sheet,280ms) var(--cy-ease-out,ease-out),transform var(--cy-dur-fast,150ms) var(--cy-ease-out,ease);display:grid;place-items:center;width:190px!important;height:190px;border-radius:50%;border:0;margin:15px auto 25px;padding:8px}.budget-ring:active{transform:scale(0.975)}.budget-ring>span{display:flex;flex-direction:column;align-items:center;justify-content:center;width:100%;height:100%;background:var(--cy-card);border-radius:50%;gap:8px}.budget-ring strong{font-size:25px;max-width:150px;overflow-wrap:anywhere}.budget-ring small{font-size:12px;color:var(--cy-muted)}.budget-stats{display:grid;grid-template-columns:repeat(4,1fr);gap:7px;margin:15px 0}.budget-stats span,.budget-detail-values span{display:block;font-size:11px;color:var(--cy-muted)}.budget-stats strong{display:block;font-size:13px;overflow-wrap:anywhere;margin-top:7px}.budget-stats>a{display:block;color:inherit}.budget-list-row{width:100%;border:0;background:none;text-align:left;padding:17px 0}.budget-list-row>div{display:flex;justify-content:space-between;gap:8px}.budget-list-row>div:first-child{font-size:14px}.budget-list-row small{font-size:11px;color:var(--cy-muted);font-weight:400}.budget-list-row>div:last-child{margin-top:8px;font-size:10px}.budget-list-row .stat-progress{display:block}.budget-form{display:grid;gap:18px}.budget-form label{display:grid;gap:9px;font-size:13px;color:var(--cy-muted)}.budget-form input:not([type=checkbox]),.budget-form select{width:100%;max-width:none;border:1px solid var(--cy-line);padding:12px;background:var(--cy-bg);border-radius:7px;color:var(--cy-ink);font-size:15px}.budget-toggle{display:flex!important;justify-content:space-between;align-items:center;padding:16px 0;gap:16px;border-bottom:1px solid var(--cy-line);font-size:14px}.budget-toggle span small{display:block;font-size:12px;color:var(--cy-muted);margin-top:3px}.budget-toggle input{accent-color:var(--cy-accent);width:19px;height:19px;flex:none}.budget-detail-values{display:grid;grid-template-columns:repeat(3,1fr);gap:10px;text-align:center;margin-top:24px}.budget-detail-values strong{display:block;margin-top:9px;font-size:17px;overflow-wrap:anywhere}.budget-help p{font-size:14px;line-height:1.9;margin-bottom:20px;color:var(--cy-muted)}
</style>
