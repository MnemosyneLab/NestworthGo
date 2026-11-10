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
