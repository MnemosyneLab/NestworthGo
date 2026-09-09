# Analytics Linux QA 问题复核与解决方案

日期：2026-09-09。测试基线：`4cee772`；本次代码复核：`e45311cf75d35e42acb196161d46ef30255b50fa`。

## 1. 结论与证据范围

**本轮证据不足以通过发布验收，但原报告对部分失败的定性需要修正。** 已确认的产品问题是 CO-03 股息贡献缺失（当前代码已修复）、长区间计算超预算，以及中文界面残留英文警告。主样本残差仍需逐日核账；日期输入未回写不能证明请求未截断；RC-07 的样本并非真正零收益。

本报告对照[测试计划](../../testing/nestworth-analytics-test-plan.md)、[原报告](REPORT.md)、[问题记录](findings.md)、[执行记录](STATUS.md)、日志、probe JSON 和关键截图，并静态核查相关代码。它汇总本轮已暴露的问题及未关闭的验收项，不宣称发现了产品中所有潜在缺陷。

本次没有运行 App、重建数据库、重跑自动化或修改产品代码。下文的测试 PASS 均指已有 Linux 记录；代码确认与桌面复测分开表述。测试基线至当前提交的 `frontend/`、`internal/` 产品差异只有 `activity_repository.go` 的 hydration 修复，所以日期截断和 race 跳过逻辑在被测基线也已存在。

## 2. 问题总表

| ID | 问题 | 复核状态 | 建议优先级 / 处置 |
|---|---|---|---|
| P-01 | Contribution 的 Dividend & Interest 无行，Categories 有金额 | 确认产品缺陷；已有修复和 Linux 复测 | P0 历史缺陷，保留回归门禁 |
| P-02 | cold-3y 约 5.09–5.20 秒，预算 <3 秒 | 确认预算失败；race 通过不能反证 | 发布性能门禁，先定位再优化 |
| P-03 | zh-CN / zh-TW 警告仍是英文 | 已记录的本地化缺陷 | 通常 P2；关键可信度信息无法理解时提高优先级 |
| P-04 | Calendar / Category 明细用 UUID 作标签；Residual 默认显示内部 key | 截图和静态代码确认；见第 7 节 | UUID 替代名称建议 P1；冗余诊断 key 建议 P2 |
| P-05 | Return % 图表纵轴显示原始小数 | 确认格式缺陷，未证明公式错误 | P2，补轴单位与精度 |
| P-06 | 不完整日的金额与收益率可用性说明不清楚 | 确认展示歧义；金额应否为 null 需按依赖核实 | P1，明确金额状态；不能按 ratedDays 一刀切 |
| P-07 | 合成现金分组显示未翻译的 `cash` | 简体 / 繁体截图和代码确认 | P2，结构化分组类型本地化 |
| U-01 | 主样本 Unexplained difference +A$1,428.80 | 现象确认，根因和最终样本状态未闭环 | 优先调查，不能先认定为金额算法错误 |
| U-02 | FL-22 / FL-23 输入框未截断 | 原 FAIL 证据不足；请求已有 clamp | 改为待请求级复测；可另提范围提示优化 |
| Q-01 | RC-07 未构造出真正零收益日 | 确认测试数据缺陷 | 修 seed，补 P0 桌面零值验收 |
| Q-02 | 缺 quote 测试通过直接改快照模拟 | 仅证明注入 partial 后的展示 | 补真实缺 quote / FX 链路 |
| Q-03 | 初期 0/38 coverage、空 Sheet | seed FX preference / 时间戳错误；已修 | 固定 seed 前置校验 |
| Q-04 | 错误 settings 导致一直 Loading | 环境已缓解；启动错误恢复仍待查 | 补错误可恢复性验收 |
| Q-05 | 完整 check 被 gofmt 阻断 | 确认门禁未完成 | clean checkout 重跑全量 check |
| Q-06 | Node 20 测试 worker 崩溃；工具链混用 | Node 22 下 Vitest 通过 | 固定可复现工具链 |
| Q-07 | seed、截图、probe 与二进制版本缺少统一对应 | 证据可重复性不足 | 固定时钟、manifest、阶段产物 |
| G-01 | 原生平台、数值 E2E、stale、错误态等覆盖不足 | 未测试 / 部分测试，不等于产品失败 | 按第 5 节补齐 |

