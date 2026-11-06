package einoopenai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
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
	page := capabilityTool{session: s, name: "operate_page_control", writing: true}
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
