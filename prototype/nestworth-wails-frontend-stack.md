# Nestworth Wails 前端技术方案

## 1. 目标

Nestworth 是一个桌面端、local-first 的家庭资产负债管理应用。前端需要重点支持：

- 高信息密度的桌面界面；
- 账户、投资持仓、历史记录等复杂表格；
- 多步骤和渐进式表单；
- 净资产、收益和历史趋势图表；
- Light / Dark Mode；
- 简体中文、繁体中文和英文；
- 键盘操作；
- 清晰表达 Manual、Remote、Stale、Unavailable 等数据状态；
- 与 Go 后端直接通信，不引入额外 HTTP API。

因此前端方案应优先考虑成熟度、可维护性、桌面交互能力和设计自由度，而不是 Web SSR 或服务端渲染能力。

---

## 2. 推荐技术栈

推荐组合：

```text
Wails
+ Go
+ React
+ TypeScript
+ Vite
+ Tailwind CSS
+ shadcn/ui
+ TanStack Query
+ TanStack Table
+ Zustand
+ React Hook Form
+ Zod
+ Apache ECharts
+ Lucide React
+ date-fns
+ i18next
```

建议使用 `pnpm` 管理前端依赖。

---

## 3. 核心技术选择

### 3.1 React + TypeScript

推荐作为 Nestworth 的主要前端框架。

原因：

- Wails 对 React + TypeScript 支持成熟；
- 组件生态丰富；
- 适合复杂表格、Drawer、Dialog、Popover、多步骤表单等桌面应用交互；
- 与 TanStack、shadcn/ui 等库组合成熟；
- TypeScript 可以很好地承接 Wails 生成的 Go bindings 和数据模型。

Wails 项目可以从 React + TypeScript 模板开始：

```bash
wails3 init -n nestworth -t react
```

