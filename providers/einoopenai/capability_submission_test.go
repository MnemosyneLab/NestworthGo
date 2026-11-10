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

type submissionTestSession struct {
	err   error
	calls int
}

func (*submissionTestSession) Definitions() ([]agentcapability.Definition, error) { return nil, nil }
func (s *submissionTestSession) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	s.calls++
	return nil, s.err
}

type submissionTestRejection struct{}

func (submissionTestRejection) Error() string       { return "synthetic validation rejection" }
func (submissionTestRejection) NotDispatched() bool { return true }

func TestWriteDiagnosticsRequireSubmissionEvidence(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want error
	}{
		{submissionTestRejection{}, errCapabilityWriteRejected},
		{assistant.ErrRecoveryRequired, errCapabilityWriteRecoveryRequired},
		{errors.New("validation"), errCapabilityWriteRefused},
		{context.Canceled, errCapabilityWriteRefused},
	} {
		s := &submissionTestSession{err: tc.err}
		tool := capabilityTool{session: s, name: "commit_plan", writing: true}
		_, err := tool.InvokableRun(t.Context(), `{}`)
		if !errors.Is(err, tc.want) || s.calls != 1 {
			t.Fatalf("wrong write diagnostic: %v calls=%d", err, s.calls)
		}
	}
	s := &submissionTestSession{}
	tool := capabilityTool{session: s, name: "commit_plan", writing: true}
	_, err := tool.InvokableRun(t.Context(), strings.Repeat(" ", 65<<10))
	if !errors.Is(err, errCapabilityWriteRejected) || s.calls != 0 {
		t.Fatal("oversized input was dispatched or marked uncertain", err)
	}
}

func TestPagePreflightProofDoesNotRequestNonexistentReceipt(t *testing.T) {
	s := &submissionTestSession{err: submissionTestRejection{}}
	page := capabilityTool{session: s, name: "operate_page", writing: true}
	_, err := page.InvokableRun(t.Context(), `{}`)
	if !errors.Is(err, errPageOperationRefused) {
		t.Fatal("lost page preflight proof", err)
	}
	if !strings.Contains(pageOperationRefusedDiagnostic, `"code":"page_operation_refused"`) || strings.Contains(pageOperationRefusedDiagnostic, "Inspect the original operation receipt") || strings.Contains(pageOperationRefusedDiagnostic, "write_requires_inspection") {
		t.Fatal("preflight diagnostic invented a submitted receipt")
	}
	s.err = assistant.ErrStale
	_, err = page.InvokableRun(t.Context(), `{}`)
	if !errors.Is(err, errCapabilityWriteRefused) {
		t.Fatal("unproved page failure became retryable", err)
	}
}

func TestPageBatchInputBudgetSupportsBoundedUnicodeValues(t *testing.T) {
	actions := make([]obj, 20)
	for i := range actions {
		actions[i] = obj{"targetRef": "synthetic-target", "action": "set_value", "value": strings.Repeat("😀", 2048)}
	}
	raw, err := jsoncodec.Marshal(obj{"snapshotId": "synthetic-snapshot", "actions": actions})
	if err != nil || len(raw) <= 64<<10 || len(raw) > 256<<10 {
		t.Fatal("invalid Unicode batch fixture", len(raw), err)
	}
	s := &submissionTestSession{err: submissionTestRejection{}}
	page := capabilityTool{session: s, name: "operate_page", writing: true}
	if _, err := page.InvokableRun(t.Context(), string(raw)); !errors.Is(err, errPageOperationRefused) || s.calls != 1 {
		t.Fatal("valid bounded Unicode envelope did not reach host validation", err, s.calls)
	}
	if _, err := page.InvokableRun(t.Context(), strings.Repeat(" ", (256<<10)+1)); !errors.Is(err, errPageOperationRefused) || s.calls != 1 {
		t.Fatal("oversized batch reached host", err, s.calls)
	}
	ordinary := capabilityTool{session: s, name: "commit_plan", writing: true}
	if _, err := ordinary.InvokableRun(t.Context(), string(raw)); !errors.Is(err, errCapabilityWriteRejected) || s.calls != 1 {
		t.Fatal("page envelope limit broadened ordinary tool limits", err, s.calls)
	}
}

type unicodeBatchSession struct {
	pageRecoverySession
	calls int
}

func (s *unicodeBatchSession) Call(_ context.Context, name string, raw json.RawMessage) (json.RawMessage, error) {
	if name != "operate_page" || len(raw) <= 64<<10 || len(raw) > 256<<10 {
		return nil, errors.New("unexpected batch")
	}
	s.calls++
	return json.RawMessage(`{"state":"dispatched"}`), nil
}

func TestPageUnicodeBatchPassesStreamingAndNonStreamingWireLimits(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			actions := make([]obj, 20)
			for i := range actions {
				actions[i] = obj{"targetRef": "synthetic-target", "action": "set_value", "value": strings.Repeat("😀", 2048)}
			}
			session := &unicodeBatchSession{}
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
				if round == 0 {
					writeMode(w, request, []any{liveCallItem(round, "operate_page", obj{"snapshotId": "synthetic-snapshot", "actions": actions})})
					return
				}
				writeMode(w, request, []any{textItem()})
			})
			e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, completionMode(fake.model, stream), session, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			if _, err := e.Complete(t.Context(), nil, "Set synthetic text controls"); err != nil || session.calls != 1 {
				t.Fatal("bounded batch rejected before dispatch", err, session.calls)
			}
		})
	}
}

type recordedFailureSession struct{ calls int }

func (*recordedFailureSession) FullAccessEnabled() bool { return true }
func (*recordedFailureSession) Definitions() ([]agentcapability.Definition, error) {
	closed := false
	return []agentcapability.Definition{{Name: "create_member", Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{OpenWorldHint: &closed}, InputSchema: map[string]any{"type": "object"}}}, nil
}
func (s *recordedFailureSession) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	s.calls++
	return nil, agentcapability.WriteFailure{OperationID: "synthetic-operation", Cause: &apierror.WireError{Code: "revision_conflict", Field: "input.expectedRevision", Message: "expectedRevision is stale; read the current product revision"}}
}
func TestEngineExplainsDurableFailedWrite(t *testing.T) {
	s := &recordedFailureSession{}
	f := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
		if round == 0 {
			writeMode(w, request, []any{liveCallItem(round, "create_member", obj{})})
			return
		}
		raw := latestToolOutput(t, request)
		for _, want := range []string{`"state":"failed"`, `"code":"write_failed"`, `"causeCode":"revision_conflict"`, `"field":"input.expectedRevision"`, `"operationId":"synthetic-operation"`, `"message":"expectedRevision is stale; read the current product revision"`} {
			if !strings.Contains(raw, want) {
				t.Errorf("missing %s in %s", want, raw)
			}
		}
		if strings.Contains(raw, "write_requires_inspection") {
			t.Errorf("incorrect failure: %s", raw)
		}
		writeMode(w, request, []any{textItem()})
	})
	e, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, f.model, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err = e.Complete(t.Context(), nil, "Explain the synthetic write result"); err != nil || s.calls != 1 {
		t.Fatalf("result: %v calls=%d", err, s.calls)
	}
}
