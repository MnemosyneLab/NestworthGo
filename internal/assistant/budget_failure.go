package assistant

import (
	"errors"
	"strings"
)

// Only host-owned categories cross persistence/IPC. No provider text, tool
// arguments, ledger values, prompts or credentials are diagnostic fields.
var budgetRules = map[string]bool{
	"model_rounds":      true,
	"tool_calls":        true,
	"history_bytes":     true,
	"history_messages":  true,
	"prompt_bytes":      true,
	"text_bytes":        true,
	"provider_payload":  true,
	"tool_result_bytes": true,
	"read_evidence":     true,
}

const budgetFailurePrefix = "assistant_budget_exceeded."

type budgetFailure struct{ rule string }

func (e budgetFailure) Error() string { return budgetFailurePrefix + e.rule }
func (e budgetFailure) Unwrap() error { return ErrBudget }

// BudgetFailure rejects unknown categories instead of reflecting their text.
func BudgetFailure(rule string) error {
	if !budgetRules[rule] {
		return ErrBudget
	}
	return budgetFailure{rule}
}
func IsBudgetFailureCode(code string) bool {
	return strings.HasPrefix(code, budgetFailurePrefix) && budgetRules[strings.TrimPrefix(code, budgetFailurePrefix)]
}

// NormalizeBudgetFailure removes untrusted error wrappers. A nil result means
// this is not a budget failure; callers must still sanitize other failures.
func NormalizeBudgetFailure(err error) error {
	var detail budgetFailure
	if errors.As(err, &detail) && budgetRules[detail.rule] {
		return budgetFailure{detail.rule}
	}
	if errors.Is(err, ErrBudget) {
		return ErrBudget
	}
	return nil
}
