package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type failingFXRepriceRepository struct {
	*sqlite.Repository
	stored bool
	err    error
}

func (r *failingFXRepriceRepository) AppendFXQuote(ctx context.Context, quote domain.FXQuote) error {
	err := r.Repository.AppendFXQuote(ctx, quote)
	r.stored = err == nil
	return err
}

func (r *failingFXRepriceRepository) ListInstruments(ctx context.Context, id domain.HouseholdID, includeArchived bool) ([]domain.Instrument, error) {
	if r.stored {
		return nil, r.err
	}
	return r.Repository.ListInstruments(ctx, id, includeArchived)
}

func TestManualFXReceiptIdentifiesCommittedQuoteAfterRepriceFailure(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "manual-fx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := &failingFXRepriceRepository{Repository: sqlite.NewRepository(db), err: errors.New("synthetic metal repricing failure")}
	s := NewService(repo)
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	if err := s.completeBaselineFixture(ctx, OnboardingInput{HouseholdName: "Synthetic FX receipt", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetFXPreference(ctx, "USD", "JPY", "manual"); err != nil {
		t.Fatal(err)
	}
	const rate = "140.123456789012"
	stored, err := s.AppendManualFXQuoteWithReceipt(ctx, "USD", "JPY", rate, "2026-11-24")
	if !errors.Is(err, repo.err) || stored.ID == "" || stored.Rate.Canonical() != rate || stored.SourceKind != domain.QuoteSourceManual {
		t.Fatalf("committed receipt = %+v, %v", stored, err)
	}
	got, err := s.CurrentFXQuote(ctx, "USD", "JPY")
	if err != nil || got == nil || got.ID != stored.ID {
		t.Fatalf("stored current quote = %+v, %v", got, err)
	}
	legacy, err := s.AppendManualFXQuote(ctx, "USD", "JPY", "141", "2026-11-25")
	if !errors.Is(err, repo.err) || legacy.ID != "" {
		t.Fatalf("legacy Wails service result = %+v, %v", legacy, err)
	}
}
