package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Real command-path cycles, rather than fabricated evaluation-state rows. Both
// the flat-position boundary and re-entry are published and independently
// checked. Intraday cycles keep closed daily votes unchanged.
func seedReviewPerfEpochCycles(t *testing.T, source reviewFullPipelineFixture, cycles int) {
	t.Helper()
	var holding domain.HoldingID
	var quantityText string
	if err := source.db.SQL.QueryRow(`SELECT id,quantity FROM holdings WHERE instrument_id=? AND archived_at IS NULL ORDER BY id LIMIT 1`, source.instruments[0].ID.String()).Scan(&holding, &quantityText); err != nil {
		t.Fatal(err)
	}
	quantity := mustQuantity(t, quantityText)
	cost, err := domain.ParseUnitPrice("10")
	if err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < cycles; cycle++ {
		for _, added := range []bool{false, true} {
			*source.now = source.now.Add(time.Second)
			if _, err := source.service.RecordChange(t.Context(), domain.PositionAdjustmentInput{HouseholdID: source.householdID, HoldingID: holding, Quantity: quantity, Added: added, UnitCost: &cost, EffectiveAt: *source.now}); err != nil {
				t.Fatal(err)
			}
			var captures [2]domain.ReviewProjectionCapture
			for path, repo := range []*sqlite.Repository{sqlite.NewRepository(source.db).WithReviewWindowBaseline(), sqlite.NewRepository(source.db).WithBoundedReviewWindow()} {
				capture, err := repo.CaptureReviewProjection(t.Context(), source.householdID, *source.now)
				if err != nil {
					t.Fatal(err)
				}
				captures[path] = capture
				publication, err := PrepareReviewPublication(t.Context(), capture, *source.now, source.service.FXProviderKey())
				if err != nil {
					t.Fatal(err)
				}
				assertReviewFullOracle(t, source, publication)
			}
			if !reflect.DeepEqual(captures[0], captures[1]) {
				t.Fatalf("epoch cycle=%d added=%t capture differs", cycle, added)
			}
			var durable [2]string
			for path := range durable {
				t.Run("epoch-publish", func(t *testing.T) {
					run := cloneReviewPerfFixture(t, source)
					repo := sqlite.NewRepository(run.db).WithBoundedReviewWindow()
					if path == 0 {
						repo = repo.WithReviewWindowBaseline()
					}
					service := NewService(repo)
					service.setClock(func() time.Time { return *run.now })
					if _, err := service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
						t.Fatal(err)
					}
					durable[path] = reviewPerfPublishedRows(t, run, source)
				})
			}
			if durable[0] != durable[1] {
				t.Fatalf("epoch cycle=%d added=%t durable lifecycle differs", cycle, added)
			}
			if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
	}
	var epochs, states int
	if err := source.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_position_epochs WHERE instrument_id=?`, source.instruments[0].ID.String()).Scan(&epochs); err != nil {
		t.Fatal(err)
	}
	if err := source.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_evaluation_state WHERE instrument_id=?`, source.instruments[0].ID.String()).Scan(&states); err != nil {
		t.Fatal(err)
	}
	if epochs != cycles+1 || states != len(capturesRules(t, source))*(cycles+1) {
		t.Fatalf("full exits/re-entry not materialized: cycles=%d epochs=%d states=%d", cycles, epochs, states)
	}
	t.Logf("real epoch history cycles=%d positionEpochs=%d evaluationStates=%d; both exit and re-entry checked against full oracle and durable baseline", cycles, epochs, states)
}

func capturesRules(t *testing.T, source reviewFullPipelineFixture) []domain.ReviewRule {
	t.Helper()
	capture, err := sqlite.NewRepository(source.db).WithFullReviewRecompute().CaptureReviewProjection(t.Context(), source.householdID, *source.now)
	if err != nil {
		t.Fatal(err)
	}
	return capture.Rules
}

func TestReviewRepeatedEpochWindowOracle(t *testing.T) {
	runReviewPerfEpochPairs(t, reviewPerfConfig{Accounts: 1, Instruments: 1, Holdings: 1, Rules: 4, HistoryDays: 10, Quotes: 10}, 1, 8)
}
