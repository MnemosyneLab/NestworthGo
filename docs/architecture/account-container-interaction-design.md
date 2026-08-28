# Account 容器交互契约

## 1. 文档状态与权威边界

- 状态：Implemented / current contract
- 配套领域契约：[account-container-and-position-model-design.md](account-container-and-position-model-design.md)
- 适用基线：Nestworth-go `0.2.1` / schema v7 / 当前 Wails 前端
- 数据策略：这是未发布版本的 breaking cutover。只支持全新的 schema v7 数据库；不设计旧数据、旧交互或旧页面兼容层。

本文冻结 Account 容器模型在桌面端的创建、查看和操作方式。三元组合法性、分类、inclusion、tracking 不可变等规则仍以领域契约为准；本文不重新定义领域模型。

产品原则：

> 创建时让用户描述「这是什么现实账户、想记录多细」；详情页展示「这个账户里面有什么」；Overview 再按底层资产重新聚合。

Asset Type 与 Account Type 是两套视图，不能混在一起。不引入 SubAccount；用户看到的始终是一个现实 Account 及其内部现金和持仓。

## 2. 最终决策摘要

1. Account 是主导航对象；Accounts 列表点击后进入 Account 详情，而不是直接打开元数据编辑。
2. 创建流程先选 Institution，再选现实账户类型。普通用户不直接选择 `balance_sheet_role` 或内部 tracking 枚举。
3. 银行账户和数字钱包询问「只记录总余额」还是「分别记录现金和持仓」；券商、投资账户和加密交易所默认记录现金和持仓。
4. `tracking_mode=holdings` 的 Account 统一显示 Cash + Investments。银行综合账户与券商账户使用同一套详情结构。
5. `tracking_mode=balance` 和 `manual_value` 的 Account 使用轻量 Simple 详情，只显示当前余额或估值。
6. Account 详情必须提供真正的 Buy / Sell 主路径。「记录已有持仓」是另一项次要操作，不能冒充买入，也不会扣减现金。
7. Composite Account 必须支持按币种记录现金。Account 默认币种是默认输入和展示上下文，不是该账户现金的唯一允许币种。
8. History 尚未开始时，允许录入期初现金、期初持仓和期初估值；交易类动作先明确启动 History，再返回原操作继续。
9. Overview 默认按底层 Asset Type 展示；另设 By institution 与 By account type，标题必须写清聚合维度。
10. Portfolio 是独立页面，只包含整户 `include_in_portfolio=true` 的资产账户；Composite Account 的现金和持仓会整户进入。
11. 金额、市值、完整性和分类由后端 read model 提供。前端只组合和展示，不重算财务权威值。
12. 全家持仓表可以保留为「所有持仓索引」，但不再承担 Account 详情或 Portfolio 的职责。

## 3. 当前实现与边界

当前 `0.2.1` 实现已经接入 Account 容器交互闭环：

- `AccountsPage` 按 Institution 分组；点击账户进入同一 workspace 的详情页，创建使用 Institution-first wizard。
- 创建向导从 Catalog 读取合法组合，以现实账户类型和记录方式提问；Role、Tracking 和 inclusion 使用后端 Catalog 结果，所有人必须至少选择一名。
- `AccountDetail` 对 Holdings Account 展示按币种的 Cash 和按 Instrument 的 Investments；Balance / Manual Value Account 展示单一当前值。金额、完整性和缺失原因来自后端 valuation DTO。
- 详情页提供期初现金、现金校准、存取款、换汇、转账、买入、卖出、记录已有持仓和 Simple value 更新入口；需要 History 的动作会先显示 Start History。
- Overview 提供 component 粒度的 `assetsByType` / `liabilitiesByType`，并保留账户级 `byAccountType`、`byInstitution` 和 `byGroup`；Portfolio 是独立导航页面并按整户 inclusion 工作。
- 归档账户可以查看但以只读方式展示；账户设置、归档和恢复位于详情页，而不是列表行的隐式编辑操作。

持续有效的边界如下：

- Simple Account（`balance` / `manual_value`）的值必须使用 Account 默认币种；Holdings Account 的 Cash component 可以使用系统支持的其他币种。
- `tracking_mode` 创建后不可变；本版本不支持 tracking transition 或 component-level inclusion。
- Holdings Account 的 Portfolio inclusion 仍是 whole-account 语义；现金和全部持仓一起进入或一起排除。
- 历史 Simple Account 的 bucket 名称来自当前 metadata，并标记为 `current-metadata-derived`。

