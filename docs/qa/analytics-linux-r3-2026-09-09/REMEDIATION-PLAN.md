# Round-3 问题分析与修复方案

审查日期：2026-09-09。报告测试版本：`91d10c0`；本地审查版本：`19eda27`（追加 R3 QA 文档）。下方原始问题清单保留审查时的事实；本文件末尾的“当前修复记录”是本轮实现后的权威状态。

## 1. 结论与优先级

目前不能认定完整验收通过。DEF-R2-02 的关闭有探针和桌面记录支持；另外仍有一个确认的 History 筛选遗漏、一个待量化的瀑布对账异常，以及多项必要验收缺口。

| 编号 | 优先级 | 判断 | 下一步 |
|---|---|---|---|
| R3-D5 | P1 排查优先级 | 报告记录完整样本有瀑布对账警告，尚未证明金额公式错误，也不能降为纯 UX 问题 | 导出精确 DTO 和逐日差额，定位后修复 |
| R3-D6 | P2 产品修复 | 已确认 realized 详情分支遗漏 HistoryHint 账户/标的 | 后端补导航语义，覆盖 DTO 和页面链路 |
| R3-Q1 | P1 验收缺口 | Scope、Native/Base、Sources、Transfer 定量探针缺失 | 完成 DB-backed 验收 |
| R3-Q2 | P2 验收缺口 | 缺数据桌面矩阵只补了 missing-both 浅测；Drivers 证据错页 | 补齐三场景和中文实际页面 |
| R3-Q3 | P2 验收缺口 | D8 没有数据变更，不能证明软失效 | 修改行情/活动后验证刷新结果 |
| R3-Q4 | P2 证据治理 | 正文与补充记录冲突、用例编号漂移、引用产物未全部归档 | 整理唯一验收清单和必要 JSON |

这里的 P1 表示本轮验收应优先解决的事项；不把未复现的 D5 直接宣称为已确认的 P1 金额缺陷。Linux cold-3y 按已接受的 Apple M3 Pro 口径不阻塞，也不在本方案中安排性能优化。

## 2. R3-D6：已实现收益的 History 筛选丢失

### 证据与原因

`screenshots/desktop-d/d6-history.png` 显示 Account / Instrument 为 All。单凭该图无法确定用户点击的是哪个 Contribution 分组，因此不能要求所有深链都必须同时设置两个筛选。

但当前 `internal/application/analysis_return_projections.go` 的 `ContributionItem` realized 分支（约 277–306 行）存在确定遗漏：初始化 `HistoryHint` 时只有 From/To；随后只给展示用的 `component.InstrumentID` 或 `component.AccountID` 赋值，没有填入 HistoryHint。前端 `ContributionTab.tsx` 的 `ItemDetail`（约 100–104 行）按 DTO 原样导航，Wails 映射也保留字段，主要修复点在 application 层。

### 实施方案

1. 按 Instrument 分组时，把所选 groupKey 写入 `HistoryHint.InstrumentID`；按 Account 分组时写入 `HistoryHint.AccountID`。
2. 保留查询中已经确定、且与所选分组相容的账户或标的约束。若需要推导另一维度，应来自实际 realized 数据；不能从显示名称或金额推断。
3. 跨账户的同一标的可以只设置 Instrument；跨标的账户可以只设置 Account。不要为了让两个控件都不显示 All 而随意选一条记录。
4. Kinds 仅在确认收益服务所涵盖活动类型后设置合法 History 类型；不直接使用 `realized` 或 return component key，也不预设所有已实现收益都只由 Sell 产生。
5. 保留原有 total-return / dividend-interest 对混合现金、跨账户、跨标的的安全筛选行为。

### 验收

- Realized → Apple 分组 → History 必须设置 Apple；如原查询限定 US Brokerage，同时保留该账户。
- Realized → US Brokerage 分组 → History 必须设置该账户；有多个标的时不擅自指定一个。
- 同标的跨账户、同账户多标的、分组不存在、日期边界都有后端测试。
- Wails DTO 测试验证 HistoryHint；前端测试验证导航 payload 和落地控件，不只检查详情文本。
- 桌面证据同时记录点击前的 returnType/groupBy/groupKey 和跳转后的筛选状态。

## 3. R3-D5：瀑布对账警告先量化，后修复

### 已知与未知

报告和 `desktop-d/RESULTS.json` 记录 complete、2026-08-01..31、Portfolio/Base AUD/Include cash 下出现警告。但 `d5-drivers.png` 已滚动到图表和归因区域，没有拍到警告、摘要精确值或完整查询条件。显示金额之和不能证明原始金额是否对账。

