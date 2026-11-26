<template>
    <f7-page class="cy-mobile-surface cy-asset-surface" @page:afterin="onPageAfterIn">
        <f7-navbar title="添加默认分类" back-link="返回">
            <f7-nav-right v-if="isPresetHasCategories">
                <f7-link :class="{ disabled: submitting || !selectedCount }" @click="save">添加{{ selectedCount ? `(${selectedCount})` : '' }}</f7-link>
            </f7-nav-right>
        </f7-navbar>

        <main class="management-body">
            <p class="management-caption">勾选需要补充的默认分类，已存在的同名分类已自动跳过以避免重复。</p>
            <div v-if="selectableKeys.length" class="preset-toolbar">
                <button type="button" class="cy-button" @click="selectAllMissing">全选未添加</button>
                <button type="button" class="cy-button" @click="selectedKeys = new Set()">清空选择</button>
            </div>
            <template v-for="(categories, catType) in allPresetCategories" :key="catType">
                <h2 class="preset-group-title">{{ getCategoryTypeName(parseInt(catType)) }}</h2>
                <section v-for="(category, idx) in categories" :key="`${catType}_${idx}`" class="management-card preset-card">
                    <label class="preset-parent">
                        <input type="checkbox" :checked="selectedKeys.has(presetKey(catType, category.name))" :disabled="isExisting(parseInt(catType), category.name)" @change="toggleKey(presetKey(catType, category.name), ($event.target as HTMLInputElement).checked)" />
                        <span class="management-icon"><ItemIcon :icon-type="getCategoryIconType(category.iconType)" :icon-id="category.icon" :color="category.color" /></span>
                        <span class="management-name">{{ category.name }}<small v-if="isExisting(parseInt(catType), category.name)">已存在</small><small v-else-if="category.subCategories.length">含 {{ category.subCategories.length }} 个二级分类</small></span>
                    </label>
                    <div v-if="category.subCategories.length" class="preset-children">
                        <span v-for="(sub, subIdx) in category.subCategories" :key="subIdx">{{ sub.name }}</span>
                    </div>
                </section>
            </template>
        </main>
    </f7-page>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import type { Router } from 'framework7/types';

import { useTransactionCategoriesStore } from '@/stores/transactionCategory.ts';
import { useI18n } from '@/locales/helpers.ts';
import { useI18nUIComponents, showLoading, hideLoading } from '@/lib/ui/mobile.ts';

import { type LocalizedPresetCategory, CategoryType } from '@/core/category.ts';
import { getObjectOwnFieldCount } from '@/lib/common.ts';
import { getCategoryIconType } from '@/lib/icon.ts';
import { localizedPresetCategoriesToTransactionCategoryCreateWithSubCategories } from '@/lib/category.ts';

const props = defineProps<{
    f7route: Router.Route;
    f7router: Router.Router;
}>();

const { getAllTransactionDefaultCategories } = useI18n();
const { showToast, routeBackOnError } = useI18nUIComponents();

const transactionCategoriesStore = useTransactionCategoriesStore();

const loadingError = ref<unknown | null>(null);
const categoryType = ref<number>(0);
const submitting = ref<boolean>(false);
const selectedKeys = ref<Set<string>>(new Set());

const allPresetCategories = computed<Record<string, LocalizedPresetCategory[]>>(() => getAllTransactionDefaultCategories(categoryType.value, 'zh-Hans'));
const isPresetHasCategories = computed<boolean>(() => getObjectOwnFieldCount(allPresetCategories.value) > 0);

function presetKey(catType: string | number, name: string): string {
    return `${catType}:${name}`;
}

function isExisting(catType: number, name: string): boolean {
    const existing = transactionCategoriesStore.allTransactionCategories[catType as CategoryType] || [];
    const normalized = name.trim().toLowerCase();
    return existing.some(item => item.name.trim().toLowerCase() === normalized);
}

const selectableKeys = computed<string[]>(() => {
    const keys: string[] = [];
    for (const [catType, list] of Object.entries(allPresetCategories.value)) {
        for (const cat of list) {
            if (!isExisting(parseInt(catType), cat.name)) {
                keys.push(presetKey(catType, cat.name));
            }
        }
    }
    return keys;
});

const selectedCount = computed<number>(() => selectedKeys.value.size);

function selectAllMissing(): void {
    selectedKeys.value = new Set(selectableKeys.value);
}

function toggleKey(key: string, checked: boolean): void {
    const next = new Set(selectedKeys.value);
    if (checked) next.add(key);
    else next.delete(key);
    selectedKeys.value = next;
}

function getCategoryTypeName(catType: number): string {
    switch (catType) {
        case CategoryType.Income: return '收入分类';
        case CategoryType.Expense: return '支出分类';
        case CategoryType.Transfer: return '转账分类';
        default: return '交易分类';
    }
}

function save(): void {
    if (submitting.value || !selectedCount.value) return;
    const router = props.f7router;
    const chosen: LocalizedPresetCategory[] = [];
    for (const [catType, list] of Object.entries(allPresetCategories.value)) {
        for (const cat of list) {
            if (selectedKeys.value.has(presetKey(catType, cat.name))) {
                chosen.push(cat);
            }
        }
    }
    if (!chosen.length) return;

    submitting.value = true;
    showLoading(() => submitting.value);
    const submitCategories = localizedPresetCategoriesToTransactionCategoryCreateWithSubCategories(chosen);

    transactionCategoriesStore.addPresetCategories({
        categories: submitCategories
    }).then(() => {
        submitting.value = false;
        hideLoading();
        showToast(`已添加 ${chosen.length} 组默认分类`);
        router.back();
    }).catch(error => {
        submitting.value = false;
        hideLoading();
        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function onPageAfterIn(): void {
    routeBackOnError(props.f7router, loadingError);
}

const query = props.f7route.query;
categoryType.value = parseInt(query['type'] || '0');

if (categoryType.value !== 0 &&
    categoryType.value !== CategoryType.Income &&
    categoryType.value !== CategoryType.Expense &&
    categoryType.value !== CategoryType.Transfer) {
    showToast('参数无效');
    loadingError.value = 'Parameter Invalid';
} else {
    void transactionCategoriesStore.loadAllCategories({ force: false }).finally(() => {
        selectAllMissing();
    });
}
</script>

<style scoped src="@/styles/mobile/management.css"></style>
<style scoped>
.preset-toolbar{display:flex;gap:10px;margin-bottom:14px}
.preset-group-title{font-size:13px;color:var(--cy-muted);font-weight:500;margin:16px 4px 10px}
.preset-card{padding:12px 14px}
.preset-parent{display:flex;align-items:center;gap:12px;cursor:pointer}
.preset-parent input{width:19px;height:19px;flex:none;accent-color:var(--cy-accent)}
.preset-children{display:flex;flex-wrap:wrap;gap:6px;margin:10px 0 2px 31px}
.preset-children span{padding:3px 8px;border-radius:8px;background:var(--cy-bg);color:var(--cy-muted);font-size:11px}
</style>