用例的 P0 执行优先级不自动等于缺陷 P0。原报告给出的 P0/P1 标签应结合计划 §28 的实际影响重新评定。

## 3. 产品缺陷与待调查问题

### P-01：股息 Contribution 缺失，已有修复

**现象与证据：** 2026-08-28 记录 USD 12.50 股息；Categories 显示 A$18.83，Contribution 的 Dividend & Interest 原来无行。见 [修复前截图](screenshots/supplement/sup-co03-dividend.png)、[修复后截图](screenshots/supplement/sup-co03-dividend-after-fix.png)、[明细截图](screenshots/supplement/sup-co03-dividend-sheet-after-fix.png)及 [probe](logs/11-co03-div.txt)。probe 中精确金额为 AUD `18.8325`。

**根因：** `internal/infrastructure/sqlite/activity_repository.go:286` 的历史读取 `listActivitiesUntilQuery` 原来只调用 `attachActivityEffects`，未装载 `DividendDetail`。Categories 的 cash AssetBuckets 有金额，但 Contribution 需要完整活动明细来生成股息 ReturnComponents。

**现有方案：** 当前第 306 行改为 `hydrateActivities`，统一补齐 effects、trade、dividend 和 resulting 信息。不能在前端抄 Categories 总额补行，否则会掩盖范围和归属问题。

**关闭条件：** 已有记录称 sqlite tests 和重建后二进制桌面复测 PASS；本次确认补丁存在。后续补一个经过真实 SQLite 历史读取及应用投影的针对性回归，核对 Contribution、Categories、Sheet、History 的 `18.8325` / 显示 `18.83`；确认 Include/Exclude Cash 不使股息消失或重复、Portfolio/Account/Instrument 归属正确。完整 check 应在修复后的同一提交上重跑。不要把修复前的 341 个前端 PASS 当作修复后的整套验证。

### P-02：3Y / 500 components 冷计算超预算

**证据：** [application 日志](logs/04-go-application-wailsapi.log)、[engine 日志](logs/07-layer-b-engine.log)记录 `5.196028125s`、`5.092925095s`、`5.177705945s`、`5.190736845s`，均超过 3 秒，约超出 70%–73%。warm-memo 与 cold-month 的已有测试通过。

**修正原结论：** `internal/application/analysis_projections_test.go:668` 明确在 `analysisRaceDetector` 为 true 时 `t.Skip` cold-3y；cold-month 在 race 下也放宽预算。因此 race 包通过只能作为并发检查证据，不能说明 cold-3y 在 race 下更快，也不能据此定性为 box load / flaky。已有多次普通运行失败，性能门禁仍然失败。

**方案：** 在固定硬件、Go 版本和空闲负载下，以 `-count=1` 单独重跑预算测试，再用现有 `BenchmarkAnalysisColdComputeUpperBound` 采集 CPU / allocation profile。该预算计时的是已准备输入上的 `ComputeAnalysis`，不包含 fixture 构建；应先找计算和分配热点，不能无证据归咎 SQLite I/O。根据 profile 消除重复解析、重复扫描或不必要分配；保留精确 decimal、归因公式和 memo 失效语义。不要直接放宽 3 秒或跳过门禁作为修复。

**验收：** 原预算用例通过，warm <100ms、month <300ms 无回退；在 macOS 原生 production App 上另测 3Y/All 的 loading、Tab/filter 响应与 snapshot rebuild。算法超时不直接证明窗口冻结，窗口响应也不替代算法预算。

