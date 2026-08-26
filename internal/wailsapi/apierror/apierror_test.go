package apierror

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestWrapNil(t *testing.T) {
	if err := Wrap(nil); err != nil {
		t.Fatalf("Wrap(nil) = %v, want nil", err)
	}
}

func TestWrapDomainError(t *testing.T) {
	original := &domain.Error{Code: domain.ErrValidation, Field: "name", Message: "must not be empty"}
	wrapped := Wrap(original)
	parsed, ok := Parse(wrapped.Error())
	if !ok {
		t.Fatalf("Parse(%q) failed", wrapped.Error())
	}
	if parsed.Code != string(domain.ErrValidation) || parsed.Field != "name" || parsed.Message != "must not be empty" {
		t.Fatalf("Wrap() = %+v, want code/field/message preserved", parsed)
	}
}

func TestWrapDomainErrorWrappedByFmtErrorf(t *testing.T) {
	original := &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
	wrapped := Wrap(fmt.Errorf("loading account: %w", original))
	parsed, ok := Parse(wrapped.Error())
	if !ok {
		t.Fatalf("Parse(%q) failed", wrapped.Error())
	}
	if parsed.Code != string(domain.ErrNotFound) {
		t.Fatalf("Code = %q, want %q (errors.As must unwrap %%w)", parsed.Code, domain.ErrNotFound)
	}
}

func TestWrapGenericErrorNeverLeaksDetail(t *testing.T) {
	sensitive := fmt.Errorf("sqlite: near \"SELECT\": syntax error in /Users/alice/Library/db.sqlite")
	wrapped := Wrap(sensitive)
	parsed, ok := Parse(wrapped.Error())
	if !ok {
		t.Fatalf("Parse(%q) failed", wrapped.Error())
	}
	if parsed.Code != internalCode {
		t.Fatalf("Code = %q, want %q", parsed.Code, internalCode)
	}
	if parsed.Message == sensitive.Error() {
		t.Fatalf("wrapped message must not equal the raw underlying error")
	}
	raw := wrapped.Error()
	if strings.Contains(raw, "sqlite") || strings.Contains(raw, "/Users/alice") {
		t.Fatalf("wrapped error leaked underlying detail: %s", raw)
	}
}

func TestWireErrorErrorIsValidJSON(t *testing.T) {
	wrapped := Wrap(&domain.Error{Code: domain.ErrConflict, Message: "a Household already exists"})
	var decoded map[string]any
	if err := json.Unmarshal([]byte(wrapped.Error()), &decoded); err != nil {
		t.Fatalf("WireError.Error() is not valid JSON: %v", err)
	}
	if decoded["code"] != string(domain.ErrConflict) {
		t.Fatalf("decoded code = %v, want %v", decoded["code"], domain.ErrConflict)
	}
	if _, hasField := decoded["field"]; hasField {
		t.Fatalf("empty field must be omitted from the JSON payload, got %v", decoded)
	}
}

func TestParseMalformedOrNonJSONMessage(t *testing.T) {
	cases := []string{"", "not json at all", "{", `{"message":"missing code"}`}
	for _, message := range cases {
		if _, ok := Parse(message); ok {
			t.Fatalf("Parse(%q) unexpectedly succeeded", message)
		}
	}
}

// Every domain.ErrorCode must reach the frontend with its exact code
// preserved across the Wails error contract.
func TestWrapPreservesEveryDomainErrorCode(t *testing.T) {
	codes := []domain.ErrorCode{
		domain.ErrValidation, domain.ErrNotFound, domain.ErrConflict, domain.ErrUnsupportedDB,
		domain.ErrMigration, domain.ErrIntegrity, domain.ErrUnavailable, domain.ErrDecimalOverflow,
		domain.ErrProviderUnavailable, domain.ErrProviderAuthentication, domain.ErrProviderRateLimit,
		domain.ErrUnsupportedProviderSymbol, domain.ErrMalformedProviderResponse, domain.ErrMarketDataResponseTooLarge,
		domain.ErrHistoryNotStarted, domain.ErrHistoryTimezoneRequired, domain.ErrInvalidChange,
		domain.ErrInvalidChangeTime, domain.ErrNoChange, domain.ErrInsufficientBalance,
		domain.ErrInsufficientQuantity, domain.ErrAlreadyUndone, domain.ErrCannotFixChange,
		domain.ErrTransferMismatch, domain.ErrInvalidTrade, domain.ErrHistoryUpdateFailed,
		domain.ErrCostBasisRequired,
	}
	for _, code := range codes {
		wrapped := Wrap(&domain.Error{Code: code, Message: "message"})
		parsed, ok := Parse(wrapped.Error())
		if !ok {
			t.Fatalf("Parse failed for code %q", code)
		}
		if parsed.Code != string(code) {
			t.Fatalf("Code = %q, want %q", parsed.Code, code)
		}
	}
}
