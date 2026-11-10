// Package agentcapability defines the transport-independent, typed allowlist
// shared by agent adapters. It has no HTTP, MCP, database or model dependency.
package agentcapability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type Effect string

const (
	Interaction Effect = "interaction" // Desktop control; host classifies its actual side effects.
	Read        Effect = "read"
	Analysis    Effect = "analysis" // Derived snapshots only; may invalidate previews.
	Preview     Effect = "preview"  // Private plan only, never approval.
	Write       Effect = "write"
)

// Annotations preserve the existing external discovery contract. Effect is the
// host's authority; advisory protocol annotations do not grant permission.
type Annotations struct {
	ReadOnlyHint    bool  `json:"readOnlyHint"`
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	IdempotentHint  bool  `json:"idempotentHint"`
	OpenWorldHint   *bool `json:"openWorldHint,omitempty"`
}

type Definition struct {
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	InputSchema  any          `json:"inputSchema"`
	OutputSchema any          `json:"outputSchema,omitempty"`
	Annotations  *Annotations `json:"annotations,omitempty"`
	Effect       Effect       `json:"-"`
}

type entry struct {
	definition Definition
	input      *jsonschema.Resolved
	output     *jsonschema.Resolved
	handler    func(context.Context, json.RawMessage) (any, error)
	typed      any
}

// Registry is built once before publication, then read concurrently. Returned
// definitions are detached copies so adapters cannot weaken validation/policy.
type Registry struct {
	instructions string
	entries      map[string]entry
}

func New(instructions string) *Registry {
	return &Registry{instructions: instructions, entries: map[string]entry{}}
}

func (r *Registry) Instructions() string { return r.instructions }

var ErrUnknownTool = errors.New("unknown capability")

// InvalidArguments excludes submitted values from its public error. The MCP
// adapter retains SDK validation/errors before entering the shared dispatcher.
var ErrInvalidArguments = errors.New("invalid capability arguments")

var ErrInvalidResult = errors.New("invalid capability result")

func Add[I, O any](r *Registry, definition *Definition, handler func(context.Context, I) (O, error)) {
	if definition == nil || definition.Name == "" || handler == nil {
		panic("invalid capability registration")
	}
	if _, exists := r.entries[definition.Name]; exists {
		panic("duplicate capability: " + definition.Name)
	}
	switch definition.Effect {
	case Read, Analysis, Preview, Write:
	default:
		panic("capability requires explicit effect: " + definition.Name)
	}
	if definition.Annotations != nil && definition.Annotations.IdempotentHint && definition.Effect != Write {
		panic("mutation registration requires write effect: " + definition.Name)
	}
	d := clone(*definition)
	input, err := inferSchema[I](d.InputSchema)
	if err != nil {
		panic(fmt.Sprintf("%s input schema: %v", d.Name, err))
	}
	normalizeNullableInputSchema(input)
	if input.Type != "object" {
		panic("capability input must be an object: " + d.Name)
	}
	d.InputSchema = input
	resolved, err := input.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		panic(fmt.Sprintf("%s input schema: %v", d.Name, err))
	}
	var outputResolved *jsonschema.Resolved
	if d.OutputSchema != nil || reflect.TypeFor[O]() != reflect.TypeFor[any]() {
		output, err := inferSchema[O](d.OutputSchema)
		if err != nil {
			panic(fmt.Sprintf("%s output schema: %v", d.Name, err))
		}
		outputResolved, err = output.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
		if err != nil {
			panic(fmt.Sprintf("%s output schema: %v", d.Name, err))
		}
		d.OutputSchema = output
	}
	r.entries[d.Name] = entry{definition: d, input: resolved, output: outputResolved, typed: handler, handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var input I
		if err := jsoncodec.Unmarshal(raw, &input); err != nil {
			return nil, ErrInvalidArguments
		}
		return handler(ctx, input)
	}}
}

// CallValidated is for a typed adapter that has already validated and decoded
// against Definition.InputSchema. It preserves that adapter's existing codec
// and error contract while dispatching through the same registered handler.
func CallValidated[I, O any](ctx context.Context, r *Registry, name string, input I) (O, error) {
	var zero O
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	e, ok := r.entries[name]
	if !ok {
		return zero, ErrUnknownTool
	}
	handler, ok := e.typed.(func(context.Context, I) (O, error))
	if !ok {
		return zero, errors.New("capability adapter type mismatch")
	}
	return handler(ctx, input)
}