### P-03：中文界面仍显示英文可信度警告

**证据：** [简体 Return](screenshots/i18n/i18n-zh-CN-return.png)、[简体 Asset](screenshots/i18n/i18n-zh-CN-asset.png)、[繁体 Return](screenshots/i18n/i18n-zh-TW-return.png)，原报告与 STATUS 均记录 warning/banner 英文残留。现有截图范围不足以宣称六 Tab 三语言已全覆盖。

**方案：** 先列出实际可见的英文及对应控件，再定位是缺少翻译 key、英文 fallback 还是直显后端 message。对已知 issue code / 参数使用本地化模板；保留未知错误的可读 fallback 和诊断详情，避免依赖英文句子匹配。覆盖 partial、missing price、missing FX、forced-base、residual、coverage 和 Sheet 说明。

截图中可具体追踪的残留是 `one or more return days are incomplete`：简体和繁体 Return 页周围已有翻译，但这条详情仍为英文。

**验收：** 三语言浏览六 Tab、四类 Sheet、空态与警告；无 raw key，关键警告可理解，中文长文案不溢出。金融 warning 的信息和金额不能因翻译而丢失。

### U-01：主样本 A$1,428.80 残差尚未解释

**证据：** [Drivers residual](screenshots/drivers/dr-residual.png)、[补充 residual Sheet](screenshots/supplement/mf-residual-sheet.png)及 STATUS。记录的期初 A$45,061.53、期末 A$49,899.72、变动 A$4,838.19 同时伴随 partial/reconciliation warning。

**证据边界：** residual 作为独立项展示本来就是正确的可信度行为；其存在不证明 financial identity 被破坏，也不能由其他 golden PASS 推断主样本正确。seed 做过 origin/entity 回填及快照标记修改，是候选原因，尚无原数据库逐日核账证明。后期 CO-03 改为完整 hydration，可能改变历史解释，需在最终版本重新抓取同一查询，不能把各阶段数值直接混用。

**方案：** 固定原样本或其可重复生成版本，导出同一 query、revision 下逐日逐 component 的 beginning、ending、全部 signed buckets、residual、completeness、activity、价格与 FX 来源。用独立核账核对 `ending − beginning = 已解释 buckets 之和 + residual`，再核对整期 summary / waterfall / detail。若正常样本输入不一致，修 fixture 后从 origin 重建；若数据完整仍有误差，缩成一个独立最小回归再修相应归因路径。

**验收：** 正常完整样本残差为零，或每项有清楚且可接受的原因；不能把差额塞进 FX、Adjustments 或前端抹零。故意损坏样本仍应暴露 residual、partial 和正确 History 维度。

**不要混淆另一项已通过的实验：** [probe-residual.json](seed/probe-residual.json)中 8 月 12 日为 +100、13 日为 −100，证明跨日承接异常可追踪。两项在整期可抵消，`waterfall=<nil>` 不等于丢失日级 residual；该实验不解释主样本 +1,428.80，也不能替代主样本核账。

### U-02：FL-22 / FL-23 原失败判定需撤回并补请求级证据

**计划要求：** §9.4 明确写的是“request clamp 到 last closed day / origin”，没有要求输入框自动回写。

**现有实现：** `frontend/src/features/insights/analysisRequest.ts:94` 的 `clampAnalyzableRange` 上截 `To`、下截 `From`，origin 日期按 origin timezone 转换。`analysisProjectionContext.ts:16` 先算 effectiveRange，再构造请求，并在范围无交集时禁用请求。`AnalysisFilterBar.tsx:98`、`:102` 仅限制 From/To 的相对顺序，所以输入保留原选择与请求被截断可以同时成立。这些代码在测试基线已存在。

