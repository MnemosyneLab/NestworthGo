# Gio 账户页对照验收 · 2026-11-27

以《Nestworth 账户页 Gio/Wails 差异表》中的 01–37 为索引。原报告实测早于 #297。本次先复核 #297 后的 `4148989e`，再同步已合入主分支的 #298（`ef09d4cc`）。余额表单按更新后的 Wails 实现核对。Gio 实现与原生验收基线为 `e0b81f66`。

## 逐项结果

| # | 原核对项 | 状态 | 本次证据及仍有的差异 |
| --- | --- | --- | --- |
| 01 | 账户列表页首 | 本次修复 | #297 的标题栏「添加账户」保留。正文恢复账户用途说明。列表不再用读取时间替换说明。见账户列表对照。 [标题栏入口](../../../experiments/gio/internal/desktop/page_title_actions.go) |
| 02 | 归档开关 | 本次修复 | 固定文案「显示已归档」及复选状态。账户列表回归同时覆盖勾选、归档余额和返回。 [列表回归](../../../experiments/gio/internal/desktop/accounts_test.go) |
| 03 | 机构分组和净额 | 本次修复 | 统一「未指定机构」。保留 Gio 的机构图标和服务计算的组净额。账户列表回归比较 Wails 估值。 [机构和估值对照](../../../experiments/gio/internal/desktop/accounts_test.go) |
| 04 | 行、字号和徽标 | 本次修复 | 类型、负债、归档成为独立标签，保留记账方式。估值完整性仍靠近对应金额。见账户列表对照。 [账户行渲染](../../../experiments/gio/internal/desktop/accounts_view.go) |
| 05 | 余额、折算和归档金额 | 已对齐 | 共享估值读取继续提供原币、折算和部分估值信息。合成 AUD 银行账户、USD 余额和信用卡负号均纳入核对。 [精确余额、估值与归档回归](../../../experiments/gio/internal/desktop/accounts_test.go) |
| 06 | 机构、成员、类型、分组范围 | 本次修复 | 保留已有范围及清除动作，增加范围标签和成员说明。机构／返回由账户测试覆盖。未做四种范围的逐一原生点击。 [范围过滤](../../../experiments/gio/internal/desktop/account_scope.go) |
| 07 | 开户类型选择 | 本次修复 | 同一开户侧栏，中文枚举改为「证券账户」「数字钱包」「房产及其他财产」。开户选择器用例覆盖可选组合。 [开户选择器测试](../../../experiments/gio/internal/desktop/account_create_chooser_test.go) |
| 08 | 选择机构／新建机构 | 有理由保留 | 已有侧栏支持选机构、无机构及就地新建。保留 Gio 可搜索选择器与返回机制。开户机构用例覆盖创建及选中。未原生提交新机构。 [机构创建与选择测试](../../../experiments/gio/internal/desktop/account_create_institution_test.go) |
| 09 | 记账方式 | 本次修复 | 文案改为「记账方式」「余额」「现金和持仓」。推荐说明随相应选项展示，合法组合仍来自共享领域模型。 [合法方式及推荐选项](../../../experiments/gio/internal/desktop/account_create_chooser.go) |
| 10 | 开户详情和所有权 | 有理由保留 | 开户和编辑仍使用已有所有权控件。开户保留选配分组、份额和纳入净资产信息。设置详情默认收起。自动化覆盖合法组合和精确份额。 [所有权写入和失败重试测试](../../../experiments/gio/internal/desktop/account_maintenance_test.go) |
| 11 | 开户确认与建成后 | 本次修复 | 确认摘要按宽度使用双列。标准货币格式之外，只有舍入会隐藏非零精度时才补充原始精度。开户用例覆盖提交、读回、新账户 ID 和返回范围。 [开户确认及持久化测试](../../../experiments/gio/internal/desktop/account_shell_create_test.go) |
| 12 | 详情面包屑及工具栏 | 本次修复 | 将「账户 /」放到标题栏，移除正文重复的分析和设置按钮。#297 的记录与更多菜单继续使用。见详情对照。 [标题栏入口](../../../experiments/gio/internal/desktop/page_title_actions.go) |
| 13 | 详情摘要与元信息 | 本次修复 | 类型／机构与金额分列，读取时间靠近金额，保留分组、所有人及份额。沿用 Gio 原有大金额字形。见详情对照。 [账户摘要渲染](../../../experiments/gio/internal/desktop/accounts_view.go) |
| 14 | 归档详情和恢复 | 本次修复 | 详情提示只读及净资产排除，恢复从设置发起。归档／恢复测试覆盖写入与刷新后读回。 [归档与恢复测试](../../../experiments/gio/internal/desktop/account_shell_maintenance_test.go) |
| 15 | 核对卡片 | 本次修复 | 常规宽度让上次核对状态与入口同排，窄宽可换行。保留信用卡账单对账。 [核对卡片与使用范围测试](../../../experiments/gio/internal/desktop/account_detail_test.go) |
| 16 | 回报、资产变化、相关交易 | 本次修复 | 菜单使用账户范围及「相关交易」文案。回归测试验证交易范围与返回菜单焦点。 [菜单范围与焦点测试](../../../experiments/gio/internal/desktop/account_positions_test.go) |
| 17 | 环图和数据表 | 本次修复 | 环图显示已估值小计，分组切换位于右上，图例百分比按一位小数舍入。保留完整数据表和缺值状态。见构成对照。 [环图数据和精度测试](../../../experiments/gio/internal/desktop/account_detail_test.go) |
| 18 | 外币构成明细 | 本次修复 | 同一图例显示 AUD 原币及 USD 折算。本位币条目只显示一个金额。渲染数据断言覆盖精度和缺值。见构成对照。 [原币与折算金额断言](../../../experiments/gio/internal/desktop/account_detail_test.go) |
| 19 | 现金区 | 本次修复 | 原币与折算分行，移除多余币种前缀。「校准余额」及 Gio 的直接转账、换汇和债务动作继续可达。现金动作保留现有独立入口，Wails 把部分动作放在菜单。 [现金校准与精度测试](../../../experiments/gio/internal/desktop/account_cash_balance_test.go) |
| 20 | 持仓列表、分组和排序 | 本次修复 | 常规窗口列式行，窄宽使用紧凑堆叠。标的名、代码、分类、现有排序、筛选和组小计均保留。快捷动作移到列表后。 [排序分组与小计测试](../../../experiments/gio/internal/desktop/account_positions_test.go) |
| 21 | 持仓原位展开与快捷操作 | 本次修复 | 账户内展开共用日行情、周期、缺行情、错误及重试。额外保留显式打开证券、转移和调整。自动化验证两类行情及原账户的交易范围。 [有／无行情及导航范围测试](../../../experiments/gio/internal/desktop/account_positions_test.go) |
| 22 | 现金历史 | 本次修复 | 默认可见。类型、变动、余额、币种、生效、记录六列。原因和备注分行。复用共享金额及时间显示。长列表继续使用虚拟滚动。 [流水列和原因展示断言](../../../experiments/gio/internal/desktop/account_detail_test.go) |
| 23 | 「记录变更」选择器 | 本次修复 | 从标题栏打开右侧表单，选动作后继续在同一侧栏。账户背景及范围保留。原有分类和精确表单复用。 [选择器和返回测试](../../../experiments/gio/internal/desktop/account_detail_test.go) |
| 24 | 普通账户余额更新 | 本次修复 | #298 后加入账本时区的日期、时间、原因和备注，并在预览核对。测试比较 Wails 提交后的精确余额、原因、备注、时间及 Escape 后未写入。 [生效时间、原因、备注及取消对照](../../../experiments/gio/internal/desktop/account_bank_balance_test.go) |
| 25 | 信用卡／负债余额 | 本次修复 | 信用卡保留正数录入和负债展示符号。日期、时间和备注可编辑，原因仍是负债校准允许的对账原因。测试覆盖多次点击和同一请求重试。 [信用卡负债及幂等测试](../../../experiments/gio/internal/desktop/account_card_balance_test.go) |
| 26 | 手工估值、借款和还款 | 本次修复 | 估值与债务活动留在账户侧栏。保留本金／利息和作用域校验。相关用例覆盖精确确认。未原生提交每种贷款动作。 [债务本金、利息和账户范围测试](../../../experiments/gio/internal/desktop/account_liability_posting_test.go) |
| 27 | 现金校准、存取、换汇、转账 | 本次修复 | 固定账户的编辑搬到侧栏。保留现金校准的专用币种和原因。自动化覆盖多币种配对、返回和焦点。 [跨币种现金读写对照](../../../experiments/gio/internal/desktop/account_cash_balance_test.go) |
| 28 | 买入、卖出、现金分红 | 有理由保留 | 共用账户侧栏，继续使用限定候选标的与币种的专用买卖表单。从持仓可直达分红。买、卖、分红的精确回执由配对测试覆盖。 [外币买卖精确结算测试](../../../experiments/gio/internal/desktop/account_foreign_trade_test.go) |
| 29 | 已有持仓、转移、调整 | 本次修复 | 保留锁定账户或持仓的快捷操作、原单位成本和预览。#298 移除了历史未启用时的旧入口。本次以当前账本历史路径为准。 [已有持仓及原始成本测试](../../../experiments/gio/internal/desktop/account_position_import_test.go) |
| 30 | 设置布局与保存 | 本次修复 | 从标题菜单打开右侧设置。机构／分组／图标折叠，固定底部操作。自动化覆盖取消、失效引用、保存、读回。 [账户设置侧栏](../../../experiments/gio/internal/desktop/account_maintenance_view.go) |
| 31 | 归档／恢复确认和焦点 | 本次修复 | 设置内入口、危险按钮及居中确认。保留历史提示与 Escape。自动化验证取消不写入、受管产品限制和刷新重试。 [归档刷新与复核测试](../../../experiments/gio/internal/desktop/account_maintenance_test.go) |
| 32 | 无持仓账户的产品引导 | 本次修复 | 银行余额与手工估值等非持仓资产账户显示「存款与产品」说明，可打开新持仓账户表单。原账户及其历史保留。尚未原生从此入口完成开户。 [产品引导入口](../../../experiments/gio/internal/desktop/accounts_view.go) |
| 33 | 持仓账户产品区和添加 | 本次修复 | 紧凑产品行、空态、共享金额格式的可用现金及账户内添加侧栏。复杂产品条款仍用现有表单。自动化覆盖开立和录入已有产品。 [从账户录入产品测试](../../../experiments/gio/internal/desktop/available_funds_product_test.go) |
| 34 | 合约详情、历史和回执 | 有理由保留 | 使用共享产品详情和操作回执页，保留预计结算信息。账户带着原范围、滚动和焦点返回。回归测试覆盖保存精确估值后的读回。未原生完成到期实收或撤销。 [产品回执和返回焦点测试](../../../experiments/gio/internal/desktop/managed_product_lifecycle_test.go) |
| 35 | 从账户发起核对 | 有理由保留 | 使用预选本账户的核对会话，并保留上次成功核对入口。目标页复用现有账户核对流程。全账户选择属于待办页。 [账户核对作用域测试](../../../experiments/gio/internal/desktop/account_detail_test.go) |
| 36 | 信用卡账单专项 | 有理由保留 | 保留独立的账单对账入口。自动化覆盖共享核对会话、证据和恢复。未原生完整导入并提交账单。 [信用卡账单会话测试](../../../experiments/gio/internal/desktop/account_statement_test.go) |
| 37 | 空态、缺值和读取失败 | 本次修复 | 持仓提供无日行情提示，现金流水失败有重试。缺估值仍使用共享估值状态。自动化覆盖读取重试和迟到结果。未以线上故障验证。 [迟到读取与缺值等回归](../../../experiments/gio/internal/desktop/account_detail_test.go) |

