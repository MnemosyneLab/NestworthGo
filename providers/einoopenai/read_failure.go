package einoopenai

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
	"regexp"
	"strings"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// This error carries only a closed model-facing vocabulary. Its string form
// also excludes the original error and its potentially sensitive message.
type readFailure struct {
	Code        string   `json:"code"`
	Rule        string   `json:"rule,omitempty"`
	Expected    []string `json:"expected,omitempty"`
	Message     string   `json:"message,omitempty"`
	Field       string   `json:"field,omitempty"`
	RetryAction string   `json:"retryAction"`
	Recovery    string   `json:"recovery"`
}

func (readFailure) Error() string { return errCapabilityRefused.Error() }
func (readFailure) Unwrap() error { return errCapabilityRefused }

func classifyReadFailure(err error) readFailure {
	var wire *apierror.WireError
	var rejected agentcapability.ReadRejection
	var argument agentcapability.ArgumentError
	var business *domain.Error
	switch {
	case errors.As(err, &argument):
		result := newReadFailure("validation", "")
		result.Field, result.Rule, result.Expected = argument.Field, argument.Rule, argument.Expected
		result.Message = argument.Message()
		return result
	case errors.As(err, &business):
		return newReadFailure(string(business.Code), business.Field)
	case errors.Is(err, agentcapability.ErrInvalidArguments):
		return newReadFailure("validation", "")
	case errors.As(err, &rejected):
		return newReadFailure(rejected.Code, rejected.Field)
	case errors.As(err, &wire) && wire != nil:
		return newReadFailure(wire.Code, wire.Field)
	case errors.Is(err, assistant.ErrStale):
		return newReadFailure("session_revoked", "")
	case errors.Is(err, assistant.ErrUnavailable):
		return newReadFailure("unavailable", "")
	default:
		return newReadFailure("", "")
	}
}

func newReadFailure(code, field string) readFailure {
	switch code {
	case "validation", "read_expired", "read_revoked", "context_expired", "context_revoked", "stale_cursor",
		"managed_position_conflict", "unsupported_partial_operation", "receipt_pending", "unsafe_undo", "revision_conflict", "insufficient_quantity", "already_undone", "cannot_fix_change", "transfer_mismatch", "invalid_trade", "cost_basis_required", "history_update_failed", "database_upgrade_required", "database_from_newer_version", "database_integrity_failed", "malformed_provider_response", "market_data_response_too_large", "stale_preview", "conflict", "too_large", "read_busy", "context_busy", "backup_restore_busy", "not_found",
		"approval_required", "permission_denied", "session_revoked", "database_unavailable",
		"household_unavailable", "unavailable", "cancelled", "insufficient_balance", "invalid_change", "invalid_change_time", "history_not_started", "history_timezone_required", "cash_balance_time_conflict", "no_change", "decimal_overflow", "provider_unavailable", "provider_authentication", "provider_rate_limit", "unsupported_provider_symbol", "unresolved_reservation_release", "internal":
	default:
		code, field = "controlled_read_refused", ""
	}
	prefix := ""
	parts := strings.Split(field, ".")
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "input", "changes", "replacement", "open", "recordExisting", "receiveInterest", "settle", "renew", "terms", "policy":
		default:
			if !batchField.MatchString(part) {
				field = ""
			}
		}
	}
	if field != "" && len(parts) > 1 {
		prefix = strings.Join(parts[:len(parts)-1], ".") + "."
		field = parts[len(parts)-1]
	}

	// A domain handler can set arbitrary fields. Preserve only names declared
	// by our tool contracts, never a supplied value or dynamic map key.
	switch field {
	case "productId", "expectedRevision", "interestMode", "accessKind", "earlyKind", "earlyAmountMode", "dayBasis", "earlyDayBasis", "settlementDays", "earlySettlementDays", "unlockOn", "maturityOn", "startOn", "annualRate", "annualRatePercent", "maturityInterest", "interestPaidThroughOn", "earlyFee", "earlyGrossAmount", "accessibleAmountCap", "normalExitFee", "receiptOnOverride", "reason", "side", "fee", "feeCurrency", "interestOrFee", "interestOrFeeCurrency", "quantity", "holdingId", "principal", "effectiveLocalDate", "effectiveLocalTime", "action", "replacement", "operationId", "planId", "accountId", "accountIds", "amount", "asOf", "baseCurrency", "captureId", "commands", "comparisonId", "contextId", "currency",
		"cursor", "date", "effectiveAt", "filter", "from", "fromDate", "groups", "historyLayout", "id", "instrumentId", "instrumentIds", "itemId", "kind",
		"leftAsOf", "limit", "mode", "module", "moduleCursor", "quoteCurrency", "range", "rightAsOf", "scope", "section", "sections",
		"sessionId", "snapshotId", "sort", "status", "targetRef", "targets", "to", "toDate", "valuation", "view":
	default:
		field = ""
	}
	if field != "" {
		field = prefix + field
	}
	action := agentcapability.ReadRetryAction(code, field)
	recovery := "No tool result was returned. Explain the limitation or ask for clarification. Do not infer any financial fact, widen permissions or repeat an uncertain operation."
	switch action {
	case "refresh_preview":
		recovery = "The preview is stale or expired. Read current evidence and prepare a fresh preview only if no prior write is uncertain."
	case "restart_read":
		recovery = "Restart the original read with the same scope, without the old cursor or capture ID. Do not combine old and new captures."
	case "correct_input":
		recovery = "Check the tool schema and correct the arguments. For a continuation, copy its exact cursor and other required references from the same capture. Do not resubmit unchanged rejected arguments. Ask for any missing details."
	case "inspect_limits_or_narrow_read":
		recovery = "Use a smaller page or narrow the requested scope with the user's intent intact. Disclose incomplete evidence."
	case "inspect_current_state":
		recovery = "Check the target and current state with a permitted read. Do not infer missing data or repeat an uncertain operation."
	case "retry_later":
		recovery = "The read is temporarily busy. Explain the limitation and retry later."
	}
	return readFailure{Code: code, Field: field, Message: safeFailureMessage(code, field), RetryAction: action, Recovery: recovery}
}

