# Nestworth 性能优化实施设计

状态：按阶段实现；P0 PR-A/PR-B 基础已合入 PR #110，B 的 epoch/小账本补充供评审，默认切换及后续阶段尚待验收。日期：2026-10-19。

Implementation tracking: [P0 PR-A/PR-B initial evidence](../testing/performance-p0-2026-10-19.md); [PR-B epoch/small-ledger follow-up](../testing/performance-p0-epochs-2026-10-19.md); [C1 coverage](../testing/performance-c1-source-coverage-2026-10-19.md); [C2 source/temporal evidence](../testing/performance-c2-temporal-2026-10-19.md); [corrected C3 bootstrap/fallback/RSS evidence](../testing/performance-c3-corrected-2026-10-19.md); [C3 completeness-lineage shadow](../testing/performance-c3-lineage-shadow-2026-10-19.md). C1/C2 and the default-off C3 no-change candidate are merged (PR123); the [certified C3 implementation and activation gates](../testing/performance-c3-certified-2026-10-19.md) now cover default-off no-op, source publication CAS and runtime-audited bounded publication. Stage-two rollout acceptance and later dependent stages remain gated. This does not authorize activation or treat Linux evidence as native acceptance.
基线：`2c585d1c3a26a55c94a5152d2c3f94275969d6b6`（main，已包含 PR #106、#107）。
本文按任务要求使用简体中文，是仓库默认英文文档规范的明确例外。

## 1. 决策摘要

推荐先减少不必要读取，再建立安全增量计算基础，最后改变存储布局。不要先扩大缓存或并行度。

1. **P0：可重复基线 + 有界证据窗口**。去掉每次为少量窗口结果排序全部历史键的工作；先复用现有主键，不默认增加历史试验中的大索引。保持全量源重检。
2. **P1：事务源版本与变更日志**。先以影子模式证明覆盖完整，再用同事务 CAS 替代发布时的全源读取；支持无变化快速确认。现有 `input_generation` 不能直接充当可靠源版本。
3. **P1：依赖和生效日期失效**。正常新行情与单条历史更正只重算受影响输入和规则后缀；不以“全局版本变了”作为全部重算的唯一依据。未知依赖、日志断档、版本不兼容仍全量重算。
4. **P2：共享持久化财务输入与独立规则证据**。在跨规则内存共享已存在的基础上，消除数据库中按规则重复存放的财务事实；规则真值、窗口、episode 证据继续独立。
5. **并行可排期但独立提交：分析行情批读、账户范围 hydration**。先批读行情，再缩小账户读取；保留转账来源、已清仓持仓和历史归属。首次历史顺序重放/检查点只有新 profile 证明必要时才实施。

优先批准 P0 和 P1 的安全基础，不批准一次性大改。每阶段发布后用相同夹具复测，再决定下一阶段。目标是减少 SQL 访问行数、重复序列化、重复财务计算；分页、后台任务和进度条属于交互改善，不能计作计算加速。

## 2. 已有架构与证据边界

以下路径和符号均已在上述基线核对；后续 PR 应重新核对主分支。相对链接指向仓库文件，符号是精确定位点。

| 路径 / 符号 | 当前行为 | 对方案的约束 |
| --- | --- | --- |
| [`review_reconcile_v2.go`](../../internal/application/review_reconcile_v2.go) `ReconcileInvestmentReviewsV2` | 显式刷新，capture → prepare → ledger write → publish；冲突最多 3 次重新捕获 | GUI/MCP 必须继续共享同一用例；不能让列表读隐式触发重算 |
| [`review_projection_capture.go`](../../internal/infrastructure/sqlite/review_projection_capture.go) `CaptureReviewProjection` | 单一只读事务捕获财务、规则、处理状态；全指纹相等才读取可复用证据；按窄键排名后取 JSON | key-first 已实现；窗口 SQL 的 `ROW_NUMBER` 仍遍历历史键 |
| [`review_projection_cache.go`](../../internal/application/review_projection_cache.go) `newReviewProjectionCache`、`dailyInput`、`portfolioForDay` | publication 内共享每日快照/输入、按 instrument 索引活动、复用 quote series | 不能把“新增每日内存共享”重复列为收益 |
| [`review_projection_plan.go`](../../internal/application/review_projection_plan.go) `planReviewRuleInstrumentIncremental`；[`review_daily_policy.go`](../../internal/domain/review_daily_policy.go) | 源不变时使用已持久化日期槽和有界滚动增量；策略时间线单遍计算 | 保留完整连续 true 后缀与三值逻辑，不能只取最近 N 个命中 |
| [`review_projection_publish.go`](../../internal/infrastructure/sqlite/review_projection_publish.go) `PublishReviewProjection` | 事务内全源重捕获和指纹验证；每规则一次 revision 检查；批取 state epochs；prepared input/result writes；同事务更新 episode/control | 上述批处理已经存在；删除重检前必须提供等价并发保证 |
| [`review_source_token.go`](../../internal/domain/review_source_token.go) `ReviewHistoricalFingerprint` | JSON 加 exact 私有标量的确定性指纹，覆盖删除与原位价格/FX 更正 | 不能仅用 JSON 或 MAX(timestamp)/COUNT 替代 |
| [`historical_batch_repository.go`](../../internal/infrastructure/sqlite/historical_batch_repository.go) `loadHistoricalSnapshotBatchQueryMode`；[`review_historical_batch.go`](../../internal/infrastructure/sqlite/review_historical_batch.go) | 历史 origin、状态、偏好、bindings、行情/FX/coverage、活动、更正和成本都参与读取 | 更正链及元数据也是依赖，不仅是 quotes/activities |
| [`repository.go`](../../internal/infrastructure/sqlite/repository.go) `loadLatestValues` | 已按账户使用 `idx_account_values_latest` 查最新值 | 不再做同一优化；保留 effective_at、created_at、rowid 次序 |
| [`analysis_service.go`](../../internal/application/analysis_service.go) `loadInputs`、`memoize`、`Invalidate` | bounded memo + generation 已存在，使用 daily valuation snapshots；仍逐 instrument `ListInstrumentQuotes`，先 household portfolio hydration | 优化读取与范围，不新增第二套收益算法；memo generation 防止旧结果回填的语义保留 |

当前 schema 是 [`database.go`](../../internal/infrastructure/sqlite/database.go) 的 `CurrentSchemaVersion = 19`，review 表来自 [`review_schema_v19.sql`](../../internal/infrastructure/sqlite/review_schema_v19.sql)。工程指南中 schema 18 的段落不是本设计迁移起点。

