# OpenBill 验收与交接

状态日期：2026-11-26。本轮已完成全部界面对齐、10 月 8 日四项功能融合（25 个加密平台图标与搜索、初始成本换算与首次定投、部分未知持仓保留已知小计、账单详情居中日期滚轮与逐项直编/关联账单）、全项目细腻动效升级，以及本机测试机与主力机的同签名升级验收。

## 当前现役功能状态

- **日常记账与详情直编**：支持快速四则键盘、分类/子分类、两级标签、优惠与实付反算、手续费/转账优惠同事务联动、居中日期与时间滚轮弹窗、详情页字段单项直编、双向手动关联账单、独立 `Refund` 模型退款及超额/分期联动，以及分类/标签/账户统计下钻。
- **多账本与资产管理**：支持独立创建/编辑/归档/隐藏账本与分类显示范围、整本事务级迁移（含关联退款、报销及同账本投资事件）、资金/信贷/预付/保险/二手/报销/债务/定存全链路管理，以及信用卡分期（含退款月账单与 `offset`/`reduce` 协同）。
- **投资与加密货币**：覆盖 25 个加密交易所/钱包品牌图标与搜索、初始总成本↔均价换算与同事务首次定投、基于 `DcaScheduler`/`DcaNotifier` 的定时提醒与补记、基金确认/定投、货币基金逐日收益防重、钱包多币种收支（含标签/附件/优惠/地点名称）与部分未知持仓保留“已知”小计展示。
- **统计与预算**：支持日常/月/年/自定义周期、一级标签含子级按账单去重、由账务事实实时重算的分类与总预算（携带乐观锁修订校验）、年度热力图、月年总结及真实历史时间轴资产走势。
- **动效体系与无障碍**：通过 `src/lib/motion.ts`、`src/styles/mobile/_motion.scss` 及原生 `OpenBillSplash`/`MainActivity`/`PatternView` 统一交互曲线、等宽数字、页面与弹窗过渡、列表 FLIP 重排及错误轻量反馈；完整遵循系统 `prefers-reduced-motion` 与应用内动画开关。

## 现役验证范围

- **前端与类型检查**：`node node_modules/vue-tsc/bin/vue-tsc.js --noEmit`、全量 Vitest 及 `npm run build`（仅输出 Android 资产 `dist/mobile.html`，无 `dist/desktop.html`）。
- **Go 与集成检查**：`go test -count=1 ./cmd ./pkg/services ./pkg/api ./pkg/localbackup ./pkg/marketquotes ./pkg/marketrelay ./android/backend`、`python scripts/test_market_network_config.py`，及 Java 8 语法兼容编译检查。
- **无头浏览器与真机验效**：Playwright 覆盖 `320×640` / `390×844` / `1280×800` × 浅色/深色与降低动态效果验证（0 错误、0 横向溢出）；测试机已完成 QA `dumpsys gfxinfo` 帧率验证及完整备份/恢复验证；主力机（Redmi K70，Android 16）已完成同签名 `install --user 0 --no-incremental -r` 就地覆盖更新与只读启动核验，`firstInstallTime` 与数据目录 inode 保持不变。

## 交付产物与边界

- **正式安装包**：`dist/android/OpenBill-Android-arm64.apk`（`com.cyledger.android`，`versionCode=4`，`versionName=0.3.0`，`64,231,814` 字节，SHA-256 `5a5cd500db2aafd055cdcbce631ef537b9802945e41e151833beb3ab10816716`）。
- **签名与持久化材料**：证书指纹 `19d9a950a1d3608d8098f6ea8ebe47b464126d537946470eb7a369007ad1add3`（密钥位于 `.runtime/android-signing/`，长期保留且不入库）。
- **范围之外与外部依赖**：暂不开发自动记账、桌面小组件与云同步；钱包收支暂不支持报销、地图坐标与跨币草稿自动换算。公开行情/汇率依赖网络及供应商，可选行情中转须自行部署并透过 `--market-network` 配置。界面依赖现代 Android System WebView（已验证 WebView 155+，WebView 83 等过旧版本不兼容）。

## 预览与构建

- **主力机 / 正式版**：直接打开手机 **OpenBill**（同签名更新使用 `adb -s <设备序列号> install --user 0 --no-incremental -r dist/android/OpenBill-Android-arm64.apk`，严禁卸载或清数据）。
- **构建与测试预览**：在项目根目录执行 `python scripts/build_android.py` 构建正式包，或加 `--qa` 构建独立测试包 **OpenBill 测试**（`com.cyledger.android.qa`，数据完全隔离）。
