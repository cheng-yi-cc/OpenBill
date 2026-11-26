<template>
    <div ref="dialogElement" class="dialog stat-month-dialog cy-mobile-surface" role="dialog" aria-modal="true" aria-label="选择年月">
        <div ref="pickerElement" class="stat-month-picker" />
        <div class="stat-month-actions">
            <button type="button" class="dialog-button" @click="close">取消</button>
            <button type="button" class="dialog-button stat-month-today" @click="goCurrentMonth">回到本月</button>
            <button type="button" class="dialog-button stat-month-confirm" @click="confirm">确定</button>
        </div>
    </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, useTemplateRef, watch } from 'vue';
import { f7 } from 'framework7-vue';
import type { Dialog, Picker } from 'framework7/types';
import moment from 'moment-timezone';
import { useLedgerScopeStore } from '@/stores/ledgerScope.ts';
import { useLedgerExperienceStore } from '@/stores/ledgerExperience.ts';
import { ledgerAccountingMonth } from '@/lib/ledger-preferences.ts';

const props = defineProps<{ open: boolean; modelValue: string }>();
const emit = defineEmits<{ 'update:open': [value: boolean]; 'update:modelValue': [value: string] }>();
const scope = useLedgerScopeStore();
const experience = useLedgerExperienceStore();
const dialogElement = useTemplateRef<HTMLElement>('dialogElement');
const pickerElement = useTemplateRef<HTMLElement>('pickerElement');
let dialog: Dialog.Dialog | undefined;
let picker: Picker.Picker | undefined;

function destroyPicker(): void {
    picker?.destroy(); picker = undefined;
    pickerElement.value?.replaceChildren();
}
function close(): void { dialog?.close(); }
function goCurrentMonth(): void {
    const current = ledgerAccountingMonth(moment().tz(scope.timeZone), experience.preferences.monthStart);
    emit('update:modelValue', current);
    close();
}
function confirm(): void {
    const values = picker?.getValue();
    if (Array.isArray(values) && values.length === 2) {
        emit('update:modelValue', `${values[0]}-${values[1]}`);
        close();
    }
}

watch(() => props.open, open => {
    if (!open) { close(); return; }
    if (!dialogElement.value || !pickerElement.value) return;
    dialog ??= f7.dialog.create({
        el: dialogElement.value,
        closeByBackdropClick: true,
        on: {
            open() {
                const nowYear = moment().tz(scope.timeZone).year();
                const selectedYear = Number(props.modelValue.slice(0, 4)) || nowYear;
                const minYear = Math.min(2000, selectedYear - 5);
                const maxYear = Math.max(nowYear + 2, selectedYear + 2);
                const years = Array.from({ length: maxYear - minYear + 1 }, (_, i) => String(minYear + i));
                picker = f7.picker.create({
                    containerEl: pickerElement.value!, toolbar: false, rotateEffect: true,
                    value: props.modelValue.split('-'),
                    cols: [
                        { values: years, textAlign: 'center', cssClass: 'stat-year-column' },
                        { divider: true, content: '年' },
                        { values: Array.from({ length: 12 }, (_, i) => String(i + 1).padStart(2, '0')), displayValues: Array.from({ length: 12 }, (_, i) => String(i + 1)), textAlign: 'center', cssClass: 'stat-month-column' },
                        { divider: true, content: '月' }
                    ]
                });
            },
            closed() {
                destroyPicker();
                emit('update:open', false);
            }
        }
    });
    dialog.open();
}, { flush: 'post' });

onBeforeUnmount(() => { destroyPicker(); dialog?.destroy(); });
</script>

<style scoped>
.stat-month-dialog{width:min(360px,calc(100% - 48px));max-width:none;margin-left:0;transform:translate(-50%,-50%) scale(.94);opacity:0;transition:transform var(--cy-dur-normal,220ms) var(--cy-ease-out,ease-out),opacity var(--cy-dur-fast,150ms) var(--cy-ease-out,ease-out);border-radius:12px;background:var(--cy-card);color:var(--cy-ink);padding:20px 16px 10px;box-sizing:border-box;--f7-picker-inline-height:190px;--f7-picker-item-height:36px;--f7-picker-column-font-size:22px;--f7-picker-item-text-color:var(--cy-muted);--f7-picker-item-selected-text-color:var(--cy-ink);--f7-picker-divider-text-color:var(--cy-muted)}
.stat-month-dialog.modal-in{transform:translate(-50%,-50%) scale(1);opacity:1}
.stat-month-dialog.modal-out{transform:translate(-50%,-50%) scale(.94);opacity:0}
.stat-month-picker :deep(.picker-columns){gap:8px;--f7-picker-mask-bg-color:var(--cy-card)}
.stat-month-picker :deep(.picker-center-highlight){border-top:1px solid var(--cy-line);border-bottom:1px solid var(--cy-line);background:color-mix(in srgb,var(--cy-soft) 45%,transparent)}
.stat-month-picker :deep(.stat-year-column){width:108px}
.stat-month-picker :deep(.stat-month-column){width:66px}
.stat-month-picker :deep(.picker-column-divider){font-size:16px}
.stat-month-actions{display:flex;justify-content:space-between;gap:4px;margin-top:18px}
.stat-month-actions button{border:0;background:none;font:inherit;font-size:15px;color:var(--cy-muted);height:44px;flex:1;border-radius:6px}
.stat-month-actions .stat-month-today{color:var(--cy-ink);font-size:14px}
.stat-month-actions .stat-month-confirm{color:var(--cy-accent);font-weight:600}
</style>
