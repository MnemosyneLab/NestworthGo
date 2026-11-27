# Gio 账户页对照验收 · 2026-11-27

以《Nestworth 账户页 Gio/Wails 差异表》中的 01–37 为索引。原报告实测早于 #297。本次先复核 #297 后的 `4148989e`，再同步已合入主分支的 #298（`ef09d4cc`）。余额表单按更新后的 Wails 实现核对。原生首轮 Gio 基线为 `e0b81f66`。接力从 PR #299 的 `188b4413` 开始，先在 Mac mini 用该提交运行两端的原生流程，再修复复现的问题。表中状态按接力结果填写，「已实测」可包含明确标出的首轮原图。

本轮继续复核账户页视觉、账户范围和定存详情。复现步骤及同一合成种子的双尺寸原图见[视觉对照与原生复测](visual-alignment.md)。下表保留首轮及接力证据，并标注最新复测结果。

## 逐项结果

| # | 原核对项 | 接力状态 | 交接证据与接力复现 |
| --- | --- | --- | --- |
| 01 | 账户列表页首 | 已实测 | #297 的标题栏「添加账户」保留。正文恢复账户用途说明。列表不再用读取时间替换说明。见账户列表对照。 [标题栏入口](../../../experiments/gio/internal/desktop/page_title_actions.go) **接力**　接力在常规和窄窗均打开账户列表，保留 #297 的窗口和标题栏机制。 |
| 02 | 归档开关 | 已实测 | 固定文案「显示已归档」及复选状态。账户列表回归同时覆盖勾选、归档余额和返回。 [列表回归](../../../experiments/gio/internal/desktop/accounts_test.go) **接力**　接力直接在详情归档和恢复。列表开关切换沿用自动化结果。 **视觉复测**　两端窄窗实测开关、只读详情及返回后保持勾选，Gio 同时恢复原行焦点。 [原图与步骤](visual-alignment.md) |
| 03 | 机构分组和净额 | 已实测 | 统一「未指定机构」。保留 Gio 的机构图标和服务计算的组净额。账户列表回归比较 Wails 估值。 [机构和估值对照](../../../experiments/gio/internal/desktop/accounts_test.go) **接力**　从账户列表检查合成机构分组。前轮原图包含有报价分组净额。 |
| 04 | 行、字号和徽标 | 本次修复 | 类型、负债、归档成为独立标签，保留记账方式。估值完整性仍靠近对应金额。见账户列表对照。 [账户行渲染](../../../experiments/gio/internal/desktop/accounts_view.go) **接力**　前轮原图核对账户行、类型、归档徽标。接力复看信用卡行负债符号。 **视觉复测**　对齐列表名称、类型及负债标签、状态和金额对齐，保存两种窗口宽度的前后原图。 [原图与步骤](visual-alignment.md) |
| 05 | 余额、折算和归档金额 | 已实测 | 共享估值读取继续提供原币、折算和部分估值信息。合成 AUD 银行账户、USD 余额和信用卡负号均纳入核对。 [精确余额、估值与归档回归](../../../experiments/gio/internal/desktop/accounts_test.go) **接力**　重开两端账户后核对 AUD 原币及 USD 缺值，另核对信用卡负数。有效汇率折算见前轮原图。 |
| 06 | 机构、成员、类型、分组范围 | 本次修复 | 保留已有范围及清除动作，增加范围标签和成员说明。机构／返回由账户测试覆盖。未做四种范围的逐一原生点击。 [范围过滤](../../../experiments/gio/internal/desktop/account_scope.go) **接力**　接力只核对开户和定存返回的同一账户。四种列表筛选各自的原生交互待测。 **视觉复测**　两端各验证四种范围。Gio 修复键盘清除筛选，并复测组内及机构内详情返回。 [原图与步骤](visual-alignment.md) |
| 07 | 开户类型选择 | 已实测 | 同一开户侧栏，中文枚举改为「证券账户」「数字钱包」「房产及其他财产」。开户选择器用例覆盖可选组合。 [开户选择器测试](../../../experiments/gio/internal/desktop/account_create_chooser_test.go) **接力**　从普通银行账户的产品引导进入开户表单，原生建成持仓银行账户。其他类型和全量组合沿用自动化。 |
| 08 | 选择机构／新建机构 | 合理差异 | 已有侧栏支持选机构、无机构及就地新建。保留 Gio 可搜索选择器与返回机制。开户机构用例覆盖创建及选中。未原生提交新机构。 [机构创建与选择测试](../../../experiments/gio/internal/desktop/account_create_institution_test.go) **接力**　从账户打开时预选原机构，实测保存到该机构。就地新建机构未原生提交。 |
| 09 | 记账方式 | 已实测 | 文案改为「记账方式」「余额」「现金和持仓」。推荐说明随相应选项展示，合法组合仍来自共享领域模型。 [合法方式及推荐选项](../../../experiments/gio/internal/desktop/account_create_chooser.go) **接力**　开户预览和数据库均确认记账方式为现金和持仓。 |
| 10 | 开户详情和所有权 | 已实测 | 开户和编辑仍使用已有所有权控件。开户保留选配分组、份额和纳入净资产信息。设置详情默认收起。自动化覆盖合法组合和精确份额。 [所有权写入和失败重试测试](../../../experiments/gio/internal/desktop/account_maintenance_test.go) **接力**　展开更多详情，核对默认分组、所有人和净资产信息。自定义份额未原生提交。 |
| 11 | 开户确认与建成后 | 已实测 | 确认摘要按宽度使用双列。标准货币格式之外，只有舍入会隐藏非零精度时才补充原始精度。开户用例覆盖提交、读回、新账户 ID 和返回范围。 [开户确认及持久化测试](../../../experiments/gio/internal/desktop/account_shell_create_test.go) **接力**　关闭长表单后回到原入口。重开表单并提交后显示新账户，使用标题栏后退可回原账户。见[开户原图](followup/gio-regular-account-create.jpg)。 |
| 12 | 详情面包屑及工具栏 | 已实测 | 将「账户 /」放到标题栏，移除正文重复的分析和设置按钮。#297 的记录与更多菜单继续使用。见详情对照。 [标题栏入口](../../../experiments/gio/internal/desktop/page_title_actions.go) **接力**　在原生账户详情中打开更多菜单和账户设置。保留 #297 的标题栏、面包屑和已有返回入口。 |
| 13 | 详情摘要与元信息 | 已实测 | 类型／机构与金额分列，读取时间靠近金额，保留分组、所有人及份额。沿用 Gio 原有大金额字形。见详情对照。 [账户摘要渲染](../../../experiments/gio/internal/desktop/accounts_view.go) **接力**　原生核对持仓银行账户的机构、记账方式、所有人和金额，见开户及恢复原图。 |
| 14 | 归档详情和恢复 | 已实测 | 详情提示只读及净资产排除，恢复从设置发起。归档／恢复测试覆盖写入与刷新后读回。 [归档与恢复测试](../../../experiments/gio/internal/desktop/account_shell_maintenance_test.go) **接力**　从设置归档，详情提示只读和排除净资产。SQLite 确认归档值写入，恢复后为空。 |
| 15 | 核对卡片 | 已实测 | 常规宽度让上次核对状态与入口同排，窄宽可换行。保留信用卡账单对账。 [核对卡片与使用范围测试](../../../experiments/gio/internal/desktop/account_detail_test.go) **接力**　完成账单后回原信用卡，最近成功核对状态更新。保留独立账单按钮。 |
| 16 | 回报、资产变化、相关交易 | 未测 | 菜单使用账户范围及「相关交易」文案。回归测试验证交易范围与返回菜单焦点。 [菜单范围与焦点测试](../../../experiments/gio/internal/desktop/account_positions_test.go) **接力**　接力未遍历三项分析菜单。原账户交易范围仍由自动化覆盖。 **视觉复测**　两端实测相关交易限定同一账户及返回，Gio 恢复更多菜单焦点。回报和资产变化仍未原生打开。 [原图与步骤](visual-alignment.md) |
| 17 | 环图和数据表 | 已实测 | 环图显示已估值小计，分组切换位于右上，图例百分比按一位小数舍入。保留完整数据表和缺值状态。见构成对照。 [环图数据和精度测试](../../../experiments/gio/internal/desktop/account_detail_test.go) **接力**　前轮常规窗口构成原图覆盖环图、图例、分组和表格。接力定存现金余额变动后构成随之刷新。 |
| 18 | 外币构成明细 | 已实测 | 同一图例显示 AUD 原币及 USD 折算。本位币条目只显示一个金额。渲染数据断言覆盖精度和缺值。见构成对照。 [原币与折算金额断言](../../../experiments/gio/internal/desktop/account_detail_test.go) **接力**　使用前轮 AUD 折算对照。接力特意使用无汇率的澳元账户，验证缺值单独标出。 |
| 19 | 现金区 | 已实测 | 原币与折算分行，移除多余币种前缀。「校准余额」及 Gio 的直接转账、换汇和债务动作继续可达。现金动作保留现有独立入口，Wails 把部分动作放在菜单。 [现金校准与精度测试](../../../experiments/gio/internal/desktop/account_cash_balance_test.go) **接力**　在新持仓账户提交 USD 存入，从账务回执返回后看到该账户现金余额。 |
| 20 | 持仓列表、分组和排序 | 已实测 | 常规窗口列式行，窄宽使用紧凑堆叠。标的名、代码、分类、现有排序、筛选和组小计均保留。快捷动作移到列表后。 [排序分组与小计测试](../../../experiments/gio/internal/desktop/account_positions_test.go) **接力**　前轮原图覆盖常规和窄窗布局。接力未手动切换全部排序和分组。 |
| 21 | 持仓原位展开与快捷操作 | 已实测 | 账户内展开共用日行情、周期、缺行情、错误及重试。额外保留显式打开证券、转移和调整。自动化验证两类行情及原账户的交易范围。 [有／无行情及导航范围测试](../../../experiments/gio/internal/desktop/account_positions_test.go) **接力**　前轮原图覆盖有行情的原位展开及无行情状态。接力未逐项提交持仓快捷动作。 |
| 22 | 现金历史 | 已实测 | 默认可见。类型、变动、余额、币种、生效、记录六列。原因和备注分行。复用共享金额及时间显示。长列表继续使用虚拟滚动。 [流水列和原因展示断言](../../../experiments/gio/internal/desktop/account_detail_test.go) **接力**　接力从定存结算返回原账户可见现金历史。六列展示及长列表实现沿用前轮对照。 |
| 23 | 「记录变更」选择器 | 已实测 | 从标题栏打开右侧表单，选动作后继续在同一侧栏。账户背景及范围保留。原有分类和精确表单复用。 [选择器和返回测试](../../../experiments/gio/internal/desktop/account_detail_test.go) **接力**　前轮原图从记录入口打开选择器。接力从现金卡进入同一侧栏并写入合成存款。 |
| 24 | 普通账户余额更新 | 本次修复 | #298 后加入账本时区的日期、时间、原因和备注，并在预览核对。测试比较 Wails 提交后的精确余额、原因、备注、时间及 Escape 后未写入。 [生效时间、原因、备注及取消对照](../../../experiments/gio/internal/desktop/account_bank_balance_test.go) **接力**　两端真实保存相同日期、时间、原因、备注与 AUD 精确余额，退出后重开读回。Wails 关闭或确认后焦点回原余额按钮。 |
| 25 | 信用卡／负债余额 | 本次修复 | 信用卡保留正数录入和负债展示符号。日期、时间和备注可编辑，原因仍是负债校准允许的对账原因。测试覆盖多次点击和同一请求重试。 [信用卡负债及幂等测试](../../../experiments/gio/internal/desktop/account_card_balance_test.go) **接力**　Gio 正数欠款转负数展示及预览的精确差额，双击只形成一条活动。修复 900×600 保存后入口移出视口所致的焦点丢失。 |
| 26 | 手工估值、借款和还款 | 未测 | 估值与债务活动留在账户侧栏。保留本金／利息和作用域校验。相关用例覆盖精确确认。未原生提交每种贷款动作。 [债务本金、利息和账户范围测试](../../../experiments/gio/internal/desktop/account_liability_posting_test.go) **接力**　接力未原生提交估值、借款与还款。精确本金及利息回归保留。 |
| 27 | 现金校准、存取、换汇、转账 | 已实测 | 固定账户的编辑搬到侧栏。保留现金校准的专用币种和原因。自动化覆盖多币种配对、返回和焦点。 [跨币种现金读写对照](../../../experiments/gio/internal/desktop/account_cash_balance_test.go) **接力**　实测 USD 存入、读取账务回执和回到原账户。取款、换汇和跨账户转账未在接力原生提交。 |
| 28 | 买入、卖出、现金分红 | 合理差异 | 共用账户侧栏，继续使用限定候选标的与币种的专用买卖表单。从持仓可直达分红。买、卖、分红的精确回执由配对测试覆盖。 [外币买卖精确结算测试](../../../experiments/gio/internal/desktop/account_foreign_trade_test.go) **接力**　保留专用买卖及分红表单，接力未提交这些写入。外币精确配对回归保留。 |
| 29 | 已有持仓、转移、调整 | 未测 | 保留锁定账户或持仓的快捷操作、原单位成本和预览。#298 移除了历史未启用时的旧入口。本次以当前账本历史路径为准。 [已有持仓及原始成本测试](../../../experiments/gio/internal/desktop/account_position_import_test.go) **接力**　接力未原生提交这些持仓动作。沿用 #298 自动启用历史的路径。 |
| 30 | 设置布局与保存 | 已实测 | 从标题菜单打开右侧设置。机构／分组／图标折叠，固定底部操作。自动化覆盖取消、失效引用、保存、读回。 [账户设置侧栏](../../../experiments/gio/internal/desktop/account_maintenance_view.go) **接力**　从标题栏打开账户设置，实测默认焦点与归档控件。普通资料字段独立保存沿用自动化。 |
| 31 | 归档／恢复确认和焦点 | 已实测 | 设置内入口、危险按钮及居中确认。保留历史提示与 Escape。自动化验证取消不写入、受管产品限制和刷新重试。 [归档刷新与复核测试](../../../experiments/gio/internal/desktop/account_maintenance_test.go) **接力**　Escape 返回归档按钮，确认后显示只读。恢复后焦点回原账户更多菜单。见[归档](followup/gio-archive-dialog.jpg)和[恢复](followup/gio-restore-saved.jpg)。 |
| 32 | 无持仓账户的产品引导 | 已实测 | 银行余额与手工估值等非持仓资产账户显示「存款与产品」说明，可打开新持仓账户表单。原账户及其历史保留。尚未原生从此入口完成开户。 [产品引导入口](../../../experiments/gio/internal/desktop/accounts_view.go) **接力**　从 AUD 余额账户的存款与产品区直接新建持仓账户。原账户余额和历史保留。 |
| 33 | 持仓账户产品区和添加 | 本次修复 | 紧凑产品行、空态、共享金额格式的可用现金及账户内添加侧栏。复杂产品条款仍用现有表单。自动化覆盖开立和录入已有产品。 [从账户录入产品测试](../../../experiments/gio/internal/desktop/available_funds_product_test.go) **接力**　从新账户录入已有定存，预览现金仍为 2000 USD，真实保存后查到本金、到期日和价值。修复账户定存侧栏重复标题。复测还真实开立定存，精确扣款 125.25 USD。 |
| 34 | 合约详情、历史和回执 | 本次修复 | 使用共享产品详情和操作回执页，保留预计结算信息。账户带着原范围、滚动和焦点返回。回归测试覆盖保存精确估值后的读回。未原生完成到期实收或撤销。 [产品回执和返回焦点测试](../../../experiments/gio/internal/desktop/managed_product_lifecycle_test.go) **接力**　Wails 原生确认详情抽屉保留账户背景，并修复关闭后焦点。Gio 实测跨页、结算、双回执和原行返回。原因见定存对照。 **视觉复测**　Gio 改用账户内右侧抽屉，实测两种窗口尺寸、窄窗结算和双回执往返、Escape 与读回。 [原图与步骤](visual-alignment.md) |
| 35 | 从账户发起核对 | 合理差异 | 使用预选本账户的核对会话，并保留上次成功核对入口。目标页复用现有账户核对流程。全账户选择属于待办页。 [账户核对作用域测试](../../../experiments/gio/internal/desktop/account_detail_test.go) **接力**　普通核对仍复用带账户范围的共用会话，专项账单完成后最近成功核对状态可读。多账户核对不在本轮。 |
| 36 | 信用卡账单专项 | 本次修复 | 保留独立的账单对账入口。自动化覆盖共享核对会话、证据和恢复。首轮未通过原生界面完整提交账单证据。 [信用卡账单会话测试](../../../experiments/gio/internal/desktop/account_statement_test.go) **接力**　用手工账单证据完成余额和完整性两项核对，保存后从账户读回。修复返回账户时的入口焦点。本版没有文件上传或解析入口。 |
| 37 | 空态、缺值和读取失败 | 已实测 | 持仓提供无日行情提示，现金流水失败有重试。缺估值仍使用共享估值状态。自动化覆盖读取重试和迟到结果。未以线上故障验证。 [迟到读取与缺值等回归](../../../experiments/gio/internal/desktop/account_detail_test.go) **接力**　接力原生看到 USD 折算缺值及产品、持仓空态。读取故障和重试沿用合成回归，未用线上故障复现。 |

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

