<template>
  <f7-popup class="bill-date-sheet cy-mobile-surface" :opened="opened" backdrop close-by-backdrop-click close-on-escape
            role="dialog" aria-modal="true" aria-label="选择账单日期和时间"
            @popup:opened="scrollRollers" @popup:close="opened=false" @popup:closed="onClosed">
    <div class="bill-date-body">
      <div class="bill-date-title">
        <strong>{{ viewMonth }}月 <small>{{ viewYear }}</small></strong>
        <nav aria-label="切换月份">
          <button type="button" aria-label="上个月" :disabled="busy" @click="shiftMonth(-1)"><f7-icon f7="chevron_left" /></button>
          <button type="button" aria-label="下个月" :disabled="busy" @click="shiftMonth(1)"><f7-icon f7="chevron_right" /></button>
        </nav>
      </div>
      <div class="bill-date-week"><span v-for="w in ['日','一','二','三','四','五','六']" :key="w">{{ w }}</span></div>
      <div class="bill-date-grid">
        <div v-for="(cell,index) in cells" :key="index" class="bill-date-slot">
          <button v-if="cell" type="button" class="bill-date-cell" :disabled="busy"
                  :class="{selected:isSelected(cell),today:isToday(cell)}" :aria-pressed="isSelected(cell)"
                  :aria-current="isToday(cell)?'date':undefined" :aria-label="`${cell.year}年${cell.month}月${cell.day}日 ${subText(cell)}`"
                  @click="selected=cell">
            <b>{{ cell.day }}</b><small :class="{festival:isFestival(cell)}">{{ subText(cell) }}</small>
          </button>
        </div>
      </div>
      <button type="button" class="bill-date-time-row" :aria-expanded="timeExpanded" :disabled="busy" @click="toggleTime">
        <f7-icon f7="clock_fill" /><span>设置时间</span><small v-if="!timeExpanded">{{ hour }} : {{ minute }}</small><f7-icon f7="chevron_right" />
      </button>
      <div v-show="timeExpanded" class="bill-date-rollers" :class="{disabled:busy}">
        <div ref="hourCol" class="bill-date-roller" role="listbox" aria-label="小时" @scroll.passive="onRoll('hour')">
          <button v-for="h in hours" :key="h" type="button" role="option" :data-value="h" :aria-selected="h===hour" :disabled="busy"
                  :style="rollStyle(h,hour)" @click="pickTime('hour',h)">{{ h }}</button>
        </div>
        <span class="bill-date-colon" aria-hidden="true">:</span>
        <div ref="minuteCol" class="bill-date-roller" role="listbox" aria-label="分钟" @scroll.passive="onRoll('minute')">
          <button v-for="m in minutes" :key="m" type="button" role="option" :data-value="m" :aria-selected="m===minute" :disabled="busy"
                  :style="rollStyle(m,minute)" @click="pickTime('minute',m)">{{ m }}</button>
        </div>
      </div>
      <p v-if="error" class="bill-date-error" role="alert">{{ error }}</p>
    </div>
    <div class="bill-date-footer">
      <button type="button" @click="opened=false">取消</button>
      <button type="button" class="confirm" :disabled="busy" @click="confirm">{{ busy?'保存中':'确定' }}</button>
    </div>
  </f7-popup>
</template>

<script setup lang="ts">
import {computed,nextTick,ref,useTemplateRef,watch} from 'vue';
import moment from 'moment-timezone';
import {getChineseYearMonthDayInfo,getChineseCalendarAlternateDisplayDate} from '@/lib/calendar/chinese_calendar.ts';
import {DEFAULT_CONTENT} from '@/locales/calendar/chinese/index.ts';

