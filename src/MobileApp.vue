<template>
    <f7-app v-bind="f7params">
        <f7-view id="main-view" main url="/"></f7-view>
        <f7-sheet v-model:opened="updateSheetOpen" class="cy-mobile-surface cy-update-sheet" style="height: auto" backdrop swipe-to-close>
            <div class="cy-update-sheet-body">
                <div class="cy-update-sheet-head">
                    <strong>发现新版本 {{ updateState.latestTag }}</strong>
                    <small v-if="updateState.apkSize > 0">{{ formatUpdateBytes(updateState.apkSize) }}</small>
                </div>
                <p v-if="updateState.releaseNotes" class="cy-update-sheet-notes">{{ updateState.releaseNotes }}</p>
                <p v-else class="cy-update-sheet-notes">已有新版本发布，升级将保留手机上的全部账本与备份数据。</p>
                <div v-if="updateState.status === 'downloading' || updateState.status === 'verifying'" class="cy-update-progress-wrap">
                    <div class="cy-update-progress-bar"><span :style="{ width: `${updateState.progress}%` }"></span></div>
                    <small>{{ updateState.status === 'verifying' ? '正在校验安装包完整性…' : `正在下载 ${updateState.progress}%` }}</small>
                </div>
                <p v-else-if="updateState.status === 'pending_permission'" class="cy-update-sheet-hint">请在系统设置中开启「允许安装应用」，返回后将自动继续安装。</p>
                <p v-else-if="updateState.status === 'installing'" class="cy-update-sheet-hint">正在启动系统安装界面…</p>
                <p v-else-if="updateState.status === 'error' && updateState.error" class="cy-update-sheet-error" role="alert">{{ updateState.error }}</p>
                <div class="cy-update-sheet-actions">
                    <button v-if="updateState.status === 'downloading' || updateState.status === 'verifying'" type="button" class="cy-button" @click="cancelAppUpdateDownload()">取消下载</button>
                    <button v-else type="button" class="cy-button cy-primary" :disabled="updateState.status === 'installing'" @click="downloadAndInstallAppUpdate()">{{ updateState.status === 'error' ? '重试下载并安装' : '立即更新' }}</button>
                    <div class="cy-update-sheet-subactions">
                        <button v-if="updateState.status === 'available' || updateState.status === 'error'" type="button" class="cy-button" @click="ignoreCurrentUpdate">忽略此版本</button>
                        <button v-if="updateState.status === 'error'" type="button" class="cy-button" @click="openAppReleasePage(updateState.releaseUrl)">浏览器打开</button>
                        <button type="button" class="cy-button" @click="updateSheetOpen = false">稍后再说</button>
                    </div>
                </div>
            </div>
        </f7-sheet>
    </f7-app>
	<CryptoDCAMonitor />
</template>

<script setup lang="ts">
import '@/styles/mobile/cyledger.scss';
import CryptoDCAMonitor from '@/components/CryptoDCAMonitor.vue';
import { syncCryptoDCAOnOpen } from '@/lib/crypto-dca.ts';
import { ref, computed, watch, onMounted, onUnmounted } from 'vue';
import moment from 'moment-timezone';

import type { Framework7Parameters, Notification, Actions, Dialog, Popover, Popup, Sheet } from 'framework7/types';
import { f7ready } from 'framework7-vue';
import routes from './router/mobile.ts';

import { useI18n } from '@/locales/helpers.ts';

import { useRootStore } from '@/stores/index.ts';
import { useSettingsStore } from '@/stores/setting.ts';
import { useLedgerExperienceStore } from '@/stores/ledgerExperience.ts';
import { useEnvironmentsStore } from '@/stores/environment.ts';
import { useUserStore } from '@/stores/user.ts';
import { useTokensStore } from '@/stores/token.ts';
import { useExchangeRatesStore } from '@/stores/exchangeRates.ts';

import { APPLICATION_LOGO_PATH } from '@/consts/asset.ts';
import { ThemeType } from '@/core/theme.ts';

