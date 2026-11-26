<template>
  <f7-page class="cy-mobile-surface cy-investment-page" @page:beforein="start">
    <f7-navbar :title="title" back-link="返回"><f7-nav-right><f7-link v-if="action!=='void'" :disabled="busy||loading" aria-label="保存投资记录" @click="save"><f7-preloader v-if="busy" /><f7-icon v-else f7="checkmark" /></f7-link></f7-nav-right></f7-navbar>
    <main class="inv-body">
      <p v-if="error" class="cy-message" role="alert">{{ error }}</p><p v-if="loading" class="cy-empty">正在加载…</p>
      <form v-else @submit.prevent="save">
        <template v-if="action==='account'">
          <section class="inv-card inv-form"><label class="inv-row"><span>账户名称</span><input v-model="accountDraft.name" maxlength="64" required placeholder="输入名称" /></label><label class="inv-row"><span>账户类型</span><select v-model="accountDraft.kind" :disabled="!!accountDraft.id"><option value="BROKER">证券账户</option><option value="OTHER">其他理财</option></select></label></section>
        </template>
        <template v-else-if="action==='quote'">
          <section class="inv-card inv-form"><label class="inv-row"><span>理财名称</span><select v-model="draft.instrumentId"><option v-for="a in assets" :key="a.id" :value="a.id">{{ a.name }}</option></select></label><label class="inv-row"><span>每{{ unitLabel }}价格</span><input v-model="price" inputmode="decimal" required placeholder="输入参考价格" /></label><label v-if="asset?.type==='CRYPTO'" class="inv-row"><span>报价币种</span><select v-model="quoteCurrency"><option value="CNY">人民币</option><option value="USD">美元</option></select></label><label v-else class="inv-row"><span>报价币种</span><span>人民币（CNY）</span></label><label class="inv-row"><span>价格时间</span><input v-model="at" type="datetime-local" required /></label></section>
          <p class="inv-caption">手动价格仅用于估值展示，不修改持仓数量和成本。</p><button v-if="hasAutoQuote" type="button" class="inv-link" :disabled="busy" @click="restoreQuote">恢复自动报价</button>
        </template>
        <template v-else-if="action==='void'">
          <section class="inv-card"><h2>撤销{{ investmentNames[draft.type] }}</h2><p class="inv-caption">{{ asset?.name }} · {{ draft.quantity }} {{ unitLabel }}</p><p>关联资金变动或结算币种持仓也会一并撤回，后续持仓与成本将重新计算。</p></section>
        </template>
        <template v-else-if="isCoinSettled">
          <p class="inv-caption">{{ asset?.name }}（{{ asset?.symbol }}） · {{ settlementSymbol }} 结算</p>
          <section class="inv-card inv-form">
            <label class="inv-row"><span>{{ draft.type==='BUY'?'获得数量':'卖出数量' }}（{{ asset?.symbol }}）</span><input v-model="draft.quantity" inputmode="decimal" required aria-label="成交数量" /></label>
            <label class="inv-row"><span>{{ draft.type==='BUY'?'支付数量':'获得数量' }}（{{ settlementSymbol }}）</span><input v-model="draft.amount" inputmode="decimal" required aria-label="结算数量" /></label>
            <label class="inv-row"><span>手续费（{{ settlementSymbol }}）</span><input v-model="feeInput" inputmode="decimal" placeholder="默认 0" aria-label="手续费" /></label>
            <label class="inv-row"><span>发生时间</span><input v-model="at" type="datetime-local" required aria-label="发生日期" /></label>
            <label class="inv-row"><span>历史人民币汇率（元 / {{ settlementSymbol }}）</span><input v-model="draft.exchangeRate" inputmode="decimal" placeholder="可留空，未知成本" aria-label="历史人民币汇率" /></label>
            <label class="inv-row"><span>备注</span><input v-model="draft.note" maxlength="300" placeholder="选填" aria-label="备注" /></label>
            <div v-if="books.activeBooks.length > 1" class="inv-row"><span>所属账本</span><BookPicker v-model="draft.bookId" compact /></div>
          </section>
          <p class="inv-caption">修订币币兑换记录会重新回放后续持仓与成本。</p>
        </template>
        <template v-else>
          <p class="inv-caption">{{ asset?.name || '选择理财' }}{{ asset?.symbol?' · '+asset.symbol:'' }}</p>
          <nav v-if="trade" class="inv-mode"><button type="button" :aria-pressed="mode==='shares'" @click="mode='shares';feeMode='amount'">份额 × 价格</button><button v-if="draft.type==='BUY'" type="button" :aria-pressed="mode==='amount'" @click="mode='amount'">按买入总额</button><button v-else type="button" :aria-pressed="mode==='received'" @click="mode='received'">实际到账与份额</button></nav>
          <section class="inv-card inv-form">
            <label v-if="!q['accountId']" class="inv-row"><span>投资账户</span><select v-model="draft.accountId" required><option value="" disabled>选择账户</option><option v-for="a in accounts" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
            <label v-if="!q['instrumentId']" class="inv-row"><span>理财名称</span><select v-model="draft.instrumentId" required><option value="" disabled>选择理财</option><option v-for="a in assets" :key="a.id" :value="a.id">{{ a.name }} · {{ a.symbol }}</option></select></label>
            <template v-if="trade">
              <label v-if="mode==='amount'" class="inv-row"><span>买入总额（{{ cashCurrency }}）</span><input v-model="gross" inputmode="decimal" placeholder="含手续费的实际支付金额" aria-label="买入总额" /></label>
              <label v-if="mode!=='received'" class="inv-row"><span>{{ isFund?'确认净值':'成交价格' }}（{{ cashCurrency }} / {{ unitLabel }}）</span><input v-model="price" inputmode="decimal" :placeholder="isFund?'可留空，待净值公布后确认':'每份成交价格'" aria-label="成交价格" /></label>
              <label v-if="mode==='shares'||draft.type==='SELL'" class="inv-row"><span>{{ isFund?'确认份额':'成交数量' }}</span><input v-model="draft.quantity" inputmode="decimal" placeholder="输入实际成交数量" aria-label="成交数量" /></label>
              <div v-if="draft.type==='SELL'" class="inv-row"><span>当前可用{{ unitLabel === '份' ? '份额' : '数量' }}</span><span>{{ assetText(available) }}</span><button type="button" class="inv-link" @click="draft.quantity=available">全部</button></div>
              <label v-if="mode==='received'" class="inv-row"><span>实际到账（{{ cashCurrency }}）</span><input v-model="netReceipt" inputmode="decimal" maxlength="80" placeholder="扣除手续费后的实收金额" aria-label="实际到账" /></label>
              <label class="inv-row"><span>手续费（{{ feeMode==='percent'?'%':cashCurrency }}）</span><input v-model="feeInput" inputmode="decimal" placeholder="默认0" aria-label="手续费" /><select v-if="mode==='amount'" v-model="feeMode" aria-label="手续费单位" style="flex:0 0 64px"><option value="amount">金额</option><option value="percent">%</option></select></label>
            </template>
            <template v-else>
              <label class="inv-row"><span>{{ draft.type==='TRANSFER'?'转入数量':'持有数量' }}</span><input v-model="draft.quantity" inputmode="decimal" required aria-label="持有数量" /></label>
              <label v-if="draft.type==='TRANSFER'" class="inv-row"><span>转移手续费</span><input v-model="feeInput" inputmode="decimal" placeholder="按资产数量填写" /></label>
              <label v-else class="inv-row"><span>每{{ unitLabel }}成本（元）</span><input v-model="unitCost" inputmode="decimal" placeholder="不清楚可留空" /></label>
            </template>
            <label class="inv-row"><span>备注</span><input v-model="draft.note" maxlength="300" placeholder="输入备注" aria-label="备注" /></label>
          </section>
          <section class="inv-card inv-form">
            <label class="inv-row"><span>{{ draft.type==='SELL'?'卖出日期':draft.type==='BUY'?'买入日期':'发生日期' }}</span><input v-model="at" type="datetime-local" required aria-label="发生日期" /></label>
            <label v-if="trade&&isFund" class="inv-row"><span>确认日期</span><input v-model="confirmDate" type="date" aria-label="确认日期" /></label>
            <label v-if="trade" class="inv-row"><span>{{ draft.type==='SELL'?'收款账户':'付款账户' }}</span><select v-model="draft.cashAccountId" required aria-label="结算资金账户" @change="cashChanged"><option value="" disabled>请选择</option><option v-for="a in cash" :key="a.id" :value="a.id">{{ a.name }} · {{ a.currency }}</option></select></label>
            <label v-if="draft.type==='TRANSFER'" class="inv-row"><span>转入账户</span><select v-model="draft.toAccountId" required><option value="" disabled>请选择</option><option v-for="a in accounts.filter(a=>a.id!==draft.accountId)" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
            <div v-if="books.activeBooks.length > 1" class="inv-row"><span>所属账本</span><BookPicker v-model="draft.bookId" compact /></div>
            <label v-if="trade&&cashCurrency!=='CNY'" class="inv-row"><span>人民币汇率</span><input v-model="draft.exchangeRate" inputmode="decimal" placeholder="1结算币种折合人民币" aria-label="历史汇率" /></label>
            <div v-if="trade && !willSavePending" class="inv-row"><span>{{ draft.type==='BUY'?'实际支付':'实际到账' }}</span><strong style="margin-left:auto">{{ assetCurrencyMoney(calculation?.cash, cashCurrency) }}</strong></div>
          </section>
          <p v-if="willSavePending" class="cy-message">未填确认净值或确认日未到，保存后将进入「待确认」列表，净值公布后自动入账。</p>
          <p v-else-if="trade" class="inv-caption">价格、金额和手续费均按付款或收款账户的币种填写。</p>
          <p v-if="!trade" class="inv-caption">{{ draft.type==='TRANSFER'?'同一项资产转移到另一投资账户，手续费按资产数量扣除。':'只调整投资持仓，不扣付款账户。' }}</p>
        </template>
        <button class="inv-save" :class="{ 'inv-save-danger': action==='void' }" :disabled="busy||loading">{{ busy?'正在保存…':action==='void'?'确认撤销':willSavePending?'保存为待确认':'保存' }}</button>
      </form>
    </main>
  </f7-page>
