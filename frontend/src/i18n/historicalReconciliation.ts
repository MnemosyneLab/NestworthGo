export const historicalReconciliationEn = {
  title: "Explain net worth change", identity: "B − A = supported contributions + unexplained residual",
  total: "Net worth change · B − A", supported: "Supported contributions", residual: "Unexplained residual",
  explanation: "How these amounts reconcile", contribution: "Contribution", categories: "Exact amounts in the shared base currency",
  basis: "Recorded money movements use the later date’s FX rate. FX measures the change on opening balances. Reverse comparisons negate the same chronological bridge. Later corrections and reversals use retained economic facts; this is not investment return.",
  marketBasis: "Price change covers unchanged quantities with comparable quotes. Trades, quantity changes, missing evidence and changes in scope or quote basis can leave a residual. A zero residual does not prove complete attribution.",
  internal: "Internal transfers and debt principal are neutral within the included household scope. Foreign transfer exposure and internal imbalances remain unexplained; they are never treated as wealth creation. Adjustments describe recorded remeasurement, not earnings.",
  status: { reconciled: "The supported amounts reconcile; this is an arithmetic explanation, not proof of causation.", partial: "Attribution is partial. Review the residual and evidence limits below.", unavailable: "Attribution is unavailable for these endpoints; unknown amounts remain unknown." },
  category: { external_flow: "External flows", income: "Income / gifts", spending: "Spending", dividend_interest: "Dividends / interest", fee: "Fees / taxes", adjustment: "Recorded adjustments", liability_impact: "Liability impact on net worth", price_change: "Price / valuation change", fx_impact: "FX change on opening balances" },
  reason: { currency_mismatch: "The endpoints have different base currencies.", current_endpoint: "Current uses a different quote authority; select two closed dates to reconcile.", incomplete_endpoints: "Missing endpoint values prevent a complete total or residual.", budget_exceeded: "The reconciliation work budget was exceeded; the ordinary comparison remains available.", scope_changed: "Components appeared, disappeared or changed inclusion/state during the interval. Their change stays in the residual.", missing_evidence: "A required amount, price or FX observation is missing.", basis_changed: "Currency, quote source or valuation basis changed; affected contributions remain unexplained.", unsupported_activity: "Some activity cannot be measured reliably by this endpoint bridge.", quantity_activity: "A holding quantity changed or was touched by activity; trading/path price and FX attribution is deferred.", foreign_internal_exposure: "Foreign internal or trading movements changed exposure; their FX path stays in the residual.", internal_imbalance: "Internal monetary legs do not cancel at the later endpoint rate; the difference stays in the residual.", unexplained_movement: "The remaining movement may include unsupported events, observations, exposure changes or decimal division precision. It is not labelled investment profit." },
};
export const historicalReconciliationZhCN = {
  title: "解释净资产变化", identity: "B − A = 有依据的贡献 + 未解释差额",
  total: "净资产变化 · B − A", supported: "有依据的贡献", residual: "未解释差额",
  explanation: "金额如何核对", contribution: "贡献类别", categories: "统一基准货币的精确金额",
  basis: "已记录的资金变动按较晚日期的汇率换算。汇率贡献衡量期初余额的汇率变化。反向比较对同一时间顺序的核对结果取负。后续更正和冲销使用保留的经济事实；这不是投资回报。",
  marketBasis: "价格变化仅涵盖数量不变且报价依据可比的持仓。交易、数量变动、证据缺失以及范围或报价依据变化可能留下差额。差额为零并不证明归因完整。",
  internal: "计入家庭范围内的内部转账和债务本金为中性变动。外币转移带来的敞口变化及内部不平衡仍属未解释差额，不计为财富创造。调整表示已记录的重新计量，不代表收益。",
  status: { reconciled: "有依据的金额已核对；这是算术解释，并非因果证明。", partial: "归因不完整。请展开查看差额和证据限制。", unavailable: "这些端点无法归因；未知金额仍为未知。" },
  category: { external_flow: "外部资金流", income: "收入／赠与", spending: "支出", dividend_interest: "股息／利息", fee: "费用／税款", adjustment: "已记录的调整", liability_impact: "负债对净资产的影响", price_change: "价格／估值变化", fx_impact: "期初余额的汇率变化" },
  reason: { currency_mismatch: "端点使用不同的基准货币。", current_endpoint: "当前值使用不同的报价来源规则；请选择两个已结束日期进行核对。", incomplete_endpoints: "端点金额缺失，无法计算完整变化或差额。", budget_exceeded: "已超出归因工作预算；普通比较仍可使用。", scope_changed: "区间内组成项出现、消失或计入范围／状态变化，其变化留在差额中。", missing_evidence: "缺少必要的金额、价格或汇率记录。", basis_changed: "货币、报价来源或估值依据变化；受影响的贡献仍未解释。", unsupported_activity: "部分活动无法通过此端点核对可靠计量。", quantity_activity: "持仓数量有变化或涉及活动；交易路径的价格和汇率归因暂未支持。", foreign_internal_exposure: "外币内部转移或交易改变敞口，其汇率路径留在差额中。", internal_imbalance: "内部资金腿按较晚端点汇率换算后未抵消，差异留在差额中。", unexplained_movement: "剩余变动可能包含未支持的事件、观测、敞口变化或十进制除法精度差异，不标记为投资利润。" },
};
export const historicalReconciliationZhTW = {
  title: "解釋淨資產變化", identity: "B − A = 有依據的貢獻 + 未解釋差額",
  total: "淨資產變化 · B − A", supported: "有依據的貢獻", residual: "未解釋差額",
  explanation: "金額如何核對", contribution: "貢獻類別", categories: "統一基準貨幣的精確金額",
  basis: "已記錄的資金變動按較晚日期的匯率換算。匯率貢獻衡量期初餘額的匯率變化。反向比較對同一時間順序的核對結果取負。後續更正和沖銷使用保留的經濟事實；這不是投資報酬。",
  marketBasis: "價格變化僅涵蓋數量不變且報價依據可比的持倉。交易、數量變動、證據缺失以及範圍或報價依據變化可能留下差額。差額為零並不證明歸因完整。",
  internal: "計入家庭範圍內的內部轉帳和債務本金為中性變動。外幣轉移帶來的曝險變化及內部不平衡仍屬未解釋差額，不計為財富創造。調整表示已記錄的重新衡量，不代表收益。",
  status: { reconciled: "有依據的金額已核對；這是算術解釋，並非因果證明。", partial: "歸因不完整。請展開查看差額和證據限制。", unavailable: "這些端點無法歸因；未知金額仍為未知。" },
  category: { external_flow: "外部資金流", income: "收入／贈與", spending: "支出", dividend_interest: "股息／利息", fee: "費用／稅款", adjustment: "已記錄的調整", liability_impact: "負債對淨資產的影響", price_change: "價格／估值變化", fx_impact: "期初餘額的匯率變化" },
  reason: { currency_mismatch: "端點使用不同的基準貨幣。", current_endpoint: "目前值使用不同的報價來源規則；請選擇兩個已結束日期進行核對。", incomplete_endpoints: "端點金額缺失，無法計算完整變化或差額。", budget_exceeded: "已超出歸因工作預算；普通比較仍可使用。", scope_changed: "區間內組成項出現、消失或計入範圍／狀態變化，其變化留在差額中。", missing_evidence: "缺少必要的金額、價格或匯率記錄。", basis_changed: "貨幣、報價來源或估值依據變化；受影響的貢獻仍未解釋。", unsupported_activity: "部分活動無法透過此端點核對可靠衡量。", quantity_activity: "持倉數量有變化或涉及活動；交易路徑的價格和匯率歸因暫未支援。", foreign_internal_exposure: "外幣內部轉移或交易改變曝險，其匯率路徑留在差額中。", internal_imbalance: "內部資金腿按較晚端點匯率換算後未抵銷，差異留在差額中。", unexplained_movement: "剩餘變動可能包含未支援的事件、觀測、曝險變化或十進位除法精度差異，不標記為投資利潤。" },
};
