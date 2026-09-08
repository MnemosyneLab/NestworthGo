# Analytics Phase 1–6 整体 Review

更新日期：2026-09-08。范围：Phase 1a、1b、2a、2b、3、4、5、6，包含内核、投影、六个标签页与最终导航切换。

基线提交：`699917f9fab670b0def3019d2bd340be167fb6cb`。审查开始时工作区干净；重点增量为 `1f80646..699917f`，并重新运行上一版 Phase 1–4 的边界探针。为保留已有链接，文件路径仍使用 `analytics-phase-1-to-4-review.md`。

本次只更新此文档，没有修改产品代码、正式测试、设计计划或用户数据库。审查探针通过 `/tmp` 下的 Go overlay 和 Vite 内存加载补入。前端标准检查自动运行了 bindings 生成，生成结果未产生 Git 差异。

## 1. 结论

**六个标签页及旧 Analysis 的替换已实现；建议下一步集中修复正确性问题并做六页统一验收，再进入新的产品计划。无需推翻现有架构。**

上一版 10 项发现中，**F01–F09 仍存在，F10 已关闭**。本轮新增 F11–F13 三项，当前共有 **12 项未解决发现：3 项 P1、9 项 P2**。P1 仍是缓存污染、未知金额变零和实物转仓本金遗漏；新增问题主要在 Contribution 导航、不可用状态恢复以及数据完整性提示。

全量前端 **332/332 通过**，类型检查、定向 ESLint、三个后端包的 race 检查通过，但定向审查探针仍能击穿现有测试。首次常规 Go 套件因性能断言失败，单独复跑性能用例通过，详见 §6，不能只报“全部绿色”。

“需求代码已完成”和“已完整验收”需要分开：开发计划顶部写 Phase 1a–5 code complete，但 Phase 4/5/6 小节仍标注 desktop smoke pending，Phase 5 还明确要求桌面通过前不能标记 Complete。本次未运行 Wails 桌面、真实家庭数据、原生图表和键盘布局验收。

## 2. 范围与已确认的基础

依据：[开发计划](../design/analytics/analytics-development-plan.md)、[技术架构](../design/analytics/analytics-redesign-architecture.md)、[产品设计](../design/analytics/analytics-product-design.md) 及相关实现/测试。

| 阶段 | 检查重点 | 结果与边界 |
|---|---|---|
| 1a | Universe、分类器、范围资本流、Price/FX、Residual、快照输入 | F03；application/domain 现有测试及实物转仓探针 |
| 1b | 日收益、Dietz 本金、几何链接、缺失数据、分组折叠 | F02/F03；没有穷举全部合法账本组合 |
| 2a | Asset projections、memo、失效、明细、DTO | F01/F07；新增 Asset Trend rate 投影有 coverage 门槛 |
| 2b | Return projections、valuation fallback、coverage、Contribution | F01/F02/F11；明确不可用的成本基础不作为缺陷 |
| 3 | Calendar 月/年、日期、共享筛选、日明细 | F05/F06 仍在 |
| 4 | Summary、瀑布、分组、Residual、History | F04/F07/F08/F09 仍在，F10 日期测试已修复 |
| 5 | Return Trend、Contribution、Asset Trend、Categories | 四页已接 API；F06/F08 传播，新增 F12/F13 |
| 6 | Calendar → Drivers → Contribution、Sheets → History、旧页移除 | App 和导航已切换；F11 暴露多账户 drill-down 缺口；native gate 未执行 |

值得保留的实现：

- `AssetBucket`、`ReturnComponent`、`DietzCapitalFlow` 职责分离，后端十进制金额经 Wails 字符串 DTO 传递。
- Price/FX 按路径计算，Residual 独立，memo 继续有容量及 generation；修复应落在现有机制内。
- activity/quote/valuation 失效覆盖 `queryKeys.analysis.all`，新页继续使用同一查询体系。
- 新 Return Trend sources 来自 Price/Dividend/FX/Fees，而非直接冒用 Calendar 的持仓贡献榜。
- Asset Trend level 使用期间末值、flow 使用期间和；新增月/周 rate 使用几何链接和至少一半日期有率的门槛。
- Contribution 明细已改为 return component 构成；不再把 `price_change` 等归因 key 当 History activity kind。多账户范围仍有 F11。
- `AnalyticsPage` 及旧 nav 已删除，Investments 的 `HoldingGain` / `AccountGains` 保留；全量前端测试包含切换后的页面和导航用例。

