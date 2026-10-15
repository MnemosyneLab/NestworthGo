# 账户核对评审（2026-10-15）

评审对象是今天合并进 `main` 的账户核对：[#89](https://github.com/MnemosyneLab/NestworthGo/pull/89)（`3f3d73f`，会话、有来源的核对、可审计更正）和 [#94](https://github.com/MnemosyneLab/NestworthGo/pull/94)（`c33c735`，冒烟修复里与核对相关的部分）。对照的是当前 `main` 源码、用户/架构/测试文档，以及 Skill `skills/nestworth/references/reconciliation-and-corrections.md`。

本次只读。没有改产品代码，没有推修复，没有合并。没有打开 `~/.config/Nestworth`、R2 或任何真实账本。浏览器手测没有做：本环境没有原生 GUI，且本任务不允许使用第三方 computer-use 子代理。证据来自源码、现有测试，以及下面附录里对隔离代码路径跑过的 Go / jsdom 测试。

严重程度：

- **P0**：会写错金额、把未核对写成已成功，或丢掉已提交证据。本次没有。
- **P1**：家庭用户高概率会信错状态、开错单，或在清单里看不到一笔还在账上的数量。
- **P2**：能完成，但恢复路径、文案或架构边界容易绊倒人。
- **P3**：抛光。

## 执行摘要

核对的账本边界是扎实的：更正不会自动把某一项打成「已确认」，完成记录不会被后来的记账改写，收据优先于过期。缺的是家庭心智上的几件事：什么叫「上次对过」、一场核对还在不在、账单数字和账本数字是不是同一笔、漏掉的东西会不会自己出现在清单里。

最重要的五项：

1. **账户页的「上次成功核对」不会告诉你后来账变了。** 时间戳一直留着，重新核对的提示只出现在那一场已完成记录里，不出现在账户卡片上。
2. **同一批账户可以同时开多场进行中的核对。** 入口先让你再开一场，未完成的场次埋在下面的列表里。
3. **持仓上的「补记买入或卖出」默认打开收入单。** 种类没有锁住，但起点是「钱进来了」。
4. **已归档标的上如果还留着数量，核对清单直接不列它。** 完整性仍可被确认，整场仍可完成。
5. **「已确认」不要求来源上的数字等于账本数字。** 来源金额是可选项，填了不一致也不会警告。完整性又排在清单最后，标题是一整段说明。

#94 已经修掉、不必再报的：核对项字段错误能指到具体输入；空的来源片段不再拼出多余分隔符；持仓检查点显示数量和单位；已完成会话再提交时，MCP 会明确说这是只读冲突，要新开一场。

## 问题表

| ID | 严重程度 | 范围 | 摘要 | 证据 | 建议方向 |
| --- | --- | --- | --- | --- | --- |
| AR-01 | P1 | 产品 | 账户卡片上的上次成功时间，在之后的记账之后仍然像「现在还对得上」 | `ReconciliationLastSuccess` 读库存结果，不计算 `RecheckRequired`。`AccountReconciliation` 只渲染 `completedAt` | 上次成功时间旁边同时显示「之后有记账，需要再看一眼」，并链到那一场记录 |
| AR-02 | P1 | 产品 / 交互 | 进行中或已暂停的核对不会阻止再开一场；首页也不把「继续」放在最前 | `prepareReconciliationSession` 的 `start` 不查重。会话表没有「每户一场进行中」的约束。页面先渲染账户勾选 | 有重叠的未完成场次时，先给出继续卡片；再开一场要明确说这是另一份记录 |
| AR-03 | P1 | 交互 | 「补记收入、支出、买入或卖出」对持仓默认是收入 | `route === "record"` 使用 `ChangeMoneyAdded`，并带上 `holdingId`。导入持仓才锁种类 | 持仓行默认买卖该持仓；现金行默认收支。按钮文案和打开的表单一致 |
| AR-04 | P1 | 产品 / 正确性 | 标的已归档但持仓数量还在时，该项不会进入清单，也不会标成已移出 | `captureReconciliation` 在 `instrument.ArchivedAt != nil` 时 `continue`。`ArchiveInstrument` 不要求数量为零 | 只要数量还在，就列为待核对或「标的已归档、数量仍在」。不要靠完整性一句带过 |
| AR-05 | P1 | 产品 | 确认不比较来源金额和账本金额。不一致仍可记为已确认 | `confirmed` 分支只要求来源名称和日期。`referenceValue` 可选，且无相等检查 | 填了来源金额就标出是否与账本一致；不一致时请人选更正、跳过或未解决，不要默默已确认 |
| AR-06 | P2 | 信息架构 | 「还有没有漏记」排在现金、持仓、产品后面，标题是一整段话 | 项目按 `id` 排序，`scope` 在 `holding:` / `product:` 之后。界面用 `reconciliation.scope` 整句当标题 | 固定放在账户第一行，短标题如「还有遗漏吗？」，说明放在下一行 |
| AR-07 | P2 | 交互 | 部分完成的错误引用了界面上没有的「允许部分完成」。接着打结束说明会把错误清掉 | 简体勾选是「明确保留待核对项并部分完成」。`closing-note` 的 `onChange` 调用 `clearError`。测试 `identifies partial finish intent...` 把这个清空锁进了回归 | 错误复述勾选框自己的话。勾选和说明都齐了再清错误，并改测试 |
| AR-08 | P2 | 交互 | 账本变了以后，界面只说「因存在冲突，操作未能完成」，也不刷新当前这场 | `conflict` 的简体文案是这句。`financialRevision` 不在 `fieldLabelKeys.json`。预览失败不会 `invalidateQueries` | 说「数字已经变了，请关掉后重新看」。冲突时刷新这场会话 |
| AR-09 | P2 | 交互 | 增加数量时，两个成本框都写「可选」。填两个或一个都不填，只会得到笼统错误 | `addedUnitCost` / `totalCost`。`AdjustmentForm` 把服务端错误交给 `displayError`，没有字段定位 | 数量变多时要求且只要求其中一项，并把错误放在对应输入上 |
| AR-10 | P2 | 产品 / UI | 托管产品更正把条款、收息、续期、到账、估值堆在一张表里。到期不等于到账没有写在这张表上 | `ProductCorrectionForm` 的模式下拉。用户文档有这句话，表单没有 | 四个短选项。到账时加一行：到期日本身不是钱已经进账 |
| AR-11 | P2 | UI | 这一页比账户、持仓、可用资金更密、更像审计文件。状态是纯文字，差额没有颜色 | 常驻三句说明。每项最多四个长按钮。`recheck` 是一整句，没有警示色。预览里的增减用历史文案，没有涨跌色 | 短状态：对上了、还没看、对不上、先跳过。重新核对用颜色。更正预览写从多少到多少 |
| AR-12 | P2 | 交互 | 暂停和继续走与改账相同的「提交已审阅结果」。暂停中仍可直接完成 | 非完成状态同时显示暂停和完成。`finish` 不要求状态为 `in_progress` | 暂停和继续立即保存。完成的提交写明「完成后这份记录不能再改」。暂停中先继续再完成 |
| AR-13 | P2 | 信息架构 | 同一套「核对 / reconcile」用在两件不同的事上。账户上的历史按钮名不副实 | 导航是「账户核对」。历史原因 `history.reason.reconciliation` 是「对账」。历史全景徽章是「已核对」。账户按钮 `reconciliation.history` 打开的是上次成功那场，不是进行中。返回按钮文案也是「全部核对」，从账户进入时筛选还在 | 本功能保持「账户核对」。历史全景徽章改成「对上了」。历史按钮写成「上次对过的记录」。返回写成「返回列表」 |
| AR-14 | P2 | 交互 / 无障碍 | 没有账户、筛空、加载失败时，下一步不清楚。已移出的项仍提供补记 | 空列表一律「还没有核对记录」。`ErrorState` 未传 `retryLabel`，图标 `aria-hidden`。`kind === "removed"` 仍走 `item.kind !== "product"` 的补记按钮 | 各写一句空状态和下一步。重试按钮有名字。已移出只保留「这项已经不在账户里」 |
| AR-15 | P2 | 架构 | 产品合约的任何更新都会抬高账户财务修订，包括备注 | `reconciliation_product_contracts_update` 没有列过滤。写入 upsert 会改 `note` 和 `updated_at` | 只在本金、状态、币种、日期、利率变化时抬修订 |
| AR-16 | P2 | 产品 / 架构 | 完成记录说要重新核对，但「上次成功以来的变更」不含完成后的那些变动。JSON 导出也没有修订日志 | `ReconciliationChanges` 在已完成时丢掉 `revision > FinancialRevision` 的行。导出名录只有 sessions/accounts/items/links | 重新核对提示旁列出之后的事实。导出增加财务变更数据集；计划和收据配置继续排除 |
| AR-17 | P2 | 架构 | 一笔更正可以挂到同账户的另一类核对项上 | `ValidateReconciliationLink` 只要求效果落在所选账户。不看 `cash` / `holding` / `scope` | 拒绝与该项种类不符的关联。结果仍保持「更正不等于已确认」 |
| AR-18 | P2 | 架构 | 桌面计划从内存消失后，提交到不了收据回放。MCP 在计划文件缺失时会把同一次操作记成失败 | Wails `Commit` 在 `sessions.Get` 失败时直接返回，字段是 `planId`。该字段不在 GUI 的「可关闭」列表里。MCP `loadPreviewPlan` 先返回 `not_found`，`executeWithRecovery` 把非 internal 错误写成 `failed` | 计划正文没有时，先按计划 ID 查会话收据，再决定是失败还是未知 |
| AR-19 | P3 | 交互 | 核对结果默认是「已确认」 | `CheckForm` 的 `useState("confirmed")`。真正写入仍要来源和日期，并再经预览 | 默认「待核对」，或按用户上次的选择 |
| AR-20 | P3 | 数据展示 | 完成时的结束说明覆盖这场核对开头的备注 | `finish` 执行 `session.Note = in.Note`。部分完成的展示用的就是这个字段 | 开场备注和结束说明分开存 |
| AR-21 | P3 | 文案 | 第一次核对没有「上次成功」水位，标题仍是「上次成功核对以来的变更」 | `PriorRevision == 0` 时 `ReconciliationChanges` 直接跳过。这是文档里的设计，标题没有说「还没有上次」 | 第一次显示「这是第一次核对，没有更早的对照」 |

## 分轴说明

### 1. 功能与产品

家庭对账的心智是：拿着银行或券商账单，一个账户一个账户看现金、持仓、理财是不是账上这些，有没有漏记，对完留下一个还能信的时间。实现已经按账户圈定范围，现金、普通持仓、托管产品和一条完整性分开列，部分完成不会把没确认的账户推进上次成功时间。这些和心智是对齐的。

对不齐的是「对过了」这句话的有效期，以及「没列出来的东西」。

AR-01：`reconciliationLastSuccess` 按完成时间从新到旧找会话，读库存的账户结果，看到 `Outcome == "confirmed"` 就返回整场会话。它不调用 `getReconciliationSession`，因此不会根据当前 `account_financial_revisions` 计算 `RecheckRequired`。账户页 `AccountReconciliation` 只显示 `last.data.completedAt`，没有重新核对句。会话详情里是有这句的（`a.recheckRequired` 时渲染 `reconciliation.recheck`）。所以证据存在，只是不在人最常看的地方。一笔之后的收入会让完成记录上出现提示，账户卡片仍写着上次成功的时间。

AR-02：`start` 只检查账户属于本家庭、未归档、不重复、1 到 100 个。空的 `accountIds` 表示全部活跃账户。没有「这些账户已经有一场 `in_progress` 或 `paused`」的拒绝。表 `reconciliation_sessions` 也没有这样的唯一约束。两场都可以把同一账户确认到不同的上次成功时间；后完成且该账户为 confirmed 的那场胜出。列表按钮只显示名称、开始时间和状态，虽然列表接口会给未完成场次补上计数，界面不用。从侧栏进入时，勾选默认是全部活跃账户（`selected === null` 时 `chosen` 取全部）。从账户页进入只勾该账户，这是好的。

AR-04：捕获时，持仓本身已归档、或对应标的已归档，就 `continue`。会话进行中如果标的被归档，财务修订会变，旧行会变成 `kind: "removed"`，这是对的。若在开场之前就已经归档，这行从未存在，完整性仍可凭来源名称和日期确认，全部确认后可以完整结束。`ArchiveInstrument` 只拒绝托管标的，不要求剩余数量为零。这和用户文档「对上已知持仓并不能说明没有遗漏」是同一类洞，只是洞在系统自己知道的数量上。

AR-05：确认现金、余额、持仓时，空的当前值或空的持仓成本会被拒绝，这是对的；已知零成本可以确认，测试 `TestReconciliationKnownZeroAndUnknownCost` 覆盖了。来源上看到的数字不参与判断。用户可以不填，也可以填一个不同的数，结果仍是已确认。书面契约把 `referenceValue` 定义成可选证据，所以这不是「实现违反了文档」，而是文档允许了一种家庭用户会理解成「对上了」的操作。

现金只在快照里已经有该币种行时出现（`snapshot.CashValues`）。持仓账户没有单独的「本位币现金 0」行。余额账户在有最新价值时有一条 `balance`，若同时有现金行还会再列现金。漏记的币种、漏记的品种，全靠那条 `scope`。设计如此，和 AR-06 叠在一起时，人很容易对完列出的行就点完成。

托管产品走现有产品模型，含已结束合约；由产品托管的持仓不会再当成普通持仓。直接更正不制造买卖。这些应保留。

部分完成的规则是清楚的：未确认、跳过、未解决都在，就必须 `allowPartial` 加结束说明；已全部确认的账户才会写下 `LastSuccessfulAt`。一场部分完成里可以有一个账户成功、其他账户不成功。`TestReconciliationSessionEvidenceDurabilityPartialAndImmutable` 覆盖了单账户部分完成。

### 2. 代码与架构

Schema 17 的新库 DDL 和 v16 升级读的是同一份核对定义（`upgrade_v16.go` 读 `reconciliation_schema.sql`，`schema.sql` 含等价触发器）。升级前有只读恢复副本。收据在过期判断之前查找。同一计划哈希回放原结果，换了内容则冲突。关联写入在财务事务里失败即整体回滚，并且不会把核对项标成已确认。`TestReconciliationLinkedCorrectionAtomicReceiptAndExplicitRecheck` 期望确认数仍为 0。行情、净值和汇率不抬财务修订；纯 `value_observation` / `valued` 也不抬，条款和现金仍抬。这是 #89 里值得留下的部分，#94 没有改坏它。

#94 在 `prepareReconciliationSession` 里把「会话已完成」从「证据变了」拆出来。完成会话返回 `conflict` / `expectedRevision`，文案是 `session is completed and read-only; start a new reconciliation session`。`TestReconciliationSessionHTTPWorkflowBothCorrectionRoutes` 断言了这些词。GUI 和 MCP 走同一方法。

AR-15：`reconciliation_product_contracts_update`（`reconciliation_schema.sql` 约 76–79 行，`schema.sql` 约 858 行）在任意 `UPDATE` 后把该账户修订加一。产品保存会 upsert 名称、备注、到期、利息、状态和 `updated_at`。改一句备注就会让该账户所有已确认项失效。估值豁免本身是窄的，只跳过 `role='valued'` 且 `kind='value_observation'`，这是对的；不要为了修备注把这条例外放宽。

AR-16：已完成会话的 `RecheckRequired` 是读时投影，库存结果不动，这是对的。但 `ReconciliationChanges` 在已完成时只保留 `row.Revision <= a.FinancialRevision` 的行。触发「需要重新核对」的，恰恰是比冻结修订更新的那些行，它们不出现在这场记录的变更列表里。新开一场时 `PriorRevision` 指向旧修订，那些行才会出现。导出格式 2 有四张核对表，没有 `account_financial_revisions` 和 `reconciliation_financial_changes`。文档写明导出不是可恢复数据库、排除计划和收据，这个边界应保留；变更日志是「为什么要重核」的业务证据，不是私密计划。

AR-17：链接校验保证每个效果打到所选账户，然后事务里插入链接。现金调整可以挂在持仓行或完整性行上。核对结果不会因此变成已确认，所以不是假成功，但审计链会指错行。

AR-18：应用层 `CommitReconciliationSession` 的顺序是对的：先 `ReconciliationReceipt`，再看 `ExpiresAt`，再重算并比较。桌面适配器在这之前就失败了。`preview.Store.Get` 找不到计划时返回 `stale_preview`，字段是 `planId`，文案是 `request a fresh preview`。内存计划在进程重启后本来就没有；24 小时或 1024 条上限也会删掉旧计划。GUI `CommitReview` 把 `expiresAt` 当成可以关掉的过期，把 `planId` 留在未知结果里：表格不关，重试仍用同一个已经不在内存里的 `planId`，收据不会被查到。MCP 的计划文件在磁盘上，重启通常还在，这比桌面好。文件真的没了时，`loadPreviewPlan` 返回 `not_found`，恢复层把这次 `operationId` 记成 `failed`。下一次同一 `operationId` 直接返回这个失败，不再进入 `CommitReconciliationSession`。Skill 要求先重试原操作再考虑新操作；这一步会得到「计划不存在」，而不是收据里的原结果。会话类操作因此可能被再开一场（见 AR-02）。财务更正如果换新 `operationId` 再提交，才有重复入账的风险；那条路径和别的账本提交一样，取决于计划文件还在不在。

移除项的排序和 100 次尝试上限是稳的。刷新后按 `id` 排序再比较预览和提交。`TestReconciliationRemovedItemsHaveStablePreviewAndCommitOrder` 覆盖了顺序、待核对，以及尝试上保留的旧修订。第 101 次被拒绝，而不是丢掉历史。

成本只通过 `activity_effects` 抬修订，不改 `holdings.quantity`，所以成本更正会使该项失效。数量变更会因持仓更新和效果插入各抬一次，偏保守，不是漏掉。当前代码里没有 `UPDATE holdings SET account_id`。`position_transfer` 是两条持仓上的数量效果。触发器 `reconciliation_holdings_update` 在 `account_id` 变化时只给 `NEW.account_id` 加修订。这是潜伏缺口，不是今天转账路径上的错。见文末未验证。

### 3. 交互

开始、核对、暂停、继续、完成、直接更正、产品操作，都先预览再提交。未知提交会挡住 Escape 和导航，并重试同一个计划。已证明过期（字段 `expiresAt`）的预览可以关掉再审。这些在 `ReconciliationPage.test.tsx` 里有对应用例，应保留。

AR-03 的按钮在所有非产品行上都会出现，包括现金、余额、持仓、完整性和已移出。导入已有持仓才会 `lock.kind = position_import`。补记路径的 `initial` 用 `ChangeMoneyAdded`。真实的记录表单没有在这条路径上被锁死，用户可以改种类，但打开时是收入。测试把 `RecordChangeForm` mock 掉了，只断言链接 JSON，没有断言持仓行的默认种类。

AR-07：服务端文案是 `partial finish requires explicit intent and a closing explanation`。界面把它映射成简体「请勾选“允许部分完成”并填写结束说明」。勾选框写的是「明确保留待核对项并部分完成」。英文两者更接近（checkbox “Finish partially with outstanding checks”，错误 “enable partial completion”），仍然不是同一句。结束说明一输入就 `clearError()`。测试在只打了说明、没有勾选时断言警告消失。人会以为已经好了，再预览又得到同一句。

AR-08：`translateWailsError` 按错误码取文案，不用服务端英文。`conflict` 的简体是「因存在冲突，操作未能完成。」`financialRevision`、`expectedRevision`、`preview` 都没有字段标签，所以「evidence changed; reread and review before confirming」到不了界面。预览的 `catch` 不刷新 `["reconciliation"]`。打开着的清单仍是旧数字。数据没有被写错，因为提交被拒绝了；人不知道该重看。

AR-09：增加数量必须在「新增份额的单位成本」和「整个剩余持仓的总成本」里正好填一个。两个框都标可选。帮助段落解释了差别，但错误落到整表一句。填错成本会改变家庭以后看到的持仓成本，所以这个文案值得收紧，即使服务端最终会拒绝空提交。

AR-12：暂停的预览和改余额的预览共用「提交已审阅结果」。完成一场也会用这句，不说明记录将冻结。暂停状态下核对按钮隐藏了，完成按钮还在。后端允许对暂停会话直接 `finish`。晚饭时点暂停的人，可能把「提交」理解成入账。

AR-14：零个活跃账户时，开始按钮禁用，空状态仍是「还没有核对记录。」筛选无结果用同一句。账户页和历史页的错误状态会传「再试一次」；这一页不传，重试按钮只剩一个隐藏图标。关联变更和最近变更出错时没有重试。已移出的行仍提供补记。

AR-19、焦点：核对表单默认已确认。完成表和预览表没有像编辑表那样 `finalFocus`。分页只有上一页 / 下一页，页码不朗读。这些是 P3。

不可逆的更正有一次预览，没有第二层「你确定吗」。这可以接受，因为预览展示增减，提交按钮是明确的。完成一场全部确认时不需要额外勾选；有未完成项时需要勾选。这个分层是对的，文案问题见 AR-07。

### 4. 界面

Nestworth 其他账本页面用卡片、徽章、账户图标和短状态。核对页是边框列表加等宽数字。常驻说明三句都对，但不该永远占着第一屏。每项标题是「名称 · 种类」，状态是「待核对 / 已确认 / 已跳过 / 差异未解决」纯文本。重新核对是一整句灰色说明。直接更正的预览写「金额增加 / 金额减少」，没有用持仓里已有的涨跌颜色；核对本身也不是盈亏，颜色应表示「对上 / 对不上 / 要重看」，而不是收益。

中文偏审计腔，和家庭记账的其余页面不一致：审阅核对项、提交已审阅结果、明确关联的变更、变更前已审阅值、已审阅单位成本、来源凭证（字段本身叫「对账单或来源参考」）、结束说明。简体、繁体是成套的，问题是语气，不是漏译。`en` / `zh-CN` / `zh-TW` 的测试确认业务文案会渲染，协议 ID 不漏到界面，这点应保留。

持仓检查点经 `checkpointValue` 显示数量和单位，现金保留货币，预览保留精确小数。这是 #94 的修正，测试 `shows holding quantities without quote currency...` 和 `preserves the canonical cash amount...` 在本次 jsdom 运行里通过。不要为了「看起来像钱」把数量再格式成金额。

`ProductCorrection` 复用存款条款、策略和时间字段，预览里再用禁用的条款块展示前后。对家庭用户，这是第二套产品管理界面。可用资金把这些叫存款或理财；这里叫托管产品。AR-10。

加载用通用骨架，页面实际是列表。空状态没有下一步。见 AR-14。

没有在原生窗口里看过真实排版、键盘走查或屏幕阅读器。上面是从组件结构和文案得出的，不是截图结论。

### 5. 信息架构

导航在「活动」组，紧挨近期事项和历史，名称是「账户核对」（`nav.reconciliation` 与页面标题一致）。这比放进投资或历史全景更合理。图标 `GitCompareArrows` 同时用于「资产变动」。

碰撞不在导航标题，而在动词：

| 位置 | 简体 | 英文 |
| --- | --- | --- |
| 导航和本页标题 | 账户核对 | Reconciliation |
| 历史记录原因 | 对账 | Reconciliation |
| 成本调整帮助 | 请重新对账目标成本 | re-reconcile |
| 历史全景说明 | 金额如何核对 | How these amounts reconcile |
| 历史全景徽章 | 已核对 | Reconciled |

历史全景是净资产变化的算术解释。本页是「账单和账本是不是一回事」。英文都叫 reconcile。简体徽章「已核对」和本功能的动词「核对」会靠在一起。本功能应继续叫账户核对。历史全景不要用「核对」当成功徽章。

账户页两个按钮：`reconciliation.open` 是「开始、继续或查看核对」，打开的是带账户筛选的列表，不是自动续上未完成的那场。`reconciliation.history` 是「进行中的核对与历史」，打开的是上次完全确认的那场。名字和动作是反的。从账户进入后，返回按钮也写「全部核对」，查询仍按该账户过滤（`useReconciliationSessions(accountId)`，测试 `limits account-entry history...` 锁定了这个筛选）。AR-13。

完成记录是只读的，计数、来源、尝试和关联变更都在。重新核对句在，触发它的后续事实不在。见 AR-16。

`docs/design` 没有这份 GUI 的线框。`monthly-review-and-data-confidence.md` 是另一件计划中的复核，也使用「核对」，并曾说不要加侧栏项。本功能已经有侧栏项。两套「核对」以后还会再撞，值得在文案上先分开。

## 应该留下的

- 更正、补记、产品操作都不会自动确认核对项。工作点在财务事务里快照，会话修订加一，下一次读取把当前结果放回待核对，并保留尝试。
- 完成记录冻结。后来的记账只加 `RecheckRequired` 投影，不改原来的完成时间和当时的结果。#94 把「已完成」说成只读冲突，而不是「证据变了」。
- 全部确认才推进该账户的上次成功时间。跳过和未解决会挡住该账户。部分场次可以只成功其中一些账户。
- 确认需要用户提供的来源和民事日期。只有日期时不发明时间。有时间则用账本时区。跳过和未解决必须有说明。空的当前金额或未知成本不能确认；已知零成本可以。
- 预览十分钟过期。同一计划先查收据再谈过期。桌面和 MCP 的会话动作共用 `application.Service`。MCP 读有 25/50 行和 96 KiB 上限，游标绑定筛选和修订。
- 直接目标调整不制造交易。增加数量的单位成本和整个剩余持仓的总成本仍然是两件不同的事。托管产品不走普通持仓调整。
- 会话类 MCP 通知只刷新核对读，不把描述性价格 / 净值表现标脏。
- Schema 17 升级有校验过的恢复副本。JSON 导出带上了四类核对证据。Skill 1.10 的会话步骤和「不要编造来源」写清楚了。

## 未验证 / 开放问题

- **重放行改写现金而不抬修订（假设）。** 清单上的现金来自 `listCashValuesQuery`，按生效时间取最新一行，不区分 `projection_kind`。回放行的插入被触发器跳过。`replaceCurrentProjectionsTx` 会删掉并重插 `replay` 行。若一次重建让当前现金金额变了、又没有新的活动效果，已确认项可能不会变成要重核。`TestReconciliationUnchangedRebuildPreservesEvidence` 只锁定「没变就不抬修订」。没有跑「重算后金额不同」的夹具。持仓数量若被 `UPDATE holdings SET quantity` 改掉，现有触发器会抬修订，那条路径不是这个假设。
- **持仓换账户的触发器（潜伏）。** `reconciliation_holdings_update` 只更新 `NEW.account_id`。仓库里的持仓更新是数量、备注、排序、归档和时间，没有改 `account_id`。转账用两条效果。今天的转账不会踩中。若以后有人直接改持仓所属账户，源账户的修订和已完成记录都不会动。产品合约更新同样只写 `NEW.account_id`，当前 upsert 也不改账户。
- **余额账户是否同时出现「账户余额」和分币种现金。** 捕获逻辑会在非持仓账户上既加 `balance`，又追加已有现金行。没有用一户合成数据把两种展示并排跑出来。
- **原生 macOS、屏幕阅读器、窗口关闭时的桌面计划恢复。** #89 / #94 的说明已经把这些标成未做。本次同样没有做。jsdom 不能代替。
- **真实浏览器走查。** 没有。原因见附录。
- **#94 最终 CI 在合并说明里写的是成功。** 本次没有重跑 GitHub CI，也没有重跑带 `-race` 的全部分片。本地非竞态的核对测试通过，见附录。

## 附录

### 审阅的提交与文档

- `main` @ `c33c735`（2026-10-15，包含 #89 与 #94）
- #89 合并提交 `3f3d73f07759a177f3726cdb10637ee6716856a8`
- #94 合并提交 `c33c735cea1beb3dfb77e75cd050be940103bcdd`
- `docs/user/account-reconciliation.md`
- `docs/development/account-reconciliation.md`
- `docs/development/account-reconciliation-plan.md`
- `docs/testing/account-reconciliation.md`
- `skills/nestworth/references/reconciliation-and-corrections.md`
- 历史全景文案：`frontend/src/i18n/historicalReconciliation.ts`（对照用，不评审 #93 的可用资金 / 历史全景改版本身）

#94 里与核对无关、本次不展开的：历史全景估计变化文案、日历月份箭头、持仓检查点以外的行情空状态。

### 跑过的命令

隔离在本仓库测试进程内，没有个人数据库。

```sh
export GOFLAGS='-mod=readonly -tags=server'
CGO_ENABLED=0 go test -count=1 -timeout 12m \
  ./internal/application ./internal/infrastructure/sqlite ./internal/mcpserver \
  -run 'TestReconciliation'
```

结果：三个包 `ok`（application 1.117s，sqlite 0.544s，mcpserver 1.388s）。其中包含会话耐久性、修订与上次成功、关联更正不自动确认、混合账户、移除项顺序、过期与收据优先、已知零成本、产品估值不抬修订、MCP 完成会话只读冲突、分页预算。

```sh
CGO_ENABLED=0 go test -count=1 -timeout 5m ./internal/wailsapi/reconciliation
```

结果：`ok`（0.174s）。

前端依赖用 pnpm 10.33.3 安装（锁文件已满足；系统里的 pnpm 12.2.1 占位脚本不能执行，因此关闭了版本切换）。绑定是本地生成的，被 `.gitignore` 忽略，没有进提交。

```sh
cd frontend
npm_config_manage_package_manager_versions=false \
  pnpm exec vitest run src/features/reconciliation src/queries/reconciliation.test.tsx --maxWorkers=2
```

结果：3 个文件，41 个测试通过（vitest 4.1.11，约 6.8s）。覆盖预览后才提交、来源日期时间、未知提交重试、过期预览可关闭、部分完成、字段错误、数量与现金展示、账户筛选缓存。`ProductCorrection` 在页面测试里是 mock。

没有跑：`go test -race`、`go test ./...`、前端全量、lint、typecheck、生产构建、原生 Linux/macOS GUI、屏幕阅读器。

### 界面测试没有覆盖、本次是读代码得出的

暂停 / 继续、零账户、全选、账户内返回按钮的文案、重新核对的样式、已移出行、关联变更与最近变更、真实产品表单、成本字段错误、并行会话、简体「允许部分完成」与勾选框不一致。最后一项的英文测试反而把「一打说明就清错误」固定下来了。
