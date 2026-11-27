# Gio / Wails 账户页视觉对照

[37 项验收和前两轮原图](report.md) 保留。此页记录 PR #299 的账户页视觉对齐、原生交互复测及仍存在的差异。

## 复现环境

在 Mac mini（macOS 27.2、ARM64、Go 1.27.4）运行固定的 `Nestworth.app`（`com.nestworth.dev`）及 `Nestworth Gio.app`（`com.nestworth.gio`）。启动前核对签名、二进制源码提交、包名、Bundle ID、SQLite 合成家庭标记和数据库及设置真实路径。构建和 UI 验收分开执行。

对照起点为 `75b8069d`，其应用源码与已签验证包的 `86b3371f` 相同。Wails 为参考端。使用同一只含合成数据的种子独立副本，SHA-256 为 `01ce198bb9d2ad5889e12ecd46cea327522a3a91bd20da2a7383b43982960d65`。家庭标记为 `Account Parity Synthetic Household`。账户包括 USD 和 AUD 银行账户、信用卡、空证券账户，以及持有已到期且尚未确认实收的 USD 定存的银行账户。保留缺汇率和负债作为金额展示边界。

窗口设置为常规 1280×800、窄窗 900×600 点。Wails 原生标题栏计入对照高度，窗口捕获比 Gio 窄 2 个 Retina 像素。图像为原生窗口输出的原始 JPEG，保持完整画面和像素尺寸。

## 页面及流程对照


| 场景 | Wails 参考 | Gio 对齐前 | Gio 对齐后 |
| --- | --- | --- | --- |
| 常规账户列表 | [原图](visual/before/wails-wide-list.jpg) | [原图](visual/before/gio-wide-list.jpg) | [原图](visual/after/gio-wide-list.jpg) |
| 窄窗账户列表 | [原图](visual/before/wails-narrow-list.jpg) | [原图](visual/before/gio-narrow-list.jpg) | [原图](visual/after/gio-narrow-list.jpg) |
| 常规账户详情 | [原图](visual/before/wails-wide-detail.jpg) | [原图](visual/before/gio-wide-detail.jpg) | [原图](visual/after/gio-wide-detail.jpg) |
| 窄窗账户详情 | [原图](visual/before/wails-narrow-detail.jpg) | [原图](visual/before/gio-narrow-detail.jpg) | [原图](visual/after/gio-narrow-detail.jpg) |
| 定存行及详情，常规 | [产品行](visual/before/wails-wide-product-row.jpg) · [抽屉](visual/before/wails-wide-product.jpg) | [产品行](visual/before/gio-wide-product-row.jpg) · [跨页](visual/before/gio-wide-product.jpg) | [产品行](visual/after/gio-wide-product-row.jpg) · [抽屉](visual/after/gio-wide-product.jpg) · [关闭](visual/after/gio-wide-product-closed.jpg) |
| 定存行及详情，窄窗 | [产品行](visual/before/wails-narrow-product-row.jpg) · [抽屉](visual/before/wails-narrow-product.jpg) | [产品行](visual/before/gio-narrow-product-row.jpg) · [跨页](visual/before/gio-narrow-product.jpg) | [产品行](visual/after/gio-narrow-product-row.jpg) · [抽屉](visual/after/gio-narrow-product.jpg) · [滚动](visual/after/gio-narrow-product-scrolled.jpg) · [关闭](visual/after/gio-narrow-product-closed.jpg) |
| 窄窗记账类型和更多菜单 | [记账](visual/before/wails-narrow-record.jpg) · [菜单](visual/before/wails-narrow-more-menu.jpg) | [记账](visual/before/gio-narrow-record.jpg) · [菜单](visual/before/gio-narrow-more-menu.jpg) | [记账](visual/after/gio-narrow-record.jpg) · [菜单](visual/after/gio-narrow-more-menu.jpg) |
| 相关交易与返回 | [交易](visual/before/wails-narrow-transactions.jpg) · [返回](visual/before/wails-narrow-transactions-back.jpg) | [交易](visual/before/gio-narrow-transactions.jpg) · [返回](visual/before/gio-narrow-transactions-back.jpg) | [交易](visual/after/gio-narrow-transactions.jpg) · [返回](visual/after/gio-narrow-transactions-back.jpg) |
| 归档开关和详情 | [关闭](visual/after/wails-narrow-archive-off.jpg) · [开启](visual/after/wails-narrow-archive-on.jpg) · [详情](visual/after/wails-narrow-archive-detail.jpg) | | [关闭](visual/after/gio-narrow-archive-off.jpg) · [开启](visual/after/gio-narrow-archive-on.jpg) · [详情](visual/after/gio-narrow-archive-detail.jpg) |

### 账户页样式

