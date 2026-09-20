# Nestworth 全面 Review（2026-09-20）

## 摘要

- **范围**：对 Nestworth-go（Wails v3 桌面应用：Go + SQLite 为权威源，React/TypeScript 前端）做只读全应用复查。对照当前源码与 2026-09-17 之后的提交（HEAD `cdd4eab`，2026-09-19：Insights UX、Wails beta.23、instrument holdings 聚合、设置/导航精简、v0.3.4、CoinGecko、贵金属模板、历史修复、市场数据 repair details）。不把 `frontend/bindings` 当作手写缺陷。
- **方法**：以 `codegraph_explore` 与源码阅读为主；对照 `docs/architecture/`、`docs/product/`、`docs/handoffs/`、`docs/reviews/2026-09-19-insights-ux.md` 作定向，不以文档为真理。已跑两组聚焦测试作证据，未跑全量 `go test ./...`、未跑 `wails3 task check`。
- **明确未执行**：原生窗口/菜单、活体行情（Yahoo/Tiingo/CoinGecko/Frankfurter/Worker）、打包/签名/公证、打开含真实家庭数据的 Nestworth 实例。
- **计数**：P0 **0** · P1 **2** · P2 **5** · P3 **9** · 待验证 **4**

**最重要的 5 项：**

- **多币种现金的 `Complete` 串扰**：`missingForComponent` 把同一账户上所有 `InstrumentID == nil` 的缺失（FX / 历史覆盖）套到每一笔现金成分上。已估值的本位币现金会被标成不完整，Insights 按成分丢弃该金额，而快照表头仍计入 `Available` 金额。
- **CSV 导入后前端缓存不全失效**：`useCommitCSV` 未失效 `analysis` / `history` / `analytics.instrumentHoldings` / quotes。Portfolio Holdings 用 `keepMounted` + `useInstrumentHoldings`，导入后持仓汇总与 Insights 会显示旧数据。
- **设置精简后日历偏好不可改**：`weekStart` / `dateFormat` 仍驱动 DatePicker 与 Insights 收益率日历，但 Settings UI 已无编辑入口。
- **备份包密钥与静默丢 settings**：`CreateBackup` 把完整 settings（含 Tiingo/CoinGecko/Worker 密钥）写入备份；Load/Marshal 失败时静默写成 `{}`，恢复后偏好与密钥丢失且无错误。
- **贵金属最新价缓存持锁跨 HTTP**：`latestMetalQuote` 在 `metalFetchCache.mu` 持有期间调用 `provider.LatestInstrument`，同一次刷新里黄金/白银会串行卡住。

## 严重程度说明

- **P0** 数据损坏 / 资金计算错误 / 安全漏洞（可导致错误金额被当成完整结果，或密钥/路径被意外暴露给不可信方）
- **P1** 明确功能缺陷，或高概率用户会踩到的错误金额/错误状态
- **P2** 明显缺陷、错误处理或契约不一致；用户可感知，但通常有不完整标记或旁路
- **P3** 坏味道、可维护性、次要 UX、文档漂移

本次 **未发现 P0**：快照日级 `Complete` 仍会因真实缺失而 fail-close，未看到把缺失金额制成 0 并当作完整净值的路径。P1 的现金串扰会让 Insights **漏记已估值现金**，而不是把未知值写成 0。

## 问题清单

### 估值与快照

#### [P1] 多币种现金：兄弟成分的 FX/覆盖缺失会污染本位币现金的 `Complete`

- **现状 / 证据**：`internal/application/historical_snapshot.go` 的 `valueHistoricalSnapshot` 用 `missingForComponent` 决定 `DailyValuationSnapshotItem.Complete`（约 L126–127）。对无 `InstrumentID` 的现金成分，函数把该账户上**所有** `InstrumentID == nil` 的 `MissingInputView` 都算作匹配（L291–293）。`valueNative` / `convert`（`internal/application/valuation.go` L473–480、L464–468）在缺 FX 或历史覆盖不完整时，正好生成 `InstrumentID == nil` 的 `MissingFXRate` / `MissingHistoryCoverage`。
  - 同一 holdings 账户上：USD 现金（与家庭本位币相同，`Available == true`、已有 `BaseAmountExact`）+ CNY 现金（缺 FX 或覆盖未验证）时，USD 项也会 `Complete == false`。
  - 表头合计仍在 `component.Available` 时把金额加入 `assets`/`liabilities`（`historical_snapshot.go` L157–163）。
  - Insights 的 `snapshotItemValue`（`internal/application/analysis_daily.go` L412–415）在 `!item.Complete` 时直接返回不可用；`buildComponentDay`（L177–182）把该成分日标为 `CompletenessUnavailable` / `Partial`。成分日是 Insights 的金额来源，**与快照表头独立**。`NetWorthTrend` 用日级 `snapshot.Complete`（`trend.go` L31–35），日级已因真实缺失而为不完整，但资产变动/驱动/分类仍按成分求和，会漏掉那笔已估值的本位币现金。