## 4. 目标与非目标

### 4.1 目标

- 用户能按现实账户心智创建银行、券商、数字钱包、房产、信用卡等账户。
- 用户能在 Account 详情看到多币种现金、持仓数量和市值。
- 用户能在综合银行账户中记录现金，并直接买入基金、理财、黄金等标的。
- 用户能清楚区分期初头寸、余额校准、存取款、买卖和换汇。
- Overview 与 Portfolio 的聚合维度清晰且与领域分类一致。
- 所有不可变规则在创建时被说明；编辑页不提供注定失败的控件。

### 4.2 明确不做

- 不做 schema v6 迁移、旧数据转换或旧 UI 兼容。
- 不引入 SubAccount。
- 不支持创建后转换 tracking mode。
- 不支持 component 级 inclusion；三个 inclusion 仍是整户开关。
- 不新增 Activity kinds。
- 不根据 Institution 名称猜测或强制 tracking。
- 不把现金行扩展为「活期/定期」子账户模型。

## 5. 产品语言

主路径使用左侧产品语言；写入仍使用右侧领域值。Catalog 的合法组合是创建和编辑的唯一选项来源。

| 用户看到的 | 领域值 |
| --- | --- |
| 只记录账户总余额 | `tracking_mode=balance` |
| 分别记录现金和持仓 | `tracking_mode=holdings` |
| 只记录一个估值 | `tracking_mode=manual_value` |
| 资产 / 负债（仅 Other 创建时显式选择） | `balance_sheet_role` |
| 计入净资产 / 投资组合 / 流动资产 | 三个 Account inclusion 开关 |

主路径禁止出现：`holdings`、`balance`、`manual_value`、`tracking mode`、`balance sheet role`、SubAccount、Investment-only。

设置页可以使用更正式的词，但仍显示产品名：

| 设置项 | 示例 |
| --- | --- |
| Account type | 银行账户、券商账户、信用卡 |
| Tracking method | Detailed positions / Account total / Manual value |
| 不可变提示 | This cannot currently be changed after account creation |

## 6. 信息架构与导航

```text
Overview
Accounts          现实账户列表与详情
Portfolio         被纳入投资组合的资产账户
History           资金与头寸变化
Instruments       Household 级标的目录
Market data
Settings
```

- Accounts 是「钱在哪里、账户里有什么」的主入口。
- Portfolio 是 `include_in_portfolio` 范围内的投资视图。
- Instruments 是可复用标的目录，不代表标的已经被任何账户持有。
- 全家持仓表如保留，应命名为「所有持仓索引」，作为跨账户查询工具，而不是 Portfolio 或 Account 的替代品。

## 7. 创建 Account

创建使用分步向导，不再用一张表直接暴露三维领域字段。创建成功后，Composite 进入空的 Cash + Investments 详情；Simple 进入余额/估值详情。

### 7.1 流程

```text
1. Where is it held?              Institution（可选，可当场新建）
2. What kind of account is it?    Account type
3. How would you like to track it? 仅在有两种常见记法时出现
4. Account details                名称、默认币种、所有人、inclusion
5. Review and create
```

不可变选择必须在 Review 中用自然语言回显，例如：

```text
招商银行 · 银行账户
分别记录现金和持仓
创建后暂不能改为只记录总余额
```

### 7.2 Institution

- 第一步列出已有 Institution，并提供「没有对应机构」和「新建机构」。
- Institution 保持可选。现金、房产、车辆、收藏品常常没有托管机构；银行、券商、信用卡应提示选择，但不强制。
- Institution 不决定三元组。同一机构下可以同时有银行账户、信用卡和投资账户。

### 7.3 Account type、Role 与记录方式

主列表：

```text
银行账户
券商账户
投资账户
加密交易所
数字钱包
房产
车辆
信用卡
贷款
其他
```

「更多」中保留 Catalog 里同样合法但较少使用的类型：现金、养老金、保险、收藏品、应收款。前端不得硬编码一个缩水枚举；展示集合和合法组合来自 Catalog。

除 `other` 外，Role 由 `DefaultBalanceSheetRole(type)` 自动写入。`other` 必须询问「这是资产还是负债」，再显示对应合法记录方式。

