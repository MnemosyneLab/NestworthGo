# Analytics Phase 1–4 整体 Review

审查日期：2026-09-08。范围：Phase 1a、1b、2a、2b、3，以及当前工作区的 Phase 4 实现。

基线提交：`04e4e78d38f9047edd9faacbe8c7ad3bdd03ceac`，加上审查开始时已暂存的 20 个文件改动。当前 Phase 4 尚未提交；开发计划仍标记 `In progress`，并注明 desktop smoke pending。

本次只新增此文档，没有修改产品代码、正式测试、设计计划或用户数据库。审查用例通过 `/tmp` 下的 Go overlay 和 Vite 内存加载补入，未写入仓库测试文件。

## 1. 结论

**建议先修复本报告中的正确性问题，再以当前内核继续 Phase 5。无需推翻现有架构。**

现有结构已经把资产变动、投资收益、Dietz 本金流分开，并具备黄金用例、Wails DTO、缓存失效和页面测试。但现有绿色测试没有覆盖几个跨阶段的重要边界：不同计算集合共享缓存、没有已知金额的日子、实物转仓、低覆盖率的月度聚合，以及负数图表。

本次确认 **10 项发现：3 项 P1、7 项 P2**。P1 涉及错误的资产金额、把未知金额展示成零、遗漏收益率本金；P2 涉及展示正确性、可用性和验收可靠性。各项均有代码依据；除明确标注的端到端推导外，还使用了定向自动复现。

不能据此宣称 Phase 1–4 已完整验收：没有运行 Wails 桌面程序，没有使用真实家庭数据，也没有完成原生图表、键盘和布局验收。

## 2. 范围与已确认的基础

审查依据为 [开发计划](../design/analytics/analytics-development-plan.md)、[技术架构](../design/analytics/analytics-redesign-architecture.md)、[产品设计](../design/analytics/analytics-product-design.md) 和对应 wireframes。范围不限于 Phase 4 的暂存 diff。

| 阶段 | 本次检查重点 | 结论边界 |
|---|---|---|
| 1a | Universe、范围交集、金额符号、分类器、Price/FX、Residual、快照输入 | 已运行现有 application/domain 测试；没有穷举所有合法账本组合 |
| 1b | 日收益、加权本金、几何链接、分组折叠、缺失数据 | 发现 F02、F03；现有黄金用例未挡住这两个边界 |
| 2a | Asset projections、memo、失效、明细、DTO | 发现 F01、F07；检查了供 Phase 4 使用的投影链路 |
| 2b | Return projections、独立 valuation fallback、coverage、Wails 映射 | F01/F02 在这一层继续传播；尚未实现的成本基础明确不可用，不作为缺陷 |
| 3 | Return Calendar、年视图、共享筛选、日期范围、日明细与导航 | 发现 F05、F06 |
| 4 | Summary、瀑布图、分组、Residual 明细、History 入口 | 发现 F04、F07–F10；桌面视觉验收未执行 |

值得保留的实现：

- `AssetBucket`、`ReturnComponent`、`DietzCapitalFlow` 的职责分离。
- Price/FX 按路径计算，Residual 没有被统一塞入 Adjustments。
- 后端使用十进制运算，wire 金额使用字符串；图表定位之外没有必要引入浮点财务计算。
- memo 有容量限制和 generation；问题在计算集合的隔离，而非缺少缓存框架。
- 常规 activity/quote/valuation 失效已覆盖 `queryKeys.analysis.all`，不应重新建设第二套查询失效机制。
- Calendar → Drivers 已传递日期、scope、valuation、cash 和更多筛选；旧 Analysis 仍保留，符合分阶段切换要求。
- Residual DTO 已增加 component/day 身份，并有 Wails 序列化覆盖。

## 3. 发现总表

