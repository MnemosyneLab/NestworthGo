# 可用资金原生对照

Mac mini，macOS 27.2 arm64，Go 1.27.4。Wails 和基线 Gio 来自 `d75b6c5909fd0c46299835a5a557cfbbe0283b5c`，最终 Gio 来自 `f8abc58c0c9970606625a8c59ad9d740182972cd`。Wails 使用原生 WebView 和 Go 后端，复用同源的已生成生产前端。两个已签名 QA 包分别使用 `com.nestworth.dev` 和 `com.nestworth.gio`。

每组应用使用同一个服务生成种子的独立副本，启动前核对包标识、源码版本、账本和设置路径，以及 `Synthetic funds` 标记。两端均为简体中文，账本时区 UTC，本位币 USD。预览日期为 2026-11-28。本轮使用本地种子的可复算金额。

## 首屏原始截图

| 窗口和主题 | Wails 基线 | Gio 基线 | 最终 Gio |
| --- | --- | --- | --- |
| 1280×800 浅色 | [Wails](native/base-wails-wide-light.jpg) | [基线](native/base-gio-wide-light.jpg) | [最终](native/fixed-gio-wide-light.jpg) |
| 1280×800 深色 | [Wails](native/base-wails-wide-dark.jpg) | [基线](native/base-gio-wide-dark.jpg) | [最终](native/fixed-gio-wide-dark.jpg) |
| 900×600 浅色 | [Wails](native/base-wails-narrow-light.jpg) | [基线](native/base-gio-narrow-light.jpg) | [最终](native/fixed-gio-narrow-light.jpg) |
| 900×600 深色 | [Wails](native/base-wails-narrow-dark.jpg) | [基线](native/base-gio-narrow-dark.jpg) | [最终](native/fixed-gio-narrow-dark.jpg) |

原始 JPEG 保持原字节。Gio 宽／窄截图为 2560×1600／1800×1200，Wails 为 2558×1600／1798×1200，含各自原生标题栏。

## 业务与交互验收

