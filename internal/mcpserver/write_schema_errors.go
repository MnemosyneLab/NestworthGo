package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// The SDK validates arguments before dispatch and may echo rejected values.
// Scrub only its unstructured write-tool errors, never a success, business
// error, execution receipt or unknown outcome. This needs no response buffer.
func writeSchemaErrors(set *toolSet) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, request)
			if err != nil || method != "tools/call" {
				return result, err
			}
			call, ok := request.(*mcp.CallToolRequest)
			if !ok || call.Params == nil {
				return result, err
			}
			d, e := set.Definition(call.Params.Name)
			if e != nil || d.Effect != agentcapability.Write {
				return result, err
			}
			reply, ok := result.(*mcp.CallToolResult)
			if !ok || !reply.IsError {
				return result, err
			}
			if len(reply.Content) == 1 {
				if text, ok := reply.Content[0].(*mcp.TextContent); ok {
					var safe struct{ Code, Message string }
					if jsoncodec.Unmarshal([]byte(text.Text), &safe) == nil && safe.Code != "" && safe.Message != "" {
						return result, err
					}
				}
			}
			cleaned := &mcp.CallToolResult{}
			cleaned.SetError(fail("validation", "invalid write arguments; inspect tool schema; no business execution occurred"))
			return cleaned, nil
		}
	}
}