**处理：** [FL-22](screenshots/filters/fl22-to-clamp.png)、[FL-23](screenshots/filters/fl23-from-origin.png)只能支持“输入框未回写”。将算法/请求 FAIL 改为“现有失败证据不足、待复测”，不能直接改成桌面 PASS。若用户无法理解显示日期与实际计算区间的差别，可另加“实际分析区间”提示，或由产品明确是否需要输入限制；不要为满足错误断言强行重写 session。

**验收：** 固定时钟和 origin timezone，记录六 Tab 的 Wails 请求及响应有效范围；覆盖 today、future、pre-origin、完全不相交区间、日期边界及无 origin。确认无未来/起点前数据参与、空交集不发非法请求；保持 Calendar cursor 与全局筛选区间独立。

## 4. 测试数据、环境与报告质量问题

### Q-01：RC-07 零收益样本无效

seed 把 day 35/36 的 AAPL 都设成 207，但 `cmd/analytics-qa-seed/main.go:215` 仍按 `1.50 + d × 0.0002` 递增 USD/AUD。Portfolio Base AUD 持有美元资产和现金，即使股价不变，仍可产生 FX return。截图中的 +0.01% / A$4.27 不能据此定性为舍入或收益算法错误；精确 4.27 的来源仍需 component probe 核对。

**解决与验收：** 单独建立完整、正 capital、无活动、价格及所有 FX 均不变的固定两日样本；先独立确认 return amount = 0、rate = 0、status = ok，再验证 Calendar 显示真实 0 / 0%，与 partial 的 `—` 明确区分。保留当前 FX 变动样本作为“平股价但 Base 收益非零”的测试。

### Q-02：缺行情验收实际是人工 partial 注入

`cmd/analytics-qa-seed/main.go:396` 说明缺一天报价会沿用 prior quote，重建仍完整；随后 SQL 手工设置 snapshot items 的 `complete=0`、`missing_reason`，并修改 snapshot 的 complete/missing_count。且这一天同时跳过 AAPL 和 FX，无法隔离两种依赖。

**解决与验收：** 将既有 PASS 命名为“注入不完整快照后的展示 PASS”。另建独立缺 price、缺 FX 样本，使按真实来源偏好及 as-of 规则确实无可用估值；通过正常重建生成不完整状态，逐层核对 snapshot→engine→DTO→Calendar/Trend/Contribution/Sheet。缺 FX 时分别验证 Base 与 Native 的依赖差异。缺某天新报价不必然意味着缺可用报价，不应改坏 as-of 选择规则。

### Q-03：初期 seed 导致空结果，已缓解

初期未正确建立历史起点、手工 FX 未设置 manual preference、holding/instrument 创建时间晚于回放区间，导致稀疏/缺失 holding 快照及 Contribution 0/38。STATUS 记录修正后 45/45 快照完整、37/38 rated；后续又注入缺价及新增交易，最终 probe 为 34/38。不同数字来自不同阶段，不能互相替代。

**解决与验收：** 在启动桌面前校验 origin components、source preferences、实体生效时间、快照 revision、每日期待 completeness 与 ratedDays；普通完整样本、故意缺数据样本、损坏快照样本分开。所有 seed PASS 必须是实际断言，不能只写“计划构造了某场景”。

### Q-04：无效 settings 导致无限 Loading

findings 记录错误 settings keys 和 `could not load saved settings` / `could not persist window size`，换合法配置后启动成功。配置错误是明确触发条件，但记录同时做了 seed/WebKit/relaunch 调整，缺少单变量复现，不能直接认定产品恢复机制已修好。

**解决与验收：** 在隔离环境分别注入 malformed JSON、非法枚举/字段、读取或持久化失败，确认显示可理解错误与恢复入口，Retry 或换回合法配置后能恢复。若设置失败仍无限 Loading，再修 bootstrap/settings 的错误分支。保留坏配置用于诊断，不以静默覆盖作为默认修复。

### Q-05：完整自动化门禁未跑完

