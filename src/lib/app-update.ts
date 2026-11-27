import { computed, ref } from 'vue';
import { getClientVersionInfo } from '@/lib/version.ts';

export interface NativeAppInfo {
    versionName: string;
    versionCode: number;
    applicationId: string;
    isQa: boolean;
    isNative: boolean;
}

export type AppUpdateStatus =
    | 'idle'
    | 'checking'
    | 'up_to_date'
    | 'available'
    | 'downloading'
    | 'verifying'
    | 'pending_permission'
    | 'installing'
    | 'error';

export interface AppUpdateState {
    status: AppUpdateStatus;
    hasUpdate: boolean;
    currentVersion: string;
    latestTag: string;
    latestVersion: string;
    releaseName: string;
    releaseNotes: string;
    publishedAt: string;
    releaseUrl: string;
    apkSize: number;
    downloadedBytes: number;
    progress: number;
    error: string;
    ignoredTag: string;
    lastCheckedAt: number;
}

const DEFAULT_RELEASES_URL = 'https://github.com/cheng-yi-cc/OpenBill/releases';

function defaultAppInfo(): NativeAppInfo {
    return {
        versionName: getClientVersionInfo().version || '0.3.0',
        versionCode: 0,
        applicationId: '',
        isQa: false,
        isNative: false
    };
}

function defaultUpdateState(): AppUpdateState {
    return {
        status: 'idle',
        hasUpdate: false,
        currentVersion: getClientVersionInfo().version || '0.3.0',
        latestTag: '',
        latestVersion: '',
        releaseName: '',
        releaseNotes: '',
        publishedAt: '',
        releaseUrl: DEFAULT_RELEASES_URL,
        apkSize: 0,
        downloadedBytes: 0,
        progress: 0,
        error: '',
        ignoredTag: '',
        lastCheckedAt: 0
    };
}

function readNativeAppInfo(): NativeAppInfo {
    try {
        const raw = window.CYLedgerLocal?.appInfo?.();
        if (raw) {
            return { ...defaultAppInfo(), ...(JSON.parse(raw) as Partial<NativeAppInfo>) };
        }
    } catch {
        // Fallback to build-time version info.
    }
    return defaultAppInfo();
}

function readNativeUpdateState(): AppUpdateState {
    try {
        const raw = window.CYLedgerLocal?.updateState?.();
        if (raw) {
            return { ...defaultUpdateState(), ...(JSON.parse(raw) as Partial<AppUpdateState>) };
        }
    } catch {
        // Fallback to idle update state.
    }
    return defaultUpdateState();
}

export const appInfo = ref<NativeAppInfo>(readNativeAppInfo());
export const updateState = ref<AppUpdateState>(readNativeUpdateState());

export const hasVisibleUpdate = computed<boolean>(() =>
    updateState.value.hasUpdate
    && !!updateState.value.latestTag
    && updateState.value.latestTag !== updateState.value.ignoredTag
);

let initialized = false;

export function initAppUpdater(): void {
    appInfo.value = readNativeAppInfo();
    updateState.value = readNativeUpdateState();
    if (initialized || typeof window === 'undefined') return;
    initialized = true;
    window.addEventListener('cyledger-update-state', (event: Event) => {
        const detail = (event as CustomEvent<Partial<AppUpdateState> | undefined>).detail;
        if (detail && typeof detail === 'object') {
            updateState.value = { ...defaultUpdateState(), ...detail };
        } else {
            updateState.value = readNativeUpdateState();
        }
    });
}

export function checkAppUpdate(manual: boolean): void {
    initAppUpdater();
    window.CYLedgerLocal?.checkUpdate?.(manual);
}

export function downloadAndInstallAppUpdate(): void {
    initAppUpdater();
    window.CYLedgerLocal?.downloadAndInstallUpdate?.();
}

export function cancelAppUpdateDownload(): void {
    window.CYLedgerLocal?.cancelUpdateDownload?.();
}

export function ignoreAppUpdate(tag: string): void {
    if (!tag) return;
    window.CYLedgerLocal?.ignoreUpdate?.(tag);
}

export function openAppReleasePage(url?: string): void {
    const target = url || updateState.value.releaseUrl || DEFAULT_RELEASES_URL;
    if (window.CYLedgerLocal?.openReleasePage) {
        window.CYLedgerLocal.openReleasePage(target);
        return;
    }
    window.open(target, '_blank', 'noopener,noreferrer');
}

export function formatUpdateBytes(bytes: number): string {
    if (!Number.isFinite(bytes) || bytes <= 0) return '';
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
