<template>
 <f7-page class="cy-mobile-surface cy-investment-page" @page:afterin="refresh">
  <f7-navbar title="理财定投" back-link="理财"><f7-nav-right><f7-link :disabled="busy" @click="newPlan">新增</f7-link></f7-nav-right></f7-navbar>
  <main class="inv-body">
   <p v-if="error" class="cy-message" role="alert">{{ error }}</p><p v-if="notice" class="cy-message" role="status">{{ notice }}</p>
   <section v-for="alert in fundAlerts" :key="alert.cashAccountId" class="inv-card inv-warning">
    <strong>{{ alert.insufficientNow ? '基金定投余额不足待处理' : '基金定投余额即将不足' }}</strong>
    <p>{{ alert.cashAccountName }} 当前 {{ assetCurrencyMoney(alert.balance,'CNY') }}，{{ alert.insufficientNow ? '待扣款/近期定投' : '未来 3 天定投' }}（{{ alert.planNames.join('、') || '基金定投' }}）需 {{ assetCurrencyMoney(alert.required,'CNY') }}。</p>
   </section>
   <section v-if="permission === 'denied'" class="inv-card">
    <div class="inv-event-line"><strong>后台消息提醒</strong><button type="button" class="inv-link" style="padding:0" @click="requestNotificationPermission">允许系统通知</button></div>
    <p class="inv-muted" style="margin-top:6px">开启系统通知后，余额不足可在后台主动提醒。</p>
   </section>
   <details v-else-if="permission === 'granted'" class="inv-card inv-notify-collapsed">
    <summary class="inv-event-line"><span>后台消息提醒</span><small class="inv-muted">已开启 ⌄</small></summary>
    <p class="inv-muted" style="margin-top:8px">余额不足时将通过系统通知提醒；若划掉应用后收不到，可允许本应用后台运行。</p>
    <div class="inv-toolbar">
     <button type="button" class="inv-link" @click="testDcaNotification">发送测试通知</button>
     <button type="button" class="inv-link" @click="openBackgroundSettings">后台运行设置</button>
    </div>
   </details>
   <nav class="inv-mode"><button :aria-pressed="tab==='plans'" @click="tab='plans'">定投计划</button><button :aria-pressed="tab==='orders'" @click="tab='orders'">待确认 {{ scopedOrders.filter(o=>o.status==='pending').length||'' }}</button></nav>
   <template v-if="tab==='plans'">
    <section v-for="p in scopedPlans" :key="p.id" class="inv-card">
     <div class="inv-event-line"><strong>{{ assetName(p.instrumentId) }}</strong><strong>{{ assetCurrencyMoney(p.amount,'CNY') }}</strong></div>
     <p class="inv-muted" style="margin-top:8px">{{ cycleNames[p.cycle] }} · {{ p.time }} · {{ cashAccountName(p.cashAccountId) }} · <span :class="p.paused?'inv-status-paused':'inv-status-active'">{{ p.paused?'已暂停':p.endDate&&p.nextDate>p.endDate?'已结束':'下次 '+p.nextDate }}</span></p>
     <div class="inv-toolbar"><button class="inv-link" @click="edit(p)">编辑</button><button class="inv-link" :disabled="busy" @click="changePlan(p,{paused:!p.paused})">{{ p.paused?'恢复':'暂停' }}</button><button class="inv-link cy-expense" :disabled="busy" @click="removePlan(p)">删除</button></div>
    </section>
    <section v-if="!loading&&!scopedPlans.length" class="inv-card cy-empty">
     <p>还没有定投计划</p>
     <f7-link v-if="!fundRows.length" class="inv-link" href="/investments/add?type=FUND">先去添加一只基金 ›</f7-link>
     <button v-else class="inv-link" @click="newPlan">新增定投</button>
    </section>
    <p class="inv-caption">按期使用基金已公布净值自动入账，份额保留两位小数。15:00后及非净值公布日顺延至下一公布日。应用重新打开时补齐到期记录；净值缺失或余额不足时保留待确认。</p>
   </template>
   <template v-else>
    <div class="inv-toolbar"><button class="inv-link" :disabled="busy" @click="sync(false)">{{ busy?'正在确认…':'刷新并自动确认' }}</button><button class="inv-link" @click="showHistory=!showHistory">{{ showHistory?'只看待确认':'查看全部记录' }}</button></div>
    <section v-for="o in shownOrders" :key="o.id" class="inv-card">
     <div class="inv-event-line"><strong>{{ assetName(o.instrumentId) }}</strong><strong>{{ o.amount?assetCurrencyMoney(o.amount,'CNY'):assetText(o.quantity)+' 份' }}</strong></div>
     <p class="inv-muted" style="margin-top:8px">{{ o.tradeDate }} · {{ o.type==='BUY'?'买入':'卖出' }} · {{ statuses[o.status] }}</p>
     <p v-if="o.error&&o.status==='pending'" class="inv-caption" style="padding:0;margin-bottom:0;color:var(--cy-expense)">{{ o.error }}</p>
     <p v-if="o.priceDate" class="inv-muted">{{ o.priceDate }} 净值 {{ o.price }}</p>
     <div v-if="o.status==='pending'" class="inv-toolbar inv-order-actions"><button class="inv-link" @click="openConfirm(o)">填写实际净值</button><button class="inv-link cy-expense" :disabled="busy" @click="cancel(o)">取消本期</button></div>
     <f7-link v-else-if="o.eventId" class="inv-link" :href="`/investments/record?action=revise&eventId=${o.eventId}&accountId=${o.accountId}&instrumentId=${encodeURIComponent(o.instrumentId)}`">查看入账记录</f7-link>
    </section>
    <p v-if="!loading&&!shownOrders.length" class="cy-empty">没有{{ showHistory?'':'待确认' }}记录</p>
    <p class="inv-caption">待确认记录尚未扣款或增加持仓。确认后将资金与份额一起入账；已取消的本期不会再次自动生成。</p>
   </template>
  </main>
  <f7-popup v-model:opened="showEditor" class="cy-mobile-surface cy-investment-page"><f7-page class="cy-mobile-surface cy-investment-page"><f7-navbar :title="draft.id?'编辑定投':'新增定投'"><f7-nav-left><f7-link @click="showEditor=false">取消</f7-link></f7-nav-left><f7-nav-right><f7-link :disabled="busy" @click="save">保存</f7-link></f7-nav-right></f7-navbar><main class="inv-body"><p v-if="formError" class="cy-message" role="alert">{{ formError }}</p><section v-if="!fundRows.length" class="inv-card cy-empty"><p>请先添加一只基金持仓，再来设置定投计划。</p><f7-link class="cy-button cy-primary" href="/investments/add?type=FUND" @click="showEditor=false">去添加基金</f7-link></section><template v-else><section class="inv-card inv-form">
   <label class="inv-row"><span>重复周期</span><select v-model="draft.cycle"><option v-for="(label,value) in cycleNames" :key="value" :value="value">{{ label }}</option></select></label>
   <label class="inv-row"><span>生效日期</span><input v-model="draft.startDate" type="date" required /></label><label class="inv-row"><span>结束日期</span><input v-model="draft.endDate" type="date" aria-label="结束日期，留空永不结束" /></label><label class="inv-row"><span>定投时间</span><input v-model="draft.time" type="time" required /></label>
  </section><section class="inv-card inv-form"><label class="inv-row"><span>买入基金</span><select v-model="selectedHolding" required><option value="" disabled>选择基金</option><option v-for="r in fundRows" :key="r.key" :value="r.key">{{ r.name }} · {{ r.account?.name }}</option></select></label><label class="inv-row"><span>付款账户</span><select v-model="draft.cashAccountId" required><option value="" disabled>选择账户</option><option v-for="a in cash" :key="a.id" :value="a.id">{{ a.name }}</option></select></label><label class="inv-row"><span>金额（元）</span><input v-model="draft.amount" inputmode="decimal" placeholder="每期支付总额" aria-label="每期金额" /></label><label class="inv-row"><span>手续费（%）</span><input v-model="draft.feePercent" inputmode="decimal" placeholder="默认0" aria-label="定投费率" /></label><label class="inv-row"><span>备注</span><input v-model="draft.note" maxlength="300" /></label><div v-if="books.activeBooks.length > 1" class="inv-row"><span>记入账本</span><BookPicker v-model="draft.bookId" compact /></div></section><p class="inv-caption">金额包含手续费。结束日期留空表示永不结束；每月按生效日期对应的日子执行，短月份使用月末。</p><button class="inv-save" :disabled="busy" @click="save">{{ busy?'正在保存…':'保存定投' }}</button></template></main></f7-page></f7-popup>
  <f7-sheet class="inv-sheet cy-mobile-surface cy-investment-page" :opened="!!confirming" @sheet:closed="confirming=undefined" swipe-to-close backdrop><h2>确认基金净值</h2><p v-if="formError" class="cy-message" role="alert">{{ formError }}</p><label class="inv-row"><span>确认日期</span><input v-model="confirmDate" type="date" /></label><label class="inv-row"><span>确认净值</span><input v-model="confirmPrice" inputmode="decimal" placeholder="例如 1.2345" aria-label="确认净值" /></label><p v-if="estimatedConfirmShares" class="inv-caption" style="color:var(--cy-accent)">按此净值预计确认约 {{ estimatedConfirmShares }} 份</p><p class="inv-caption">确认后将按该净值计算份额，并同步更新资金账户。</p><button class="inv-save" :disabled="busy" @click="confirm">确认入账</button></f7-sheet>
 </f7-page>
