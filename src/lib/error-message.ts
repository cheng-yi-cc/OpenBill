import axios from 'axios';
import { KnownErrorCode, PARAMETERIZED_ERRORS } from '@/consts/api.ts';
import { ALL_LANGUAGES, DEFAULT_LANGUAGE } from '@/locales/index.ts';
import { getSessionCurrentLanguageKey } from '@/lib/settings.ts';
import { isNativePersonalMode } from '@/lib/native.ts';

export type ErrorContext = 'read' | 'write' | 'quote';

type LocaleDict = Record<string, unknown>;

function currentLocaleDict(): LocaleDict {
    const key = getSessionCurrentLanguageKey() || DEFAULT_LANGUAGE;
    const entry = ALL_LANGUAGES[key] || ALL_LANGUAGES['zh-Hans'] || ALL_LANGUAGES[DEFAULT_LANGUAGE];
    return (entry?.content as LocaleDict | undefined) || {};
}

function lookupPath(dict: LocaleDict, path: string): string | undefined {
    if (typeof dict[path] === 'string') return dict[path] as string;
    const parts = path.split('.');
    let node: unknown = dict;
    for (const part of parts) {
        if (!node || typeof node !== 'object' || !(part in (node as LocaleDict))) return undefined;
        node = (node as LocaleDict)[part];
    }
    return typeof node === 'string' ? node : undefined;
}

function containsChinese(text: string): boolean {
    return /[\u4e00-\u9fff]/.test(text);
}

function translateApiErrorPayload(payload: { errorCode?: number; errorMessage?: string; error?: string }): string | null {
    const raw = (payload.errorMessage || payload.error || '').trim();
    if (!raw) return null;
    if (containsChinese(raw)) return raw;

    const dict = currentLocaleDict();

    if (payload.errorCode === KnownErrorCode.ValidatorError || /^parameter\s+"/.test(raw)) {
        for (const info of PARAMETERIZED_ERRORS) {
            const matches = raw.match(info.regex);
            if (matches && matches.length === info.parameters.length + 1) {
                let template = lookupPath(dict, `parameterizedError.${info.localeKey}`);
                if (template) {
                    info.parameters.forEach((param, idx) => {
                        const rawVal = matches[idx + 1] || '';
                        const val = param.localized ? (lookupPath(dict, `parameter.${rawVal}`) || '输入项') : rawVal;
                        template = template!.replace(`{${param.field}}`, val);
                    });
                    return template;
                }
            }
        }
        return '输入内容格式有误，请检查后重试';
    }

    const fromErrorDict = lookupPath(dict, `error.${raw}`);
    if (fromErrorDict) return fromErrorDict;

    const direct = lookupPath(dict, raw);
    if (direct) return direct;

    return null;
}

function translateRawMessage(raw: string, context: ErrorContext): string {
    const trimmed = raw.trim();
    if (!trimmed) return fallbackByContext(context);
    if (containsChinese(trimmed)) return trimmed;

    const dict = currentLocaleDict();
    const fromError = lookupPath(dict, `error.${trimmed}`);
    if (fromError) return fromError;
    const direct = lookupPath(dict, trimmed);
    if (direct) return direct;

    if (/timeout/i.test(trimmed) || trimmed === 'ECONNABORTED') {
        return context === 'quote' ? '获取行情超时，请稍后重试' : '请求超时，请稍后重试';
    }
    if (/network error|failed to fetch|load failed|err_network/i.test(trimmed)) {
        return fallbackByContext(context);
    }
    if (/^Request failed with status code (\d+)/i.test(trimmed)) {
        const status = Number(trimmed.match(/(\d+)/)?.[1] || 0);
        if (status === 409) return '内容刚有变动，请刷新后重试';
        if (status === 413) return '上传文件过大，请压缩后重试';
        if (status >= 500) return isNativePersonalMode() ? '本机账本暂时繁忙，请稍后重试' : '服务暂时繁忙，请稍后重试';
        return fallbackByContext(context);
    }
    if (/^parameter\s+"/.test(trimmed)) {
        const translated = translateApiErrorPayload({ errorCode: KnownErrorCode.ValidatorError, errorMessage: trimmed });
        if (translated) return translated;
    }

    return fallbackByContext(context);
}

function fallbackByContext(context: ErrorContext): string {
    if (context === 'quote') return '暂未获取到行情，请稍后重试或改用手动估值';
    if (context === 'read') return isNativePersonalMode() ? '本机账本加载失败，请稍后重试' : '数据加载失败，请稍后重试';
    return isNativePersonalMode()
        ? '本机账本暂时无响应，本次修改尚未保存，请重试'
        : '网络连接异常，请检查网络后重试';
}

export function formatAppError(error: unknown, context: ErrorContext = 'write'): string {
    if (!error) return fallbackByContext(context);

    if (typeof error === 'string') {
        return translateRawMessage(error, context);
    }

    if (axios.isAxiosError(error)) {
        const data = error.response?.data as { errorCode?: number; errorMessage?: string; error?: string } | undefined;
        if (data && typeof data === 'object') {
            const translated = translateApiErrorPayload(data);
            if (translated) return translated;
        }
        if (error.code === 'ECONNABORTED' || /timeout/i.test(error.message || '')) {
            return context === 'quote' ? '获取行情超时，请稍后重试' : '请求超时，请稍后重试';
        }
        if (!error.response) {
            return fallbackByContext(context);
        }
        return translateRawMessage(error.message || '', context);
    }

    if (error instanceof Error) {
        return translateRawMessage(error.message, context);
    }

    if (typeof error === 'object') {
        const obj = error as {
            error?: { errorCode?: number; errorMessage?: string; error?: string } | string;
            message?: string | { error?: { errorCode?: number; errorMessage?: string; error?: string } };
            errorMessage?: string;
            errorCode?: number;
        };
        if (obj.error && typeof obj.error === 'object') {
            const translated = translateApiErrorPayload(obj.error);
            if (translated) return translated;
        }
        if (obj.message && typeof obj.message === 'object' && obj.message.error) {
            const translated = translateApiErrorPayload(obj.message.error);
            if (translated) return translated;
        }
        if (typeof obj.errorMessage === 'string' && obj.errorMessage) {
            const translated = translateApiErrorPayload({ errorCode: obj.errorCode, errorMessage: obj.errorMessage });
            if (translated) return translated;
            return translateRawMessage(obj.errorMessage, context);
        }
        if (typeof obj.message === 'string' && obj.message) {
            return translateRawMessage(obj.message, context);
        }
        if (typeof obj.error === 'string' && obj.error) {
            return translateRawMessage(obj.error, context);
        }
    }

    return fallbackByContext(context);
}

export function isRevisionConflict(error: unknown): boolean {
    if (axios.isAxiosError(error)) {
        if (error.response?.status === 409) return true;
        const msg = String((error.response?.data as { errorMessage?: string } | undefined)?.errorMessage || '');
        return /内容已更新|内容已变更|刚有变动|版本/.test(msg);
    }
    const text = formatAppError(error, 'write');
    return /内容已更新|内容已变更|刚有变动/.test(text);
}
