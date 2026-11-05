package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestApplyInstrumentMetadataNullClearsAndEmptyRejects(t *testing.T) {
	note := "keep"
	current := Instrument{Name: "Fund", ShortName: "Alias", Note: &note}
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cleared, changed, err := ApplyInstrumentMetadata(current, InstrumentMetadataChanges{ShortName: json.RawMessage("null")}, now)
	if err != nil || !changed || cleared.ShortName != "" || cleared.Note == nil || *cleared.Note != "keep" || cleared.Name != "Fund" {
		t.Fatalf("null shortName = %+v changed=%v err=%v", cleared, changed, err)
	}
	if _, _, err = ApplyInstrumentMetadata(cleared, InstrumentMetadataChanges{ShortName: json.RawMessage(`""`)}, now); err == nil {
		t.Fatal("empty shortName cleared a field")
	}
	if _, _, err = ApplyInstrumentMetadata(cleared, InstrumentMetadataChanges{Note: json.RawMessage(`"  "`)}, now); err == nil {
		t.Fatal("blank note cleared a field")
	}
	noted, changed, err := ApplyInstrumentMetadata(cleared, InstrumentMetadataChanges{Note: json.RawMessage("null")}, now)
	if err != nil || !changed || noted.Note != nil || noted.ShortName != "" {
		t.Fatalf("null note = %+v changed=%v err=%v", noted, changed, err)
	}
	if _, _, err = ApplyInstrumentMetadata(noted, InstrumentMetadataChanges{Name: json.RawMessage("null")}, now); err == nil {
		t.Fatal("null name was accepted")
	}
}