| `account_type` | 主路径记录方式 | 创建时可选项 |
| --- | --- | --- |
| `cash_on_hand` | 只记录总余额 | 无 |
| `bank_account` | 询问；默认只记录总余额 | 总余额 / 分别记录现金和持仓 |
| `digital_wallet` | 同银行账户 | 总余额 / 分别记录现金和持仓 |
| `brokerage` | 分别记录现金和持仓 | 高级：只记录一个总市值 |
| `investment_account` | 分别记录现金和持仓 | 高级：只记录一个总市值 |
| `crypto_exchange` | 分别记录现金和持仓 | 无 |
| `pension` | 分别记录现金和持仓 | 高级：只记录一个总额 |
| `insurance_policy` / `property` / `vehicle` / `collectible` | 只记录一个估值 | 无 |
| `receivable` | 只记录总余额 | 高级：按估值记录 |
| `credit_card` / `loan` | 只记录总余额 | 无 |
| `other` + asset | 必须选择 | 总余额 / 现金和持仓 / 一个估值 |
| `other` + liability | 只记录总余额 | 无 |

银行和数字钱包的提问：

```text
你希望怎样记录这个账户？

○ 只记录账户总余额
  适合普通存款或单一余额账户

○ 分别记录现金、基金、理财、黄金等
  适合综合账户。创建后不能改成只记录总额。
```

券商、投资账户和加密交易所直接进入详细记录方式，不要求用户理解 tracking。

### 7.4 Details 与 Inclusion

主要字段：名称、默认币种、所有人。Institution 回显且可返回修改。分组、图标、图片及三个 include 放在「更多设置」。

所有人必须由用户明确勾选，至少一名；默认不预选任何人。未勾选时 Continue / 添加账户为 disabled，不可点击。创建提交路径也必须拒绝空所有权；仅禁用按钮不够。不得把空列表默认成「全体家庭成员、平均分配」。已勾选的多名所有人，比例留空仍可在已勾选的人之间平均分配；也可填写明确百分比（例如 70/30），总和必须为 100%。空所有人集合是禁用主按钮，不是 §14 的原位错误、也不是无响应的 Continue。

Inclusion 初始值使用 `SuggestedInclusion`。Composite Account 勾选投资组合时必须显示整户说明：

```text
计入投资组合

此账户内的全部现金和持仓都会进入投资组合，
而不是只计算基金、理财或其他投资持仓。
```

Simple Account 可以在创建流程录入初始余额或估值。Composite Account 不录入一个虚构总额；创建后进入详情添加现金或期初持仓。

## 8. Accounts 列表

Accounts 页是现实账户入口，不是元数据编辑表。

- 默认按 Institution 分组；没有机构的账户放在「未指定机构」。
- 每行显示名称、Account type 产品名、家庭本位合计和数据完整性状态。
- Account type chip 只表示账户类型，不能用 Cash / Stock 等底层资产类型冒充。
- 点击进入详情；编辑、归档、图片等操作放在详情的 Account settings。
- archived Account 默认不出现在活跃列表；查看归档项时详情为只读，除非先恢复。
- 合计不完整时显示「部分估值」及未计价原因，不把缺失金额当作零。

## 9. Account 详情

### 9.1 通用页头

```text
MooMoo SG Brokerage                         128,420 CNY

MooMoo SG · 券商账户
计入净资产 · 计入投资组合
```

标题合计使用后端 Account valuation。页头还应容纳：

- `asOf` 时间；
- 完整 / 部分估值状态；
- archived 只读状态；
- Account settings 入口；
- 整户 inclusion 提示。

### 9.2 Composite：Cash + Investments

所有 `tracking_mode=holdings` 的 Account 使用同一布局：

```text
Cash
------------------------------------------------
SGD                              12,000 SGD
USD                               8,500 USD
CNY                               3,000 CNY

[Add cash balance]  [Deposit or withdraw]  [Convert]

Investments
------------------------------------------------
NVDA     Stock          20       3,648 USD
QQQ      ETF            15       ...
招银理财A Bank product   1       200,000 CNY

[Buy investment]  [Record existing position]
```

招商银行综合账户与券商使用同一套结构。差异只来自内部现金和 Instrument 类型，不来自另一套 Account 页面。

现金行按币种聚合；持仓行至少显示 Instrument、类型、数量、当前市值与估值状态。若当前报价缺失，显示数量和「缺少当前价格」，不显示推算的零市值。

空状态必须带下一步动作：

```text
Cash          还没有现金余额       Add cash balance
Investments   还没有持仓           Buy investment
```