当前 `WaterfallChart.tsx:67–73` 用 canonical decimal 严格比较期初加驱动之和与期末；`foldAssetChange` 汇总全部 AssetBuckets，期初/期末则从边界日期取值。警告可能来自实际归因差额、边界组件集合/日期连续性、序列化精度等，现有归档不足以锁定。

### 实施顺序

1. 使用固定 anchor 的 v3 complete 数据库和上述精确查询，导出 application 结果及真实 Wails AssetChange DTO：期初、期末、change、每个 waterfall bucket、currency、availability、residualIssueCount。
2. 计算并存档 `delta = ending - beginning - sum(waterfall)`；保留原始十进制字符串，不能先四舍五入为两位小数。该计算用于探针诊断，不新增前端财务计算口径。
3. 若 delta 非零，逐日、逐 component 输出期初/期末/所有 buckets 与差额，同时核查相邻日衔接、新增/清仓组件、内部转账双腿和币种一致性。定位第一处偏离。
4. 若 application 对账而 DTO 不对账，修复序列化精度；若 DTO 对账而 UI 警告，修复图表输入/计算路径。若引擎或投影不对账，修复对应边界或归因逻辑，并用 v3 场景锁定回归。
5. 只有实际差额被证明属于既定数值精度规则时，才采用有依据的统一容差。禁止直接加任意 epsilon、隐藏警告或在 UI 填一笔 residual 来让图表闭合。

### 验收

- complete 的精确八月窗口对账，桌面无虚假 mismatch。
- 缺数据时明确呈现不完整状态；不能把未知金额当零以强行对账。
- 正向和负向 residual 均可见、可追到日期/账户/标的；净额相抵但仍有 residual issues 的场景保留入口。
- 建立 DB 场景回归及 DTO→图表回归，精确记录是否使用容差及其来源。

## 4. 定量验收补齐

扩展 `cmd/analytics-qa-probe`；用稳定语义 ID，并在清单中映射旧 C 编号。预期值从 seed 活动、持仓、价格和 FX 独立列算；禁止用被测函数自身的结果生成 expected。

| 探针 | 最小断言与边界 |
|---|---|
| Scope | 在相同 Base AUD、日期和 IncludeCash 下，家庭金额与不重叠账户分组核对；标的分组必须单独计入现金，不能直接拿证券总和比较家庭净值。收益率不直接相加 |
| Native/Base | 单币种 Native 的币种与预期金额正确；混币种需要 fallback 的查询返回 Base 及 forced 标志；缺 FX 返回约定的 partial/unavailable，不 panic |
| Return sources | 同一投资口径下 Price/FX/Dividend/Fee 等 sources 与对应 ReturnAmount 核对；未知金额和覆盖范围显式检查。不要把独立的 Realized 与 Total Return 强行相加 |
| Transfer | 对照有/无内部转账的场景，固定或单独核算同期价格、FX、费用影响；验证转账本身不创造家庭收益，并检查账户两端。不要直接断言一个同时有市场变动的整日收益为零 |
| Residual | clean 预期 issues=0；独立复制的故障 fixture 预期有 residual，检查金额和明细；正负相抵仍保留 issues。故障注入只用于隔离测试库，正常缺价/FX 继续使用真实缺口 |

统一 JSON 输出：代码版本、fixture 版本、场景、查询、expected、actual、delta/tolerance、状态、错误原因；实际失败返回非零退出码。clean residual 场景不应调用“必须有 residual”的断言后再将 `pass=false` 包装成 PASS；应有独立正反预期。短期不执行的 C9 明确记录 waiver/延期，不算已完成。

## 5. 桌面补测

### 缺数据矩阵

现有 `incomplete/RESULTS.json` 和 `RECHECK.json` 已补 missing-both；正文“目录为空”过时。`04-drivers.png` 实际是 Return Analysis → Contribution，不能证明 Asset Changes → Change Drivers 的缺数据行为。

对 missing-price、missing-fx、missing-both 分别验证：

1. Trend All 的请求日期、图表、coverage 和无 generic load error；补实际 UI 的 YTD/1Y/3Y 日期 clamp 检查。45 天样本可以验证预设边界，不必因历史短而完全跳过这些按钮。
2. Aug 17 gap 日的 amountStatus、未知金额、部分已知零/非零与真零的区分；Aug 31 真零保留 `0%` 和 `0.00`。
3. 真正的 Asset Changes → Change Drivers 页面及警告、Residual 入口；Contribution 单独记录。
4. 英文和简体中文的提示、名称及标记解释。All/YTD 若映射同一有效日期区间，不能只凭芯片高亮判定加载失败，应验证实际日期和页面结果。

每项记录页面、场景、语言、查询、预期和实际；关键页面截图包含筛选条件及警告，不用另一页面替代。

### D8 软失效