### 2.1 历史测量，不能当作本次结果

主要来源：[每日 review 验收记录 / Performance measurement and limits](../testing/investment-review-daily-acceptance-2026-10-16.md#performance-measurement-and-limits)。下表都是该记录中的特定历史运行，不是本任务新测的 p95，也不是相同环境下所有方案的配对比较。

| 场景 | 历史观测 | 解读 |
| --- | --- | --- |
| rewrite 后 365 天 warm 单次 | capture + fingerprint 7.938s；planning 287ms；atomic publish（含重检/写入）3.136s | 另一次只读诊断 recheck 7.592s **不能再加进端到端时间**；内存 fingerprint 640ms 也不应重复加入 capture |
| 同次 capture | 20 个 evidence 查询合计返回 6,000 行，Query-to-Rows.Close 5.593s；100,100 quotes 977ms | 包含迭代、scan、JSON 消费；不是纯 SQL 执行时间；返回行数少不代表访问历史键少 |
| 30 天小范围变化 | 新增 100 quotes + 4 FX，或更正单条历史 quote，都使全部 62,000 pair-days 重算 | 一次 append：planning 3.354s、publish 5.136s、128,241 SQL executions、37,538,300 序列化字节；不是 62,000 账户 |
| 365 天索引试验 | key-first 查询 5.366s → 2.119s；建索引 2.248s | DB pages +127,528,960 bytes；DB+WAL+SHM +256,042,808 bytes，两者不能混称持久索引大小；单次试验不足以支持默认加索引 |
| rewrite 前 clock-only 30 次 warm | 365 天 p95 17.262s；30 天 p95 7.290s | 这是旧实现的分布，不是 rewrite 后分布；此前不同 cold 数字也不能直接当受控前后对比 |
| rewrite 前列表首页 | 365 天 p95 47.570ms；30 天 p95 25.696ms | 已低于 150ms；不能用首页达标证明 reconcile 达标 |

历史标准夹具实际是 **1 个 brokerage 账户、100 个正持仓/100 instruments × 20 rules = 2,000 rule/instrument pairs**，100,000 生成行情 + 100 baseline manual quotes，50,000 cash-only activities，30/365 天；FX 分别 120/1,460。现金活动代表更正链查询的无关背景负载，并不代表已有大量交易重放证据。见 [`newReviewFullPipelineFixture`](../../internal/application/review_full_pipeline_performance_evidence_test.go) 和 `seedReviewFullPipelineFacts`。

[账户最新值测量](../development/account-value-read-performance.md)是 warm SQL 微基准，不能外推启动时间。[分析内核测量](../development/analysis-performance.md)只计 `ComputeAnalysis`，500 USD balance components、1,096 天，无活动和行情；优化后历史中位 3.217514s、累计分配约 2.148GB，既不是峰值 RSS，也不是整页耗时。不能拿它说明实际投资组合 SQL 已无瓶颈。

### 2.2 目标与阶段门槛

- **用户/既有产品目标**：review cold 完整 reconcile 3s、warm 500ms、列表首页 150ms；前两者仍未通过历史验收。冷启动是否含进程启动、OS 冷页及 UI 必须在报告中分列，不偷换为 kernel 时间。
- **本设计建议的验收口径**：完整 reconcile 从调用到原子发布完成；warm no-op、clock-only、行情追加、历史更正分别报告，不混为一个 p95。目标平台为独立测量的原生 Apple Silicon；Linux CI 只报告自身证据。
- **阶段工程门槛**：正确性差异为零；P0 新查询输出与旧查询全等，访问量随窗口而非总历史增长；P1 无变化路径不读取全部 quote/evidence JSON，且并发更改必被拒绝；P2 单 instrument 变化不重写无依赖 pair-days。复杂度门槛不等价于毫秒承诺。
- 建议回归阈值：相同机器配对 30 次样本，p95 或峰值 RSS 回退 >10% 时阻止默认启用并分析噪声。10% 是待评审的工程阈值，不是既有产品 SLA；3s/500ms 未达成就继续标记未达成，不提高预算掩盖失败。
- 本文不承诺加速倍数；存储共享的理论行数节约也不是实测磁盘节约。

## 3. 可复现基线和工作负载矩阵（PR-A）

现有命令可直接运行，使用测试创建的临时合成数据库；先确认临时卷空间，运行过程禁止 provider 网络、真实账本、云备份和真实设置：

```sh
mkdir -p /tmp/nestworth-perf-isolated
TMPDIR=/tmp/nestworth-perf-isolated GOCACHE=/tmp/nestworth-perf-go \
  NW_REVIEW_PERF_EVIDENCE=1 NW_REVIEW_PERF_MODE=stages365 \
  go test ./internal/application -run '^TestReviewFullPipelinePerformanceEvidence$' -count=1 -v
```

现有模式：`stages` / `stages365`（30/365 天分段诊断）、`profile` / `profile365`（查询与索引试验）、`warm`（30 天 clock-only）、`warm365`（现有默认 365 天分支）、`warm-marketdata` / `warm-correction`（30 天）、`cold`（365 天）。不要声称已有任意参数矩阵；PR-A 才将 fixture 构造器参数化，并让未知模式直接报错。cold 模式仅记录，不自动证明预算通过。

| 夹具 | 账户 A | 持仓 H / instruments I | 活动 E | quotes Q / FX | 日期 D | 规则 R | 用途 |
| --- | ---: | --- | --- | --- | --- | ---: | --- |
| R30 / R365（现有） | 1 | 100 / 100 | 50,000 cash-only | 100,100 / 120 或 1,460 | 30 / 365 | 20 | 保留历史证据可比性 |
| S（拟新增） | 1 | 10 / 10 | 0、100 混合交易 | 310 / 120 | 30 | 1、20 | 小账本回归与规则乘数 |
| A（拟新增） | 1、20、200 | 总 H=100 / I=100 | 固定 50,000 | 固定 100,100 / 1,460 | 365 | 20 | 单独改变账户数量；另设多个账户共持一个 instrument |
| E（拟新增） | 20 | 100 / 100 | 0、5,000、50,000；cash-only 与 buy/sell/transfer/cost 各独立变体 | 固定 R365 | 365 | 20 | 分离无关扫描与有效 replay 成本 |
| Q（拟新增） | 20 | 100 / 100 | 固定 5,000 混合 | 10,100、100,100、1,000,100 / 固定 1,460 | 365 | 20 | 行情历史深度；百万行仅离线容量测试 |
| D/R（拟新增） | 20 | 100 / 100 | 固定 5,000 | 固定 100,100 / 固定 1,460 | 30、365、1,095 | 1、20、50 | 分开改变 D 和 R，禁止全部相乘造成无法解释的实验 |
| Analysis（现有 + 新增） | 500 balance components；另设 20 accounts | 0；另设 100 holdings | 0；另设 5,000 混合 | 0；另设 100,100 | 1,096；另设 365 | 不适用 | 保留 kernel 基线，并补 SQL → ensure snapshots → kernel → DTO 的整条路径 |

每一可承受夹具执行下列场景，昂贵容量变体仅跑代表性子集并明确缺项：

1. 全新投影 cold；另测重开进程且 DB 投影已存在的 startup。OS 冷页需可控设备，不用普通“新进程”冒充。
2. 相同 revision、相同闭市日重复调用 no-op；随后时钟推进一天但无新行情（clock-only）。
3. 新日行情/FX append；同日晚到行情；单 instrument 历史 price correction，分别位于近期与 origin 附近。
4. 历史活动回填、linked reversal/replacement、删除/withdrawal；规则阈值、lookback、rolling/consecutive、target、timezone/cutoff edit。
5. capture 后、CAS 前插入变化；同时 acknowledge/suppress；跨本地午夜；发布中取消、SQL 故障、进程崩溃恢复。

PR-A 修改 `review_full_pipeline_performance_evidence_test.go`、`review_perf_sql_metrics_test.go` 并新增 analysis end-to-end evidence test（均为后续实现 PR 的文件，不在本次添加）。记录固定随机种子、clock、schema、commit、机器/Go/SQLite、GOMAXPROCS、CPU 配额、存储介质；保留 fixture manifest 和原始日志，不提交 DB。每个变更场景从相同合成快照克隆后执行，避免连续追加导致每个样本工作量不同；另测真实连续 30 日增长。

计量项：capture、纯 fingerprint、plan、序列化、publish 内 recheck/CAS、SQL 写入、commit、首页；每项列出包含关系。记录 QueryContext/Next/Scan/decode 和整个 Rows.Close 时间，EXPLAIN、访问/返回行数（可用计数器明确来源）、SQL executions、pair-days、共享 input-days、B/op/allocs、峰值 RSS、DB pages、checkpoint 前后 DB/WAL/SHM、日志写放大、写事务占用时间。正式计时串行运行，不与 race/build/profile 同时执行；30 次 nearest-rank p95 即排序后的第 29 个样本，冷样本不足则只报 n 和原始值。

## 4. 有界 evidence 查询（PR-B，依赖 A，无迁移）

修改 `review_projection_capture.go`，提取私有 `loadReusableReviewWindow`，保持公开 `ReviewProjectionRepository` 不变。目标集合来自当前规则目标、已评估 instruments、未结束 episodes 的并集，不能只用当前正持仓。按 instrument 和完整输入命名空间获取最后 W 个日期槽；W 使用现有 immediate/consecutive/rolling 定义，不是 SQL 中 true 的数量。

现有主键顺序为 `(household_id,instrument_id,timezone_version,on_date,input_policy)`。优先实验精确前缀定位 + 最后日期/窗口范围，避免 household 全历史 `ROW_NUMBER`：

```sql
-- 概念 SQL；使用既有事务与参数化绑定，按每个已知 instrument 定位。
SELECT on_date
FROM investment_review_daily_inputs
WHERE household_id=? AND instrument_id=? AND timezone_version=? AND input_policy=?
ORDER BY on_date DESC LIMIT ?;
-- 取得日期键后，以完整主键取 input JSON，并以完整 result 主键 join。
```

不能盲目用 `latestOn - W` 限定：timezone 的不存在日期、缺槽、恢复后的最新槽和 rule history origin 都可能使集合不同。先保持与旧 SQL “最后 W 个已存日期槽”精确等价，再验证连续 civil-date 覆盖；缺槽必须进入修复/全量路径。`latest_streak` 的 latest_closed_on 与读取窗口锚点一致，保留完整连续 true 计数。

比较逐 instrument 小查询与分块 VALUES/CTE 请求表（参数数受控，例如先试 100 个 instrument 一块，仍以驱动实际限制为准）；一次 query 合并所有 rules 的既有试验曾回退，不能以 SQL 数少作为唯一结论。现有主键中 input_policy 位于日期之后，旧规则版本很多时仍可能扫描大量键；EXPLAIN 与 rule-version 压力测试失败才考虑增加 `(household_id,input_policy,timezone_version,instrument_id,on_date DESC)` 索引，另开 schema PR。禁止把 JSON 放入覆盖索引。

验收：30/365/1,095 天、旧 condition versions、缺槽、长 streak、清仓与重新进入的返回集合及 typed evidence 全等；SQL 访问量/总耗时下降，写入和磁盘成本记录。回滚只切回旧 SQL，不影响财务或持久化协议。本阶段仍保留全部 fingerprint/recheck，因此不声称 warm 500ms 已解决。

## 5. 事务版本和变更日志（PR-C1/C2/C3，依赖 A）

### 5.1 选择与 schema

选择 SQLite **数据库触发器维护的单调源版本 + 精简变更日志**，而非应用内计数器、mtime、MAX(date)、仅行情 ID hash。数据库触发器覆盖同数据库的直接 DML 和导入路径；应用仅写脏标记无法证明覆盖完整。新表/名字均为提案：

```sql
CREATE TABLE financial_source_revision (
 household_id TEXT PRIMARY KEY REFERENCES households(id),
 revision INTEGER NOT NULL CHECK(revision >= 0),
 journal_floor INTEGER NOT NULL CHECK(journal_floor >= 0),
 tracking_version TEXT NOT NULL, tracking_epoch TEXT NOT NULL
);
CREATE TABLE financial_source_changes (
 household_id TEXT NOT NULL REFERENCES households(id),
 revision INTEGER NOT NULL, change_ordinal INTEGER NOT NULL,
 source_table TEXT NOT NULL, source_id TEXT NOT NULL, operation TEXT NOT NULL,
 old_scope_json TEXT, new_scope_json TEXT,
 old_effective_at TEXT, new_effective_at TEXT,
 dependency_kind TEXT NOT NULL,
 PRIMARY KEY(household_id,revision,change_ordinal)
);
```

每个源行变更原子递增 revision 并写日志；同一业务事务可以递增多次，不能假设 revision 等于事务数。`old/new_scope_json` 只保留定位依赖所需的 account/holding/instrument、币种对、provider/binding、effective date、关联更正源等，不保存整个金融 payload。需要审计的完整值继续由原始账本承担；旧依赖无法从当前行推回，必须在变更时留下 OLD 信息。未知或无法解析范围写 `dependency_kind=household_all`，不能漏记。跨 household 移动对 OLD/NEW 两侧都记；无有效 household 的历史数据必须先阻止或修复，不能静默跳过。

暂不为日志添加第二索引，按主键扫描 `(published_revision,current_revision]`；先以全 household revision 做 CAS，保守冲突可接受。日志压缩前持久化 `journal_floor`，低于 floor 的消费者全量重算。不得把“日志删完”解释成“没有变更”。DB restore/import 后初始化新的 `tracking_epoch` 并纳入 token，使同数字 revision 不会和旧进程 memo 相等。

当前为 v19：C1 预定 v20 additive migration（合并时重新分配，不能抢占其他 PR 版本）。新增 `source_changes_schema_v20.sql`、`upgrade_v19.go`，调整 `database.go` embed/open、`schema_verify.go` 和专用 verifier；参考 `upgrade_v18.go` 的验证/恢复副本流程，不修改既有 v19 文件冒充迁移。既有 v19 数据创建版本行、将所有投影标为需重新验证；不伪造历史 journal。不在启动事务内全历史回填。索引和后续共享事实迁移分别排号。

同时更新 `export_repository.go`、`internal/infrastructure/backup`、`internal/application/json_export_schema_test.go` 的版本/完整性约定。日志视为派生失效元数据：完整 SQLite backup 必须一致保留；JSON 导入可丢弃并重建 epoch、全失效，不允许恢复旧缓存 token。只读库不补版本行、不写日志，使用原只读路径并报告不可发布。

### 5.2 覆盖矩阵先于切换

C1 交付一张逐表/逐字段/write-owner 的依赖清单，逐一对照 `loadReviewHistoricalBatchQuery` 和 `ReviewHistoricalFingerprint`。起始审计范围包括：

| 来源 | 必须捕获的变化 |
| --- | --- |
| households、accounts、account_ownership、members、holdings、instruments | 币种、可见性/归属、状态、类型、持仓身份/数量来源；目录字段先允许保守失效 |
| history_origins 及全部 history_origin_* | 起点、baseline、起点持仓/成本/偏好/归属；任何改变从起点全量 |
| activities、activity_effects、activity_trade_details、activity_dividend_details、activity_correction_groups | insert/update/delete、有效时间、数量/成本、链接更正、跨账户 transfer；成本事件是 `cost_basis_repository.go:costBasisEventsSelect` 从 activity_effects/trade details 解析的领域模型，并无独立 cost_basis_events 表 |
| account_values、account_cash_values、holding_quantity_values、各 account/instrument/holding state observations 与 ownership | authoritative baseline 与状态的旧/新范围；与 replay 派生记录区分，防止重算反过来无限使自己失效 |
| instrument_quotes、fx_quotes、instrument_observation_slots、fx_observation_slots、market_data_day_status | 原位修改、withdrawal、删除、replacement、价格 basis、effective date 与 availability 变化 |
| fx_preferences、各 preference observations、instrument_provider_bindings/revisions、app_configuration 中相关 policy/provider 配置 | FX/provider 路由与 policy 变化；凭证本身不进日志、不输出 |
| history_snapshot_state / resolver policy | 审计其中哪些字段是源语义、哪些是派生状态；不能仅依赖其 generation，也不能把投影刷新当作金融源变更 |

表清单是审计起点而非完整性证明；PR-C1 必须补齐实际读取闭包、FK 级联与删除顺序。对每个相关表执行合成 INSERT/UPDATE/DELETE（含不改变数量但改变 decimal 值）并验证版本变化；同事务 rollback 后版本和日志不变。审计 `INSERT OR REPLACE` 的隐含 DELETE、UPSERT、recursive_triggers、恢复路径；无法保证 OLD 捕获的 REPLACE 写路径需改成明确 UPDATE/DELETE+INSERT，或可靠地全 household 失效。所有触发器和关键 PRAGMA 纳入 startup verifier；缺失/篡改时禁用快路径或阻止写入。

### 5.3 分阶段接入与 CAS

- **C1（仅跟踪）**：新增存储、触发器、migration/backup 测试；保留旧算法。测写入耗时与日志大小，避免一次性引入增量 planner。
- **C2（影子验证）**：`domain/review_projection.go` 新增 `SourceRevisionToken`（household、epoch、revision、tracking/resolver/evaluator versions）；capture 同一个 read transaction 读取 token 与全量源；publish 同时验证新 token 和原全量指纹。任意“相同 capture cutoff 下 token 相等但指纹不同”阻止默认启用，定位漏记；token 不同但结果相同属于保守失效。
- **C3（切换）**：新 `review_source_revision.go` 提供事务内读 token 和 journal；修改 capture/publish/reconcile。受 verifier、tracking version、完整 checkpoint 约束后启用 fast path。保留配置开关与完整旧重算路径，不只是保留测试代码。

```text
read transaction:
  token := read source revision + epoch + semantic versions
  control/rules/handling := read one coherent snapshot
  if token == published token AND per-rule closed-date frontier unchanged
     AND rule/config/state revisions unchanged AND projection complete
     AND published temporal certificate covers captureNow (no clock rollback):
       prepare no-op confirmation
  else:
       load full inputs (C3 初版) / journal + dirty inputs (PR-D)
       compute outside transaction
write transaction:
  acquire SQLite write reservation before checking (driver-specific implementation tested)
  verify token, control, every rule revision, handling/episode revision
  verify current per-rule closed-date vector (timezone/cutoff/午夜)
  verify capture temporal certificate covers publishNow; otherwise discard and recapture
  CAS published/control revision using captured expected values; require affectedRows == 1
  write staged rows, episodes, checkpoint and published token atomically
  commit
```

源版本的检查必须在持有写事务保护后进行；Go ledger mutex 不能保护其他连接/进程。不能先在读事务检查，再另开事务写。SQLite deferred snapshot 升级冲突应作为整个事务 conflict 重试，禁止以旧 snapshot 重试写；若使用 `BEGIN IMMEDIATE`，通过专用连接事务封装实现，不能在已 `BeginTx` 的事务里再次 BEGIN。具体驱动事务方式在 C2 合成双连接测试中确定。

C3 初版只加速已证明时间依赖不变的同日 no-op 和 publish 重检；clock-only 若还需未持久化的历史财务输入，仍会全量 capture。避免该扫描依赖 D 的受控 dirty input loader 或 E 的完整共享事实，不能提前计入 C3 收益。

no-op 也需短事务确认，以避免 capture 后变更被当作“最新完成”；无变化不重复写 daily rows。clock-only 不是 no-op，闭市日期推进、suppression 到期或财务生效时间激活都必须重新评估。发生三次 conflict 后沿用明确冲突返回，不无限占锁。配置、规则、ack/suppression 与时间向量检查不能被 financial token 替代。

### 5.4 时间生效边界：同日且无 DML 也可能变化

已核对 `internal/infrastructure/sqlite/activity_repository.go:listActivitiesUntilQueryMode`：capture 只读取 `effective_at <= capture cutoff` 的 economic activities；`review_projection_plan.go:planReviewRuleInstrument` 及增量分支使用 `now + 1ns` 调用半开区间的 position replay，使恰在 now 生效的效果被包含。因此，预先写入的未来 quantity event 可以在同一 civil day 内变为有效，改变当前 eligibility、清仓/重新进入 epoch；此时 source revision 和 latest closed day 都可不变。仅比较上述两个值不构成 no-op 证明。数据库写锁也不能阻止墙上时钟跨过生效时间。

**保守默认**：未完成时间依赖闭包审计、无法给出完整有效区间时，禁止同日 no-op，执行全量 capture/plan；不得把空 future-event 查询或未发现边界当作证明。用于替代 publish 全量重检的 token 仍需保留时间校验，不能顺带删除 existing cutoff/time 验证。若 publish 时刻已越过 capture 的有效区间，丢弃 stage 并重新 capture，不能只用同一旧 cutoff 再算一遍指纹。

可选安全优化是同一个 capture 读事务内建立持久化 `ReviewTemporalCertificate`，与 published token/checkpoint 原子保存：`evaluated_at`、`next_effective_transition_at`、`temporal_policy_version`、依赖 scope 和 source token。边界使用 UTC instant 和现有纳秒精度，不按日期字符串或 wall-clock 秒取整；有效区间是 `[evaluated_at, next_effective_transition_at)`。`next_effective_transition_at` 必须从**未被当前 cutoff 截掉的权威源**查询得到，不能只遍历已捕获的 Activities。没有下一边界仅在完整审计证明后表示正无穷，缺失 certificate 表示不可复用。

必须审计并纳入的时间相关输入包括：

- 当前 position/eligibility/epoch 所需的 quantity activities、transfer 双端、reversal/replacement 与更正链；从预先存在的未来经济事件求边界，不能仅查未来新插入时间。
- 参与 capture 或 replay 选择的 account/holding/instrument 状态、ownership、origin/baseline、成本与偏好/provider binding 的有效时间；按真实 resolver 的 inclusive/half-open 规则映射激活点。即使某类仅影响已闭市历史，也需证明它不影响当前 scope 后才可排除。
- 行情/FX 的 quoted/value-effective/provider 时间、有效日期、slot 和 coverage 状态中受 capture cutoff/当前时刻筛选的部分；规则 timezone/cutoff、calendar/tzdata、闭市日期向量，以及 suppression expiry。配置版本变化使证书失效，不能用一个 household midnight 代替各规则/市场边界。

第一版可取全 household 的最早边界，接受过早失效；不要求一开始精确到 instrument。任何未审计 source/时间谓词使证书不可用。边界跨越无需写入 journal：调度器把它当作时间失效事件，重新捕获并重建当前 position/eligibility/epoch；只有证明历史 daily facts 未变时才复用那些事实。D 的 dirty planner 必须接受这种没有 DML 的失效来源，不能只遍历 journal。

读取 no-op 时要求 `published.evaluated_at <= captureNow < published.next_transition`；publish 的最终时间检查要求 `captureNow <= publishNow < capture.next_transition`，同时检查所有 revision 和闭市日期向量。将最终检查时刻定义为该 publication 的逻辑 as-of，并返回/保存该时刻；提交完成后已跨边界的读取必须标 stale，不能把 as-of 结果宣称为较晚时刻仍最新。检查后或 commit 后墙上时间继续前进不延长证书有效区间。

任何观测到的时钟回退（包括同日回退、回到上一闭市日、重启后 now 小于持久 evaluated_at）都禁止复用旧证书/当前 epoch，重新 capture 和全量 oracle 路径；不以 `max(now, oldNow)` 掩盖回退。进程内另外记录最后观测时刻，发现回退即失效，即便仍高于上次 publication 时间。重新计算依旧遵守已有 episode/ack 生命周期语义，不通过删除处理记录来“回到过去”；无跨边界的前进才允许证书复用。

C2/C3 必须新增以下确定性合成测试（注入 clock，不 sleep），作为默认启用的阻断门槛：

1. 首次 capture 之前就写入 T 时刻的未来 quantity event；在同 civil day 的 T−1ns、T、T+1ns reconcile，期间无 DML，明确断言 source revision 与 closed-date vector 相等。T−1ns 可复用，T 起必须重新评估，与分别在各时刻全量 capture/plan 的 oracle 全等。
2. 分别测试退出正持仓、从零进入、clear/rebuy 与 transfer，比较 eligibility、position epochs、episode 关联/ack、coverage 和结果，不只比较 daily bool；即使已闭市 votes 完全不变，当前状态仍需更新。
3. capture 在 T 前、publish 在 T 或 T 后，断言旧 stage 被拒绝并重新 capture；故障/重试不能产生部分 episode 更新。增加边界后无新写入的重开进程测试，确保 certificate 和 token 组合不误判。
4. 注入同日向后跨 T、跨午夜回退、重启后早于 evaluated_at，以及进程内回退但仍晚于 published evaluated_at；全部禁用 no-op。与同一目标时刻和相同处理历史的完整 oracle 比较，并验证 ack 不丢失。
5. 对上述每类时间输入分别做边界用例；未支持的类别必须走保守路径。证明边界前复用和边界后失效、精确纳秒端点、DST 与每规则时区均正确；测量中把 activation 与普通 no-op 分列。

## 6. 依赖与后缀重算（PR-D1/D2，依赖 C3、B）

D1 新增应用层 `review_invalidation.go`、domain dependency 类型及纯函数 `PlanReviewInvalidation(changes, rules, frontier)`；先只记录 dirty 计划并与全量 oracle 比较。D2 接入 `review_projection_plan.go`、`review_projection_cache.go`、`review_historical_batch.go` 和 publication stage，保留旧 path。不要一次同时改变依赖算法和共享存储。

依赖粒度为 `(household, instrument/历史持仓范围, financial policy, timezone/cutoff, on_date, metric dependency)`；规则 spec 映射所需 amount/cost/quantity、价格 lookback、FX 模式。对于变化 c，初始 frontier 是 `min(old_effective, new_effective)` 映射到各规则 civil day 后的最早可能影响日期；交易成本/数量变化通常影响其后全部日期。新旧 instruments、账户与币种依赖取并集。无法确定 earliest date 时退回 origin。

| 变化 | 失效与重算规则 |
| --- | --- |
| 新日行情、同日晚到价格 | 对使用该 instrument/有效日价格的事实失效；price-change 同时考虑端点和 lookback baseline 候选窗，不能假设只影响该日 |
| 历史 quote 删除、replacement / 旧日期改新日期 | OLD/NEW 两个有效日期和旧/新 provider/binding 范围并集；重新解析候选与缺失 coverage；失去候选可能把 true 变 unknown |
| FX rate、FX source/provider preference | 按币种转换依赖、FX economic-date 分支和生效偏好重算；无法界定影响币种时 household 全量 |
| backdated activity、reversal/replacement、更正链 | 从旧/新有效时间与更正原记录时间最小值开始；覆盖成本事件、原/目标 holding、transfer 双端；删除必须使用 tombstone OLD 范围 |
| account ownership/member/filter、归档、清仓 | 重建受影响历史 scope；不能把当前归属代替历史归属；无法精确映射时全账户/household 后缀 |
| rule threshold/edit/target | 只该规则条件版本从所需历史起点重算；可复用兼容财务事实，但不能沿用旧规则真值或首次触发日期 |
| timezone/cutoff、calendar/tzdata、resolver/evaluator policy | 命名空间版本改变；重建闭市日与相关事实/规则；未实现精确迁移则全量，禁止用相同日期字符串继续复用 |
| suppression expiry、ack | 不改变金融事实；更新处理视图并 CAS state revision，保留已确认 episode 的身份和历史 |

传播不能停在一个 rolling window：恢复一个早期 false/unknown 可能改变连续 true streak、episode 边界和首次触发日期。重算从 dirty day 前的**有效 checkpoint**开始，携带 rolling 最近 W 个三值 votes、完整 consecutive count、position epochs、未结束 episode 与 acknowledged 状态关联。为得到 dirty day 输入，额外加载最大 metric lookback 与经济日期 baseline，但这些前置读取不等于把所有前置结果改写。

默认重算到 latest closed day。只有在同日期上财务输入指纹、完整策略状态（非仅最终 bool）、epoch、episode 边界/证据关联均与旧结果一致，且后续依赖没有 dirty 区间时，才能提前停止；D2 初版不实现提前停止以减少证明成本。日期槽包括 false 和 unknown，unknown→false→true 恢复不得复活已被证明结束的旧 episode。same-day clear/rebuy 按原子数量效果建立新 epoch，持仓退出不直接删掉历史窗口或 ignore。ack 只按既有 proven policy interval 规则迁移，不按“instrument 一样”复制。

验收复杂度：在孤立 instrument quote correction 场景，未依赖它的 rules/instruments 不产生日结果写入；append 仅新日及依赖影响窗；成本回填长后缀允许昂贵，必须如实报告。日志断档、checkpoint 不可信、rule 版本不兼容立即全量重算，并保持旧 published projection 标 stale，不能先清空用户可见历史。

## 7. 共享持久化输入（PR-E1/E2，依赖 D）

目前 `investment_review_daily_inputs.input_policy = rule:<id>/v:<condition>`，即使内存计算共享，持久化仍按规则重复。新增 `review_financial_daily_facts`（下一个独立 schema 版本）和规则→fact 引用，不直接重命名既有表：

- fact 主键：`household_id, instrument_id, scope_key, timezone_version, on_date, financial_policy_key, fact_kind`。`scope_key` 起初固定为 household aggregate，预留未来账户过滤不能碰撞；不得拿账户不同的结果共用。
- `financial_policy_key` 含 resolver、price basis、provider/FX 选择、cutoff/calendar、依赖需求版本；币种/FX mode/lookback 的派生 metric 使用明确 metric key。只在语义完全相同时共享，不能因日期/instrument 相同就合并。
- 保存 quantity/amount/cost 等 exact decimal 文本及币种、coverage/来源、input fingerprint、source revision；基础值与 metric facts 分离，避免将“所有规则所需字段的可变大 JSON”作为一个键。
- daily rule results 保留 `(rule,condition_version,instrument,timezone,on_date)`、三值真值、原因和独立证据；增加引用表 `review_result_fact_refs`，使用完整复合外键或稳定 fact ID。episode 触发证据继续保持历史版本，不能因共享 fact 覆写而改写已触发的解释。

E1 建新表和投影 adapter，先影子写入/比较、旧读不变；E2 修改 `review_publication_stage.go`、`review_projection_publish.go`、`review_daily_evidence_read.go`、`review_episode_evidence.go`，同事务写一次共享事实和多条规则结果，兼容现有 wire evidence。历史事实如被更正，应采用 revision 身份/保留引用版本，或把 episode 所需精确证据冻结；推荐后者延续现有 episode evidence 机制，并测试导出还原。

后台/显式合成迁移采用有界批次 staging，再原子切换有效版本；不在启动时强制全量 backfill。旧表保留到迁移验收和恢复期结束，GC 仅回收没有 rule/episode/checkpoint 引用且超过保留边界的派生行。峰值磁盘可能同时包含旧/新表、WAL 和恢复副本，迁移前检查空间，失败保留旧投影可读。

理论上相同 financial policy 下输入行从 `R×I×D` 接近 `K×I×D`（K 是不同财务策略/时区/metric 组合，可能接近 R）；规则结果仍是 `R×I×D`。不能承诺 20 倍缩小。报告实际 JSON bytes、索引 pages、重复率、join 读取成本及迁移峰值；不要用丢弃每日证据或改用 float 换空间。

内存预算：保留 analysis memo 的现有容量 2，不用无限 memo 抵消 SQL 成本。review 的 publication 内 map 仍可能随日期/策略数增长；A 记录峰值 live heap/RSS 后，E/F 试验按日期块释放 portfolio 与已序列化 stage（例如 30 日块只是试验值），仅保留后续依赖的 lookback、策略状态与 epoch。块写入使用独立 staging publication ID，用户查询只读已提交版本；不允许通过分块提前发布部分业务结果。最终 byte cap 和 chunk size 由低内存合成运行决定；超过预算选择落盘 staging 或明确取消，不能静默丢证据。

## 8. 首次历史与分析路径（PR-F/G/H）

### 8.1 首次历史顺序 replay / checkpoints：条件实施的 F

依赖 E 后的 cold profile。若成本仍集中于 `review_projection_cache.go:portfolioForDay` 重复历史 reconstruction，再修改 `historical_replay.go`、`historical_snapshot.go` 和 review cache：从 verified origin 顺序推进事件游标，按各 timezone/cutoff 产生闭市快照；financial event 一次应用，排序继续使用既有有效时间/更正/原子效果规则。不能只保存余额，必须携带持仓、成本批次、历史账户状态/ownership、偏好、epoch 与覆盖状态。

先做单次内存顺序 replay，独立证明与每日期全量 reconstruction 等价；随后才试持久 checkpoint。建议实验每 30/90 天而非预设生产间隔，checkpoint key 含源依赖 revision/frontier、schema、算法、calendar/policy 版本；dirty 时间之前最近有效点恢复。checkpoint 无效或损坏回 origin，取消/崩溃未完成点不参与读取。选择间隔依据 cold/回填耗时、checkpoint bytes、峰值 RSS，不因 warm 目标而无条件加检查点。分块 materialization 可以缩短持锁时间和支持恢复，但只有减少总工作才算性能提升。

### 8.2 分析 instrument quote batching：G，可独立于 C

`analysis_service.go:loadInputs` 已逐 instrument 调 `ListInstrumentQuotes`。在 `internal/application/repository.go` 旁定义可选 batching port，增加 `LoadAnalysisQuoteBatch(ctx, householdID, instrumentIDs)`，SQLite 实现放 `portfolio_repository.go` 或新 `analysis_input_repository.go`，使用有界参数块和一个一致只读事务；旧 port adapter 先保留。首版只批读、不裁剪日期，确保排序、decimal、price basis 与 provider candidates 和旧结果相同。后续裁剪需要单独证明 opening day、lookback/baseline 与历史缺失选择的边界，不能直接 BETWEEN 查询区间。

测试空/重复 IDs、跨 household ID 拒绝、closed holdings、manual/provider/FX、history origin opening；比较全量 inputs 与最终 Analysis result。请求从 I 次降为按 chunk 数（不是无条件 1 次），记录全部 Rows 消费、内存和 query 数，不宣称内核加速。

### 8.3 账户范围 hydration：H，依赖 G 的独立正确性证明

入口是 `repository.go:loadLatestValues`、`portfolio_repository.go:ReadPortfolioSnapshot` 与 `analysis_service.go:loadInputs`。先为普通账户列表/详情引入 internal scope-aware loader，在查询上游传 account IDs，复用既有最新值排序和 scanner；公共 `AccountFilter` 语义不改。此步骤无新索引，验证 `idx_account_values_latest` 并保留 UUID 与 rowid 反序 tie 测试。

分析范围收缩单独提交：先从 origin、daily snapshots、活动 effects、历史 ownership/state 得到期间 universe，再闭包扩展 transfer/reversal/cost 的来源与目标、已归档账户、已清仓 holdings、起点状态。在内核完成归因后应用输出 filter。仅以“当前选中账户的当前 holdings”加载会漏掉成本与 realized contribution，禁止实施。PR #106 的币种/类别过滤、#107 的统一 return summary 是 oracle 的一部分；未能证明依赖闭包完整前保持 household hydration，只启用 G 的安全 batching。

## 9. 正确性 oracle、故障恢复与验收

每个实现 PR 必须提供开关强制走基线全量路径；oracle 禁用增量状态、共享 fact 和 checkpoint，使用同一 authoritative 合成 snapshot 重建，避免两条路径共享同一个错误 invalidation 结果。对金融核心尽量复用既有算法而独立重建输入，必要时保留旧 SQL 为选择 oracle。

逐日比较：exact decimal 的值、币种与必要的 exponent，quantity/cost、行情/FX 来源和日期、coverage 缺失原因、每个 predicate 真值、rolling votes/consecutive count、first hit/trigger/recovery、epoch、episode 身份关联及 ack/ignore、全部分页 evidence 和游标 revision。随机 ID/计算耗时等非语义字段应使用可控生成器或明确映射后比较，不能忽略金融/生命周期字段。GUI 和 MCP 同 household/scope/query/revision 必须得到同结果，旧 revision 游标必须失效。

可直接扩展的既有测试入口：

- `TestReviewHistoricalSourceTokenDetectsQuoteDeletionWithoutGenerationBump`、`TestReviewPublicationRejectsDeletedFinancialSource`：新版本机制必须覆盖旧 generation 未变化的删除，不许删测试迁就快路径。
- `TestReviewHistoricalQuoteActivityAndFXCorrectionsInvalidateAndRecompute`、`TestReviewCorrectedPositionHistoryDropsObsoleteEpochStreak`：更正与 epoch oracle。
- `TestReviewPublicationRejectsConcurrentAcknowledgement`、`TestReviewPublicationRejectsConcurrentIgnoreAndMidnight`、`TestReviewPublicationFailureAfterFinancialCommitDoesNotHalfWrite`：处理中并发和事务边界。
- `TestReviewPublicationPreservesAcknowledgementAcrossUnknownDay`、`TestReviewSameDayClearAndRebuyUsesNewEpisodeWithClosedVotes`、`TestReviewIncrementalReplayPreservesFullDailyTrueSuffix`：unknown 恢复、重新进入与长 streak。
- `TestReviewCivilDateBoundaries`、`TestReviewFXEconomicDateBranches`、`TestReviewDailyCloseDoesNotCarryPrices`：DST 23/25 小时、不存在日期、FX economic date、闭市价格不向前填充；增加 tzdata 版本变化失效案例。
- analysis 原有归因/zero holding/FX/origin/reconciliation 测试加 transfer 来源闭包、选定币种/类别、closed holding 全量对照；不把未实现的范围裁剪算作既有测试已覆盖。

故障注入矩阵：源写事务 rollback、日志写失败、版本表缺失/触发器损坏、capture 后并发源写、CAS 后写失败、commit 前取消/kill、commit 后响应丢失重试、日志截断、epoch reset、磁盘满、backup/restore/import、只读库。任何失败要么没有提交，要么完整提交；财务事务已成功但投影失败时财务不得回滚，保留旧投影并标 stale/needs_repair，下一次可幂等恢复。ack 和 episode events 不作为可随意重建缓存删除。

回滚顺序：关新读/增量 flag → 原全量重算并原子发布 → 保留新表供诊断。已升级数据库不能靠旧二进制直接打开；schema rollback 只能走已验证的升级前恢复副本，并明确其中不含升级后新金融写入，必须先安全导出/迁移这些写入。普通算法回退不降 schema，不自动覆盖现账本。共享事实迁移失败清理未发布 staging，不清理当前版本。

实现 PR 的检查：相关 domain/application/SQLite 测试 + differential oracle、迁移/backup/export、双连接 race/故障测试；按工程指南执行 `go test ./...`、`go test -race ./...`、`go vet ./...`、Go build；wire/UI 改动还需生成 bindings（不提交生成物）、frontend lint/typecheck/test/build。真实 macOS 性能、Wails 取消、VoiceOver/IME、package/restore 验收单列，Linux 通过不代替这些门槛。

## 10. UI 状态是独立工作流

保留前台显式 reconcile；展示阶段进度与实际完成 pair-days，不凭线性百分比推断剩余时间。cancel 传 context，publish 前取消不提交；commit 已完成但响应被取消时下次读取 revision 判定真实状态。旧 projection 可读但注明 source revision、scope、最后闭市日和 stale/refreshing/failed；计算完成必须原子替换，不能显示一半新一半旧的 totals/episodes。分页和后台预热不放进 500ms 计算达标数字。GUI/MCP 暴露相同状态与错误，不新增另一套 MCP shortcut。

另一个独立任务处理 F12 preview errors、F5 rounding help、F18 reversal tests；本设计不修改其文件或重复实现。后续复用 F18 reversal fixtures 验证日志和 dirty frontier，F12 错误传递用于取消/冲突状态一致性，F5 文案不作为更改 exact money/舍入规则的授权。

## 11. 小 PR 顺序、替代方案与未决事项

| PR | 依赖 | 可评审交付及退出条件 | 默认回退 |
| --- | --- | --- | --- |
| A 基线 | 无 | 参数化矩阵、计量口径、固定数据和 oracle；无新延迟结论直到运行 | 原夹具 |
| B 有界查询 | A | 同集合、新旧 EXPLAIN/行数/耗时；无 migration | 原 key-first SQL |
| C1 跟踪 schema | A | 全 writer 审计、trigger verifier、恢复/导入、写放大报告 | 原 fingerprint 算法继续用 |
| C2 影子 token | C1 | 逐表 DML + 并发/rollback + 时间生效边界/回退，token 相等不能漏 fingerprint 变化 | 只记录诊断 |
| C3 CAS/no-op | C2 | 等价事务与时间证书证明、no-op/activation/clock-only 分开测；默认开启需人工审查覆盖矩阵 | 全源 recheck |
| D1 dirty planner | C3 | 纯依赖计划、保守失效与 oracle | 全 history |
| D2 suffix | D1、B | 每日期/生命周期全等；不相关结果不写 | 全 history |
| E1/E2 shared facts | D2 | 独立迁移 + 双写对照，再读切换；磁盘峰值及 evidence 不变 | 旧 input/result layout |
| F 顺序 replay/checkpoint | E 后 profile | 仅热点证据充分再拆成内存 replay / checkpoint 两 PR | origin 全重建 |
| G quote batch | A | 独立输入/result 对照和 SQL 计量 | 单 instrument 读 |
| H scoped hydration | G | 列表/详情先行，analysis 闭包另 PR；过滤/transfer oracle | household hydration |

替代方案比较：

| 方案 | 优点 | 不足 / 决策 |
| --- | --- | --- |
| 仅加 evidence 覆盖索引 | 修改范围小，已有单次改善证据 | 127.5MB pages 成本且仍全源扫描；作为 B 未能利用现有索引时的独立候选 |
| 应用缓存 + in-memory generation | 实现简单 | 绕过写入、重启、删除/更正不可证明；已有 bounded memo，不选作正确性边界 |
| 只做全局 durable generation | 无变化场景可快速验证 | daily append 仍全重算；只作为 C 的安全基础，后接 D |
| 事务日志 + 依赖后缀 + 共享 facts | 同时处理 unchanged、small change、重复存储 | 迁移和证明工作较多；推荐分步、小 PR、随时全量回退 |
| 全量并行计算/后台预计算 | 可能改善等待感或吞吐 | 放大内存、SQLite 写锁竞争；不解决扫描和写放大，暂不选 |
| 直接砍历史/证据或降低金额精度 | 可减少工作量 | 改变产品/金融语义，不接受 |

评审前待确认：目标 Mac 型号/存储与 cold 定义；10% 回归阈值；日志保留上限和压缩策略（按 bytes/revision，需保证消费者回退）；epoch reset 的 restore/import 钩子；驱动 BEGIN IMMEDIATE 的封装；所有 source 表的完整 writer 清单；K 的实际共享率与 episode evidence 保留成本；跨时区 calendar/tzdata 版本标识；account scope 闭包的实测价值。未确认时均用保守全量路径，不省略正确性检查。

## 12. 本次文档验证记录

本次仅核对源码、历史记录和现有测试入口，未运行新性能基准，没有新增 p95 或速度承诺。已核对 main 包含 PR #106/#107；仓库根 `AGENTS.md` 明确没有 `.agents/skills`，已阅读 engineering/local-workflow 指南。只修改本设计和导航链接，无生产代码、测试代码、schema 或生成物改动。

环境没有可用 Go 工具链：`go version` 返回 `Go: Unknown option: version`，`gofmt` 不存在；因此工程指南要求的 Go tests/vet/build 未运行，不能记为通过。文档的 17 个相对链接目标与 13 个命名测试入口均已检查存在，`git diff --check` 通过；原生 macOS、UI、备份恢复和性能验收均未运行，留待实现 PR。
