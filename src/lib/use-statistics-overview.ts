import { computed, onScopeDispose, ref, watch } from 'vue';
import moment from 'moment-timezone';
import { useMobileLedger, ledgerMoney, ledgerTotals, LedgerDecimal, type LedgerEntry } from '@/lib/mobile-ledger.ts';
import { useBooksStore } from '@/stores/books.ts';
import { useAccountsStore } from '@/stores/account.ts';
import { useTransactionCategoriesStore } from '@/stores/transactionCategory.ts';
import { useLedgerScopeStore } from '@/stores/ledgerScope.ts';
import { useLedgerExperienceStore } from '@/stores/ledgerExperience.ts';
import { useTransactionTagsStore } from '@/stores/transactionTag.ts';
import { useStatisticsWorkspaceStore } from '@/stores/statisticsWorkspace.ts';
import { investments, investmentError } from '@/lib/investments.ts';
import { assetCny } from '@/lib/asset-visibility.ts';
import { isRevisionConflict } from '@/lib/error-message.ts';
import { ledgerMonthRange, ledgerAccountingMonth } from '@/lib/ledger-preferences.ts';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import type { WealthSnapshot, WealthSummary } from '@/models/investment.ts';
import { reimbursements, type ReimbursementClaim } from '@/lib/reimbursements.ts';
import { statisticsModuleNames, type StatisticsPeriod, type StatisticsModule } from '@/lib/statistics-workspace.ts';
import { withinRange, groupReport, reportBreakdown, reportBuckets, elapsedDays, comparisonValue, budgetReports, statisticsEligible, type StatisticsMetric, type TagStatisticsMode, type ReportGroup, type BudgetReport } from '@/lib/statistics-report.ts';
import { cashChart, shareChart, flowChart, chartColors, metricLabels } from '@/lib/statistics-charts.ts';
import { statisticsLink } from '@/lib/statistics-links.ts';

