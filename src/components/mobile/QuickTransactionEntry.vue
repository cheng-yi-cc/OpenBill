<template>
    <section class="cy-quick-entry" aria-label="快速记账">
        <div class="cy-entry-selection"><slot name="selection"><LedgerCategoryGrid :categories="categories" :category-type="categoryType" :book-id="bookId" :preserve-category="preserveCategory" :disabled="disabled" v-model="categoryId" /></slot></div>
        <div ref="detailsBoxEl" class="cy-entry-details">
            <div class="cy-note-amount-row">
                <button class="cy-tag-button" :disabled="disabled" @click="$emit('tags')"><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="mdiTagOutline" /></svg><span>{{ tags.length ? `${tags.length} 个标签` : '标签' }}</span></button>
                <input v-model="comment" :maxlength="TRANSACTION_MAX_COMMENT_LENGTH" aria-label="备注" placeholder="添加备注" />
                <output ref="amountOutputEl" :class="[amountClass, amountSizeClass]" aria-label="记账金额" aria-live="polite">
                    <button v-if="currencyToggleable && !destinationActive" type="button" class="cy-currency-toggle" :disabled="disabled" :aria-label="activeCurrency === 'CNY' ? '切换为美元记账' : '切换为人民币记账'" @click="$emit('toggle-currency')">{{ currencySymbol }}</button>
                    <small v-else>{{ currencySymbol }}</small>
                    <span ref="exprEl" class="cy-amount-expr">{{ displayExpression }}</span>
                </output>
            </div>
            <p v-if="conversionPreview && !destinationActive" class="cy-conversion-preview" aria-live="polite">{{ conversionPreview }}</p>
            <div v-if="tags.length" class="cy-selected-tags"><button v-for="tag in tags" :key="tag.id" @click="$emit('tags')">{{ tag.name }}</button></div>
            <div v-if="experience.preferences.historicalRemarks && matchingRemarks.length" class="cy-selected-tags"><button v-for="remark in matchingRemarks" :key="remark" @click="comment=remark">{{ remark }}</button></div>
            <slot name="attachments" />
            <div v-if="transfer && currency !== destinationCurrency" class="cy-amount-tabs"><button :aria-pressed="!destinationActive" @click="selectAmount(false)">转出 · {{ currency }}</button><button :aria-pressed="destinationActive" @click="selectAmount(true)">转入 · {{ destinationCurrency }}</button></div>
            <div class="cy-entry-meta"><slot name="context" /><button class="cy-meta-more" aria-label="更多账单属性" @click="openMore"><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="mdiDotsHorizontal" /></svg></button></div>
            <Transition name="cy-fade"><p v-if="conversionError || amountError || validationMessage" class="cy-amount-error" role="alert">{{ conversionError || amountError || validationMessage }}</p></Transition>
        </div>
        <div class="cy-entry-keyboard" aria-label="金额键盘">
            <button v-for="key in experience.preferences.keyboard === 'ascending' ? ['1','2','3'] : ['7','8','9']" :key="key" :disabled="disabled" @click="press(key)">{{ key }}</button>
            <button :disabled="disabled" aria-label="删除一位（长按清空）" @click="press('⌫')" @taphold="clearAll"><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="mdiBackspaceOutline" /></svg></button>
            <button v-for="key in ['4','5','6']" :key="key" :disabled="disabled" @click="press(key)">{{ key }}</button>
            <div class="cy-key-operations"><button v-for="key in ['+','−','×','÷']" :key="key" :disabled="disabled" :aria-label="({'+':'加','−':'减','×':'乘','÷':'除'})[key]" @click="press(key)">{{ key }}</button></div>
            <button v-for="key in experience.preferences.keyboard === 'ascending' ? ['7','8','9'] : ['1','2','3']" :key="key" :disabled="disabled" @click="press(key)">{{ key }}</button>
            <button class="cy-key-next" :disabled="disabled || (allowContinue && (!canContinue || !!validationMessage || !!conversionError))" @click="allowContinue ? saveAndContinue() : $emit('cancel')">{{ allowContinue ? '再记' : '取消' }}</button>
            <button :disabled="disabled" @click="press('0')">0</button>
            <button :disabled="disabled" aria-label="小数点" @click="press('.')">.</button>
            <button class="cy-key-save" :disabled="disabled" @click="complete">{{ operation ? '=' : '保存' }}</button>
        </div>
    </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import Decimal from 'decimal.js';
