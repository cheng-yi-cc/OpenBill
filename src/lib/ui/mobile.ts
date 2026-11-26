import { type Ref, watch } from 'vue';
import { f7, f7ready } from 'framework7-vue';
import type { Dialog, ColorPicker, Router, Toast } from 'framework7/types';

import { useI18n } from '@/locales/helpers.ts';
import { isNativePersonalMode } from '@/lib/native.ts';
import { formatAppError } from '@/lib/error-message.ts';

let activeToast: Toast.Toast | null = null;
const backOverlayHandlers: (() => boolean)[] = [];

export function registerBackOverlayHandler(handler: () => boolean): () => void {
    backOverlayHandlers.push(handler);
    return () => {
        const index = backOverlayHandlers.lastIndexOf(handler);
        if (index >= 0) backOverlayHandlers.splice(index, 1);
    };
}

export function runBackOverlayHandlers(): boolean {
    for (let i = backOverlayHandlers.length - 1; i >= 0; i--) {
        if (backOverlayHandlers[i]?.()) return true;
    }
    return false;
}

export interface ConfirmDialogOptions {
    title?: string;
    confirmText?: string;
    cancelText?: string;
    destructive?: boolean;
    raw?: boolean;
}

import type { ColorValue } from '@/core/color.ts';
import { TextDirection } from '@/core/text.ts';
import { FontSize, FONT_SIZE_PREVIEW_CLASSNAME_PREFIX } from '@/core/font.ts';

import { isEnableAnimate } from '../settings.ts';

export interface Framework7Dom {
    length: number;
    [index: number]: Element;
    find: (selector?: string) => Framework7Dom;
    offset(): { top: number; left: number };
    scrollTop(position: number, duration?: number, callback?: () => void): Framework7Dom;
    outerHeight(includeMargin?: boolean): number;
    css(property: string): string | number;
}

export function isiOS(): boolean {
    return ((/iphone|ipod|ipad/gi).test(navigator.platform) && (/Safari/i).test(navigator.appVersion));
}

export function isiOSHomeScreenMode(): boolean {
    return isiOS() && !!window.matchMedia && window.matchMedia('(display-mode: standalone)').matches;
}

export function showLoading(delayConditionFunc?: () => boolean, delayMills?: number): void {
    if (!delayConditionFunc) {
        f7ready((f7) => {
            return f7.preloader.show();
        });
        return;
    }

    f7ready((f7) => {
        setTimeout(() => {
            if (delayConditionFunc()) {
                f7.preloader.show();
            }
        }, delayMills || 200);
    });
}

export function hideLoading(): void {
    f7ready((f7) => {
        return f7.preloader.hide();
    });
}

export function openPopover(selector: string, targetEl: HTMLElement): void {
    f7ready((f7) => {
        f7.popover.open(selector, targetEl);
    });
}

export function closePopover(selector: string): void {
    f7ready((f7) => {
        const popover = f7.popover.get(selector);

        if (popover) {
            popover.close();
        }
    });
}

export function closeAllDialog(): void {
    f7ready((f7) => {
        return f7.dialog.close();
    });
}

