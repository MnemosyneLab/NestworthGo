package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Keep discovery and prose bounded for both public permission modes. Read-write
// includes review management. The explicit finite review spec adds 4,628 bytes
// per preview/create/update definition. Ledger enums and variant exclusions
// add about 3.7 KiB across preview_batch/correction; unrelated budgets stay bounded.
const (
	maxToolDescriptionBytes = 1500
	maxInstructionsBytes    = 2400
)

var discoveryBudgetBytes = map[string]int{
	ReadOnly:  65300,
	ReadWrite: 135000,
}

func TestDiscoveryBudget(t *testing.T) {
	for _, mode := range []string{ReadOnly, ReadWrite} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			s, _, _ := fixture(t)
			if _, err := s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			client := connect(t, s)
			listed, err := listAllTools(t.Context(), client)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := jsoncodec.Marshal(listed.Tools)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: tools=%d tools/list bytes=%d instructions=%d", mode, len(listed.Tools), len(raw), len(client.InitializeResult().Instructions))
			if len(raw) > discoveryBudgetBytes[mode] {
				t.Errorf("tools/list is %d bytes, budget %d", len(raw), discoveryBudgetBytes[mode])
			}
			if n := len(client.InitializeResult().Instructions); n > maxInstructionsBytes {
				t.Errorf("instructions are %d bytes, budget %d", n, maxInstructionsBytes)
			}
			// The measured increase belongs only to the three explicit rule
			// and ledger schemas. Unrelated discovery must still fit its previous ceiling.
			other := []*mcp.Tool{}
			for _, tool := range listed.Tools {
				switch tool.Name {
				case "preview_batch", "preview_correction", "preview_investment_review_rule", "create_investment_review_rule", "update_investment_review_rule":
					schema, err := jsoncodec.Marshal(tool.InputSchema)
					if err != nil || len(schema) > 6000 {
						t.Errorf("%s finite spec exceeds 6000 bytes", tool.Name)
					}
				default:
					other = append(other, tool)
				}
				if len(tool.Description) > maxToolDescriptionBytes {
					t.Errorf("%s description is %d bytes, budget %d", tool.Name, len(tool.Description), maxToolDescriptionBytes)
				}
			}
			remainder, err := jsoncodec.Marshal(other)
			oldBudget := map[string]int{ReadOnly: 64700, ReadWrite: 116400}[mode]
			if err != nil || len(remainder) > oldBudget {
				t.Error("unrelated discovery exceeded its existing budget")
			}
		})
	}
}
