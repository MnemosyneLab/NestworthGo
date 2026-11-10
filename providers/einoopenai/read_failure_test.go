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

func TestReadRefusalsGiveDistinctSafeModelRecovery(t *testing.T) {
	const secret = "SENSITIVE_BACKEND_DETAIL"
	for _, tc := range []struct {
		name    string
		failure error
		code    string
		field   string
		action  string
	}{
		{"product nested", &apierror.WireError{Code: "validation", Field: "open.policy.earlyDayBasis", Message: secret}, "validation", "open.policy.earlyDayBasis", "correct_input"},
		{"product conflict", &apierror.WireError{Code: "managed_position_conflict", Field: "productId", Message: secret}, "managed_position_conflict", "productId", "inspect_current_state"},
		{"private prefix", &apierror.WireError{Code: "validation", Field: secret + ".policy.dayBasis", Message: secret}, "validation", "", "correct_input"},
		{"numeric money", agentcapability.ArgumentError{Field: "commands[0].amount", Rule: "type", Expected: []string{"string"}}, "validation", "commands[0].amount", "correct_input"},
		{"registry_validation", agentcapability.ErrInvalidArguments, "validation", "", "correct_input"},
		{"typed_validation", &apierror.WireError{Code: "validation", Field: "limit", Message: secret}, "validation", "limit", "correct_input"},
		{"expired", &apierror.WireError{Code: "read_expired", Message: secret}, "read_expired", "", "restart_read"},
		{"revoked_capture", &apierror.WireError{Code: "context_revoked", Message: secret}, "context_revoked", "", "restart_read"},
		{"stale_page", agentcapability.ReadRejection{Code: "stale_cursor", Field: "snapshotId"}, "stale_cursor", "snapshotId", "restart_read"},
		{"too_large", &apierror.WireError{Code: "too_large", Field: "section", Message: secret}, "too_large", "section", "inspect_limits_or_narrow_read"},
		{"busy", &apierror.WireError{Code: "context_busy", Message: secret}, "context_busy", "", "retry_later"},
		{"permission", &apierror.WireError{Code: "approval_required", Message: secret}, "approval_required", "", "stop"},
		{"revoked_session", assistant.ErrStale, "session_revoked", "", "stop"},
		{"unknown", errors.New(secret), "controlled_read_refused", "", "stop"},
		{"untrusted_code", &apierror.WireError{Code: secret, Field: "cursor", Message: secret}, "controlled_read_refused", "", "stop"},
		{"untrusted_field", &apierror.WireError{Code: "validation", Field: secret, Message: secret}, "validation", "", "correct_input"},
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
				if tc.name == "numeric money" && (!strings.Contains(raw, "Field commands[0].amount must have JSON type string") || !strings.Contains(raw, `"expected":["string"]`)) {
					t.Errorf("numeric money detail lost: %s", raw)
				}
				if jsoncodec.Unmarshal([]byte(raw), &output) != nil || output.Error.Code != tc.code ||
					output.Error.Field != tc.field || output.Error.RetryAction != tc.action ||
					strings.Contains(raw, secret) || strings.Contains(raw, "synthetic internal wrapper") ||
					(strings.Contains(raw, "Restart the original read") != (tc.action == "restart_read")) {
					t.Errorf("unexpected refusal: %s", raw)
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
			if err != nil || strings.Contains(string(result.History), secret) {
				t.Fatalf("safe refusal did not complete: %v", err)
			}
		})
	}
}