func inferSchema[T any](supplied any) (*jsonschema.Schema, error) {
	if supplied != nil {
		raw, err := jsoncodec.Marshal(supplied)
		if err != nil {
			return nil, err
		}
		var schema jsonschema.Schema
		if err := jsoncodec.Unmarshal(raw, &schema); err != nil {
			return nil, err
		}
		return &schema, nil
	}
	t := reflect.TypeFor[T]()
	if t == reflect.TypeFor[any]() {
		return &jsonschema.Schema{Type: "object"}, nil
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return jsonschema.ForType(t, &jsonschema.ForOptions{TypeSchemas: patchTypeSchemas()})
}

func clone(d Definition) Definition {
	// Metadata contains only JSON-compatible values. Registration errors should
	// fail at construction rather than publish partly shared policy objects.
	effect := d.Effect
	raw, err := jsoncodec.Marshal(d)
	if err != nil {
		panic(err)
	}
	var detached Definition
	if err := decodeExact(raw, &detached); err != nil {
		panic(err)
	}
	detached.Effect = effect
	return detached
}

func (r *Registry) Definitions() []Definition {
	definitions := make([]Definition, 0, len(r.entries))
	for _, e := range r.entries {
		definitions = append(definitions, clone(e.definition))
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	return definitions
}

func (r *Registry) Definition(name string) (Definition, error) {
	e, ok := r.entries[name]
	if !ok {
		return Definition{}, ErrUnknownTool
	}
	return clone(e.definition), nil
}

// Call validates before invoking exactly one allowlisted typed handler. Input
// defaults match the existing SDK object-schema semantics. Authorization and
// lifetime fences belong to the owning session, before this dispatcher.
func (r *Registry) Call(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	return r.CallObserved(ctx, name, raw, nil)
}

// CallObserved reports actual handler entry, not merely arrival at dispatch.
// The observation belongs to this serial call and grants no additional authority.
func (r *Registry) CallObserved(ctx context.Context, name string, raw json.RawMessage, entered *bool) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e, ok := r.entries[name]
	if !ok {
		return nil, ErrUnknownTool
	}
	input, err := r.validatedInput(name, raw)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if entered != nil {
		*entered = true
	}
	result, err := e.handler(ctx, input)
	if err != nil || e.output == nil {
		return result, err
	}
	encoded, err := jsoncodec.Marshal(result)
	if err != nil {
		return nil, ErrInvalidResult
	}
	var output any
	if err := decodeExact(encoded, &output); err != nil {
		return nil, ErrInvalidResult
	}
	if _, ok := output.(map[string]any); ok {
		if err := e.output.ApplyDefaults(&output); err != nil {
			return nil, ErrInvalidResult
		}
	}
	validation, err := validationView(output)
	if err != nil {
		return nil, ErrInvalidResult
	}
	if err := e.output.Validate(&validation); err != nil {
		return nil, ErrInvalidResult
	}
	// Return the validated/defaulted JSON as an immutable capture, detached from
	// application pointers and from the next invocation's destination values.
	encoded, err = jsoncodec.Marshal(output)
	if err != nil {
		return nil, ErrInvalidResult
	}
	return json.RawMessage(encoded), nil
}

// ValidateArguments checks the same defaults/schema as dispatch without invoking a handler.
func (r *Registry) ValidateArguments(name string, raw json.RawMessage) error {
	_, err := r.validatedInput(name, raw)
	return err
}
func (r *Registry) validatedInput(name string, raw json.RawMessage) ([]byte, error) {
	e, ok := r.entries[name]
	if !ok {
		return nil, ErrUnknownTool
	}
	values := map[string]any{}
	if len(raw) > 0 {
		if err := decodeExact(raw, &values); err != nil {
			return nil, ErrInvalidArguments
		}
	}
	if values == nil {
		return nil, ErrInvalidArguments
	}
	var value any = values
	if err := e.input.ApplyDefaults(&value); err != nil {
		return nil, ErrInvalidArguments
	}
	validation, err := validationView(value)
	if err != nil {
		return nil, ErrInvalidArguments
	}
	if err := e.input.Validate(&validation); err != nil {
		return nil, argumentError(e.definition.InputSchema, validation)
	}
	input, err := jsoncodec.Marshal(value)
	if err != nil {
		return nil, ErrInvalidArguments
	}
	return input, nil
}

func decodeExact(raw []byte, target any) error {
	decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalidArguments
	}
	return nil
}

// jsonschema-go v0.4.3 uses reflect.Kind for JSON type checks, where json.Number
// appears as a string. Validate a detached numeric view, retaining the original
// lexemes for typed decoding and returned evidence. Integers never pass through
// float64. Nonintegral numbers use the library's finite-number semantics, and
// representations that would change their integer/noninteger type are refused.
func validationView(value any) (any, error) {
	switch value := value.(type) {
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return n, nil
		}
		if n, err := strconv.ParseUint(value.String(), 10, 64); err == nil {
			return n, nil
		}
		f, err := value.Float64()
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, ErrInvalidArguments
		}
		// Avoid materializing arbitrary exponent-sized big integers. Extremely
		// long numeric forms are unnecessary for this bounded capability surface.
		if len(value.String()) > 1024 {
			return nil, ErrInvalidArguments
		}
		if at := strings.IndexAny(value.String(), "eE"); at >= 0 {
			exponent, err := strconv.ParseInt(value.String()[at+1:], 10, 32)
			if err != nil || exponent < -1024 || exponent > 1024 {
				return nil, ErrInvalidArguments
			}
		}
		rational, ok := new(big.Rat).SetString(value.String())
		if !ok {
			return nil, ErrInvalidArguments
		}
		if rational.IsInt() {
			if rational.Num().IsInt64() {
				return rational.Num().Int64(), nil
			}
			if rational.Num().IsUint64() {
				return rational.Num().Uint64(), nil
			}
			return nil, ErrInvalidArguments
		}
		if math.Trunc(f) == f {
			return nil, ErrInvalidArguments
		}
		return f, nil
	case map[string]any:
		copy := make(map[string]any, len(value))
		for key, item := range value {
			converted, err := validationView(item)
			if err != nil {
				return nil, err
			}
			copy[key] = converted
		}
		return copy, nil
	case []any:
		copy := make([]any, len(value))
		for i, item := range value {
			converted, err := validationView(item)
			if err != nil {
				return nil, err
			}
			copy[i] = converted
		}
		return copy, nil
	default:
		return value, nil
	}
}