## 3. 发现总表与上一版状态

| ID | 优先级 | 问题 | 本轮状态 / 阶段 |
|---|---|---|---|
| F01 | P1 | Return fallback 污染完整资产的 base 缓存 | 仍存在；2a/2b，传播至 3–5 |
| F02 | P1 | 全部估值未知的一天返回可用零金额 | 仍存在；1b/2b/3/5 |
| F03 | P1 | 实物转仓未计入范围 Dietz 本金 | 仍存在；1a/1b/2b/5 |
| F04 | P2 | 负数或跨零瀑布图堆叠错误 | 仍存在；4 |
| F05 | P2 | 年视图月卡显示低 coverage 收益率 | 仍存在；3 |
| F06 | P2 | 默认范围早于 History Origin | 仍存在；3/4/5，named range 仅局部处理 |
| F07 | P2 | 净额零隐藏非零分组和 Residual 入口 | 仍存在；2a/4 |
| F08 | P2 | 明细刷新后 Total 使用旧 row | 仍存在并扩展至 Categories；4/5 |
| F09 | P2 | 瀑布自行生成未知期末金额 | 仍存在；4 |
| F10 | 原 P2 | 页面测试依赖运行日期 | 已关闭；4，固定查询范围后本轮通过 |
| F11 | P2 | Contribution 的 History hint 错误缩窄聚合范围 | 新增；2b/5/6 |
| F12 | P2 | 不可用状态移除类型控件，无法在原 tab 切回 | 新增；5 |
| F13 | P2 | 新页遗漏 partial / forced-base 提示 | 新增；5 |

F01/F02/F03/F07/F11 有后端定向复现；F05/F06/F08/F09/F12/F13 有组件或函数探针；F04 有当前安装 ECharts 的 SSR 计算证据。新增 finding 表示本轮首次记录，不意味着其所有根因都在 Phase 5/6 才引入。

## 4. 详细发现与修复方案

### F01 — [P1] Return fallback 的 base 缓存必须与完整资产集合隔离

**位置：** [analysis_service.go](../../internal/application/analysis_service.go) `computeWithValuationFallback`（129–140 行）；[analysis_daily.go](../../internal/application/analysis_daily.go) 的 valuation universe 选择。

`investmentOnly=true` 时计算只生成 InvestmentUniverse 的 component days。native 不可用而回退 base 后，这个缩小的结果却写入普通 `analysisQueryHash(baseQuery)`，与完整 AnalysisUniverse 共用 key。后续 `AssetChange(base)` 调用 `Compute`，会直接取得缺少负债或其他排除组件的结果。

**已复现：** 两个不同币种的现金资产，折算后各 100；另有负债 100。相同日期、`IncludeCash=true`：

1. 清空 memo，直接查 base AssetChange，期初净资产为 **100**。
2. 清空 memo，先查 native ReturnCalendar，触发 base fallback。
3. 再查相同 query 的 base AssetChange，期初变成 **200**。

这不是显示误差，返回值取决于此前打开过哪个页面。当前 case 43 覆盖的是 Return native 不回退、Assets 回退的方向，没有覆盖 Return 自身回退后对公共 base key 的污染。

**修复步骤：**

1. 将“计算结果所包含的组件集合”纳入所有相关 memo key，而不只 native key；或者禁止投资集合结果写入完整集合的 base key。
2. 保留现有 LRU 容量和 generation 机制，避免为了修复而取消缓存。
3. 检查普通 base 查询、native 查询及 fallback alias 的读写路径，确保 producer 与 consumer 对集合含义一致。

**验收：** 同一 fixture 比较 Assets-first、Return-first、native fallback 后切 base、缓存失效后重查；AssetChange 的期初/期末/驱动不能随调用顺序变化，负债始终保留。同时回归 case 43/44 和容量限制。

### F02 — [P1] 区分“有组件”与“有已知金额”

**位置：** [analysis_return.go](../../internal/application/analysis_return.go) `finalizeAnalysisReturns`；[analysis_return_projections.go](../../internal/application/analysis_return_projections.go) `projectReturnDays`、`dailyAvailability`。

