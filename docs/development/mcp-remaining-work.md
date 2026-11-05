# MCP / Skill 剩余工作:并行拆分方案

基线:PR #236(`a076f094`)之后。目标是对照最初评审,把没做完的事拆成可以并行的工作流。
每个工作流一个独立 worktree、一个分支、一个 PR,各自带测试。

## 0. 先看这里:共享文件冲突

下面这些文件几乎每个改动工具面的工作流都会碰到。冲突是机械性的,但必须有人统一处理,否则会互相覆盖。

| 共享文件 | 为什么冲突 | 处理方式 |
| --- | --- | --- |
| `internal/mcpserver/testdata/discovery-contract.json` | 每个增删或改动 schema 的工具都会变 | 不手工合并。合并后在最新 main 上重新生成,并核对 diff 只含预期工具(生成方法见第 5 节) |
| `internal/mcpserver/discovery_budget_test.go` | 体积变化后预算要下调 | 每个 PR 只放宽或收紧自己的部分,合并后由协调者统一收紧 |
| 工具总数断言 | `in_process_test.go`、`repair_progress_test.go`、`service_test.go`、`internal/builtinai/session_test.go` | 每个 PR 只改自己的增量,合并后重新核对 |
| `internal/mcpserver/task_tool_discovery.go` | Wave 2 G 已删除。新工具不再归入发现分组 | 不要恢复这个文件 |
| `skills/nestworth/VERSION` 与 `docs/user/nestworth-skill.md` | 规则要求改 skill 必须升版本;并行 PR 会撞同一个号 | 各 PR 先不抢号;由协调者按合并顺序依次升 PATCH,不并行升 |
| `skills/nestworth/references/connection.md` | 工具分组表列出每个工具名 | 各自改自己那几行 |
| `docs/development/mcp.md` | 变更记录式堆叠,所有人都往里加 | 各 PR 只加自己的小节;结构重写放到最后(工作流 F) |

## 1. 工作流总览

```
Wave 1(立即并行,互不依赖)
  A  验证与基线(不改代码)
  B  目录类工具合并
  C1 历史概览 / 市场数据合并
  C2 下钻工具折叠进 get_read_item
  E1 nullable 数组 schema 修复
  E2 大输入 schema 瘦身
  F3 前端 i18n 残留清理

Wave 2(要先有决定或数据)
  B2 账本提交的 operationId 变为可选          <- 需要决定
  C3 净值/估值入口收敛                         <- 依赖 A 的数据和 C1 的结论
  D  investment review 默认暴露策略            <- 需要产品决定
  E3 PATCH 语义统一(破坏性)                   <- 需要决定
  G  toolGroups 机制与双份响应                 <- 依赖 A 的真实客户端数据

Wave 3(最后,依赖前面都合并)
  F1 Skill 缩短(products.md、financial-context.md)
  F2 docs/development/mcp.md 重写为当前合约
```

规模估计:S = 半天以内,M = 1-2 天,L = 3 天以上。

## 2. Wave 1:立即并行

### A. 验证与基线(不改代码)

**目标**:补上目前没有证据的结论,并给 Wave 2 的决定提供数据。

- **A1 CI**:检查 #236 最新 head 的 CI,按仓库规则区分排队、构建、缓存、测试耗时,把 head SHA 和运行链接交给协调者。
- **A2 真实客户端测量**:用至少一个真实 MCP 客户端,测发现开销、典型任务的调用次数和 token。典型任务:查净值、记一笔买入、对账、导入基金净值。记录客户端版本、配置、是否转发 `structuredContent` 与文本两份。
- **A3 原生 macOS 验证**:按 `local-workflow.md` 的隔离 QA 步骤,用 `com.nestworth.dev` 身份和显式隔离的数据库/设置目录,验证 MCP 连接与一次写入流程。不碰真实账本。

**产出**:一份测量记录,与现有“头less 配对测量”分开标注。
**规模**:A1 S,A2 M,A3 M。
**输出被谁使用**:C3、G、B2 的决定。

### B. 目录类工具合并

