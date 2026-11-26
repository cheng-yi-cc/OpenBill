<template>
    <f7-page class="cy-mobile-surface" :ptr="!sortable" @ptr:refresh="reload" @page:afterin="onPageAfterIn">
        <f7-navbar>
            <f7-nav-left :class="{ 'disabled': loading }" back-link="返回" v-if="!sortable"></f7-nav-left>
            <f7-nav-left v-else-if="sortable">
                <f7-link icon-f7="xmark" :class="{ 'disabled': displayOrderSaving }" aria-label="取消排序" @click="cancelSort"></f7-link>
            </f7-nav-left>
            <f7-nav-title :title="templateType === TemplateType.Schedule.type ? '周期记账' : '记账模板'"></f7-nav-title>
            <f7-nav-right :class="{ 'navbar-compact-icons': true, 'disabled': loading }">
                <f7-link icon-f7="ellipsis" :class="{ 'disabled': !templates.length || sortable }" aria-label="更多操作" @click="showMoreActionSheet = true"></f7-link>
                <f7-link icon-f7="plus" aria-label="新增" :href="'/template/add?templateType=' + templateType" v-if="!sortable"></f7-link>
                <f7-link icon-f7="checkmark_alt" :class="{ 'disabled': displayOrderSaving || !displayOrderModified }" aria-label="保存排序" @click="saveSortResult" v-else-if="sortable"></f7-link>
            </f7-nav-right>
        </f7-navbar>

        <f7-list strong inset dividers class="margin-top-half skeleton-text" v-if="loading">
            <f7-list-item title="加载模板中…" :key="itemIdx" v-for="itemIdx in [ 1, 2, 3 ]">
                <template #media>
                    <f7-icon f7="doc_plaintext"></f7-icon>
                </template>
            </f7-list-item>
        </f7-list>

        <div class="cy-page-body" v-if="!loading && noAvailableTemplate">
            <section class="cy-panel cy-empty">
                <p v-if="templateType === TemplateType.Normal.type">还没有记账模板<br /><small>添加后点击即可快速记账，也可在首页长按「添加一条新记账」直接调用</small></p>
                <p v-else>还没有周期记账<br /><small>设置房租、会员费等固定周期收支，到点自动入账</small></p>
                <f7-link :href="'/template/add?templateType=' + templateType" class="cy-button cy-primary" style="margin-top:14px">立即添加</f7-link>
            </section>
        </div>

        <f7-list strong inset dividers sortable class="margin-top-half template-list"
                 :sortable-enabled="sortable"
                 v-if="!loading && !noAvailableTemplate"
                 @sortable:sort="onSort">
            <f7-list-item
                          :class="{ 'actual-first-child': template.id === firstShowingId, 'actual-last-child': template.id === lastShowingId }"
                          :id="getTemplateDomId(template)"
                          :title="template.name"
                          :subtitle="templateSubtitle(template)"
                          :footer="template.hidden ? (templateType === TemplateType.Schedule.type ? '已停用' : '已隐藏') : ''"
                          :key="template.id"
                          link="#"
                          no-chevron
                          v-for="template in templates"
                          v-show="showHidden || !template.hidden"
                          @click="onTemplateClick(template)"
                          @taphold="openTemplateMenu(template)">
                <template #media>
                    <f7-icon :f7="templateType === TemplateType.Schedule.type ? 'clock' : 'doc_plaintext'">
                        <f7-badge color="gray" class="right-bottom-icon" v-if="template.hidden">
                            <f7-icon f7="eye_slash_fill"></f7-icon>
                        </f7-badge>
                    </f7-icon>
                </template>
                <template #after>
                    <span class="template-amount" :class="templateAmountClass(template)">{{ templateAmount(template) }}</span>
                    <button v-if="!sortable" type="button" class="template-more-btn" aria-label="更多模板操作" @click.stop.prevent="openTemplateMenu(template)">
                        <f7-icon f7="ellipsis_vertical" size="18" />
                    </button>
                </template>
            </f7-list-item>
        </f7-list>

        <f7-actions close-by-outside-click close-on-escape :opened="showMoreActionSheet" @actions:closed="showMoreActionSheet = false">
            <f7-actions-group>
                <f7-actions-button :class="{ 'disabled': !templates || templates.length < 2 }" @click="setSortable()">调整顺序</f7-actions-button>
                <f7-actions-button v-if="!showHidden" @click="showHidden = true">{{ templateType === TemplateType.Schedule.type ? '显示已停用项' : '显示隐藏模板' }}</f7-actions-button>
                <f7-actions-button v-if="showHidden" @click="showHidden = false">{{ templateType === TemplateType.Schedule.type ? '隐藏已停用项' : '隐藏停用模板' }}</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button bold close>取消</f7-actions-button>
            </f7-actions-group>
        </f7-actions>

        <f7-actions close-by-outside-click close-on-escape :opened="showItemActionSheet" @actions:closed="showItemActionSheet = false">
            <f7-actions-group v-if="activeTemplate">
                <f7-actions-label>{{ activeTemplate.name }}</f7-actions-label>
                <f7-actions-button v-if="templateType === TemplateType.Normal.type" @click="useTemplate(activeTemplate)">使用此模板记账</f7-actions-button>
                <f7-actions-button @click="edit(activeTemplate)">编辑</f7-actions-button>
                <f7-actions-button @click="hide(activeTemplate, !activeTemplate.hidden)">{{ activeTemplate.hidden ? (templateType === TemplateType.Schedule.type ? '恢复启用' : '取消隐藏') : (templateType === TemplateType.Schedule.type ? '暂停此周期记账' : '隐藏模板') }}</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group v-if="activeTemplate">
                <f7-actions-button color="red" @click="remove(activeTemplate, false)">删除</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button bold close>取消</f7-actions-button>
            </f7-actions-group>
        </f7-actions>

        <f7-actions close-by-outside-click close-on-escape :opened="showDeleteActionSheet" @actions:closed="showDeleteActionSheet = false">
            <f7-actions-group>
                <f7-actions-label>{{ `确定删除「${templateToDelete?.name || ''}」？` }}</f7-actions-label>
                <f7-actions-button color="red" @click="remove(templateToDelete, true)">删除</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button bold close>取消</f7-actions-button>
            </f7-actions-group>
        </f7-actions>
    </f7-page>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import type { Router } from 'framework7/types';