`buildComponentDay` 在估值不完整时可以返回没有金额的 ComponentDay。但 `finalizeAnalysisReturns` 只检查 `len(componentDays)>0`，仍将初始化的 decimal zero 包装成非 nil 的 `daily.Amount`。投影层又将未赋值的期初/期末合计包装成 Money。

**已复现：** 单一现金资产，第一天完整，第二天估值完全缺失。第二天返回 `available=true`、`status=partial`，且 return amount、beginning、ending 都是非 nil 的 **0 USD**。Calendar 因而把未知的一天画成带 partial 标记的零金额，而不是不可用金额。

partial 标记不能使虚构的零变成有效数据；架构 §5.1 要求金额只累计有定义的数据。

**修复步骤：**

1. 日聚合分别跟踪是否存在已知 return amount、beginning、ending；不能用组件数量代替。
2. 全部未知时保留 nil 和 unavailable；部分组件已知时允许部分金额，但保留 partial。
3. 日 DTO 与期末汇总也采用显式 availability，不把缺失字段的 Go 零值转成权威金额。
4. 保留“完整、确实为零”和“完整但本金非正、没有收益率”的合法情况。

**验收：** 全缺失、部分缺失、完整零收益、完整零本金四组 fixture，经真实 kernel → projection → wire → Calendar 后状态和金额各不相同；未知不得显示 `$0.00`，已知零必须正常显示。

### F03 — [P1] 实物转仓也是范围边界上的本金流

**位置：** [analysis_classifier.go](../../internal/application/analysis_classifier.go) `potentialDietzCapitalAmount`；[analysis_return.go](../../internal/application/analysis_return.go) 的资本流折叠。

`potentialDietzCapitalAmount` 排除了全部 `ActivityPositionTransfer`；后续又排除了没有 Money/TradeDetail 的 effect。真实实物转仓正是 quantity effect，所以即使已知转入市值，也无法给接收账户提供 Dietz 本金。

**已复现：** 使用 `domain.PreviewChange` / `ApplyEffects` 生成合法的 `PositionTransferInput`：中午向期初为零的账户转入一股、价格始终为 100 CNY。接收账户的资产流入可以归类，但其日 `InvestedCapital=0`、`Rate=nil`、day status 为 ok。按日中点加权，应该有 **50 CNY 本金和有定义的 0% 收益率**。

同样的遗漏在有原始本金或其他收益时会改变分母；账户级分组折叠也需要这类 potential flow。架构 §7.3 规定资金是否进入/离开 InvestmentUniverse 决定本金流，不能仅按是否为现金活动判断。

**修复步骤：**

1. 区分真实持仓转移与复用相同 activity kind 的 reconciliation/split；后者不能凭空产生本金。
2. 对已知市场价值的 transfer quantity leg 保留 signed potential capital flow 和生效时间。
3. 依据全部 endpoints 在所选 InvestmentUniverse 中的关系决定实际资本流；家庭范围内双边都在集合中时保持内部中性。
4. 让账户/工具分组折叠复用 potential flow，验证与对应 scope 独立计算一致。
5. 价值未知时明确 partial，不使用成本价或零作为臆造的市场价值。

**验收：** 接收空账户、接收已有持仓账户、转出账户、家庭整体、账户分组各有用例；至少包含非零收益场景，以及 split/reconciliation 不产生本金的反向断言。

### F04 — [P2] 负数瀑布条不能使用默认同号堆叠

**位置：** [WaterfallChart.tsx](../../frontend/src/features/insights/WaterfallChart.tsx) `makeStep`、`WaterfallChart` 的两个 `stack: "waterfall"` series（103–122 行）。

当前把条形编码成透明 `base` 加正的 `span`。ECharts 默认 `stackStrategy=samesign`，正 span 不会堆叠到负 base 上。因此负净资产、负账户价值以及累计值跨零时，实际条形位置与 tooltip/表格数值不一致。

**已复现：** 使用本项目安装的 ECharts 实际执行 SSR 堆叠计算，输入期初 -100、增加 20、期末 -80。可见条应覆盖 `[-100,0]`、`[-100,-80]`、`[-80,0]`，实际三个条的堆叠终点为 **100、20、80**，没有堆叠到负 base。这个检查不依赖猜测库行为，但也不等于 Wails 视觉验收。

**修复步骤：** 采用能够表达有符号起终点的堆叠策略或显式区间绘制。若采用 `stackStrategy: "all"`，仍必须验证全负区间和跨零区间，不能只补一个配置后凭正数 fixture 宣称完成。

