package assistant

import (
	"context"
	"errors"
)

// Fixed host-owned classifications only. Never attach provider bodies, request
// parameters, tool output, credentials or arbitrary error strings to these codes.
var (
	ErrProviderAuthentication = errors.New("provider_authentication")
	ErrProviderRateLimited    = errors.New("provider_rate_limited")
	ErrProviderRequest        = errors.New("provider_request_rejected")
	ErrProviderUnavailable    = errors.New("provider_unavailable")
	ErrProviderConnection     = errors.New("provider_connection")
	ErrProviderResponse       = errors.New("provider_response_rejected")
	ErrModelHistory           = errors.New("model_history_rejected")
	ErrToolResult             = errors.New("tool_result_rejected")
)

// safeFailure discards wrappers, even when a provider error wraps a known code.
func safeFailure(err error) error {
	for _, fixed := range []error{ErrProviderAuthentication, ErrProviderRateLimited, ErrProviderRequest, ErrProviderUnavailable, ErrProviderConnection, ErrProviderResponse, ErrModelHistory, ErrToolResult, ErrBudget, context.DeadlineExceeded} {
		if errors.Is(err, fixed) {
			return fixed
		}
	}
	return ErrUnavailable
}

// FailureCode copies no text from err, including wrapped third-party errors.
func FailureCode(err error) string {
	fixed := safeFailure(err)
	switch fixed {
	case ErrUnavailable:
		return "provider_or_storage_failure"
	case context.DeadlineExceeded:
		return "timeout"
	default:
		return fixed.Error()
	}
}