import { useI18n } from '@/locales/helpers.ts';
import { useI18nUIComponents, showLoading, hideLoading } from '@/lib/ui/mobile.ts';

import { useTransactionTemplatesStore } from '@/stores/transactionTemplate.ts';
import { useAccountsStore } from '@/stores/account.ts';
import { useTransactionCategoriesStore } from '@/stores/transactionCategory.ts';

import { TemplateType, ScheduledTemplateFrequencyType } from '@/core/template.ts';
import { TransactionType } from '@/core/transaction.ts';
import { TransactionTemplate } from '@/models/transaction_template.ts';
import { LedgerDecimal, ledgerMoney } from '@/lib/ledger-display.ts';

import { isDefined } from '@/lib/common.ts';
import {
    isNoAvailableTemplate,
    getFirstShowingId,
    getLastShowingId
} from '@/lib/template.ts';

const props = defineProps<{
    f7route: Router.Route;
    f7router: Router.Router;
}>();

const { getMultiWeekdayLongNames, getMultiMonthdayShortNames } = useI18n();
const { showAlert, showToast, routeBackOnError } = useI18nUIComponents();

const transactionTemplatesStore = useTransactionTemplatesStore();
const accountsStore = useAccountsStore();
const categoriesStore = useTransactionCategoriesStore();

const templateType = ref<number>(TemplateType.Normal.type);
const loading = ref<boolean>(true);
const loadingError = ref<unknown | null>(null);
const showHidden = ref<boolean>(false);
const sortable = ref<boolean>(false);
const templateToDelete = ref<TransactionTemplate | null>(null);
const activeTemplate = ref<TransactionTemplate | null>(null);
const showMoreActionSheet = ref<boolean>(false);
const showItemActionSheet = ref<boolean>(false);
const showDeleteActionSheet = ref<boolean>(false);
const displayOrderModified = ref<boolean>(false);
const displayOrderSaving = ref<boolean>(false);