**验收：** 正区间增减、负区间增减、正转负、负转正、零起点、负的期初/期末；检查 ECharts 实际 layout/stack 结果，并在 Wails 中看一次。现有 `stepsFromData` 测试仅验证数学中间值，无法证明绘制正确。

### F05 — [P2] 年视图月卡必须遵守同一 coverage 门槛

**位置：** [ReturnCalendarTab.tsx](../../frontend/src/features/insights/ReturnCalendarTab.tsx) `aggregateMonth`（53–86 行）、`YearGrid`；对照架构 §5.1。

`aggregateMonth` 只要遇到任意一个日 rate，就生成月 rate，没有实施“少于一半日期有定义时省略期间收益率”的规则。这样同一月份单独查询时后端返回 `—`，年视图却显示一个百分比。

**已复现：** 四天只有一天 rate=10%，另三天无 rate，`ratedDays=1,totalDays=4`。`aggregateMonth` 返回 **0.1**，正确结果应为 null。金额可以继续累计有定义的部分，不能和 rate 一起省略。

**修复步骤：**

1. 短期修复月聚合的门槛：有 rate，且 ratedDays 至少覆盖一半日期才输出链接结果。
2. 部分覆盖的月卡仍展示清楚的 coverage；没有 rate 不应影响已知金额。
3. 加上前后端一致性用例，防止两份聚合规则继续漂移。以后若把月 summary 放回 API，应复用同一内核，而非每个月重放一次。

**验收：** 0/4、1/4 不输出率，2/4 可输出但标记 partial，4/4 正常；月卡与同范围 ReturnCalendar summary 一致。维持年视图请求数量约束。

### F06 — [P2] 自动范围不能跨到历史起点之前

**位置：** [analysisRequest.ts](../../frontend/src/features/insights/analysisRequest.ts) `effectiveRange`、`periodRange`；[analysisProjectionContext.ts](../../frontend/src/features/insights/analysisProjectionContext.ts) 14–20 行；[analysis_service.go](../../internal/application/analysis_service.go) 285–291 行。

前端默认从月初开始，年视图从 1 月 1 日开始，但没有与 `HistoryOrigin.startedAt` 的本地日期求交集。后端明确拒绝 `query.From < originDate`。因此月中开始记录的家庭默认打开分析就可能报错；同年稍后打开 Year 也会报错。

**已复现：** UI fixture 的 origin 为 2026-09-03，默认 AssetChange 请求仍发送 **2026-09-01**。后端拒绝该范围是直接代码证据；本次没有通过实际桌面 IPC 复现错误页面。

**Phase 5 复核：** 新增 `returnTrendRange` 已对 30D/YTD 等按钮求起点交集，但四个新 tab 共用的 `useAnalysisProjectionContext` 仍调用不接收 origin 的 `effectiveRange`，所以默认进入页面的问题没有解决。

**修复步骤：**

1. 统一计算可分析区间，使用 History Origin 的 timezone 将 startedAt 转成本地日期。
2. 自动月份/年份区间与该起点及 last closed day 求交集。
3. 可保留用户所选筛选期间，但明确实际覆盖范围；完全无交集时呈现空态，不发送必然失败的请求。
4. 保留 ReturnCalendar cursor 与分析期间的区别；不要为解决问题把 full-range summary 改成 visible-month summary。

**验收：** 月中起点、年中起点、跨年、起点当天尚未闭日、DST 时区、显式早于起点的范围、完全无交集的过去月份。

### F07 — [P2] 净额为零不意味着没有需要展示的明细

**位置：** [analysis_projections.go](../../internal/application/analysis_projections.go) `foldAssetChange`、`foldAssetGroups`；[ChangeDriversTab.tsx](../../frontend/src/features/insights/ChangeDriversTab.tsx) 的 residual sheet 入口。

这里有两个相关的净额筛选问题：

- `foldAssetGroups` 在分组总额为零时删除整个分组，即使 rows 中仍有非零 Income/Spending。收入 +100、支出 -100 时，Cash Flow 明细不应整组消失。
- component/day 的 Residual +5 和 -5 抵消后，waterfall 与 groups 都没有 residual 行；页面没有独立的问题入口，所以两个真实异常无法展开。

