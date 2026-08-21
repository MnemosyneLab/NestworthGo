package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

func TestRegionIsLazyAndInvalidatesExplicitly(t *testing.T) {
	builds := 0
	region := newRegion(func() fyne.CanvasObject {
		builds++
		return container.NewVBox()
	})

	if builds != 0 {
		t.Fatalf("region built before Object(), builds = %d", builds)
	}
	first := region.Object()
	if builds != 1 {
		t.Fatalf("builds after first Object() = %d, want 1", builds)
	}
	if second := region.Object(); second != first {
		t.Fatal("Object() returned a different object without invalidation")
	}
	if builds != 1 {
		t.Fatalf("builds after cached Object() = %d, want 1", builds)
	}

	region.Invalidate()
	third := region.Object()
	if third == first {
		t.Fatal("Object() returned the old object after invalidation")
	}
	if builds != 2 {
		t.Fatalf("builds after invalidation = %d, want 2", builds)
	}
}
