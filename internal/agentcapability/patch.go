package agentcapability

import (
	"bytes"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// PatchString is a JSON string for PATCH tools. A Go pointer cannot tell an
// omitted field from JSON null, and both decode as nil. The zero value means
// the field was omitted. JSON null is a present clear. A JSON string, including
// an empty one, is a present value; callers decide whether that empty string is
// valid. The published schema is anyOf string|null, not a nullable type union,
// so the nullable-schema pass leaves the null branch in place.
type PatchString struct {
	present bool
	clear   bool
	value   string
}

// Present reports whether the JSON field was included.
func (p PatchString) Present() bool { return p.present }

// Null reports whether the field was JSON null.
func (p PatchString) Null() bool { return p.present && p.clear }

// Value reports a present JSON string. Null and omission return false.
func (p PatchString) Value() (string, bool) {
	if !p.present || p.clear {
		return "", false
	}
	return p.value, true
}

// IsZero reports omission so omitzero drops the field when a request is
// re-encoded. A clear is present and must stay JSON null.
func (p PatchString) IsZero() bool { return !p.present }

func (p PatchString) MarshalJSON() ([]byte, error) {
	if !p.present || p.clear {
		return []byte("null"), nil
	}
	return jsoncodec.Marshal(p.value)
}

func (p *PatchString) UnmarshalJSON(data []byte) error {
	p.present = true
	p.value = ""
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		p.clear = true
		return nil
	}
	p.clear = false
	return jsoncodec.Unmarshal(data, &p.value)
}

func patchTypeSchemas() map[reflect.Type]*jsonschema.Schema {
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[PatchString](): {
			AnyOf: []*jsonschema.Schema{{Type: "string"}, {Type: "null"}},
		},
	}
}
