package application

import "testing"

func TestBeginExclusiveOperationRejectsOverlap(t *testing.T) {
	service, _, _, _ := newOnboardedService(t, "exclusive", []string{"Alice"})
	if err := service.BeginExclusiveOperation(); err != nil {
		t.Fatal(err)
	}
	if err := service.BeginExclusiveOperation(); err == nil {
		t.Fatal("expected busy")
	}
	service.EndExclusiveOperation()
	if err := service.BeginExclusiveOperation(); err != nil {
		t.Fatal(err)
	}
}
