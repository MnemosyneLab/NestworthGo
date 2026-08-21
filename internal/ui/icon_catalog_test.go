package ui

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestIconCatalogCoversPersistedKeys(t *testing.T) {
	available := make(map[string]bool, len(iconChoices))
	for _, choice := range iconChoices {
		available[choice.key] = true
		if iconResource(choice.key) == nil {
			t.Fatalf("icon %q has no theme resource", choice.key)
		}
	}
	for _, key := range domain.SupportedIconKeys() {
		if !available[key] {
			t.Fatalf("persisted icon %q is missing from the UI catalog", key)
		}
	}
}
