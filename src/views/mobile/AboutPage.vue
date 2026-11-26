<template>
    <f7-page class="cy-mobile-surface">
        <f7-navbar title="关于 OpenBill" back-link="返回" />

        <main class="cy-page-body about-body">
            <section class="cy-panel about-brand">
                <div class="about-badge" aria-hidden="true"><f7-icon f7="book_fill" /></div>
                <h1>OpenBill</h1>
                <p class="cy-muted">打开账本，理清生活 · 数据仅存于本机</p>
                <span class="about-version">版本 {{ clientVersion }}</span>
            </section>

            <section class="cy-panel about-links">
                <div class="about-row"><span>客户端版本</span><small>{{ clientVersion }}</small></div>
                <div v-if="clientBuildTime" class="about-row"><span>构建时间</span><small>{{ clientBuildTime }}</small></div>
                <div v-if="exchangeRatesData && !isUserCustomExchangeRates" class="about-row">
                    <span>汇率数据源</span>
                    <small>{{ exchangeRatesData.dataSource }}</small>
                </div>
                <button type="button" class="about-row about-action" @click="openExternalUrl('https://github.com/cheng-yi-cc/OpenBill')">
                    <span>开源仓库</span>
                    <f7-icon f7="chevron_right" />
                </button>
                <button type="button" class="about-row about-action" @click="openExternalUrl('https://github.com/cheng-yi-cc/OpenBill/issues')">
                    <span>问题与建议反馈</span>
                    <f7-icon f7="chevron_right" />
                </button>
                <button type="button" class="about-row about-action" @click="licenseOpen = true">
                    <span>开源许可与第三方鸣谢</span>
                    <f7-icon f7="chevron_right" />
                </button>
            </section>

            <p class="cy-muted about-note">
                OpenBill 新增与修改部分采用 Apache 2.0 许可协议；衍生自 ezBookkeeping（MIT 许可协议，Copyright © 2020–2026 MaysWind）。所有账务、资产与附件数据均保存在您的设备本地。
            </p>
        </main>

        <f7-popup v-model:opened="licenseOpen" class="cy-mobile-surface">
            <f7-page class="cy-mobile-surface">
                <f7-navbar title="开源许可与鸣谢">
                    <f7-nav-right><f7-link @click="licenseOpen = false">关闭</f7-link></f7-nav-right>
                </f7-navbar>
                <main class="cy-page-body about-license">
                    <section class="cy-panel">
                        <h2>OpenBill 许可说明</h2>
                        <p>OpenBill / CY Finance Manager — Copyright 2026 OpenBill contributors.</p>
                        <p>OpenBill 新增与修改部分基于 Apache License 2.0 发布；派生自上游 ezBookkeeping v2.0.0（MIT License，Copyright © 2020–2026 MaysWind），完整保留其原始版权与 MIT 许可声明。</p>
                        <p>部分交易所与钱包图标来自 Web3 Icons；部分银行标志来自 IconGo bank-logos（MIT License），商标权归各自权利人所有。</p>
                    </section>
                    <section v-if="licenseLines.length" class="cy-panel">
                        <h2>许可证正文</h2>
                        <pre class="about-pre">{{ licenseLines.join('\n') }}</pre>
                    </section>
                    <section v-if="thirdPartyLicenses.length" class="cy-panel">
                        <h2>第三方组件</h2>
                        <div v-for="item in thirdPartyLicenses" :key="item.name" class="about-tp">
                            <strong>{{ item.name }}</strong>
                            <small v-if="item.copyright">{{ item.copyright }}</small>
                            <small v-if="item.license || item.licenseUrl">{{ item.license || 'License' }}<template v-if="item.licenseUrl"> · {{ item.licenseUrl }}</template></small>
                        </div>
                    </section>
                </main>
            </f7-page>
        </f7-popup>
    </f7-page>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import { useAboutPageBase } from '@/views/base/AboutPageBase.ts';

const { openExternalUrl } = useI18nUIComponents();
const {
    clientVersion,
    clientBuildTime,
    exchangeRatesData,
    isUserCustomExchangeRates,
    licenseLines,
    thirdPartyLicenses,
    init
} = useAboutPageBase();

const licenseOpen = ref(false);
init();
</script>

<style scoped>
.about-body{padding-top:16px!important}
.about-brand{display:flex;flex-direction:column;align-items:center;text-align:center;gap:8px;padding:28px 16px!important}
.about-badge{width:56px;height:56px;border-radius:16px;background:var(--cy-accent);color:#fff;display:grid;place-items:center}
.about-badge .icon{font-size:28px}
.about-brand h1{font-size:calc(22px * var(--cy-font-scale, 1));font-weight:650;margin:4px 0 0}
.about-version{display:inline-block;margin-top:4px;padding:3px 10px;border-radius:12px;background:var(--cy-soft);color:var(--cy-accent);font-size:calc(12px * var(--cy-font-scale, 1))}
.about-links{padding:4px 18px!important}
.about-row{display:flex;align-items:center;justify-content:space-between;gap:12px;width:100%;min-height:54px;padding:14px 0;border:0;border-bottom:1px solid var(--cy-line);background:none;color:var(--cy-ink);text-align:left;font-size:calc(15px * var(--cy-font-scale, 1))}
.about-row:last-child{border-bottom:0}
.about-row small{color:var(--cy-muted);font-size:calc(13px * var(--cy-font-scale, 1));text-align:right;overflow-wrap:anywhere}
.about-action .icon{font-size:13px;color:var(--cy-muted)}
.about-note{padding:0 6px;line-height:1.75}
.about-license h2{margin-bottom:10px}
.about-license p{font-size:calc(13px * var(--cy-font-scale, 1));line-height:1.75;color:var(--cy-muted);margin-bottom:10px}
.about-pre{white-space:pre-wrap;font-family:inherit;font-size:calc(12px * var(--cy-font-scale, 1));line-height:1.65;color:var(--cy-muted);margin:0;overflow-wrap:anywhere}
.about-tp{padding:10px 0;border-bottom:1px solid var(--cy-line)}
.about-tp:last-child{border-bottom:0;padding-bottom:0}
.about-tp strong{display:block;font-size:calc(14px * var(--cy-font-scale, 1));font-weight:500}
.about-tp small{display:block;font-size:calc(11px * var(--cy-font-scale, 1));color:var(--cy-muted);margin-top:3px;word-break:break-all}
</style>