### 9.3 Cash 操作的语义

不同动作不能都叫「Add cash」：

| 用户动作 | 意义 | 现有命令/写入 |
| --- | --- | --- |
| Add cash balance / Reconcile balance | 把某币种当前余额校准到结果值 | `AppendAccountCashValue` |
| Deposit | 外部资金进入账户，输入变化额 | `MoneyAdded` |
| Withdraw | 资金离开账户，输入变化额 | `MoneyRemoved` |
| Convert currency | 同账户或账户间换汇 | `FXConversion` |
| Transfer | 两个账户间移动现金 | `CashTransfer` |

「Add cash balance」表单先选币种，再输入该币种的结果余额。Composite 可选择任何支持币种，默认选 Account 默认币种。确认页需说明 History 已开始后会把差额记录为 reconciliation。

### 9.4 Buy / Sell 与「记录已有持仓」

这两类操作必须分开：

- **Buy investment / Sell** 是交易。它改变现金和持仓，走现有 `ChangeTrade`。
- **Record existing position** 是录入期初或修正当前持仓数量，不扣现金。它走 `CreateHolding` / Position Adjustment。

「记录已有持仓」是次要或高级入口，文案明确：

```text
记录账户中已经存在的持仓数量。
这不会记录买入交易，也不会扣减现金。
```

主路径不创建零数量占位 Holding：

- Record existing position 要求数量大于零；
- 用户首次 Buy 一个该账户尚未持有的 Instrument 时，应用层在同一事务内创建 Holding 并完成 Trade；
- Instrument 可以从 Household 目录选择，也可以在 sheet 中创建最小必要信息；Instrument 不是 SubAccount。

因此「银行账户能买基金」的完整闭环是：创建银行综合账户 → 记录对应币种现金 → Buy investment → 选择/创建基金 → 输入数量与成交信息 → 完成交易并刷新现金、持仓和估值。

### 9.5 History 尚未开始

History 未开始时，详情允许录入期初状态：

- 期初现金余额；
- 已有持仓及数量；
- Simple Account 的初始余额或估值。

Deposit、Withdraw、Buy、Sell、Convert、Transfer 等事件型动作需要 History。用户触发这些动作时：

1. 显示为什么需要开始 History，以及将使用的起始日期；
2. 打开现有 Start History 流程；
3. 成功后返回原 Account 和原动作，保留安全的已填字段；
4. 用户取消则不写入、不静默开始 History。

不得用「请先去 Settings 开启」把用户赶出当前任务。

### 9.6 Simple 详情

`balance` 与 `manual_value` 不显示 Cash / Investments：

```text
自住房                                      5,000,000 CNY

房产
上次估值 5,000,000 CNY · 2026-08-27

[Update value]
```

```text
招商银行信用卡                                  8,420 CNY

信用卡 · 负债
当前余额 8,420 CNY

[Update balance]
```

负债用绝对值显示，并用「负债」说明其净资产方向；除非全应用统一符号规则，不在详情标题单独引入负号。

- History 未开始：写入初始 Account value。
- History 已开始：余额型使用现有 Balance Adjustment；估值型使用 Manual Valuation / Value Update。

### 9.7 Account settings

设置中：

- 可改：名称、Account type（仅仍合法的兼容项）、Institution、分组、所有人、三个 inclusion、图标/图片；
- 只读：Tracking method，并显示「创建后暂不能更改」；
- 只读：Role；`other` 的 Role 也不允许创建后修改；
- Account type 更新不重算 inclusion、不产生 Activity、不改变金额；
- holdings Account 的 Portfolio inclusion 旁始终显示整户提示。
- 所有人与创建向导同一闸门：必须至少勾选一名；Save 在未勾选时 disabled，更新提交路径同样拒绝空所有权。不得把空列表默认成全体家庭成员。已勾选多人时，比例留空仍可在已勾选者之间平分，或填写明确比例。

## 10. Overview

Overview 继续以净资产、资产和负债为顶层合计。资产区的默认主图按底层资产展示：

```text
Asset allocation          assetsByType，component 粒度
Cash
Stocks
ETFs
Mutual funds
Bank products
Crypto
Precious metals
Property
...

By institution            byInstitution，Account 粒度
招商银行
MooMoo SG
DBS
未指定机构

By account type           byAccountType，Account 粒度
银行账户
券商账户
房产
...
```