import { mdiTagOutline, mdiDotsHorizontal, mdiBackspaceOutline } from '@mdi/js';
import LedgerCategoryGrid from './LedgerCategoryGrid.vue';
import type { CategoryType } from '@/core/category.ts';
import type { TransactionCategory } from '@/models/transaction_category.ts';
import { calculateEntryAmount, type EntryOperation } from '@/lib/ledger-entry.ts';
import { formatEntryCurrencySymbol, sanitizeDecimalInput } from '@/lib/currency-conversion.ts';
import { TRANSACTION_MAX_AMOUNT, TRANSACTION_MIN_AMOUNT, TRANSACTION_MAX_COMMENT_LENGTH } from '@/consts/transaction.ts';
import { useLedgerExperienceStore } from '@/stores/ledgerExperience.ts';
import { animateSettle, animateShake } from '@/lib/motion.ts';

const props = defineProps<{
    categories: TransactionCategory[]; bookId: string; preserveCategory: boolean; categoryType: CategoryType;
    currency: string; destinationCurrency: string; transfer: boolean; canSave: boolean; disabled: boolean; allowContinue: boolean;
    tags: { id: string; name: string }[]; amountClass?: string; validationMessage?: string;
    remarks?: string[];
    currencyToggleable?: boolean; conversionPreview?: string; conversionError?: string;
}>();
const amount = defineModel<number>('amount', { required: true });
const destinationAmount = defineModel<number>('destinationAmount', { required: true });
const categoryId = defineModel<string>('categoryId', { required: true });
const comment = defineModel<string>('comment', { required: true });
const emit = defineEmits<{
    (e: 'save'): void;
    (e: 'continue'): void;
    (e: 'more'): void;
    (e: 'tags'): void;
    (e: 'cancel'): void;
    (e: 'toggle-currency'): void;
    (e: 'preview', value: string | null): void;
}>();
const Money = Decimal.clone({ precision: 40 });
const experience=useLedgerExperienceStore();
const detailsBoxEl = ref<HTMLElement>(), amountOutputEl = ref<HTMLElement>(), exprEl = ref<HTMLElement>();
const matchingRemarks=computed(()=>(props.remarks||[]).filter(text=>text!==comment.value&&(!comment.value||text.includes(comment.value))).slice(0,5));
const destinationActive = ref(false), input = ref(''), operand = ref<string | null>(null), operation = ref<EntryOperation | ''>(''), amountError = ref('');
const activeCurrency = computed(() => destinationActive.value ? props.destinationCurrency : props.currency);
const currencySymbol = computed(() => formatEntryCurrencySymbol(activeCurrency.value));
const displayExpression = computed(() => {
    if (operation.value && operand.value !== null) {
        return `${operand.value}${operation.value}${input.value}`;
    }
    return input.value || '0.00';
});
const amountSizeClass = computed(() => {
    const len = displayExpression.value.length;
    if (len > 14) return 'cy-amount-tight';
    if (len > 9) return 'cy-amount-compact';
    return '';
});
const evaluatedPreview = computed<string | null>(() => {
    if (operation.value && operand.value !== null) {
        if (!input.value || input.value === '-') return sanitizeDecimalInput(operand.value);
        try {
            return calculateEntryAmount(sanitizeDecimalInput(operand.value), operation.value, sanitizeDecimalInput(input.value));
        } catch {
            return null;
        }
    }
    if (!input.value || input.value === '-') return '0';
    return sanitizeDecimalInput(input.value);
});
const canContinue = computed(() => {
    if (props.canSave) return true;
    return !!operation.value && !!evaluatedPreview.value && !new Money(evaluatedPreview.value).isZero();
});
const activeAmount = computed({ get: () => destinationActive.value ? destinationAmount.value : amount.value, set: (value: number) => { if (destinationActive.value) destinationAmount.value = value; else amount.value = value; } });
watch(() => props.transfer, transfer => { if (!transfer) destinationActive.value = false; });
watch(evaluatedPreview, value => {
    if (!destinationActive.value) emit('preview', value);
}, { immediate: true, flush: 'sync' });
watch(displayExpression, () => {
    void nextTick(() => {
        if (exprEl.value) exprEl.value.scrollLeft = exprEl.value.scrollWidth;
    });
});
watch([activeAmount, destinationActive], () => {
    if (operation.value) return;
    const value = new Money(activeAmount.value).div(100);
    if (new Money(input.value && input.value !== '-' ? sanitizeDecimalInput(input.value) : '0').eq(value)) return;
    input.value = value.isZero() ? '' : value.toString();
}, { immediate: true });