(Verified against Wails v3.0.0-beta.12: the template name is `react`, not
`react-ts`; TypeScript is that template's default language. See
[technical design, Phase 0 spike findings](../docs/migration/wails-v3-technical-design.md#phase-0-spike-findings-recorded-not-duplicated-elsewhere).)

前端不需要额外建立 REST API。

调用链保持为：

```text
React
  ↓
Wails generated bindings
  ↓
Go Service
  ↓
Domain / Repository
  ↓
SQLite
```

Go 继续作为业务规则和数据正确性的最终 authority。

---

### 3.2 Vite

使用 Wails React 模板自带的 Vite 即可。

不建议引入：

- Next.js
- SSR
- Server Components
- TanStack Start

这些能力对于 Wails 桌面 SPA 基本没有实际收益。

---

## 4. UI 层

### 4.1 shadcn/ui

推荐作为基础组件体系。

主要用途：

- Button
- Input
- Select
- Dialog
- Sheet / Drawer
- Dropdown Menu
- Popover
- Tooltip
- Tabs
- Command
- Alert Dialog
- Checkbox
- Switch
- Form controls

优势是它不会像 Ant Design 或 Material UI 那样强制 Nestworth 使用固定视觉风格。

组件代码直接存在项目中：

```text
src/components/ui/
├── button.tsx
├── dialog.tsx
├── input.tsx
├── select.tsx
├── sheet.tsx
├── tabs.tsx
└── ...
```

因此可以逐步形成自己的：

```text
Nestworth Design System
```

建议使用 shadcn 当前默认的 Base UI primitives。

---

### 4.2 Tailwind CSS

推荐与 shadcn/ui 一起使用。

Tailwind 主要用于：

- layout；
- spacing；
- typography；
- responsive layout；
- dark mode；
- design token；
- 状态颜色。

建议尽早建立 Nestworth 自己的 semantic tokens，例如：

```text
background
foreground
card
border
muted

primary
success
warning
destructive

gain-positive
gain-negative

data-current
data-manual
data-remote
data-stale
data-unavailable
```

不要在业务组件中大量直接写具体颜色。

---

### 4.3 Lucide React

推荐作为统一图标库。

适合 Nestworth 的常用图标包括：

```text
House
Wallet
Landmark
CreditCard
ChartNoAxesCombined
History
ArrowRightLeft
Users
Building2
Folder
Settings
RefreshCw
CircleAlert
Archive
```

不建议同时混用多套 icon library。

---

## 5. 数据获取与状态管理

### 5.1 TanStack Query

推荐用于管理所有来自 Go backend 的数据。

虽然 Wails 不使用 HTTP API，但 TanStack Query 仍然很有价值。

例如：

```tsx
useQuery({
  queryKey: ["accounts"],
  queryFn: () => ListAccounts(),
})
```

这里的 `ListAccounts()` 是 Wails 生成的 Go binding。

建议由 TanStack Query 管理：

```text
Accounts
Overview
History
Investments
Holdings
Market Data
Analytics
```

它统一解决：

```text
loading
error
data
refetch
mutation
cache invalidation
```

例如更新账户余额后：

```text
UpdateAccountValue()
        ↓
invalidate ["accounts"]
invalidate ["overview"]
invalidate ["history"]
```

这样对应页面可以自动刷新。

---

### 5.2 Zustand

仅用于前端 UI 状态。

推荐管理：

```text
sidebarCollapsed
selected filters
current drawer
temporary UI selections
Record Change draft
window-level preferences
```

不建议把 Accounts、Holdings、History 等后端数据复制进 Zustand。

职责划分建议：

```text
Go / SQLite
    ↓
TanStack Query
    ↓
Backend/domain state

Zustand
    ↓
Frontend-only UI state
```

---

## 6. 表格

### TanStack Table

推荐作为 Accounts、Holdings、History 等复杂数据表格的底层实现。

适合：

- sorting；
- filtering；
- column visibility；
- selection；
- resizing；
- pagination；
- custom cell rendering。

典型场景：

```text
Accounts

Account        Owner        Type         Value       Status
DBS Savings    Shared       Cash         S$96,200    Stale
Moomoo SG      Walt         Investment   S$226,010   Current
Mortgage       Shared       Liability   -S$51,100    Current
```

以及：

```text
Holdings

Instrument   Qty   Avg Cost   Current Value   Unrealized Gain
NVDA          96   $131.70    S$31,820        +S$14,380
QQQ           83   $430.12    S$55,900         +S$9,140
```

TanStack Table 是 headless library，因此表格外观仍然完全由 Nestworth 控制。

---

## 7. 图表

### Apache ECharts

推荐作为主要图表库。

适合 Nestworth 后续需求：

- Net Worth Trend；
- Asset / Liability；
- Historical Value；
- Investment Gain；
- Instrument Movement；
- Currency Movement；
- External Flow；
- 多时间范围切换；
- Tooltip；
- Zoom；
- 多 series。

建议自己封装一个轻量 React wrapper：

```tsx
<EChart option={option} />
```

不必为了 React 再额外引入复杂封装层。

---

## 8. 表单

### React Hook Form + Zod

推荐组合使用。

适合：

#### Account

```text
Asset / Liability
Name
Category
Currency
Initial Value
Value Date
Institution
Group
Ownership
```

#### Transfer

```text
From Account
To Account
From Amount
To Amount
Date
Fee
```

#### Buy / Sell

```text
Account
Instrument
Quantity
Price
Fee
Date
```

React Hook Form 负责：

- field state；
- validation state；
- dynamic fields；
- nested forms；
- dirty state；
- submit lifecycle。

Zod 负责前端输入 schema。

但是：

> Zod 不能替代 Go domain validation。

例如：

```text
Ownership total == 100%
Amount > 0
Currency consistency
Transfer semantics
Investment cost calculation
```

这些规则仍应在 Go 后端再次验证。

---

## 9. 路由

Nestworth 不需要 Web 风格的复杂路由系统。

第一阶段甚至可以使用简单页面状态。

如果页面逐渐增加，可以引入 React Router，并建议使用：

```text
MemoryRouter
```

推荐路由结构：

```text
/overview

/accounts
/accounts/:accountId

/history
/history/:activityId

/investments
/instruments/:instrumentId

/settings
```

没有必要依赖浏览器真实 URL。

---

## 10. 日期和国际化

### date-fns

负责：

- 日期格式化；
- 时间范围计算；
- 相对时间；
- Timeline 日期显示。

不要在 UI 中手工拼接日期字符串。

### i18next + react-i18next

负责：

```text
English
简体中文
正體中文
```

建议从开发初期就避免：

```tsx
<Button>Add Account</Button>
```

散落在组件中。

统一使用：

```tsx
t("account.add")
```

避免后期重构整个 UI。

---

## 11. Toast 和反馈

推荐直接使用 shadcn 体系兼容的 Sonner。

适合：

```text
Account created
Value updated
Change recorded
Settings saved
Market refresh failed
History recalculated
```

对于高风险操作，例如：

```text
Undo
Fix
Archive
Restore
```

不要只使用 Toast。

应该采用：

```text
Alert Dialog
+
明确影响范围
+
执行后 Toast
```

---

## 12. 推荐依赖

核心依赖可以控制在：

```bash
pnpm add   @tanstack/react-query   @tanstack/react-table   zustand   react-hook-form   zod   echarts   lucide-react   date-fns   i18next   react-i18next   sonner
```

UI：

```bash
pnpm add tailwindcss @tailwindcss/vite
pnpm dlx shadcn@latest init
```

按需添加 shadcn components，而不是一次性引入所有组件。

---

## 13. 推荐目录结构

建议采用 feature-oriented architecture：

```text
frontend/
├── src/
│   ├── app/
│   │   ├── App.tsx
│   │   ├── AppShell.tsx
│   │   ├── navigation.ts
│   │   └── providers.tsx
│   │
│   ├── features/
│   │   ├── onboarding/
│   │   ├── overview/
│   │   ├── accounts/
│   │   ├── history/
│   │   ├── investments/
│   │   └── settings/
│   │
│   ├── components/
│   │   ├── ui/
│   │   ├── finance/
│   │   ├── charts/
│   │   └── layout/
│   │
│   ├── queries/
│   │   ├── accounts.ts
│   │   ├── overview.ts
│   │   ├── history.ts
│   │   └── investments.ts
│   │
│   ├── stores/
│   │   └── ui.ts
│   │
│   ├── hooks/
│   ├── lib/
│   ├── i18n/
│   └── bindings/
│
└── package.json
```

不要一开始按这种方式组织：

```text
components/
pages/
services/
utils/
```

因为随着 Nestworth domain 增长，会很快出现大量跨目录跳转。

---

## 14. 与 Go 后端的职责边界

### Go 负责

```text
SQLite
Repository
Domain model
Financial calculations
Decimal precision
Ownership validation
Activity semantics
History replay
Investment cost basis
FX conversion
Market data
Business validation
```

### React 负责

```text
Layout
Navigation
Forms
Tables
Charts
Filtering
Presentation state
Loading / Error states
Interaction
Keyboard UX
```

前端不得复制核心财务计算逻辑。

尤其不要在 TypeScript 中重新实现：

```text
Net Worth calculation
Ownership allocation
Cost basis
Realized gain
FX gain decomposition
History replay
```

这些应统一来自 Go。

---

## 15. 不推荐的方案

### Ant Design

不推荐。

原因：

- Web / SaaS 产品风格过强；
- 视觉定制成本较高；
- 很难形成 Nestworth 自己的 macOS 桌面产品风格。

### Material UI

不推荐。

Material Design 的产品语言过于明显。

### Bootstrap

没有必要。

### Redux Toolkit

目前 Nestworth 不需要如此重量级的前端 state architecture。

TanStack Query + Zustand 已经足够。

### Next.js

Wails 环境中没有明显收益。

### Axios

没有 REST API 时基本没有必要。

### AG Grid

目前 Nestworth 的数据规模和表格复杂度还不需要承担 AG Grid 的重量。

优先使用 TanStack Table。

---

## 16. 最终推荐

Nestworth 的推荐前端方案为：

```text
Wails
└── Go
    ├── Domain
    ├── Service
    ├── Repository
    ├── SQLite
    ├── Market Data
    └── Wails Bindings

Frontend
└── React + TypeScript
    ├── Vite
    ├── Tailwind CSS
    ├── shadcn/ui + Base UI
    ├── TanStack Query
    ├── TanStack Table
    ├── Zustand
    ├── React Hook Form
    ├── Zod
    ├── Apache ECharts
    ├── Lucide React
    ├── date-fns
    └── i18next
```

这是一个比较适合 Nestworth 的组合：

- UI 表达能力明显高于 Fyne；
- 保留 Go 作为核心业务实现；
- 不引入额外 HTTP 层；
- 前端生态成熟；
- 可以实现接近原生 macOS 的高质量桌面 UI；
- 对后续 Accounts、History、Investments 和 Analytics 的复杂度有足够扩展空间；
- 不会过早引入 SSR、Redux、AG Grid 等不必要的重量级基础设施。
