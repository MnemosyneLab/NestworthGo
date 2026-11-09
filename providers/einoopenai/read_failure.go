package einoopenai

import (
	"errors"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// This error carries only a closed model-facing vocabulary. Its string form
// also excludes the original error and its potentially sensitive message.
type readFailure struct {
	Code        string `json:"code"`
	Field       string `json:"field,omitempty"`
	RetryAction string `json:"retryAction"`
	Recovery    string `json:"recovery"`
}

func (readFailure) Error() string { return errCapabilityRefused.Error() }
func (readFailure) Unwrap() error { return errCapabilityRefused }

func classifyReadFailure(err error) readFailure {
	var wire *apierror.WireError
	var rejected agentcapability.ReadRejection
	switch {
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
		"conflict", "too_large", "read_busy", "context_busy", "backup_restore_busy", "not_found",
		"approval_required", "permission_denied", "session_revoked", "database_unavailable",
		"household_unavailable", "unavailable", "cancelled":
	default:
		code, field = "controlled_read_refused", ""
	}
	// A domain handler can set arbitrary fields. Preserve only names declared
	// by our tool contracts, never a supplied value or dynamic map key.
	switch field {
	case "accountId", "accountIds", "amount", "asOf", "baseCurrency", "captureId", "commands", "comparisonId", "contextId", "currency",
		"cursor", "date", "effectiveAt", "filter", "from", "fromDate", "groups", "historyLayout", "id", "instrumentId", "instrumentIds", "itemId", "kind",
		"leftAsOf", "limit", "mode", "module", "moduleCursor", "quoteCurrency", "range", "rightAsOf", "scope", "section", "sections",
		"sessionId", "snapshotId", "sort", "status", "targetRef", "targets", "to", "toDate", "valuation", "view":
	default:
		field = ""
	}
	action := agentcapability.ReadRetryAction(code, field)
	recovery := "No tool result was returned. Explain the limitation or ask for clarification. Do not infer any financial fact, widen permissions or repeat an uncertain operation."
	switch action {
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
	return readFailure{Code: code, Field: field, RetryAction: action, Recovery: recovery}
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
