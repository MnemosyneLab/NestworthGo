# Nestworth 分析功能测试方案

**文档状态：** QA Test Plan v1  
**适用版本：** Nestworth `main`（Analytics Redesign 已完成实现）  
**目标平台：** macOS / Apple Silicon 为主，Wails v3 Desktop  
**测试范围：** `Return Analysis`（收益分析）与 `Asset Changes`（资产变动）  
**日期：** 2026-09-08

---

## 1. 目的

本测试方案用于对 Nestworth 新版分析工作区进行一次完整的发布前验证。

重点不是只确认“页面能打开、按钮能点击”，而是同时验证：

1. **金融计算正确性**：收益金额、Modified Dietz 收益率、Price / FX 拆分、资产变化归因、Residual 等结果可信。
2. **Scope / Filter 语义正确性**：同一事件在 Portfolio、Account、Instrument 不同范围下应得到不同但合理的解释。
3. **六个分析 Tab 的功能完整性**：Calendar、Return Trend、Contribution、Change Drivers、Asset Trend、Categories。
4. **跨页面 Drill-down 正确性**：Return Analysis ↔ Asset Changes ↔ History。
5. **数据可信度状态正确**：missing / partial / unavailable / forced-base 不得伪装成 0 或完整结果。
6. **桌面端实际交互和视觉质量**：自动化测试无法完全替代 Wails 原生窗口验证。
7. **回归安全**：新版 Insights 不应破坏 Investments、History、账户和行情等现有功能。

---

## 2. 当前实现基线

当前侧边栏 `Insights` 下应只有两个分析入口：

```text
Insights
  Return Analysis
  Asset Changes
```

旧的 `Analysis` 页面不再存在。

### 2.1 Return Analysis

包含三个 Tab：

```text
Return Calendar
Return Trend
Contribution
```

### 2.2 Asset Changes

包含三个 Tab：

```text
Change Drivers
Asset Trend
Categories
```

### 2.3 共用筛选条件

当前实现支持：

```text
Scope
  Portfolio
  Account
  Instrument

Valuation
  Base currency
  Native currency

Cash
  Include
  Exclude

Date
  From
  To

More Filters
  Account
  Currency
  Asset Class
  Member
  Instrument
```

`Return Basis` 当前固定为 `investment`，不作为可见筛选项。

### 2.4 当前 v1 明确不测试为缺陷的能力

以下能力不属于当前 v1，不应作为 release blocker：

- XIRR / MWR。
- Intraday mark-to-market TWR。
- Contribution 中的 Unrealized Gain UI（当前界面只暴露 Total Return / Realized Gain / Dividend & Interest）。
- Tags。
- Multi-select scope。
- Budget category tree。
- Per-currency native section。
- Contribution export / standalone data-table。
- 跨应用重启持久化分析筛选条件。
- Live “Today” return；当天不是 closed snapshot，应显示 muted / explanatory state。
- Split-adjusted historical price series / spin-off cost basis allocation。

---

## 3. 测试策略

测试分成六层。

| 层级 | 目标 | 是否 release gate |
|---|---|---|
| A. Static / automated | 编译、lint、unit/integration、bindings | 是 |
| B. Engine correctness | Golden cases、计算恒等式、收益率 | 是 |
| C. Functional desktop | 六个 Tab 和筛选交互 | 是 |
| D. Cross-page E2E | Insights ↔ History / 页面间跳转 | 是 |
| E. Trust / robustness | partial、missing、residual、error | 是 |
| F. UX / performance | 桌面视觉、键盘、长区间性能 | 是 |

原则：

> 自动化负责证明“算法和状态机”；桌面 QA 负责证明“用户实际看到和操作到的结果”。

---

## 4. 测试前置条件

### 4.1 代码验证

在 clean checkout 上执行：

```bash
wails3 task setup
wails3 task check
```

`wails3 task check` 应完成：

- frontend build
- `gofmt`
- `go test ./...`
- `go vet ./...`
- Go build
- frontend lint
- frontend typecheck
- frontend Vitest
- `git diff --check`

全部必须通过。

额外建议执行一次：

```bash
cd frontend
pnpm test
```

以及针对分析 engine 的 focused run：

```bash
go test ./internal/application/... ./internal/wailsapi/...
```

如开发环境允许，建议再跑：

```bash
go test -race ./internal/application/...
```

