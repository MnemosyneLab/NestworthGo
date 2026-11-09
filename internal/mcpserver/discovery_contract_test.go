package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// The external discovery contract is locked per tool and permission mode by a
// hash of everything a client can validate or enforce: name, annotations, input
// schema and output schema. Prose (tool and schema-property descriptions) is
// deliberately excluded so guidance can be shortened without blessing a changed
// permission, argument or result shape; prose size is bounded separately by the
// discovery budget test. Changes to the golden file must be made by hand and
// reviewed as an intentional external contract change; there is no regenerate
// switch.
const discoveryContractGolden = "testdata/discovery-contract.json"

type discoveryContract map[string]map[string]string

// stripProse removes JSON Schema description strings but keeps a property that
// happens to be named "description".
func stripProse(value any, insideProperties bool) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if !insideProperties && key == "description" {
				if _, isText := child.(string); isText {
					continue
				}
			}
			out[key] = stripProse(child, !insideProperties && key == "properties")
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = stripProse(child, false)
		}
		return out
	}
	return value
}

func contractHash(t *testing.T, tool any) string {
	t.Helper()
	raw, err := jsoncodec.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := jsoncodec.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	delete(object, "description")
	projected, err := jsoncodec.MarshalIndent(stripProse(object, false), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(projected)
	return hex.EncodeToString(sum[:])
}

func discoveryContractHashes(t *testing.T, tools []*mcp.Tool) map[string]string {
	t.Helper()
	hashes := make(map[string]string, len(tools))
	for _, tool := range tools {
		if _, duplicate := hashes[tool.Name]; duplicate {
			t.Fatalf("duplicate discovered tool %s", tool.Name)
		}
		hashes[tool.Name] = contractHash(t, tool)
	}
	return hashes
}

func TestExternalDiscoveryContractSnapshot(t *testing.T) {
	raw, err := os.ReadFile(discoveryContractGolden)
	if err != nil {
		t.Fatal(err)
	}
	var golden discoveryContract
	if err := jsoncodec.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden) != 2 || golden[ReadOnly] == nil || golden[ReadWrite] == nil {
		t.Fatal("discovery fixture must cover exactly read_only and read_write")
	}
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
			got := discoveryContractHashes(t, listed.Tools)
			want := golden[mode]
			for name, hash := range want {
				if got[name] == "" {
					t.Errorf("%s: tool %s is missing from discovery", mode, name)
				} else if got[name] != hash {
					t.Errorf("%s: tool %s changed its name/annotation/schema contract: sha256=%s", mode, name, got[name])
				}
			}
			for name := range got {
				if want[name] == "" {
					t.Errorf("%s: unreviewed tool %s: sha256=%s", mode, name, got[name])
				}
			}
			// Instructions are shared by the external and in-process adapters.
			registry := s.toolRegistry(mode)
			if client.InitializeResult().Instructions != registry.Instructions() {
				t.Error("shared/external instructions differ")
			}
			definitions := registry.Definitions()
			if len(definitions) != len(listed.Tools) {
				t.Fatal("local registry lost tools")
			}
			sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
			sort.Slice(listed.Tools, func(i, j int) bool { return listed.Tools[i].Name < listed.Tools[j].Name })
			for i, d := range definitions {
				local, err := jsoncodec.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
				// Effect is host policy and intentionally absent from the wire.
				if !reflect.DeepEqual(jsonObject(t, json.RawMessage(local)), jsonObject(t, listed.Tools[i])) {
					t.Errorf("%s local/external metadata differ", d.Name)
				}
				if listed.Tools[i].Annotations.IdempotentHint && d.Effect != agentcapability.Write {
					t.Errorf("%s mutation lacks a write boundary", d.Name)
				}
			}
		})
	}
}