现有操作只证明往返导航成功，没有证明数据变更后缓存失效。新增隔离 fixture 验证：记录初值 → 修改某日行情或新增明确金额的 Activity → 等待写入及必要重建完成 → 回到 Insights → 核对相关 Calendar/Trend/Drivers 的新金额和 coverage。记录预期差额，确认不是仅展示旧缓存；同时检查返回后日期/范围仍合理。

## 6. 报告与证据修订

- 将 REPORT/STATUS 及 `report/` 镜像中的空目录说法改为“missing-both 浅测已执行，missing-price/missing-fx/中文矩阵未完成”。保留原始记录，说明补测覆盖了哪些旧结论。
- D8 暂记“导航 smoke PASS，失效验收未执行”；incomplete Drivers 暂记“证据不足，需补正确页面”。
- PLAN 与报告的 C4–C7 名称并不一致，D7 也从 incomplete matrix 变成 true-zero；建立用例映射，不能拿重编号后的 PASS 代替原计划项。
- 归档用于关闭 D5/D6 的原始 DTO、查询、seed manifest、探针结果和必要日志。当前仓库镜像未包含报告引用的全部 DB/原始日志/seed JSON，应明确哪些仅存在于运行机；不要求将二进制或大 DB 提交 Git。
- 区分 snapshot complete 与收益日 rated：complete 的 `incomplete_days=0` 和 All `39/45` 并不直接矛盾；把 origin/早期日期未评级的原因明确列出，不将所有 partial 误判为缺价。
- DEF-R2-02 继续关闭；Linux cold-3y 仅保留环境观测，不重新设为发布阻塞。

## 8. 当前修复记录（2026-09-09）

本轮已经修改产品代码、探针和回归测试；旧截图/旧 RESULTS 不被改写为新的桌面证据。

| 项目 | 当前结果 | 证据 |
|---|---|---|
| R3-D5 | **代码修复 + 复查边界 PASS**。活动导致的新增/清仓组件按真实活动边界参与分析；waterfall、驱动详情、分类详情及 bucket 投影先聚合精确 attribution，最后按四位公开金额契约舍入。多 bucket 舍入余数只在精确总和按同一精度等于摘要变化时确定性分配；真实精确差额仍保留为 mismatch。固定 v3 complete、Base AUD、Include cash、2026-08-01..31 的 `ending - beginning - sum(waterfall)=0 AUD`，residual issues=0。 | `cmd/analytics-qa-probe` `NESTWORTH_PROBE_MODE=reconcile`；`probes/REMEDIATION-2026-09-09.json`；`TestReviewD5AssetChangeAggregatesExactAttributionBeforeRounding`；`TestReviewD5WaterfallAllocatesMultiBucketRoundingRemainder`；`TestReviewD5AssetDriverDetailAggregatesExactAttributionBeforeRounding`；分类/趋势/全零明细边界测试 |
| R3-D6 | **后端/Wails/frontend 自动化 PASS**。Realized 按 Instrument 时写入 instrument；按 Account 时写入 account；另一维只有在 query scope/filter 明确约束时才保留，混合 cash/security 不凭名称推导。 | `realizedContributionHistoryHint`；`TestReviewF11RealizedContributionHistoryHintKeepsOnlyExplicitDimensions`；`TestContributionItemDTOPreservesHistoryHintDimensions`；frontend 344 tests |
| R3-Q1 | **C5–C8 DB-backed PASS**。Scope expected 独立读取边界 snapshot items；Native/Base 检查混币种 forced Base 与 instrument quote currency；sources 精确相加；Transfer 使用固定 snapshots/quotes 的有无转账反事实，逐日核对 household change/external flow、Price/FX/Fee、Return sources 和两端账户。 | `NESTWORTH_PROBE_MODE=quantitative`；`probes/REMEDIATION-2026-09-09.json` |
| R3-Q1/C9 | **clean/corrupt residual PASS**。探针复制数据库后才注入 +100 AUD，clean 样本 issues=0；corrupt 样本 native 不变、residual 明细可追到 2026-08-12/13 且金额 ±100 AUD。 | `NESTWORTH_PROBE_MODE=residual`，`NESTWORTH_RESIDUAL_INJECT=false/true`；同上 JSON |
| R3-Q2 | **仍需真实桌面补测**。仓库现有 incomplete 证据是 missing-both；missing-price、missing-fx 及中文/正确 Change Drivers 页面没有在本轮原生 Wails 中重跑。 | `screenshots/incomplete/RESULTS.json` |
| R3-Q3 | **代码/应用层验证 PASS，原生桌面变更流程未重跑**。既有 `TestAnalysisCase44MemoInvalidatesAfterActivityAndSnapshotBuild` 覆盖 activity 与 snapshot rebuild 后结果变化；frontend invalidation tests 通过。 | application test、`frontend/src/queries/invalidation.test.ts` |
| R3-Q4 | **文档状态已更新**：C5–C9 不再标为 tooling SKIP；旧桌面截图继续标为历史证据，未把自动化结果伪装成新截图。 | 本文件、`STATUS.md`、`REPORT.md`、`probes/C-phase.md`、`probes/SUMMARY.json` |

