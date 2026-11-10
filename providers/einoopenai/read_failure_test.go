package einoopenai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type readFailureSession struct{ failure error }

func (s readFailureSession) Definitions() ([]agentcapability.Definition, error) {
	closed := false
	return []agentcapability.Definition{{
		Name: "get_read_page", Effect: agentcapability.Read,
		InputSchema: map[string]any{"type": "object"},
		Annotations: &agentcapability.Annotations{ReadOnlyHint: true, OpenWorldHint: &closed},
	}}, nil
}

func (s readFailureSession) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, fmt.Errorf("synthetic internal wrapper: %w", s.failure)
}

func TestBusinessFailuresReachModelWithoutSecondFilter(t *testing.T) {
	const secret = "SENSITIVE_BACKEND_DETAIL"
	for _, tc := range []struct {
		name    string
		failure error
		code    string
		field   string
		message string
		action  string
	}{
		{"product nested", &apierror.WireError{Code: "validation", Field: "open.policy.earlyDayBasis", Message: "early day basis must match the selected interest policy"}, "validation", "open.policy.earlyDayBasis", "early day basis must match the selected interest policy", "correct_input"},
		{"product conflict", &apierror.WireError{Code: "managed_position_conflict", Field: "productId", Message: "use the product lifecycle for this managed position"}, "managed_position_conflict", "productId", "use the product lifecycle for this managed position", "inspect_current_state"},
		{"numeric money", agentcapability.ArgumentError{Field: "commands[0].amount", Rule: "type", Expected: []string{"string"}}, "validation", "commands[0].amount", "Field commands[0].amount must have JSON type string.", "correct_input"},
		{"ledger kind", agentcapability.ArgumentError{Field: "commands[0].kind", Rule: "enum", Expected: []string{"money_added", "money_removed"}}, "validation", "commands[0].kind", "Field commands[0].kind must be one of: money_added, money_removed.", "correct_input"},
		{"ledger seconds", &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "commands[0].effectiveLocalTime", Message: "Use HH:MM or RFC3339 effectiveAt with an explicit offset to preserve seconds."}, "invalid_change_time", "commands[0].effectiveLocalTime", "Use HH:MM or RFC3339 effectiveAt with an explicit offset to preserve seconds.", "correct_input"},
		{"registry validation", agentcapability.ErrInvalidArguments, "validation", "", "", "correct_input"},
		{"malformed date", &apierror.WireError{Code: "validation", Field: "fromDate", Message: "observation dates must be YYYY-MM-DD"}, "validation", "fromDate", "observation dates must be YYYY-MM-DD", "correct_input"},
		{"reversed date range", &apierror.WireError{Code: "validation", Field: "fromDate", Message: "observation date range must be forward"}, "validation", "fromDate", "observation date range must be forward", "correct_input"},
		{"expired", &apierror.WireError{Code: "read_expired", Message: "read capture expired"}, "read_expired", "", "read capture expired", "restart_read"},
		{"revoked capture", &apierror.WireError{Code: "context_revoked", Message: "context revoked"}, "context_revoked", "", "context revoked", "restart_read"},
		{"stale page", agentcapability.ReadRejection{Code: "stale_cursor", Field: "snapshotId"}, "stale_cursor", "snapshotId", "", "restart_read"},
		{"too large", &apierror.WireError{Code: "too_large", Field: "section", Message: "section exceeds the response budget"}, "too_large", "section", "section exceeds the response budget", "inspect_limits_or_narrow_read"},
		{"busy", &apierror.WireError{Code: "context_busy", Message: "context is busy"}, "context_busy", "", "context is busy", "retry_later"},
		{"permission", &apierror.WireError{Code: "approval_required", Message: "host review is required"}, "approval_required", "", "host review is required", "stop"},
		{"revoked session", assistant.ErrStale, "session_revoked", "", "", "stop"},
		{"raw infrastructure", errors.New(secret), "internal", "", "an unexpected error occurred", "stop"},
		{"wrapped infrastructure", apierror.Wrap(errors.New(secret)), "internal", "", "an unexpected error occurred", "stop"},
		{"new business code", &apierror.WireError{Code: "historical_series_budget", Field: "fromDate", Message: "local trend exceeded its computation deadline"}, "historical_series_budget", "fromDate", "local trend exceeded its computation deadline", "stop"},
		{"business amount detail", &domain.Error{Code: domain.ErrInsufficientBalance, Field: "amount", Message: "requested 12.34 USD exceeds available 10.00 USD"}, "insufficient_balance", "amount", "requested 12.34 USD exceeds available 10.00 USD", "inspect_current_state"},
		{"unlisted business field", &domain.Error{Code: domain.ErrValidation, Field: "settlementAccountId", Message: "settlement account currency must match the product"}, "validation", "settlementAccountId", "settlement account currency must match the product", "correct_input"},
		{"unlisted nested field", &apierror.WireError{Code: "validation", Field: "commands[0].unitCost", Message: "original unit cost is required"}, "validation", "commands[0].unitCost", "original unit cost is required", "correct_input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
				if n == 0 {
					writeMode(w, request, []any{liveCallItem(n, "get_read_page", obj{})})
					return
				}
				var output struct {
					Error readFailure `json:"error"`
				}
				raw := latestToolOutput(t, request)
				if tc.name == "numeric money" && !strings.Contains(raw, `"expected":["string"]`) {
					t.Errorf("schema constraints lost: %s", raw)
				}
				if jsoncodec.Unmarshal([]byte(raw), &output) != nil || output.Error.Code != tc.code ||
					output.Error.Field != tc.field || output.Error.Message != tc.message || output.Error.RetryAction != tc.action ||
					strings.Contains(raw, secret) || strings.Contains(raw, "synthetic internal wrapper") ||
					(strings.Contains(raw, "Restart the original read") != (tc.action == "restart_read")) {
					t.Errorf("unexpected diagnostic: %s", raw)
				}
				writeMode(w, request, []any{textItem()})
			})
			cfg := assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}
			e, err := engineWithModel(t.Context(), cfg, f.model, readFailureSession{failure: tc.failure}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			result, err := e.Complete(t.Context(), nil, "Continue this read")
			if err != nil || strings.Contains(string(result.History), secret) || !strings.Contains(string(result.History), tc.message) {
				t.Fatalf("diagnostic did not survive model history: %v", err)
			}
		})
	}
}

