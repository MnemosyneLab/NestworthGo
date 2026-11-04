package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
)

// toolSet is only a construction adapter. Business registrations and handlers
// use transport-independent definitions; no SDK server exists for local calls.
type toolSet struct {
	*agentcapability.Registry
	server *mcp.Server
}

func newToolSet(instructions string, external bool) *toolSet {
	set := &toolSet{Registry: agentcapability.New(instructions)}
	if external {
		set.server = mcp.NewServer(&mcp.Implementation{Name: "Nestworth", Version: "2.0.0"}, &mcp.ServerOptions{Instructions: instructions})
	}
	return set
}

func registerTool[I any](set *toolSet, definition *agentcapability.Definition, handler func(context.Context, I) (Response, error)) {
	agentcapability.Add(set.Registry, definition, handler)
	if set.server == nil {
		return
	}
	d, err := set.Definition(definition.Name)
	if err != nil {
		panic(err)
	}
	tool := &mcp.Tool{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema, OutputSchema: d.OutputSchema}
	if a := d.Annotations; a != nil {
		tool.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: a.ReadOnlyHint, DestructiveHint: a.DestructiveHint, IdempotentHint: a.IdempotentHint, OpenWorldHint: a.OpenWorldHint}
	}
	// Keep the SDK's original typed validation, defaults, decoding, structured
	// errors and response envelope. External tools acquire no App approval gate.
	mcp.AddTool(set.server, tool, func(ctx context.Context, _ *mcp.CallToolRequest, input I) (*mcp.CallToolResult, Response, error) {
		output, err := agentcapability.CallValidated[I, Response](ctx, set.Registry, d.Name, input)
		return nil, output, err
	})
}

func (s *Service) tools(mode string) *mcp.Server { return s.buildToolSet(mode, true).server }

func (s *Service) toolRegistry(mode string) *agentcapability.Registry {
	return s.buildToolSet(mode, false).Registry
}
