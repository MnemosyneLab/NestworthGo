package einoopenai

import (
	"errors"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// Business diagnostics have already crossed the shared application error
// boundary. Preserve them for model repair just as external MCP does; arbitrary
// infrastructure errors still never become tool-result text.
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
	case errors.As(err, &business) && business != nil:
		result := newReadFailure(string(business.Code), business.Field)
		result.Message = business.Message
		return result
	case errors.Is(err, agentcapability.ErrInvalidArguments):
		return newReadFailure("validation", "")
	case errors.As(err, &rejected):
		return newReadFailure(rejected.Code, rejected.Field)
	case errors.As(err, &wire) && wire != nil:
		result := newReadFailure(wire.Code, wire.Field)
		result.Message = wire.Message
		return result
	case errors.Is(err, assistant.ErrStale):
		return newReadFailure("session_revoked", "")
	case errors.Is(err, assistant.ErrUnavailable):
		return newReadFailure("unavailable", "")
	default:
		result := newReadFailure("internal", "")
		result.Message = "an unexpected error occurred"
		return result
	}
}

func newReadFailure(code, field string) readFailure {
	if code == "" {
		code = "internal"
	}
	action := agentcapability.ReadRetryAction(code, field)
	recovery := "The tool returned no usable result. Explain the error or ask for clarification."
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
