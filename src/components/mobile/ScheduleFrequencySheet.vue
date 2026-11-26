<template>
    <f7-sheet swipe-to-close swipe-handler=".swipe-handler"
              style="height: auto" :opened="show" @sheet:open="onSheetOpen" @sheet:closed="onSheetClosed">
        <f7-toolbar class="toolbar-with-swipe-handler">
            <div class="swipe-handler"></div>
            <div class="left">
                <f7-link sheet-close icon-f7="xmark" :aria-label="tt('Close')"></f7-link>
            </div>
            <div class="right">
                <f7-button round fill icon-f7="checkmark_alt" :class="{ disabled: !canApply }" :aria-label="tt('Apply')" @click="save"></f7-button>
            </div>
        </f7-toolbar>
        <f7-page-content class="margin-top">
            <div class="grid grid-cols-2 grid-gap">
                <div>
                    <div class="schedule-frequency-type-container">
                        <f7-list dividers class="schedule-frequency-type-list no-margin-vertical">
                            <f7-list-item link="#" no-chevron
                                          :key="type.type"
                                          :title="type.displayName"
                                          v-for="type in allTransactionScheduledFrequencyTypes"
                                          @click="changeFrequencyType(type.type)">
                                <template #after>
                                    <f7-icon class="list-item-showing icon-with-direction" f7="chevron_right" v-if="currentFrequencyType === type.type"></f7-icon>
                                </template>
                            </f7-list-item>
                        </f7-list>
                    </div>
                </div>
                <div>
                    <div class="schedule-frequency-value-container">
                        <f7-list dividers class="schedule-frequency-value-list no-margin-vertical"
                                 v-if="currentFrequencyType === ScheduledTemplateFrequencyType.Disabled.type">
                            <f7-list-item :title="tt('None')"></f7-list-item>
                        </f7-list>
                        <f7-list dividers class="schedule-frequency-value-list no-margin-vertical"
                                 v-if="currentFrequencyType === ScheduledTemplateFrequencyType.Daily.type">
                            <f7-list-item :title="tt('Daily')"></f7-list-item>
                        </f7-list>
                        <f7-list dividers class="schedule-frequency-value-list no-margin-vertical"
                                 v-if="currentFrequencyType === ScheduledTemplateFrequencyType.EveryNDays.type">
                            <f7-list-item checkbox
                                          :class="isChecked(n.type) ? 'list-item-selected' : ''"
                                          :key="n.type"
                                          :value="n.type"
                                          :checked="isChecked(n.type)"
                                          :title="n.displayName"
                                          v-for="n in allAvailableNDays"
                                          @change="setFrequencyValue">
                            </f7-list-item>
                        </f7-list>
                        <f7-list dividers class="schedule-frequency-value-list no-margin-vertical"
                                 v-if="currentFrequencyType === ScheduledTemplateFrequencyType.Weekly.type">
                            <f7-list-item checkbox
                                          :class="isChecked(weekDay.type) ? 'list-item-selected' : ''"
                                          :key="weekDay.type"
                                          :value="weekDay.type"
                                          :checked="isChecked(weekDay.type)"
                                          :title="weekDay.displayName"
                                          v-for="weekDay in allWeekDays"
                                          @change="changeFrequencyValue">
                            </f7-list-item>
                        </f7-list>
                        <f7-list dividers class="schedule-frequency-value-list no-margin-vertical"
                                 v-if="currentFrequencyType === ScheduledTemplateFrequencyType.Monthly.type">
                            <f7-list-item checkbox
                                          :class="isChecked(monthDay.type) ? 'list-item-selected' : ''"
                                          :key="monthDay.type"
                                          :value="monthDay.type"
                                          :checked="isChecked(monthDay.type)"
                                          :title="monthDay.displayName"
                                          v-for="monthDay in allAvailableMonthDays"
                                          @change="changeFrequencyValue">
                            </f7-list-item>
                        </f7-list>
                        <div class="schedule-yearly-picker" v-if="currentFrequencyType === ScheduledTemplateFrequencyType.Yearly.type">
                            <div class="yearly-months">
                                <button v-for="m in 12" :key="m" type="button" :class="{ active: yearlyMonth === m }" @click="yearlyMonth = m">{{ m }}月</button>
                            </div>
                            <f7-list dividers class="schedule-frequency-value-list no-margin-vertical">
                                <f7-list-item checkbox
                                              :class="isChecked(monthAndDay.type) ? 'list-item-selected' : ''"
                                              :key="monthAndDay.type"
                                              :value="monthAndDay.type"
                                              :checked="isChecked(monthAndDay.type)"
                                              :title="monthAndDay.displayName"
                                              v-for="monthAndDay in yearlyDaysInSelectedMonth"
                                              @change="changeFrequencyValue">
                                </f7-list-item>
                            </f7-list>
                        </div>
                    </div>
                </div>
            </div>
        </f7-page-content>
    </f7-sheet>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';

import { useI18n } from '@/locales/helpers.ts';
import { type CommonScheduleFrequencySelectionProps, useScheduleFrequencySelectionBase } from '@/components/base/ScheduleFrequencySelectionBase.ts';

import { useUserStore } from '@/stores/user.ts';

import { itemAndIndex } from '@/core/base.ts';
import { type WeekDayValue } from '@/core/datetime.ts';
import { ScheduledTemplateFrequencyType } from '@/core/template.ts';
import { sortNumbersArray } from '@/lib/common.ts';
import { scrollToSelectedItem } from '@/lib/ui/common.ts';
import { type Framework7Dom, useI18nUIComponents } from '@/lib/ui/mobile.ts';