const templates = computed<TransactionTemplate[]>(() => transactionTemplatesStore.allTransactionTemplates[templateType.value] || []);
const firstShowingId = computed<string | null>(() => getFirstShowingId(templates.value, showHidden.value));
const lastShowingId = computed<string | null>(() => getLastShowingId(templates.value, showHidden.value));
const noAvailableTemplate = computed<boolean>(() => isNoAvailableTemplate(templates.value, showHidden.value));

function templateAmount(t: TransactionTemplate): string {
    if (!t.sourceAmount) return '';
    const formatted = ledgerMoney(new LedgerDecimal(t.sourceAmount).div(100).toString(), false);
    if (t.type === TransactionType.Expense) return `-¥${formatted}`;
    if (t.type === TransactionType.Income) return `+¥${formatted}`;
    return `¥${formatted}`;
}

function templateAmountClass(t: TransactionTemplate): string {
    if (t.type === TransactionType.Expense) return 'cy-expense';
    if (t.type === TransactionType.Income) return 'cy-income';
    return '';
}

function scheduleSummary(t: TransactionTemplate): string {
    if (t.templateType !== TemplateType.Schedule.type) return '';
    if (t.scheduledFrequencyType === ScheduledTemplateFrequencyType.Disabled.type) return '未启用周期';
    if (t.scheduledFrequencyType === ScheduledTemplateFrequencyType.Daily.type) return '每天自动记账';
    const vals = (t.scheduledFrequency || '').split(',').filter(Boolean);
    if (t.scheduledFrequencyType === ScheduledTemplateFrequencyType.Weekly.type && vals.length) {
        return `每周 ${getMultiWeekdayLongNames(vals.map(Number))}`;
    }
    if (t.scheduledFrequencyType === ScheduledTemplateFrequencyType.Monthly.type && vals.length) {
        return `每月 ${getMultiMonthdayShortNames(vals.map(Number))}`;
    }
    if (t.scheduledFrequencyType === ScheduledTemplateFrequencyType.Yearly.type && vals.length) {
        return `每年 ${vals.length} 个日期`;
    }
    return '';
}

function templateSubtitle(t: TransactionTemplate): string {
    const parts: string[] = [];
    const catId = t.type === TransactionType.Expense ? t.expenseCategoryId : t.type === TransactionType.Income ? t.incomeCategoryId : t.transferCategoryId;
    const cat = catId ? categoriesStore.allTransactionCategoriesMap[catId]?.name : '';
    const acc = t.sourceAccountId ? accountsStore.allAccountsMap[t.sourceAccountId]?.name : '';
    const freq = scheduleSummary(t);
    if (freq) parts.push(freq);
    if (cat) parts.push(cat);
    if (acc) parts.push(acc);
    if (t.comment) parts.push(t.comment);
    return parts.join(' · ');
}

function onTemplateClick(template: TransactionTemplate): void {
    if (sortable.value) return;
    if (templateType.value === TemplateType.Normal.type) {
        useTemplate(template);
    } else {
        edit(template);
    }
}

function useTemplate(template: TransactionTemplate): void {
    showItemActionSheet.value = false;
    props.f7router.navigate(`/transaction/add?templateId=${template.id}`);
}

function openTemplateMenu(template: TransactionTemplate): void {
    if (sortable.value) return;
    activeTemplate.value = template;
    showItemActionSheet.value = true;
}

function getTemplateDomId(template: TransactionTemplate): string {
    return 'template_' + template.id;
}

function parseTemplateIdFromDomId(domId: string): string | null {
    if (!domId || domId.indexOf('template_') !== 0) {
        return null;
    }

    return domId.substring(9); // template_
}

function init(): void {
    if (props.f7route.path === '/template/list') {
        templateType.value = TemplateType.Normal.type;
    } else if (props.f7route.path === '/schedule/list') {
        templateType.value = TemplateType.Schedule.type;
    }

    loading.value = true;

    Promise.all([
        transactionTemplatesStore.loadAllTemplates({ templateType: templateType.value, force: false }),
        accountsStore.loadAllAccounts({ force: false }).catch(() => undefined),
        categoriesStore.loadAllCategories({ force: false }).catch(() => undefined)
    ]).then(() => {
        loading.value = false;
    }).catch(error => {
        if (error.processed) {
            loading.value = false;
        } else {
            loadingError.value = error;
            showToast(error.message || error);
        }
    });
}