## 自动化及构建

- 在 macOS 27.2.1 ARM64、Go 1.27.4 运行 Gio 全模块测试，排除单个字体测试后通过。该字体测试曾在未排除的运行失败，也在原始基线的相同字体测试复现。账页相关测试未排除。
- Go 的 `internal/application`、`internal/wailsapi/history`、`internal/format` 在实现提交通过。
- Wails 使用现有 `com.nestworth.dev` 开发包身份，Gio 使用 `com.nestworth.gio`。本机可运行构建与 Go 测试分开执行。发布 DMG／ZIP、签名公证和 Linux 原生窗口未验收。

## macOS 原生对照

Gio 使用 `e0b81f66`。Wails 使用含 #298 的 `ef09d4cc` 源码，同步提交中的 Wails 源码与该提交相同。原图由 macOS 原生窗口截取，保留原始像素。常规窗口 1280×800，窄窗口 900×600，以下为窗口点数。两端分别使用同一个只含合成数据的种子副本，家庭名称为 `Account Parity Synthetic Household`。副本启动前通过家庭标记验证，数据库和设置路径各自隔离。种子数据库 SHA-256 为 `c6f5cf9b9faa0b087bc2749ab4c6c85c9c0f34aeda006f7f1be9077f3218db04`。

| 场景 | Wails 原图 | Gio 原图 |
| --- | --- | --- |
| 常规账户列表，含 AUD 折算、USD 和信用卡负债 | [列表](native/wails-regular-list.png) | [列表](native/gio-regular-list.png) |
| 外币资产构成明细，AUD 850.01 折 USD 563.37 | [构成](native/wails-regular-composition.png) | [构成](native/gio-regular-composition.png) |
| 证券行内有日行情 | [走势](native/wails-regular-holding-history.png) | [走势](native/gio-regular-holding-history.png) |
| 无日行情、产品与现金流水 | [无行情](native/wails-regular-no-history.png) · [流水](native/wails-regular-cash-history.png) | [无行情及流水](native/gio-regular-cash-history.png) |
| 窄窗口账户详情和记账 | [详情](native/wails-narrow-broker.png) · [记账](native/wails-narrow-record.png) | [详情](native/gio-narrow-broker.png) · [记账](native/gio-narrow-record.png) |
| 窄窗口账户余额 | [余额抽屉](native/wails-narrow-balance.png) | [字段](native/gio-narrow-balance.png) · [预览](native/gio-narrow-balance-review.png) · [保存后读回](native/gio-narrow-balance-saved.png) |
| Gio 账户内其他入口 | | [设置](native/gio-narrow-settings.png) · [信用卡账单入口](native/gio-narrow-credit-card.png) · [开户](native/gio-narrow-create.png) |