</template>
<script setup lang="ts">
import {computed,reactive,ref} from 'vue';
import type {Router} from 'framework7/types';
import moment from 'moment-timezone';
import {useI18nUIComponents} from '@/lib/ui/mobile.ts';
import {investments,investmentError} from '@/lib/investments.ts';
import {useInvestmentData,investmentNames,validInvestmentNumber,invalidateInvestmentData} from '@/lib/investment-mobile.ts';
import {isPresetInstrument} from '@/lib/investment-display.ts';
import {LedgerDecimal} from '@/lib/ledger-display.ts';
import {assetCurrencyMoney,assetText} from '@/lib/asset-visibility.ts';
import {isCryptoAccount} from '@/lib/crypto-platforms.ts';
import {generateRandomUUID} from '@/lib/misc.ts';
import BookPicker from '@/components/mobile/BookPicker.vue';
import type {InvestmentAccount,InvestmentEvent,InvestmentEventType,InvestmentOrder} from '@/models/investment.ts';
const props=defineProps<{f7route:Router.Route;f7router:Router.Router}>(),q=props.f7route.query;
const {showToast}=useI18nUIComponents();
const {wealth,assets,accounts,events,loading,error,ready,zone,books,load}=useInvestmentData();
const action=q['action']||'BUY',busy=ref(false),at=ref(''),confirmDate=ref(''),price=ref(''),gross=ref(''),unitCost=ref(''),feeInput=ref('0'),feeMode=ref('amount'),mode=ref(action==='SELL'?'received':'shares'),quoteCurrency=ref<'CNY'|'USD'>('CNY');
const netReceipt=ref('');
const available=computed(()=>wealth.value?.positions.find(p=>p.accountId===draft.accountId&&p.instrumentId===draft.instrumentId)?.quantity||'0');
const draft=reactive<InvestmentEvent&{bookId:string}>({id:'',version:0,voided:false,type:action as InvestmentEventType,accountId:q['accountId']||'',instrumentId:q['instrumentId']||'',toAccountId:'',bookId:'',quantity:'',amount:'0',fee:'0',cost:null,settlementInstrumentId:'',settlementAccountId:'',cashAccountId:'',exchangeRate:'1',occurredAt:0,note:''});
const accountDraft=reactive<InvestmentAccount>({id:'',name:'',kind:q['kind']||'BROKER',currency:'CNY'});
const asset=computed(()=>assets.value.find(a=>a.id===draft.instrumentId)),isFund=computed(()=>asset.value?.type==='FUND'),trade=computed(()=>['BUY','SELL'].includes(draft.type)),cash=computed(()=>wealth.value?.cashAccounts||[]),cashCurrency=computed(()=>cash.value.find(a=>a.id===draft.cashAccountId)?.currency||'CNY');
const isCoinSettled=computed(()=>action==='revise'&&!!draft.settlementInstrumentId);
const settlementSymbol=computed(()=>assets.value.find(a=>a.id===draft.settlementInstrumentId)?.symbol||draft.settlementInstrumentId);
const unitLabel=computed(()=>asset.value?.type==='CRYPTO'?(asset.value.symbol||'枚'):asset.value?.type==='STOCK'?'股':'份');
const hasAutoQuote=computed(()=>!!asset.value&&(!!asset.value.provider||isPresetInstrument(asset.value)));
const willSavePending=computed(()=>!isCoinSettled.value&&action!=='revise'&&action!=='void'&&action!=='quote'&&action!=='account'&&trade.value&&isFund.value&&mode.value!=='received'&&(!price.value.trim()||(!!confirmDate.value&&confirmDate.value>moment().tz(zone.value).format('YYYY-MM-DD'))));
const title=computed(()=>action==='account'?accountDraft.id?'编辑投资账户':'添加投资账户':action==='quote'?'更新参考价格':action==='void'?'撤销记录':action==='revise'?'编辑'+(investmentNames[draft.type]||'投资记录'):(investmentNames[draft.type]||'记录投资')+(isFund.value?'基金':''));
const calculation=computed(()=>{try{
 const fee=new LedgerDecimal(validInvestmentNumber(feeInput.value||'0','手续费'));let amount,actualFee=fee,quantity=draft.quantity,unitPrice=price.value;
 if(draft.type==='SELL'&&mode.value==='received'){
  validInvestmentNumber(quantity,'数量',true);const received=new LedgerDecimal(validInvestmentNumber(netReceipt.value,'实际到账'));if(received.decimalPlaces()>2||fee.decimalPlaces()>2)return null;amount=received.plus(fee);unitPrice=amount.div(quantity).toDecimalPlaces(18).toString();
 }else{
  const p=new LedgerDecimal(validInvestmentNumber(price.value,'价格',true));
  if(mode.value==='amount'&&draft.type==='BUY'){const g=new LedgerDecimal(validInvestmentNumber(gross.value,'总额',true));amount=feeMode.value==='percent'?g.div(new LedgerDecimal(1).plus(fee.div(100))).toDecimalPlaces(2):g.minus(fee);actualFee=g.minus(amount);quantity=amount.div(p).toDecimalPlaces(isFund.value?2:18,LedgerDecimal.ROUND_DOWN).toString();}
  else amount=p.mul(validInvestmentNumber(quantity,'数量',true)).toDecimalPlaces(2);
 }
 if(!amount.gt(0)||actualFee.lt(0)||draft.type==='SELL'&&amount.lt(actualFee))return null;
 return {amount:amount.toString(),fee:actualFee.toString(),quantity,price:unitPrice,cash:(draft.type==='BUY'?amount.plus(actualFee):amount.minus(actualFee)).toString()};
}catch{return null;}});
let initialized=false,initialPrice='',initialQuantity='',initialUnitCost='',initialAmount='0',initialCost:string|null=null;const requestKey=generateRandomUUID();
function cashChanged():void{draft.exchangeRate=cashCurrency.value==='CNY'?'1':'';}
async function start():Promise<void>{if(initialized)return;await load();if(!ready.value)return;draft.bookId=books.defaultBookId;at.value=moment().tz(zone.value).format('YYYY-MM-DDTHH:mm');if(action==='instrument'){props.f7router.navigate('/investments/add?type=OTHER',{reloadCurrent:true});return;}
 if(action==='account'){const old=accounts.value.find(a=>a.id===q['accountId']);if(old)Object.assign(accountDraft,old);if(isCryptoAccount(old?.kind||q['kind'])){props.f7router.navigate('/crypto/add?'+new URLSearchParams({kind:old?.kind||q['kind']||'WALLET',id:old?.id||''}),{reloadCurrent:true});return;}}
 else if(action==='revise'||action==='void'){const old=events.value.find(e=>e.id===q['eventId']);if(!old){error.value='找不到这笔记录';return;}if(old.wallet){props.f7router.navigate('/crypto/entry?'+new URLSearchParams({eventId:old.id,action}),{reloadCurrent:true});return;}Object.assign(draft,old);mode.value='shares';netReceipt.value=new LedgerDecimal(old.amount).minus(old.fee).toString();at.value=moment.unix(old.occurredAt).tz(zone.value).format('YYYY-MM-DDTHH:mm');if(old.fund)at.value=old.fund.tradeDate+'T'+moment.unix(old.occurredAt).tz(zone.value).format('HH:mm');confirmDate.value=old.fund?.confirmDate||'';feeInput.value=old.fee;price.value=old.fund?.price||(new LedgerDecimal(old.quantity||'1').gt(0)?new LedgerDecimal(old.amount).div(old.quantity).toDecimalPlaces(18).toString():'');unitCost.value=old.cost&&new LedgerDecimal(old.quantity).gt(0)?new LedgerDecimal(old.cost).div(old.quantity).toDecimalPlaces(18).toString():'';}
 else if(isCryptoAccount(accounts.value.find(a=>a.id===draft.accountId)?.kind)&&['BUY','SELL','TRANSFER','OPENING'].includes(action)){props.f7router.navigate('/crypto/convert?'+new URLSearchParams({mode:({BUY:'cash',SELL:'redeem',TRANSFER:'transfer',OPENING:'opening'} as Record<string,string>)[action]!,accountId:draft.accountId,instrumentId:draft.instrumentId}),{reloadCurrent:true});return;}
 if(!draft.accountId)draft.accountId=accounts.value.find(a=>!isCryptoAccount(a.kind))?.id||'';if(!draft.instrumentId)draft.instrumentId=assets.value.find(a=>a.type!=='CRYPTO')?.id||'';if(action==='BUY'&&isFund.value)mode.value='amount';initialPrice=price.value;initialQuantity=draft.quantity;initialUnitCost=unitCost.value;initialAmount=draft.amount;initialCost=draft.cost;if(!draft.cashAccountId)draft.cashAccountId=cash.value.find(a=>a.currency===(asset.value?.currency||'CNY')&&!a.liability)?.id||'';if(action!=='revise')cashChanged();initialized=true;}