### 4.2 桌面测试运行方式

启动：

```bash
wails3 task dev
```

至少再使用一次 production build：

```bash
wails3 task build
```

建议最终 release candidate 使用 `.app` 进行完整 smoke。

---

## 5. 测试环境矩阵

不要求每个 Test Case 在每个组合上重复执行。P0 case 应覆盖主要组合。

| 维度 | 必测 |
|---|---|
| OS | macOS Apple Silicon |
| Window | 约 1280×800、1440×900、较窄桌面宽度 |
| Locale | English、简体中文、繁体中文 |
| Base currency | CNY、USD 至少各一次 |
| Timezone | Asia/Singapore、UTC |
| DST | America/Los_Angeles 使用自动化测试验证 |
| Week start | Monday；如可配置再抽测 Sunday |
| Valuation | Base、Native |
| Scope | Portfolio、Account、Instrument |
| Cash | Include、Exclude |

---

## 6. 建议 QA 数据集

不要只用真实个人数据做验收。真实数据适合 smoke，但不适合核对精确结果。

建议准备一个固定的 **Analytics QA Household**，能够重复恢复。

### 6.1 账户

```text
Household base currency: CNY

CMB Cash
  mode: balance
  currency: CNY

DBS
  mode: holdings/balance
  currencies: SGD / USD

MooMoo SG
  mode: holdings
  cash: USD / SGD

Loan
  liability
  currency: CNY
```

### 6.2 标的

```text
QQQ    USD
NVDA   USD
ES3    SGD
```

### 6.3 必须能构造的事件

- contribution
- withdrawal
- salary / income
- interest
- spending
- internal account transfer
- buy
- partial sell
- same-day buy + sell
- trade commission
- dividend
- associated tax
- unrelated bank fee
- FX conversion
- loan drawdown
- loan repayment
- loan interest
- manual value update
- missing quote
- missing FX
- residual / inconsistent snapshot fixture

---

## 7. 四组基准计算数据

这些 case 应作为手工核对和自动化测试之间的桥梁。

### Fixture A — Modified Dietz

```text
Start invested capital       10,000 CNY
12:00 contribution          +90,000 CNY
Investment profit            +1,000 CNY
End                         101,000 CNY
```

期望：

```text
Return amount = +1,000

Dietz denominator
= 10,000 + 90,000 × 0.5
= 55,000

Daily return
≈ 1.81818%
```

绝不能显示 `10%`。

同样金额若是 `salary` 且 `Include Cash = true`：

- Asset Changes：`Income +90,000`
- 不得再出现第二个 External Flow +90,000
- Return denominator 仍应把 +90,000 当作 Dietz capital
- Return amount 仍只包含 +1,000 投资收益

### Fixture B — Foreign holding Price / FX

```text
1 share

Open:
price = $100
USD/CNY = 7.0

Close:
price = $110
USD/CNY = 7.2
```

Base CNY：

```text
Beginning = 700
Ending    = 792
Change     = 92

Price Change = +70
FX Impact    = +22
```

Native USD：

```text
Price Change = +10 USD
FX Impact    = 0
```

### Fixture C — Foreign cash + income

```text
Open USD cash      $100 @ 7.0
12:00 salary       +$10 @ 7.1
Close USD cash     $110 @ 7.2
```

Base CNY：

```text
Beginning = 700
Ending    = 792

Income     = +71
Cash FX    = +21
Residual   = 0
```

绝不能显示 `FX Impact = +92`。

### Fixture D — Scope semantics

DBS → MooMoo transfer 100 CNY：

Portfolio scope：

```text
Net worth change = 0
Return           = 0
```

MooMoo Account scope：

```text
External-to-scope flow = +100
Return                 = 0
```

MooMoo cash → QQQ buy：

Account scope：

```text
principal movement is internal
account value does not change because of principal
```

QQQ Instrument scope：

```text
principal becomes External Flow into instrument scope
subsequent price movement becomes Return
```

---

## 8. P0 Smoke Test

以下用例必须首先执行。任何失败都停止后续 release 验收。