import { isFunction } from '@/lib/common.ts';
import { syncNativeSettings, isNativePersonalMode, syncNativeAppearance } from '@/lib/native.ts';
import {
    appInfo,
    updateState,
    hasVisibleUpdate,
    initAppUpdater,
    checkAppUpdate,
    downloadAndInstallAppUpdate,
    cancelAppUpdateDownload,
    ignoreAppUpdate,
    openAppReleasePage,
    formatUpdateBytes
} from '@/lib/app-update.ts';
import {syncAssetAutomationOnOpen} from '@/lib/asset-tools.ts';
import {syncInvestmentPlansOnOpen} from '@/lib/investment-mobile.ts';
import { syncMonetaryIncomeOnOpen } from '@/lib/monetary-income.ts';
import { getTheme, isEnableSwipeBack } from '@/lib/settings.ts';
import { initMotionPreferences, shouldAnimate, syncMotionDocumentFlag } from '@/lib/motion.ts';
import { initMapProvider } from '@/lib/map/index.ts';
import { isUserLogined, isUserUnlocked } from '@/lib/userstate.ts';
import { updateMapCacheExpiration } from '@/lib/cache.ts';
import { setExpenseAndIncomeAmountColor } from '@/lib/ui/common.ts';
import { isiOSHomeScreenMode, isModalShowing, setAppFontSize, runBackOverlayHandlers } from '@/lib/ui/mobile.ts';

const { tt, getCurrentLanguageInfo, setLanguage, initLocale } = useI18n();
document.documentElement.classList.toggle('cy-native',isNativePersonalMode());

const rootStore = useRootStore();
const settingsStore = useSettingsStore();
const ledgerExperience = useLedgerExperienceStore();
watch(() => ledgerExperience.preferences, prefs => {
    const style = document.documentElement.style;
    style.setProperty('--cy-preference-accent', prefs.accent);
    style.setProperty('--cy-preference-income', prefs.colorScheme === 'monochrome' ? 'var(--cy-ink)' : prefs.colorScheme === 'green-expense' ? '#e65757' : '#269785');
    style.setProperty('--cy-preference-expense', prefs.colorScheme === 'monochrome' ? 'var(--cy-ink)' : prefs.colorScheme === 'green-expense' ? '#269785' : '#e65757');
    window.CYLedgerLocal?.preferences(JSON.stringify({exitConfirm:prefs.exitConfirm,hideRecents:prefs.hideRecents}));
}, { deep: true, immediate: true });
let nativeSettingsTimer:ReturnType<typeof setTimeout>|undefined;
watch(()=>[ledgerExperience.preferences,settingsStore.appSettings],()=>{clearTimeout(nativeSettingsTimer);nativeSettingsTimer=setTimeout(syncNativeSettings,500);},{deep:true});
const environmentsStore = useEnvironmentsStore();
const userStore = useUserStore();
const tokensStore = useTokensStore();
const exchangeRatesStore = useExchangeRatesStore();
let appearanceTimer: ReturnType<typeof setInterval> | undefined;
function applyAppearance(): void {
    const p=ledgerExperience.preferences,clock=moment().format('HH:mm');
    const night=p.nightStart===p.nightEnd?false:p.nightStart<p.nightEnd?clock>=p.nightStart&&clock<p.nightEnd:clock>=p.nightStart||clock<p.nightEnd;
    f7ready(app => {
        app.setDarkMode(p.nightSchedule ? night : settingsStore.appSettings.theme === 'auto' ? 'auto' : settingsStore.appSettings.theme === 'dark');
        // Framework7 only emits darkModeChange for system theme changes.
        environmentsStore.framework7DarkMode = app.darkMode;
    });
    syncNativeAppearance();
}
watch(()=>[settingsStore.appSettings.theme,ledgerExperience.preferences.nightSchedule,ledgerExperience.preferences.nightStart,ledgerExperience.preferences.nightEnd],applyAppearance);