负债保持 `liabilitiesByType`，不并入 Asset allocation。

UI 必须解释维度差异：

> Asset allocation 按现金和标的类型汇总；By account type 按现实账户种类汇总。一个银行综合账户会同时出现在「银行账户」和多个底层资产行中。

`byAccountType` 必须由后端加入 `OverviewResult`，而不是前端从 Account 列表重算。规则：

- 只聚合 `include_in_net_worth=true` 且 role=asset 的 Account；
- 按 Account 的当前 valued subtotal 整户归入 `account_type`，Composite 不拆分；
- 使用与 Overview 顶层资产相同的 as-of、换算和 incomplete 语义；
- 缺失估值不按零处理；
- 百分比分母使用同一结果中的资产 valued subtotal。

By member / by group 可以保留为次要块，但不与 Asset allocation 抢主位。

## 11. Portfolio

Portfolio 是独立页面，直接使用现有 `PortfolioService.Portfolio`，只展示 `include_in_portfolio=true` 且 role=asset 的 Account。

```text
Portfolio                              320,000 CNY

Allocation                             byInstrumentType
Cash            12%
Stocks          35%
ETFs            40%
Crypto           5%
...

Accounts
MooMoo SG Brokerage                   180,000 CNY
IBKR                                  140,000 CNY
```

- 点击 Account 进入同一 Account 详情。
- Composite Account 被纳入时，现金和全部持仓都会出现；页面和设置都重复整户说明。
- 未纳入的 Account 不显示，也不进入分母。
- 缺失报价或汇率时沿用后端 incomplete / excluded amount 语义，不把它们当作零。

## 12. 动作与现有领域命令映射

| 详情入口 | 领域意图 | 约束 |
| --- | --- | --- |
| Add cash balance | `AppendAccountCashValue` | Composite 任意支持币种；输入结果余额 |
| Deposit / Withdraw | `MoneyAdded` / `MoneyRemoved` | History 已开始；输入变化额 |
| Convert | `FXConversion` | History 已开始 |
| Transfer | `CashTransfer` | History 已开始 |
| Buy / Sell | `ChangeTrade` | History 已开始；首次 Buy 原子创建 Holding |
| Record existing position | `CreateHolding` / Position Adjustment | 不动现金；数量 > 0 |
| Update Simple value | Account Value / Balance Adjustment / Manual Valuation | 使用 Account 默认币种 |

不得新增 Activity kinds 来支撑这些界面。详情页只负责预填 Account 上下文并选择正确命令，不复制一套不同的 History 语义。

## 13. Read model 与 API 接缝

### 13.1 前端可先组合的现有能力

| 表面 | 权威来源 | 前端职责 |
| --- | --- | --- |
| 创建向导 | Catalog `accountCombinations` + `CreateAccount` | 展示现实语言，提交合法组合 |
| Accounts 列表 | `ListAccounts` + Institution + Account valuations | 分组与导航 |
| 详情合计/完整性 | `AccountValuation(s)` | 直接展示 |
| 当前现金 | valuation 中的 cash components | 按币种展示 |
| 持仓数量 | `HoldingsByAccounts` | 与 valuation component 按 `holdingId` 对齐 |
| Instrument 元数据 | Instrument 列表/详情 | 显示名称、类型、报价币种 |
| 当前价格 | 当前 quote read model | 有则展示；无则显示缺失 |
| Portfolio | `PortfolioService.Portfolio` | 页面布局与跳转 |

`ListAccountCashValues` 是观察历史，不应由前端自行挑一条「最新记录」作为当前现金权威值。当前状态使用与 Account valuation 同一 as-of 的 cash components，避免历史排序、币种缺口和刷新时序产生两套真相。

### 13.2 当前后端事实

- Composite cash 的多币种校验由应用层和领域层共同保证，并覆盖 History 开始前后的写入路径。
- `OverviewResult.ByAccountType` 由后端生成，与 Overview 其余结果共用换算、as-of 和 incomplete 规则。
- Account 详情使用现有 Account、Holding、Instrument 和 valuation read models 组合展示，不新增独立的财务写入模型。

### 13.3 前端禁止事项

- 不自己把数量乘报价生成权威市值；
- 不自己做汇率换算或净资产加总；
- 不把缺少报价、汇率或 component 的金额当作零；
- 不从 append-only 观察记录推断当前余额；
- 不复制 `IsValidAccountCombination`、分类或 SuggestedInclusion 规则；
- 不在 Account type 更新后重算 inclusion；
- 不使用浮点数进行金额计算。