### 8.1 复查追加修复（2026-09-09）

复查确认 D6 原修复成立；上一轮 D5 的瀑布/下钻口径和 Transfer 验收已补齐。本轮又发现并修复三处更细的舍入边界：全零明细余差、分类行提前舍入、趋势周期提前舍入。

1. **瀑布精度契约**：摘要、driver rows、groups 和前端严格对账均以四位 `SignedMoney` 为公开契约。精确 bucket 总和按四位舍入后等于摘要变化时，才把行级舍入余数分配给绝对值最大的确定性 bucket；若精确总和本身不一致，不使用 epsilon 或虚构 residual 隐藏问题。
2. **下钻口径**：`AssetDriverDetail`、Categories/CategoryDetail 及 bucket 型趋势读取 `AssetBucketExact`，跨天/跨组件先聚合精确值，最后一次舍入；旧调用者没有 exact map 时保留四位视图兼容路径。
3. **Transfer 对照**：C8 在不修改 fixture 的前提下，使用相同 snapshots/quotes，分别执行包含和移除区间 `cash_transfer` 活动的分析；逐转账日核对 household change/external flow、Price/FX/Fee，逐账户核对两端 signed transfer leg，并核对 Return amount 与 Price/FX/Fee sources。跨币种 endpoint 不伪装成 base-currency 精确断言，而是明确失败并要求专门的 FX 口径。
4. **维度明细余差**：`reconcileDimensionAmounts` 不再因所有单项四位舍入后为零而提前返回；从原始非零维度中按绝对值和稳定 key 选择承接项，使例如两个 `0.00004` 账户仍能显示聚合后的 `0.0001`。
5. **分类与趋势累加**：Categories 按 row key、Asset Trend 按 period 保存 decimal 累加器；仅在生成 `CategoryRow`、trend point/summary DTO 时调用 `SignedMoney` 舍入，避免多个 `0.00004` 日值在聚合前丢失。

复查边界测试覆盖两个 `0.00006` bucket 的 `0.0002` vs `0.0001` 舍入样本、跨账户 `0.00004 + 0.00004` 明细、分类行和月趋势的同类样本。application/Wails 定向回归和 probe 包编译测试通过；本轮未重跑数据库探针，因此既有 C8/数据库证据保持原样。Native Wails replay、缺 price/FX 中文矩阵和真实 D8 变更流程仍按原计划保持未执行，不由自动化结果替代。

### 本轮可复现命令

```text
GOCACHE=/tmp/nestworth-remediation-cache go run ./cmd/analytics-qa-seed
NESTWORTH_PROBE_MODE=reconcile ... go run ./cmd/analytics-qa-probe
NESTWORTH_PROBE_MODE=quantitative ... go run ./cmd/analytics-qa-probe
NESTWORTH_PROBE_MODE=residual NESTWORTH_RESIDUAL_INJECT=false ... go run ./cmd/analytics-qa-probe
NESTWORTH_PROBE_MODE=residual NESTWORTH_RESIDUAL_INJECT=true ... go run ./cmd/analytics-qa-probe
```

其中 `...` 必须指向 seed 生成的数据库和各自 JSON 输出路径；residual 模式只修改复制品。Linux cold-3y 仍为已知环境性能观察项；Wails 原生 GUI 截图、missing-price/missing-fx 矩阵和中文页面仍是未执行门禁，不能由上述自动化结果替代。

## 7. 推荐交付顺序与完成标准

1. **修复 D6，并建立 D5 精确重现**：先提交范围清楚的 History 修复；D5 必须给出差额和根因证据。
2. **修复 D5 实际根因、补定量探针**：完成 Scope/Native-Base/Sources/Transfer，并执行 clean/corrupt Residual 正反样本或明确延期。
3. **补桌面矩阵与真实 D8 变更验证**：复用固定 fixture，集中完成三种缺数据场景及中文验收。
4. **统一报告状态与产物引用**：已测、未测、延期清晰可追溯，再给出最终验收结论。

实现后运行相关 application/Wails/frontend 回归；涉及 DTO 时执行 bindings 一致性、typecheck、lint，再运行 Go 和前端完整套件。性能继续用已批准的 M3 Pro 常规模式；原生桌面证据单列，不能由单元测试替代。

最终通过条件：D6 导航范围正确；D5 差额有解释且缺陷修复；关键定量探针实际通过；缺数据页面与失效流程有正确证据；所有剩余 SKIP 明确延期或接受，不再用“0 FAIL”代替完整验收。
