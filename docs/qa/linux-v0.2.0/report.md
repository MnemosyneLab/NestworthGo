# Nestworth v0.2.0 Linux 手工测试报告

- **产品**: Nestworth（Wails v3 + Go + React）
- **版本**: v0.2.0 · Build 1
- **Bundle ID**: `com.nestworth.app`
- **测试日期**: 2026-08-26
- **测试环境**: Ubuntu 24.04 LTS (Noble), linux/amd64, X11
- **测试对象**: `wails3 task linux:build` 产出的生产二进制 `bin/nestworth`
- **数据隔离**: 全新目录（`NESTWORTH_DATABASE_PATH` + `XDG_CONFIG_HOME`），从零初始化
- **测试员**: Cursor Cloud Agent（GUI 手工操作，非单元测试）

录像（Onboarding → Directory → 三个账户）：

[linux_onboarding_directory_accounts.mp4](linux_onboarding_directory_accounts.mp4)

## 1. 结论

Linux 生产包可以编译、打包并启动。从 **首次初始化（Onboarding）** 开始，成员 / 机构 / 分组 / 账户 / 投资标的与持仓 / History 起点 / 入金 / Overview / Analytics / Market data / Settings / 简体中文切换 **均可走通**。

**阻塞级问题 1 个**：History 的 **Buy or sell（买入）** 预览失败，无法完成一笔股票成交。其余主路径通过。

综合判定：**Linux 构建可用，核心资产负债表流程可用，投资成交（Trade）不可用。**

| 模块 | 结果 |
| --- | --- |
| Linux 编译 / 打包 | **PASS** |
| 冷启动 + Onboarding | **PASS** |
| Directory（Members / Institutions / Groups） | **PASS** |
| Accounts（现金 / 持仓投资账户 / 负债） | **PASS** |
| Investments（Instrument / Holding） | **PASS**（创建成功；成交未完成） |
| History 启动 + 入金 | **PASS** |
| History 买入成交 | **FAIL** |
| Overview / Analytics / Market data / Settings / i18n | **PASS** |

## 2. 编译与打包

### 2.1 环境

| 组件 | 版本 |
| --- | --- |
| OS | Ubuntu 24.04.4 LTS, linux/amd64 |
| Go | go1.26.0 |
| Node / pnpm | v22.14.0 / 10.33.3 |
| Wails CLI | v3.0.0-beta.12 |
| GTK / WebKit | gtk4 4.14.5, webkitgtk-6.0 2.52.3 |
| `wails3 doctor` | SUCCESS — ready for Wails development |

构建命令：

```bash
wails3 task linux:build
wails3 task linux:create:deb
wails3 task linux:create:rpm
wails3 task linux:create:aur
wails3 task linux:create:appimage
```

生产 `ldflags`：`-X ...Version=v0.2.0 -X ...Build=1`，tags `production`，strip。

### 2.2 产物

二进制与安装包留在构建机上，未纳入本仓库。当时测得的摘要：

| 文件 | 大小 | SHA-256 |
| --- | --- | --- |
| `nestworth-0.2.0-linux-amd64` | 18 MB | `938b9929174c69f60c1b5d2aace85d954791657f8a1dc00a8fe9961aeaa6108f` |
| `nestworth-0.2.0-linux-amd64.deb` | 7.9 MB | `a902268754657f8071df9ec70edee814c81ecfffb0640464378479236be1159f` |
| `nestworth-0.2.0-linux-amd64.rpm` | 8.2 MB | `c34118230bd25db0f7c5e9b2a5340058b1bd8fe159d3e77dfd4828846ff4f91c` |
| `nestworth-0.2.0-linux-amd64.pkg.tar.zst` | 8.0 MB | `bd39f02b7f5448ba517b092b8b1280456be36dcc8d9b348edb92f5cb94b180f8` |
| `nestworth-0.2.0-linux-amd64.AppImage` | 80 MB | `c2255c8a0a162a70b3b5a03e76b6df85e350492e4c462a6edf52f2c97a9bb1b1` |

