import { nextTick } from 'vue';
import { isEnableAnimate } from '@/lib/settings.ts';

export const MOTION_EASE_OUT = 'cubic-bezier(0.22, 1, 0.36, 1)';
export const MOTION_EASE_SHAKE = 'cubic-bezier(0.36, 0.07, 0.19, 0.97)';

let reducedMotionQuery: MediaQueryList | null = null;

function getReducedMotionQuery(): MediaQueryList | null {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
        return null;
    }

    reducedMotionQuery ??= window.matchMedia('(prefers-reduced-motion: reduce)');
    return reducedMotionQuery;
}

export function prefersReducedMotion(): boolean {
    return !!getReducedMotionQuery()?.matches;
}

export function shouldAnimate(enabledSetting?: boolean): boolean {
    const allowed = enabledSetting ?? isEnableAnimate();
    return allowed && !prefersReducedMotion();
}

export function syncMotionDocumentFlag(enabledSetting?: boolean): void {
    if (typeof document === 'undefined') {
        return;
    }

    document.documentElement.dataset['reducedMotion'] = shouldAnimate(enabledSetting) ? 'false' : 'true';
}

export function initMotionPreferences(getEnabledSetting?: () => boolean): () => void {
    const sync = () => syncMotionDocumentFlag(getEnabledSetting?.());
    sync();

    const query = getReducedMotionQuery();
    if (!query) {
        return () => {};
    }

    if (typeof query.addEventListener === 'function') {
        query.addEventListener('change', sync);
        return () => query.removeEventListener('change', sync);
    }

    return () => {};
}

function cancelAnimationsById(el: Element, id: string): void {
    if (typeof el.getAnimations !== 'function') {
        return;
    }

    for (const anim of el.getAnimations()) {
        if (anim.id === id) {
            anim.cancel();
        }
    }
}

export function animateSettle(
    el: HTMLElement | null | undefined,
    opts?: { x?: number; y?: number; scale?: number; duration?: number; opacity?: number }
): void {
    if (!el || !shouldAnimate() || typeof el.animate !== 'function') {
        return;
    }

    const x = opts?.x ?? 0;
    const y = opts?.y ?? 0;
    const scale = opts?.scale ?? 1;
    const opacity = opts?.opacity ?? 0.65;
    const duration = opts?.duration ?? 170;

    cancelAnimationsById(el, 'cy-settle');
    el.animate(
        [
            { opacity, transform: `translate3d(${x}px, ${y}px, 0) scale(${scale})` },
            { opacity: 1, transform: 'translate3d(0, 0, 0) scale(1)' }
        ],
        {
            id: 'cy-settle',
            duration,
            easing: MOTION_EASE_OUT
        }
    );
}

export function animateShake(el: HTMLElement | null | undefined): void {
    if (!el || typeof el.animate !== 'function') {
        return;
    }

    cancelAnimationsById(el, 'cy-shake');

    if (!shouldAnimate()) {
        el.animate(
            [{ opacity: 0.62 }, { opacity: 1 }],
            { id: 'cy-shake', duration: 150, easing: 'ease-out' }
        );
        return;
    }

    el.animate(
        [
            { transform: 'translate3d(0, 0, 0)' },
            { transform: 'translate3d(-6px, 0, 0)', offset: 0.18 },
            { transform: 'translate3d(5px, 0, 0)', offset: 0.38 },
            { transform: 'translate3d(-3.5px, 0, 0)', offset: 0.58 },
            { transform: 'translate3d(2px, 0, 0)', offset: 0.78 },
            { transform: 'translate3d(0, 0, 0)' }
        ],
        {
            id: 'cy-shake',
            duration: 260,
            easing: MOTION_EASE_SHAKE
        }
    );
}

export function animateFlipSwap(
    container: HTMLElement | null | undefined,
    mutate: () => void,
    selector = ':scope > *'
): void {
    if (!container || !shouldAnimate()) {
        mutate();
        return;
    }

    const beforeNodes = Array.from(container.querySelectorAll<HTMLElement>(selector));
    const beforeTops = new Map<HTMLElement, number>();
    for (const node of beforeNodes) {
        beforeTops.set(node, node.getBoundingClientRect().top);
    }

    mutate();

    void nextTick(() => {
        if (!shouldAnimate()) {
            return;
        }

        const afterNodes = Array.from(container.querySelectorAll<HTMLElement>(selector));
        for (const node of afterNodes) {
            const oldTop = beforeTops.get(node);
            if (oldTop === undefined || typeof node.animate !== 'function') {
                continue;
            }
            const deltaY = oldTop - node.getBoundingClientRect().top;
            if (Math.abs(deltaY) < 1) {
                continue;
            }
            cancelAnimationsById(node, 'cy-flip');
            node.animate(
                [
                    { transform: `translate3d(0, ${deltaY}px, 0)` },
                    { transform: 'translate3d(0, 0, 0)' }
                ],
                {
                    id: 'cy-flip',
                    duration: 200,
                    easing: MOTION_EASE_OUT
                }
            );
        }
    });
}
