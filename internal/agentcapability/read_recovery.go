package agentcapability

// ReadRejection is a typed refusal for host-owned readers outside the
// registry. Adapters still validate the code and field before displaying them.
type ReadRejection struct {
	Code  string
	Field string
}

func (ReadRejection) Error() string { return "capability read refused" }

// ReadRetryAction classifies a read refusal. It gives recovery guidance, never
// permission to retry automatically or repeat an operation with side effects.
func ReadRetryAction(code, field string) string {
	switch code {
	case "read_expired", "read_revoked", "context_expired", "context_revoked", "stale_cursor":
		return "restart_read"
	case "stale_preview":
		return "refresh_preview"
	case "conflict":
		if field == "cursor" {
			return "restart_read"
		}
		return "inspect_current_state"
	case "validation", "decimal_overflow":
		return "correct_input"
	case "too_large":
		return "inspect_limits_or_narrow_read"
	case "read_busy", "context_busy", "backup_restore_busy":
		return "retry_later"
	case "revision_conflict", "managed_position_conflict", "unsupported_partial_operation", "insufficient_quantity", "already_undone", "cannot_fix_change", "transfer_mismatch", "invalid_trade", "cost_basis_required", "unsafe_undo", "not_found", "insufficient_balance", "invalid_change", "invalid_change_time", "cash_balance_time_conflict", "no_change":
		return "inspect_current_state"
	default:
		return "stop"
	}
}