| ID | 优先级 | 问题 | 涉及阶段 |
|---|---|---|---|
| F01 | P1 | Return 的强制 base 结果污染 Asset Changes 的 base 缓存 | 2a/2b/3/4 |
| F02 | P1 | 全部估值未知的一天被返回为可用的零收益、零期初和零期末 | 1b/2b/3 |
| F03 | P1 | 实物持仓转入/转出未计入所选范围的 Dietz 本金 | 1a/1b/2b |
| F04 | P2 | 负数或跨零瀑布图使用错误的堆叠方式 | 4 |
| F05 | P2 | 年视图月卡重新显示后端规则本应省略的低覆盖率收益率 | 3 |
| F06 | P2 | 默认月份/年份范围可能早于 History Origin，导致分析报错 | 3/4 |
| F07 | P2 | 汇总为零时隐藏仍有意义的分组和 Residual 明细入口 | 2a/4 |
| F08 | P2 | 明细刷新后总额仍来自点击时保存的旧 row | 4 |
| F09 | P2 | 缺失期末金额时，瀑布图自行生成一个“期末金额” | 4 |
| F10 | P2 | 页面测试依赖真实日期，次日即失败 | 4 |

## 4. 详细发现与修复方案

### F01 — [P1] Return fallback 的 base 缓存必须与完整资产集合隔离

**位置：** [analysis_service.go](../../internal/application/analysis_service.go) 129–139 行；[analysis_daily.go](../../internal/application/analysis_daily.go) 43–46、105–113 行。

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

**位置：** [analysis_return.go](../../internal/application/analysis_return.go) 63–107 行；[analysis_return_projections.go](../../internal/application/analysis_return_projections.go) 448–460 行及 `dailyAvailability`。

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

**位置：** [analysis_classifier.go](../../internal/application/analysis_classifier.go) 189–234 行，尤其 218–219 和 233–234 行。

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

**位置：** [WaterfallChart.tsx](../../frontend/src/features/insights/WaterfallChart.tsx) 30–43、87–104 行。

当前把条形编码成透明 `base` 加正的 `span`。ECharts 默认 `stackStrategy=samesign`，正 span 不会堆叠到负 base 上。因此负净资产、负账户价值以及累计值跨零时，实际条形位置与 tooltip/表格数值不一致。

**已复现：** 使用本项目安装的 ECharts 实际执行 SSR 堆叠计算，输入期初 -100、增加 20、期末 -80。可见条应覆盖 `[-100,0]`、`[-100,-80]`、`[-80,0]`，实际三个条的堆叠终点为 **100、20、80**，没有堆叠到负 base。这个检查不依赖猜测库行为，但也不等于 Wails 视觉验收。

**修复步骤：** 采用能够表达有符号起终点的堆叠策略或显式区间绘制。若采用 `stackStrategy: "all"`，仍必须验证全负区间和跨零区间，不能只补一个配置后凭正数 fixture 宣称完成。

**验收：** 正区间增减、负区间增减、正转负、负转正、零起点、负的期初/期末；检查 ECharts 实际 layout/stack 结果，并在 Wails 中看一次。现有 `stepsFromData` 测试仅验证数学中间值，无法证明绘制正确。

### F05 — [P2] 年视图月卡必须遵守同一 coverage 门槛

**位置：** [ReturnCalendarTab.tsx](../../frontend/src/features/insights/ReturnCalendarTab.tsx) 53–86 行及 `YearGrid`；对照架构 §5.1。

`aggregateMonth` 只要遇到任意一个日 rate，就生成月 rate，没有实施“少于一半日期有定义时省略期间收益率”的规则。这样同一月份单独查询时后端返回 `—`，年视图却显示一个百分比。

**已复现：** 四天只有一天 rate=10%，另三天无 rate，`ratedDays=1,totalDays=4`。`aggregateMonth` 返回 **0.1**，正确结果应为 null。金额可以继续累计有定义的部分，不能和 rate 一起省略。

**修复步骤：**

1. 短期修复月聚合的门槛：有 rate，且 ratedDays 至少覆盖一半日期才输出链接结果。
2. 部分覆盖的月卡仍展示清楚的 coverage；没有 rate 不应影响已知金额。
3. 加上前后端一致性用例，防止两份聚合规则继续漂移。以后若把月 summary 放回 API，应复用同一内核，而非每个月重放一次。

