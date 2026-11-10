package einoopenai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

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
		if d.Name == "" || d.Name == loadToolGroups || seen[d.Name] || (d.Effect != agentcapability.Read && d.Effect != agentcapability.Analysis && d.Effect != agentcapability.Preview && !(d.Name == "operate_page" && d.Effect == agentcapability.Interaction) && !((full || d.Name == "operate_page") && d.Effect == agentcapability.Write)) ||
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
		out = append(out, &capabilityTool{session: session, name: d.Name, description: d.Description, parameters: raw, group: d.ToolGroup(), writing: d.Effect == agentcapability.Write || d.Effect == agentcapability.Interaction})
	}
	return out, nil
}

// catalogInfos deliberately constructs no invokable tools and has no session.
// Metadata comes from the host catalog, never from saved tool descriptions.
func catalogInfos(definitions []agentcapability.Definition) ([]*schema.ToolInfo, error) {
	if len(definitions) > 128 {
		return nil, errors.New("catalog exceeds budget")
	}
	infos := make([]*schema.ToolInfo, 0, len(definitions))
	seen := map[string]bool{}
	for _, d := range definitions {
		if d.Name == "" || seen[d.Name] {
			return nil, errors.New("invalid catalog")
		}
		seen[d.Name] = true
		raw, err := jsoncodec.Marshal(d.InputSchema)
		if err != nil || len(raw) > 64<<10 {
			return nil, errors.New("invalid catalog schema")
		}
		var parameters jsonschema.Schema
		if jsoncodec.Unmarshal(raw, &parameters) != nil {
			return nil, errors.New("invalid catalog schema")
		}
		roundtrip, err := jsoncodec.Marshal(&parameters)
		if err != nil || !sameSchemaJSON(raw, roundtrip) {
			return nil, errors.New("lossy catalog schema")
		}
		infos = append(infos, &schema.ToolInfo{Name: d.Name, Desc: d.Description, ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&parameters)})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos, nil
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
	group       agentcapability.ToolGroup
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
	if len(arguments) > toolArgumentLimit(t.name) {
		if t.name == "operate_page" {
			return "", errPageOperationRefused
		}
		if t.writing {
			return "", submissionFailure{cause: newReadFailure("too_large", "")}
		}
		return "", newReadFailure("too_large", "")
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
			var recorded agentcapability.WriteFailure
			if errors.As(err, &recorded) {
				return "", recordedWriteFailure{cause: classifyReadFailure(recorded.Cause), operationID: recorded.OperationID}
			}
			var unsubmitted interface{ NotDispatched() bool }
			if errors.As(err, &unsubmitted) && unsubmitted.NotDispatched() {
				if t.name == "operate_page" {
					return "", submissionFailure{cause: classifyReadFailure(err), page: true}
				}
				return "", submissionFailure{cause: classifyReadFailure(err)}
			}
			return "", errCapabilityWriteRefused
		}
		return "", classifyReadFailure(err)
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

var errPageOperationRefused = errors.New("page operation refused before dispatch")

const pageOperationRefusedDiagnostic = `{"state":"not_dispatched","error":{"code":"page_operation_refused","recovery":"No event was dispatched. Read fresh page content and check the target, action and current permission before a new intentional operation. Do not replay an uncertain earlier submission."}}`

// toolArgumentLimit is shared by wire validation and dispatch. A page batch
// contains up to 20 values of 8192 UTF-8 bytes; ordinary tools stay at 64 KiB.
func toolArgumentLimit(name string) int {
	if name == "operate_page" {
		return 256 << 10
	}
	return 64 << 10
}

// A proven pre-dispatch refusal can include safe repair information. Unknown
// outcomes never use this type and retain the inspection-only response.
type submissionFailure struct {
	cause readFailure
	page  bool
}

func (e submissionFailure) Error() string { return e.Unwrap().Error() }
func (e submissionFailure) Unwrap() error {
	if e.page {
		return errPageOperationRefused
	}
	return errCapabilityWriteRejected
}
func submissionDiagnostic(err error, fallback string) string {
	var failure submissionFailure
	if !errors.As(err, &failure) {
		return fallback
	}
	detail := failure.cause
	reasonCode := detail.Code
	detail.Code = "write_not_submitted"
	detail.Recovery += " This attempt did not dispatch; it does not resolve an older uncertain operation. Never retry an older uncertain write through another tool."
	if failure.page {
		detail.Code = "page_operation_refused"
	}
	raw, _ := jsoncodec.Marshal(map[string]any{"state": "not_dispatched", "error": detail, "causeCode": reasonCode})
	return string(raw)
}

var errRecordedWriteFailure = errors.New("write has a durable failed receipt")

type recordedWriteFailure struct {
	cause       readFailure
	operationID string
}

func (e recordedWriteFailure) Error() string { return errRecordedWriteFailure.Error() }
func (e recordedWriteFailure) Unwrap() error { return errRecordedWriteFailure }
func recordedWriteDiagnostic(err error) string {
	var failure recordedWriteFailure
	if !errors.As(err, &failure) {
		return ""
	}
	detail := failure.cause
	cause := detail.Code
	detail.Code = "write_failed"
	detail.RetryAction = "inspect_failed_operation"
	detail.Recovery = "The exact operation has a durable failed receipt. Explain its cause and inspect current state before correcting the intent. Do not automatically replay or replace this write; this says nothing about other uncertain operations."
	raw, _ := jsoncodec.Marshal(map[string]any{"state": "failed", "operationId": failure.operationID, "causeCode": cause, "error": detail})
	return string(raw)
}
