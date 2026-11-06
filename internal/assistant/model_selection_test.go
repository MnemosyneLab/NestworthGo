package assistant_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	ai "github.com/waltwang/nestworth-go/internal/assistant"
)

func TestModelCatalogExactCapabilities(t *testing.T) {
	catalog := ai.ModelCatalog()
	if len(catalog) != 8 {
		t.Fatal(catalog)
	}
	for _, m := range catalog {
		if !ai.ValidReasoning(m.ID, "max") || ai.ValidReasoning(m.ID, "ultra") {
			t.Fatal(m)
		}
	}
	for _, id := range []string{"gpt-6-astra", "gpt-6.1-sol"} {
		if ai.ValidReasoning(id, "none") {
			t.Fatal(id)
		}
	}
	for _, id := range []string{"gpt-6-luna", "gpt-5.6-sol", "gpt-6.1-terra"} {
		if !ai.ValidReasoning(id, "none") {
			t.Fatal(id)
		}
	}
	for _, id := range []string{"gpt-4.1", "custom", "gpt-6.1-sol-unverified"} {
		if ai.ValidReasoning(id, "high") || !ai.ValidReasoning(id, "") {
			t.Fatal(id)
		}
	}
	catalog[0].Efforts[0] = "corrupt"
	if ai.ModelCatalog()[0].Efforts[0] != "none" {
		t.Fatal("shared catalogue memory")
	}
}

func TestSelectionPersistenceConcurrencyAndPortableHistory(t *testing.T) {
	dir := privateTestDirectory(t)
	creds := &credentials{available: true}
	type observed struct {
		model, effort string
		history       string
		turns         []ai.HistoryTurn
	}
	got := make(chan observed, 8)
	release := make(chan struct{})
	f := func(_ context.Context, cfg ai.ModelConfig, _ []byte, _ string) (ai.Engine, error) {
		return &engine{func(ctx context.Context, h []byte, p string) (ai.Completion, error) {
			got <- observed{cfg.Model, cfg.ReasoningEffort, string(h), ai.PortableHistory(ctx)}
			if p == "blocked" {
				select {
				case <-release:
				case <-ctx.Done():
					return ai.Completion{}, ctx.Err()
				}
			}
			return ai.Completion{History: []byte("native-" + cfg.Model), Text: "answer-" + p}, nil
		}}, nil
	}
	r, _ := open(t, dir, creds, f, time.Second)
	enable(t, r)
	s := r.Snapshot()
	cfg := s.Config
	cfg.Model = "gpt-6-sol"
	cfg.EnabledModels = []string{"gpt-6-sol", "gpt-6.1-sol"}
	if err := r.Configure(t.Context(), s.Epoch, s.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	s = r.Snapshot()
	if err := r.SelectModel(t.Context(), s.Epoch, s.Revision, ai.ModelSelection{Model: "gpt-6-sol", ReasoningEffort: "max"}); err != nil {
		t.Fatal(err)
	}
	q := request(t, r)
	if _, err := r.Start(t.Context(), q); err != ai.ErrUnavailable {
		t.Fatal("multi-model request silently chose default", err)
	}
	q.Model = "gpt-6-sol"
	q.ReasoningEffort = "max"
	q.Prompt = "blocked"
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	first := <-got
	if first.model != q.Model || first.effort != "max" {
		t.Fatal(first)
	}
	effortChange := q
	effortChange.ReasoningEffort = "high"
	if _, err := r.Start(t.Context(), effortChange); err != ai.ErrConflict {
		t.Fatal("retry effort changed", err)
	}
	changed := q
	changed.Model = "gpt-6.1-sol"
	if _, err := r.Start(t.Context(), changed); !errors.Is(err, ai.ErrConflict) {
		t.Fatal(err)
	}
	if err := r.SelectModel(t.Context(), s.Epoch, s.Revision, ai.ModelSelection{Model: "gpt-6.1-sol", ReasoningEffort: "high"}); err != nil {
		t.Fatal(err)
	}
	// A second conversation captures a different model while the first is active.
	q2 := request(t, r)
	q2.Model = "gpt-6.1-sol"
	q2.ReasoningEffort = "high"
	if _, err := r.Start(t.Context(), q2); err != nil {
		t.Fatal(err)
	}
	second := <-got
	if second.model != q2.Model {
		t.Fatal(second)
	}
	waitRun(t, r, q2)
	close(release)
	if run := waitRun(t, r, q); run.Model != "gpt-6-sol" || run.ReasoningEffort != "max" || run.State != "succeeded" {
		t.Fatal(run)
	}
	q.RequestID = uuid.NewString()
	q.Model = "gpt-6.1-sol"
	q.ReasoningEffort = "none"
	q.Prompt = "switch"
	if _, err := r.Start(t.Context(), q); !errors.Is(err, ai.ErrUnavailable) {
		t.Fatal("invalid effort admitted", err)
	}
	q.ReasoningEffort = "high"
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	switched := <-got
	if switched.history != "" || !reflect.DeepEqual(switched.turns, []ai.HistoryTurn{{Prompt: "blocked", Text: "answer-blocked"}}) {
		t.Fatal(switched)
	}
	waitRun(t, r, q)
	q.RequestID = uuid.NewString()
	q.Prompt = "same"
	if _, err := r.Start(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	same := <-got
	if same.history != "native-gpt-6.1-sol" || len(same.turns) != 0 {
		t.Fatal(same)
	}
	waitRun(t, r, q)
	s = r.Snapshot()
	cfg = s.Config
	cfg.Model = "gpt-6-sol"
	cfg.EnabledModels = []string{"gpt-6-sol"}
	if err := r.Configure(t.Context(), s.Epoch, s.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	if r.Snapshot().Selection.Model != "gpt-6.1-sol" {
		t.Fatal("silent reselection")
	}
	q.Revision = r.Snapshot().Revision
	q.RequestID = uuid.NewString()
	if _, err := r.Start(t.Context(), q); !errors.Is(err, ai.ErrUnavailable) {
		t.Fatal("removed model admitted", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, _ = open(t, dir, creds, f, time.Second)
	defer r.Close()
	if r.Snapshot().Selection.Model != "gpt-6.1-sol" || r.Snapshot().Selection.ReasoningEffort != "high" {
		t.Fatal("selection not durable")
	}
}

func TestOfflineCatalogNeedsNoCredentialsAndConfigCopies(t *testing.T) {
	r, _ := open(t, privateTestDirectory(t), &credentials{available: true}, factory(func(context.Context, []byte, string) (ai.Completion, error) { panic("no provider calls") }), time.Second)
	defer r.Close()
	s := r.Snapshot()
	ids, err := r.Models(t.Context(), s.Epoch, s.Revision)
	if err != nil || len(ids) != 8 {
		t.Fatal(ids, err)
	}
	if _, err = r.Models(t.Context(), s.Epoch, s.Revision+1); !errors.Is(err, ai.ErrStale) {
		t.Fatal(err)
	}
	cfg := s.Config
	cfg.Model = "gpt-6-sol"
	cfg.EnabledModels = []string{"gpt-6-sol", "gpt-6-luna"}
	if err = r.Configure(t.Context(), s.Epoch, s.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.EnabledModels[0] = "mutated"
	snapshot := r.Snapshot()
	snapshot.Config.EnabledModels[0] = "mutated"
	if r.Snapshot().Config.EnabledModels[0] != "gpt-6-sol" {
		t.Fatal("config aliased")
	}
}