function accept(value: string): boolean {
    const decimal = new Money(value && value !== '-' ? sanitizeDecimalInput(value) : '0');
    const cents = decimal.mul(100);
    if (cents.lt(TRANSACTION_MIN_AMOUNT) || cents.gt(TRANSACTION_MAX_AMOUNT)) { amountError.value = '金额超出允许范围。'; animateShake(detailsBoxEl.value); return false; }
    amountError.value = ''; input.value = value;
    if (!operation.value) activeAmount.value = cents.toNumber();
    return true;
}
function calculate(): boolean {
    if (!operation.value || operand.value === null) return true;
    if (!input.value || input.value === '-') {
        const settled = sanitizeDecimalInput(operand.value);
        operation.value = ''; operand.value = null;
        return accept(settled === '0' ? '' : settled);
    }
    try {
        const result = calculateEntryAmount(sanitizeDecimalInput(operand.value), operation.value, sanitizeDecimalInput(input.value));
        operation.value = ''; operand.value = null;
        const accepted = accept(result);
        if (accepted) animateSettle(amountOutputEl.value, { scale: 0.96, duration: 150, opacity: 0.8 });
        return accepted;
    } catch (cause) { amountError.value = (cause as Error).message; animateShake(detailsBoxEl.value); return false; }
}
function press(key: string): void {
    experience.vibrate();
    if (['+', '−', '×', '÷'].includes(key)) {
        if (operation.value && !input.value) { operation.value = key as EntryOperation; return; }
        if (!calculate()) return;
        operand.value = input.value && input.value !== '-' ? sanitizeDecimalInput(input.value) : '0'; operation.value = key as EntryOperation; input.value = ''; return;
    }
    if (key === '⌫') {
        if (!input.value && operation.value && operand.value !== null) {
            const prev = operand.value;
            operation.value = '';
            operand.value = null;
            accept(prev === '0' ? '' : prev);
            return;
        }
        accept(input.value.slice(0, -1));
        return;
    }
    if (key === '.') { if (!input.value.includes('.')) accept(`${input.value === '-' ? '-0' : input.value || '0'}.`); return; }
    if (input.value.includes('.') && input.value.split('.')[1]!.length >= 2) return;
    accept((input.value === '0' ? '' : input.value) + key);
}
function clearAll(): void {
    experience.vibrate();
    operation.value = '';
    operand.value = null;
    accept('');
}
function complete(): void {
    experience.vibrate();
    if (operation.value) { calculate(); return; }
    if (props.conversionError || props.validationMessage) { amountError.value = props.conversionError || props.validationMessage || ''; animateShake(detailsBoxEl.value); return; }
    if (!props.canSave || !input.value || new Money(input.value === '-' ? '0' : sanitizeDecimalInput(input.value)).isZero()) {
        amountError.value = '请输入金额后再保存';
        animateShake(detailsBoxEl.value);
        return;
    }
    amountError.value = '';
    emit('save');
}
function saveAndContinue(): void {
    if (!calculate()) return;
    if (props.conversionError || props.validationMessage) { amountError.value = props.conversionError || props.validationMessage || ''; animateShake(detailsBoxEl.value); return; }
    emit('continue');
}
function selectAmount(destination: boolean): void {
    if (!calculate()) return;
    destinationActive.value = destination;
    const value = new Money(activeAmount.value).div(100);
    input.value = value.isZero() ? '' : value.toString();
}
function openMore(): void { if (calculate()) emit('more'); }
defineExpose({ prepareSave: calculate });
</script>