| ID | 测试 | 期望 |
|---|---|---|
| SM-01 | 启动 App | 无 crash、无白屏 |
| SM-02 | 查看 Insights 导航 | 只有 Return Analysis / Asset Changes，无旧 Analysis |
| SM-03 | 打开 Return Analysis | 默认进入 Return Calendar |
| SM-04 | 打开 Asset Changes | 默认进入 Change Drivers |
| SM-05 | 在两个页面之间切换 | 无 crash，shared filter 不丢失 |
| SM-06 | 打开三个 Return tabs | Calendar / Trend / Contribution 都能加载 |
| SM-07 | 打开三个 Asset tabs | Drivers / Asset Trend / Categories 都能加载 |
| SM-08 | 修改日期和 Scope | 请求刷新，页面不残留旧结果 |
| SM-09 | 点击 Calendar day | 右侧 Day Sheet 正常打开/关闭 |
| SM-10 | 点击 Change Driver | Driver Sheet 正常打开 |
| SM-11 | 点击 Contribution row | Contribution Sheet 正常打开 |
| SM-12 | 点击 Category row | Category Sheet 正常打开 |
| SM-13 | Insights → History | History 收到正确日期 / account / instrument |
| SM-14 | Calendar → Asset Changes | 日期、scope、cash、valuation 保持 |
| SM-15 | Driver → Return Analysis | 日期范围、scope、return type 保持 |

---

## 9. Shared Filter Bar 测试

### 9.1 Scope

| ID | Priority | 操作 | 期望 |
|---|---|---|---|
| FL-01 | P0 | Scope = Portfolio | 不要求 Scope ID |
| FL-02 | P0 | Scope = Account | 出现 account selector |
| FL-03 | P0 | Account scope 未选 account | 展示 Scope Required empty state |
| FL-04 | P0 | Scope = Instrument | 出现 searchable instrument selector |
| FL-05 | P0 | Instrument scope 未选 instrument | 展示 Scope Required |
| FL-06 | P1 | Account A → Account B | 所有当前 Tab 数据重新计算 |
| FL-07 | P1 | Instrument QQQ → NVDA | 所有当前 Tab 数据重新计算 |
| FL-08 | P1 | 切换 Scope 后切 Tab | Scope 保持 |
| FL-09 | P1 | Return → Asset Changes | Scope / Scope ID 保持 |
| FL-10 | P1 | Reset | 恢复 Portfolio + Base + Include Cash + 默认 filters |

### 9.2 Valuation

| ID | Priority | 操作 | 期望 |
|---|---|---|---|
| FL-11 | P0 | Base | 金额使用 household base currency |
| FL-12 | P0 | Native + 单币种 scope | 使用 native currency，无 FX Impact |
| FL-13 | P0 | Native + 不支持 native 的混合范围 | backend 强制 base；UI 显示 forced-base 标记 |
| FL-14 | P0 | 强制 base 后切换 Tab | session 仍保存用户选择的 Native |
| FL-15 | P1 | Portfolio vs Instrument Native | projection 可独立决定 forced-base |

### 9.3 Include Cash

| ID | Priority | 操作 | 期望 |
|---|---|---|---|
| FL-16 | P0 | Return Include Cash | cash 进入 InvestmentUniverse |
| FL-17 | P0 | Return Exclude Cash | 普通 cash movement 不进入 return capital |
| FL-18 | P0 | Salary + Include Cash | Salary 是 Income，一次；同时影响 Dietz capital |
| FL-19 | P0 | Salary + Exclude Cash | Salary 仍在 Asset Changes；不影响 Dietz capital |
| FL-20 | P1 | Asset Changes 切 Include Cash | 不得改变 Income / Spending physical attribution |

### 9.4 Date

| ID | Priority | 操作 | 期望 |
|---|---|---|---|
| FL-21 | P0 | From > To | UI 不允许或产生 invalid/empty state |
| FL-22 | P0 | To = Today / future | request clamp 到 last closed day |
| FL-23 | P0 | From 早于 History Origin | request clamp 到 origin |
| FL-24 | P1 | Return Trend 30D | 最后 30 个 closed local days |
| FL-25 | P1 | YTD / 1Y / 3Y / All | 起止日期正确 |
| FL-26 | P1 | Custom | shared From / To 生效 |

### 9.5 More Filters

逐项验证：

```text
Account
Currency
Asset Class
Member
Instrument
```

要求：

- 筛选是 AND 语义。
- filter 改变后结果更新。
- 空值表示 All。
- 清空后恢复。
- Scope + filter 交集为空时使用正常 empty state。
- 不得显示范围外 activity / contribution / category。

---