- **影响**：USD/CNY/SGD 等多币种现金的经纪账户，在任一外币现金缺汇率或历史覆盖时，Insights 会少计本位币现金；瀑布图可能对不齐；用户看到「不完整」却无法从 MissingReason 判断真正缺的是哪一币种（`missingReason` 会给好的那一袖套上「missing account value or FX rate」）。
- **建议**：`missingForComponent` 对现金按 `QuoteCurrency` / `NativeCurrency` 对齐；FX 缺失只应落在需要该汇率的成分上。不要用「账户上任意 nil-instrument missing」作现金匹配。

#### [P3] `DailySnapshotStateDTO` 声称 mirror domain，却丢掉 `DirtyTo` / generation

- **现状 / 证据**：`internal/wailsapi/history/history.go` L320–336。注释写 “mirrors domain.DailySnapshotState”，DTO 只有 `HouseholdID`、`DirtyFrom`、`LastCompletedClosedOn`。域模型还有 `DirtyTo`、`InputGeneration`（`internal/domain/history.go`）。前端 History 页实际用 Data Health 的 `rangeStart`/`rangeEnd` 做修复，不读 `dirtyTo`。
- **影响**：若将来 UI 用该 DTO 展示脏区间，上界会永远缺失；目前用户路径主要走 health issue，实际伤害有限。
- **建议**：要么补齐字段，要么改注释，避免后续误用。

#### [P3] History 修复文案只展示 `rangeStart`

- **现状 / 证据**：`frontend/src/features/history/HistoryPage.tsx` L326–327 使用 `t("history.snapshotNeedsRepair", { date: snapshotIssue.rangeStart })`。health issue 本身带 `rangeEnd`（`DataHealthPage` 会格式化区间）。`review_regression_test.go` 的 `TestSnapshotHealthAndPreviewOnlyIncludeClosedDays` 明确脏区间可跨多日。
- **影响**：用户可能以为只修一天，实际会重建 `rangeStart`–`rangeEnd` 的整个闭市区间。
- **建议**：与 Data Health 一样展示起止日。

### 市场数据

#### [P2] 贵金属最新价缓存在持锁期间发 HTTP

- **现状 / 证据**：`internal/application/precious_metal.go` `latestMetalQuote`（L57–65）：`cache.mu.Lock()` 之后，若 key 不存在则调用 `provider.LatestInstrument`，然后才 `Unlock`。Yahoo COMEX 期货请求通常数百毫秒到数秒。
- **影响**：同一次估值/刷新若同时需要黄金与白银（再加 USD FX），第二次金属报价会卡在锁上直到第一次网络返回。UI 刷新变慢，取消/超时也不易打断已排队的第二次。
- **建议**：锁内只读/占位，网络在锁外；或按 symbol 用 singleflight 且不把 HTTP 放进互斥区。

#### [P3] CoinGecko `fetch` 复用 `classifyTiingoResponse`，并在持锁期间限速+HTTP

- **现状 / 证据**：`internal/infrastructure/marketdata/coingecko.go` `fetch`（L72–92）在 `p.mu` 持有期间 sleep 至 `nextRequest` 再 `doFetch(..., classifyTiingoResponse)`。`classifyTiingoResponse`（`tiingo.go` L156–174）按 HTTP 状态映射通用 `ErrProviderAuthentication` / rate limit，语义可用，但 404 会变成 `unsupportedProviderSymbol`，错误文案与调用栈都像 Tiingo。
- **影响**：诊断与日志误导；持锁会把 search/latest/history 强行串行（有意的 Demo 限速），长时间占用时取消只能等到当前请求结束。
- **建议**：抽共享 `classifyProviderHTTP`；锁只保护 cache/nextRequest 时间戳。

#### [P3] 贵金属证据 JSON 序列化错误被丢弃

- **现状 / 证据**：`precious_metal.go` `metalEvidence` L49：`data, _ := json.Marshal(evidence)`。
- **影响**：理论上证据字符串可变成空；当前结构体字段都是可序列化类型，实际几乎不会失败。属于静默失败味道。
- **建议**：Marshal 失败应返回错误，而不是写入空证据。

