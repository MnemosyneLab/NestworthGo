package application

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func comparisonSelectionFixture(t *testing.T) (*Service, *sqlite.DB, domain.Instrument) {
	t.Helper()
	db, e := sqlite.Open(filepath.Join(t.TempDir(), "synthetic-selection.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	s := NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	if e = s.CompleteOnboarding(t.Context(), OnboardingInput{HouseholdName: "Synthetic selection", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); e != nil {
		t.Fatal(e)
	}
	i, e := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic selection target", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "manual"})
	if e != nil {
		t.Fatal(e)
	}
	return s, db, i
}
func seedComparisonProvider(t *testing.T, db *sqlite.DB, i domain.Instrument, kind, basis, date, at, conversion string, revision int) string {
	t.Helper()
	id := uuid.NewString()
	_, e := db.SQL.ExecContext(t.Context(), `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,delayed,observation_kind,effective_date,price_basis,conversion_json,revision) VALUES(?,?,'9',?,'provider','synthetic_provider',?,'2026-09-29T00:00:00.000Z',0,?,?,?,?,?)`, id, i.ID.String(), i.QuoteCurrency.String(), at, kind, date, basis, conversion, revision)
	if e != nil {
		t.Fatal(e)
	}
	return id
}
func comparisonChartFlag(t *testing.T, e MarketComparisonEvidence) (any, string) {
	t.Helper()
	b, err := jsoncodec.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err = jsoncodec.Unmarshal(b, &obj); err != nil {
		t.Fatal(err)
	}
	reason, _ := obj["chartSelectionReason"].(string)
	return obj["chartSelected"], reason
}
func TestMarketComparisonDailySelectionImpactUsesEachActualInstant(t *testing.T) {
	s, i, _ := newAgentMarketDataFixture(t)
	_, e := s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "USD", Kind: "nav", Date: "2026-09-28", QuotedAt: "2026-09-28T12:00:00Z", Timezone: "UTC", Value: "2", SourceTitle: "Synthetic persisted noon NAV"}}})
	if e != nil {
		t.Fatal(e)
	}
	a := MarketComparisonItem{ClientItemID: "early", InstrumentID: i.ID.String(), Currency: "USD", Kind: "nav", Date: "2026-09-28", QuotedAt: "2026-09-28T09:00:00Z", Timezone: "UTC", Value: "1"}
	b := a
	b.ClientItemID = "late"
	b.QuotedAt = "2026-09-28T15:00:00Z"
	for _, items := range [][]MarketComparisonItem{{a, b}, {b, a}} {
		r, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: items})
		if e != nil {
			t.Fatal(e)
		}
		for j, item := range items {
			single, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{item}})
			if e != nil {
				t.Fatal(e)
			}
			var got MarketComparisonMatch
			for _, m := range r.Matches {
				if m.ID == r.Results[j].MatchID {
					got = m
				}
			}
			want := "current_unchanged"
			if item.ClientItemID == "late" {
				want = "would_select_candidate"
			}
			if single.Matches[0].SelectionImpact != want || got.SelectionImpact != want {
				t.Fatalf("%s: batch=%s single=%s want=%s", item.ClientItemID, got.SelectionImpact, single.Matches[0].SelectionImpact, want)
			}
		}
		if len(r.Matches) != 2 || r.Results[0].DuplicateGroup != r.Results[1].DuplicateGroup || r.Results[0].BatchConflict {
			t.Fatal("selection cache/group identities conflated", r.Results)
		}
	}
}
func TestMarketComparisonChartUsesOriginalNAVAndConversionOrdering(t *testing.T) {
	for _, variant := range []string{"nav_daily_source", "conversion_raw_time", "conversion_revision"} {
		t.Run(variant, func(t *testing.T) {
			s, db, i := comparisonSelectionFixture(t)
			wantID := ""
			if variant == "nav_daily_source" {
				imported, e := s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "USD", Kind: "nav", Date: "2026-09-28", QuotedAt: "2026-09-28T09:00:00Z", Timezone: "UTC", Value: "1", SourceTitle: "Synthetic issuer NAV"}}})
				if e != nil {
					t.Fatal(e)
				}
				wantID = imported.QuoteIDs[0]
				seedComparisonProvider(t, db, i, "close", domain.YahooRawClosePriceBasis, "2026-09-28", "2026-09-28T15:00:00.000Z", "", 1)
			} else {
				metal, e := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic converted metal", Type: "precious_metal", QuoteCurrency: "CNY", QuoteSource: "manual", MetalTemplate: "gold", QuantityUnit: "g"})
				if e != nil {
					t.Fatal(e)
				}
				i = metal
				raw := "2026-09-28T08:00:00Z"
				if variant == "conversion_raw_time" {
					raw = "2026-09-28T09:00:00Z"
				}
				conversion := fmt.Sprintf(`{"policy":"%s","rawQuotedAt":"%s"}`, domain.MetalConversionPolicy, raw)
				wantID = seedComparisonProvider(t, db, i, "realtime", "", "2026-09-28", "2026-09-28T10:00:00.000Z", conversion, 3)
				other := fmt.Sprintf(`{"policy":"%s","rawQuotedAt":"2026-09-28T08:00:00Z"}`, domain.MetalConversionPolicy)
				seedComparisonProvider(t, db, i, "realtime", "", "2026-09-28", "2026-09-28T15:00:00.000Z", other, 2)
			}
			oracle, e := s.ReadMarketDataBatch(t.Context(), MarketReadRequest{Targets: []MarketReadTarget{{InstrumentID: i.ID.String()}}, View: "history", Range: "all"})
			if e != nil {
				t.Fatal(e)
			}
			if len(oracle.Targets[0].Series.Points) != 1 || oracle.Targets[0].Series.Points[0].ID != wantID {
				t.Fatalf("invalid independent chart fixture: %+v", oracle.Targets[0].Series.Points)
			}
			r, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{{ClientItemID: "daily", InstrumentID: i.ID.String(), Currency: i.QuoteCurrency.String(), Kind: "nav", Date: "2026-09-28", Value: "1"}}})
			if e != nil {
				t.Fatal(e)
			}
			for _, ev := range r.Evidence {
				flag, _ := comparisonChartFlag(t, ev)
				if flag != (ev.ID == wantID) {
					t.Fatalf("%s chart flag=%v want=%v; raw chart chose %s", ev.ID, flag, ev.ID == wantID, wantID)
				}
			}
		})
	}
}
func TestMarketComparisonLatestChartScopeIsUnknownUntilDayCaptured(t *testing.T) {
	s, i, _ := newAgentMarketDataFixture(t)
	ids := []string{}
	for _, hour := range []string{"09", "15"} {
		r, e := s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "USD", Kind: "latest", QuotedAt: "2026-09-28T" + hour + ":00:00Z", Value: "1", SourceTitle: "Synthetic intraday source"}}})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, r.QuoteIDs[0])
	}
	latest := MarketComparisonItem{ClientItemID: "instant", InstrumentID: i.ID.String(), Currency: "USD", Kind: "latest", QuotedAt: "2026-09-28T09:00:00Z", Value: "1"}
	r, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{latest}})
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Evidence) != 1 || r.Targets[0].CurrentQuoteID != ids[1] {
		t.Fatal("fixture failed to separate current and captured instant", r)
	}
	flag, reason := comparisonChartFlag(t, r.Evidence[0])
	if flag != nil || reason != "incomplete_day_scope" {
		t.Fatalf("partial instant asserted chart membership: flag=%v reason=%q", flag, reason)
	}
	daily := MarketComparisonItem{ClientItemID: "day", InstrumentID: i.ID.String(), Currency: "USD", Kind: "nav", Date: "2026-09-28", Value: "1"}
	r, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{latest, daily}})
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Evidence) != 2 {
		t.Fatal("daily scope did not capture the full chart day")
	}
	for _, ev := range r.Evidence {
		flag, _ := comparisonChartFlag(t, ev)
		if flag != (ev.ID == ids[1]) {
			t.Fatalf("complete day chart flag=%v for %s", flag, ev.ID)
		}
	}
}
func TestMarketComparison500DailyInstantsStayWithinSQLiteExpressionBudget(t *testing.T) {
	s, i, setClock := newAgentMarketDataFixture(t)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	setClock(now)
	items := make([]MarketComparisonItem, 500)
	for j := range items {
		date := now.AddDate(0, 0, -j-1).Format(time.DateOnly)
		items[j] = MarketComparisonItem{ClientItemID: fmt.Sprint(j), InstrumentID: i.ID.String(), Currency: "USD", Kind: "nav", Date: date, QuotedAt: date + "T12:00:00Z", Timezone: "UTC", Value: "1"}
	}
	r, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: items})
	if e != nil {
		t.Fatalf("supported 500-row modernc request failed: %v", e)
	}
	if len(r.Results) != 500 {
		t.Fatal("rows lost")
	}
	for _, row := range r.Results {
		if row.Status != "missing" {
			t.Fatal(row)
		}
	}
}

func TestMarketComparison500LatestInstantsUseBoundedBalancedSQL(t *testing.T) {
	s, i, setClock := newAgentMarketDataFixture(t)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	setClock(now)
	items := make([]MarketComparisonItem, 500)
	for j := range items {
		items[j] = MarketComparisonItem{ClientItemID: fmt.Sprint(j), InstrumentID: i.ID.String(), Currency: "USD", Kind: "latest", QuotedAt: now.Add(-time.Duration(j+1) * time.Minute).Format(time.RFC3339), Value: "1"}
	}
	r, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: items})
	if e != nil || len(r.Results) != 500 {
		t.Fatal("supported 500-instant request failed", e)
	}
}
