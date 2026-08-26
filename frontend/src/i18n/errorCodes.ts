/**
 * The `errorCode` i18next namespace, keyed directly by `domain.ErrorCode`
 * (`domain.ErrorCode` is the single source of truth for the i18next
 * `error.<code>` key namespace; a new domain error code and a new i18next key
 * must ship together).
 *
 * Error messages are keyed by stable backend error codes rather than English
 * prose. The Wails error helper resolves these codes and combines them with
 * localized field labels when a backend error identifies a specific input.
 *
 * Every key here MUST have a counterpart in every locale; missing-key
 * coverage is checked by the locale-coverage test.
 */
export type ErrorCodeCatalog = Record<string, string>;

export const errorCodesEn: ErrorCodeCatalog = {
  validation: "This value is not valid.",
  not_found: "This could not be found.",
  conflict: "This could not be completed because of a conflict.",
  unsupported_database: "This database format is not supported.",
  migration_failed: "The database could not be upgraded.",
  integrity_failed: "A data integrity check failed.",
  unavailable: "This is currently unavailable.",
  decimal_overflow: "This value is outside the supported range.",
  provider_unavailable: "The market data provider is currently unavailable.",
  provider_authentication: "The market data provider rejected authentication.",
  provider_rate_limit: "The market data provider's rate limit was reached.",
  unsupported_provider_symbol: "This provider symbol is not supported.",
  malformed_provider_response: "The provider returned an invalid response.",
  market_data_response_too_large: "The provider response was too large.",
  history_not_started: "Start history before recording a change.",
  history_timezone_required: "Confirm a Household timezone before continuing.",
  invalid_change: "This change is not valid.",
  invalid_change_time: "This change time is not valid.",
  no_change: "The new value is unchanged.",
  insufficient_balance: "There is not enough balance for this change.",
  insufficient_quantity: "There is not enough quantity for this change.",
  already_undone: "This change has already been undone.",
  cannot_fix_change: "This change cannot be fixed.",
  transfer_mismatch: "The transfer amounts do not match.",
  invalid_trade: "This trade is not valid.",
  history_update_failed: "The history could not be updated.",
  cost_basis_required: "A per-unit cost is required for this change.",
  internal: "An unexpected error occurred.",
};

export const errorCodesZhCN: ErrorCodeCatalog = {
  validation: "该值无效。",
  not_found: "未找到。",
  conflict: "因存在冲突，操作未能完成。",
  unsupported_database: "不支持此数据库格式。",
  migration_failed: "数据库升级失败。",
  integrity_failed: "数据完整性检查失败。",
  unavailable: "当前不可用。",
  decimal_overflow: "该数值超出支持的范围。",
  provider_unavailable: "行情数据提供方当前不可用。",
  provider_authentication: "行情数据提供方拒绝了认证。",
  provider_rate_limit: "已达到行情数据提供方的速率限制。",
  unsupported_provider_symbol: "不支持此 Provider 标的。",
  malformed_provider_response: "Provider 返回了无效的响应。",
  market_data_response_too_large: "Provider 响应过大。",
  history_not_started: "请先开始历史记录，再记录变化。",
  history_timezone_required: "请先确认家庭时区，再继续。",
  invalid_change: "此变化无效。",
  invalid_change_time: "此变化时间无效。",
  no_change: "新值与原值相同，未发生变化。",
  insufficient_balance: "余额不足，无法完成此变化。",
  insufficient_quantity: "数量不足，无法完成此变化。",
  already_undone: "此变化已被撤销。",
  cannot_fix_change: "此变化无法被修正。",
  transfer_mismatch: "转账金额不匹配。",
  invalid_trade: "此交易无效。",
  history_update_failed: "历史记录更新失败。",
  cost_basis_required: "此变化需要填写单位成本。",
  internal: "发生了意外错误。",
};

export const errorCodesZhTW: ErrorCodeCatalog = {
  validation: "此數值無效。",
  not_found: "找不到。",
  conflict: "因存在衝突，操作未能完成。",
  unsupported_database: "不支援此資料庫格式。",
  migration_failed: "資料庫升級失敗。",
  integrity_failed: "資料完整性檢查失敗。",
  unavailable: "目前無法使用。",
  decimal_overflow: "此數值超出支援的範圍。",
  provider_unavailable: "行情資料提供方目前無法使用。",
  provider_authentication: "行情資料提供方拒絕了驗證。",
  provider_rate_limit: "已達到行情資料提供方的速率限制。",
  unsupported_provider_symbol: "不支援此 Provider 標的。",
  malformed_provider_response: "Provider 回傳了無效的回應。",
  market_data_response_too_large: "Provider 回應過大。",
  history_not_started: "請先開始歷史記錄，再記錄變化。",
  history_timezone_required: "請先確認家庭時區，再繼續。",
  invalid_change: "此變化無效。",
  invalid_change_time: "此變化時間無效。",
  no_change: "新值與原值相同，未發生變化。",
  insufficient_balance: "餘額不足，無法完成此變化。",
  insufficient_quantity: "數量不足，無法完成此變化。",
  already_undone: "此變化已被撤銷。",
  cannot_fix_change: "此變化無法被修正。",
  transfer_mismatch: "轉帳金額不符。",
  invalid_trade: "此交易無效。",
  history_update_failed: "歷史記錄更新失敗。",
  cost_basis_required: "此變化需要填寫單位成本。",
  internal: "發生了意外的錯誤。",
};