### 账户与交易

#### [P3] `CreateHolding` 在 History 已开始且未填单位成本时，用最新行情当作成本

- **现状 / 证据**：`internal/application/portfolio.go` `CreateHolding` L442–453：History 已开始且数量非 0 时，若 `input.UnitCost` 为空，则 `selectInstrumentQuote` 的最新价写入 `PositionAdjustment` 的 `UnitCost`。前端 `HoldingsTab` 有 `holdingsSummary.unitCostHint` 说明。
- **影响**：用户若以为「留空 = 成本未知」，实际会把市价记成成本，未实现盈亏被压成 0。这是有文档的产品选择，不是静默 0，但容易在未读 hint 时记错成本。
- **建议**：留空应保持 `CostBasisAvailable == false`（与 CSV `missing_cost` 警告一致），或强制填写。

#### [P3] Holdings CSV 在 History 已开始后不生成头寸 Activity

- **现状 / 证据**：`planHoldingsCSV`（`csv_plan.go` L461–475）只插入 `Holding` + 可选手工报价；`origin != nil` 时只加 `missing_cost` 警告。`commitCSVImportTx` 仅在账户 CSV 带 `Activity` 时 `markHistoryDirtyTx`。对比 `CreateHolding` 会写 `PositionAdjustment`。
- **影响**：CSV 导入的是「现在的持仓」，不是带成本的历史开仓。当前估值会出现新持仓；已闭合快照通常不受「今天才创建」的持仓影响。用户若把 CSV 当历史回填，成本与历史分析会一直不完整（有警告）。
- **建议**：在产品文案中写清；若要支持回填，需要数量日期与成本，并走与 UI 相同的 Activity。

### 前端状态与交互

#### [P1] CSV 提交后未失效 Insights / History / 持仓聚合查询

- **现状 / 证据**：`frontend/src/queries/data.ts` `useCommitCSV` `onSuccess`（L101–108）只失效 `accounts` / `holdings` / `instruments` / `directory` / `overview` / `portfolio`。后端 `CommitCSVImportBuilt`（`csv.go` L273）会 `invalidateAnalysis()`（进程内 memo），但 React Query 仍缓存：
  - `queryKeys.analysis.all`
  - `queryKeys.history.all`
  - `queryKeys.analytics.instrumentHoldings`（Holdings 页数据源）
  - `queryKeys.quote.all`
  正常账户/持仓变更走 `invalidateCurrentValuation` / `invalidateHoldingChange`（`frontend/src/queries/invalidation.ts` L34–56），CSV 没有复用这套。
  `PortfolioPage` Holdings tab 为 `keepMounted`（`PortfolioPage.tsx` L212），`HoldingsTab` 用 `useInstrumentHoldings`（`HoldingsTab.tsx` L18、L39），**不**订阅 `queryKeys.holdings.all`。
- **影响**：导入账户/持仓/标的后：总览可能更新，但已打开的 Holdings 汇总、Insights、History 时间线/快照健康会停留在导入前。用户以为导入失败或数据丢失。
- **建议**：`onSuccess` 调用 `invalidateCurrentValuation` + `invalidateHistoryReads` + quotes，与其它 ledger 写入对齐。

#### [P2] 设置精简后无法再改 `weekStart` / `dateFormat`

- **现状 / 证据**：`d1652a1 feat: streamline settings and consolidate portfolio navigation` 后，`SettingsPage` 的 dirty 检测（`SettingsPage.tsx` L90–98）不再包含 `weekStart` / `dateFormat` / `timeFormat` / `decimalPlaces`，页面也无对应控件。这些字段仍在 `SettingsDTO`（`internal/wailsapi/settings/settings.go` L35–41），并被：
  - `frontend/src/components/ui/date-picker.tsx`（`weekStartsOn`、`displayDate`）
  - `frontend/src/features/insights/ReturnCalendarTab.tsx` L268
  使用。`decimalPlaces` 在前端展示路径中已无引用（金额格式走 ISO 4217/`Intl`）。
- **影响**：Insights 日历周起始、全应用日期显示格式只能靠旧 settings 或默认值；新用户无法改「周日为一周之始」或日/月序。Reset defaults 会把这些一并打回默认且仍不能从 UI 改回来。
- **建议**：要么恢复精简的日期/周起始控件，要么在产品上写死并停止从 settings 读取以免造成「幽灵偏好」。

#### [P3] 收益率日历「当前年」用浏览器时区，不用家庭时区