**已复现后者：** 两个 partial component days 分别有 +5、-5 Residual。AssetChange 返回 `status=partial`、`waterfall=0`、`groups=0`，而 AssetDriverDetail 仍有 **2 条 residualDetails**。用户只看到泛化提示，无法从页面进入这些明细。

**修复步骤：**

1. 一般分组按有无可展示的 rows 决定是否保留，而不是按 group total 是否为零。
2. 独立保留“存在超过 component/day tolerance 的异常”这一事实，例如 residual issue count/入口。
3. 净 Residual 为零时可以继续不画零宽瀑布条，但必须保留可点击的问题入口；不要通过制造非零柱值解决。

**验收：** 收入/支出抵消仍有两行；Residual 跨组件、跨日期抵消后仍能逐条展开；真正没有 driver/异常时保持空态简洁。

### F08 — [P2] 明细总额会与刷新后的明细行矛盾

**位置：** [ChangeDriversTab.tsx](../../frontend/src/features/insights/ChangeDriversTab.tsx) `selected` state、`DriverDetailContent`（152 行）；[CategoriesTab.tsx](../../frontend/src/features/insights/CategoriesTab.tsx) 42、60 行。

selected state 保存完整 `AssetChangeRowDTO`。明细和主查询刷新后，表格使用新 DTO，但 Total 仍读取 `selected.amount`，即点击那一刻的副本。请求范围改变也没有将 selection 与原 request 绑定。

**已复现：** 打开 residual=-5 的 sheet，将主查询和明细 mock 同时更新为 -10，然后 invalidate `['analysis']`。sheet 内新 component/day 金额为 **-10**，顶部 Total 仍为 **-5**。

**Phase 5 扩展：** Categories 同样把完整 `CategoryRowDTO` 放进 selected，并向新 detail 传 `total={selected?.amount}`，存在相同刷新风险。Drivers 已用组件测试复现；Categories 此项依据代码路径，未另跑刷新探针。Contribution sheet 使用 `item.data.amount`，不属于这个旧金额问题。

**修复步骤：** 仅保存稳定 driver/category key，从当前响应解析 row；或使 detail DTO 自带与明细同一版本的 total。范围变化时关闭 sheet 或重置 selection；driver 消失时也要明确关闭/不可用。不要让两个独立响应的新旧金额静默混用。

**验收：** sheet 打开期间刷新金额、切到已缓存的另一个范围、driver 变为零/消失，顶部与明细一致，且没有残留旧标题或金额。

### F09 — [P2] 不要用累计驱动代替未知的期末估值

**位置：** [WaterfallChart.tsx](../../frontend/src/features/insights/WaterfallChart.tsx) `stepsFromData` 的 `summary.endingValue ??` fallback。

`summary.endingValue` 缺失时，代码用 `running` 自行生成 Ending。DTO 的缺失本来意味着没有已知期末金额，这段 fallback 却把它变成了一个看似真实的期末柱和表格数值。

**已复现：** 只有 beginning=100、没有 ending、没有 drivers 的输入仍生成 ending=100。对于部分历史可用但期末不可用的数据，这与 Summary 的 `—` 不一致。

**修复步骤：** 保留未知 endpoint 状态，不生成权威 Ending。可以显示已知驱动及不完整说明，或直接对不完整瀑布使用明确空态；选定一种与 Summary 一致的表现。

**验收：** 缺期初、缺期末、只有部分 drivers 都不会制造金额；两端已知时，仍验证 signed identity 和 residual，而不是使用补差让图表看起来对账。

### F10 — 已关闭：页面测试的默认日期漂移

[AssetChangesPage.test.tsx](../../frontend/src/features/insights/AssetChangesPage.test.tsx) `beforeEach` 已显式设置 `from=2026-09-01,to=2026-09-06`，不再依赖当天的默认 last-closed date。本轮原有测试全部通过。关闭的是上一版可复现失败，不代表穷举了所有未来时区/时钟场景；默认日期逻辑仍需专门的固定时钟边界测试。

### F11 — [P2] Contribution → History 不能把整个分组缩成第一条组件

**位置：** [analysis_return_projections.go](../../internal/application/analysis_return_projections.go) `projectContributionItem`（343–350 行）；[ContributionTab.tsx](../../frontend/src/features/insights/ContributionTab.tsx) `ItemDetail.openHistory`（94–99 行）。