**目标**:成员、机构、分组各有 create/update/archive,共 9 个工具;评审建议按 `kind` 合并,并把 archive 并入 update。

**范围**:
- `tools.go` 目录部分、`directory_edit.go`、`update_conventions_test.go`。
- 候选形态(先评估再选):
  1. `create_directory_entry/update_directory_entry({kind, ...})`。
  2. 只把 `archive_*` 并入 `update_*`(`archived?: boolean`),保留三种实体名。

  形态 2 风险小很多,建议先做它;形态 1 要重新处理 `institutionType` 只在机构创建时出现的问题。
- 保持:`directory_write` 权限、归档实体的规则、“至少保留一名活跃成员”的约束、一次调用一个回执。

**不碰**:账户、投资品(它们有各自的 patch 约定)。
**测试**:沿用 `directory_edit_test.go` 的思路,加归档与“最后一名成员不能归档”的用例。
**破坏性**:是(删除 3 个 `archive_*` 或 9 个工具)。
**规模**:M。

### C1. 历史概览与市场数据合并

**目标**:两组读取工具各合并为一个。

- `get_historical_overview` + `get_historical_overview_series` → 一个工具,入参 `{date, compareTo}` 或 `{from, to}`。
- `get_market_data` + `get_market_data_batch` → 一个工具(要先读两者的差异:批量的预算、分区、证据字段)。

**范围**:`feature_tools.go`、`market_data_tools.go`、`market_data_batch_tools.go`、`output_schemas.go` 的归类函数 `readOutputKind`、相关预算测试。
**注意**:一个工具两种结果形状,要用 `oneOf` 说明或拆成明确的模式字段;`oneOf` 会增加发现体积,需要实测。
**破坏性**:是。**规模**:M。

### C2. 下钻工具折叠

**目标**:`get_contribution_item`、`get_return_day`、`get_asset_driver_detail` 是父报告的下钻,折叠为 `get_read_item` 的 section。

**前提**:父捕获(`list_contributions`、`analyze_period`)必须冻结下钻详情。先测内存和 TTL 成本再决定;设计文档第 4 节要求“先测量”。
**范围**:`attribution_tools.go`、`bounded_read.go`。
**与 C1 的关系**:都改 `output_schemas.go` 的 `readOutputKind` 成员列表,冲突按行合并。
**破坏性**:是。**规模**:L(捕获成本是风险点)。若测量显示成本过高,可以只交付测量结论并关闭该项。

**状态**: CLOSED-as-measure-only。测量记录在 [mcp-wave2-c2-measure.md](mcp-wave2-c2-measure.md)。合成账本上,把每个 return day 冻进 `analyze_period` 后,14 年区间的捕获是 4,473,188 字节,超过 4 MiB 上限。折叠没有实现。

### E1. nullable 数组 schema 修复

**目标**:`type:["null","array"]` 触发 Inspector 告警,现在靠文档解释。改成客户端友好的形式(例如省略可选字段,或 `anyOf`),并删除 `docs/development/mcp.md` 里的解释段落。

**范围**:生成输入 schema 的位置(`jsonschema` 标签处理、`UpdateAccountInput` 一类的指针切片字段)。先列出受影响的工具,再逐个核对“省略”与“null”的语义是否不同(尤其是 `update_account` 的 `Set` 标志)。
**测试**:快照测试扩展,断言发现的 schema 中不再出现 `"type":["null",...]`。
**破坏性**:schema 变化会改合约快照,需要说明。**规模**:S-M。

### E2. 大输入 schema 瘦身

**目标**:`preview_product_operation`(约 7.6 KB)、`preview_change`(约 3.2 KB)是单个工具里最大的 schema。

**手段**(逐项量化后再选):按 `kind` 的 `oneOf` 分支去掉重复的字段描述、共用子 schema 引用、把说明文字从 schema 挪到描述里一次。
**限制**:不能弱化输入校验;必须保留每个 kind 的必填字段。
**测试**:现有 skill 示例的 schema 一致性测试(`skill_schema_consistency_test.go`)必须不变地通过。
**规模**:M。

### F3. 前端 i18n 残留清理