**验收：** 0/4、1/4 不输出率，2/4 可输出但标记 partial，4/4 正常；月卡与同范围 ReturnCalendar summary 一致。维持年视图请求数量约束。

### F06 — [P2] 自动范围不能跨到历史起点之前

**位置：** [analysisRequest.ts](../../frontend/src/features/insights/analysisRequest.ts) 27–35、48–51 行；[ReturnCalendarTab.tsx](../../frontend/src/features/insights/ReturnCalendarTab.tsx) 231–240 行；[AssetChangesPage.tsx](../../frontend/src/features/insights/AssetChangesPage.tsx) 20–26 行；[analysis_service.go](../../internal/application/analysis_service.go) 285–291 行。

前端默认从月初开始，年视图从 1 月 1 日开始，但没有与 `HistoryOrigin.startedAt` 的本地日期求交集。后端明确拒绝 `query.From < originDate`。因此月中开始记录的家庭默认打开分析就可能报错；同年稍后打开 Year 也会报错。

**已复现：** UI fixture 的 origin 为 2026-09-03，默认 AssetChange 请求仍发送 **2026-09-01**。后端拒绝该范围是直接代码证据；本次没有通过实际桌面 IPC 复现错误页面。

**修复步骤：**

1. 统一计算可分析区间，使用 History Origin 的 timezone 将 startedAt 转成本地日期。
2. 自动月份/年份区间与该起点及 last closed day 求交集。
3. 可保留用户所选筛选期间，但明确实际覆盖范围；完全无交集时呈现空态，不发送必然失败的请求。
4. 保留 ReturnCalendar cursor 与分析期间的区别；不要为解决问题把 full-range summary 改成 visible-month summary。

**验收：** 月中起点、年中起点、跨年、起点当天尚未闭日、DST 时区、显式早于起点的范围、完全无交集的过去月份。

### F07 — [P2] 净额为零不意味着没有需要展示的明细

**位置：** [analysis_projections.go](../../internal/application/analysis_projections.go) 290–295、317–328 行；[ChangeDriversTab.tsx](../../frontend/src/features/insights/ChangeDriversTab.tsx) 158–170 行。

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

**位置：** [ChangeDriversTab.tsx](../../frontend/src/features/insights/ChangeDriversTab.tsx) 108–119、125–142、148–150 行。

selected state 保存完整 `AssetChangeRowDTO`。明细和主查询刷新后，表格使用新 DTO，但 Total 仍读取 `selected.amount`，即点击那一刻的副本。请求范围改变也没有将 selection 与原 request 绑定。

**已复现：** 打开 residual=-5 的 sheet，将主查询和明细 mock 同时更新为 -10，然后 invalidate `['analysis']`。sheet 内新 component/day 金额为 **-10**，顶部 Total 仍为 **-5**。

**修复步骤：** 仅保存稳定 driver key，从当前响应解析 row；或使 detail DTO 自带与明细同一版本的 total。范围变化时关闭 sheet 或重置 selection；driver 消失时也要明确关闭/不可用。不要让两个独立响应的新旧金额静默混用。

**验收：** sheet 打开期间刷新金额、切到已缓存的另一个范围、driver 变为零/消失，顶部与明细一致，且没有残留旧标题或金额。

### F09 — [P2] 不要用累计驱动代替未知的期末估值

**位置：** [WaterfallChart.tsx](../../frontend/src/features/insights/WaterfallChart.tsx) 60–61 行。

`summary.endingValue` 缺失时，代码用 `running` 自行生成 Ending。DTO 的缺失本来意味着没有已知期末金额，这段 fallback 却把它变成了一个看似真实的期末柱和表格数值。

**已复现：** 只有 beginning=100、没有 ending、没有 drivers 的输入仍生成 ending=100。对于部分历史可用但期末不可用的数据，这与 Summary 的 `—` 不一致。

**修复步骤：** 保留未知 endpoint 状态，不生成权威 Ending。可以显示已知驱动及不完整说明，或直接对不完整瀑布使用明确空态；选定一种与 Summary 一致的表现。

**验收：** 缺期初、缺期末、只有部分 drivers 都不会制造金额；两端已知时，仍验证 signed identity 和 residual，而不是使用补差让图表看起来对账。