单位价格只显示后端当前 quote；没有可靠 quote 时可以省略该列或显示「缺少当前价格」，不能用市值和数量反推展示值。

## 14. 状态、错误与可访问性

所有新页面和 sheet 必须覆盖：

- loading skeleton；
- 无现金、无持仓、无 Portfolio Account 等空状态；
- 部分估值、缺少 quote、缺少 FX 的解释；
- API validation error 原位展示，并保留用户输入；
- History 未开始的可恢复流程；
- archived Account 的只读状态；
- 写入成功后只使相关 Account、Overview、Portfolio 和 History query 失效；
- 防重复提交与明确的进行中状态。

交互与可访问性要求：

- 所有操作可用键盘完成，sheet 打开后聚焦标题或首个字段，关闭后焦点返回触发按钮；
- 不只用颜色表达资产/负债、included/excluded、完整/不完整；
- tab、radio、menu 使用正确语义和 `aria-selected` / `aria-checked`；
- 金额同时显示币种代码或无歧义符号；
- 删除、归档等破坏性操作不与 Buy / Update value 等主操作并排使用相同视觉权重。

## 15. 行为矩阵

### 15.1 创建与不可变规则

- 银行账户默认询问记录方式，默认只记总额；选择详细记录后创建为合法 holdings 组合。
- 券商不询问内部 tracking 术语，默认进入 Cash + Investments。
- 信用卡自动创建为负债；Other 显式选择资产或负债。
- 主路径不出现内部枚举或 SubAccount。
- 编辑不能修改 Role / Tracking；兼容 type 更新不产生 Activity、不改变金额、不重算 inclusion；非法 type 更新被拒。
- 未勾选所有人时不能 Continue / 添加账户 / Save；创建和更新的提交路径拒绝空所有权。不得把空所有人列表默认成全体家庭成员平分。已勾选多人且比例留空时，在已勾选者之间平均分配仍被允许。

### 15.2 Composite Account

- 默认币种 SGD 的 MooMoo Account 可同时录入 SGD、USD、CNY 现金。
- MooMoo 和招行综合账户使用同一详情结构，并能显示 Instrument 类型不同的持仓。
- 当前现金来自 valuation cash components；持仓数量与估值能稳定对齐。
- 缺 quote / FX 时显示部分估值与原因，金额不按零处理。
- Record existing position 不扣现金，且不允许零数量占位。
- 首次 Buy 未持有的基金时自动创建 Holding，现金与数量在同一成功操作后更新。

### 15.3 History 边界

- History 未开始时可记录期初现金、已有持仓和 Simple 初始值。
- 从详情发起 Buy / Deposit 等动作会进入 Start History，完成后返回原动作；取消不写入。
- History 已开始后的 cash reconciliation、Trade、FX 和 Transfer 产生现有 Activity kinds，不新增种类。

### 15.4 Overview 与 Portfolio

- 招行综合账户整户出现在 By account type 的「银行账户」，其 cash / fund / gold components 同时进入 Asset allocation 对应行。
- `byAccountType` 与顶层 Overview 使用相同 as-of、换算和 incomplete 语义。
- Portfolio 不含未勾选 Account；勾选 Composite 时现金和持仓整户进入，并显示说明。
- 所有持仓索引与 Portfolio 使用不同名称和目的，不再让用户误以为两者相同。

### 15.5 端到端关键旅程

```text
创建招商银行综合账户
→ 选择「分别记录现金、基金、理财、黄金等」
→ 添加 CNY 现金余额
→ 点击 Buy investment
→ 若需要，完成 Start History 并返回
→ 选择或创建基金，完成首次买入
→ Account 详情同时显示减少后的现金与基金持仓
→ Overview 按 Cash / Mutual fund 分类
→ 若整户 included，Portfolio 同时包含该账户现金与基金
```

## 16. 明确延期项

以下内容可以以后单独设计，不阻塞本方案：

- tracking mode 转换；
- component 级 inclusion；
- cash component 的用户别名或存款产品子类型；
- tax lot、订单状态、费用拆分等更完整交易模型；
- Account 详情永久 URL / 多窗口路由；第一版可在 Accounts workspace 内全幅切换；
- 更复杂的 Account detail 聚合 DTO，除非现有 read models 无法保证一致 as-of。