`frontend/src/i18n/assistant.ts` 里还有已删除工具 `get_financial_context_page`、`get_financial_comparison_page` 的显示名(多语言)。同时核对 `ConversationTimeline.tsx` 等处是否还有已删除名字。
**规模**:S。需要前端测试通过;不涉及原生 UI 验证。

## 3. Wave 2:需要决定或数据

### B2. 账本提交的 `operationId` 变为可选

**问题**:对 `commit_plan`,`planId` 已经是原子幂等键(换 operationId 重复提交也返回原结果),`operationId` 是冗余输入,Agent 还要自己生成 UUID。

**方案**:`operationId` 可选。缺省时服务端从 `planId` 派生确定性的 operation ID(例如对 planId 做 UUID v5)。
**必须保持**:
- 持久回执、`get_operation` 恢复、未知结果重试的现有语义;已有显式 `operationId` 的调用行为不变。
- liquidity 类“不重放 pending/unknown”的规则。
- 用相同 `planId` 但不同显式 `operationId` 仍返回 `stale_preview`。
- 派生 ID 与显式 ID 冲突时的行为要明确并有测试。

**决定**(Walt):只对 `commit_plan` 放宽。缺省或空白 `operationId` 派生为 UUID v5:命名空间 `f1f65af0-bbb8-5d57-af9c-b624de715dcb`(URL 命名空间下名称 `nestworth:mcp:commit_plan:operation` 的 v5),名字是规范化小写 `planId`。回执哈希仍覆盖提交的 `input` 文本。派生 ID 若已属于另一工具或另一输入,返回 `conflict` 且不落账、不覆盖原回执;同一 ID 与同一输入则重放该回执。其它写工具的 `operationId` 仍必填。
**破坏性**:否(放宽必填),但会改合约快照。**规模**:M。风险高(最敏感的写路径)。

### C3. 净值/估值入口收敛

**问题**:“净值是多少”有 5 个入口。
**先做**:比较 `get_account_snapshot` 与 `get_account_valuations` 是否是同一捕获的单账户投影(设计文档里的开放问题);比较结果决定是合并还是只在描述里写清区别。已决定:是同一捕获的单账户投影,但空结果语义不同,本轮不合并,只改描述。见 [mcp-wave2-c3-nav-entries.md](mcp-wave2-c3-nav-entries.md)。
**依赖**:A2 的数据(Agent 实际选哪个工具)、C1 的结论。
**范围**:先出结论文档,再决定是否改工具。语义边界(终点变动不是收益率)不能因合并而模糊。
**规模**:S(结论)到 L(实现)。

### D. investment review 默认暴露策略

**问题**:16 个 `investment_review` 管理类工具默认暴露,更适合在 GUI 里做。
**选项**:
1. 加一个能力开关,默认只暴露只读工具;
2. 保持现状,仅改善分组;
3. 把写入类(创建/更新/归档规则、确认告警、屏蔽)放到新的权限模式。

**需要产品决定**:选哪种。涉及设置项和权限模型,不属于纯 MCP 改动。本轮不做,见 [mcp-wave2-deferred.md](mcp-wave2-deferred.md)。
**注意**:权限模式变化会触及 `Mode` 与外部授权流程,需单独评审。
**规模**:M-L。

### E3. PATCH 语义统一

**目标**:所有 patch 工具统一为“省略 = 保留,`null` = 清除”,取代 `<field>Set`、`replace` 开关等三种风格。
**风险**:破坏性,且 `update_account` 的“空字符串与 null”区别要重新定义;必须有迁移说明和合约快照评审。
**依赖**:E1(nullable schema 的形式要先定)。
**需要的决定**:是否值得做。审计已经用测试固定了现状,统一的收益是减少 Agent 误操作,代价是又一次破坏性变更。本轮不做,见 [mcp-wave2-deferred.md](mcp-wave2-deferred.md)。
**规模**:L。

### G. `toolGroups` 机制与双份响应

- **`toolGroups`**:已删除。见 [mcp-wave2-g-toolgroups.md](mcp-wave2-g-toolgroups.md)。
- **`structuredContent` 与文本两份**:A2 没有模型宿主转发证据,两份都保留。