async function save():Promise<void>{if(busy.value||!initialized)return;busy.value=true;error.value='';try{
 if(action==='account'){if(accountDraft.id)await investments.updateAccount({...accountDraft});else await investments.createAccount(accountDraft.name.trim(),accountDraft.kind);showToast('投资账户已保存');}
 else if(action==='quote'){validInvestmentNumber(price.value,'价格',true);await investments.manualQuote(draft.instrumentId,price.value,moment.tz(at.value,zone.value).unix(),asset.value?.type==='CRYPTO'?quoteCurrency.value:'CNY');showToast('参考价格已更新');}
 else if(action==='void'){await investments.voidEvent({...draft});showToast('记录已撤销');}
 else if(isCoinSettled.value){
  const date=moment.tz(at.value,'YYYY-MM-DDTHH:mm',true,zone.value);if(!date.isValid())throw Error('请选择有效日期');
  validInvestmentNumber(draft.quantity,'成交数量',true);
  validInvestmentNumber(draft.amount,'结算数量',true);
  const fee=validInvestmentNumber(feeInput.value||'0','手续费');
  const fx=draft.exchangeRate.trim()?validInvestmentNumber(draft.exchangeRate.trim(),'历史人民币汇率',true):'';
  await investments.saveEvent({...draft,occurredAt:date.unix(),fee,exchangeRate:fx},requestKey);
  showToast('交易记录已更新');
 }
 else {const date=moment.tz(at.value,'YYYY-MM-DDTHH:mm',true,zone.value);if(!date.isValid())throw Error('请选择有效日期');const event={...draft,occurredAt:date.unix()};
  if(trade.value){if(isFund.value&&mode.value!=='received'&&(!price.value||confirmDate.value>moment().tz(zone.value).format('YYYY-MM-DD'))){if(event.id)throw Error('已入账记录不能改为待确认；可先撤销再新建');const order:InvestmentOrder={id:'',planId:'',accountId:event.accountId,instrumentId:event.instrumentId,cashAccountId:event.cashAccountId,bookId:event.bookId,type:event.type as 'BUY'|'SELL',amount:mode.value==='amount'?gross.value:'',quantity:mode.value==='shares'?event.quantity:'',fee:feeMode.value==='amount'?feeInput.value||'0':'0',feePercent:feeMode.value==='percent'?feeInput.value||'0':'0',tradeDate:date.format('YYYY-MM-DD'),confirmDate:confirmDate.value,time:date.format('HH:mm'),timeZone:zone.value,note:event.note,status:'pending',eventId:'',price:'',priceDate:'',error:'',version:0};await investments.saveOrder(order,requestKey);invalidateInvestmentData();showToast('已保存为待确认记录');props.f7router.back();return;}
   if(!calculation.value)throw Error('请核对价格、份额、金额和手续费');event.amount=event.id&&mode.value==='shares'&&price.value===initialPrice&&event.quantity===initialQuantity?initialAmount:calculation.value.amount;event.quantity=calculation.value.quantity;event.fee=calculation.value.fee;event.exchangeRate=validInvestmentNumber(cashCurrency.value==='CNY'?'1':event.exchangeRate,'历史汇率',true);event.cost=null;event.toAccountId='';if(isFund.value){const confirmation=confirmDate.value||date.format('YYYY-MM-DD');const d=moment.tz(confirmation+'T'+date.format('HH:mm'),'YYYY-MM-DDTHH:mm',true,zone.value);event.occurredAt=d.unix();event.fund={tradeDate:date.format('YYYY-MM-DD'),confirmDate:confirmation,price:calculation.value.price,priceDate:confirmation,source:'手动确认'};}}
  else{validInvestmentNumber(event.quantity,'数量',event.type!=='ADJUST');event.amount='0';event.cashAccountId='';event.fee=event.type==='TRANSFER'?validInvestmentNumber(feeInput.value||'0','手续费'):'0';event.exchangeRate=event.type==='TRANSFER'?'':'1';event.cost=event.type==='TRANSFER'?null:unitCost.value?new LedgerDecimal(validInvestmentNumber(unitCost.value,'成本')).mul(event.quantity).toDecimalPlaces(18).toString():null;if(event.id&&unitCost.value===initialUnitCost&&event.quantity===initialQuantity)event.cost=initialCost;}
  await investments.saveEvent(event,requestKey);
  showToast('投资记录已保存');
 }
 invalidateInvestmentData();props.f7router.back();
 }catch(e){error.value=investmentError(e);}finally{busy.value=false;}}
async function restoreQuote():Promise<void>{if(busy.value)return;busy.value=true;try{await investments.automaticQuote(draft.instrumentId);showToast('已恢复自动报价');props.f7router.back();}catch(e){error.value=investmentError(e);}finally{busy.value=false;}}
</script>
<style scoped>
.inv-save-danger{background:var(--cy-expense)!important;color:#fff!important}
</style>