const f7params = ref<Framework7Parameters>({
    name: 'OpenBill',
    theme: 'ios',
    colors: {
        primary: '#12786f'
    },
    routes: routes,
    darkMode: (() => {
        let darkMode: boolean | string = 'auto';

        if (getTheme() === ThemeType.Light) {
            darkMode = false;
        } else if (getTheme() === ThemeType.Dark) {
            darkMode = true;
        }

        return darkMode;
    })(),
    touch: {
        disableContextMenu: true,
        tapHold: true
    },
    actions: {
        animate: shouldAnimate(),
        backdrop: true,
        closeOnEscape: true
    },
    dialog: {
        buttonOk: '确定',
        buttonCancel: '取消',
        // @ts-expect-error there is an "animate" field in dialog parameters, but it is not declared in the type definition file
        animate: shouldAnimate(),
        backdrop: true
    },
    popover: {
        animate: shouldAnimate(),
        backdrop: true,
        closeOnEscape: true
    },
    popup: {
        animate: shouldAnimate(),
        backdrop: true,
        closeOnEscape: true,
        swipeToClose: true
    },
    sheet: {
        animate: shouldAnimate(),
        backdrop: true,
        closeOnEscape: true
    },
    smartSelect: {
        routableModals: false
    },
    view: {
        animate: shouldAnimate(),
        browserHistory: !isNativePersonalMode() && !isiOSHomeScreenMode(),
        browserHistoryInitialMatch: true,
        browserHistoryAnimate: false,
        iosSwipeBack: isEnableSwipeBack(),
        iosSwipeBackAnimateShadow: shouldAnimate(),
        mdSwipeBack: isEnableSwipeBack(),
        mdSwipeBackAnimateShadow: shouldAnimate()
    }
});

const notification = ref<Notification.Notification | null>(null);

const hasPushPopupBackdrop = ref<boolean | undefined>(undefined);
const hasBackdrop = ref<boolean | undefined>(undefined);
const currentNotificationContent = computed<string | null>(() => rootStore.currentNotification);

function setThemeColorMeta(darkMode: boolean | undefined): void {
    syncNativeAppearance();
    if (hasPushPopupBackdrop.value) {
        document.querySelector('meta[name=theme-color]')?.setAttribute('content', '#000');
        return;
    }

    if (darkMode) {
        if (hasBackdrop.value) {
            document.querySelector('meta[name=theme-color]')?.setAttribute('content', '#0b0b0b');
        } else {
            document.querySelector('meta[name=theme-color]')?.setAttribute('content', '#121212');
        }
    } else {
        if (hasBackdrop.value) {
            document.querySelector('meta[name=theme-color]')?.setAttribute('content', '#949495');
        } else {
            document.querySelector('meta[name=theme-color]')?.setAttribute('content', '#f6f6f8');
        }
    }
}

function onBackdropChanged(element: { push?: boolean, opened?: boolean }): void {
    if (element.push) {
        hasPushPopupBackdrop.value = element.opened;
    } else {
        hasBackdrop.value = element.opened;
    }

    setThemeColorMeta(environmentsStore.framework7DarkMode);
}