## 接力原生验证

接力使用 macOS 27.2 ARM64、Go 1.27.4。基线包取自 `188b4413`，修复验证包取自 `86b3371f`。Gio 为 `Nestworth Gio.app`（`com.nestworth.gio`），Wails 为开发身份 `Nestworth.app`（`com.nestworth.dev`）。两次打包和验收分开执行。启动前核对签名、Bundle ID、名称、二进制源码 SHA、数据库与设置真实路径，以及 SQLite 家庭标记。正式应用和发布 DMG／ZIP 未用于此验收。

两端余额写入使用同一合成种子的独立副本。家庭标记为 `Account Parity Synthetic Household`，种子 SHA-256 为 `1689d228c51b08e0db0f87a73ded41fd8fe41f9fa107259dbdd190b78665af3d`。复现时创建一个以 USD 为本位币的合成家庭，账本起点设在 2026-11-15 或之前，使用 UTC 账本时区，并使用 Asia/Singapore 展示时区。添加 AUD 余额银行账户（17400.25）、USD 余额银行账户（925.50）及 USD 信用卡（欠款 37.12）。澳元汇率留空以覆盖缺值。数据库、设置和账单证据均为合成数据。

### 余额和信用卡

1. 打开 AUD 账户的余额更新，按 Escape 关闭。在 Gio 关闭后复核活动总数仍为 3。两端选 `2026-11-20 14:12 UTC`、原因 Interest，填备注 `Synthetic PR299 dated balance` 及目标 `17486.2751 AUD`。预览差额为 `+86.0251 AUD`，确认并退出应用。重开独立账本，两端均显示 A$17,486.28。SQLite 各有一条对应的 `value_update`，原因 `interest`、活动日期 `2026-11-20`，`event` 类型余额为精确的 `17486.2751 AUD`。用于当前显示的 `replay` 投影另有重算时刻。
2. Gio 双击确认后再次进入相同余额预览，显示金额未变，活动计数仍为 4。响应丢失时重试同一提交及同一回执的覆盖来自真实 SQLite 的故障注入回归。
3. Gio 信用卡输入正数 `42.3456 USD`，使用对账原因和备注 `Synthetic PR299 card balance`，选 2026-11-21 并预览。账面由 `-37.12` 变为 `-42.3456 USD`，界面重开后显示 -US$42.35。SQLite 只追加一条对应日期和原因的活动，`event` 余额保存精确欠款数。原始对照见下表。