[03-check.log](logs/03-check.log)最后停在 `test -z "$(gofmt -l cmd internal)"`，记录的 offender 为未跟踪 `cmd/align-smoke-check/main.go`。这不是已证明的产品格式问题，但后续 focused Go tests 与 Vitest 不能覆盖完整 `go test ./...`、vet、lint、check 尾部步骤。frontend build 已执行 tsc，亦不能称完全没有类型检查证据。

**解决与验收：** 在无本地临时工具干扰的 clean checkout，固定工具链执行完整 `wails3 task check`；保留每步 exit code。加入 bindings 一致性检查，最终工作树预期干净。全量结果应绑定修复后提交。

### Q-06：测试工具链不一致

记录称 Node 20.19.2 下出现 44 个 worker errors，换 Node 22.19.0 后 44 files / 341 tests PASS；不能把前者算成 44 个产品测试失败，也不能仅据此宣称所有 Node ≥22 都兼容。日志使用 bun 和 Wails CLI beta.12，而仓库 `frontend/package.json` 声明 pnpm，Go Wails 与 `@wailsio/runtime` 为 beta.16。

**解决与验收：** 固定并记录验证过的 Node、包管理器、锁文件、Go、CLI、Go module 和 JS runtime 版本；优先复用仓库声明流程，确认生成器与运行时匹配。差异本身不是已证实运行故障，但应以一致工具链补跑。apt fuse.conf 交互阻塞已通过非交互安装缓解；GTK deprecated warnings 与 bundle size warning 仅作构建提示，现有证据不支持列为产品缺陷。

### Q-07：复现工具和产物缺少稳定身份

seed 使用 `time.Now()-45d`，而报告/probe 使用固定历史区间，换日运行不能保证复现原数据；JSON commit 字段硬编码 `4cee772`，不能证明实际二进制版本；输出目录默认硬编码 `/workspace/...`。此外 seed 会无条件 `os.Remove` 指定 DB 及 WAL/SHM，后续复测必须只用专属临时数据库。

**解决与验收：** seed 支持固定 anchor time、显式输出目录和机器可校验的场景版本；拒绝覆盖已有 DB，显式 reset 仅作用于 QA 临时路径。记录实际 commit/dirty diff、DB fixture 摘要、query、snapshot revision、二进制 hash、工具链、locale、viewport、截图时间。分别保存初始、扩展、修复后 manifest。当前原始 DB 和 binary 未随仓库提交，旧 hash/路径不能代替可用原件。

根目录与 `report/` 两份 REPORT 当前相同，但容易分别更新；应确定单一主报告，另一份只保留指向链接。保留原始日志，使用本报告纠正结论，避免覆盖历史事实。

## 5. 计划覆盖缺口及补测方案

以下均为“现有归档不能证明完成”，不据此新增产品 bug。细粒度计划项没有逐项记录时不从相近截图推断 PASS。