## 10. Return Calendar

### 10.1 Month View

| ID | Priority | 场景 | 期望 |
|---|---|---|---|
| RC-01 | P0 | 打开 Calendar | 默认当前 month |
| RC-02 | P0 | Previous / Next | 月份正确变化 |
| RC-03 | P0 | Today 所在格 | muted；不显示 0% 假数据 |
| RC-04 | P0 | Future day | disabled / muted |
| RC-05 | P0 | Positive day | amount / % 正确；正向轻色背景 |
| RC-06 | P0 | Negative day | amount / % 正确；负向轻色背景 |
| RC-07 | P0 | True zero | 中性，不显示 partial marker |
| RC-08 | P0 | Partial zero | 与 true zero 明显不同，显示 `◇` |
| RC-09 | P1 | Hover day | 显示 Price / FX / Dividend / Fee composition |
| RC-10 | P0 | Click day | 右侧 Day Sheet |
| RC-11 | P0 | Day Sheet | amount、rate、coverage、composition、contributors 一致 |
| RC-12 | P1 | Sheet Escape | 可关闭 |
| RC-13 | P1 | 点击 sheet 外 | 符合 Sheet 组件约定 |
| RC-14 | P0 | Day → Asset Changes | range 精确为该日 |

### 10.2 Summary

验证：

```text
Beginning invested value
Ending invested value
Return amount
Return rate
Rated days / total days
Valuation forced
Partial status
```

要求：

- Return amount 与每天可定义 amount 加总一致。
- Return % 是 daily Modified Dietz geometric link，不是 amount / beginning。
- partial coverage 必须有标记。
- `%` 与 amount coverage 状态不得被伪装为完整结果。

### 10.3 Year View

| ID | Priority | 场景 | 期望 |
|---|---|---|---|
| RC-20 | P0 | Month → Year | 出现 12 month cards |
| RC-21 | P0 | Year total | 使用完整 query 结果 |
| RC-22 | P0 | Month card amount | 对该月日 amount 求和 |
| RC-23 | P0 | Month card % | 几何链接该月 daily rate |
| RC-24 | P0 | Partial month | coverage 正确 |
| RC-25 | P0 | 点击 month | 回到该月 Month View |
| RC-26 | P1 | Future months | no data / muted，不制造 0 return |
| RC-27 | P1 | custom date intersection | 只统计交集 |

---

## 11. Return Trend

### 11.1 Range

验证：

```text
30D
YTD
1Y
3Y
All
Custom
```

要求：

- range button 选中状态正确。
- Custom 仅在 custom range 时 selected。
- named range 修改 shared date filter。
- start 不早于 History Origin。
- end 不晚于 last closed day。

### 11.2 Display

| ID | Priority | Display | 期望 |
|---|---|---|---|
| RT-01 | P0 | Cumulative Amount | money series |
| RT-02 | P0 | Linked Return % | percentage series |
| RT-03 | P0 | Period Return Amount | period amount 正确 |
| RT-04 | P1 | 切换 display | 不修改 scope / filters |
| RT-05 | P1 | Reset display | 回到 cumulative amount |
| RT-06 | P1 | Chart hover | 日期和值格式正确 |
| RT-07 | P1 | Accessible table | 表格值与图表一致 |

### 11.3 Return Sources

必须验证：

```text
Price Change
FX Impact
Dividend & Interest
Investment Fee
```

要求：

- amount 与 period return decomposition 一致。
- share 是普通百分比。
- unrelated bank fee 不进入 Investment Fee。
- associated trade/dividend tax 可进入 Investment Fee。
- source list 不应错误展示 holding contributor 代替 return component。

---

## 12. Contribution

当前支持：

```text
Total Return
Realized Gain
Dividend & Interest
```

### 12.1 Return Type

| ID | Priority | 场景 | 期望 |
|---|---|---|---|
| CO-01 | P0 | Total Return | amount + group Dietz % |
| CO-02 | P0 | Realized Gain | amount；不显示 Total Return % |
| CO-03 | P0 | Dividend & Interest | amount；不显示 Total Return % |
| CO-04 | P1 | Realized unavailable | toolbar 仍保留，可切回 Total |
| CO-05 | P1 | 从 Driver deep-link Dividend | 默认打开 Dividend & Interest |

### 12.2 Group By

Total / Dividend：

