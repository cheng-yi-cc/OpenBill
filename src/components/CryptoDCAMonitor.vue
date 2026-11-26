<template>
    <aside v-if="visible.length" class="dca-banner" role="status" aria-live="polite" @keydown.esc="dismiss">
        <header class="dca-banner-head">
            <strong>定投余额提醒</strong>
            <button type="button" class="dca-banner-close" aria-label="关闭提醒" @click="dismiss">知道了</button>
        </header>
        <div v-for="alert in visible" :key="alert.id" class="dca-banner-item">
            <div class="dca-banner-copy">
                <strong>{{ alert.heading }}</strong>
                <p>{{ alert.detail }}</p>
            </div>
            <button type="button" class="dca-banner-action" @click="goTo(alert)">去处理 →</button>
        </div>
    </aside>
</template>
<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue';
import { f7 } from 'framework7-vue';
import { cryptoDCAState, syncCryptoDCAOnOpen } from '@/lib/crypto-dca.ts';
import { fundDCAAlerts, syncInvestmentPlansOnOpen } from '@/lib/investment-mobile.ts';
import { buildUnifiedDCAAlerts, dispatchNativeDCAAlerts, type UnifiedDCAAlert } from '@/lib/dca-alerts.ts';
import { registerBackOverlayHandler } from '@/lib/ui/mobile.ts';
import { getCurrentUserInfo, isUserUnlocked } from '@/lib/userstate.ts';
import { useLedgerScopeStore } from '@/stores/ledgerScope.ts';
import { useUserStore } from '@/stores/user.ts';

const users = useUserStore();
const ledgerScope = useLedgerScopeStore();
const visible = ref<UnifiedDCAAlert[]>([]);
const seen = new Set<string>();
let timer: ReturnType<typeof setInterval> | undefined;
let unregisterBack: (() => void) | undefined;

function key(alert: UnifiedDCAAlert): string {
    return `cy-dca-reminder:${getCurrentUserInfo()?.username || ''}:${alert.id}:${alert.severity}:${alert.day}`;
}
function wasSeen(alert: UnifiedDCAAlert): boolean {
    if (seen.has(key(alert))) return true;
    try { return localStorage.getItem(key(alert)) === '1'; } catch { return false; }
}
function markSeen(alerts: UnifiedDCAAlert[]): void {
    for (const alert of alerts) {
        seen.add(key(alert));
        try { localStorage.setItem(key(alert), '1'); } catch { /* Ignore storage quota issues. */ }
    }
}
function dismiss(): void {
    markSeen(visible.value);
    visible.value = [];
}
function goTo(alert: UnifiedDCAAlert): void {
    markSeen([alert]);
    visible.value = visible.value.filter(item => item.id !== alert.id);
    if (alert.route) {
        f7?.views?.main?.router?.navigate(alert.route);
    }
}
function isUserTyping(): boolean {
    const el = document.activeElement;
    return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || (el as HTMLElement).isContentEditable);
}
function evaluateAlerts(): void {
    if (!isUserUnlocked()) {
        visible.value = [];
        return;
    }
    const unified = buildUnifiedDCAAlerts(cryptoDCAState.value?.alerts || [], fundDCAAlerts.value || [], ledgerScope.timeZone);
    if (dispatchNativeDCAAlerts(unified)) {
        visible.value = [];
        return;
    }
    const next = unified.filter(alert => !wasSeen(alert));
    if (!next.length) {
        visible.value = [];
        return;
    }
    if (isUserTyping() && !visible.value.length) return;
    visible.value = next;
}

watch([cryptoDCAState, fundDCAAlerts], evaluateAlerts);
watch(() => users.currentUserBasicInfo?.username, () => {
    visible.value = [];
    cryptoDCAState.value = undefined;
    fundDCAAlerts.value = [];
    tick();
});
function tick(): void {
    if (!isUserUnlocked()) {
        visible.value = [];
        return;
    }
    void syncCryptoDCAOnOpen();
    void syncInvestmentPlansOnOpen();
}
onMounted(() => {
    tick();
    timer = setInterval(tick, 60000);
    document.addEventListener('visibilitychange', tick);
    window.addEventListener('online', tick);
    window.addEventListener('cyledger-notification-permission', evaluateAlerts);
    unregisterBack = registerBackOverlayHandler(() => {
        if (!visible.value.length) return false;
        dismiss();
        return true;
    });
});
onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', tick);
    window.removeEventListener('online', tick);
    window.removeEventListener('cyledger-notification-permission', evaluateAlerts);
    unregisterBack?.();
});
</script>
<style scoped>
.dca-banner{position:fixed;top:calc(var(--f7-safe-area-top, 0px) + 10px);left:12px;right:12px;z-index:12500;max-width:540px;margin:0 auto;padding:14px 16px;border-radius:16px;background:var(--cy-card,#fff);color:var(--cy-ink,#242a30);border:1px solid var(--cy-line,#e5e9e7);box-shadow:0 12px 32px rgba(0,0,0,.18)}
.dca-banner-head{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:8px;font-size:14px}
.dca-banner-close{border:0;background:transparent;color:var(--cy-muted,#6b757c);font:inherit;font-size:13px;padding:4px 6px;cursor:pointer}
.dca-banner-item{display:flex;align-items:center;gap:12px;padding-top:8px;border-top:1px solid var(--cy-line,#edf0ef)}
.dca-banner-item:first-of-type{padding-top:2px;border-top:0}
.dca-banner-copy{flex:1;min-width:0}
.dca-banner-copy strong{display:block;font-size:13px;font-weight:600}
.dca-banner-copy p{margin:3px 0 0;font-size:12px;line-height:1.55;color:var(--cy-muted,#6b757c);overflow-wrap:anywhere}
.dca-banner-action{flex:none;border:0;border-radius:9px;background:var(--cy-accent,#087f72);color:#fff;font:inherit;font-size:12px;font-weight:500;padding:8px 12px;cursor:pointer}
.dark .dca-banner{background:#202e35;color:#e3ece9;border-color:#314148;box-shadow:0 14px 36px rgba(0,0,0,.45)}
.dark .dca-banner-action{color:#102725}
</style>
