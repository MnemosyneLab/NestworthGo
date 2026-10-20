//go:build perf

package application

import (
	"encoding/json"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"sort"
	"strings"
	"testing"
)

func certifiedPerfRows(t *testing.T, f reviewFullPipelineFixture) map[string][]string {
	result := noChangeProjectionRows(t, f.db)
	// ExactSelectedFXRates is an additive certified codec field. Both arms
	// still compare every other persisted column and JSON field byte-for-byte.
	table := "investment_review_daily_inputs"
	for i, raw := range result[table] {
		var values []any
		if err := jsoncodec.Unmarshal([]byte(raw), &values); err != nil {
			t.Fatal(err)
		}
		for j, v := range values {
			text, ok := v.(string)
			if !ok || !strings.Contains(text, "ExactSelectedFXRates") {
				continue
			}
			var obj map[string]json.RawMessage
			if err := jsoncodec.Unmarshal([]byte(text), &obj); err != nil {
				t.Fatal(err)
			}
			delete(obj, "ExactSelectedFXRates")
			// Marshal both forms via maps so key ordering is identical.
			clean, err := jsoncodec.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			values[j] = string(clean)
		}
		// Canonicalize ordinary JSON too; no fields removed here.
		for j, v := range values {
			if text, ok := v.(string); ok && strings.HasPrefix(text, "{") {
				var obj map[string]json.RawMessage
				if jsoncodec.Unmarshal([]byte(text), &obj) == nil {
					clean, _ := jsoncodec.Marshal(obj)
					values[j] = string(clean)
				}
			}
		}
		clean, err := jsoncodec.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		result[table][i] = string(clean)
	}

	sort.Strings(result[table])
	// All event UUIDs were allocated independently during the fresh bootstrap;
	// normalize only that identity through its unique event key. Episode IDs,
	// payloads, event keys and every other field remain exact.
	table = "investment_review_episode_events"
	for i, raw := range result[table] {
		var values []any
		if err := jsoncodec.Unmarshal([]byte(raw), &values); err != nil {
			t.Fatal(err)
		}
		values[0] = "new-event:" + values[2].(string)
		clean, err := jsoncodec.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		result[table][i] = string(clean)
	}
	sort.Strings(result[table])
	return result
}