```text
Instrument
Account
Currency
Asset Class
```

Realized：

```text
Instrument
Account
```

测试：

- Realized 不出现 Currency / Asset Class。
- 切 Return Type 后若当前 group 不合法，自动回 Instrument。
- label 显示实际 Account / Instrument name。
- 同 group amount 聚合正确。

### 12.3 Sorting

验证：

```text
Amount high → low
Amount low → high
Return rate high → low
Return rate low → high
Name A → Z
```

规则：

- Rate sort 只在有定义的 view 出现。
- positive / negative 正确排序。
- inline bar 长度与 amount 对应。
- 0 amount 不出现 NaN / 异常宽度。

### 12.4 Detail Sheet

验证：

- row title 正确。
- amount 与 row 一致。
- Total Return 显示 rate。
- partial 标记和原因正确。
- composition 显示 Price / FX / Dividend / Fee。
- By Account 正确。
- `View in History` 带正确 from/to/accountId/instrumentId/kinds。

---

## 13. Asset Changes — Change Drivers

### 13.1 Summary

按 Scope：

```text
Portfolio   → Net Worth Change
Account     → Account Value Change
Instrument  → Investment Asset Change
```

验证：

```text
Beginning Value
Ending Value
Change
```

恒等式：

```text
Beginning + all signed drivers + Residual = Ending
```

### 13.2 Waterfall

至少覆盖：

```text
External Flows
Income
Spending
Dividend & Interest
Price Change
FX Impact
Fees
Liability Impact
Adjustments
Unexplained difference
```

要求：

- positive / negative bar 方向正确。
- start / end 正确。
- zero driver 不显示。
- Spending / Fees 本身是 signed negative，不得二次反号。
- Residual 单独显示，不并入 Adjustments。
- waterfall 与 grouped list 一致。

### 13.3 Grouped Attribution

预期：

```text
Cash Flows
Market & Investment
Other
```

要求：

- row 可点击。
- group total = rows sum。
- Residual 使用 warning styling。
- `residualIssueCount > 0` 即使净 residual = 0 也能查看 issues。

### 13.4 Driver Detail

Return-related：

```text
Price Change
FX Impact
Dividend & Interest
Investment Fee
```

应允许 `Open Return Analysis`。

Income / Spending 等 cash-flow driver 不应显示该按钮。

验证：

- By Instrument。
- By Account。
- 当前 period。
- total。
- forced-base / partial。
- cross-navigation context。

### 13.5 Residual Detail

P0：

1. 制造 residual。
2. 页面显示 `Unexplained difference`。
3. 打开 Sheet。
4. 显示 date / component key / account / instrument / amount。
5. `View in History` 精确跳转。
6. 修正数据 / invalidation 后已打开的 Sheet 不得显示 stale total。

---

## 14. Asset Trend

### 14.1 Granularity

```text
Day
Week
Month
```

切换后 request 正确，shared filters 不变化。

### 14.2 Metric Matrix

Level：

```text
Net Worth
Assets
Liabilities
```

Week / Month = **End-of-period value**，不得 average。

Flow：

```text
External Flows
Income
Spending
Fees
```

Week / Month = **sum daily signed values**。

Performance：

```text
Investment Return
Return Rate
Dividend & Interest
Price Change
FX Impact
Net Change
Residual
```

规则：

- Return amount：sum。
- Return Rate：geometric link，不能 sum / average。
- Return Rate 使用 rate，不作为 money。
- Coverage 正确。
- forced-base / partial 标记正确。

---

## 15. Categories

Category Type：

```text
Income
Spending
Fees
Investment Return
Dividend & Interest
```

| ID | Priority | 场景 | 期望 |
|---|---|---|---|
| CAT-01 | P0 | Spending | Total + account rows |
| CAT-02 | P0 | Income | Total + account rows |
| CAT-03 | P0 | Fees | Total + account rows |
| CAT-04 | P0 | Investment Return | investment rows |
| CAT-05 | P0 | Dividend & Interest | instrument-related rows |
| CAT-06 | P1 | 空结果 | 正确 empty state |
| CAT-07 | P0 | 点击 row | Detail Sheet |
| CAT-08 | P0 | Detail children | 与 backend 一致 |
| CAT-09 | P0 | Activity refs | 日期 / amount 正确 |
| CAT-10 | P0 | View in History | 精确 day + dimensions |
| CAT-11 | P1 | Partial | Main 与 Detail 都显示状态 |
| CAT-12 | P1 | Forced Base | 主 list 可见 |

