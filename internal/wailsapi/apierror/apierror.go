// Package apierror implements the stable error contract every
// internal/wailsapi service uses to cross the Wails IPC boundary. See
// docs/migration/wails-v3-technical-design.md (Sec5, "Error contract").
package apierror

import (
	"encoding/json"
	"errors"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// WireError is the JSON shape every internal/wailsapi method returns for a
// non-nil error. Wails v3 propagates a Go method's error return to
// JavaScript as a rejected Promise whose message is err.Error(); WireError's
// Error() method returns its own JSON encoding so the frontend can parse a
// stable, structured payload instead of an English sentence.
type WireError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// Error returns the JSON encoding of the wire error. Marshaling this fixed,
// three-string-field shape never fails.
func (e *WireError) Error() string {
	if e == nil {
		return ""
	}
	b, err := json.Marshal(e)
	if err != nil {
		return e.Message
	}
	return string(b)
}

// internalCode is the stable code used for any error that is not a
// *domain.Error, e.g. a programming bug or an unexpected infrastructure
// failure. It deliberately carries no underlying error text: a raw Go error
// string can contain a SQL fragment, a file path, or other detail that must
// never reach the frontend.
const internalCode = "internal"

// Wrap converts any error into the stable WireError shape. A *domain.Error
// (including one wrapped by fmt.Errorf/%w or errors.Join) keeps its
// Code/Field/Message; any other error becomes a generic "internal" error so
// no raw Go error string, SQL detail, stack trace, or file path ever
// reaches the frontend. Wrap(nil) returns nil.
func Wrap(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *domain.Error
	if errors.As(err, &domainErr) && domainErr != nil {
		return &WireError{Code: string(domainErr.Code), Field: domainErr.Field, Message: domainErr.Message}
	}
	return &WireError{Code: internalCode, Message: "an unexpected error occurred"}
}

// Parse recovers a WireError from its own Error() JSON encoding. It is used
// by internal/wailsapi tests to assert on the Code/Field a service method
// returned; the frontend performs the equivalent parse of a rejected
// Promise's message in TypeScript (technical design Sec5).
func Parse(message string) (*WireError, bool) {
	var parsed WireError
	if err := json.Unmarshal([]byte(message), &parsed); err != nil {
		return nil, false
	}
	if parsed.Code == "" {
		return nil, false
	}
	return &parsed, true
}