| 计划范围 | 已有证据及不足 | 解决 / 关闭标准 |
|---|---|---|
| §5 平台矩阵 | Linux amd64、AUD、SGT；不是主目标 macOS Apple Silicon | macOS production `.app` smoke；CNY/USD 各一次，UTC/SGT 边界；DST 自动化记录单列 |
| §9 Scope / More Filters | 选择器与部分 Currency/Asset Class/clear/tab persist 已测；非全部维度交集 | 补 FL-06–10、15、20、24–26；Account/Member/Instrument moreFilters 与 scope 的交集、Reset、跨页保持 |
| FL-12、§18 Native | 单币种 Native 仍 PARTIAL；混币 forced-base 已截图 | 单币种 Instrument Native 金额与 FX=0；同一 stored Native 下 Return allowed、Asset forced-base 的完整跨页链路 |
| FL-18/19 | probe 已证 cash 改变 Dietz capital；不替代完整 UI 数值核对 | 同一 salary 独立预期：Income 只记一次，Include/Exclude 的 capital 和 Return 差异贯穿 DTO/页面/Sheet |
| §7、§16 金融 E2E | Fixture A–D 在内存/单元层通过；不足以证明完整存储到桌面链路 | 补 FC-01 transfer、FC-02 scope buy、FC-03 当日买卖归零、FC-04 Price/FX、FC-05 interest vs salary、FC-06 dividend/tax/fee、FC-07 loan 的桌面精确核对 |
| §10–15 六 Tab 深层行为 | 主要打开/切换/Sheet 有记录，部分粗粒度标签缺少完整计划映射 | 逐项登记 RC/RT/CO/DR/AT/CAT 原 ID；补 year/month 聚合、linked rate、粒度/metric、排序和不可用限制等未证明条目 |
| §17 Trust | 注入 partial、residual 有证据；真实 missing FX 未独立验收 | Q-02 自然缺价链路；30/31 rated 与多数无 rate；验证全 projection 一致性及 reason/coverage |
| §19 Navigation | smoke 可跳转；不代表每个 payload 字段正确 | 捕获 date/account/instrument/kinds/moreFilters；尤其 Contribution/Category/Residual→History 与 Driver→Return type；确认进入 History 后仍可改筛选 |
| §20 Error/Loading/Empty | Scope Required 已测，其他空态/Retry 缺证据 | 六 Tab 抽测无 origin、无投资、历史不足、无贡献/分类/变动、请求失败恢复；chrome 保持，无布局塌陷 |
| §21 Invalidation（明确 P0） | `SUP-STALE` 仅导航；quote mutation 被跳过 | 保持同 query，按正常流程更正 Activity；再改 Quote/FX、account/holding、rebuild；页面及已开 Sheet 自动更新，无旧 memo |
| §22 i18n | 仅部分页三语言截图 | 完成 P-03 全部 Tab/Sheet/状态核查 |
| §23 Keyboard / a11y | 没有最低 release gate 的完整记录 | 键盘 Tabs、label、day focus、Escape/Sheet focus、图表 summary/table、非颜色状态、无需 hover 的 warning |
| §24 Visual | 1280 有记录；标为 1440 的截图实际仍 1280 | 真正 1440×900、较窄窗口；记录实际像素；检查 wrapping、year grid、长名称/金额/负数、tooltip、waterfall labels |
| §25 Performance | 引擎 warm/month PASS，cold-3y FAIL；未证明原生响应 | 关闭 P-02；补 PF-01–04：长区间响应、连续筛选内存/stale、年历请求节奏、重建不冻结 |
| §26 Regression | 部分 API tests 与 History 导航，不等于完整回归 | Investments Holding/Account gains；归档/删除实体；Market Data 刷新；backup restore 后重建且无旧 memo |

建议建立逐用例矩阵，列出 `计划 ID / 状态 / fixture 版本 / 实际 query / 预期值 / 实际值 / 日志或截图 / commit / 未完成原因`。未测使用 NOT RUN，人工注入使用 INJECTED，自动化与原生桌面分别记账。

六 Tab 细项中，归档尚未提供明确逐项证据的包括 RC-09、RC-12–13、RC-26–27、RT-04–07、CO-04–05、CAT-06、CAT-11–12，以及 Contribution 的完整 group-by/sorting 组合、Drivers 的 liability/adjustments/zero bars 和 Asset Trend 的期末值/流量求和/收益率几何连接。补测时按原计划逐项确认，不把本轮使用的 `DR-SUM`、`RT-SRC` 等简写视为原计划整节已通过。

## 6. 建议执行顺序与发布关闭条件

