<template>
    <f7-page class="cy-mobile-surface cy-asset-surface" @page:afterin="load">
        <f7-navbar title="标签分组" back-link="返回"><f7-nav-right><f7-link :class="{ disabled: busy }" @click="add">新增</f7-link><f7-link :class="{ disabled: busy || store.allTransactionTagGroups.length < 2 }" @click="orderError = ''; orderOpen = true">排序</f7-link></f7-nav-right></f7-navbar>
        <main class="management-body">
            <p v-if="error" class="cy-message" role="alert">{{ error }}</p>
            <section class="management-card"><div class="management-head"><span class="management-main"><f7-icon f7="folder" /><span class="management-name">默认分组<small>{{ count('0') }} 个标签</small></span></span></div></section>
            <section v-for="group in store.allTransactionTagGroups" :key="group.id" class="management-card"><div class="management-head"><button class="management-main" :disabled="busy" @click="rename(group)"><f7-icon f7="folder" /><span class="management-name">{{ group.name }}<small>{{ count(group.id) }} 个标签</small></span></button><button class="management-action" :aria-label="`${group.name}的更多操作`" :disabled="busy" @click="selected = group; actionsOpen = true"><f7-icon f7="text_alignleft" /></button></div></section>
        </main>
        <f7-actions :opened="actionsOpen" @actions:closed="actionsOpen = false"><f7-actions-group><f7-actions-label>{{ selected?.name }}</f7-actions-label><f7-actions-button @click="selected && openEditor(selected)">重命名分组</f7-actions-button><f7-actions-button color="red" :class="{ disabled: !!count(selected?.id || '') }" @click="confirmDelete">删除空分组</f7-actions-button><f7-actions-label v-if="count(selected?.id || '')">先将标签移至其他分组后再删除</f7-actions-label></f7-actions-group><f7-actions-group><f7-actions-button bold>取消</f7-actions-button></f7-actions-group></f7-actions>
        <StatisticsSheet v-model:open="editorOpen" :title="editingGroup ? '重命名分组' : '新增分组'" compact>
            <template #action><button :disabled="busy || !draftName.trim()" @click="submitEditor">保存</button></template>
            <p v-if="editError" class="cy-message" role="alert">{{ editError }}</p>
            <form class="management-form" @submit.prevent="submitEditor">
                <label class="management-field"><span>分组名称</span><input v-model="draftName" :disabled="busy" maxlength="64" placeholder="填写分组名称" aria-label="分组名称" /></label>
            </form>
        </StatisticsSheet>
        <ManagementOrderPopup v-model:open="orderOpen" title="标签分组排序" :items="store.allTransactionTagGroups" :busy="busy" :error="orderError" @save="saveOrder" />
    </f7-page>
</template>
<script setup lang="ts">
import { ref } from 'vue';
import ManagementOrderPopup from '@/components/mobile/ManagementOrderPopup.vue';
import StatisticsSheet from '@/components/mobile/StatisticsSheet.vue';
import { useTransactionTagsStore } from '@/stores/transactionTag.ts';
import { TransactionTagGroup } from '@/models/transaction_tag_group.ts';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import { useManagementFeedback } from '@/lib/use-management-feedback.ts';
const { errorText } = useManagementFeedback();
import services from '@/lib/services.ts';
const store = useTransactionTagsStore(), { showConfirm, showToast } = useI18nUIComponents();
const busy = ref(false), error = ref(''), editError = ref(''), orderError = ref(''), orderOpen = ref(false), actionsOpen = ref(false), editorOpen = ref(false), selected = ref<TransactionTagGroup>(), editingGroup = ref<TransactionTagGroup | null>(null), draftName = ref('');
function count(id: string) { return store.allTransactionTagsByGroupMap[id]?.length || 0; }
async function load() { try { await store.loadAllTags({ force: false }); } catch (cause) { error.value = errorText(cause); } }
function add() { openEditor(null); }
function rename(group: TransactionTagGroup) { openEditor(group); }
function openEditor(group: TransactionTagGroup | null) { editingGroup.value = group; draftName.value = group?.name || ''; editError.value = ''; editorOpen.value = true; }
async function submitEditor() {
    if (busy.value) return;
    const name = draftName.value.trim();
    if (!name) { editError.value = '请填写分组名称'; return; }
    const draft = editingGroup.value ? editingGroup.value.clone() : TransactionTagGroup.createNewTagGroup(name);
    draft.name = name;
    busy.value = true; editError.value = '';
    try { await store.saveTagGroup({ tagGroup: draft }); editorOpen.value = false; showToast(editingGroup.value ? '分组已重命名' : '已新增分组'); }
    catch (cause) { editError.value = errorText(cause); }
    finally { busy.value = false; }
}
function confirmDelete() { const group = selected.value; if (!group || count(group.id)) return; showConfirm(`确定删除空分组「${group.name}」？`, () => { void remove(group); }, undefined, { title: '删除分组', confirmText: '删除', destructive: true }); }
async function remove(tagGroup: TransactionTagGroup) { if (busy.value) return; busy.value = true; error.value = ''; try { await store.deleteTagGroup({ tagGroup }); showToast('分组已删除'); } catch (cause) { error.value = errorText(cause); } finally { busy.value = false; } }
async function saveOrder(ids: string[]) {
    if (busy.value) return; busy.value = true; orderError.value = '';
    try {
        const response = await services.moveTransactionTagGroup({ newDisplayOrders: ids.map((id, index) => ({ id, displayOrder: index + 1 })) });
        if (!response.data.success || !response.data.result) throw new Error('分组顺序保存失败，请重试');
        await store.loadAllTagGroups({ force: true }); orderOpen.value = false; showToast('分组顺序已保存');
    } catch (cause) { orderError.value = errorText(cause); }
    finally { busy.value = false; }
}
void load();
</script>
<style scoped src="@/styles/mobile/management.css"></style>
