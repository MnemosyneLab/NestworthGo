package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type analysisQuoteProbe struct {
	queryer
	calls, maxArgs, quoteCalls int
	afterFirstQuotes           func() error
}

func (p *analysisQuoteProbe) QueryRowContext(ctx context.Context, statement string, args ...any) *sql.Row {
	p.calls++
	p.maxArgs = max(p.maxArgs, len(args))
	return p.queryer.QueryRowContext(ctx, statement, args...)
}

func (p *analysisQuoteProbe) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	p.calls++
	p.maxArgs = max(p.maxArgs, len(args))
	if strings.Contains(statement, "FROM instrument_quotes") {
		p.quoteCalls++
		// The previous chunk has been fully consumed and closed by now.
		if p.quoteCalls == 2 && p.afterFirstQuotes != nil {
			if err := p.afterFirstQuotes(); err != nil {
				return nil, err
			}
		}
	}
	return p.queryer.QueryContext(ctx, statement, args...)
}

func TestAnalysisQuoteBatchMatchesLegacyCandidates(t *testing.T) {
	t.Parallel()
	db, repo, household, _, instrument := seedPortfolioRepository(t)
	second := instrument
	second.ID, second.Name = domain.NewInstrumentID(), "No quotes"
	if err := repo.CreateInstrument(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	var withdrawn domain.InstrumentQuoteID
	for i, source := range []domain.QuoteSourceKind{domain.QuoteSourceManual, domain.QuoteSourceProvider, domain.QuoteSourceManual, domain.QuoteSourceAgent} {
		price, err := domain.ParseUnitPrice("123.45678901")
		if err != nil {
			t.Fatal(err)
		}
		quote, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: source, SourceKey: "synthetic", QuotedAt: now}, now)
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			quote.QuotedAt = now.AddDate(-4, 0, 0) // No date clipping.
		}
		if i == 3 {
			withdrawn = quote.ID
		}
		if err := repo.AppendInstrumentQuote(t.Context(), quote); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the full provider candidate projection, including exact decimal
	// adjustment metadata and provenance that must survive the shared scanner.
	if _, err := db.SQL.Exec(`UPDATE instrument_quotes SET delayed=1, observation_kind='historical_close', effective_date='2026-08-01', provider_timestamp=quoted_at, value_effective_at=quoted_at, binding_revision=7, source_policy_version='synthetic-v1', price_basis='raw_close', timestamp_basis='provider', revision=3, split_factor='1.00000001', dividend_cash='0.12345678', conversion_json='{"synthetic":true}' WHERE source_kind='provider'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO agent_quote_batches VALUES('synthetic',?,'hash',?)`, household.ID.String(), formatTimestamp(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO agent_quote_records(id,request_key,household_id,target_type,operation,target_quote_id,source_title,source_url,created_at) VALUES(?,'synthetic',?,'instrument','retract',?,'synthetic','',?)`, domain.NewActivityID().String(), household.ID.String(), withdrawn.String(), formatTimestamp(now)); err != nil {
		t.Fatal(err)
	}
	ids := []domain.InstrumentID{second.ID, instrument.ID, second.ID, instrument.ID}
	want := make([]domain.InstrumentQuote, 0)
	for _, id := range ids {
		quotes, err := repo.ListInstrumentQuotes(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, quotes...)
	}
	got, err := repo.LoadAnalysisQuoteBatch(t.Context(), household.ID, ids)
	if err != nil || !reflect.DeepEqual(got, want) || len(got) != 6 {
		t.Fatalf("candidate/order mismatch: got=%+v want=%+v err=%v", got, want, err)
	}
	for _, ids := range [][]domain.InstrumentID{nil, {}} {
		got, err := repo.LoadAnalysisQuoteBatch(t.Context(), household.ID, ids)
		if err != nil || len(got) != 0 {
			t.Fatalf("empty IDs: %v %v", got, err)
		}
	}
	for _, tc := range []struct {
		household domain.HouseholdID
		ids       []domain.InstrumentID
	}{
		{domain.NewHouseholdID(), []domain.InstrumentID{instrument.ID}},
		{household.ID, []domain.InstrumentID{instrument.ID, domain.NewInstrumentID()}},
	} {
		got, err := repo.LoadAnalysisQuoteBatch(t.Context(), tc.household, tc.ids)
		var domainErr *domain.Error
		if len(got) != 0 || !errors.As(err, &domainErr) || domainErr.Code != domain.ErrNotFound {
			t.Fatalf("invalid ownership/unknown ID accepted: %v %v", got, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := repo.LoadAnalysisQuoteBatch(ctx, household.ID, ids); !errors.Is(err, context.Canceled) || len(got) != 0 {
		t.Fatalf("canceled batch: %v %v", got, err)
	}
}

func TestAnalysisQuoteBatchBoundsQueriesAndRetainsReadSnapshot(t *testing.T) {
	t.Parallel()
	db, repo, household, _, instrument := seedPortfolioRepository(t)
	ids := []domain.InstrumentID{instrument.ID}
	seed, err := db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Rollback()
	for range 900 {
		copy := instrument
		copy.ID = domain.NewInstrumentID()
		if err := insertInstrumentTx(t.Context(), seed, copy); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, copy.ID)
	}
	last := instrument
	last.ID = ids[900]
	price, err := domain.ParseUnitPrice("1.23456789")
	if err != nil {
		t.Fatal(err)
	}
	quote, err := domain.NewInstrumentQuote(last, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceManual, QuotedAt: instrument.CreatedAt}, instrument.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	first := quote
	first.ID, first.InstrumentID = domain.NewInstrumentQuoteID(), instrument.ID
	if err := appendInstrumentQuoteTx(t.Context(), seed, first); err != nil {
		t.Fatal(err)
	}
	if err := appendInstrumentQuoteTx(t.Context(), seed, quote); err != nil {
		t.Fatal(err)
	}
	if err := seed.Commit(); err != nil {
		t.Fatal(err)
	}
	writer, err := Open(db.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	read, err := db.SQL.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer read.Rollback()
	probe := &analysisQuoteProbe{queryer: read, afterFirstQuotes: func() error {
		_, err := writer.SQL.Exec(`UPDATE instrument_quotes SET unit_price='9' WHERE id=?`, quote.ID.String())
		return err
	}}
	got, err := loadAnalysisQuoteBatch(t.Context(), probe, household.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != first.ID || got[1].ID != quote.ID || got[1].UnitPrice.String() != quote.UnitPrice.String() {
		t.Fatalf("mixed read snapshot: %+v", got)
	}
	if probe.calls != 5 || probe.quoteCalls != 2 || probe.maxArgs > 901 {
		t.Fatalf("unbounded/repeated queries: %+v", probe)
	}
	if err := read.Commit(); err != nil {
		t.Fatal(err)
	}
	current, err := repo.ListInstrumentQuotes(t.Context(), last.ID)
	if err != nil || current[0].UnitPrice.String() != "9" {
		t.Fatalf("writer not visible after snapshot: %+v %v", current, err)
	}
	t.Logf("901 distinct IDs: legacy=901 quote SELECTs; batch=%d total SELECTs, %d quote SELECTs, maxArgs=%d", probe.calls, probe.quoteCalls, probe.maxArgs)

	// A later chunk failure must not return the already loaded prefix.
	failure := errors.New("synthetic chunk failure")
	probe = &analysisQuoteProbe{queryer: db.SQL, afterFirstQuotes: func() error { return failure }}
	if got, err := loadAnalysisQuoteBatch(t.Context(), probe, household.ID, ids); !errors.Is(err, failure) || len(got) != 0 {
		t.Fatalf("partial batch escaped: %v %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	probe = &analysisQuoteProbe{queryer: db.SQL, afterFirstQuotes: func() error { cancel(); return nil }}
	if got, err := loadAnalysisQuoteBatch(ctx, probe, household.ID, ids); !errors.Is(err, context.Canceled) || len(got) != 0 {
		t.Fatalf("partial canceled batch escaped: %v %v", got, err)
	}

}
