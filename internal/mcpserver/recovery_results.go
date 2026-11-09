package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

const recoveryMeta = "nestworth.errorRecovery"

type RecoveryHint struct {
	Action         string `json:"action"`
	AutomaticRetry bool   `json:"automaticRetry"`
	PreserveIntent bool   `json:"preserveIntent"`
	DiscardCapture bool   `json:"discardCapture"`
}
type ErrorRecoveryResult struct {
	SchemaVersion string             `json:"schemaVersion"`
	Error         apierror.WireError `json:"error"`
	Recovery      RecoveryHint       `json:"recovery"`
}

// Hints are conservative navigation, never authority to replay or create a new
// financial intention. Unknown results retain their original code/message.
func errorRecovery(err apierror.WireError) ErrorRecoveryResult {
	action := agentcapability.ReadRetryAction(err.Code, err.Field)
	if action == "stop" {
		action = "inspect_error"
	}
	hint := RecoveryHint{Action: action}
	switch err.Code {
	case "read_expired", "read_revoked", "context_expired", "context_revoked":
		hint.DiscardCapture = true
	case "conflict":
		hint.PreserveIntent = err.Field != "cursor"
	case "operation_outcome_unknown", "operation_pending", "operation_unknown", "stale_preview", "integrity_failed":
		hint.Action = "inspect_operation_and_current_state"
		hint.PreserveIntent = true
	}
	return ErrorRecoveryResult{SchemaVersion: "mcp-error/1", Error: err, Recovery: hint}
}

func attachErrorRecovery(result *mcp.CallToolResult) {
	if result == nil || !result.IsError || len(result.Content) != 1 {
		return
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return
	}
	var wire apierror.WireError
	if jsoncodec.Unmarshal([]byte(content.Text), &wire) != nil || wire.Code == "" || wire.Message == "" {
		return
	}
	if result.Meta == nil {
		result.Meta = mcp.Meta{}
	}
	if _, exists := result.Meta[recoveryMeta]; !exists {
		result.Meta[recoveryMeta] = errorRecovery(wire)
	}
}
func recoveryResult(err error) *mcp.CallToolResult {
	result := &mcp.CallToolResult{}
	result.SetError(err)
	attachErrorRecovery(result)
	return result
}
func structuredRecovery() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err == nil && method == "tools/call" {
				if tool, ok := result.(*mcp.CallToolResult); ok {
					attachErrorRecovery(tool)
				}
			}
			return result, err
		}
	}
}
