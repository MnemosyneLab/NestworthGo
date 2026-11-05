package mcpserver

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Discovery size budgets guard against prose creep. They bound what every
// client pays on connection: the serialized tools/list payload per permission
// mode, the longest tool description and the server instructions. Mode ceilings
// are about 2% above the F2 measurement (read_only 63347, directory_write
// 78408, ledger_write 120549). maxToolDescriptionBytes stays a prose cap: the
// longest measured description is 1137 bytes (commit_plan), and fitting that
// cap to 2% would reject the next necessary sentence. Instructions are already
// inside 2% (2373 bytes against 2400) and are not raised. Raising a limit is a
// deliberate review decision and needs a stated reason, not a mechanical bump.
const (
	maxToolDescriptionBytes = 1500
	maxInstructionsBytes    = 2400
)

var discoveryBudgetBytes = map[string]int{
	ReadOnly:       64700,
	DirectoryWrite: 80000,
	LedgerWrite:    123000,
}

func TestDiscoveryBudget(t *testing.T) {
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
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
