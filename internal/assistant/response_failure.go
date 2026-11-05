package assistant

import "strings"

// This closed vocabulary contains code-owned stages/rules only. No provider
// event names, field names/values, bodies, IDs, account facts or secrets escape.
var responseRules = map[string]bool{
	"request_contract":          true,
	"response_headers":          true,
	"response_body":             true,
	"response_terminal":         true,
	"response_item":             true,
	"stream_frame":              true,
	"stream_order":              true,
	"stream_lifecycle":          true,
	"stream_item_added":         true,
	"stream_item_done":          true,
	"stream_function_status":    true,
	"stream_function_arguments": true,
	"stream_function_identity":  true,
	"stream_arguments":          true,
	"stream_part":               true,
	"stream_terminal":           true,
	"stream_event":              true,
	"stream_eof":                true,
	"sdk_conversion":            true,
	"engine_inventory":          true,
	"engine_empty_text":         true,
}

const responseFailurePrefix = "provider_response_rejected."

type responseFailure struct{ rule string }

func (e responseFailure) Error() string { return responseFailurePrefix + e.rule }
func (e responseFailure) Unwrap() error { return ErrProviderResponse }

// ProviderResponseFailure rejects unknown rules instead of reflecting their text.
func ProviderResponseFailure(rule string) error {
	if !responseRules[rule] {
		return ErrProviderResponse
	}
	return responseFailure{rule}
}
func IsResponseFailureCode(code string) bool {
	return strings.HasPrefix(code, responseFailurePrefix) && responseRules[strings.TrimPrefix(code, responseFailurePrefix)]
}
