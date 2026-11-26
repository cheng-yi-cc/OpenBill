<template>
 <f7-page class="cy-main-page cy-mobile-surface" @page:beforein="load">
  <f7-navbar :title="original?'钱包收支记录':'钱包记账'" back-link="返回" />
  <main class="cy-page-body wallet-entry">
   <p v-if="error" class="error" role="alert">{{error}}</p>
   <p v-if="loading" class="hint">正在读取账户与持仓…</p>
   <form v-if="ready" @submit.prevent="save"><fieldset :disabled="busy || original?.voided">
    <section class="cy-panel">
     <div class="entry-types" role="group" aria-label="钱包记账类型"><button type="button" :aria-pressed="type==='EXPENSE'" @click="changeType('EXPENSE')">支出</button><button type="button" :aria-pressed="type==='INCOME'" @click="changeType('INCOME')">收入</button></div>
     <label v-if="!original && cryptoWallets.length > 1">钱包账户<select :value="account?.id" aria-label="切换钱包账户" @change="switchWallet(($event.target as HTMLSelectElement).value)"><option v-for="w in cryptoWallets" :key="w.id" :value="w.id">{{ w.name }}</option></select></label>
     <p v-else class="account-name">{{account?.name}}</p>
     <BookPicker v-if="books.activeBooks.length > 1" v-model="bookId" />
     <label>分类<select v-model="categoryId" aria-label="钱包收支分类" required><option value="" disabled>请选择分类</option><option v-for="category in categories" :key="category.id" :value="category.id">{{category.name}}</option></select></label>
     <p v-if="categoryId && !categoryIsSelectable" class="error" role="alert">原分类在当前账本不可用，请重新选择分类。</p>
     <label>计价单位<select v-model="currency" :disabled="!!original" aria-label="收支货币单位" @change="changeCurrency"><option value="USD">美元（USD）</option><option value="CNY">人民币（元）</option></select></label>
     <label class="amount-label">{{type==='EXPENSE'?'实际支出':'实际收入'}}（{{currency==='CNY'?'元':'USD'}}）<input v-model="amount" required inputmode="decimal" autocomplete="off" maxlength="32" aria-label="钱包收支金额" placeholder="0.00" @input="suggest" /></label>
     <label>时间<input v-model="dateTime" required type="datetime-local" aria-label="钱包收支时间" @change="changeDate" /></label>
     <label>备注<input v-model="note" maxlength="255" aria-label="钱包收支备注" placeholder="选填" /></label>
     <details class="more-options" :open="hasSecondaryValues">
      <summary>更多选项（标签、优惠、地点、附件与统计）</summary>
      <button class="tag-button" type="button" @click="showTags=true"><span>标签</span><span>{{tagLabel}}</span></button>
      <label>优惠金额（{{currency==='CNY'?'元':'USD'}}）<input v-model="discountAmount" inputmode="decimal" autocomplete="off" maxlength="16" aria-label="钱包优惠金额" placeholder="0.00" /></label>
      <label>地点名称<input v-model="locationName" maxlength="200" aria-label="钱包收支地点名称" placeholder="选填" /></label>
      <div v-if="picturesEnabled || pictures.length" class="picture-block">
       <div class="picture-head"><span>附件</span><button v-if="picturesEnabled && pictures.length < 10" type="button" class="text-button" :disabled="busy || uploadingPicture" @click="pictureInput?.click()">{{ uploadingPicture ? '正在上传…' : '添加附件' }}</button></div>
       <div v-if="pictures.length" class="picture-grid">
        <div v-for="(pic, idx) in pictures" :key="pic.pictureId" class="picture-card">
         <button type="button" class="picture-thumb" aria-label="查看附件" @click="openPicture(idx)"><img :src="transactionsStore.getTransactionPictureUrl(pic)" alt="钱包附件" /></button>
         <button v-if="!original?.voided" type="button" class="picture-remove" :disabled="busy || removingPictureId===pic.pictureId" aria-label="移除附件" @click="removePicture(pic)">×</button>
        </div>
       </div>
      </div>
      <div class="flag-row"><span>计入收支</span><f7-toggle :checked="!excludeFromStatistics" @toggle:change="excludeFromStatistics = !$event" /></div>
      <div v-if="type==='EXPENSE'" class="flag-row"><span>计入预算</span><f7-toggle :checked="!excludeFromBudget" @toggle:change="excludeFromBudget = !$event" /></div>
      <div class="flag-row"><span>隐藏金额</span><f7-toggle :checked="hideAmount" @toggle:change="hideAmount = $event" /></div>
     </details>
    </section>
    <section class="cy-panel"><h2>{{type==='EXPENSE'?'实际扣除币种':'实际收到币种'}}</h2>
     <label>{{type==='EXPENSE'?'优先使用':'收款币种'}}<select v-model="primary" required aria-label="本笔优先币种" @change="changeCoins"><option value="" disabled>请选择币种</option><option v-for="coin in coins" :key="coin.id" :value="coin.id">{{coin.symbol}} · {{coin.name}}</option></select></label>
     <label v-if="type==='EXPENSE'">不足时使用<select v-model="backup" aria-label="本笔备用币种" @change="changeCoins"><option value="">不使用第二种币</option><option v-for="coin in coins.filter(c=>c.id!==primary)" :key="coin.id" :value="coin.id">{{coin.symbol}} · {{coin.name}}</option></select></label>
     <label v-if="primary">{{symbol(primary)}} {{type==='EXPENSE'?'实扣':'实收'}}数量<input v-model="primaryQuantity" inputmode="decimal" maxlength="80" aria-label="优先币种实际数量" @input="manual=true;allocationError=''" /></label>
     <p v-if="primary && type==='EXPENSE'" class="hint">可用 {{available(primary)}} {{symbol(primary)}}</p>
     <label v-if="backup && type==='EXPENSE'">{{symbol(backup)}} 实扣数量<input v-model="backupQuantity" inputmode="decimal" maxlength="80" aria-label="备用币种实际数量" @input="manual=true;allocationError=''" /></label>
     <p v-if="backup && type==='EXPENSE'" class="hint">可用 {{available(backup)}} {{symbol(backup)}}</p>
     <p class="hint">默认按 1 枚约 1 美元预填，可直接按实际扣款/到账修改。</p>
     <p v-if="currency==='CNY'" class="hint">人民币金额按参考汇率 {{allocationRate || '暂缺'}} 估算数量；历史日期请填写实际数量。</p>
     <button class="text-button" type="button" @click="manual=false;suggest()">{{type==='EXPENSE'?'重新按顺序分配':'重新计算收款数量'}}</button>
     <p v-if="allocationError" class="error" role="alert">{{allocationError}}</p>
    </section>
    <section v-if="currency==='USD'" class="cy-panel"><h2>人民币折算</h2>
     <label>1 USD 兑换人民币<input v-model="exchangeRate" required inputmode="decimal" maxlength="80" aria-label="本笔美元人民币汇率" @input="confirmRate" /></label>
     <p class="hint">{{fxSource || '请填写本笔发生时的汇率'}} · {{fxDate || '日期待确认'}}</p>
     <p v-if="fxStale && !original" class="hint">当前参考汇率已过期，请核对后保存。</p>
     <strong class="cny-preview">折合 {{currencyMoney(cnyAmount,'CNY')}}</strong>
     <p class="hint">汇率随本笔记录保存，后续行情变化不改动历史收支。</p>
    </section>
    <button type="submit" class="primary" :disabled="busy">{{busy?'正在保存…':original?'保存修订':type==='EXPENSE'?'保存支出':'保存收入'}}</button>
   </fieldset></form>
   <p v-if="original?.voided" class="hint">此记录已撤销，持仓与账单已同步回退。</p>
   <button v-if="original && !original.voided" class="void-button" :disabled="busy" @click="confirmVoid">撤销这笔收支</button>
  </main>
  <transaction-tag-selection-sheet :allow-add-new-tag="true" :enable-filter="true" v-model:show="showTags" v-model="tagIds" />
  <f7-photo-browser ref="pictureBrowser" type="popup" navbar-of-text="/" :navbar-show-count="true" :exposition="false" :photos="photoUrls" :thumbs="photoUrls" />
  <input ref="pictureInput" type="file" style="display: none" :accept="`${SUPPORTED_IMAGE_EXTENSIONS};capture=camera`" @change="onUploadPicture($event)" />
 </f7-page>
