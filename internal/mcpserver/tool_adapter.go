package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
)

const mcpImplementationVersion = "3.0.0"

// toolSet is only a construction adapter. Business registrations and handlers
// use transport-independent definitions; no SDK server exists for local calls.
type toolSet struct {
	*agentcapability.Registry
	server *mcp.Server
	pages  map[string]func(context.Context, []byte, int) (any, error)
	paging *pageCodec
}

func newToolSet(instructions string, external bool) *toolSet {
	set := &toolSet{paging: &pageCodec{key: newBoundedReadCache().key}, Registry: agentcapability.New(instructions), pages: map[string]func(context.Context, []byte, int) (any, error){}}
	if external {
		set.server = mcp.NewServer(&mcp.Implementation{Name: "Nestworth", Version: mcpImplementationVersion}, &mcp.ServerOptions{Instructions: instructions, PageSize: 16})
	}
	return set
}

func registerTool[I any](set *toolSet, definition *agentcapability.Definition, handler func(context.Context, I) (Response, error)) {
	if definition.OutputSchema == nil && definition.Effect != agentcapability.Write {
		definition.OutputSchema = readOutputSchema(definition.Name)
	}
	agentcapability.Add(set.Registry, definition, handler)
	if set.server == nil {
		return
	}
	d, err := set.Definition(definition.Name)
	if err != nil {
		panic(err)
	}
	tool := sdkTool(d)
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

// Derive production response policy from the authoritative effect allowlist.
// Execution/recovery receipts never enter the read buffer, even as tools grow.
func (set *toolSet) readResponseTools() map[string]bool {
	names := map[string]bool{}
	for _, d := range set.Definitions() {
		names[d.Name] = d.Effect != agentcapability.Write && d.Name != "get_operation"
	}
	return names
}

func sdkTool(d agentcapability.Definition) *mcp.Tool {
	tool := &mcp.Tool{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema, OutputSchema: d.OutputSchema}
	if a := d.Annotations; a != nil {
		tool.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: a.ReadOnlyHint, DestructiveHint: a.DestructiveHint, IdempotentHint: a.IdempotentHint, OpenWorldHint: a.OpenWorldHint}
	}
	return tool
}
