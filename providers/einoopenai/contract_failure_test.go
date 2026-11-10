package einoopenai

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/domain"
	"strings"
	"testing"
)

func TestContractFailurePreservesRepairWithoutPrivateValues(t *testing.T) {
	argument := agentcapability.ArgumentError{Field: "commands[0].reason", Rule: "enum", Expected: []string{"income", "expense"}}
	failure := classifyReadFailure(argument)
	raw := readFailureDiagnostic(failure)
	for _, want := range []string{"commands[0].reason", "enum", "income", "correct_input"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("missing %s: %s", want, raw)
		}
	}
	business := classifyReadFailure(&domain.Error{Code: domain.ErrInsufficientBalance, Field: "amount", Message: "PRIVATE ACCOUNT BALANCE"})
	raw = readFailureDiagnostic(business)
	if business.Code != "insufficient_balance" || business.RetryAction != "inspect_current_state" || strings.Contains(raw, "PRIVATE") {
		t.Fatal(raw)
	}
	batch := newReadFailure("validation", "commands[0].side")
	if batch.Field != "commands[0].side" {
		t.Fatal(batch)
	}
	unknown := newReadFailure("validation", "commands[0].PRIVATE_ACCOUNT")
	if unknown.Field != "" {
		t.Fatal("unsafe field", unknown)
	}
	submitted := submissionFailure{cause: failure}
	if !errors.Is(submitted, errCapabilityWriteRejected) {
		t.Fatal("lost pre-dispatch marker")
	}
	raw = submissionDiagnostic(submitted, "")
	if !strings.Contains(raw, "not_dispatched") || !strings.Contains(raw, "commands[0].reason") || !strings.Contains(raw, "validation") {
		t.Fatal(raw)
	}
	if got := submissionDiagnostic(errCapabilityWriteRefused, "inspect-only"); got != "inspect-only" {
		t.Fatal("unknown write became retryable")
	}
}