注意：当前 v1 没有 Household / Travel / Grocery 等预算 taxonomy。

---

## 16. 关键金融语义 E2E

### FC-01 Internal Transfer

```text
DBS → MooMoo 100
```

Portfolio：

```text
Asset Change = 0
Return = 0
```

MooMoo：

```text
External Flow = +100
Return = 0
```

### FC-02 Buy Within Account

```text
MooMoo USD cash → QQQ
```

Account scope：principal internal。  
QQQ instrument scope：principal external-to-scope。  
购买后的 price movement 才是 Price Change。

### FC-03 Same-day Buy / Sell

```text
buy 1 @ 10
sell 1 @ 11
```

期望：

```text
Price return = +1
Ending quantity = 0
```

不得因为 opening quantity = 0 丢收益。

### FC-04 Foreign Holding

使用 Fixture B：

```text
Price +70 CNY
FX +22 CNY
```

Base / Native 都核对。

### FC-05 Interest vs Salary

相同 +500 cash：

Interest：

```text
Asset bucket        Dividend & Interest
Return component    Dividend & Interest
Dietz capital       none
```

Salary：

```text
Asset bucket        Income
Return component    none
Dietz capital       +500（cash included 时）
```

两者 Return % 必须不同。

### FC-06 Dividend

```text
cash_dividend 70
```

期望：

```text
Dividend & Interest = +70
```

不得自动假设 gross 100 / withholding 30。

Associated tax 可为 Investment Fee；无关联 tax 只属于 Asset Changes Fee。

### FC-07 Loan

Drawdown：

```text
Cash +100,000
Debt +100,000
Net worth change = 0
```

Principal repayment：

```text
Cash -10,000
Debt -10,000
Net worth change = 0
```

Cash interest：

```text
Cash -500
Debt principal unchanged
Spending -500
Net worth -500
```

---

## 17. Partial / Missing / Trust 状态

### 17.1 Missing Quote

制造一天缺 quote：

- 日历不得显示真实 `0 / 0%`。
- 显示 partial / unavailable。
- period banner 有 issue。
- issue list 有 missing reason。
- coverage 正确。
- Trend / Contribution completeness 一致。

### 17.2 Missing FX

同上。Base 依赖缺失 FX 时必须 partial/unavailable；Native 是否可用按实际 dependency 判断。

### 17.3 Poor Rate Coverage

例如：

```text
31 days
30 rated
```

期望：

```text
rate exists
ratedDays = 30
totalDays = 31
status = partial
```

若多数日期无 rate：

```text
period rate = —
```

### 17.4 Residual

制造 snapshot inconsistency：

```text
Price / FX 保持公式值
difference → Residual
status → partial
```

Base valuation 也必须能出现 Residual，不能吞进 FX。

---

## 18. Native / Base Currency

### 18.1 单币种 Instrument

Native：

```text
allowed
FX Impact = 0
```

### 18.2 Return 可 Native、Asset Changes 强制 Base

构造：

```text
USD instruments
+
CNY cash outside InvestmentUniverse
```

同一 stored valuation = Native：

```text
Return Analysis → Native allowed
Asset Changes   → forced Base
```

切 Tab 不得把 session valuation 改成 Base。

### 18.3 Multi-currency Portfolio

Native：

```text
forced Base
```

必须明确提示，不可静默转换。

---

## 19. Cross-page Navigation

Return Calendar → Asset Changes 传递：

```text
scope
scopeId
valuation
includeCash
moreFilters
from = selected day
to   = selected day
```

Change Driver → Return Analysis：

- date range preserved
- scope preserved
- filters preserved
- returnType appropriate

Contribution → History：

```text
from / to
accountId
instrumentId
kinds
```

Category → History：selected day + account/instrument。

Residual → History：精确到 residual day + dimensions。

---

## 20. Empty / Error / Loading

六个 Tab 均抽测。

### Loading

- title / tabs / filter chrome 不因 query reload 消失。
- 无明显 layout collapse。

### Error

- 统一 ErrorState。
- Retry 有效。
- 成功后恢复。

### Empty

覆盖：

```text
No History Origin
Scope requires selection
No investment assets
Insufficient history
No asset change data
No contribution data
No category data
```