### F10 — [P2] 固定页面测试的时钟

**位置：** [AssetChangesPage.test.tsx](../../frontend/src/features/insights/AssetChangesPage.test.tsx) 101 行，以及该文件 beforeEach。

测试固定期望 `to=2026-09-06`，却没有固定 Date。2026-09-08 运行时，正确请求为 `to=2026-09-07`，该测试失败。ReturnAnalysisPage 测试已经采用只 fake Date 的方式，可以复用这个模式。

**修复步骤：** 给日期相关测试注入固定时钟或只 fake Date，afterEach 恢复。保留独立的 last-closed-day 边界测试，不要把期望改成今天的日期后就结束。

**验收：** 同一测试在不同自然日/时区运行不漂移；新增日期边界用例通过。

## 5. 其他观察与范围边界

### 值得顺手处理，但不扩大为架构重做

- Calendar 的 contributor label 在后端多处直接设置为 group key，前端 `ContributorList` 原样展示；对于 instrument group，这会是内部 ID。Drivers residual 明细也直接展示 component key。建议用已加载名称或 DTO label 展示资产/账户名称，ID 作为辅助信息；名称解析失败不能阻止查看金额。
- 新增的零 driver 页面测试没有给 mock 提供零 driver，只断言页面上不存在名为 Zero 的按钮。这无法证明过滤行为，应让 fixture 真正包含要验证的边界。
- `git diff HEAD --check` 报告 `ChangeDriversTab.tsx:166` 行尾空格。属于已有暂存改动的格式问题，本次未修正。

### 不应在本次修复中顺便扩大范围

- Phase 5 的四个完整标签页、Phase 6 的旧 Analysis 移除不属于本轮应补齐的缺陷。
- `ContributionUnrealized` 明确返回 unavailable，是现有 Phase 2b 计划允许的降级；接 Phase 5 时需要继续遵守“没有成本基础就不造公式”。
- 本报告没有重新审查全部备份恢复、CSV 写入、供应商接入或安全边界；未对整个项目给出发布保证。
- 不建议增加通用状态管理框架、重写账本、改变数据库 schema，或清理用户已有记录来绕过这些问题。

## 6. 实际验证结果

以下结果均来自此次工作区，不引用历史运行的绿色结果。

| 检查 | 结果 |
|---|---|
| `go test ./internal/application ./internal/wailsapi/analysis ./internal/domain`，独立 `/tmp` GOCACHE | 全部通过 |
| 同三个包 `go test -race` | 全部通过 |
| `pnpm exec tsc --noEmit` | 通过 |
| Insights、returnAnalysis、invalidation、analysis store、navigation、App 的定向 ESLint | 通过 |
| Insights/store/navigation/locale 定向 Vitest | 24 个测试：23 通过，1 个 F10 日期失败 |
| 全量 `pnpm exec vitest run --testTimeout=15000` | 43 个文件、320 个测试：318 通过，2 失败 |
| 全量失败之一 | F10，稳定复现 |
| 全量另一失败 | `HistoryPage > prompts to Start History when none exists yet` 找不到 heading；单独复跑通过。未确认根因，不作为确定的 Analytics 回归 |
| 4 个新增 Go 审查探针 | 均按预期失败，分别证实 F01/F02/F03/F07 |
| 4 个新增前端审查探针 | 均按预期失败，分别证实 F05/F06/F08/F09；同次原有 5 个用例在固定 Date 后通过 |
| 本地 ECharts SSR 堆叠计算 | 证实 F04，负 base 没有承接正 span |
| Wails desktop smoke、真实数据、原生截图 | 未执行 |
| 本次重新跑性能 benchmark | 未执行；不据旧记录重申当前性能预算已满足 |

全量 History 用例单独通过，不足以宣布其并发/时序问题已解决；发布前应保留这一未解释失败的记录。

### 审查探针及临时证据

临时文件位于 `/tmp/nestworth-phase1-4-review/`，可能被系统清理。长期复现应依据每条 finding 的输入和验收条件，将回归用例正式加入对应测试文件。

