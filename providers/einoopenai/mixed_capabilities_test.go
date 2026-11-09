package einoopenai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

type mixedCapabilitySession struct {
	full  bool
	calls []string
}

func (s *mixedCapabilitySession) FullAccessEnabled() bool { return s.full }
func (s *mixedCapabilitySession) Definitions() ([]agentcapability.Definition, error) {
	closed, open := false, true
	definitions := []agentcapability.Definition{{Name: "operate_page", Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{OpenWorldHint: &closed}, InputSchema: map[string]any{"type": "object"}}}
	if s.full {
		definitions = append(definitions, agentcapability.Definition{Name: "refresh_market_data", Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{OpenWorldHint: &open}, InputSchema: map[string]any{"type": "object"}})
	}
	return definitions, nil
}
func (s *mixedCapabilitySession) Call(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	s.calls = append(s.calls, name)
	if name == "operate_page" {
		return nil, assistant.ErrRecoveryRequired
	}
	return []byte(`{"state":"succeeded"}`), nil
}
func TestMixedFullAndPageCapabilitiesKeepWriteDiagnostics(t *testing.T) {
	for _, full := range []bool{false, true} {
		session := &mixedCapabilitySession{full: full}
		tools, err := CapabilityTools(session)
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if full {
			want = 2
		}
		if len(tools) != want {
			t.Fatal("lost mixed inventory")
		}
		for _, entry := range tools {
			info, err := entry.Info(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			_, err = entry.(tool.InvokableTool).InvokableRun(t.Context(), "{}")
			if info.Name == "operate_page" && !errors.Is(err, errCapabilityWriteRecoveryRequired) {
				t.Fatalf("page refusal suggested retry: %v", err)
			}
			if info.Name == "refresh_market_data" && err != nil {
				t.Fatal(err)
			}
		}
		if len(session.calls) != want {
			t.Fatal("wrong dispatch routing")
		}
	}
}
