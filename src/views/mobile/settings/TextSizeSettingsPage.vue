<template>
    <f7-page class="cy-mobile-surface">
        <f7-navbar title="字体大小" back-link="返回">
            <f7-nav-right>
                <f7-link @click="setFontSize">保存</f7-link>
            </f7-nav-right>
        </f7-navbar>

        <main class="cy-page-body ts-body">
            <div class="ts-preview" :style="{ '--cy-font-scale': String(previewScale) }">
                <section class="cy-panel cy-hero ts-hero">
                    <p class="cy-muted">本月结余</p>
                    <p class="cy-major">¥8,426.50</p>
                    <div class="cy-hero-stats" style="margin-bottom:0;margin-top:12px">
                        <div><span class="cy-muted">本月支出</span><strong>¥4,373.50</strong></div>
                        <div><span class="cy-muted">本月收入</span><strong>¥12,800.00</strong></div>
                        <div><span class="cy-muted">剩余预算</span><strong>¥1,626.50</strong></div>
                    </div>
                </section>

                <section class="ts-day">
                    <header class="ts-day-head">
                        <span>{{ previewDate }}</span>
                        <span>收 <b class="cy-income">¥12,800.00</b> · 支 <b class="cy-expense">¥32.50</b></span>
                    </header>
                    <div class="ts-card">
                        <div class="ts-entry">
                            <span class="ts-icon"><f7-icon f7="cart" /></span>
                            <div class="ts-copy">
                                <strong>餐饮-午餐</strong>
                                <div class="ts-sub">
                                    <small class="ts-tag">工作日</small>
                                    <small class="ts-note">楼下简餐加咖啡</small>
                                </div>
                            </div>
                            <div class="ts-val">
                                <strong class="cy-expense">-¥32.50</strong>
                                <small>微信零钱</small>
                            </div>
                        </div>
                        <div class="ts-entry">
                            <span class="ts-icon"><f7-icon f7="money_dollar_circle" /></span>
                            <div class="ts-copy">
                                <strong>工资收入</strong>
                                <div class="ts-sub">
                                    <small class="ts-note">本月实发薪资</small>
                                </div>
                            </div>
                            <div class="ts-val">
                                <strong class="cy-income">+¥12,800.00</strong>
                                <small>招商银行</small>
                            </div>
                        </div>
                    </div>
                </section>
            </div>
            <p class="cy-muted" style="text-align:center">上方为真实首页账单排版效果，保存后全应用立即生效。</p>
        </main>

        <section class="ts-panel">
            <div class="ts-panel-head">
                <strong>{{ currentLabel }}</strong>
                <span class="cy-muted">{{ Math.round(previewScale * 100) }}%</span>
                <button v-if="fontSize !== FontSize.Default.type" type="button" class="ts-reset" @click="fontSize = FontSize.Default.type">恢复标准</button>
            </div>
            <div class="ts-scale-marks">
                <span class="ts-mark-min">A</span>
                <span class="ts-mark-std">标准</span>
                <span class="ts-mark-max">A</span>
            </div>
            <f7-range
                :min="FontSize.MinimumFontSize.type"
                :max="FontSize.MaximumFontSize.type"
                :step="1"
                :scale="true"
                :scale-steps="FontSize.MaximumFontSize.type"
                :scale-sub-steps="1"
                :format-scale-label="() => ''"
                v-model:value="fontSize"
                @range-change="fontSize = $event"
            />
        </section>
    </f7-page>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import moment from 'moment-timezone';
import type { Router } from 'framework7/types';

import { useSettingsStore } from '@/stores/setting.ts';
import { FontSize } from '@/core/font.ts';
import { setAppFontSize, getFontSizeScale, FONT_SIZE_LABELS, useI18nUIComponents } from '@/lib/ui/mobile.ts';

const props = defineProps<{
    f7router: Router.Router;
}>();

const settingsStore = useSettingsStore();
const { showToast } = useI18nUIComponents();

const fontSize = ref<number>(settingsStore.appSettings.fontSize);
const previewScale = computed<number>(() => getFontSizeScale(fontSize.value));
const currentLabel = computed<string>(() => FONT_SIZE_LABELS[fontSize.value] || '标准');
const previewDate = computed<string>(() => {
    const now = moment();
    return `${now.format('M月D日')}  今天 ${['周日', '周一', '周二', '周三', '周四', '周五', '周六'][now.day()]}`;
});

function setFontSize(): void {
    if (fontSize.value !== settingsStore.appSettings.fontSize) {
        settingsStore.setFontSize(fontSize.value);
        setAppFontSize(fontSize.value);
    }
    showToast(`已切换为「${currentLabel.value}」字号`);
    props.f7router.back();
}
</script>

<style scoped>
.ts-body{padding-bottom:190px!important}
.ts-hero{margin-bottom:14px!important}
.ts-day{margin-bottom:14px}
.ts-day-head{display:flex;justify-content:space-between;align-items:baseline;gap:8px;margin-bottom:7px;font-size:calc(12px * var(--cy-font-scale, 1));color:var(--cy-muted)}
.ts-day-head b{font-weight:500}
.ts-card{border-radius:12px;background:var(--cy-card);padding:4px 12px;border:1px solid var(--cy-line)}
.ts-entry{display:flex;align-items:center;gap:10px;padding:10px 0;min-height:54px}
.ts-entry+.ts-entry{border-top:1px solid var(--cy-line)}
.ts-icon{flex:0 0 28px;display:grid;place-items:center;color:var(--cy-muted)}.ts-icon .icon{font-size:22px}
.ts-copy{flex:1;min-width:0}
.ts-copy strong{display:block;font-weight:500;font-size:calc(15px * var(--cy-font-scale, 1));line-height:1.45;color:var(--cy-ink)}
.ts-sub{display:flex;align-items:center;gap:6px;margin-top:3px;min-width:0}
.ts-tag{background:var(--cy-soft);color:var(--cy-muted);border-radius:10px;padding:1px 6px;font-size:calc(10px * var(--cy-font-scale, 1));white-space:nowrap}
.ts-note{color:var(--cy-muted);font-size:calc(11px * var(--cy-font-scale, 1));overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ts-val{text-align:right;flex-shrink:0}
.ts-val strong{display:block;font-size:calc(16px * var(--cy-font-scale, 1));font-weight:500}
.ts-val small{display:block;font-size:calc(10px * var(--cy-font-scale, 1));color:var(--cy-muted);margin-top:2px}
.ts-panel{position:fixed;left:0;right:0;bottom:0;background:var(--cy-card);border-top:1px solid var(--cy-line);padding:16px 20px calc(22px + var(--f7-safe-area-bottom, env(safe-area-inset-bottom)));z-index:20}
.ts-panel-head{display:flex;align-items:center;gap:8px;margin-bottom:10px}
.ts-panel-head strong{font-size:15px}
.ts-reset{margin-left:auto;border:0;background:none;color:var(--cy-accent);font-size:13px;padding:4px 0}
.ts-scale-marks{display:flex;justify-content:space-between;align-items:flex-end;margin-bottom:6px;position:relative}
.ts-mark-min{font-size:14px;color:var(--cy-muted)}
.ts-mark-std{position:absolute;left:calc(100% / 6 - 14px);font-size:12px;color:var(--cy-accent)}
.ts-mark-max{font-size:23px;color:var(--cy-muted)}
</style>
