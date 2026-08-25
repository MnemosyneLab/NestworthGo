package ui

import (
	"testing"

	"fyne.io/fyne/v2/widget"
)

func TestCollectOnboardingMemberNamesTrimsAndSkipsEmptyRows(t *testing.T) {
	entries := []*widget.Entry{widget.NewEntry(), widget.NewEntry(), widget.NewEntry()}
	entries[0].SetText("  Alice ")
	entries[1].SetText("  ")
	entries[2].SetText("Bob")

	got := collectOnboardingMemberNames(entries)
	want := []string{"Alice", "Bob"}
	if len(got) != len(want) {
		t.Fatalf("collected %d members, want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("member %d = %q, want %q", index, got[index], want[index])
		}
	}
}