**规模**:S-M。**完全取决于 A2**。

## 4. Wave 3:最后做

### F1. Skill 缩短

`products.md`(401 行)、`financial-context.md`(398 行)、`paging-and-batches.md`(280 行)仍长于约 150 行的目标。
- 按任务拆分,每个文件只放一类任务的步骤和失败恢复。
- 必须在 B、C、E 合并之后做,否则会反复冲突。
- 保持:示例块的 schema 校验、`SKILL.md` 不超过 900 词、每个注册工具名在某个 reference 里出现。
- 升 skill 版本,按第 0 节的规则。

### F2. `docs/development/mcp.md` 重写

现在 1288 行,是按时间堆叠的变更记录(Stage、extension、follow-up)。
- 改成“当前合约”:权限、工具清单、读写流程、恢复、预算,历史移到单独的变更记录文件。
- 在其他工作流全部合并后做,避免改一半又被覆盖。

## 5. 每个工作流都要遵守的流程

1. **隔离**:每个工作流一个独立 worktree 和分支,互不复用。开始前记录分支、SHA 和未提交改动。
2. **先量后改**:涉及体积的改动,先用 `TestDiscoveryBudget` 记录改动前的字节数,改完再记录;只报告实测数,不报估算。
3. **合约快照**:`testdata/discovery-contract.json` 没有自动再生成开关。做法是写一个临时测试,按 `discoveryContractHashes` 生成文件,运行后删除临时测试,并核对 diff 只含预期工具。
4. **断言同步**:工具数量断言、`internal/builtinai/session.go` 的白名单(只读白名单,新增读工具要在这里评估)。`task_tool_discovery.go` 已删除,不要再给新工具补发现分组。
5. **JSON**:仓库内的 JSON 编解码只用 `internal/jsoncodec`。
6. **测试范围**:先跑受影响的包(`internal/mcpserver`、`internal/application`、`internal/builtinai`),需要时再扩大;不重复跑全量、race 或 benchmark。性能证据放在 `perf`。
7. **Skill**:改 `skills/nestworth/**` 的 PR 要说明需要升版本,由协调者按合并顺序统一落版本号;`tools/check-nestworth-skill.py` 与 `tools/tests/test_nestworth_skill*.py` 必须通过。
8. **PR 描述**:范围、环境、在确切 head 上通过的检查、未验证项、性能影响(改动前后字节)。破坏性变更要单独列出被删或改名的工具。
9. **提交信息**:Conventional Commits;破坏性变更用 `!` 或 `BREAKING CHANGE` 页脚。
10. **合并**:提交、推送和创建 PR 不等于授权合并。CI 监控由协调者负责,交接时只给 head SHA 和运行链接。

## 6. 建议的执行顺序

1. 同时启动 **A、B(形态 2)、C1、E1、E2、F3**。它们改动的代码区域基本不重叠。
2. **C2** 先做测量,把结论写成短文档;测量通过再实现。
3. A2 出来后,协调者召集决定:B2、D、E3、G 要不要做、怎么做。
4. Wave 2 按决定启动 B2、C3;D 与 E3 取决于产品决定。
5. 所有工具面变更合并后,做 F1、F2,最后统一收紧预算并做一次发现体积的最终测量。

## 7. 需要你决定的事

| 决定 | 影响的工作流 | 建议 |
| --- | --- | --- |
| 目录合并走形态 1(按 kind)还是形态 2(仅归档并入 update) | B | 形态 2 |
| `operationId` 只对 `commit_plan` 可选,还是所有写工具 | B2 | 已决定:只对 `commit_plan` |
| review 写工具是否默认隐藏 | D | 本轮不做，见 mcp-wave2-deferred.md |
| 是否做 PATCH 统一 | E3 | 本轮不做，见 mcp-wave2-deferred.md |
| 是否删除 `toolGroups` | G | 已删除，见 mcp-wave2-g-toolgroups.md |
| 破坏性批次 skill 版本用 PATCH 还是 MINOR | 所有工具面变更 | 当前政策是 PATCH,若希望标出不兼容可改 MINOR |