- `analysis_projections_test.go`：原测试内容加四个审查用例；`overlay.json` 将其映射到原测试路径。
- `vitest.config.mjs`：只在内存中加载扩展测试，并暴露月聚合函数供审查调用；没有改动实际源文件。
- `repro-go.log`、`repro-ui.log`：新增失败用例的实际输出。
- `frontend-tests.log`、`go-race.log`、`history-recheck.log`、`eslint.log`：相关验证记录。

Go 探针名称：

```text
TestReviewNativeReturnCacheDoesNotDropLiability
TestReviewAllMissingDayDoesNotBecomeZeroReturn
TestReviewInKindTransferSuppliesReceivingAccountDietzCapital
TestReviewOffsettingResidualsRemainReachable
```

本次运行方式为 `go test -overlay /tmp/nestworth-phase1-4-review/overlay.json ./internal/application -run '^TestReview' -v`。这些用例没有进入仓库，直接在原代码上执行 `-run '^TestReview'` 不构成验证。

## 7. 建议修复顺序与完成条件

建议按下面四个小批次交给实现者。每批只调整必要代码和对应回归测试，不需要重新阅读/重写整个项目。

| 批次 | 范围 | 可观察的完成条件 |
|---|---|---|
| A | F01 | Return/Assets 查询顺序不再改变资产金额，case 43/44 及 bounded memo 继续通过 |
| B | F02、F03 | 未知金额保持未知；实物转仓的 account/group Dietz 本金正确；既有黄金用例继续通过 |
| C | F05、F06、F07、F08 | 月卡与 API 同口径；新家庭默认分析可用；抵消异常仍可追查；刷新后 sheet 不混用新旧值 |
| D | F04、F09、F10 与桌面验收 | 负数图表正确、未知 endpoint 不补数、测试日期稳定，并完成下面的原生验收 |

完成 A/B 后再连接更多 Phase 5 页面，能避免把错误结果扩散到新的趋势和贡献视图。C/D 可以作为小范围界面修复继续处理。

### 自动验证命令

后端在仓库根目录运行：

```sh
env GOCACHE=/tmp/nestworth-review-phase1-4-gocache go test ./internal/application ./internal/wailsapi/analysis ./internal/domain
env GOCACHE=/tmp/nestworth-review-phase1-4-gocache go test -race ./internal/application ./internal/wailsapi/analysis ./internal/domain
```

前端在 `frontend/` 运行：

```sh
pnpm exec tsc --noEmit
pnpm exec eslint src/features/insights src/queries/returnAnalysis.ts src/queries/invalidation.ts src/stores/analysis.ts src/app/navigation.ts src/App.tsx
pnpm exec vitest run --testTimeout=15000
```

DTO 有变化时，按项目既有流程重新生成 bindings，再执行上述检查。缓存/折叠实现若改变热路径，应额外复跑 Phase 2a/2b 上界 benchmark，并记录输入规模和观察值。

### Wails 桌面验收脚本

使用独立的已知 fixture household，不覆盖或清理真实数据库：

1. 配置至少两个资产币种、一笔负债、一个可交易持仓，打开 Return native，再切 Asset Changes base；核对两页口径和净资产。
2. 选择净资产为负、以及累计驱动跨零的期间，核对瀑布图坐标、tooltip、数据表和 Summary。
3. 查看全缺失与部分缺失估值日，确认未知金额显示不可用；年视图月卡不会展示低于 coverage 门槛的率。
4. 用月中开始 History 的家庭打开默认月视图和 Year，确认范围提示和空态，不出现必然失败的请求。
5. 打开正负抵消的 residual 明细，确认能看到两个 component/day；进入 History 后日期和账户/工具筛选正确。
6. 在明细打开期间刷新相关分析输入，确认 Total 与明细同一版本；切换筛选后没有遗留 selection。
7. 检查三种语言、窄窗口、键盘打开/关闭 sheet、图表数据表。旧 Analysis 与 Investments holding gains 继续可用。

完成这些检查并记录结果后，再更新计划中的 Phase 4 状态；不要仅因单元测试通过就把桌面 gate 标记完成。