type CalendarDay={year:number;month:number;day:number};
type TimeColumn='hour'|'minute';
const props=defineProps<{show:boolean;modelValue:number;timeZone:string;busy?:boolean;error?:string}>();
const emit=defineEmits<{'update:show':[value:boolean];confirm:[value:number]}>();
const hourCol=useTemplateRef<HTMLDivElement>('hourCol'),minuteCol=useTemplateRef<HTMLDivElement>('minuteCol');
const initial=moment.unix(props.modelValue).tz(props.timeZone);
const viewYear=ref(initial.year()),viewMonth=ref(initial.month()+1);
const selected=ref<CalendarDay>({year:initial.year(),month:initial.month()+1,day:initial.date()});
const hour=ref('00'),minute=ref('00'),timeExpanded=ref(true),opened=ref(false);
const today=ref<CalendarDay>({year:0,month:0,day:0});
const hours=Array.from({length:24},(_,i)=>String(i).padStart(2,'0'));
const minutes=Array.from({length:60},(_,i)=>String(i).padStart(2,'0'));
const LUNAR_FESTIVALS:Record<string,string>={'1-1':'春节','1-15':'元宵','5-5':'端午','7-7':'七夕','8-15':'中秋','9-9':'重阳','12-8':'腊八'};
const GREGORIAN_FESTIVALS:Record<string,string>={'1-1':'元旦','5-1':'劳动节','10-1':'国庆节','10-31':'万圣节','12-25':'圣诞节'};
const cells=computed(()=>{
  const first=new Date(viewYear.value,viewMonth.value-1,1).getDay();
  const count=new Date(viewYear.value,viewMonth.value,0).getDate();
  const list:(CalendarDay|null)[]=Array.from({length:first},()=>null);
  for(let day=1;day<=count;day++)list.push({year:viewYear.value,month:viewMonth.value,day});
  return list;
});
function infoOf(cell:CalendarDay){return getChineseYearMonthDayInfo(cell,DEFAULT_CONTENT);}
function subText(cell:CalendarDay):string{
  const festival=GREGORIAN_FESTIVALS[`${cell.month}-${cell.day}`];
  if(festival)return festival;
  const info=infoOf(cell);
  if(!info)return '';
  if(!info.isLeapMonth&&LUNAR_FESTIVALS[`${info.month}-${info.day}`])return LUNAR_FESTIVALS[`${info.month}-${info.day}`]!;
  return getChineseCalendarAlternateDisplayDate(info).displayDate;
}
function isFestival(cell:CalendarDay):boolean{
  const info=infoOf(cell);
  return !!GREGORIAN_FESTIVALS[`${cell.month}-${cell.day}`]||!!info&&(!!info.solarTermName||!info.isLeapMonth&&!!LUNAR_FESTIVALS[`${info.month}-${info.day}`]);
}
function sameDay(a:CalendarDay,b:CalendarDay):boolean{return a.year===b.year&&a.month===b.month&&a.day===b.day;}
function isSelected(cell:CalendarDay):boolean{return sameDay(cell,selected.value);}
function isToday(cell:CalendarDay):boolean{return sameDay(cell,today.value);}
function shiftMonth(delta:number):void{
  const date=new Date(viewYear.value,viewMonth.value-1+delta,1);
  viewYear.value=date.getFullYear();viewMonth.value=date.getMonth()+1;
}
function syncFromModel():void{
  const date=moment.unix(props.modelValue).tz(props.timeZone),now=moment().tz(props.timeZone);
  selected.value={year:date.year(),month:date.month()+1,day:date.date()};
  today.value={year:now.year(),month:now.month()+1,day:now.date()};
  viewYear.value=date.year();viewMonth.value=date.month()+1;
  hour.value=date.format('HH');minute.value=date.format('mm');timeExpanded.value=true;
}
function column(which:TimeColumn){return which==='hour'?hourCol.value:minuteCol.value;}
function rowHeight(col:HTMLDivElement):number{return (col.querySelector('button') as HTMLElement|null)?.offsetHeight||28;}
function scrollRollers():void{
  for(const [col,value] of [[hourCol.value,hour.value],[minuteCol.value,minute.value]] as const){
    if(col?.clientHeight)col.scrollTo({top:Number(value)*rowHeight(col),behavior:'instant'});
  }
}
function onRoll(which:TimeColumn):void{
  const col=column(which);
  if(!opened.value||!col?.clientHeight||props.busy)return;
  const index=Math.max(0,Math.min(which==='hour'?23:59,Math.round(col.scrollTop/rowHeight(col))));
  const value=String(index).padStart(2,'0');
  if(which==='hour')hour.value=value;else minute.value=value;
}
function pickTime(which:TimeColumn,value:string):void{
  if(which==='hour')hour.value=value;else minute.value=value;
  const col=column(which);
  if(col)col.scrollTo({top:Number(value)*rowHeight(col),behavior:'instant'});
}
function rollStyle(value:string,current:string){
  const distance=Math.abs(Number(value)-Number(current));
  return {opacity:distance===0?1:distance===1?.55:.2,transform:`scale(${distance===0?1.12:.85})`};
}
async function toggleTime():Promise<void>{
  timeExpanded.value=!timeExpanded.value;
  await nextTick();
  if(timeExpanded.value)scrollRollers();
}
function onClosed():void{
  opened.value=false;
  emit('update:show',false);
}
function confirm():void{
  if(props.busy)return;
  if(timeExpanded.value){onRoll('hour');onRoll('minute');scrollRollers();}
  const day=selected.value,base=moment.unix(props.modelValue).tz(props.timeZone);
  const picked=moment.tz({year:day.year,month:day.month-1,date:day.day,hour:Number(hour.value),minute:Number(minute.value),second:base.second()},props.timeZone);
  emit('confirm',picked.unix());
}
watch(()=>props.show,async(show)=>{
  if(show)syncFromModel();
  opened.value=show;
  await nextTick();
  if(show)scrollRollers();
},{immediate:true});
</script>