function resumeMonetaryIncome(): void { syncNativeSettings(); syncNativeAppearance();void syncMonetaryIncomeOnOpen();void syncCryptoDCAOnOpen();void syncInvestmentPlansOnOpen();void syncAssetAutomationOnOpen(settingsStore.appSettings.timeZone).catch(()=>{}); }
function onVisibilityChange(): void { if (!document.hidden) resumeMonetaryIncome(); }
let stopMotionObserver: (() => void) | undefined;
let updateCheckTimer: ReturnType<typeof setTimeout> | undefined;
const updateSheetOpen = ref(false);
let startupUpdatePrompted = false;
function ignoreCurrentUpdate(): void {
    if (updateState.value.latestTag) {
        ignoreAppUpdate(updateState.value.latestTag);
    }
    updateSheetOpen.value = false;
}
watch([hasVisibleUpdate, () => updateState.value.status], ([visible, status]) => {
    if (visible && status === 'available' && !startupUpdatePrompted) {
        startupUpdatePrompted = true;
        updateSheetOpen.value = true;
    }
});
onUnmounted(() => {
    stopMotionObserver?.();
    if (appearanceTimer) clearInterval(appearanceTimer);
    if (updateCheckTimer) clearTimeout(updateCheckTimer);
    document.removeEventListener('visibilitychange', onVisibilityChange);
    window.removeEventListener('online', resumeMonetaryIncome);
    delete (window as Window & { __openbillHandleBack?: () => boolean }).__openbillHandleBack;
});
onMounted(() => {
    initAppUpdater();
    if (isNativePersonalMode() && !appInfo.value.isQa) {
        updateCheckTimer = setTimeout(() => checkAppUpdate(false), 2500);
    }
    stopMotionObserver = initMotionPreferences(() => settingsStore.appSettings.animate);
    watch(() => settingsStore.appSettings.animate, enabled => {
        syncMotionDocumentFlag(enabled);
        const allow = shouldAnimate(enabled);
        f7ready(app => {
            if (app.params.view) {
                app.params.view.animate = allow;
                app.params.view.iosSwipeBackAnimateShadow = allow;
                app.params.view.mdSwipeBackAnimateShadow = allow;
            }
            if (app.params.sheet) app.params.sheet.animate = allow;
            if (app.params.popup) app.params.popup.animate = allow;
            if (app.params.popover) app.params.popover.animate = allow;
            if (app.params.actions) app.params.actions.animate = allow;
        });
    }, { immediate: true });
    syncNativeSettings();
    applyAppearance();appearanceTimer=setInterval(()=>{applyAppearance();void syncInvestmentPlansOnOpen();void syncMonetaryIncomeOnOpen();},60000);
    document.addEventListener('visibilitychange', onVisibilityChange);
    window.addEventListener('online', resumeMonetaryIncome);
    setAppFontSize(settingsStore.appSettings.fontSize);

    f7ready((f7) => {
        (window as Window & { __openbillHandleBack?: () => boolean }).__openbillHandleBack = (): boolean => {
            if (runBackOverlayHandlers()) return true;
            const modals = Array.from(document.querySelectorAll<HTMLElement>('.modal-in'));
            const top = modals.pop();
            if (top) {
                if (top.classList.contains('dialog')) {
                    const cancelBtn = top.querySelector<HTMLElement>('.cy-dialog-cancel');
                    if (cancelBtn) {
                        cancelBtn.click();
                        return true;
                    }
                    f7.dialog.close(top);
                    return true;
                }
                const api = (top as HTMLElement & { f7Modal?: { close?: () => void } }).f7Modal;
                if (api?.close) {
                    api.close();
                    return true;
                }
                if (top.classList.contains('sheet-modal')) { f7.sheet.close(top); return true; }
                if (top.classList.contains('popup')) { f7.popup.close(top); return true; }
                if (top.classList.contains('actions-modal')) { f7.actions.close(top); return true; }
                if (top.classList.contains('popover')) { f7.popover.close(top); return true; }
                const closeBtn = top.querySelector<HTMLElement>('.popup-close, .sheet-close');
                if (closeBtn) { closeBtn.click(); return true; }
            }
            const viewEl = document.querySelector<HTMLElement & { f7View?: { router: { history: string[]; currentRoute?: { path?: string }; back: () => void; navigate: (path: string, opts?: Record<string, unknown>) => void } } }>('.view-main');
            const router = viewEl?.f7View?.router;
            if (router) {
                if (router.history.length > 1) {
                    router.back();
                    return true;
                }
                const path = router.currentRoute?.path || '/';
                if (path !== '/' && path !== '/unlock' && path !== '/login') {
                    router.navigate('/', { animate: false, clearPreviousHistory: true });
                    return true;
                }
            }
            return false;
        };
        f7.on('pageAfterIn', resumeMonetaryIncome);
        void syncMonetaryIncomeOnOpen();void syncInvestmentPlansOnOpen();
        void syncAssetAutomationOnOpen(settingsStore.appSettings.timeZone).catch(()=>{});
        environmentsStore.framework7DarkMode = f7.darkMode;
        setThemeColorMeta(f7.darkMode);

        f7.on('actionsOpen', (actions: Actions.Actions) => onBackdropChanged(actions));
        f7.on('actionsClose', (actions: Actions.Actions) => onBackdropChanged(actions));
        f7.on('dialogOpen', (dialog: Dialog.Dialog) => onBackdropChanged(dialog));
        f7.on('dialogClose', (dialog: Dialog.Dialog) => onBackdropChanged(dialog));
        f7.on('popoverOpen', (popover: Popover.Popover) => onBackdropChanged(popover));
        f7.on('popoverClose', (popover: Popover.Popover) => onBackdropChanged(popover));
        f7.on('popupOpen', (popup: Popup.Popup) => onBackdropChanged(popup));
        f7.on('popupClose', (popup: Popup.Popup) => onBackdropChanged(popup));
        f7.on('sheetOpen', (sheet: Sheet.Sheet) => onBackdropChanged(sheet));
        f7.on('sheetClose', (sheet: Sheet.Sheet) => onBackdropChanged(sheet));

        f7.on('pageBeforeOut', () => {
            if (isModalShowing()) {
                f7.actions.close('.actions-modal.modal-in', false);
                f7.dialog.close('.dialog.modal-in', false);
                f7.popover.close('.popover.modal-in', false);
                f7.popup.close('.popup.modal-in', false);
                f7.sheet.close('.sheet-modal.modal-in', false);
            }
        });

        f7.on('darkModeChange', (darkMode) => {
            environmentsStore.framework7DarkMode = darkMode;
            setThemeColorMeta(darkMode);
        });
    });

    document.addEventListener('DOMContentLoaded', () => {
        const languageInfo = getCurrentLanguageInfo();
        initMapProvider(languageInfo?.alternativeLanguageTag);
    });

    document.addEventListener('dragstart', (e) => {
        if (!e.target || !('closest' in e.target) || !isFunction(e.target.closest) || !e.target.closest('.dragenabled')) {
            e.preventDefault();
        }
    }, true);
});

