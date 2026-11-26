# OpenBill

OpenBill 是面向个人使用的 Android 记账与资产管理应用，以一木记账的已有功能和操作体验为参考，结合本地账本与加密货币资产管理，提供日常收支、多账本、预算统计、投资、报销与债务等功能。

项目正按这一方向逐项检查和改进已有功能的界面与交互；暂不开发自动记账，不提供桌面小组件和云同步。加密货币资产支持持仓、交易记录、估值与本地定投参考记账，不连接交易所账户或代替用户交易。

仅提供 Android 应用，在手机内运行 Go / SQLite，断网也能记账。应用采用深墨绿与暖白的展开账本图标，启动时播放轻量原生动画，页面就绪后直接进入。

## 开始使用

手机安装包：`dist/android/OpenBill-Android-arm64.apk`。**首页**提供账单搜索、手动记账和数据卡片；账户与持仓在 **资产**，预算和总结在 **统计**，愿望、导入导出、完整备份和个性设置在 **我的**。已有正式版须使用同签名覆盖升级，不能卸载或清除数据。构建和安装见 [Android 说明](android/README.md)。

## 文档

| 文档 | 内容 |
|---|---|
| [使用与开发入口](README.OPENBILL.md) | 记账、资产操作、本机工具与验证命令 |
| [产品范围](docs/OPENBILL_BRIEF.md) | 产品目标、账务口径与未实现范围 |
| [架构](docs/OPENBILL_ARCHITECTURE.md) | 数据模型、原子提交、估值、路由与 Android 边界 |
| [接口指南](docs/OPENBILL_API.md) | API 路径、鉴权、输入、幂等和错误处理 |
| [运维](docs/OPENBILL_OPERATIONS.md) | Android 安装、备份、恢复和故障排查 |
| [验收与交接](docs/OPENBILL_ACCEPTANCE.md) | 已验证范围、正式 APK 校验值及限制 |

公开行情依赖网络及供应商；缺失成本、价格或历史汇率保留未知。应用不连接银行或交易所账户，不执行扣款、下单或转币。

## 许可与归属

OpenBill 新增与修改部分采用 [Apache License 2.0](LICENSE)。上游 [ezBookkeeping](https://github.com/mayswind/ezbookkeeping) 固定为 v2.0.0，基线提交 `b2a3f4ede42c8a7aa3bab1e6dbbc7b8e6196ef35`；上游代码保留其 [MIT 许可](licenses/ezbookkeeping-MIT-LICENSE)。其他依赖及图标声明见 [NOTICE](NOTICE)、[第三方清单](third-party-dependencies.json) 和 `licenses/`。Go 模块路径保持上游名称。
