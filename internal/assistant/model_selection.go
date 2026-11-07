package assistant

import (
	"context"
	"slices"
)

// ModelCatalog is a deliberately offline, exact-ID capability registry. Never
// infer capabilities from a prefix or from an account's /models response.
// Sources and verification date: docs/architecture/ai-model-selection.md.
type ModelOption struct {
	ID      string   `json:"id"`
	Efforts []string `json:"efforts"`
}

func ModelCatalog() []ModelOption {
	all := []string{"none", "low", "medium", "high", "xhigh", "max"}
	thinking := []string{"low", "medium", "high", "xhigh", "max"}
	return []ModelOption{
		{"gpt-5.6-sol", slices.Clone(all)}, {"gpt-5.6-terra", slices.Clone(all)}, {"gpt-5.6-luna", slices.Clone(all)},
		{"gpt-6-sol", slices.Clone(all)}, {"gpt-6-luna", slices.Clone(all)}, {"gpt-6-astra", slices.Clone(thinking)},
		{"gpt-6.1-sol", slices.Clone(thinking)}, {"gpt-6.1-terra", slices.Clone(all)},
	}
}
func ReasoningEfforts(model string) []string {
	if model == "gpt-5.6" {
		model = "gpt-5.6-sol"
	} // documented legacy alias
	for _, option := range ModelCatalog() {
		if option.ID == model {
			return option.Efforts
		}
	}
	return []string{}
}
func ValidReasoning(model, effort string) bool {
	return effort == "" || slices.Contains(ReasoningEfforts(model), effort)
}
func (c ModelConfig) Models() []string {
	if len(c.EnabledModels) > 0 {
		return slices.Clone(c.EnabledModels)
	}
	if c.Model == "unconfigured" {
		return []string{}
	}
	return []string{c.Model} // version-1 single-model migration, never a catalogue entry
}
func (c ModelConfig) allows(model string) bool { return slices.Contains(c.Models(), model) }
func validModelID(model string) bool {
	return modelName.MatchString(model) && len(model) >= 1 && !(len(model) >= 3 && model[:3] == "sk-")
}
func validModelConfig(c ModelConfig) bool {
	if c.Provider != "openai-responses" || !validModelID(c.Model) || (c.Enabled && c.Model == "unconfigured") || !ValidReasoning(c.Model, c.ReasoningEffort) || len(c.EnabledModels) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range c.EnabledModels {
		if !validModelID(id) || id == "unconfigured" || seen[id] {
			return false
		}
		seen[id] = true
	}
	return len(c.EnabledModels) == 0 || seen[c.Model]
}

type ModelSelection struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
}

// Selection is a durable preference, not authority. A removed model remains
// visible and invalid until explicitly replaced. Changing it never alters runs
// already admitted, native history, or the session's permission grant.
func (r *Runtime) SelectModel(ctx context.Context, epoch string, revision uint64, selection ModelSelection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(ctx, epoch); err != nil {
		return err
	}
	if revision != r.state.Revision {
		return ErrStale
	}
	if !r.state.Config.allows(selection.Model) || !ValidReasoning(selection.Model, selection.ReasoningEffort) {
		return ErrUnavailable
	}
	r.state.Selection = selection
	return r.persist()
}
func (r *Runtime) selection() ModelSelection {
	if r.state.Selection.Model != "" {
		return r.state.Selection
	}
	return ModelSelection{Model: r.state.Config.Model}
}

type HistoryTurn struct{ Prompt, Text string }
type portableHistoryKey struct{}

func PortableHistory(ctx context.Context) []HistoryTurn {
	turns, _ := ctx.Value(portableHistoryKey{}).([]HistoryTurn)
	return slices.Clone(turns)
}

// Native state is replayed only for its exact model. A switch carries successful
// visible turns as plain text, never tool calls, encrypted reasoning or grants.
func portableTurns(c *ConversationState, model string) ([]HistoryTurn, bool) {
	previous := ""
	turns := []HistoryTurn{}
	for _, saved := range c.Runs {
		if saved.Run.State == "succeeded" {
			previous = saved.Run.Model
			turns = append(turns, HistoryTurn{saved.Run.Prompt, saved.Run.Text})
		}
	}
	return turns, previous != "" && previous != model
}

// Models returns the offline curated IDs. It never leases a credential or calls a provider.
func (r *Runtime) Models(ctx context.Context, epoch string, revision uint64) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(ctx, epoch); err != nil {
		return nil, err
	}
	if r.state.Revision != revision {
		return nil, ErrStale
	}
	ids := []string{}
	for _, model := range ModelCatalog() {
		ids = append(ids, model.ID)
	}
	return ids, nil
}
