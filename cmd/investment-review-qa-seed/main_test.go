package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestSeedRefusesExistingPaths(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "do-not-change")
	if err := os.WriteFile(marker, []byte("synthetic marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if seed(dir, "2026-10-13") == nil {
		t.Fatal("existing directory accepted")
	}
	raw, err := os.ReadFile(marker)
	if err != nil || string(raw) != "synthetic marker" {
		t.Fatal("existing data changed")
	}
	if seed("", "2026-10-13") == nil {
		t.Fatal("empty directory accepted")
	}
}
func TestSeedReviewCoverage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh")
	if err := seed(dir, "2026-10-13"); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(filepath.Join(dir, "household.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := application.NewService(sqlite.NewRepository(db))
	app.SetInvestmentReviewConfiguration(sqlite.NewConfigurationRepository(db))
	app.SetClock(func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) })
	report, err := app.InvestmentReviews(t.Context())
	if err != nil || len(report.Items) != 5 {
		t.Fatalf("seed report %+v %v", report, err)
	}
	for _, item := range report.Items {
		if item.Monitored || item.EpisodeID != "" {
			t.Fatal("seed auto-enrolled investment")
		}
		switch item.DisplayName {
		case "Synthetic flat fund":
			if item.Status != "low_yield" || item.AnnualizedPercent == nil || *item.AnnualizedPercent != "0" {
				t.Fatalf("flat seed %+v", item)
			}
		case "Synthetic loss fund":
			if item.Status != "loss" {
				t.Fatalf("loss seed %+v", item)
			}
		case "Synthetic missing fund":
			if item.Reason != "missing_baseline" {
				t.Fatalf("missing seed %+v", item)
			}
		case "Synthetic distribution fund":
			if item.Reason != "distribution_or_unit_change" {
				t.Fatalf("payout seed %+v", item)
			}
		case "Synthetic stale fund":
			if item.Status != "stale" {
				t.Fatalf("stale seed %+v", item)
			}
		}
	}
}