| 余额与信用卡 | Wails 原图 | Gio 原图 |
| --- | --- | --- |
| 指定历史日期和时刻 | [预览](followup/wails-dated-review.jpg) · [保存](followup/wails-dated-saved.jpg) | [预览](followup/gio-dated-review.jpg) · [保存](followup/gio-dated-saved.jpg) |
| 信用卡欠款与负债 | | [预览](followup/gio-card-review.jpg) · [保存](followup/gio-card-saved.jpg) |

### 开户、长弹层和定存

- Gio 常规 1280×800 点，从 AUD 账户的「存款与产品」打开持仓账户表单，展开更多详情，再关闭并确认回到原入口。重开后创建 `Synthetic PR299 Deposit Bank`，继承机构及默认所有人。从账户设置依次验证归档确认 Escape、提交归档和恢复。归档期间 SQLite 显示该账户归档标记，恢复后再次清空。 [开户](followup/gio-regular-account-create.jpg) · [归档](followup/gio-archive-dialog.jpg) · [恢复](followup/gio-restore-saved.jpg)
- 在新账户存入 2000 USD 并打开现金回执。返回账户后，通过「添加产品」录入 `Synthetic PR299 Mature Deposit`，本金及成本 750.125 USD、当前价值 752.25 USD、起始日 11 月 16 日、到期日 11 月 26 日。滚动条款及可用性规则，勾选现金排除声明。预览和写入前后现金同为 2000 USD。 [定存长表单](followup/gio-regular-deposit-scrolled.jpg) · [录入预览](followup/gio-deposit-create-review.jpg)
- 同一到期合同在 Wails 从账户打开为侧栏，账户背景和滚动位置保留。Gio 保留到「可用资金」的现有详情路由，因为该页统一管理合约状态、预计结算、实收写入、操作历史和回执导航。直接从账户复用该路由可沿用现有回执返回栈和单一的结算状态。原生打开时显示到期未实收，后退回到原账户的定存行及其焦点。 [Wails 账户背景](followup/wails-product-context.jpg) · [Gio 到期详情](followup/gio-existing-cross-page-detail.jpg) · [Gio 返回产品行](followup/gio-existing-product-return.jpg)
- 在 Gio 到期详情选择「确认实收」。本金 750.125 USD、利息 9.875 USD、费用 0.125 USD，结算后现金 `2759.875 USD`。分别打开本金与利息回执，并返回到原产品和账户，产品已结算。SQLite 中该合同只有一次 `record_existing` 和一次 `settle`。 [实收预览](followup/gio-deposit-receipt-review.jpg) · [已结算](followup/gio-deposit-settled.jpg) · [本金回执](followup/gio-deposit-principal-receipt.jpg)