func readFailureDiagnostic(err error) string {
	var failure readFailure
	if !errors.As(err, &failure) {
		failure = newReadFailure("", "")
	}
	raw, _ := jsoncodec.Marshal(struct {
		Error readFailure `json:"error"`
	}{Error: failure})
	return string(raw)
}

var batchField = regexp.MustCompile(`^commands\[[0-9]{1,3}\]$`)

func safeFailureMessage(code, field string) string {
	switch code {
	case "validation":
		if strings.HasSuffix(field, "reason") {
			return "Use a supported reason code (for salary use income); put the user description in note."
		}
		if strings.HasSuffix(field, "side") {
			return "Trade side must be buy or sell."
		}
		return "A required argument is missing, invalid or incompatible with this operation."
	case "managed_position_conflict":
		return "This is a managed product position. Use the product lifecycle or product terms tools, not ordinary ledger or instrument mutations."
	case "unsupported_partial_operation":
		return "This product operation requires the whole contract; partial operations are unsupported."
	case "unsafe_undo", "already_undone", "cannot_fix_change":
		return "The activity cannot be safely reversed or replaced in its current history. Inspect the original activity and later dependent operations."
	case "receipt_pending":
		return "An operation receipt is pending. Inspect the original operation; do not replay or replace it."
	case "revision_conflict":
		return "The supplied revision is stale. Read current state before preparing a new preview."
	case "cost_basis_required":
		return "An evidenced original cost basis is required; current value is not a substitute."
	case "transfer_mismatch", "invalid_trade":
		return "The trade or transfer does not match the account, position, currency or amount constraints; inspect the relevant records."
	case "insufficient_quantity", "insufficient_balance":
		return "The operation exceeds the available balance or position; inspect current evidence and the requested amount."
	case "invalid_change", "invalid_change_time", "cash_balance_time_conflict":
		return "The change conflicts with ledger rules or effective-time history; inspect the relevant account and activity."
	case "history_not_started", "history_timezone_required":
		return "Ledger history or its timezone must be configured before this operation."
	case "no_change":
		return "The request would not change the current state."
	case "decimal_overflow":
		return "The exact numeric value exceeds the supported range or precision."
	case "provider_unavailable", "provider_rate_limit", "provider_authentication", "unsupported_provider_symbol":
		return "The market-data provider could not fulfill this request; check provider configuration, symbol and availability."
	case "unresolved_reservation_release":
		return "This product operation requires reservation handling in the App."
	case "database_upgrade_required", "database_from_newer_version", "database_integrity_failed", "history_update_failed":
		return "Database or history maintenance is required in the App before continuing."
	case "malformed_provider_response", "market_data_response_too_large":
		return "The provider response was unusable; inspect provider diagnostics or narrow the requested data."
	case "internal":
		return "An internal failure prevented a usable result; inspect App diagnostics."
	default:
		return ""
	}
}