<style scoped>
.cy-quick-entry{display:flex;flex-direction:column;height:calc(100vh - var(--f7-navbar-height) - var(--f7-safe-area-top,0px));height:calc(100dvh - var(--f7-navbar-height) - var(--f7-safe-area-top,0px));min-height:470px;max-width:720px;margin:auto;padding:4px 5px max(10px,var(--f7-safe-area-bottom,env(safe-area-inset-bottom)));gap:6px;box-sizing:border-box;color:var(--cy-ink)}
.cy-entry-selection{flex:1;min-height:110px;border-radius:13px;background:var(--cy-card);overflow:auto;border:1px solid var(--cy-line)}
.cy-entry-details{flex:none;background:var(--cy-card);border:1px solid var(--cy-line);border-radius:13px;padding:10px 12px;display:flex;flex-direction:column;gap:8px}
.cy-note-amount-row{display:flex;align-items:center;gap:10px;min-height:37px}.cy-tag-button{display:flex;align-items:center;gap:5px;border:0;padding:6px 0;background:transparent;color:var(--cy-muted);white-space:nowrap;font-size:13px}.cy-tag-button svg{width:19px;height:19px;fill:currentColor;flex:none}
.cy-note-amount-row>input{flex:1;min-width:35px;width:60px;border:0;background:transparent;padding:6px 0;font-size:13px;color:var(--cy-ink)}.cy-note-amount-row>input::placeholder{color:var(--cy-muted)}
.cy-note-amount-row output{max-width:66%;min-width:0;display:inline-flex;align-items:baseline;justify-content:flex-end;gap:4px;text-align:right;font-size:clamp(22px,6vw,29px);font-weight:600;line-height:1.25;font-variant-numeric:tabular-nums}.cy-note-amount-row output.cy-amount-compact{font-size:clamp(18px,4.8vw,23px)}.cy-note-amount-row output.cy-amount-tight{font-size:clamp(15px,4vw,19px)}
.cy-note-amount-row output small{font-size:12px;font-weight:400;color:var(--cy-muted);flex:none}.cy-amount-expr{min-width:0;white-space:nowrap;overflow-x:auto;scrollbar-width:none}.cy-amount-expr::-webkit-scrollbar{display:none}
.cy-currency-toggle{flex:none;display:inline-flex;align-items:center;justify-content:center;min-width:28px;min-height:30px;padding:0 5px;border:0;border-radius:6px;background:var(--cy-soft);color:inherit;font-size:15px;font-weight:600;line-height:1;transition:none}
.cy-note-amount-row output.cy-expense{color:var(--cy-expense)}.cy-note-amount-row output.cy-income{color:var(--cy-accent)}.cy-note-amount-row output.cy-expense small,.cy-note-amount-row output.cy-income small{color:inherit}
.cy-conversion-preview{font-size:12px;color:var(--cy-muted);text-align:right;line-height:1.4;margin-top:-2px}
.cy-selected-tags{display:flex;gap:5px;overflow:auto}.cy-selected-tags button{border:0;flex:none;border-radius:6px;padding:3px 8px;background:var(--cy-soft);color:var(--cy-accent);font-size:11px}
.cy-entry-meta{display:flex;flex-wrap:nowrap;overflow-x:auto;align-items:center;gap:18px;min-height:30px}.cy-entry-meta :deep(>*){flex-shrink:0}.cy-meta-more{margin-left:auto;border:0;padding:4px;background:transparent;color:var(--cy-muted);line-height:1}.cy-meta-more svg{width:20px;height:20px;fill:currentColor}
.cy-amount-tabs{display:flex;gap:8px}.cy-amount-tabs button{background:transparent;border:0;border-radius:6px;color:var(--cy-muted);padding:5px 8px;font-size:11px}.cy-amount-tabs button[aria-pressed=true]{background:var(--cy-soft);color:var(--cy-accent)}
.cy-amount-error{font-size:12px;color:var(--cy-expense);line-height:1.5}
.cy-entry-keyboard{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));grid-template-rows:repeat(4,48px);gap:5px;flex:none}.cy-entry-keyboard>button,.cy-key-operations{border:1px solid var(--cy-line);border-radius:10px;background:var(--cy-card);color:var(--cy-ink);font-size:22px;font-weight:500;padding:0;touch-action:manipulation}.cy-entry-keyboard>button{transition:transform 140ms var(--cy-ease-out,ease-out),background-color 120ms var(--cy-ease-out,ease-out),opacity 120ms ease}.cy-entry-keyboard>button svg{width:27px;height:27px;fill:currentColor;vertical-align:middle}
.cy-key-operations{grid-column:4;grid-row:2 / 4;display:grid;grid-template-columns:1fr 1fr;overflow:hidden}.cy-key-operations button{border:0;padding:0;background:transparent;color:var(--cy-ink);font-size:22px;touch-action:manipulation;transition:transform 140ms var(--cy-ease-out,ease-out),background-color 120ms var(--cy-ease-out,ease-out)}.cy-entry-keyboard button:active:not(:disabled){background:var(--cy-soft);transition-duration:55ms;transform:scale(0.945)}.cy-entry-keyboard .cy-key-save{background:var(--cy-accent);color:var(--cy-card);font-size:16px;border-color:var(--cy-accent)}.cy-entry-keyboard .cy-key-save:active:not(:disabled){background:var(--cy-accent);filter:brightness(0.92)}.cy-entry-keyboard .cy-key-next{font-size:16px}
.cy-quick-entry button:disabled{opacity:.45}.cy-quick-entry button:focus-visible{outline:2px solid var(--cy-accent);outline-offset:-2px}
@media(min-height:850px){.cy-entry-keyboard{grid-template-rows:repeat(4,53px)}}@media(max-height:600px){.cy-quick-entry{height:560px}}@media(max-width:350px){.cy-entry-details{padding-inline:8px}.cy-note-amount-row{gap:7px}.cy-tag-button{font-size:12px}}
</style>