- **现状 / 证据**：`frontend/src/features/insights/calendar.ts` `isCurrentYear`（L80–82）用 `new Date().getFullYear()`。同文件 `isFuture` 使用 `ymdInTimeZone`。
- **影响**：跨年附近、家庭时区与系统时区不同时，日历可能把「家庭日历上的今年」标错。窗口很窄。
- **建议**：与 `isFuture` 一样用 household/settings timezone。

### 备份 / 导入导出 / 安全

#### [P2] 备份写入完整密钥，且 settings 序列化失败时静默变成空对象

- **现状 / 证据**：`internal/wailsapi/data/data.go` `CreateBackup` L99–105：`store.Load()` 或 `json.Marshal` 失败时保持 `settingsJSON := []byte("{}\n")`，不返回错误。成功时 Marshal 的是完整 `settings.Settings`（`internal/settings/settings.go` L101–104 含 `coingecko_api_key` / `tiingo_api_key` / `worker_api_token`）。IPC `Settings.Load` 则刻意不把密钥送到前端（`TiingoKeyStatusDTO.Configured` 等）。Settings UI 有 `settings.data.backupSensitive` 文案。
- **影响**：
  1. 备份文件是含密钥的明文包；拷贝/云同步备份等于拷贝全部行情凭证。这与「Load 不跨 IPC 传密钥」的边界不一致。
  2. Load/编码失败时备份仍显示成功，恢复后 settings 为空：FX 提供方、Worker URL、语言、密钥全部丢失，用户只会看到「验证 ok」。
- **建议**：序列化失败应让备份失败；备份内密钥需与产品安全说明一致（本地明文可接受，但不要静默丢包）。不要在失败时用 `{}` 顶替。

#### [P3] Worker Base URL 允许 `http`

- **现状 / 证据**：`internal/infrastructure/marketdata/worker.go` `configured`（L267）接受 `https` 与 `http`。Token 以 Bearer 发出。
- **影响**：用户若填内网/本机 http URL，token 明文出站。本地桌面、用户自填，不是远程注入。仍弱于「只允许 https」。
- **建议**：默认拒绝 http，或明确开发开关。

#### [P3] `LogFilePath` 经 Settings DTO 暴露

- **现状 / 证据**：`SettingsDTO.LogFilePath`（`settings.go` L29、L195）。Save 会忽略客户端改写的路径（测试 `settings_test.go` L302–307）。
- **影响**：前端能读本机日志绝对路径。本地可信 UI，不是远程 IPC。信息暴露面很小。
- **建议**：诊断区块只读展示即可；保持 Save 忽略客户端路径（已如此）。

### 架构与坏味道

#### [P3] 架构文档内部自相矛盾：schema 11 vs 10，版本 0.3.3 vs 0.3.4

- **现状 / 证据**：`docs/architecture/data-and-ipc-contracts.md` 开篇写当前线是 `0.3.3`、SQLite schema `11`，同文件后段（约 L79–80、L99）仍写「current supported schema is `10`」且「Schema `9` is the only older generation that migrates」。`docs/development/engineering-guide.md` 基线已是 `0.3.4` / Wails `v3.0.0-beta.23`。`docs/handoffs/2026-09-17-review-fixes.md` 不存在（可能未入库）。CoinGecko handoff 仍写「从未持有的标的 latest-only」，与当前测试 `TestUnusedInstrumentStartsSevenDaysBeforeCreation`（用创建日前 7 天拉历史）不一致。
- **影响**：后续迁移/审查容易按错 schema 代数；不造成运行时错误。
- **建议**：以 `schema.sql` / `schema_verify.go` 为准，删掉文档里过期的 schema 10 段落，并更新 handoff。

#### [P3] `instrumentHistoryStarts` 第二次 `LoadLocation` 忽略错误

- **现状 / 证据**：`internal/application/instrument_history_start.go` L28：`location, _ := time.LoadLocation(origin.Timezone)`。同文件 `instrumentHistoryStartDates` 已对同一 timezone 严格报错，因此正常路径不会用到 nil location。
- **影响**：目前无用户可见 bug；若将来有人把两段拆开，`Time.In(nil)` 会 panic。
- **建议**：复用已解析的 `*time.Location`，不要忽略错误。

#### [P3] Wails 适配层多处 `context.Background()`（备份/恢复/设置保存）

