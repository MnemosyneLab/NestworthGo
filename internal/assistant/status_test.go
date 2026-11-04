package assistant

import (
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestInstalledGateIsClosedAndContainsNoCredentials(t *testing.T) {
	for _, surface := range []string{"desktop", "mobile"} {
		status := Closed(surface)
		if status.Version != 1 || !status.PageGuide || status.ModelRuns || status.FinancialWrites || status.Provider != "unavailable" || status.Credentials != "unavailable" || status.Conversations != "memory" {
			t.Fatalf("unsafe assistant gate: %+v", status)
		}
		raw, err := jsoncodec.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := jsoncodec.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 9 {
			t.Fatalf("review assistant status contract change: %s", raw)
		}
	}
	// There is deliberately no runtime runner or credential configuration API.
	if reflect.TypeFor[Status]().NumField() != 9 {
		t.Fatal("review assistant status fields")
	}
}
