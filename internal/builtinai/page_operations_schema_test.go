package builtinai

import (
	"strings"
	"testing"
)

func TestPageOperationValueSchema(t *testing.T) {
	schema := pageOperationDefinition().InputSchema.(map[string]any)
	value := schema["properties"].(map[string]any)["value"].(map[string]any)
	if value["maxLength"] != 2048 {
		t.Fatal("schema must bound Unicode code points")
	}
	description := value["description"].(string)
	for _, contract := range []string{"2048 Unicode code points", "8192 UTF-8 bytes", "no NUL", "empty allowed"} {
		if !strings.Contains(description, contract) {
			t.Fatalf("missing contract %s", contract)
		}
	}
}