<style scoped>
.popup.bill-date-sheet{
  --bill-bg:#fff;--bill-text:#4b5355;--bill-muted:#868e90;--bill-accent:#68a99a;--bill-row:28px;
  position:absolute;top:50%;left:50%;bottom:auto;width:min(92%,420px);height:auto;max-height:calc(100% - 48px);
  margin:0;border-radius:14px;background:var(--bill-bg);color:var(--bill-text);overflow:hidden;
  transform:translate(-50%,-50%) scale(.96);opacity:0;
  transition-property:transform,opacity;
  transition-duration:var(--cy-dur-normal,220ms);
  transition-timing-function:var(--cy-ease-out,cubic-bezier(0.22,1,0.36,1));
  box-shadow:0 12px 32px rgba(0,0,0,.14);
}
.popup.bill-date-sheet.modal-in,.popup.bill-date-sheet.modal-out{display:flex!important;flex-direction:column}
.popup.bill-date-sheet.modal-in{transform:translate(-50%,-50%) scale(1);opacity:1}
.popup.bill-date-sheet.modal-out{transform:translate(-50%,-50%) scale(.96);opacity:0}
.dark .popup.bill-date-sheet{--bill-bg:#232a31;--bill-text:#bbc1c2;--bill-muted:#91999b}
.bill-date-sheet button{border:0;background:none;color:inherit;font:inherit;cursor:pointer;-webkit-tap-highlight-color:transparent;transition:transform var(--cy-dur-press,70ms) var(--cy-ease-out,ease),background-color var(--cy-dur-fast,150ms) ease}
.bill-date-sheet button:focus-visible{outline:2px solid var(--bill-accent);outline-offset:-2px}
.bill-date-body{min-height:0;overflow-y:auto;overscroll-behavior:contain;padding:10px 14px 0}
.bill-date-title{display:flex;align-items:center;justify-content:space-between;gap:8px;margin:0 6px 17px;min-height:48px}
.bill-date-title strong{font-size:28px;font-weight:500;white-space:nowrap;font-variant-numeric:tabular-nums}
.bill-date-title small{font-size:13px;font-weight:400;margin-left:5px;vertical-align:top;display:inline-block;padding-top:6px}
.bill-date-title nav{display:flex;gap:2px}
.bill-date-title button{width:34px;height:40px;color:var(--bill-muted)}
.bill-date-title :deep(.icon){font-size:15px}
.bill-date-week,.bill-date-grid{display:grid;grid-template-columns:repeat(7,minmax(0,1fr));text-align:center}
.bill-date-week{font-size:13px;margin-bottom:10px;color:var(--bill-text)}
.bill-date-grid{row-gap:8px}
.bill-date-slot{height:48px;display:grid;place-items:center;min-width:0}
.bill-date-cell{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:2px;width:42px;height:42px;border-radius:50%;padding:0;line-height:1.25}
.bill-date-cell b{font-size:17px;font-weight:500;font-variant-numeric:tabular-nums}
.bill-date-cell small{font-size:10px;color:var(--bill-muted);white-space:nowrap}
.bill-date-cell small.festival{color:var(--bill-accent)}
.bill-date-cell.today{background:#edf4f1;color:var(--bill-accent)}
.dark .bill-date-cell.today:not(.selected){background:#f7f7f7}
.bill-date-cell.today small{color:var(--bill-accent)}
.bill-date-cell.selected{background:var(--bill-accent);color:#fff}
.bill-date-cell.selected small{color:#fff}
.bill-date-time-row{display:flex;align-items:center;gap:12px;width:100%;min-height:52px;margin-top:12px;padding:4px 7px;font-size:17px!important;text-align:left}
.bill-date-time-row>small{font-size:13px;margin-left:auto;color:var(--bill-muted);font-variant-numeric:tabular-nums}
.bill-date-time-row :deep(.icon){font-size:18px}
.bill-date-time-row :deep(.icon:last-child){margin-left:auto;color:var(--bill-muted);transition:transform var(--cy-dur-fast,150ms) var(--cy-ease-out,ease)}
.bill-date-time-row[aria-expanded=true] :deep(.icon:last-child){transform:rotate(90deg)}
.bill-date-rollers{display:flex;align-items:center;justify-content:center;gap:8px;margin:0 35px 9px;height:calc(5 * var(--bill-row));overflow:hidden;mask-image:linear-gradient(transparent,#000 25%,#000 75%,transparent)}
.bill-date-roller{position:relative;flex:1;min-width:0;height:100%;box-sizing:border-box;padding-block:calc(2 * var(--bill-row));overflow-y:auto;scroll-snap-type:y mandatory;overscroll-behavior:contain;scrollbar-width:none;touch-action:pan-y}
.bill-date-roller::-webkit-scrollbar{display:none}
.bill-date-roller button{display:block;width:100%;height:var(--bill-row);min-height:var(--bill-row);padding:0;line-height:var(--bill-row);font-size:23px;scroll-snap-align:center;font-variant-numeric:tabular-nums}
.bill-date-colon{font-size:26px;padding-bottom:3px}
.bill-date-error{font-size:13px;line-height:1.5;color:var(--cy-expense,#c34e4e);margin:8px}
.bill-date-footer{flex:none;display:flex;justify-content:space-around;padding:6px 10px 9px}
.bill-date-footer button{font-size:18px;width:45%;min-height:46px;padding:8px 16px}
.bill-date-footer .confirm{color:var(--bill-accent)}
.bill-date-sheet button:disabled{opacity:.5;cursor:default}
.bill-date-rollers.disabled{pointer-events:none}
@media(max-width:350px){.bill-date-body{padding-inline:9px}.bill-date-cell{width:38px;height:38px}.bill-date-grid{row-gap:4px}}
@media(max-height:700px){.bill-date-grid{row-gap:3px}.bill-date-title{margin-bottom:10px}.bill-date-time-row{margin-top:5px}.popup.bill-date-sheet{--bill-row:26px}}
@media(prefers-reduced-motion:reduce){.popup.bill-date-sheet,.bill-date-sheet button,.bill-date-time-row :deep(.icon:last-child){transition-duration:1ms}}
:global(:root[data-reduced-motion="true"]) .popup.bill-date-sheet,:global(:root[data-reduced-motion="true"]) .bill-date-sheet button,:global(:root[data-reduced-motion="true"]) .bill-date-time-row :deep(.icon:last-child){transition-duration:1ms}
</style>