- 列表把账户类型、负债和归档标签放在名称旁，估值状态放在账户标识下方，金额列右对齐。实测 USD 信用卡显示负数，AUD 缺汇率账户保留原币金额、折算待定及可访问缺值说明。标题栏添加账户入口沿用 #297。
- 详情在常规窗口将账户标识与金额并列，窄窗先显示标识再显示金额和状态。Gio 继续展示记账方式、所有权、分组和无行情说明。Gio 原生应用栏和 Wails 的 AI 助手入口维持各自应用导航。
- 账户记账类型侧栏使用固定账户说明和分组响应式按钮。侧栏宽度为 576 点。两端更多菜单的相关交易入口都进入同一合成账户的记录。详情中的账户产品用紧凑卡片显示本金、当前价值和到期日。普通金额遵循共享货币格式，复核和账务回执继续显示原币精确值。

### 定存结算和回执

1. 两种尺寸从 `Synthetic PR299 Deposit Bank` 的产品行打开已到期定存。Gio 在同一账户上打开右侧抽屉，按 Escape 恢复滚动位置和定存行焦点。窄窗的关闭按钮起始获得焦点，Page Down 可滚动抽屉正文。「可用资金」直接进入仍显示完整产品页。 [可用资金直达原图](visual/after/gio-wide-funds-deeplink.jpg)
2. 在窄窗确认实收，本金 `750.125 USD`、利息 `9.875 USD`、费用 `0.125 USD`，现金由 `2000 USD` 变为 `2759.875 USD`。分别通过本金回执的「返回产品详情」和利息回执的「打开产品」回到同一抽屉及对应回执按钮。最后 Escape 回到原账户的定存行。预计结算、操作历史、实收和回执沿用共用详情及写入控制器。 [精确预览](visual/after/gio-narrow-product-review.jpg) · [已结算](visual/after/gio-narrow-settled.jpg) · [本金回执](visual/after/gio-narrow-principal-receipt.jpg) · [本金回执返回](visual/after/gio-narrow-principal-return.jpg) · [利息回执](visual/after/gio-narrow-interest-receipt.jpg) · [利息回执返回](visual/after/gio-narrow-interest-return.jpg)
3. 重开同一隔离账本，原生读回已结算合同及 `2759.875 USD` 的利息回执余额。SQLite 中一次 `record_existing`、一次 `settle`，结算关联两条活动。响应丢失后重试只记一次由故障注入回归覆盖。 [重开产品](visual/after/gio-narrow-product-reopened.jpg) · [重开回执](visual/after/gio-narrow-reopened-interest.jpg)

### 账户范围与归档

| 范围 | Wails 原图 | Gio 原图及实测行数 |
| --- | --- | --- |
| Synthetic Parity Bank | [机构](visual/before/wails-wide-scope-institution.jpg) | [机构](visual/after/gio-wide-scope-institution.jpg) · 5 |
| Bank account | [类型](visual/before/wails-wide-scope-type.jpg) | [类型](visual/after/gio-wide-scope-type.jpg) · 3 |
| Synthetic Owner | [成员](visual/before/wails-wide-scope-member.jpg) | [成员](visual/after/gio-wide-scope-member.jpg) · 5 |
| Default group | [分组](visual/before/wails-wide-scope-group.jpg) | [分组](visual/after/gio-wide-scope-group.jpg) · 1 |

- 两端验证四种范围。Wails 从概览进入。Gio 从目录的机构详情进入机构范围，从概览进入其余三个范围。Gio 打开组内账户后返回，保留范围和行焦点，再用 Shift+Tab 到清除按钮并按 Return，恢复全部 5 行。机构范围返回时也恢复账户行和机构详情「查看账户」焦点。 [分组返回](visual/after/gio-wide-scope-return.jpg) · [键盘清除后](visual/after/gio-wide-scope-cleared.jpg)
- 相关交易只显示选定存款账户的一笔入账。Gio 返回同一账户后将焦点交还更多菜单。Wails 返回同一账户详情。
- 在 Gio 窄窗归档空的合成证券账户。默认隐藏，启用归档复选框后可打开只读详情。返回时保留复选状态和账户行焦点，关闭开关再次隐藏。Wails 使用同一已结算、已归档账本的独立副本复查开关、只读详情和返回。

### 检查和限制

- Gio 视觉、回执、结算和归档原图使用 `fb6db8e8`。常规列表与范围复测、重开回执使用 `22219054`，该修复只启用账户范围的清除控件。Wails 始终使用上述参考包。账本、设置和签名包留在隔离 QA 目录。
- 在 `fb6db8e8` 通过 macOS Gio 全模块 `go test -mod=readonly -count=1 ./...`。范围键盘回归在 `22219054` 单独通过。测试还覆盖结算重试、回执往返和窄窗滚动。Wails 定向检查、jsdom 和此前余额与账单写入见 [37 项验收](report.md)。
- 回报和资产变化菜单本轮未原生打开。Gio 概览的机构入口沿用自动化覆盖。未做 VoiceOver、Linux 原生窗口或发布打包验收。信用卡账单核对支持手工证据，本版没有账单文件导入入口。PR 新提交的检查见 [PR #299](https://github.com/MnemosyneLab/NestworthGo/pull/299/checks)。