循环遇到第一个非零 component 后填入 `HistoryHint.AccountID` 和 `InstrumentID`，之后不再检查分组是否还包含别的账户/工具。前端将二者一起传给 History，成为交集筛选。按工具看跨账户持仓、按账户看多个工具，或者按 currency/asset class 聚合时，会遗漏当前 Contribution 金额所包含的其他记录。

**已复现：** 两个 USD 现金账户分别有股息/利息 10、20，currency group 明细金额为 **30**，`ByAccount` 有 **2** 项，History hint 却只有第一个账户 ID。前端现有单账户导航测试不会发现该问题。本轮验证到 projection 和前端 payload 路径，未运行实际 Wails 跳转。

**修复步骤：** 根据完整分组而非第一条记录生成 hint。只在所有相关组件共享同一 account/instrument 时设置对应单值；跨账户工具组至少保留 instrument、跨工具账户组至少保留 account。若现有 History payload 无法准确表达 currency/class 或多个 ID，应提供逐账户入口，或明确告知正在打开更广的历史范围；不要静默缩窄或声称宽范围等于原组。保留用户原有 scope/filter 约束。

**验收：** 同一工具两个账户、同一账户两个工具、currency/asset-class 组、相同聚合但 component 顺序相反，History 范围覆盖所有相关记录且不随遍历顺序改变。按账户单独打开则只显示该账户。

### F12 — [P2] 返回 unavailable 后仍需保留可恢复的类型控件

**位置：** [ContributionTab.tsx](../../frontend/src/features/insights/ContributionTab.tsx) 126–133 行；后端合法触发路径为 [analysis_return_projections.go](../../internal/application/analysis_return_projections.go) `projectRealizedContribution` 631–633 行。

`if (!data?.available) return contributionEmpty(...)` 在整个类型/分组工具栏之前。带 currency 或 asset-class filter 从 Total 切到 Realized 时，后端按设计返回 unavailable；随后 Return type 下拉框一起消失，用户无法在当前 tab 切回本来可用的 Total/Dividend。全局过滤栏和 tab 仍可操作，所以并非整个应用无法恢复，但需要绕路修改范围或重新进入页面。

**已复现：** 在组件测试中先返回可用 Total，选 Realized 后返回上述 unavailable reason，页面显示原因但 `Return type` 控件已不存在。同样的提前返回结构也出现在 Categories、Asset Trend、Return Trend；修复时应检查所有依赖本地模式的恢复操作。

**修复步骤：** 将已取得 origin/scope 后的工具栏保持在稳定外层，只让结果区域切换 loading/error/empty。不可用的某个类型不得阻止用户选择另一个合法类型。无需新建状态管理框架。

**验收：** 支持的 Total → 不支持的 Realized → Total/Dividend 能在原 tab 完成，筛选范围不被重置；错误重试、无记录和切换查询中的状态也有恢复入口。

### F13 — [P2] 主结果必须显示其完整性和实际估值口径

**位置：** [CategoriesTab.tsx](../../frontend/src/features/insights/CategoriesTab.tsx) 51–60 行；[ContributionTab.tsx](../../frontend/src/features/insights/ContributionTab.tsx) `ContributionRow`、主列表；[AssetTrendTab.tsx](../../frontend/src/features/insights/AssetTrendTab.tsx) 58–60 行。

- Categories 主列表显示 `data.total` / rows，却不消费 `data.status`、`missingReason`；只有打开 detail 才提示 partial。已知部分金额因此看起来像完整总额。
- Contribution 主列表没有 forced-base 或 row partial 提示；Total 的 coverage 符号只覆盖部分情况，Dividend/Realized 也不能靠无关的日数表达金额完整性。detail 有提示，但不替代榜单本身的状态。
- Asset Trend 显示 partial badge，但完全不显示 `valuationForced`，rate 摘要也没有展示 DTO 已提供的 ratedDays/totalDays。全局选择仍为 native 时，用户无法在本 tab 看到回退原因。

**已复现：** 组件探针向 Categories 提供 `available=true,status=partial` 及明确 missing reason，主结果没有 partial/原因；向 Asset Trend 提供 `valuationForced=base`，没有既有 forced-base 提示。Contribution 遗漏由当前 JSX 路径确认，未另加探针。