watch(currentNotificationContent, (newValue) => {
    if (notification.value) {
        notification.value.close();
        // @ts-expect-error there is an "destroy" function in the Notification component, but it is not defined in the type definition file
        // see https://framework7.io/docs/notification
        notification.value.destroy();
        notification.value = null;
    }

    if (newValue) {
        f7ready((f7) => {
            notification.value = f7.notification.create({
                icon: `<img alt="logo" src="${APPLICATION_LOGO_PATH}" />`,
                title: tt('global.app.title'),
                text: newValue,
                closeOnClick: true,
                on: {
                    close() {
                        rootStore.setNotificationContent(null);
                    }
                }
            }).open();
        });
    }
});

let localeDefaultSettings = initLocale(userStore.currentUserLanguage, settingsStore.appSettings.timeZone);
settingsStore.updateLocalizedDefaultSettings(localeDefaultSettings);

setExpenseAndIncomeAmountColor(userStore.currentUserExpenseAmountColor, userStore.currentUserIncomeAmountColor);

if (isUserLogined()) {
    if (!settingsStore.appSettings.applicationLock || isUserUnlocked()) {
        // refresh token if user is logined
        tokensStore.refreshTokenAndRevokeOldToken().then(response => {
            if (response.user) {
                localeDefaultSettings = setLanguage(response.user.language);
                settingsStore.updateLocalizedDefaultSettings(localeDefaultSettings);

                setExpenseAndIncomeAmountColor(response.user.expenseAmountColor, response.user.incomeAmountColor);

                if (response.notificationContent) {
                    rootStore.setNotificationContent(response.notificationContent);
                }
            }

            updateMapCacheExpiration(settingsStore.appSettings.mapCacheExpiration);
            exchangeRatesStore.removeExpiredExchangeRates(true);
            exchangeRatesStore.autoUpdateExchangeRatesData();
        });
    }
}
</script>

<style scoped>
.cy-update-sheet { border-radius: 18px 18px 0 0; }
.cy-update-sheet-body { padding: 18px 18px max(22px, var(--f7-safe-area-bottom, env(safe-area-inset-bottom))); display: grid; gap: 12px; color: var(--cy-ink); }
.cy-update-sheet-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.cy-update-sheet-head strong { font-size: 17px; font-weight: 600; }
.cy-update-sheet-head small { font-size: 12px; color: var(--cy-muted); }
.cy-update-sheet-notes { max-height: 180px; overflow-y: auto; white-space: pre-wrap; font-size: 13px; line-height: 1.65; color: var(--cy-muted); margin: 0; }
.cy-update-progress-wrap { display: grid; gap: 6px; }
.cy-update-progress-wrap small { font-size: 12px; color: var(--cy-muted); }
.cy-update-progress-bar { height: 6px; border-radius: 999px; background: var(--cy-soft); overflow: hidden; }
.cy-update-progress-bar span { display: block; height: 100%; background: var(--cy-accent); border-radius: 999px; }
.cy-update-sheet-hint { font-size: 13px; color: var(--cy-muted); line-height: 1.5; margin: 0; }
.cy-update-sheet-error { font-size: 13px; color: var(--cy-expense); line-height: 1.5; margin: 0; }
.cy-update-sheet-actions { display: grid; gap: 8px; margin-top: 4px; }
.cy-update-sheet-subactions { display: flex; gap: 8px; }
.cy-update-sheet-subactions button { flex: 1; }
</style>