二进制：`ELF 64-bit LSB executable, x86-64, stripped`。  
动态库：`libgtk-4.so.1`、`libwebkitgtk-6.0.so.4`，`ldd` 无缺失。  
`.deb` 元数据：`Package: nestworth`、`Version: 0.2.0-1`、`Architecture: amd64`、`Depends: libgtk-4-1, libwebkitgtk-6.0-4`、`License: MIT`。

手工测试运行的是 **未安装的本地 ELF**，未把 `.deb` / AppImage 再装进系统。

### 2.3 启动现象

- 窗口标题 `Nestworth`，侧栏显示 `v0.2.0`
- Settings / About：`v0.2.0 · Build 1`、`com.nestworth.app`、MIT
- 日志仅有无 GPU 加速的 EGL 警告（`DRI3 error`），不阻止 WebView 渲染
- 冷启动直接进入 Onboarding（无 Household）

## 3. 测试数据

| 字段 | 值 |
| --- | --- |
| Household | Chen Household |
| Base currency | USD |
| Members | Alice（onboarding）、Bob（onboarding）、Carol（Directory 追加） |
| Institution | Chase Bank |
| Group | Daily cash |
| Checking | Cash & equivalents / Bank account / 初始 $5,000 / 所有者 Alice / Chase Bank / Daily cash |
| Brokerage | Investment / Brokerage account / Holdings / Include in investment / 所有者 Alice |
| Credit Card | Liability / Credit card / 初始 $0 / 所有者 Alice |
| Instrument | NVIDIA / Stock / USD / Manual |
| Holding | Brokerage × NVIDIA × quantity 0 |
| History | timezone UTC，然后入金 Checking $1,000 Contribution |

## 4. 分步结果

### 4.1 初始化 / Onboarding — PASS

冷启动展示 “Start your Household”。填写 Household、USD、Alice + Add member → Bob，点 **Get started**。成功进入主壳，默认落在 Accounts 空态 “No accounts yet”。

![Onboarding empty form](page_onboarding.webp)

![Onboarding filled form](onboarding_form_filled.webp)

![Accounts empty after onboarding](page_accounts_empty.webp)

### 4.2 Directory：成员 / 机构 / 分组 — PASS

![Directory members](page_directory_members.webp)

![Carol added](directory_member_carol_added.webp)

![Directory institutions](page_directory_institutions.webp)

![Directory groups](page_directory_groups.webp)

| 步骤 | 结果 |
| --- | --- |
| Members 已有 Alice、Bob | PASS |
| 追加 Carol | PASS，toast “Carol added” |
| Institutions 新增 Chase Bank | PASS，toast “Chase Bank added” |
| Groups 新增 Daily cash | PASS，toast “Daily cash added” |

### 4.3 Accounts — PASS

![Empty account form](account_form_empty.webp)

![Checking form](account_form_checking.webp)

![Brokerage form](account_form_brokerage.webp)

![Credit Card form](account_form_credit_card.webp)

![Accounts table with three accounts](page_accounts_populated.webp)

| 账户 | 类别 | 现值 | 结果 |
| --- | --- | --- | --- |
| Checking | Cash & equivalents | **$5,000.00** | PASS，Details 绑定 Chase Bank + Daily cash |
| Brokerage | Investment / Holdings | No current value | PASS |
| Credit Card | Liability | **$0.00** | PASS |

### 4.4 Investments — PASS（创建）

![Instruments empty](page_investments_instruments_empty.webp)

![NVIDIA instrument form](instrument_form_nvidia.webp)

![NVIDIA instrument added](page_investments_instrument_added.webp)

![Holdings empty](page_investments_holdings_empty.webp)

![Add holding form](holding_form.webp)

![NVIDIA holding quantity 0](page_investments_holding_added.webp)

| 步骤 | 结果 |
| --- | --- |
| 添加 NVIDIA（Stock / USD / Manual） | PASS |
| 添加 Holding：Brokerage + NVIDIA + qty 0 | PASS |
| 持仓表 | NVIDIA / Brokerage / 0 / Cost $0.00 |

### 4.5 History — 部分 FAIL