**修复步骤：** 在金额/榜单/趋势主结果旁展示后端提供的完整性、fallback 和 rate coverage；按 row 状态标记局部不完整。复用已有 i18n/badge，不能通过前端重算金额或从数值为零推断状态。

**验收：** 六页同一 query 的 complete、partial、unavailable、native→base、金额可用但率不可用五类场景有一致含义。无需打开 sheet 就能知道当前总额是否完整、实际币种口径和率覆盖日期。

## 5. 其他观察与范围边界

- Return Trend `linked_rate` 点当前显示每日 Modified Dietz，父级摘要显示期间链接率。架构 §11 与前端提示明确了此合同，不能仅凭名称把每日曲线列为计算 bug；如以后改成累计率曲线，应先修改合同再由后端生成。
- Unrealized 因缺少 range-end cost basis 隐藏，符合当前计划允许的降级。本报告不要求凭空推导收益率或恢复占位页。
- 上版提到的 residual 原始 key 展示和行尾空格已不再作为待办；当前 Drivers 已解析账户/工具名称，净差不闭合也有提示。净差提示不修复 F04/F09。
- Categories child 和部分 contribution labels 仍可能落到内部 key；属于后续易读性改进，不与财务正确性 P1 混在一起。
- 没有重新审查全部备份、CSV、供应商和安全边界；本报告不对整个项目作发布保证，也不要求重写账本、改 schema 或清理用户记录。

## 6. 本轮实际验证结果

| 检查 | 当前结果 |
|---|---|
| `pnpm test` | 43 个文件，**332/332 通过**；上一版 History 偶发失败本轮未重现，不据此宣称已定位根因 |
| `pnpm typecheck` | 通过 |
| Insights / queries / store / navigation / App 定向 ESLint | 通过 |
| `go test ./internal/application ./internal/domain ./internal/wailsapi/analysis` | domain、analysis API 通过；application 仅报 `TestPhase2aPerformanceBudgets/cold-3y` 失败，9.061s > 3s |
| 单独 `TestPhase2aPerformanceBudgets -count=1 -v` | warm-memo、cold-month、cold-3y 全过，cold-3y 子用例约 2.41s |
| 同三个包 `go test -race` | 通过；cold-3y 在 race 模式按测试源码跳过，不能用 race 绿色证明 3s 预算 |
| 当前源码 + 原 4 个 Go 审查探针 | 4 个均失败，复核 F01/F02/F03/F07 |
| 当前源码 + 原 4 个前端审查探针 | 4 个均失败，复核 F05/F06/F08/F09；同次原有 9 个测试通过 |
| 新 Contribution History Go 探针 | 1 个失败，证实 F11 |
| 新 Phase 5 UI 探针 | 3 个失败，证实 F12 及 F13 两个页面；同次原有 11 个测试通过 |
| 当前 ECharts SSR 负数堆叠 | `[100,20,80]`，预期 `[0,-80,0]`，证实 F04 |
| Wails desktop、真实家庭数据、原生截图/键盘/布局 | **未执行** |

性能首次失败发生在 Go、前端测试及类型检查同时运行时，后续轻负载定向运行通过，说明结果受负载影响；现有证据不足以认定单机稳定超预算，也不足以宣布长期预算关闭。正式验收应在记录硬件和负载的条件下重复测量冷查询，另记录桌面端到端耗时；不要简单提高阈值让测试变绿。没有重跑全部 Phase 2b benchmark。

### 临时探针与证据

本轮临时目录 `/tmp/nestworth-phase1-6-review/` 可能被系统清理；长期依据是本报告中的输入与验收条件。Go overlay 从**当前**测试源文件重新构建，不覆盖为上一版旧测试。

- `overlay.json`、`analysis_projections_test.go`：原四个 `TestReview...`，新增 `TestReviewContributionHistoryDoesNotNarrowMultiAccountGroup`。
- `vitest.config.mjs`、`asset-test.tsx`、`waterfall-test.ts`：旧问题复核；`phase5.config.mjs`、`phase5-test.tsx`：新页探针。
- `go.log`、`performance.log`、`race.log`、`frontend.log`、`typecheck.log`、`eslint.log`：常规验证。
- `repro-go.log`、`repro-ui.log`、`repro-history.log`、`repro-phase5.log`、`echarts.log`：反例输出。