export function useStatisticsOverview() {
 const books = useBooksStore(), accounts = useAccountsStore(), categories = useTransactionCategoriesStore(), scope = useLedgerScopeStore(), experience = useLedgerExperienceStore(), tags = useTransactionTagsStore(), workspace = useStatisticsWorkspaceStore();
 const { showToast } = useI18nUIComponents();
 const { entries, load, loading, error } = useMobileLedger();
 // Budgets always use the complete selected books, independently of temporary report filters.
 const budgetLedger = useMobileLedger({ applyFilters: () => false });
 const now = () => moment().tz(scope.timeZone);
 const period = ref<StatisticsPeriod>('month'), month = ref(ledgerAccountingMonth(now(), experience.preferences.monthStart)), year = ref(now().format('YYYY'));
 const start = ref(now().startOf('month').format('YYYY-MM-DD')), end = ref(now().format('YYYY-MM-DD')), dailyMode = ref('seven'), dailyTagRange = ref('month');
 const periods: {id:StatisticsPeriod;name:string}[] = [{id:'daily',name:'日常'},{id:'month',name:'月统计'},{id:'year',name:'年统计'},{id:'custom',name:'自定义'}];
 const metricKeys: StatisticsMetric[] = ['expense','income','balance'];
 const metric = ref<StatisticsMetric>('expense'), chartKind = ref<'bar'|'line'>('bar'), primary = ref(true), flowPrimary = ref(true), summaryExpanded = ref(false), rankDetailed = ref(false), rankMetric = ref<StatisticsMetric>('expense');
 const shareMetrics = ref<Record<string, StatisticsMetric>>({categories:'expense',accounts:'expense',tagShare:'expense'}), showAmounts = ref<Record<string,boolean>>({});
 const tagMode = ref<TagStatisticsMode>('all'), tagSort = ref('count'), heatMetric = ref<'expense'|'income'>('expense'), activeHeatMonth = ref(0);
 const tagModes: {id:TagStatisticsMode;label:string}[] = [{id:'all',label:'所有标签'},{id:'parents',label:'一级标签（含子级）'},{id:'direct',label:'一级标签（不含子级）'},{id:'children',label:'二级标签'}];
 const hasSubTags = computed(() => Object.values(tags.allTransactionTagsMap).some(t => t.parentId && t.parentId !== '0'));
 const sheet = ref(''), saving = ref(false), actionError = ref(''), feedback = ref(''), auxiliaryError = ref('');
 const conflictDetected = ref(false), conflictServerNote = ref(''), yearDraft = ref(year.value === 'all' ? now().format('YYYY') : year.value), yearDraftError = ref('');
 const initialNoteSnapshot = ref(''), initialModulesSnapshot = ref('');
 let filterSnapshot: { bookIds: string[]; accountId: string; categoryId: string; tagId: string; type: number } | null = null;
 const sheetTitle = computed(() => ({menu:'更多操作',filters:'账本与筛选',date:'日期设置',modules:'模块设置',chart:'收支统计',wealth:'资产走势',note:'填写总结',analysis:'消费分析',heatmapMonth:`${year.value}年${activeHeatMonth.value}月收支热力`}[sheet.value] || '统计'));
 watch(sheet, () => { actionError.value = ''; feedback.value = ''; conflictDetected.value = false; conflictServerNote.value = ''; yearDraftError.value = ''; });
 function openFilters() {
  filterSnapshot = { bookIds: [...books.selectedBookIds], accountId: scope.accountId, categoryId: scope.categoryId, tagId: scope.tagId, type: scope.type };
  sheet.value = 'filters';
 }
 function applyFilters() {
  filterSnapshot = null;
  sheet.value = '';
  void refresh();
 }
 function closeSheet() {
  if (sheet.value === 'filters' && filterSnapshot) {
   books.selectedBookIds = [...filterSnapshot.bookIds];
   scope.accountId = filterSnapshot.accountId;
   scope.categoryId = filterSnapshot.categoryId;
   scope.tagId = filterSnapshot.tagId;
   scope.type = filterSnapshot.type;
   filterSnapshot = null;
  }
  sheet.value = '';
 }
 function openHeatMonth(m: number) { activeHeatMonth.value = m; sheet.value = 'heatmapMonth'; }
 const activeFilterChips = computed(() => {
  const chips: { key: string; label: string; clear: () => void }[] = [];
  if (scope.accountId) {
   const name = accounts.allAccountsMap[scope.accountId]?.name || '已选账户';
   chips.push({ key: 'account', label: `账户: ${name}`, clear: () => { scope.accountId = ''; } });
  }
  if (scope.categoryId) {
   const name = categories.allTransactionCategoriesMap[scope.categoryId]?.name || '已选分类';
   chips.push({ key: 'category', label: `分类: ${name}`, clear: () => { scope.categoryId = ''; } });
  }
  if (scope.tagId) {
   const name = tags.allTransactionTagsMap[scope.tagId]?.name || '已选标签';
   chips.push({ key: 'tag', label: `标签: ${name}`, clear: () => { scope.tagId = ''; } });
  }
  if (scope.type) {
   const typeNames: Record<number, string> = { 3: '支出', 2: '收入', 4: '转账' };
   chips.push({ key: 'type', label: `类型: ${typeNames[scope.type] || '已选'}`, clear: () => { scope.type = 0; } });
  }
  return chips;
 });
 const moduleDraft = ref<StatisticsModule[]>([]), dragIndex = ref(-1), noteDraft = ref('');
 const sheetDirty = computed(() => {
  if (sheet.value === 'note') return noteDraft.value !== initialNoteSnapshot.value;
  if (sheet.value === 'modules') return JSON.stringify(moduleDraft.value) !== initialModulesSnapshot.value;
  return false;
 });
 const visibleModules = computed(() => workspace.preferences.modules[period.value].filter(item => item.visible));
 const monthChoices = computed(() => {
  const curMonth = ledgerAccountingMonth(now(), experience.preferences.monthStart);
  const selectedYear = month.value.slice(0, 4);
  const thisYear = curMonth.slice(0, 4);
  const list: { value: string; label: string }[] = [];
  if (selectedYear === thisYear) {
   const curNum = Number(curMonth.slice(5, 7));
   const selNum = Number(month.value.slice(5, 7));
   const topNum = Math.max(curNum, selNum);
   for (let m = topNum; m >= 1; m--) {
    const val = `${thisYear}-${String(m).padStart(2, '0')}`;
    list.push({ value: val, label: val === curMonth ? '本月' : m === curNum - 1 ? '上月' : `${m}月` });
   }
  } else {
   for (let m = 12; m >= 1; m--) {
    const val = `${selectedYear}-${String(m).padStart(2, '0')}`;
    list.push({ value: val, label: `${m}月` });
   }
  }
  return list;
 });
 const yearChoices = computed(() => Array.from({length:6},(_,i) => ({value:now().subtract(i,'years').format('YYYY'),label:i===0?'今年':i===1?'去年':now().subtract(i,'years').format('YYYY年')})));
 const range = computed<[moment.Moment,moment.Moment]>(() => {
  if (period.value === 'daily') return [dailyMode.value === 'week' ? now().startOf('isoWeek') : now().subtract(6,'days').startOf('day'),now().endOf('day')];
  if (period.value === 'month') {
   const valid = moment.tz(month.value,'YYYY-MM',true,scope.timeZone);
   if (!valid.isValid()) return [valid, valid];
   const span = ledgerMonthRange(month.value, scope.timeZone, experience.preferences.monthStart);
   return [span.start, span.end];
  }
  if (period.value === 'year') {const value=moment.tz(year.value === 'all' ? '1970' : year.value,'YYYY',true,scope.timeZone);return [value.clone().startOf('year'),year.value === 'all' ? now().endOf('year') : value.clone().endOf('year')];}
  return [moment.tz(start.value,'YYYY-MM-DD',true,scope.timeZone).startOf('day'),moment.tz(end.value,'YYYY-MM-DD',true,scope.timeZone).endOf('day')];
 });
 const monthSpanCaption = computed(() => period.value === 'month' && experience.preferences.monthStart > 1 && range.value.every(v => v.isValid()) ? `账期 ${range.value[0].format('M月D日')} — ${range.value[1].format('M月D日')}` : '');
 const rangeError = computed(() => !range.value.every(value => value.isValid()) || range.value[1].isBefore(range.value[0]) || range.value[0].year()<1970 || range.value[1].year()>2200 ? '请选择有效的起止日期（1970—2200年）。' : '');
 const periodEntries = computed(() => withinRange(entries.value,range.value[0].unix(),range.value[1].unix()));
 const totals = computed(() => ledgerTotals(periodEntries.value.filter(statisticsEligible)));
 const dayCount = computed(() => elapsedDays(range.value[0],range.value[1],now()));
 const periodLabel = computed(() => ({month:'月',year:'年',custom:'',daily:''}[period.value]));
 const filterActive = computed(() => books.selectedBookIds.length || scope.accountId || scope.categoryId || scope.tagId || scope.type);
 const selectedKey = ref('');
 const chartBuckets = computed(() => reportBuckets(periodEntries.value,range.value[0],range.value[1],period.value === 'year' ? year.value === 'all' ? 'year' : 'month' : range.value[1].diff(range.value[0],'days')>366 ? 'month' : 'day'));
 const selectedBucket = computed(() => chartBuckets.value.find(row => row.key === selectedKey.value));
 const cashOption = computed(() => cashChart(chartBuckets.value, period.value === 'daily' || period.value === 'custom' ? ['expense','income'] : [metric.value],chartKind.value,period.value === 'daily'));
 function selectBucket(value: {dataIndex:number}) { selectedKey.value = chartBuckets.value[value.dataIndex]?.key || ''; }
 function money(value: string, complete=true) { return complete ? ledgerMoney(value,false) : '—'; }
 function detail(extra:Record<string,string>={},from=range.value[0].unix(),to=range.value[1].unix()) { return statisticsLink(from,to,{bookIds:books.selectedBookIds.join(','),filterAccountId:scope.accountId,filterCategoryId:scope.categoryId,filterTagId:scope.tagId,filterType:scope.type?String(scope.type):'',...extra}); }
 function bookName(id:string) { return books.allBooks.find(book => book.id===id)?.name || '账本'; }
 const categoryGroups = computed(() => groupReport(periodEntries.value,'category',{primary:primary.value}));
 const accountGroups = computed(() => groupReport(periodEntries.value,'account'));
 const shareTagGroups = computed(() => groupReport(periodEntries.value,'tag',{tags:tags.allTransactionTagsMap,tagMode:tagMode.value}));
 const flowGroups = computed(() => groupReport(periodEntries.value,'category',{primary:flowPrimary.value}));
 const flowOption = computed(() => flowChart(flowGroups.value));
 const flowUnavailable = computed(() => flowGroups.value.some(g => !g.complete || new LedgerDecimal(g.income).lt(0) || new LedgerDecimal(g.expense).lt(0)));
 function pieGroups(id:string) { return id==='categories'?categoryGroups.value:id==='accounts'?accountGroups.value:shareTagGroups.value; }
 function pieBreakdown(id:string) { return reportBreakdown(pieGroups(id).filter(item => !item.complete || !new LedgerDecimal(item[shareMetrics.value[id]!]).isZero()),shareMetrics.value[id]!); }
 function pieOption(id:string) { return shareChart(pieGroups(id),shareMetrics.value[id]!,!!showAmounts.value[id]); }
 function groupLink(id:string,item:ReportGroup) { return detail({title:item.name,type:shareMetrics.value[id]==='expense'?'3':'2',...(id==='categories'?{categoryId:item.id,primary:String(primary.value)}:id==='accounts'?{accountId:item.id}:{tagId:item.id,tagMode:tagMode.value})}); }
 const ranking = computed(() => reportBreakdown(groupReport(periodEntries.value,'category',{primary:primary.value,type:rankMetric.value==='expense'?3:2}),rankMetric.value));
 function comparisonText(id:string,samePeriod:boolean) {
  const unit=period.value==='year'?'year':'month'; const from=range.value[0].clone().subtract(1,unit); const to=(samePeriod&&now().isBetween(range.value[0],range.value[1],undefined,'[]')?now():range.value[1]).clone().subtract(1,unit);
  const before=groupReport(withinRange(entries.value,from.unix(),to.unix()),'category',{primary:primary.value,type:rankMetric.value==='expense'?3:2}).find(g=>g.id===id);
  const currentEntries=samePeriod&&now().isBetween(range.value[0],range.value[1],undefined,'[]')?withinRange(periodEntries.value,range.value[0].unix(),now().unix()):periodEntries.value;
  const current=groupReport(currentEntries,'category',{primary:primary.value,type:rankMetric.value==='expense'?3:2}).find(g=>g.id===id); const result=comparisonValue(current?.[rankMetric.value]||'0',before?.[rankMetric.value]||'0',(current?.complete??true)&&(before?.complete??true));
  return result.percent===null?'—':`${new LedgerDecimal(result.percent).gt(0)?'+':''}${result.percent}%`;
 }
 const reportUnit=ref<'day'|'month'>('day'),reportSort=ref('key'),reportDescending=ref(true),allReport=ref(false);
 const reportColumns=[{id:'key',label:'日期'},{id:'income',label:'收入'},{id:'expense',label:'支出'},{id:'balance',label:'结余'}];
 const reportRows=computed(()=>reportBuckets(periodEntries.value,range.value[0],range.value[1],reportUnit.value).filter(row=>row.count>0).sort((a,b)=>(reportSort.value==='key'?a.key.localeCompare(b.key):new LedgerDecimal(a[reportSort.value as StatisticsMetric]).comparedTo(b[reportSort.value as StatisticsMetric]))*(reportDescending.value?-1:1)));
 const shownReport=computed(()=>allReport.value?reportRows.value:reportRows.value.slice(0,12));
 function sortReport(id:string){if(reportSort.value===id)reportDescending.value=!reportDescending.value;else{reportSort.value=id;reportDescending.value=true;}}
 const tagRange=computed<[moment.Moment,moment.Moment]>(()=>period.value!=='daily'?range.value:[dailyTagRange.value==='all'?moment.tz('1970-01-01',scope.timeZone):dailyTagRange.value==='year'?now().startOf('year'):dailyTagRange.value==='week'?now().subtract(6,'days').startOf('day'):now().subtract(1,'month').startOf('day'),now().endOf('day')]);
 const tagRows=computed(()=>groupReport(withinRange(entries.value,tagRange.value[0].unix(),tagRange.value[1].unix()),'tag',{tags:tags.allTransactionTagsMap,tagMode:tagMode.value}).sort((a,b)=>tagSort.value==='name'?a.name.localeCompare(b.name):tagSort.value==='count'?b.count-a.count:new LedgerDecimal(b[tagSort.value as StatisticsMetric]).comparedTo(a[tagSort.value as StatisticsMetric])));
 const heatmaps=computed(()=>Array.from({length:12},(_,index)=>{const from=moment.tz(`${year.value}-${String(index+1).padStart(2,'0')}-01`,scope.timeZone),days=reportBuckets(periodEntries.value,from,from.clone().endOf('month'));const max=LedgerDecimal.max(1,...days.map(day=>new LedgerDecimal(day[heatMetric.value]).abs()));return {month:index+1,offset:(from.day()+6)%7,days:days.map(day=>({...day,day:Number(day.key.slice(-2)),color:!day.complete?'var(--cy-muted)':new LedgerDecimal(day[heatMetric.value]).isZero()?'var(--cy-soft)':`${heatMetric.value==='expense'?'#ff626a':'#48b69d'}${Math.round(45+new LedgerDecimal(day[heatMetric.value]).abs().div(max).mul(200).toNumber()).toString(16).padStart(2,'0')}`}))};}));
 const history=ref<WealthSnapshot[]>([]),wealth=ref<WealthSummary>(),wealthError=ref(''),wealthMetric=ref<'net'|'valued'>('net'),claims=ref<ReimbursementClaim[]>([]);
 const historyRows=computed(()=>history.value.filter(row=>row.recordedAt>=range.value[0].unix()&&row.recordedAt<=range.value[1].unix()).sort((a,b)=>a.recordedAt-b.recordedAt));
 const dailyBudgets=computed(()=>budgetReports(workspace.budgets,budgetLedger.entries.value,now().format('YYYY-MM'),workspace.preferences).filter(row=>row.budget.kind==='monthly'&&row.budget.categoryId==='0'&&(!books.selectedBookIds.length||books.selectedBookIds.includes(row.budget.bookId))));
 function isBudgetOver(row: BudgetReport): boolean { return row.remaining !== null && new LedgerDecimal(row.remaining).lt(0); }
 function budgetWidth(row:BudgetReport){if(isBudgetOver(row))return 100;if(row.available===null||new LedgerDecimal(row.available).lte(0))return 0;return LedgerDecimal.max(0,LedgerDecimal.min(100,new LedgerDecimal(workspace.preferences.budgetProgress==='remaining'?row.remaining||'0':row.spent).div(row.available).mul(100))).toNumber();}
 const auxiliaryRows=computed(()=>{
  const list=periodEntries.value, result:{label:string;value:string;complete:boolean;href:string}[]=[];
  function add(label:string,items:LedgerEntry[],extra:Record<string,string>={},discount=false){result.push({label,value:items.reduce((sum,item)=>sum.plus(discount?item.discountAmount||'0':item.cny||'0'),new LedgerDecimal(0)).abs().toString(),complete:items.every(item=>item.cny!==null),href:detail({title:label,...extra})});}
  add('转账',list.filter(item=>item.type===4&&!item.investment),{aux:'transfer'});
  for(const [key,label] of Object.entries({repay:'还款',collect:'收款',borrow:'借入',lend:'借出'}))add(label,list.filter(item=>workspace.auxiliary.debtActions[item.id]===key),{aux:'debt',debtAction:key});
  add('手续费',list.filter(item=>workspace.auxiliary.feeIds.includes(item.id)),{aux:'fees'});
  const relevant=claims.value.filter(claim=>(!books.selectedBookIds.length||books.selectedBookIds.includes(claim.bookId))&&(!scope.categoryId||scope.categoryId===claim.categoryId)&&(!scope.accountId||scope.accountId===claim.sourceAccountId));
  const inPeriod=relevant.filter(claim=>claim.time>=range.value[0].unix()&&claim.time<=range.value[1].unix());
  for(const [field,label] of [['amount','报销'],['pending','待报销']] as const)result.push({label,value:inPeriod.reduce((sum,c)=>sum.plus(c.currency==='CNY'?c[field]:'0'),new LedgerDecimal(0)).toString(),complete:inPeriod.every(c=>c.currency==='CNY')&&!scope.tagId,href:'/assets/reimbursements'});
  result.push({label:'报销到账',value:relevant.reduce((sum,c)=>sum.plus(c.receipts.filter(r=>r.time>=range.value[0].unix()&&r.time<=range.value[1].unix()).reduce((s,r)=>s.plus(c.currency==='CNY'?r.amount:'0'),new LedgerDecimal(0))),new LedgerDecimal(0)).toString(),complete:relevant.every(c=>c.currency==='CNY')&&!scope.tagId,href:'/assets/reimbursements'});
  add('退款',list.filter(item=>statisticsEligible(item)&&new LedgerDecimal(item.amount).lt(0)),{aux:'refund'});
  add('支出退款',list.filter(item=>statisticsEligible(item)&&item.type===3&&new LedgerDecimal(item.amount).lt(0)),{aux:'refund',type:'3'});
  add('收入退回',list.filter(item=>statisticsEligible(item)&&item.type===2&&new LedgerDecimal(item.amount).lt(0)),{aux:'refund',type:'2'});
  add('支出优惠',list.filter(item=>statisticsEligible(item)&&item.type===3),{aux:'discount',type:'3'},true);
  add('收入优惠',list.filter(item=>statisticsEligible(item)&&item.type===2),{aux:'discount',type:'2'},true);
  return result.filter(item => !item.complete || !new LedgerDecimal(item.value).isZero());
 });
 const activeHeatGrid=computed(()=>heatmaps.value.find(g=>g.month===activeHeatMonth.value));
 const notePeriod=computed(()=>period.value==='year'?year.value:month.value),noteBook=computed(()=>books.selectedBookIds[0]||'');
 const currentNote=computed(()=>workspace.notes.find(note=>note.bookId===noteBook.value&&note.period===notePeriod.value));
 function editNote(){if(books.selectedBookIds.length>1)return;noteDraft.value=currentNote.value?.content||'';initialNoteSnapshot.value=noteDraft.value;sheet.value='note';}
 async function syncNoteConflict(){
  saving.value=true;actionError.value='';conflictDetected.value=false;
  try{
   await workspace.load(true);
   conflictServerNote.value=currentNote.value?.content||'';
   showToast('已同步最新版本，你的草稿已保留');
  }catch(cause){actionError.value=investmentError(cause);}finally{saving.value=false;}
 }
 async function saveNote(){
  saving.value=true;actionError.value='';conflictDetected.value=false;
  try{
   await workspace.saveNote({id:currentNote.value?.id||'',bookId:noteBook.value,period:notePeriod.value,content:noteDraft.value,revision:currentNote.value?.revision||'0'});
   initialNoteSnapshot.value=noteDraft.value;
   sheet.value='';
   showToast('总结已保存');
  }catch(cause){
   conflictDetected.value=isRevisionConflict(cause);
   actionError.value=conflictDetected.value?'这份总结刚在别处更新，你的草稿已保留。点击「同步最新」查看对照后再保存。':investmentError(cause);
  }finally{saving.value=false;}
 }
 function openModules(){moduleDraft.value=workspace.preferences.modules[period.value].map(item=>({...item}));initialModulesSnapshot.value=JSON.stringify(moduleDraft.value);sheet.value='modules';}
 function moveModule(from:number,to:number){if(from<0||to<0||to>=moduleDraft.value.length)return;const [item]=moduleDraft.value.splice(from,1);moduleDraft.value.splice(to,0,item!);}
 async function syncModulesConflict(){
  saving.value=true;actionError.value='';conflictDetected.value=false;
  try{await workspace.load(true);showToast('已同步最新设置版本，请重新保存');}
  catch(cause){actionError.value=investmentError(cause);}finally{saving.value=false;}
 }
 async function saveModules(){
  saving.value=true;actionError.value='';conflictDetected.value=false;
  try{
   await workspace.savePreferences({...workspace.preferences,modules:{...workspace.preferences.modules,[period.value]:moduleDraft.value}});
   initialModulesSnapshot.value=JSON.stringify(moduleDraft.value);
   sheet.value='';
   showToast('模块设置已保存');
  }catch(cause){
   conflictDetected.value=isRevisionConflict(cause);
   actionError.value=conflictDetected.value?'模块设置刚被更新，点击「同步最新」后即可保存。':investmentError(cause);
  }finally{saving.value=false;}
 }
 function openYearPicker(){yearDraft.value=year.value==='all'?now().format('YYYY'):year.value;yearDraftError.value='';sheet.value='date';}
 function applyYearDraft(){
  const n=Number(yearDraft.value.trim());
  if(!/^\d{4}$/.test(yearDraft.value.trim())||n<1970||n>2200){yearDraftError.value='请输入 1970 至 2200 之间的四位年份';return;}
  year.value=String(n);yearDraftError.value='';sheet.value='';
 }
 function setRange(label:string){let from=now(),to=now();if(label==='本周'){from=from.startOf('isoWeek');to=to.endOf('isoWeek');}else if(label==='本月'){from=from.startOf('month');to=to.endOf('month');}else if(label==='上月'){from=from.subtract(1,'month').startOf('month');to=to.subtract(1,'month').endOf('month');}else{if(label==='去年'){from.subtract(1,'year');to.subtract(1,'year');}from.startOf('year');to.endOf('year');}start.value=from.format('YYYY-MM-DD');end.value=to.format('YYYY-MM-DD');}
 const analysisText=computed(()=>`${range.value[0].format('YYYY-MM-DD')} 至 ${range.value[1].format('YYYY-MM-DD')}\n${books.scopeName}\n\n支出：${money(totals.value.expense,totals.value.complete)} 元\n收入：${money(totals.value.income,totals.value.complete)} 元\n结余：${money(totals.value.balance,totals.value.complete)} 元\n日均支出：${money(new LedgerDecimal(totals.value.expense).div(dayCount.value).toString(),totals.value.complete)} 元\n\n支出分类\n${reportBreakdown(categoryGroups.value,'expense').items.map(g=>`${g.name}：${money(g.amount,g.complete)} 元（${g.count} 笔）`).join('\n')}\n\n${currentNote.value?.content||''}`);
 async function copyText(text:string,toastLabel='已复制'){try{await navigator.clipboard.writeText(text);showToast(toastLabel);}catch{showToast('复制失败，请长按文本复制');}}
 function copyNote(){void copyText(currentNote.value?.content||'','已复制总结');}
 function exportAnalysis(){const url=URL.createObjectURL(new Blob([analysisText.value],{type:'text/plain;charset=utf-8'}));const link=document.createElement('a');link.classList.add('external');link.href=url;link.download=`消费分析-${range.value[0].format('YYYYMMDD')}.txt`;link.hidden=true;document.body.appendChild(link);link.click();setTimeout(()=>{URL.revokeObjectURL(url);link.remove();},60000);showToast('已生成消费分析文本');}
 let refreshVersion=0;
 onScopeDispose(()=>{refreshVersion++;});
 async function refresh(){
  const version=++refreshVersion;if(rangeError.value){loading.value=false;return;}auxiliaryError.value='';
  try{await workspace.load(true);}catch(cause){if(version===refreshVersion)auxiliaryError.value=investmentError(cause,'read');}if(version!==refreshVersion)return;
  let earliest=range.value[0].clone().subtract(1,'year');if(period.value==='daily')earliest=moment.min(earliest,tagRange.value[0]);
  await Promise.all([load(earliest.unix(),range.value[1].unix()),
   period.value==='daily'?budgetLedger.load(moment.min(now().startOf('month'),...workspace.budgets.map(b=>moment.tz(b.startDate,scope.timeZone))).unix(),now().endOf('month').unix()):Promise.resolve(),
   (async()=>{try{const [h,w,c]=await Promise.all([investments.history(),investments.summary(),reimbursements.list()]);if(version===refreshVersion){history.value=h;wealth.value=w;claims.value=c;wealthError.value='';}}catch(cause){if(version===refreshVersion)wealthError.value=investmentError(cause,'read');}})()]);
 }
 watch([period,month,year,start,end,dailyMode,dailyTagRange,()=>scope.timeZone,()=>books.selectedBookIds.join(',')],()=>{selectedKey.value='';reportUnit.value=period.value==='year'?'month':'day';allReport.value=false;refresh();});
 return {books,scope,workspace,entries,loading,error,period,month,year,yearDraft,yearDraftError,start,end,dailyMode,dailyTagRange,periods,metricKeys,metric,chartKind,primary,flowPrimary,summaryExpanded,rankDetailed,rankMetric,shareMetrics,showAmounts,tagMode,tagSort,heatMetric,activeHeatMonth,activeHeatGrid,openHeatMonth,tagModes,hasSubTags,sheet,sheetDirty,saving,actionError,conflictDetected,conflictServerNote,feedback,auxiliaryError,sheetTitle,moduleDraft,dragIndex,noteDraft,visibleModules,monthChoices,yearChoices,monthSpanCaption,range,rangeError,totals,dayCount,periodLabel,filterActive,activeFilterChips,openFilters,applyFilters,closeSheet,selectedBucket,cashOption,selectBucket,money,detail,bookName,flowGroups,flowOption,flowUnavailable,pieGroups,pieBreakdown,pieOption,groupLink,ranking,comparisonText,reportUnit,reportSort,reportDescending,allReport,reportColumns,reportRows,shownReport,sortReport,tagRange,tagRows,heatmaps,wealth,wealthError,wealthMetric,historyRows,dailyBudgets,isBudgetOver,budgetWidth,auxiliaryRows,notePeriod,currentNote,editNote,saveNote,syncNoteConflict,openModules,moveModule,saveModules,syncModulesConflict,openYearPicker,applyYearDraft,setRange,analysisText,copyText,copyNote,exportAnalysis,refresh,statisticsModuleNames,chartColors,metricLabels,ledgerMoney,assetCny,LedgerDecimal};
}
