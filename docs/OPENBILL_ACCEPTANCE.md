# OpenBill 验收与交接

状态日期：2026-11-27。本轮已完成记账金额同行算式计算、账户详情「记一笔」图标间距修复、普通收支 `CNY`/`USD` 点击无动画切换与系统汇率自动折算及原币持久化、Android 应用内 GitHub Releases 检查更新与 SHA-256 校验覆盖升级，以及 GitHub Release `v0.3.1` 自动发布。

## 当前现役功能状态

- **日常记账与详情直编**：支持快速同行四则键盘、分类/子分类、两级标签、优惠与实付反算、手续费/转账优惠同事务联动、人民币/美元普通收支点击币种符号无动画切换并按系统汇率自动折算入账（持久化原币金额、汇率与日期并在列表/详情/编辑中保留）、居中日期与时间滚轮弹窗、详情页字段单项直编、双向手动关联账单、独立 `Refund` 模型退款及超额/分期联动，以及分类/标签/账户统计下钻。
- **多账本与资产管理**：支持独立创建/编辑/归档/隐藏账本与分类显示范围、整本事务级迁移（含关联退款、报销及同账本投资事件）、资金/信贷/预付/保险/二手/报销/债务/定存全链路管理，以及信用卡分期（含退款月账单与 `offset`/`reduce` 协同）。
- **投资与加密货币**：覆盖 25 个加密交易所/钱包品牌图标与搜索、初始总成本↔均价换算与同事务首次定投、基于 `DcaScheduler`/`DcaNotifier` 的定时提醒与补记、基金确认/定投、货币基金逐日收益防重、钱包多币种收支（含标签/附件/优惠/地点名称）与部分未知持仓保留“已知”小计展示。
- **统计与预算**：支持日常/月/年/自定义周期、一级标签含子级按账单去重、由账务事实实时重算的分类与总预算（携带乐观锁修订校验）、年度热力图、月年总结及真实历史时间轴资产走势。
- **动效体系与无障碍**：通过 `src/lib/motion.ts`、`src/styles/mobile/_motion.scss` 及原生 `OpenBillSplash`/`MainActivity`/`PatternView` 统一交互曲线、等宽数字、页面与弹窗过渡、列表 FLIP 重排及错误轻量反馈；完整遵循系统 `prefers-reduced-motion` 与应用内动画开关。
- **应用内更新与发布**：正式版支持启动 2.5 秒后静默检查（24 小时节流）与「设置 → 关于 OpenBill」手动检索 GitHub Releases，经 `.sha256` 摘要与包名校验后调用 `PackageInstaller` 原位覆盖升级，并提供浏览器打开 Release 页兜底；`scripts/build_android.py` 统一向前后端注入构建版本号。

## 现役验证范围

- **前端与类型检查**：`node node_modules/vue-tsc/bin/vue-tsc.js --noEmit`、全量 Vitest 及 `npm run build`（仅输出 Android 资产 `dist/mobile.html`，无 `dist/desktop.html`）。
- **Go 与集成检查**：`go test`（覆盖 `pkg/services` 跨币校验、`pkg/api`、`pkg/models`、`android/backend` 等包）、`python scripts/test_market_network_config.py`、Java 8 语法编译检查，以及 GitHub Actions 三项工作流（`OpenBill Android frontend`、`Market data regression`、`Release OpenBill Android`）全部通过。
- **无头浏览器与真机验效**：Playwright `390×844` 端到端验证通过（覆盖账户详情间距、同行算式、跨币切换与原币详情/编辑保留及关于页，0 错误、0 横向溢出）并完成截图视觉复核；主力机（Redmi K70，Android 16）已完成新版 `v0.3.0` QA 启动与同签名正式版 `install --no-incremental -r` 覆盖安装（保留原有数据）；GitHub Release `v0.3.1` 及 `/releases/latest` 接口已发布核验（手机端实际点选应用内下载覆盖升至 `v0.3.1` 待用户真机确认）。

## 交付产物与边界

- **正式安装包**：`dist/android/OpenBill-Android-arm64.apk`（`com.cyledger.android`，`versionCode=301`，`versionName=0.3.1`，`64,248,198` 字节，SHA-256 `b39abdfb547c32f9187f9872f16655b0b9d093a6c31c88b54a33fe9e744a48e4`，对应 GitHub Release `v0.3.1`）。
- **签名与持久化材料**：证书指纹 `19d9a950a1d3608d8098f6ea8ebe47b464126d537946470eb7a369007ad1add3`（密钥位于 `.runtime/android-signing/`，长期保留且不入库）。
- **范围之外与外部依赖**：暂不开发自动记账、桌面小组件与云同步；钱包收支暂不支持报销、地图坐标与跨币草稿自动换算。公开行情/汇率依赖网络及供应商，可选行情中转须自行部署并透过 `--market-network` 配置。应用内更新检查与下载依赖访问 GitHub API / Release 附件及系统「允许安装未知应用」授权。界面依赖现代 Android System WebView（已验证 WebView 155+，WebView 83 等过旧版本不兼容）。

## 预览与构建

- **主力机 / 正式版**：直接打开手机 **OpenBill**（同签名更新使用 `adb -s <设备序列号> install --user 0 --no-incremental -r dist/android/OpenBill-Android-arm64.apk`，严禁卸载或清数据）。
- **构建与测试预览**：在项目根目录执行 `python scripts/build_android.py` 构建正式包，或加 `--qa` 构建独立测试包 **OpenBill 测试**（`com.cyledger.android.qa`，数据完全隔离）。
