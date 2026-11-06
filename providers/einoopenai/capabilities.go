package einoopenai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// CapabilitySession is the model-facing, in-process surface. InProcessSession
// implements it without an MCP client, listener, bearer token or loopback HTTP.
// Host review/confirmation methods deliberately are not part of this interface.
// It remains household-wide and must not be given to a mobile/scoped principal.
type CapabilitySession interface {
	Definitions() ([]agentcapability.Definition, error)
	Call(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

// CapabilityTools constructs Eino tools from local definitions. The desktop
// Full adapter can advertise the complete shared registry. The session must also enforce its current permissions at every
// Call; a discovery snapshot is never authorization. Unsupported/lossy schema
// conversion fails closed, while the session remains the argument validator.
func CapabilityTools(session CapabilitySession) ([]tool.BaseTool, error) {
	if session == nil || (reflect.ValueOf(session).Kind() == reflect.Pointer && reflect.ValueOf(session).IsNil()) {
		return nil, errors.New("local capability session is required")
	}
	definitions, err := session.Definitions()
	if err != nil {
		return nil, errors.New("local capability discovery unavailable")
	}
	full := false
	if granted, ok := session.(interface{ FullAccessEnabled() bool }); ok {
		full = granted.FullAccessEnabled()
	}
	if len(definitions) > 128 {
		return nil, errors.New("local capability inventory exceeds budget")
	}
	out := make([]tool.BaseTool, 0, len(definitions))
	seen := map[string]bool{}
	for _, d := range definitions {
		if d.Name == "" || seen[d.Name] || (d.Effect != agentcapability.Read && d.Effect != agentcapability.Analysis && d.Effect != agentcapability.Preview && !((full || d.Name == "execute_ledger_change" || d.Name == "operate_page_control") && d.Effect == agentcapability.Write)) ||
			d.Annotations == nil || d.Annotations.OpenWorldHint == nil || (*d.Annotations.OpenWorldHint && !full) {
			return nil, errors.New("unsafe local capability inventory")
		}
		seen[d.Name] = true
		raw, err := jsoncodec.Marshal(d.InputSchema)
		if err != nil || len(raw) > 64<<10 {
			return nil, errors.New("local capability schema exceeds budget")
		}
		var converted jsonschema.Schema
		if jsoncodec.Unmarshal(raw, &converted) != nil {
			return nil, errors.New("unsupported local capability schema")
		}
		roundtrip, err := jsoncodec.Marshal(&converted)
		if err != nil || !sameSchemaJSON(raw, roundtrip) {
			return nil, errors.New("lossy local capability schema conversion")
		}
		out = append(out, &capabilityTool{session: session, name: d.Name, description: d.Description, parameters: raw, writing: d.Effect == agentcapability.Write})
	}
	return out, nil
}

func sameSchemaJSON(a, b []byte) bool {
	decode := func(raw []byte) (any, error) {
		decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		err := decoder.Decode(&value)
		return value, err
	}
	x, errA := decode(a)
	y, errB := decode(b)
	return errA == nil && errB == nil && reflect.DeepEqual(x, y)
}

var errCapabilityWriteRecoveryRequired = errors.New("existing operation requires recovery")

var errCapabilityWriteRejected = errors.New("local write rejected before dispatch")

var errCapabilityWriteRefused = errors.New("local write outcome requires inspection")

var errCapabilityRefused = errors.New("local capability request refused")

type capabilityTool struct {
	writing     bool
	session     CapabilitySession
	name        string
	description string
	parameters  json.RawMessage
}

func (t *capabilityTool) Info(context.Context) (*schema.ToolInfo, error) {
	// Return detached metadata so caller changes cannot modify later requests.
	var parameters jsonschema.Schema
	if jsoncodec.Unmarshal(t.parameters, &parameters) != nil {
		return nil, errors.New("local capability schema unavailable")
	}
	return &schema.ToolInfo{Name: t.name, Desc: t.description, ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&parameters)}, nil
}

func (t *capabilityTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	if ctx.Err() != nil {
		return "", errors.New("local capability request cancelled")
	}
	if len(arguments) > 64<<10 {
		if t.writing {
			return "", errCapabilityWriteRejected
		}
		return "", errors.New("local capability arguments exceed budget")
	}
	result, err := t.session.Call(ctx, t.name, json.RawMessage(arguments))
	if budget := assistant.NormalizeBudgetFailure(err); budget != nil {
		return "", budget
	}
	if err != nil {
		// Do not reflect submitted content or arbitrary implementation errors
		// into model history. Application-facing errors belong to the host.
		if t.writing {
			if errors.Is(err, assistant.ErrRecoveryRequired) {
				return "", errCapabilityWriteRecoveryRequired
			}
			var unsubmitted interface{ NotDispatched() bool }
			if errors.As(err, &unsubmitted) && unsubmitted.NotDispatched() {
				return "", errCapabilityWriteRejected
			}
			return "", errCapabilityWriteRefused
		}
		return "", errCapabilityRefused
	}
	if len(result) > 64<<10 {
		return "", assistant.BudgetFailure("tool_result_bytes")
	}
	if !jsoncodec.Valid(result) {
		return "", errors.New("local capability result unavailable")
	}
	return string(result), nil
}

// Shared with the engine so proof diagnostics are tested without a live provider.
const writeNotSubmittedDiagnostic = `{"error":{"code":"write_not_submitted","recovery":"This attempt was rejected before business dispatch. Review the arguments and permission before a new request. This does not resolve an older operation's uncertain receipt."}}`
