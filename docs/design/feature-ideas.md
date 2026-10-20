# Nestworth 功能提案：处置索引

此表将 2026-09-20 提案逐项归类，避免把旧讨论误读为当前需求或承诺。状态以当前实现和文档为准；未来方向只由[产品路线图](../product/roadmap.md)维护。未列出明确依据的项目不应据此实现。

| # | 当前处置 | 依据或说明 |
| --- | --- | --- |
| 1 | 已交付 | Overview 趋势与导航见 [家庭导航与 Data Health](household-navigation-and-health.md)。 |
| 2 | 部分支持 | 持仓当前收益已存在；期间末未实现收益仍不支持，见 [分析架构](analytics/analytics-redesign-architecture.md)。 |
| 3 | 已说明边界 | 财务口径见 [领域模型](../architecture/domain-model.md)；独立 UX 解释未承诺。 |
| 4 | 暂缓 | 未承诺统一导入；目前只维护版本化 JSON 导出契约，见 [导出格式](json-export-format.md)。 |
| 5 | 已有能力 | 币种与资产类型分析范围见 [分析产品设计](analytics/analytics-product-design.md)。 |
| 6 | 暂缓 | 成员份额个人回报没有当前定义或承诺。 |
| 7 | 部分支持 | 日期格式可设置；周起始与家庭名称编辑未提供；基础币种保持不可变。 |
| 8 | 已交付 | 归档与恢复路径见 [家庭导航与 Data Health](household-navigation-and-health.md)。 |
| 9 | 已交付 | 账户与持仓范围导航见 [家庭导航与 Data Health](household-navigation-and-health.md)。 |
| 10 | 不列入路线图 | 未定义具体用户需要，不新增账户字段。 |
| 11 | 未承诺 | 加持仓时创建标的尚无路线图承诺。 |
| 12 | 已交付 | 缺口入口与修复行为见 [家庭导航与 Data Health](household-navigation-and-health.md)。 |
| 13 | 暂缓 | 公司行为报价线索未承诺。 |
| 14 | 未承诺 | Insights 保存预设尚未实现，也未列入路线图。 |
| 15 | 部分支持 | 版本化 JSON 导出覆盖业务与行情数据；分析报告导出未承诺，见 [导出格式](json-export-format.md)。 |
| 16 | 暂缓 | 行情同步不自动过账公司行为；账本过账需独立设计。 |
| 17 | 规划设计 | 月度复盘仅有设计，见 [Monthly Review](monthly-review-and-data-confidence.md)。 |
| 18 | 未承诺 | 记录模板与草稿未列入路线图。 |
| 19 | 暂缓 | 关联房产、债务与保险模型未规划。 |
| 20 | 部分支持 | Available Funds 预留与产品规则已实现；应急资金月份语义未实现，见 [Available Funds](available-funds.md)。 |
| 21 | 部分支持 | 通用价格与单位 NAV 已存在；债券和国库券专用模型未实现。 |
| 22 | 未承诺 | 期间比较与基准未列入路线图。 |
| 23 | 明确暂缓 | Tax lots 不属于当前产品范围，见 [产品愿景](../product/product-vision.md)。 |
| 24 | 未承诺 | 命令面板未实现，也未列入路线图。 |

以上索引保留原编号以便查阅；原提案中的详细范围、批次和验收设想已经过期，不再作为产品规格。