function reload(done?: () => void): void {
    if (sortable.value) {
        done?.();
        return;
    }

    const force = !!done;

    transactionTemplatesStore.loadAllTemplates({
        templateType: templateType.value,
        force: force
    }).then(() => {
        done?.();

        if (force) {
            showToast('Template list has been updated');
        }
    }).catch(error => {
        done?.();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function edit(template: TransactionTemplate): void {
    props.f7router.navigate(`/template/edit?id=${template.id}&templateType=${template.templateType}`);
}

function hide(template: TransactionTemplate, hidden: boolean): void {
    showItemActionSheet.value = false;
    showLoading();

    transactionTemplatesStore.hideTemplate({
        template: template,
        hidden: hidden
    }).then(() => {
        hideLoading();
        showToast(hidden ? (templateType.value === TemplateType.Schedule.type ? '已暂停该周期记账' : '模板已隐藏') : '已恢复显示');
    }).catch(error => {
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function remove(template: TransactionTemplate | null, confirm: boolean): void {
    if (!template) {
        showAlert('发生错误');
        return;
    }

    if (!confirm) {
        showItemActionSheet.value = false;
        templateToDelete.value = template;
        showDeleteActionSheet.value = true;
        return;
    }

    showDeleteActionSheet.value = false;
    templateToDelete.value = null;
    showLoading();

    transactionTemplatesStore.deleteTemplate({
        template: template
    }).then(() => {
        hideLoading();
        showToast('已删除');
    }).catch(error => {
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function setSortable(): void {
    if (sortable.value) {
        return;
    }

    showHidden.value = true;
    sortable.value = true;
    displayOrderModified.value = false;
}

function saveSortResult(): void {
    if (!displayOrderModified.value) {
        showHidden.value = false;
        sortable.value = false;
        return;
    }

    displayOrderSaving.value = true;
    showLoading();

    transactionTemplatesStore.updateTemplateDisplayOrders({
        templateType: templateType.value
    }).then(() => {
        displayOrderSaving.value = false;
        hideLoading();

        showHidden.value = false;
        sortable.value = false;
        displayOrderModified.value = false;
    }).catch(error => {
        displayOrderSaving.value = false;
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function cancelSort(): void {
    if (!displayOrderModified.value) {
        showHidden.value = false;
        sortable.value = false;
        return;
    }

    displayOrderSaving.value = true;
    showLoading();

    transactionTemplatesStore.loadAllTemplates({
        templateType: templateType.value,
        force: false
    }).then(() => {
        displayOrderSaving.value = false;
        hideLoading();

        showHidden.value = false;
        sortable.value = false;
        displayOrderModified.value = false;
    }).catch(error => {
        displayOrderSaving.value = false;
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function onSort(event: { el: { id: string }; from: number; to: number }): void {
    if (!event || !event.el || !event.el.id) {
        showToast('Unable to move template');
        return;
    }

    const id = parseTemplateIdFromDomId(event.el.id);

    if (!id) {
        showToast('Unable to move template');
        return;
    }

    transactionTemplatesStore.changeTemplateDisplayOrder({
        templateType: templateType.value,
        templateId: id,
        from: event.from,
        to: event.to
    }).then(() => {
        displayOrderModified.value = true;
    }).catch(error => {
        showToast(error.message || error);
    });
}

function onPageAfterIn(): void {
    if ((!isDefined(transactionTemplatesStore.transactionTemplateListStatesInvalid[templateType.value]) || transactionTemplatesStore.transactionTemplateListStatesInvalid[templateType.value]) && !loading.value) {
        reload();
    }

    routeBackOnError(props.f7router, loadingError);
}

init();
</script>

<style>
.template-list {
    --f7-list-item-footer-font-size: var(--ebk-large-footer-font-size);
}

.template-list .item-footer {
    padding-top: 4px;
}

.template-amount {
    font-weight: 600;
    font-size: 14px;
    margin-inline-end: 6px;
}

.template-more-btn {
    border: 0;
    background: transparent;
    color: var(--cy-muted);
    padding: 6px 2px 6px 8px;
    display: inline-flex;
    align-items: center;
}
</style>