</template>
<script setup lang="ts">
import {computed,ref,useTemplateRef} from 'vue';
import moment from 'moment-timezone';
import type {PhotoBrowser,Router} from 'framework7/types';
import {investments,investmentError} from '@/lib/investments.ts';
import {isCryptoAccount} from '@/lib/crypto-platforms.ts';
import {useI18nUIComponents} from '@/lib/ui/mobile.ts';
import {cryptoInput} from '@/lib/crypto-entry.ts';
import {categoryAvailableInBook} from '@/lib/category-book-visibility.ts';
import {LedgerDecimal,keepUpToDate} from '@/lib/mobile-ledger.ts';
import {allocateWalletPayment,currencyMoney,suggestedPaymentCoins,usdFX,walletCurrency} from '@/lib/wallet-entry.ts';
import {generateRandomUUID} from '@/lib/misc.ts';
import {isTransactionPicturesEnabled} from '@/lib/server_settings.ts';
import {compressJpgImageByQuality} from '@/lib/ui/common.ts';
import {KnownErrorCode} from '@/consts/api.ts';
import {SUPPORTED_IMAGE_EXTENSIONS} from '@/consts/file.ts';
import {KnownFileType} from '@/core/file.ts';
import {ImageUploadQualityType} from '@/core/image.ts';
import {useBooksStore} from '@/stores/books.ts';
import {useSettingsStore} from '@/stores/setting.ts';
import {useTransactionsStore} from '@/stores/transaction.ts';
import {useTransactionCategoriesStore} from '@/stores/transactionCategory.ts';
import {useTransactionTagsStore} from '@/stores/transactionTag.ts';
import {CategoryType} from '@/core/category.ts';
import type {TransactionPictureInfoBasicResponse} from '@/models/transaction_picture_info.ts';
import type {AssetMovement,Instrument,InvestmentAccount,InvestmentEvent,WealthSummary} from '@/models/investment.ts';
import BookPicker from '@/components/mobile/BookPicker.vue';
import TransactionTagSelectionSheet from '@/components/mobile/TransactionTagSelectionSheet.vue';
const props=defineProps<{f7route:Router.Route;f7router:Router.Router}>();
const books=useBooksStore(),settingsStore=useSettingsStore(),transactionsStore=useTransactionsStore(),categoryStore=useTransactionCategoriesStore(),tagStore=useTransactionTagsStore();
const { showToast, showConfirm } = useI18nUIComponents();
const account=ref<InvestmentAccount>(),cryptoWallets=ref<InvestmentAccount[]>([]),allCoins=ref<Instrument[]>([]),summary=ref<WealthSummary>(),original=ref<InvestmentEvent>();
const ready=ref(false),loading=ref(false),busy=ref(false),error=ref(''),allocationError=ref('');
const type=ref<'INCOME'|'EXPENSE'>('EXPENSE'),currency=ref<'USD'|'CNY'>('USD');
const amount=ref(''),discountAmount=ref(''),categoryId=ref(''),bookId=ref(''),note=ref(''),locationName=ref(''),dateTime=ref(''),zone=ref('Asia/Shanghai');
const excludeFromStatistics=ref(false),excludeFromBudget=ref(false),hideAmount=ref(false);
const tagIds=ref<string[]>([]),showTags=ref(false);
const hasSecondaryValues=computed(()=>tagIds.value.length>0||!!discountAmount.value||!!locationName.value||pictures.value.length>0||excludeFromStatistics.value||excludeFromBudget.value||hideAmount.value);
const tagLabel=computed(()=>tagIds.value.map(id=>tagStore.allTransactionTagsMap[id]?.name||'未知标签').join('、')||'选择标签');
const pictures=ref<TransactionPictureInfoBasicResponse[]>([]),uploadingPicture=ref(false),removingPictureId=ref('');
const pictureInput=useTemplateRef<HTMLInputElement>('pictureInput'),pictureBrowser=useTemplateRef<PhotoBrowser.PhotoBrowser>('pictureBrowser');
const picturesEnabled=computed(()=>isTransactionPicturesEnabled());
const imageUploadQualityType=computed(()=>ImageUploadQualityType.valueOf(settingsStore.appSettings.transactionPictureQuality)??ImageUploadQualityType.Default);
const originalPictureIds=computed(()=>new Set(original.value?.wallet?.pictureIds??[]));
const photoUrls=computed(()=>pictures.value.map(p=>transactionsStore.getTransactionPictureUrl(p)||'').filter(Boolean));
const primary=ref(''),backup=ref(''),primaryQuantity=ref(''),backupQuantity=ref(''),manual=ref(false);
const exchangeRate=ref(''),fxDate=ref(''),fxSource=ref('');
let lastRateDay='';
let requestKey=generateRandomUUID(),lastPayload='';
const coins=computed(()=>allCoins.value.filter(c=>c.type==='CRYPTO'&&((account.value?.instruments||[]).includes(c.id)||summary.value?.positions.some(p=>p.accountId===account.value?.id&&p.instrumentId===c.id)||original.value?.instrumentId===c.id||original.value?.additionalMovements?.some(m=>m.instrumentId===c.id))));
const categories=computed(()=>Object.values(categoryStore.allTransactionCategoriesMap).filter(c=>{
 const parent=categoryStore.allTransactionCategoriesMap[c.parentId],book=books.allBooks.find(b=>b.id===bookId.value);
 const unchanged=!!original.value&&original.value.bookId===bookId.value&&original.value.type===type.value&&original.value.wallet?.categoryId===c.id;
 return c.parentId!=='0'&&c.type===(type.value==='INCOME'?CategoryType.Income:CategoryType.Expense)&&!!parent&&(unchanged||(categoryAvailableInBook(parent,bookId.value,book)&&categoryAvailableInBook(c,bookId.value,book)));
}).map(c=>({id:c.id,name:`${categoryStore.allTransactionCategoriesMap[c.parentId]?.name||''} / ${c.name}`})));
const categoryIsSelectable=computed(()=>categories.value.some(category=>category.id===categoryId.value));
const positions=computed(()=>{
 const items=(summary.value?.positions||[]).map(p=>({...p}));
 if(original.value?.type==='EXPENSE'&&!original.value.voided){for(const leg of [{instrumentId:original.value.instrumentId,quantity:original.value.quantity},...(original.value.additionalMovements||[])]){const p=items.find(p=>p.accountId===account.value?.id&&p.instrumentId===leg.instrumentId);if(p)p.quantity=new LedgerDecimal(p.quantity).plus(leg.quantity).toFixed();}}
 return items;
});
const fxStale=computed(()=>usdFX(summary.value)?.state==='stale');
const allocationRate=computed(()=>currency.value==='USD'?exchangeRate.value:currentDay()===moment().tz(zone.value).format('YYYY-MM-DD')?usdFX(summary.value)?.rate||'':'');
const cnyAmount=computed(()=>{try{return new LedgerDecimal(cryptoInput(amount.value,2)).mul(cryptoInput(exchangeRate.value)).toDecimalPlaces(2,LedgerDecimal.ROUND_HALF_UP).toFixed()}catch{return null}});
function symbol(id:string):string{return allCoins.value.find(c=>c.id===id)?.symbol||''}
function available(id:string):string{return positions.value.find(p=>p.accountId===account.value?.id&&p.instrumentId===id)?.quantity||'0'}
function currentDay():string{return dateTime.value.slice(0,10)}
function initializeRate():void{
 lastRateDay=currentDay();
 if(currency.value==='CNY'){exchangeRate.value='1';fxDate.value=currentDay();fxSource.value='人民币';return}
 const fx=usdFX(summary.value);
 if(currentDay()===moment().tz(zone.value).format('YYYY-MM-DD')&&fx){exchangeRate.value=fx.rate;fxDate.value=fx.date;fxSource.value=fx.source}
 else{exchangeRate.value='';fxDate.value=currentDay();fxSource.value=''}
}
function switchWallet(nextId:string):void{
 const next=cryptoWallets.value.find(w=>w.id===nextId);
 if(!next || next.id===account.value?.id)return;
 account.value=next;
 const defaults=suggestedPaymentCoins(next,allCoins.value);
 primary.value=defaults[0]||'';backup.value=defaults[1]||'';
 manual.value=false;suggest();
}
function suggest():void{
 if(manual.value)return;
 allocationError.value='';primaryQuantity.value='';backupQuantity.value='';
 if(!amount.value||!primary.value)return;
 try{
  if(currency.value==='CNY'&&!allocationRate.value)throw Error('缺少本笔美元参考汇率，请填写实际币种数量');
  if(type.value==='INCOME'){let value=new LedgerDecimal(cryptoInput(amount.value,2));if(currency.value==='CNY')value=value.div(cryptoInput(allocationRate.value));primaryQuantity.value=value.toDecimalPlaces(18,LedgerDecimal.ROUND_DOWN).toFixed();return}
  const legs=allocateWalletPayment(amount.value,currency.value,allocationRate.value,[primary.value,backup.value],positions.value,account.value!.id);
  primaryQuantity.value=legs.find(l=>l.instrumentId===primary.value)?.quantity||'0';backupQuantity.value=legs.find(l=>l.instrumentId===backup.value)?.quantity||'0';
 }catch(e){allocationError.value=investmentError(e)}
}
function changeCoins():void{if(primary.value===backup.value)backup.value='';manual.value=false;suggest()}
function changeCurrency():void{amount.value='';discountAmount.value='';manual.value=false;initializeRate();suggest()}
function changeDate():void{if(currentDay()===lastRateDay)return;initializeRate();suggest()}
function confirmRate():void{fxSource.value='用户确认';fxDate.value=currentDay();suggest()}
function changeType(next:'INCOME'|'EXPENSE'):void{if(type.value===next)return;type.value=next;categoryId.value='';manual.value=false;suggest()}
function openPicture(idx:number):void{if(photoUrls.value.length)pictureBrowser.value?.open(idx);}
function onUploadPicture(event:Event):void{
 const el=event.target as HTMLInputElement|null;const file=el?.files?.[0];
 if(!file||uploadingPicture.value||pictures.value.length>=10)return;
 el.value='';uploadingPicture.value=true;error.value='';
 compressJpgImageByQuality(file,imageUploadQualityType.value)
  .then(blob=>transactionsStore.uploadTransactionPicture({pictureFile:KnownFileType.JPG.createFileFromBlob(blob,'image')}))
  .then(pic=>{pictures.value=[...pictures.value,pic];})
  .catch(e=>{error.value=investmentError(e);})
  .finally(()=>{uploadingPicture.value=false;});
}
function removePicture(pic:TransactionPictureInfoBasicResponse):void{
 if(busy.value||removingPictureId.value)return;
 if(originalPictureIds.value.has(pic.pictureId)){pictures.value=pictures.value.filter(p=>p.pictureId!==pic.pictureId);return;}
 removingPictureId.value=pic.pictureId;
 transactionsStore.removeUnusedTransactionPicture({pictureInfo:pic})
  .then(()=>{pictures.value=pictures.value.filter(p=>p.pictureId!==pic.pictureId);})
  .catch(e=>{if(e?.error?.errorCode===KnownErrorCode.TransactionPictureNotFound){pictures.value=pictures.value.filter(p=>p.pictureId!==pic.pictureId);return;}error.value=investmentError(e);})
  .finally(()=>{removingPictureId.value='';});
}
function parsedMovements():AssetMovement[]{
 const values=[{instrumentId:primary.value,quantity:primaryQuantity.value},...(type.value==='EXPENSE'&&backup.value?[{instrumentId:backup.value,quantity:backupQuantity.value}]:[])];
 const legs=values.filter(v=>!/^0*(\.0+)?$/.test(v.quantity.trim())).map(v=>({instrumentId:v.instrumentId,quantity:cryptoInput(v.quantity)}));
 if(!legs.length)throw Error('请填写实际扣除或收到的币种数量');
 return legs;
}
function refreshStores():void{useTransactionsStore().updateStoreInvalidState({accountList:true,transactionList:true,overview:true,statistics:true})}
async function save():Promise<void>{
 if(busy.value)return;
 error.value='';
 if(!categoryIsSelectable.value){error.value='请先选择一个可用分类';return;}
 if(!amount.value.trim()){error.value='请填写金额';return;}
 if(!primary.value){error.value='请选择实际使用的币种';return;}
 if(currency.value==='USD' && (!exchangeRate.value.trim() || cnyAmount.value===null)){error.value='请填写有效的美元人民币汇率';return;}
 if(allocationError.value){error.value=allocationError.value;return;}
 busy.value=true;
 try{
  const legs=parsedMovements(),first=legs[0]!;
  const rawDiscount=discountAmount.value.trim();
  if(rawDiscount && !/^(0|[1-9]\d{0,12})(\.\d{1,2})?$/.test(rawDiscount))throw Error('优惠金额格式无效，请填写非负且最多两位小数的金额');
  const normalizedDiscount=rawDiscount?new LedgerDecimal(rawDiscount).toFixed(2):'0.00';
  const input:InvestmentEvent={id:original.value?.id||'',version:original.value?.version||0,type:type.value,accountId:account.value!.id,instrumentId:first.instrumentId,quantity:first.quantity,additionalMovements:legs.slice(1),amount:cryptoInput(amount.value,2),fee:'0',cost:null,exchangeRate:cryptoInput(exchangeRate.value),occurredAt:moment.tz(dateTime.value,zone.value).unix(),note:note.value,bookId:bookId.value,voided:false,toAccountId:'',settlementAccountId:'',settlementInstrumentId:'',cashAccountId:'',wallet:{currency:currency.value,categoryId:categoryId.value,fxDate:fxDate.value,fxSource:fxSource.value,excludeFromStatistics:excludeFromStatistics.value,excludeFromBudget:excludeFromBudget.value,hideAmount:hideAmount.value,locationName:locationName.value,tagIds:tagIds.value,discountAmount:normalizedDiscount,pictureIds:pictures.value.map(p=>p.pictureId)}};
  if(!input.wallet!.fxSource)throw Error('请确认本笔收支汇率');
  const payload=JSON.stringify(input);if(payload!==lastPayload){requestKey=generateRandomUUID();lastPayload=payload}
  await investments.saveEvent(input,requestKey);refreshStores();showToast(original.value?'已保存钱包修订':type.value==='EXPENSE'?'已保存钱包支出':'已保存钱包收入');props.f7router.back();
 }catch(e){error.value=investmentError(e)}finally{busy.value=false}
}
function confirmVoid():void{showConfirm('撤销后，这笔收支及对应币种数量会一起回退。',()=>{void voidEntry()},undefined,{title:'撤销钱包收支',confirmText:'确认撤销',destructive:true,raw:true})}
async function voidEntry():Promise<void>{if(busy.value||!original.value)return;busy.value=true;error.value='';try{await investments.voidEvent(original.value);refreshStores();showToast('已撤销钱包收支');props.f7router.back()}catch(e){error.value=investmentError(e)}finally{busy.value=false}}
async function load():Promise<void>{
 if(ready.value||loading.value)return;loading.value=true;error.value='';
 try{
  const q=props.f7route.query;
  const [accounts,assets,wealth,settings,events]=await Promise.all([investments.accounts(),investments.instruments(),investments.summary(),investments.settings(),q['eventId']?investments.events():Promise.resolve([]),books.loadBooks(),categoryStore.loadAllCategories({force:false}).catch(keepUpToDate),tagStore.loadAllTags({force:false}).catch(keepUpToDate)]);
  summary.value=wealth;allCoins.value=assets;zone.value=settings.timeZone||zone.value;
  cryptoWallets.value=accounts.filter(a=>isCryptoAccount(a.kind));
  if(q['eventId']){original.value=events.find(e=>e.id===q['eventId']&&e.wallet);if(!original.value)throw Error('找不到这笔钱包收支')}
  account.value=accounts.find(a=>a.id===(original.value?.accountId||q['accountId']));if(!account.value)throw Error('找不到这个钱包账户');
  const defaults=suggestedPaymentCoins(account.value,assets);primary.value=defaults[0]||'';backup.value=defaults[1]||'';
  if(original.value){const e=original.value;type.value=e.type==='INCOME'?'INCOME':'EXPENSE';currency.value=e.wallet!.currency;amount.value=e.amount;categoryId.value=e.wallet!.categoryId;bookId.value=e.bookId||books.defaultBookId;note.value=e.note;dateTime.value=moment.unix(e.occurredAt).tz(zone.value).format('YYYY-MM-DDTHH:mm');lastRateDay=currentDay();exchangeRate.value=e.exchangeRate;fxDate.value=e.wallet!.fxDate;fxSource.value=e.wallet!.fxSource;primary.value=e.instrumentId;primaryQuantity.value=e.quantity;backup.value=e.additionalMovements?.[0]?.instrumentId||'';backupQuantity.value=e.additionalMovements?.[0]?.quantity||'';manual.value=true}
  else{type.value=q['type']==='INCOME'?'INCOME':'EXPENSE';currency.value=walletCurrency(account.value);amount.value=q['amount']||'';categoryId.value=q['categoryId']||'';bookId.value=q['bookId']||books.defaultBookId;note.value=q['note']||'';locationName.value=q['locationName']||'';dateTime.value=(q['time']?moment.unix(Number(q['time'])):moment()).tz(zone.value).format('YYYY-MM-DDTHH:mm');excludeFromStatistics.value=q['excludeFromStatistics']==='true';excludeFromBudget.value=q['excludeFromBudget']==='true';hideAmount.value=q['hideAmount']==='true';if(q['discountAmount']&&/^(0|[1-9]\d{0,12})(\.\d{1,2})?$/.test(q['discountAmount'])&&!/^0+(?:\.0+)?$/.test(q['discountAmount']))discountAmount.value=q['discountAmount'];initializeRate();suggest()}
  if(original.value?.wallet){excludeFromStatistics.value=original.value.wallet.excludeFromStatistics??false;excludeFromBudget.value=original.value.wallet.excludeFromBudget??false;hideAmount.value=original.value.wallet.hideAmount??false;locationName.value=original.value.wallet.locationName??'';tagIds.value=[...(original.value.wallet.tagIds??[])];const d=original.value.wallet.discountAmount;discountAmount.value=d&&d!=='0.00'&&d!=='0'?d:'';pictures.value=[...(original.value.wallet.pictures??[])];}
  else{
   if(q['tagIds']){const parsed:unknown=JSON.parse(q['tagIds']);if(!Array.isArray(parsed)||parsed.length>10||!parsed.every(id=>typeof id==='string'&&/^[1-9]\d{0,18}$/.test(id)))throw Error('草稿标签无效，请返回检查');tagIds.value=parsed}
   if(q['pictures']){const parsed:unknown=JSON.parse(q['pictures']);if(!Array.isArray(parsed)||parsed.length>10||!parsed.every(p=>p&&typeof (p as TransactionPictureInfoBasicResponse).pictureId==='string'&&/^[1-9]\d{0,18}$/.test((p as TransactionPictureInfoBasicResponse).pictureId)&&typeof (p as TransactionPictureInfoBasicResponse).originalUrl==='string'))throw Error('草稿附件无效，请返回检查');pictures.value=parsed as TransactionPictureInfoBasicResponse[]}
  }
  ready.value=true;
 }catch(e){error.value=investmentError(e)}finally{loading.value=false}
}
</script>
<style scoped>
.entry-flag{display:flex;align-items:center;gap:10px;font-size:14px}.entry-flag input{width:20px;min-height:20px;height:20px;margin:0}
.more-options{margin-top:16px;padding-top:12px;border-top:1px solid var(--cy-line)}.more-options summary{font-size:13px;color:var(--cy-accent);cursor:pointer;list-style:none}.more-options summary::-webkit-details-marker{display:none}.flag-row{display:flex;align-items:center;justify-content:space-between;min-height:44px;margin-top:8px;font-size:14px}
.tag-button{display:flex;justify-content:space-between;gap:12px;width:100%;margin-top:18px;padding:12px 0;border:0;background:none;color:var(--cy-ink);font:inherit;text-align:left}.tag-button span:last-child{color:var(--cy-muted)}
.picture-block{margin-top:18px}.picture-head{display:flex;align-items:center;justify-content:space-between;font-size:13px}.picture-grid{display:flex;flex-wrap:wrap;gap:10px;margin-top:10px}.picture-card{position:relative;width:72px;height:72px}.picture-thumb{width:100%;height:100%;padding:0;border:1px solid var(--cy-line);border-radius:10px;overflow:hidden;background:var(--cy-card)}.picture-thumb img{display:block;width:100%;height:100%;object-fit:cover}.picture-remove{position:absolute;top:-6px;right:-6px;width:22px;height:22px;padding:0;border:0;border-radius:999px;background:var(--cy-expense);color:white;font-size:14px;line-height:22px;text-align:center}
.wallet-entry{max-width:640px;margin:auto;padding-bottom:35px}fieldset{margin:0;padding:0;border:0;min-width:0}h2{font-size:17px;margin:0 0 15px}.account-name{font-size:17px;margin:18px 0}.entry-types{display:flex;gap:8px}.entry-types button{flex:1;border:1px solid var(--cy-line);border-radius:10px;background:var(--cy-card);color:var(--cy-ink);padding:12px;font:inherit}.entry-types button[aria-pressed=true]{color:var(--cy-accent);background:var(--cy-soft);border-color:var(--cy-accent)}label{display:grid;gap:9px;font-size:13px;margin-top:18px}input,select{box-sizing:border-box;width:100%;min-height:46px;padding:11px;border:1px solid var(--cy-line);border-radius:10px;font:inherit;background:var(--cy-card);color:inherit}.amount-label input{font-size:25px}.hint{font-size:12px;color:var(--cy-muted);line-height:1.8;margin-top:10px}.error{color:var(--cy-expense);font-size:13px;line-height:1.6}.text-button{padding:10px 0;border:0;background:none;color:var(--cy-accent);font:inherit;font-size:13px}.cny-preview{display:block;font-size:20px;margin:15px 0}.primary{width:100%;border:0;border-radius:12px;padding:15px;background:var(--cy-accent);color:white;font:inherit}.primary:disabled{opacity:.5}.void-button{width:100%;margin-top:20px;padding:13px;border:1px solid var(--cy-line);border-radius:10px;background:var(--cy-card);color:var(--cy-expense);font:inherit}
</style>