1. **纠正证据和固定复现条件：** 处理 U-02 的错误 FAIL、P-02 的 race 误解、Q-01/Q-02 的样本边界，固定 seed 与版本 manifest。
2. **优先关闭数据可信度：** U-01 主样本逐日核账、P-01 真实 SQLite 回归、真实缺价/缺 FX、P0 Activity stale；确认是否还存在实际金融缺陷。
3. **处理明确产品问题：** profile 后优化 cold-3y，补中文警告，复现并处理 settings 错误恢复。
4. **完成修复后全量自动化与目标平台验收：** clean checkout check、macOS production、Native、金融 E2E、键盘、1440/窄窗、回归与性能响应。

最终依据计划 §29 签核：完整 check 全绿；正常样本金额、收益率和归因可独立核对；可信度状态和残差可追踪；修改数据后结果更新；主平台/布局/性能门禁完成；无 Blocker/P0，P1 已修复或有明确接受理由。若有门禁需接受偏离，应逐项记录偏离范围与依据，不能以“多数功能可打开”替代。

本次复核新增的文档是本文件；原测试记录和产品代码保持原状。

## 7. 截图补充发现复核（2026-09-09）

根据用户提供的 Luna 逐图检查结果，本轮重新查看了 Calendar Day Sheet、Category Sheet、Residual Sheet、Return % 趋势、缺价日及简繁中文 Contribution 共 7 张关键截图，并核对当前 `2675aef` 的相关实现。没有重新检查全部 99 张截图，也没有启动 App 或运行测试。本节补充首次报告未单列的问题；不改变之前的历史测试基线。

### P-04：用户明细直接显示 UUID / component key

**确认情况：** [Calendar Day Sheet](screenshots/calendar/rc10-day-sheet.png) 的 Top contributors 用完整 UUID 代替标的名称；[Category Sheet](screenshots/categories/cat07-sheet.png) 的 Breakdown 用被截断的 UUID 作标签。后者标题虽有 `AUD Cash`，子行身份仍不可读。建议 P1，因为用户难以识别具体归属。

[Residual Sheet](screenshots/drivers/dr-residual.png)也默认显示内部 component key，但同一卡片已经有 `US Brokerage · Apple Inc`、日期，以及下面的账户/标的名称。因此该处是冗余诊断信息进入日常界面，建议 P2；不应声称它完全没有友好名称。这些截图证明的是可读性问题，不构成已证实的安全信息泄露。

**原因：** `internal/application/analysis_return_projections.go:536`、`:629` 把 contributor `Label` 设为 key，`ReturnCalendarTab.tsx:170` 直接渲染 `item.label`；`internal/application/analysis_projections.go:654` 把 Category child `Label` 设为 key，`CategoriesTab.tsx:37` 直接渲染 `child.label`。Category 顶层行有名称映射，子行缺少同样的处理，解释了标题正常、Breakdown 不正常的现象。

**解决方案：** 为投影明细保留独立的稳定 key、实体维度与友好名称，通过 DTO 批量补齐名称，或复用已有前端实体名称映射；不得通过解析拼接 component key 来推导用户名称。历史归档/已删除实体需有可理解的 fallback。Residual 默认保留账户、标的、币种和日期，内部 key 放入可展开、可复制的诊断详情。

**验收：** Calendar contributors、Category children、Residual detail 默认可辨识实体；名称缺失、同名、长名称、归档实体有合理显示；金额、排序、导航 ID 和 History 筛选不变。

### P-05：Return % 纵轴缺少百分比格式

**确认情况：** [Return % 截图](screenshots/trend/rt02-linked-return-pct.png)的纵轴确实为 `0.00005`、`0.0001`、`0.00035` 等原始比率，与标题 Return % 不一致。`ReturnTrendTab.tsx:116` 传入 `rateText`，但 `TrendChart.tsx:120` 的 `yAxis.axisLabel` 只有颜色；formatter 只用于 tooltip/table。

**边界：** 摘要 `+1.08%` 是整期结果，不能直接要求它等于某个日级图表点或最大刻度。本轮确认单位表达错误，未证明收益公式或曲线数值错误。