### 信用卡账单证据

从原信用卡进入账单核对，创建会话，选择 11 月 22 日，输入合成账单来源和备注，核对余额与完整性，预览、确认，再完成会话。回到账户显示最近成功时间。SQLite 读回一个已完成会话、两项已确认的证据及精确金额 `42.3456 USD`。该入口支持手工填写账单证据，没有文件上传或解析入口。 [账单预览](followup/gio-statement-review.jpg) · [已完成](followup/gio-statement-complete.jpg)

### 修复提交上的原生复测

- Gio 900×600 点，信用卡保存并重新加载后，「更新信用卡余额」可见且获得焦点；从完成的账单记录返回到账户，焦点回「账单核对」。复测使用独立账本副本，原先两项已确认的账单证据仍能读回。 [信用卡保存焦点](followup/gio-fixed-card-focus.jpg) · [账单返回焦点](followup/gio-fixed-statement-focus.jpg)
- Gio 窄窗长定存表单只显示一个「添加产品」标题，滚到底部仍可关闭，返回后原账户、产品区滚动和「添加产品」焦点保留。随后用账户现金开立 125.25 USD 定存，到期日为 2026-11-28。现金由 2759.875 USD 精确扣为 2634.625 USD，SQLite 确认一次 `open`，产品显示在同一账户。 [单标题](followup/gio-fixed-narrow-product.jpg) · [窄窗滚动](followup/gio-fixed-narrow-product-scrolled.jpg) · [开立预览](followup/gio-fixed-open-deposit-review.jpg) · [保存读回](followup/gio-fixed-open-deposit-saved.jpg)
- Wails 窄窗的同一合同抽屉可滚至操作历史，Escape 回到原产品按钮和账户滚动位置。余额抽屉分别验证 Escape 与实际保存，焦点均回原余额按钮。复测精确保存 17500.1234 AUD。 [滚动原图](followup/wails-fixed-product-scrolled.jpg) · [合同焦点](followup/wails-fixed-product-focus.jpg) · [余额 Escape 焦点](followup/wails-fixed-balance-focus.jpg) · [余额保存焦点](followup/wails-fixed-balance-saved-focus.jpg)

