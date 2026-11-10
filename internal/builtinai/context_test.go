package builtinai

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
)

func TestContextUsesEffectiveBuiltInAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-context.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := application.NewService(sqlite.NewRepository(db))
	appports.Wire(app)
	app.SetLiveDatabasePath(path)
	if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic metadata", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner"}, Timezone: "UTC", HistoryStartDate: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	pool := NewPool(app, t.TempDir(), sqlite.NewConfigurationRepository(db))
	defer pool.Close()
	shared, err := pool.Get(t.Context(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	decode := func(raw []byte, err error) map[string]any {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if jsoncodec.Unmarshal(raw, &value) != nil {
			t.Fatal("invalid context")
		}
		return value["data"].(map[string]any)
	}
	original := decode(shared.Call(t.Context(), "get_context", []byte(`{}`)))
	if original["mode"] != mcpserver.ReadWrite {
		t.Fatal("fixture must use installed desktop storage authority")
	}
	for _, full := range []bool{false, true, false} {
		session := New(shared, assistant.PageSelection{})
		if full {
			session.WithFullAccess(func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
				var q assistant.FullCapabilityRequest
				if err := jsoncodec.Unmarshal(raw, &q); err != nil {
					return nil, err
				}
				return shared.CallFull(ctx, q.Name, q.Arguments)
			})
		}
		data := decode(session.Call(t.Context(), "get_context", []byte(`{}`)))
		mode := mcpserver.ReadOnly
		if full {
			mode = mcpserver.ReadWrite
		}
		if data["mode"] != mode {
			t.Fatalf("model mode=%v; want %s", data["mode"], mode)
		}
		caps := []string{}
		for _, value := range data["capabilities"].([]any) {
			caps = append(caps, value.(string))
		}
		for _, capability := range []string{"ledger_batch", "investment_review_v2_write", "data_repair", "period_analysis", "data_repair_preview"} {
			if slices.Contains(caps, capability) != full {
				t.Fatalf("effective capability %s drifted: %v", capability, caps)
			}
		}
		if !slices.Contains(caps, "return_attribution") || !slices.Contains(caps, "activity_history") {
			t.Fatal("legitimate reads lost")
		}
		for _, key := range []string{"household", "historyOrigin", "instanceId", "directories"} {
			if !reflect.DeepEqual(data[key], original[key]) {
				t.Fatalf("non-authority metadata changed: %s", key)
			}
		}
		if !full {
			if _, err := session.Call(t.Context(), "commit_plan", []byte(`{}`)); err == nil {
				t.Fatal("metadata granted a mutation")
			}
		}
	}
	again := decode(shared.Call(t.Context(), "get_context", []byte(`{}`)))
	if !reflect.DeepEqual(again["capabilities"], original["capabilities"]) || again["mode"] != mcpserver.ReadWrite {
		t.Fatal("adapter changed shared MCP authority")
	}
	shared.RevokeForRestore(t.Context())
	if _, err := New(shared, assistant.PageSelection{}).Call(t.Context(), "get_context", []byte(`{}`)); err == nil {
		t.Fatal("revoked metadata returned")
	}
}