Onboarding **未传 timezone**，因此 History 尚未开始（符合产品设计）。

![Start history](page_history_start.webp)

![Empty timeline after start](page_history_empty_timeline.webp)

![Money added form](history_money_added_form.webp)

![Money added preview](history_money_added_preview.webp)

![Contribution recorded](page_history_activity.webp)

![Trade currency error](history_trade_currency_error.webp)

![Trade invalid error](history_trade_invalid_error.webp)

| 步骤 | 结果 |
| --- | --- |
| Start history | PASS |
| Money added $1,000 to Checking | PASS，预览 Checking **$6,000.00** |
| Buy or sell NVIDIA | **FAIL** |

买入失败两次：

1. 首次 Preview：`Currency: This value is not valid.` 货币框视觉上已是 USD，但 `emptyChangeRequest()` 只默认 `currency`，不默认 `grossCurrency` / `feeCurrency`。
2. 手工重填 USD 后再 Preview：`This trade is not valid.` 持仓数量仍为 0，净值仍为 $6,000。

### 4.6 Overview — PASS

![Overview net worth 6000](page_overview.webp)

- 3 active accounts，徽章 **Complete**
- **Net worth $6,000.00**（Assets $6,000.00 / Liabilities $0.00）
- Recent activity: Added $1,000.00 to Checking (Contribution)
- By member Alice 100%；By institution Chase Bank 100%；By group Daily cash 100%

### 4.7 Analytics — PASS

![Analytics trend](page_analytics.webp)

- Net-worth trend：单点 2026-08-26 ≈ $6,000
- Realized gain：`No closed-day snapshots are available yet.`（预期）

### 4.8 Market data — PASS

![Market data before refresh](page_market_data.webp)

![Market data after refresh](page_market_data_refreshed.webp)

- Refresh market data：`1 targets: 0 updated, 0 reused, 1 skipped, 0 failed`
- Instrument quote = **Skipped**（NVIDIA 为 Manual，预期）

### 4.9 Settings + i18n — PASS

![Settings English](page_settings.webp)

![Settings Simplified Chinese](page_settings_zh_cn.webp)

- About：v0.2.0 · Build 1、MIT、`com.nestworth.app`
- 顶栏切到简体中文后侧栏与设置文案已翻译，再切回 English 成功

## 5. 缺陷

### BUG-1（High）History Buy or sell 无法预览

**复现**

1. 完成 Onboarding，建 Holdings 账户 + Manual 标的 + qty 0 持仓
2. Start history
3. Record change → Buy or sell，填 Brokerage / NVIDIA / Buy / 10 / 1000 / 5
4. Preview

**现象**

- 第一次：`Currency: This value is not valid.`（货币框显示 USD）
- 强制重填 USD 后：`This trade is not valid.`

**代码线索**

- `frontend/src/features/history/activityToCommand.ts` 的 `emptyChangeRequest()` 只设 `currency: "USD"`，Trade 走 `grossCurrency`
- `RecordChangeForm` 用 `?? "USD"` 显示，未写入 state
- `internal/domain/change.go` `buildTrade`：Holding / Instrument / quantity / gross 必须匹配；空 `holdingId` 时买入会尝试新建持仓，而账户里已有同标的持仓

建议：Trade 默认 `grossCurrency`（及 fee）为本位币；已有同账户同标的持仓时提交其 ID；预览错误带出后端 `field` + message。

## 6. 未覆盖

- Archive / Restore、改名、原生图片选择器
- qty>0 持仓的 unit cost
- Cash transfer / FX / Debt / Undo / Fix
- Yahoo 实价
- 安装 `.deb` / 运行 AppImage 的二次启动
- 键盘无障碍、签名

## 7. 总评

Wails v3 Linux 生产构建在 Ubuntu 24.04 + GTK4/WebKitGTK 6.0 上可打包、可启动。家庭初始化、名录、多类型账户、投资标的/持仓、History 起点与入金、总览分解、分析图、行情刷新、设置与中文界面均达到可演示质量。投资买入是当前唯一阻断完整“投资闭环”的问题，应在发布前修复 BUG-1。