文案必须区分。

---

## 21. State / Invalidation

### Session state

App session 内：

- secondary tab 切换不丢 shared filters。
- Return ↔ Asset Changes 保留兼容 filters。
- Calendar cursor 与 global date filter 分离。
- Contribution return type deep-link 正确。

App 重启恢复默认状态是 v1 允许行为。

### Data invalidation

P0：

1. 打开分析结果。
2. 修改 period 内一个 History Activity。
3. 回到分析。
4. 相同 query 结果必须变化。

再抽测：

- quote refresh
- FX refresh
- holding/account mutation
- snapshot rebuild

不得命中 stale memo。

---

## 22. Localization

六个 Tab 完整浏览：

```text
English
简体中文
繁体中文
```

重点术语：

```text
Return Analysis
Asset Changes
Return Calendar
Return Trend
Contribution
Change Drivers
Asset Trend
Categories
External Flows
Dividend & Interest
Price Change
FX Impact
Unexplained difference
Realized Gain
```

检查：

- 无 raw i18n key。
- UI 无 `P&L`。
- `Gain` / `Return` 不混用。
- 长中文不溢出。
- Sheet / Select / Chart title 不截断关键含义。

---

## 23. Accessibility / Keyboard

最低 release gate：

- Tabs 可键盘导航。
- filters 均有 label。
- Calendar day 有 focus state。
- Sheet 可 Escape 关闭。
- Sheet focus 合理。
- 图表有 accessible summary / table fallback。
- 正负不能只靠颜色区分。
- partial 不只靠黄色。
- warning 信息不依赖 hover 才能读取。

---

## 24. Visual QA

在 1280 和 1440 宽度检查：

- filter bar wrapping。
- Calendar 7 columns 不溢出。
- Year grid 4 → 3 → 2 columns。
- Sheet 宽度合理。
- 长 instrument/account name truncate。
- 负数和长金额不破坏布局。
- Waterfall labels 不重叠。
- Trend axis / tooltip 可读。
- Partial / forced-base badges 不遮挡标题。
- Empty state 不显示无意义空 chart。

长金额建议：

```text
CNY ¥12,345,678.90
USD $1,234,567.89
JPY ¥123,456,789
negative -¥12,345.67
```

---

## 25. Performance

架构预算：

| 场景 | 目标 |
|---|---:|
| Warm memo，任意 projection | < 100 ms |
| Cold compute，1 month，约 200 components | < 300 ms |
| Cold compute，3Y / All，约 500 components | < 3 s |
| Snapshot rebuild | 可超过 3 s，但 UI 不冻结 |

### PF-01 Warm tab switching

相同 filter 下 Calendar → Trend → Contribution、Drivers → Trend → Categories 应接近即时。

### PF-02 3Y / All

- App window 不冻结。
- filters / tabs 保持。
- loading feedback 可见。
- 无 beachball / unresponsive。

### PF-03 Filter exploration

连续切 Portfolio / Account / Instrument / Currency / Asset Class，检查：

- memory 不持续明显增长。
- 返回旧 query 时正常。
- 无 stale data。

### PF-04 Year Calendar

年视图不应表现为 12 个明显串行慢请求。

---

## 26. Regression

### Investments

- Holding Gain 正常。
- Account Gains 正常。
- 现有 realized/unrealized 显示不受影响。

### History

- 普通访问正常。
- analytics deep-link filters 正确。
- deep-link 后仍可修改 filters。

### Market Data

- quote / FX refresh 后 analytics invalidates。
- 无 stale result。

### Accounts / Portfolio

- 修改 account / holding 后分析更新。
- archived / deleted entities 不 crash。

### Startup / Restore

- backup restore 后可重建分析 snapshot。
- 无旧 memo 污染恢复后的数据库。

---

## 27. 已有自动化覆盖与人工 QA 的关系

仓库已经有较强自动化基础。

### Backend

现有 analysis tests 已覆盖：

- scope intersection
- internal transfer
- same-day open/close
- FX conversion
- foreign holding Price / FX
- cash FX formula
- deposit interest
- Modified Dietz
- salary vs interest
- IncludeCash
- DST day
- residual
- projections / grouping
- memo invalidation
- Wails DTOs

所以人工 QA 不需要重做每一个 decimal 单元测试；应验证：