| 用例 | 实际结果 | 原始证据 |
| --- | --- | --- |
| 今日、7 日、30 日、自定义 | 两端从今日开始，实际选择过 7 日并打开自定义日历。最终 Gio 依次选 7 日、30 日、2026-12-11 和今日，当前选择和对应日期一致。部分覆盖时，正常短窗是 USD 85.3071，30 日是 USD 125.3071。画面按币种精度显示 85.31 和 125.31。 | [Wails 自定义](native/base-wails-custom.jpg) · [Gio 自定义](native/fixed-gio-custom.jpg) |
| 考虑提前支取 | 合成定存 2026-12-12 到期。提前 USD 40 扣 USD 3，净额 USD 37，短窗已确认小计变为 USD 122.3071。正常到期 USD 42 扣 USD 2，30 日仍是 USD 125.3071。最终 Gio 关闭开关后，自定义日期恢复 85.31。提前开关现在公布原生复选状态。 | [Wails 提前](native/base-wails-early.jpg) · [Gio 提前](native/fixed-gio-early.jpg) |
| 覆盖与原币 | 部分覆盖样例缺 EUR→USD 汇率且有未报价持仓。EUR 原币可用额仍为 25.25，USD 已确认小计与预留分列。窄窗两个币种都无需滚动。另用只有未报价持仓的种子，两端今日、7 日、30 日都显示「未知」。规则补齐后两端今日完整总额 USD 140.2983、30 日 USD 180.2983。真实零和小于最小显示单位的非零金额另有定向测试。 | [Wails 未知](native/base-wails-unavailable.jpg) · [Gio 未知](native/fixed-gio-unavailable.jpg) |
| 来源、规则和帮助 | 「1 项待补可用规则」定位到该日期的 QA-FUND，显示 Hill Brokerage 和状态。Gio 的名称旁保留补充规则，其他动作收进菜单。Escape 关闭菜单后焦点返回触发按钮。取消规则回原行。两端实际保存「随时可用」，保留的筛选显示 0 项，可用「显示全部」恢复。金额帮助和来源原币帮助各自展开，Escape 回到入口。「查看项目」筛选及修复跳转另有定向测试。 | [Wails 窄窗规则](native/base-wails-narrow-rules.jpg) · [Gio 窄窗规则](native/fixed-gio-narrow-rules.jpg) · [Gio 来源菜单](native/fixed-gio-narrow-source-menu.jpg) · [Wails 金额帮助](native/base-wails-amount-help.jpg) · [Gio 金额帮助](native/fixed-gio-amount-help.jpg) · [Gio 原币帮助](native/gio-source-amount-help.jpg) · [Wails 规则保存](native/wails-rule-saved.jpg) · [Gio 规则保存](native/gio-rule-saved.jpg) |
| 预留新建、编辑、释放 | 两端在 Harbor Savings 新建 USD 10.125，编辑为 USD 12.75，然后释放。Gio 新建取消回到「添加预留」，Escape 退出时回到「管理预留」。保存第一笔后 Gio 今日显示 USD 130.17。取消一次释放保留记录，重新确认释放，今日恢复 USD 140.30。两端退出重开后仍在已释放历史中，持久记录 rev 3。 | [Wails 新建](native/wails-reserve-created.jpg) · [Gio 精确预览](native/gio-reserve-preview.jpg) · [Wails 编辑](native/wails-reserve-edited.jpg) · [Gio 编辑](native/gio-reserve-edited.jpg) · [Gio 释放确认](native/gio-reserve-release-preview.jpg) · [Wails 重开](native/wails-reopened-released-history.jpg) · [Gio 重开](native/gio-reopened-released-history.jpg) |
| 登记已有产品 | 窄窗初开已显示 Hill Brokerage、合同币种 USD 和可用现金 0。Gio 的 Tab 到录入方式，Escape 回到「添加产品」。两端均以同一录入方式登记定存，本金、成本、现值均 USD 10.125，起始日 2026-11-28，到期日 2026-12-19，并明确确认现金不含该产品。两端退出重开后均找到产品。Gio 通过该行菜单打开准确合同，返回后保持全量筛选及原菜单焦点。 | [Wails 窄窗](native/base-wails-narrow-product.jpg) · [Gio 窄窗](native/fixed-gio-narrow-product.jpg) · [Wails 预览](native/wails-product-registered-preview.jpg) · [Gio 预览](native/gio-product-registered-preview.jpg) · [Gio 返回焦点](native/gio-product-return-focus.jpg) |

退出后的 SQLite 配对使用只读连接。两份业务副本中的原有 5 笔活动、5 条活动效果、2 个持仓、现金和金额观测以及市场报价逐字段一致。两端都新增同一个标的的 `on_request` 规则，费用 0。新预留日志各有一次创建、一次编辑、一次释放，末值是 USD 12.75。初次 USD 10.125 来自实际录入和原生预览，最终主表只保存末值。两端各登记一个 `record_existing`，请求及哈希一致，只产生一个新持仓的数量 +1 效果，保留同样的 USD 10.125 估值报价，增量现金效果为零。

Gio 保留期限区的来源快捷入口、本位币汇总，以及在规则、产品和预留里的校验。Gio 新建和编辑预留先预览，释放有确认步骤，并保留已释放历史。窄窗实测保留方案计算、来源身份和键盘返回路径。

## 检查范围

macOS 定向测试通过，覆盖 Gio 可用资金、概览和待办入口、登记产品、日期选择与四种语言。共享领域、应用和 Wails 流动性适配器的相关测试通过。两款原生基线和最终 Gio 签名构建均成功。扩展的 macOS 混合字体回退测试在本机失败，基线复现相同的 PingFang 字重和 Hiragino 字形选择差异。

原生保存覆盖表中的一条普通规则、一款已有定存、一次预留完整生命周期。原生未重跑即时开立、续作、到期结算以及所有自定义日期边界。Linux 原生窗口、VoiceOver 实际朗读和云端报告附件未复验。