</template>
<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue';
import type {Router} from 'framework7/types';
import moment from 'moment-timezone';
import {useI18nUIComponents} from '@/lib/ui/mobile.ts';
import {investments,investmentError} from '@/lib/investments.ts';
import {fundDCAAlerts,useInvestmentData,invalidateInvestmentData,validInvestmentNumber,syncInvestmentPlansOnOpen} from '@/lib/investment-mobile.ts';
import {notificationStatus} from '@/lib/asset-tools.ts';
import '@/lib/native.ts';
import {LedgerDecimal} from '@/lib/ledger-display.ts';
import {assetCurrencyMoney,assetText} from '@/lib/asset-visibility.ts';
import BookPicker from '@/components/mobile/BookPicker.vue';
import type {InvestmentPlan,InvestmentOrder} from '@/models/investment.ts';
const props=defineProps<{f7route:Router.Route}>(),q=props.f7route.query;
const {showToast,showConfirm}=useI18nUIComponents();
const {wealth,rows,assets,orders,books,loading,error,zone,load}=useInvestmentData();
const tab=ref(q['tab']==='orders'?'orders':'plans'),plans=ref<InvestmentPlan[]>([]),busy=ref(false),showHistory=ref(false),showEditor=ref(false),notice=ref(''),formError=ref(''),selectedHolding=ref(''),confirming=ref<InvestmentOrder>(),confirmPrice=ref(''),confirmDate=ref('');
const permission=ref(notificationStatus());
const fundAlerts=computed(()=>fundDCAAlerts.value);
function refreshPermission():void{permission.value=notificationStatus();}
function requestNotificationPermission():void{window.CYLedgerReminders?.requestPermission();setTimeout(refreshPermission,800);}
function testDcaNotification():void{window.CYLedgerReminders?.testDca?.();showToast('已发送定投测试通知，可查看通知栏');}
function openBackgroundSettings():void{window.CYLedgerLocal?.action('permissions','{}');}
onMounted(()=>{refreshPermission();window.addEventListener('cyledger-notification-permission',refreshPermission);});
onUnmounted(()=>{window.removeEventListener('cyledger-notification-permission',refreshPermission);});
const cycleNames:Record<string,string>={daily:'每天',weekly:'每周',biweekly:'每两周',monthly:'每月'},statuses={pending:'待确认',completed:'已入账',cancelled:'已取消'};
function blank():InvestmentPlan{return {id:'',accountId:q['accountId']||'',instrumentId:q['instrumentId']||'',cashAccountId:'',bookId:books.defaultBookId,amount:'',feePercent:'0',cycle:'monthly',startDate:moment().tz(zone.value).format('YYYY-MM-DD'),endDate:'',nextDate:'',time:'09:00',timeZone:zone.value,note:'',paused:false,deleted:false,version:0};}
const draft=ref<InvestmentPlan>(blank());
const scopedPlans=computed(()=>plans.value.filter(p=>(!q['accountId']||p.accountId===q['accountId'])&&(!q['instrumentId']||p.instrumentId===q['instrumentId']))),scopedOrders=computed(()=>orders.value.filter(o=>(!q['accountId']||o.accountId===q['accountId'])&&(!q['instrumentId']||o.instrumentId===q['instrumentId']))),shownOrders=computed(()=>scopedOrders.value.filter(o=>showHistory.value||o.status==='pending'));
const fundRows=computed(()=>rows.value.filter(r=>r.asset?.type==='FUND')),cash=computed(()=>wealth.value?.cashAccounts.filter(a=>a.currency==='CNY'&&!a.liability)||[]);
function assetName(id:string):string{return assets.value.find(a=>a.id===id)?.name||id;}
function cashAccountName(id:string):string{return wealth.value?.cashAccounts.find(a=>a.id===id)?.name||'付款账户';}
const estimatedConfirmShares=computed(()=>{
 const o=confirming.value;
 if(!o||!o.amount||!confirmPrice.value.trim())return '';
 try{
  const price=new LedgerDecimal(confirmPrice.value.trim());
  if(!price.gt(0))return '';
  const gross=new LedgerDecimal(o.amount);
  const feePct=new LedgerDecimal(o.feePercent||'0');
  const net=feePct.gt(0)?gross.div(new LedgerDecimal(1).plus(feePct.div(100))):gross.minus(o.fee||'0');
  if(!net.gt(0))return '';
  return net.div(price).toDecimalPlaces(2,LedgerDecimal.ROUND_DOWN).toFixed(2);
 }catch{return '';}
});
async function refresh():Promise<void>{await load();try{plans.value=await investments.plans();}catch(e){error.value=investmentError(e);}}
function newPlan():void{draft.value=blank();selectedHolding.value=fundRows.value.find(r=>r.position.accountId===q['accountId']&&r.position.instrumentId===q['instrumentId'])?.key||fundRows.value[0]?.key||'';draft.value.cashAccountId=cash.value[0]?.id||'';formError.value='';showEditor.value=true;}
function edit(p:InvestmentPlan):void{draft.value={...p};selectedHolding.value=p.accountId+':'+p.instrumentId;formError.value='';showEditor.value=true;}
async function save():Promise<void>{if(busy.value)return;busy.value=true;formError.value='';try{const r=fundRows.value.find(r=>r.key===selectedHolding.value);if(!r)throw Error('请先添加一只基金持仓');validInvestmentNumber(draft.value.amount,'金额',true);validInvestmentNumber(draft.value.feePercent,'费率');await investments.savePlan({...draft.value,accountId:r.position.accountId,instrumentId:r.position.instrumentId,timeZone:zone.value});showEditor.value=false;showToast('定投计划已保存');await refresh();if(notificationStatus()==='denied')requestNotificationPermission();window.CYLedgerReminders?.scheduleDca?.(false);}catch(e){formError.value=investmentError(e);}finally{busy.value=false;}if(!showEditor.value)void sync(true);}
async function changePlan(p:InvestmentPlan,changes:Partial<InvestmentPlan>,doneText?:string):Promise<void>{if(busy.value)return;busy.value=true;try{await investments.savePlan({...p,...changes});showToast(doneText||(changes.paused?'已暂停定投':'已恢复定投'));await refresh();window.CYLedgerReminders?.scheduleDca?.(false);void syncInvestmentPlansOnOpen(true);}catch(e){error.value=investmentError(e);}finally{busy.value=false;}}
function removePlan(p:InvestmentPlan):void{showConfirm(`删除「${assetName(p.instrumentId)}」的定投计划？删除后停止自动定投，已入账记录完整保留。`,()=>{void changePlan(p,{deleted:true,paused:true},'定投计划已删除');},undefined,{title:'删除定投计划',confirmText:'删除',destructive:true,raw:true});}
async function sync(silent=false):Promise<void>{if(busy.value)return;busy.value=true;error.value='';try{const result=await investments.syncPlans(true);fundDCAAlerts.value=result.alerts||[];if(!silent){const text=result.created?`已自动入账 ${result.created} 笔，待确认 ${result.pending} 笔`:result.pending?`当前有 ${result.pending} 笔待确认，暂未获取到新净值或余额不足`:'已检查，暂无新的到期定投';showToast(text);}if(result.created)invalidateInvestmentData();await refresh();}catch(e){if(!silent)error.value=investmentError(e);}finally{busy.value=false;}}
function openConfirm(o:InvestmentOrder):void{confirming.value=o;confirmDate.value=o.confirmDate||o.tradeDate;confirmPrice.value='';formError.value='';}
async function confirm():Promise<void>{if(!confirming.value||busy.value)return;busy.value=true;try{validInvestmentNumber(confirmPrice.value,'净值',true);await investments.confirmOrder({id:confirming.value.id,version:confirming.value.version,price:confirmPrice.value,date:confirmDate.value});confirming.value=undefined;invalidateInvestmentData();showToast('已确认入账');await refresh();void syncInvestmentPlansOnOpen(true);}catch(e){formError.value=investmentError(e);}finally{busy.value=false;}}
function cancel(o:InvestmentOrder):void{if(busy.value)return;showConfirm(`取消 ${o.tradeDate} 这期「${assetName(o.instrumentId)}」？取消后本期不再自动扣款或生成买入，且不可恢复。`,async()=>{busy.value=true;try{await investments.cancelOrder({id:o.id,version:o.version});showToast('已取消本期定投');await refresh();void syncInvestmentPlansOnOpen(true);}catch(e){error.value=investmentError(e);}finally{busy.value=false;}},undefined,{title:'取消本期定投',confirmText:'确认取消本期',destructive:true,raw:true});}
</script>
<style scoped>
.inv-warning{background:#fff5df;color:#805816;font-size:13px;line-height:1.8}.inv-warning p{margin:6px 0 0}.inv-notify-collapsed summary{list-style:none;cursor:pointer}.inv-notify-collapsed summary::-webkit-details-marker{display:none}.inv-order-actions{justify-content:space-between}.inv-status-paused{color:var(--cy-expense)}.inv-status-active{color:var(--cy-accent)}
</style>