**解决方案：** 给共享 TrendChart 增加明确的轴格式化能力，百分比模式将比率转换为百分数并显示 `%`；例如 `0.0003 → 0.03%`。保留原始 series/DTO 值，仅格式化展示，避免重复乘 100。轴精度应能区分相邻小刻度，不能简单套两位小数导致多个刻度同名；同时保留金额图的既有格式。

**验收：** 正数、负数、零、极小比率和较大收益率的轴/tooltip/table 单位一致；null 仍保留断点。检查其他复用 TrendChart 的 rate 图，确认金额图没有回退。建议 P2。

### P-06：不完整日仍显示裸金额零，金额状态表达不足

**确认情况：** [缺价日 Sheet](screenshots/supplement/sup-tr-missing-quote-aug17.png)同时显示醒目的 `A$0.00`、`— ◇ 0/1` 和 `daily return is incomplete`；日历也显示 `—◇` 与零金额。确有 warning，因此不应描述成完全没有提示，但用户无法从金额旁区分“完整金额为零”“已知部分合计为零”与“金额未知”。建议 P1 处理这一可信度表达问题。

**代码及契约边界：** `ReturnCalendarTab.tsx:22` 对非 null 金额直接格式化，DayCell/DayDetails 没有独立的金额完整性说明。`analysis_return.go:69–100` 将已知 component amount 求和；`:114` 只有 complete 且 capital 为正才生成 rate。因此 `ratedDays=0` **不等于**金额未知：收益金额可知而 Dietz 分母不适用，或只知部分金额，都可能没有收益率。不能采用“无 rated day 一律 nil”的修复，否则会隐藏有效金额。当前缺价截图来自 Q-02 的人工标记样本，尚不足以判定自然缺价时后端错误填零。

**解决方案：** 由后端依据金额依赖明确区分：完整已知金额、仅已知部分的合计、没有可用金额。完全未知返回 null；部分已知保留数值但标为“已知部分”，零也不例外；仅 rate 不可用时保留正确金额并解释 rate 缺失原因。若现有 DTO 的 status/coverage 无法表达这些差别，再补充金额状态字段，前端不以 ratedDays 猜测金额有效性。

`◇` 并非完全没有说明：当前 DayCell 有本地化 `title` 提示 partial/coverage，Sheet 也有 warning。但常驻图例和金额状态解释不足，且 `title` 依赖 hover；应增加本地化、键盘可访问且不依赖 hover 的说明。

**验收：** 分别验证完整真零、完整非零但分母不适用、部分已知零、部分已知非零、全部未知五类；Calendar/Sheet/summary 保持一致，未知不能假零，已知金额不能因 rate 缺失被抹掉。另以真实缺价/缺 FX 链路复测，不能仅复用注入快照。

### P-07：合成现金分组 `cash` 未本地化

**确认情况：** [简体截图](screenshots/i18n/i18n-zh-CN-return.png)与[繁体截图](screenshots/i18n/i18n-zh-TW-return.png)均显示小写 `cash`。它是分组标识，不是用户自定义名称，与 P-03 的 warning 翻译遗漏属于不同显示路径。

**原因与方案：** `analysis_return.go:491–495` 在按 instrument 分组且无 instrument 时返回 `cash`，当前 Contribution 标签路径没有转换为本地化合成分组名称。保留稳定 key，按分组类型映射为 `Cash / 现金 / 現金`；Calendar contributors 等同类入口一起处理。不要全局替换文本 `cash`，以免误改用户命名账户或 instrument 名称。

**验收：** 简繁中文和英文的合成现金行及其明细标题正确；切换语言即时更新；账户真实名称、分组/排序/导航 key 不变。建议 P2，可与 P-03 合并实施但分别验收。

**其他结论：** `co02-realized.png` 空态与补充截图有 realized gain，仍按 Q-07 的不同 seed/执行阶段解释，不另立产品缺陷。本次补充没有证明新的金额计算公式错误。