type correctingReadSession struct{ calls int }

func (*correctingReadSession) Definitions() ([]agentcapability.Definition, error) {
	return (readFailureSession{}).Definitions()
}
func (s *correctingReadSession) Call(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	s.calls++
	var input struct {
		Date string `json:"date"`
	}
	if err := jsoncodec.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.Date != "2026-09-01" {
		return nil, &apierror.WireError{Code: "validation", Field: "date", Message: "observation dates must be YYYY-MM-DD"}
	}
	return []byte(`{"data":{"complete":true}}`), nil
}
func TestModelCanCorrectBusinessInputAndContinue(t *testing.T) {
	session := &correctingReadSession{}
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, n int, request obj) {
		switch n {
		case 0:
			writeMode(w, request, []any{liveCallItem(n, "get_read_page", obj{"date": "2026/09/01"})})
		case 1:
			if !strings.Contains(latestToolOutput(t, request), "observation dates must be YYYY-MM-DD") {
				t.Fatal("business repair guidance did not reach the model")
			}
			writeMode(w, request, []any{liveCallItem(n, "get_read_page", obj{"date": "2026-09-01"})})
		default:
			if !strings.Contains(latestToolOutput(t, request), `"complete":true`) {
				t.Fatal("corrected read did not complete")
			}
			writeMode(w, request, []any{textItem()})
		}
	})
	e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, session, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err := e.Complete(t.Context(), nil, "Read the observations"); err != nil || session.calls != 2 {
		t.Fatalf("corrected read failed: calls=%d err=%v", session.calls, err)
	}
}