- **现状 / 证据**：`data.go` `CreateBackup` L108；`recovery.go` `ConfirmRestore` L103；`settings.go` `Save` L237 `s.app.WithWrite(context.Background(), ...)`。
- **影响**：窗口关闭或用户取消无法取消已开始的备份/恢复/设置写。备份有 `ExclusiveBackup` 与写锁，通常可完成。不是数据算错。
- **建议**：从 Wails 调用传入可取消 context，或至少尊重应用关闭。

## 已验证未复现 / 上次已修复

对照 2026-09-17 前后的契约（`internal/application/review_regression_test.go` 及相关源码），**未发现回归**：

| 契约 | 结论 |
| --- | --- |
| 标的 `quoteCurrency` 创建后不可变 | `TestInstrumentCurrencyImmutablePreservesHistoricalCost` 通过；拒绝写入且不改成本/现价 |
| 账户图标：省略保留、显式空清除、后续改名不恢复 | `TestAccountSettingsSavePreservesRequestedIcon` 通过 |
| Snapshot health/preview/rebuild 只含已闭合日，不含今天 | `TestSnapshotHealthAndPreviewOnlyIncludeClosedDays` 通过；`BuildDailyValuationSnapshot` 拒绝 `localDate >= today` |
| Data Health ≠ 当前估值；缺失不制成 0 | 当前估值 `MissingInputs` 与 snapshot `Complete` 仍 fail-close；本次新问题是 Insights **漏计已估值现金**，不是把缺失当 0 |
| IPC Load 不返回 provider 密钥 | Settings Load/status 仍为 configured 布尔；备份包是例外（见上） |
| `DefaultApplicationMenu` 等 native-only API | 未当作缺陷（build tag / server 模式） |

`GOCACHE=/tmp/nestworth-review-gocache go test ./internal/application -count=1 -timeout 120s -run 'TestInstrumentCurrencyImmutable|TestAccountSettingsSave|TestSnapshotHealthAndPreview|TestRebuildHistoricalSnapshotsInvalidStart'` → **PASS**（约 0.85s）。

## 待验证 / 风险

这些没有在活体环境或专用测试中钉死，不升格为已确认缺陷：

1. **CoinGecko `market_chart/range` 的 `from`/`to`**：实现与 `coingecko_test.go` 使用 `YYYY-MM-DD`（`end.AddDate(0,0,1)`）。官方文档常见 Unix 秒。适配器测试用假 Transport 固化了日期字符串，**未打真实 Demo API**。若线上只接受 Unix，历史同步会 `malformed` / 空序列，Data Health 显示缺口而不是编造价格。
2. **贵金属 COMEX 期货 vs 现货**：`domain.MetalFuturesMarket` / `GC=F` `SI=F` 是明确的期货合约价 + USD FX 换算。用户若期望伦敦现货/本币金价，估值会系统性偏离；这是模板设计，不是静默用错汇率。
3. **Restore 在关闭 DB 之后失败仍 `Quit()`**：`ConfirmRestore` 在 `RestartRequired` 时 `go s.quit.Quit()`（即使 `err != nil`）。若 swap 在 `Close` 之后失败，进程退出可能是必要的（会话已关），但未在原生实例上观察恢复日志/safety 文件是否可手工挽回。
4. **未持有标的的历史窗口**：代码用创建日填 `instrumentHistoryStarts`，再向前 7 天拉历史（有测试）。9/18 CoinGecko handoff 仍写 latest-only。行为以测试为准；活体同步成本/365 天 Demo 上限未验证。

## 未覆盖与未运行

- 未运行 `wails3 task check`、完整 `go test ./...`、frontend `pnpm` lint/test/build。
- 已运行：
  - `GOCACHE=/tmp/nestworth-review-gocache go test ./internal/application -count=1 -timeout 120s -run 'TestInstrumentCurrencyImmutable|TestAccountSettingsSave|TestSnapshotHealthAndPreview|TestRebuildHistoricalSnapshotsInvalidStart'` → **ok**
  - `GOCACHE=/tmp/nestworth-review-gocache go test ./internal/infrastructure/marketdata -count=1 -timeout 60s -run 'TestCoinGecko'` → **ok**
- 未打开原生 Nestworth 窗口，未点选真实家庭数据库。
- 未请求 Yahoo / Tiingo / CoinGecko / Frankfurter / Worker 活体行情。
- 未做打包、公证、签名、菜单/托盘、VoiceOver、多窗口。
- 未做贷款摊销/转账的独立手工场景（源码抽查未见与本次 P1 同级的不变量破坏；不声称该子系统无缺陷）。
- 生成 bindings 未当作手写问题。

本次 **未修改任何产品/应用源码**；本文件是唯一写入。未提交 git。