两端余额抽屉按 Escape 前后均为 3 条活动。Gio 在窄窗口保存后，从隔离 SQLite 读回 AUD 17,486.27，匹配一条带 `interest` 原因和 `Synthetic balance QA` 备注的活动。Gio 原生焦点回到余额更新入口。另从券商详情返回账户列表时，焦点回到该券商行。记录侧栏取消后，焦点回到记录入口。显式指定日期和时间后的写入由 Gio/Wails 服务配对回归验证。

## 交接

- **未验证**　Wails 原生余额提交后的读回、Gio 原生改选日期和时间后的提交、逐类开户提交、合同到期实收或撤销、信用卡账单从导入到提交、VoiceOver 和 Linux 原生交互。Wails 和 Gio 的原生原图以上表所列窗口及流程为准。
- **有理由保留**　Gio 余额快捷操作保留原有的对账默认原因，资产账户可改选原因。负债余额校准按共享规则使用对账原因。合同详情仍用可用资金的完整生命周期页，账户核对仍预选本账户。信用卡账单专项入口和已有的大金额字体保留。
- **形态差异**　现金卡的快捷按钮位置、买卖的独立表单和开户的推荐选项样式仍有差异。逐项边界见上表。建议接手人优先复测 08、11、25、31、33–36 的原生提交、取消及返回。
- **构建与检查**　全模块测试在 `66933682` 通过单项字体排除，后续标题改动使用对应账户回归验证。已公开的工作分支以 GitHub PR 为准。
