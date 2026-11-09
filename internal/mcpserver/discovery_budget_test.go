package mcpserver

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Keep discovery and prose bounded for both public permission modes. Read-write
// includes review management. The read-only and prose ceilings are unchanged.
const (
	maxToolDescriptionBytes = 1500
	maxInstructionsBytes    = 2400
)

var discoveryBudgetBytes = map[string]int{
	ReadOnly:  64700,
	ReadWrite: 116400,
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
			for _, tool := range listed.Tools {
				if len(tool.Description) > maxToolDescriptionBytes {
					t.Errorf("%s description is %d bytes, budget %d", tool.Name, len(tool.Description), maxToolDescriptionBytes)
				}
			}
		})
	}
}
