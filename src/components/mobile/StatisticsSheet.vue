<template>
    <f7-sheet class="cy-stat-sheet cy-mobile-surface" :opened="open" :style="{height: tall ? '88%' : 'auto'}" :swipe-to-close="!dirty" swipe-handler=".cy-stat-sheet-bar" :close-by-backdrop-click="!dirty" @sheet:closed="onClosed">
        <div class="cy-stat-sheet-bar"><button type="button" @click="requestClose">{{ closeLabel || (dirty ? '取消' : '关闭') }}</button><strong>{{ title }}</strong><slot name="action" /></div>
        <div ref="bodyEl" class="cy-stat-sheet-body" :class="{ 'cy-stat-sheet-tall': tall }"><slot /></div>
    </f7-sheet>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import { animateSettle } from '@/lib/motion.ts';
const props = defineProps<{ open: boolean; title: string; compact?: boolean; tall?: boolean; dirty?: boolean; closeLabel?: string }>();
const emit = defineEmits<{ 'update:open': [value: boolean] }>();
const { showConfirm } = useI18nUIComponents();
const bodyEl = ref<HTMLElement>();
watch(() => props.title, () => {
    if (props.open) animateSettle(bodyEl.value, { y: 6, duration: 160, opacity: 0.72 });
});
function requestClose(): void {
    if (props.dirty) {
        showConfirm('有尚未保存的修改，确定放弃吗？', () => emit('update:open', false), undefined, { title: '放弃修改', confirmText: '放弃', cancelText: '继续编辑', destructive: true, raw: true });
        return;
    }
    emit('update:open', false);
}
function onClosed(): void {
    if (props.open) emit('update:open', false);
}
</script>
<style scoped>
.cy-stat-sheet{background:var(--cy-card);color:var(--cy-ink);border-radius:18px 18px 0 0;max-height:88%}.cy-stat-sheet-bar{position:relative;height:52px;display:flex;gap:12px;align-items:center;justify-content:space-between;padding:0 18px;border-bottom:1px solid var(--cy-line)}.cy-stat-sheet-bar strong{position:absolute;left:50%;transform:translateX(-50%);max-width:60%;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.cy-stat-sheet-bar :deep(button){border:0;background:none;color:var(--cy-accent);font:inherit;padding:5px 0}.cy-stat-sheet-body{padding:18px;overflow-y:auto;max-height:calc(88vh - 52px);box-sizing:border-box;padding-bottom:calc(30px + var(--f7-safe-area-bottom, env(safe-area-inset-bottom)))}.cy-stat-sheet-tall{height:calc(100% - 52px)}
</style>