interface MobileScheduleFrequencySelectionProps extends CommonScheduleFrequencySelectionProps {
    show: boolean;
}

const props = defineProps<MobileScheduleFrequencySelectionProps>();
const emit = defineEmits<{
    (e: 'update:type', value: number): void;
    (e: 'update:modelValue', value: string): void;
    (e: 'update:show', value: boolean): void;
}>();

const { tt } = useI18n();
const { showToast } = useI18nUIComponents();
const {
    allTransactionScheduledFrequencyTypes,
    allWeekDays,
    allAvailableMonthDays,
    allAvailableMonthAndDays,
    allAvailableNDays,
    getFrequencyValues
} = useScheduleFrequencySelectionBase();

const userStore = useUserStore();

const currentFrequencyType = ref<number>(props.type);
const currentFrequencyValue = ref<number[]>(getFrequencyValues(props.modelValue));
const yearlyMonth = ref<number>(1);
const yearlyDaysInSelectedMonth = computed(() => allAvailableMonthAndDays.value.filter(item => Math.floor(item.type / 100) === yearlyMonth.value));
const canApply = computed(() => currentFrequencyType.value === ScheduledTemplateFrequencyType.Disabled.type || currentFrequencyType.value === ScheduledTemplateFrequencyType.Daily.type || currentFrequencyValue.value.length > 0);

const firstDayOfWeek = computed<WeekDayValue>(() => userStore.currentUserFirstDayOfWeek);

function isChecked(value: number): boolean {
    return currentFrequencyValue.value.indexOf(value) >= 0;
}

function changeFrequencyType(value: number): void {
    if (currentFrequencyType.value !== value) {
        currentFrequencyType.value = value;

        if (value === ScheduledTemplateFrequencyType.Daily.type) {
            currentFrequencyValue.value = [0];
        } else if (value === ScheduledTemplateFrequencyType.EveryNDays.type) {
            currentFrequencyValue.value = [1];
        } else if (value === ScheduledTemplateFrequencyType.Weekly.type) {
            currentFrequencyValue.value = [firstDayOfWeek.value];
        } else if (value === ScheduledTemplateFrequencyType.Monthly.type) {
            currentFrequencyValue.value = [1];
        } else if (value === ScheduledTemplateFrequencyType.Yearly.type) {
            currentFrequencyValue.value = [101];
        } else {
            currentFrequencyValue.value = [];
        }
    }
}

function setFrequencyValue(e: Event): void {
    const currentValue = parseInt((e.target as HTMLInputElement).value);

    if ((e.target as HTMLInputElement).checked) {
        currentFrequencyValue.value.splice(0, currentFrequencyValue.value.length, currentValue);
    } else {
        currentFrequencyValue.value.splice(0, currentFrequencyValue.value.length);
    }
}

function changeFrequencyValue(e: Event): void {
    const currentValue = parseInt((e.target as HTMLInputElement).value);

    if ((e.target as HTMLInputElement).checked) {
        for (const value of currentFrequencyValue.value) {
            if (value === currentValue) {
                return;
            }
        }

        currentFrequencyValue.value.push(currentValue);
    } else {
        for (const [value, index] of itemAndIndex(currentFrequencyValue.value)) {
            if (value === currentValue) {
                currentFrequencyValue.value.splice(index, 1);
                break;
            }
        }
    }
}

function save(): void {
    if (!canApply.value) {
        showToast('请至少选择一个执行日期');
        return;
    }
    emit('update:type', currentFrequencyType.value);
    emit('update:modelValue', sortNumbersArray(currentFrequencyValue.value).join(','));
    emit('update:show', false);
}

function close(): void {
    emit('update:show', false);
}

function onSheetOpen(event: { $el: Framework7Dom }): void {
    currentFrequencyType.value = props.type;
    currentFrequencyValue.value = getFrequencyValues(props.modelValue);
    if (currentFrequencyType.value === ScheduledTemplateFrequencyType.Yearly.type && currentFrequencyValue.value.length > 0) {
        const firstMonth = Math.floor(currentFrequencyValue.value[0]! / 100);
        if (firstMonth >= 1 && firstMonth <= 12) yearlyMonth.value = firstMonth;
    }
    scrollToSelectedItem(event.$el[0], '.schedule-frequency-value-container', '.schedule-frequency-value-container', 'li.list-item-selected');
}

function onSheetClosed(): void {
    close();
}
</script>

<style>
.schedule-frequency-type-container, .schedule-frequency-value-container {
    height: 260px;
    overflow-y: auto;
}

.schedule-frequency-type-list.list .item-inner {
    padding-inline-end: 6px;
}

.schedule-frequency-value-list.list .item-content {
    padding-inline-start: 0;
}

.schedule-yearly-picker .yearly-months {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 4px;
    padding: 4px 4px 8px;
    position: sticky;
    top: 0;
    z-index: 2;
    background: var(--f7-sheet-bg-color, var(--cy-card, #fff));
}

.schedule-yearly-picker .yearly-months button {
    border: 1px solid var(--cy-line, #e0e0e0);
    background: transparent;
    color: var(--cy-muted, #666);
    border-radius: 6px;
    padding: 5px 0;
    font-size: 12px;
}

.schedule-yearly-picker .yearly-months button.active {
    background: var(--cy-accent, #087f72);
    border-color: var(--cy-accent, #087f72);
    color: #fff;
}
</style>