### 证据说明

`native/` 保存首轮原始 PNG。`followup/` 保存此次原生窗口捕获的原始 JPEG，均保留捕获时的像素尺寸。Gio 常规和窄窗的捕获分别为 2560×1600 和 1800×1200 像素。Wails 同样设置 1280×800 与 900×600 内容区，捕获包含原生标题栏。

## 接力检查与剩余范围

- 修复源码 `86b3371f` 在 macOS 27.2 ARM64 通过 Gio 全模块 `go test -mod=readonly -count=1 ./...`，包含前轮曾失败的字体用例。账户定向测试也通过。
- Wails 前端通过 lint、类型检查、生产构建与全量 Vitest（206 个文件、2304 个测试）。Vitest 为 jsdom 证据。两端开发包构建和签名检查与上述测试独立。
- 接力基线 `188b4413` 的 [Gio CI](https://github.com/MnemosyneLab/NestworthGo/actions/runs/45978127951) 通过。[常规 check](https://github.com/MnemosyneLab/NestworthGo/actions/runs/45978127946) 在正确性并行阶段取消，汇总及证据保留步骤失败。该运行不能作为通过证据。
- 账单文件导入在本版没有入口。逐类开户、就地新建机构、各类贷款与跨币种交易提交、定存撤销和 VoiceOver 未在接力原生验证。未运行 Linux 原生窗口或发布打包验收。修复仅调整账户操作标题和焦点恢复，没有改动账务写入或回执逻辑。