> Go engine 的正确值是否完整穿过 Wails → React → Calendar / Chart / Sheet。

### Frontend

已有测试覆盖：

- Calendar partial vs zero
- Calendar day sheet
- Month / Year navigation
- Calendar → Asset Changes
- empty states
- Return Trend display / named ranges
- Contribution details / History
- Realized view restrictions
- Asset Trend granularity / metric
- Categories detail / History
- forced-base
- Change Drivers waterfall / residual
- Drivers → Return Analysis
- residual → History
- invalidation 后 Sheet refresh

桌面 QA 应重点发现 integration、native shell、真实数据边界、visual、locale、performance 和 stale-state 问题。

---

## 28. 缺陷等级

### Blocker

- App crash / 无法启动。
- Analytics 导致数据损坏。
- financial identity 明显错误。
- Return % 明显错误（如 1.82% 显示 10%）。
- missing data 被显示为真实 0。
- stale memo 无法刷新。

### P0 / Critical

- Price / FX 错误归因。
- Internal transfer 被当收益。
- Scope semantics 错误。
- Dividend / Fee double count。
- Liability sign 反。
- forced-base 无提示。
- cross-navigation 到错误范围。

### P1 / Major

- 某 Tab 不可用。
- filters 不工作。
- partial / residual 不可追踪。
- year/month aggregation 错误。
- History deep-link filters 缺失。
- 3Y / All 明显冻结。

### P2 / Minor

- 文案。
- spacing / visual。
- 次要 hover / tooltip。
- 非关键 keyboard polish。

---

## 29. Release Exit Criteria

- [ ] `wails3 task check` 全绿。
- [ ] Analysis engine golden / projection tests 全绿。
- [ ] P0 Smoke 全通过。
- [ ] 四组基准 fixture 数值核对通过。
- [ ] Portfolio / Account / Instrument 三种 scope 通过。
- [ ] Base / Native / forced-base 通过。
- [ ] Include / Exclude Cash 通过。
- [ ] 六个 Tab 通过。
- [ ] Calendar / Drivers / Contribution / Categories 四种 Sheet 通过。
- [ ] Return ↔ Asset Changes ↔ History 通过。
- [ ] Partial / unavailable / residual / missing quote / missing FX 通过。
- [ ] English / zh-CN / zh-TW 无 blocker。
- [ ] 1280 / 1440 visual smoke 通过。
- [ ] 3Y / All 无不可接受冻结。
- [ ] 修改 Activity / Quote / FX 后无 stale analytics。
- [ ] Investments / History regression 通过。
- [ ] 无 Blocker / P0。
- [ ] P1 均修复或明确接受。

---

## 30. 推荐实际执行顺序

```text
1. wails3 task check

2. P0 desktop smoke
   Return Analysis
   Asset Changes
   six tabs
   sheets
   navigation

3. Fixed financial fixtures
   Modified Dietz
   Price / FX
   transfer scope
   interest / dividend
   loan

4. Shared filters
   Scope
   Valuation
   Include Cash
   Date
   More Filters

5. Trust states
   partial
   missing quote
   missing FX
   residual
   forced base

6. Drill-down
   Calendar → Drivers
   Drivers → Contribution
   Contribution / Categories / Residual → History

7. Localization + keyboard + visual

8. 3Y / All performance

9. Regression
   Investments
   History
   Market Data
   account / holding mutation
```

---

## 31. Repository References

本测试方案按当前 `main` 实现整理，重点参考：

```text
frontend/src/features/insights/
  ReturnAnalysisPage.tsx
  ReturnCalendarTab.tsx
  ReturnTrendTab.tsx
  ContributionTab.tsx
  AssetChangesPage.tsx
  ChangeDriversTab.tsx
  AssetTrendTab.tsx
  CategoriesTab.tsx
  AnalysisFilterBar.tsx

frontend/src/features/insights/
  ReturnAnalysisPage.test.tsx
  AssetChangesPage.test.tsx
  ReturnCalendarTab.test.ts
  insightTabs.test.tsx
  WaterfallChart.test.ts
  analysisRequest.test.ts

frontend/src/stores/analysis.ts
frontend/src/app/navigation.ts

internal/application/
  analysis_*.go
  analysis_*_test.go

docs/design/analytics/
  analytics-product-design.md
  analytics-wireframes.md
  analytics-redesign-architecture.md
  analytics-development-plan.md
```