export function createInlineColorPicker(containerEl: HTMLElement, value: ColorValue, onChange: (value: ColorValue) => void): ColorPicker.ColorPicker {
    return f7.colorPicker.create({
        containerEl: containerEl,
        value: { hex: `#${value}` },
        modules: ['sb-spectrum', 'hue-slider', 'hex'],
        hexValueEditable: true,
        on: {
            // @ts-expect-error there is an "change" event in the ColorPicker module, but it is not declared in the type definition file
            change: (picker: ColorPicker.ColorPicker, color: ColorPicker.ColorPickerValue) => {
                if (color && color.hex) {
                    onChange(color.hex.replace(/^#/, '').substring(0, 6).toLowerCase());
                }
            }
        }
    });
}

export function isModalShowing(): number {
    return f7.$('.modal-in').length;
}

export function onSwipeoutDeleted(domId: string, callback: () => void): void {
    f7.swipeout.delete(f7.$('#' + domId), callback);
}

export function autoChangeTextareaSize(el: HTMLElement): void {
    f7.$(el).find('textarea').each((child: HTMLElement) => {
        child.scrollTop = 0;
        child.style.height = '';
        child.style.height = child.scrollHeight + 'px';
    });
}

export const FONT_SIZE_SCALES: readonly number[] = [0.9, 1, 1.08, 1.15, 1.22, 1.3, 1.38];
export const FONT_SIZE_LABELS: readonly string[] = ['较小', '标准', '偏大', '大号', '特大', '超大', '极大'];

export function getFontSizeScale(type: number): number {
    const idx = Math.max(0, Math.min(FONT_SIZE_SCALES.length - 1, Number.isFinite(type) ? Math.round(type) : 1));
    return FONT_SIZE_SCALES[idx] ?? 1;
}

export function setAppFontSize(type: number): void {
    const htmlElement = f7.$('html');
    const allFontSizes = FontSize.values();

    for (const fontSizeType of allFontSizes) {
        if (fontSizeType.type === type) {
            if (!htmlElement.hasClass(fontSizeType.className)) {
                htmlElement.addClass(fontSizeType.className);
            }
        } else {
            htmlElement.removeClass(fontSizeType.className);
        }
    }

    document.documentElement.style.setProperty('--cy-font-scale', String(getFontSizeScale(type)));
}

export function getFontSizePreviewClassName(type: number): string {
    const allFontSizes = FontSize.values();

    for (const fontSizeType of allFontSizes) {
        if (fontSizeType.type === type) {
            return FONT_SIZE_PREVIEW_CLASSNAME_PREFIX + fontSizeType.className;
        }
    }

    return FONT_SIZE_PREVIEW_CLASSNAME_PREFIX + FontSize.Default.className;
}

export function getElementActualHeights(selector: string): Record<string, number> {
    const elements = f7.$(selector);
    const heights: Record<string, number> = {};

    if (!elements || !elements.length) {
        return heights;
    }

    for (const el of elements) {
        const rect = el.getBoundingClientRect();
        heights[el.id] = rect.height;
    }

    return heights;
}

export function getElementBoundingRect(selector: string): DOMRect | null {
    const elements = f7.$(selector);

    if (!elements || !elements.length) {
        return null;
    }

    const el = elements[0];
    return el.getBoundingClientRect();
}

export function scrollSheetToTop(sheetElement: HTMLElement | undefined, windowNormalInnerHeight: number): void {
    if (!sheetElement) {
        return;
    }

    const sheetHeight = sheetElement.offsetHeight;

    if (sheetHeight < windowNormalInnerHeight) {
        setTimeout(() => {
            const windowNewInnerHeight = window.innerHeight;

            if (windowNewInnerHeight < windowNormalInnerHeight && sheetHeight < windowNewInnerHeight) {
                window.scrollTo({ top: windowNormalInnerHeight - sheetHeight - 24, behavior: "smooth" });
            }
        }, 300);
    }
}

export function onInfiniteScrolling(callback: (e: Event) => void): void {
    f7.$('.infinite-scroll-content').on('scroll', (e: Event) => {
        callback(e);
    }, {
        passive: true
    });
}

export function useI18nUIComponents() {
    const { tt, te, getCurrentLanguageTextDirection } = useI18n();

    function routeBackOnError<T>(f7router: Router.Router, errorRef: Ref<T>): void {
        const unwatch = watch(errorRef, (newValue) => {
            if (newValue) {
                setTimeout(() => {
                    if (unwatch) {
                        unwatch();
                    }

                    f7router.back();
                }, 200);
            }
        }, {
            immediate: true
        });
    }

    function safeMessage(message: string, raw?: boolean): string {
        if (!message) return '';
        // Avoid passing already-localized Chinese strings or dynamic user names
        // containing vue-i18n meta characters (|, @, {, }) through tt().
        if (raw || /[\u4e00-\u9fff]/.test(message)) return message;
        return te(message);
    }

    function showAlert(message: string, confirmCallback?: (dialog: Dialog.Dialog, e: Event) => void, options?: { title?: string; confirmText?: string; raw?: boolean }): void {
        const displayText = typeof message === 'string' ? safeMessage(message, options?.raw) : formatAppError(message, 'read');
        f7ready((f7) => {
            f7.dialog.create({
                title: options?.title ? safeMessage(options.title, options.raw) : tt('global.app.title'),
                text: displayText,
                animate: isEnableAnimate(),
                buttons: [
                    {
                        text: options?.confirmText ? safeMessage(options.confirmText, options.raw) : tt('OK'),
                        onClick: confirmCallback
                    }
                ]
            }).open();
        });
    }

    function showConfirm(message: string, confirmCallback?: (dialog: Dialog.Dialog, e: Event) => void, cancelCallback?: (dialog: Dialog.Dialog, e: Event) => void, options?: ConfirmDialogOptions): void {
        const textDirection = getCurrentLanguageTextDirection();

        const cancelButton: Dialog.DialogButton = {
            text: options?.cancelText ? safeMessage(options.cancelText, options.raw) : tt('Cancel'),
            cssClass: 'cy-dialog-cancel',
            onClick: cancelCallback
        };

        const confirmButton: Dialog.DialogButton = {
            text: options?.confirmText ? safeMessage(options.confirmText, options.raw) : tt('OK'),
            strong: true,
            color: options?.destructive ? 'red' : undefined,
            onClick: confirmCallback
        };

        f7ready((f7) => {
            f7.dialog.create({
                title: options?.title ? safeMessage(options.title, options.raw) : tt('global.app.title'),
                text: safeMessage(message, options?.raw),
                animate: isEnableAnimate(),
                buttons: textDirection == TextDirection.RTL ? [confirmButton, cancelButton] : [cancelButton, confirmButton]
            }).open();
        });
    }

    function showPrompt(message: string, currentValue?: string, confirmCallback?: (value: string, dialog: Dialog.Dialog, e: Event) => void, cancelCallback?: (value: string, dialog: Dialog.Dialog, e: Event) => void, options?: { title?: string; placeholder?: string; raw?: boolean }): void {
        const textDirection = getCurrentLanguageTextDirection();
        const label = safeMessage(message, options?.raw);

        const cancelButton: Dialog.DialogButton = {
            text: tt('Cancel'),
            cssClass: 'cy-dialog-cancel',
            onClick: (dialog, event) => {
                if (cancelCallback) {
                    const inputValue = dialog.$el.find('.dialog-input').val();
                    cancelCallback(inputValue, dialog, event);
                }
            }
        };

        const confirmButton: Dialog.DialogButton = {
            text: tt('OK'),
            strong: true,
            onClick: (dialog, event) => {
                if (confirmCallback) {
                    const inputValue = dialog.$el.find('.dialog-input').val();
                    confirmCallback(inputValue, dialog, event);
                }
            }
        };

        f7ready((f7) => {
            f7.dialog.create({
                title: options?.title ? safeMessage(options.title, options.raw) : tt('global.app.title'),
                text: label,
                content: '<div class="dialog-input-field input"><input type="text" class="dialog-input"></div>',
                animate: isEnableAnimate(),
                buttons: textDirection == TextDirection.RTL ? [confirmButton, cancelButton] : [cancelButton, confirmButton],
                on: {
                    open(dialog) {
                        const input = dialog.$el[0]?.querySelector<HTMLInputElement>('.dialog-input');
                        if (input) {
                            input.value = currentValue || '';
                            input.placeholder = options?.placeholder ? safeMessage(options.placeholder, options.raw) : label;
                            setTimeout(() => input.focus(), 60);
                        }
                    }
                }
            }).open();
        });
    }

    function showCancelableLoading(title: string, message: string, cancelButtonText?: string, cancelCallback?: (dialog: Dialog.Dialog, e: Event) => void): void {
        const cancelButton: Dialog.DialogButton = {
            text: cancelButtonText ? tt(cancelButtonText) : '',
            onClick: (dialog, event) => {
                if (cancelCallback) {
                    cancelCallback(dialog, event);
                }
            }
        };

        f7ready((f7) => {
            f7.dialog.create({
                title: tt(title),
                content: `<div class="preloader"><span class="preloader-inner">${[0, 1, 2, 3, 4, 5, 6, 7].map(() => '<span class="preloader-inner-line"></span>').join('')}</span></div>` + (message ? `<br/><div class="margin-top">${tt(message)}</div>` : ''),
                cssClass: 'dialog-preloader',
                animate: isEnableAnimate(),
                buttons: cancelButtonText ? [cancelButton] : []
            }).open();
        });
    }

    function showToast(message: string, timeout?: number): void {
        const text = typeof message === 'string' ? safeMessage(message) : formatAppError(message, 'read');
        if (!text) return;
        const duration = timeout || Math.min(4500, Math.max(2000, text.length * 110));
        f7ready((f7) => {
            if (activeToast) {
                try { activeToast.close(); } catch { /* ignore */ }
                activeToast = null;
            }
            const toast = f7.toast.create({
                text,
                position: 'bottom',
                cssClass: 'cy-toast-bottom',
                closeTimeout: duration,
                destroyOnClose: true,
                on: {
                    close() {
                        if (activeToast === toast) activeToast = null;
                    }
                }
            });
            activeToast = toast;
            toast.open();
        });
    }

    function openExternalUrl(url: string): void {
        if (isNativePersonalMode()) {
            window.open(url, '_blank');
            return;
        }
        const textDirection = getCurrentLanguageTextDirection();

        const cancelButton: Dialog.DialogButton = {
            text: tt('Cancel'),
            cssClass: 'cy-dialog-cancel'
        };

        const confirmButton: Dialog.DialogButton = {
            text: tt('OK'),
            strong: true,
            onClick: () => {
                window.open(url, '_blank');
            }
        };

        f7ready((f7) => {
            f7.dialog.create({
                title: tt('global.app.title'),
                text: tt('Are you sure you want to open this link?'),
                content: `<div style="word-break: break-all">${url}</div>`,
                animate: isEnableAnimate(),
                buttons: textDirection == TextDirection.RTL ? [confirmButton, cancelButton] : [cancelButton, confirmButton]
            }).open();
        });
    }

    return {
        showAlert: showAlert,
        showConfirm: showConfirm,
        showPrompt: showPrompt,
        showCancelableLoading: showCancelableLoading,
        showToast: showToast,
        openExternalUrl,
        routeBackOnError
    }
}
