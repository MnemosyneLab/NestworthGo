// Package agentguidance owns compact business recipes shared by the portable
// skill and built-in tool descriptions. It does not decide permission, recovery,
// tool groups or dispatch, and never loads an installed skill at runtime.
package agentguidance

import (
	_ "embed"
	"encoding/json"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

//go:embed recipes.json
var source []byte

type recipe struct {
	ID          string          `json:"id"`
	Tool        string          `json:"tool"`
	Reference   string          `json:"reference"`
	Text        string          `json:"text"`
	ExampleName string          `json:"exampleName,omitempty"`
	Example     json.RawMessage `json:"example,omitempty"`
}

var recipes = func() []recipe {
	var result []recipe
	if err := jsoncodec.Unmarshal(source, &result); err != nil {
		panic(err)
	}
	return result
}()

// Description adds a recipe only to its entry tool. Existing turn-local schema
// loading supplies it with that tool's group, without extra calls, changing the
// stable core, persisting a loaded group or treating result data as instructions.
func Description(name, description string) string {
	if guidance := routeGuidance[name]; guidance != "" {
		description += "\nTool selection: " + guidance
	}

	for _, r := range recipes {
		if r.Tool == name {
			description += "\nBusiness recipe: " + r.Text
			if len(r.Example) != 0 {
				var value any
				if err := jsoncodec.Unmarshal(r.Example, &value); err != nil {
					panic(err)
				}
				compact, err := jsoncodec.Marshal(value)
				if err != nil {
					panic(err)
				}
				description += "\nSynthetic format example (replace placeholders and illustrative values): " + string(compact)
			}
			return description
		}
	}
	return description
}

// Select one authority for a question; overlapping views are not additive.
var routeGuidance = map[string]string{
	"get_financial_context":     "Default net-worth summary. Account detail: get_account_valuations; breakdowns: get_overview.",
	"get_overview":              "For overview breakdowns; ordinary net-worth questions use get_financial_context.",
	"compare_financial_context": "Default two-state comparison; daily series use get_historical_overview.",
	"get_read_item":             "Financial-context references require get_financial_context_item; never mix capture IDs.",
	"update_instrument":         "For multiple name/shortName/note edits use update_instruments_batch with expected versions.",
	"preview_batch":             "Use for one or many ledger records, then commit_plan. Authorized intent needs no extra user confirmation.",
	"operate_page":              "Prefer navigate_page and structured business tools. Never bypass permission or retry uncertain writes via UI.",
}