```sh
# 仓库根目录；探针没有进入正式测试，缺少 overlay 时 -run TestReview 不算验证
GOCACHE=/tmp/nestworth-review-phase1-4-gocache go test -overlay /tmp/nestworth-phase1-6-review/overlay.json ./internal/application -run '^TestReview' -v
# frontend/，两套 UI overlay 分别运行
pnpm exec vitest run --config /tmp/nestworth-phase1-6-review/vitest.config.mjs
pnpm exec vitest run --config /tmp/nestworth-phase1-6-review/phase5.config.mjs
```

## 7. 建议修复顺序与完成条件

给 Luna 的任务应按可验证边界拆分。每批包含实现和针对该缺陷的回归用例，以本报告 fixture 为验收输入，避免只给模型一句“修复所有 review 问题”。

| 批次 | 范围 | 可观察的完成条件 |
|---|---|---|
| A | F01 | 查询顺序不改变资产金额；fallback alias 集合隔离，case 43/44、memo 容量/失效仍通过 |
| B | F02、F03 | 未知保持未知；实物转仓 account/group 本金正确，家庭内部中性；既有黄金用例继续通过 |
| C | F05、F06、F07、F08 | 六页范围及 rate coverage 同口径；抵消异常可查；主结果和 sheet 刷新一致 |
| D | F11、F12、F13 | 多账户 History 范围准确，不可用模式可原地切回，主结果完整性/币种提示齐全 |
| E | F04、F09、六页桌面与性能验收 | 负数/跨零图表正确，未知 endpoint 不补数，发布 gate 有可复查结果 |

F10 已关闭，不需要为凑批次再次修改。完成这些之后，以 [后续月度复盘方案](../design/post-analytics-monthly-review-plan.md) 为候选下一轮：先把同一家庭 fixture 的六页答案对齐，再推进 M0/M1 的数据可信与月度核对；该候选方案不因本次 review 自动成为已批准需求。

### 自动检查

后端在仓库根目录运行，性能预算与其他重负载任务分开：

```sh
GOCACHE=/tmp/nestworth-review-phase1-4-gocache go test ./internal/application ./internal/wailsapi/analysis ./internal/domain
GOCACHE=/tmp/nestworth-review-phase1-4-gocache go test -race ./internal/application ./internal/wailsapi/analysis ./internal/domain
GOCACHE=/tmp/nestworth-review-phase1-4-gocache go test ./internal/application -run '^TestPhase2aPerformanceBudgets$' -count=1 -v
```

前端在 `frontend/` 运行：

```sh
pnpm run check:bindings
pnpm typecheck
pnpm exec eslint src/features/insights src/queries/returnAnalysis.ts src/queries/invalidation.ts src/stores/analysis.ts src/app/navigation.ts src/App.tsx
pnpm test
```

### 六页统一桌面验收

使用独立的已知 fixture household，不覆盖真实数据库。事先手算/独立规定期初期末、已知收益、资本流与 coverage，不能调用被测算法生成期望值。

1. 两个币种资产、一笔负债、同一工具的两个账户：Return native → Assets base → Return base，六页共享范围与实际口径正确，调用顺序不改变金额。
2. 净资产为负及累计驱动跨零：Summary、瀑布 layout、tooltip、数据表对齐；缺失 endpoint 保持未知。
3. 全缺失、部分缺失、完整零收益、非正本金：Calendar 日/月/年、两个 Trend、Contribution 状态与数值一致；1/4 和 2/4 coverage 边界正确。
4. 月中 History Origin、尚未闭日、跨年/DST：默认进入六页与命名范围可用，保留用户筛选，不发送必然失败范围。
5. 收支相抵、Residual +5/-5：明细可达；Categories/Drivers sheet 在刷新、切范围、行消失后不混用旧总额。
6. Calendar → Drivers → Contribution → History：日期和 scope/filter 不丢失，多账户/多工具不被第一条组件缩窄；Dividend 入口选择正确类型。
7. 不支持的 Realized 条件、无数据和请求失败：仍可切换类型、调整范围、重试；Unrealized 继续明确隐藏。
8. 三种语言、窄窗口、键盘操作及 Escape 关闭 sheet、图表数据表；侧栏只有两个新 Insights 页面，Investments gain 功能继续可用。

记录自动检查、性能条件与原生结果后，再同步开发计划各小节的完成状态。当前 review 不能替代这些桌面 gate。
