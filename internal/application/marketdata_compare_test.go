package application

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestMarketComparisonSharedValidationDatesAndDecimalEdges(t *testing.T) {
	s, instrument, setClock := newAgentMarketDataFixture(t)
	setClock(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	cases := []struct {
		name, kind, value, date, at, zone string
		valid                             bool
	}{
		{"zero", "nav", "0", "2026-09-28", "", "UTC", true},
		{"max", "close", "999999999999.99999999", "2026-09-28", "", "UTC", true},
		{"tiny", "nav", "0.00000001", "2026-09-28", "", "UTC", true},
		{"extra precision", "nav", "1.000000001", "2026-09-28", "", "UTC", false},
		{"float artifact", "nav", "0.30000000000000004", "2026-09-28", "", "UTC", false},
		{"negative", "nav", "-1", "2026-09-28", "", "UTC", false},
		{"exponent", "nav", "1e2", "2026-09-28", "", "UTC", false},
		{"infinity", "nav", "Inf", "2026-09-28", "", "UTC", false},
		{"date invalid", "nav", "1", "2026-02-30", "", "UTC", false},
		{"future", "nav", "1", "2026-11-05", "", "UTC", false},
		{"latest needs time", "latest", "1", "", "", "UTC", false},
		{"latest", "latest", "1", "", "2026-09-28T23:00:00-04:00", "America/New_York", true},
		{"latest rejects daily date", "latest", "1", "2026-09-28", "2026-09-28T00:00:00Z", "UTC", false},
		{"market day differs UTC", "close", "1", "2026-09-28", "2026-09-29T03:00:00Z", "America/New_York", true},
		{"date mismatch", "close", "1", "2026-09-28", "2026-09-29T05:00:00Z", "America/New_York", false},
		{"spring DST", "close", "1", "2026-03-08", "2026-03-09T03:30:00Z", "America/New_York", true},
		{"fall DST first fold", "close", "1", "2026-11-01", "2026-11-01T01:30:00-04:00", "America/New_York", true},
		{"fall DST second fold", "close", "1", "2026-11-01", "2026-11-01T01:30:00-05:00", "America/New_York", true},
		{"invalid timezone", "close", "1", "2026-09-28", "", "Not/AZone", false},
		{"no origin UTC fallback", "nav", "1", "2026-09-28", "2026-09-28T23:00:00Z", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := MarketComparisonItem{ClientItemID: tc.name, InstrumentID: instrument.ID.String(), Currency: "USD", Kind: tc.kind, Value: tc.value, Date: tc.date, QuotedAt: tc.at, Timezone: tc.zone}
			r, err := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{item}})
			if err != nil {
				t.Fatal(err)
			}
			if valid := r.Results[0].Status != "invalid"; valid != tc.valid {
				t.Fatalf("comparison=%+v want valid=%v", r.Results[0], tc.valid)
			}
			// Run the authoritative write only for invalid synthetic candidates; they
			// must fail before any fact is persisted, proving shared validation parity.
			if !tc.valid {
				a := comparisonItemAgent(item)
				a.SourceTitle = "Synthetic rejected candidate"
				if _, err = s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{a}}); err == nil {
					t.Fatal("writer accepted comparison-invalid observation")
				}
			}
		})
	}
	for _, value := range []string{"0", "0.0000000000001", "100000000", "NaN"} {
		r, err := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{{ClientItemID: "fx", BaseCurrency: "USD", QuoteCurrency: "SGD", Kind: "daily_reference", Value: value, Date: "2026-09-28"}}})
		if err != nil || r.Results[0].Status != "invalid" {
			t.Fatalf("invalid FX %s: %+v %v", value, r, err)
		}
	}
}
func TestMarketComparisonHashDuplicatesAndBounds(t *testing.T) {
	s, i, _ := newAgentMarketDataFixture(t)
	a := MarketComparisonItem{ClientItemID: "z", InstrumentID: i.ID.String(), Currency: "USD", Kind: "nav", Value: "1", Date: "2026-09-28"}
	b := a
	b.ClientItemID = "a"
	one, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{a, b}})
	if e != nil {
		t.Fatal(e)
	}
	two, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{b, a}})
	if e != nil {
		t.Fatal(e)
	}
	if one.InputHash == two.InputHash || len(one.Matches) != 1 || one.Results[0].BatchConflict || one.Results[0].DuplicateGroup == "" {
		t.Fatal("ordered input was sorted/deduplicated")
	}
	b.Value = "2"
	r, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{a, b}})
	if e != nil || !r.Results[0].BatchConflict || r.Results[0].Status != "missing" {
		t.Fatal("batch contradictions hidden by persisted absence", e, r)
	}
	inputs := make([]MarketComparisonItem, 500)
	for j := range inputs {
		inputs[j] = a
		inputs[j].ClientItemID = fmt.Sprint(j)
	}
	r, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: inputs})
	if e != nil || len(r.Results) != 500 || len(r.Matches) != 1 {
		t.Fatalf("500 rows lost: %d %v", len(r.Results), e)
	}
	inputs = append(inputs, a)
	if _, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: inputs}); e == nil {
		t.Fatal("501 items accepted")
	}
	for j := range 101 {
		inputs[j] = a
		inputs[j].ClientItemID = fmt.Sprint(j)
		inputs[j].InstrumentID = uuid.NewString()
	}
	if _, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: inputs[:101]}); e == nil {
		t.Fatal("101 targets accepted")
	}
	a.ClientItemID = strings.Repeat("😀", 33)
	if _, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{a}}); e == nil {
		t.Fatal("client id byte cap ignored")
	}
}

func TestMarketComparisonProviderManualSourcesAndIncompleteBasis(t *testing.T) {
	db, e := sqlite.Open(filepath.Join(t.TempDir(), "comparison-sources.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	repo := sqlite.NewRepository(db)
	s := NewService(repo)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	if e = s.CompleteOnboarding(t.Context(), OnboardingInput{HouseholdName: "Synthetic comparison", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); e != nil {
		t.Fatal(e)
	}
	i, e := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic source comparison", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ImportAgentMarketData(t.Context(), uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{InstrumentID: i.ID.String(), Kind: "close", Currency: "USD", Value: "5", Date: "2026-09-28", SourceTitle: "Agent close source"}, {InstrumentID: i.ID.String(), Kind: "nav", Currency: "USD", Value: "1", Date: "2026-09-28", SourceTitle: "NAV issuer"}}}); e != nil {
		t.Fatal(e)
	}
	providerID := uuid.NewString()
	insert := func(id, source, key, kind, basis, date, value, conversion string) {
		t.Helper()
		_, e := db.SQL.ExecContext(t.Context(), `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,delayed,observation_kind,effective_date,price_basis,conversion_json) VALUES(?,?,?,'USD',?,?,?,?,0,?,?,?,?)`, id, i.ID.String(), value, source, key, "2026-09-28T12:00:00Z", "2026-09-29T00:00:00Z", kind, date, basis, conversion)
		if e != nil {
			t.Fatal(e)
		}
	}
	insert(providerID, "provider", "synthetic_provider", "close", domain.YahooRawClosePriceBasis, "2026-09-28", "5", "")
	insert(uuid.NewString(), "manual", "manual", "manual", "", "", "5", "")
	item := MarketComparisonItem{ClientItemID: "raw-close", InstrumentID: i.ID.String(), Kind: "close", Currency: "USD", Value: "5", Date: "2026-09-28"}
	// A legacy latest manual price on the same local day lacks a daily date. It
	// is exposed as related incomplete evidence, not silently called raw close.
	r, e := s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{item}})
	if e != nil || r.Results[0].Status != "unavailable" {
		t.Fatal("undated legacy evidence was called missing", e, r)
	}
	if _, e = db.SQL.ExecContext(t.Context(), `DELETE FROM instrument_quotes WHERE source_kind='manual'`); e != nil {
		t.Fatal(e)
	}
	r, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{item}})
	if e != nil || r.Results[0].Status != "same" || r.Matches[0].ActiveEqual != 2 {
		t.Fatal("same values from Agent/provider not comparable", e, r)
	}
	sources := map[domain.QuoteSourceKind]bool{}
	for _, ev := range r.Evidence {
		sources[ev.SourceKind] = true
	}
	if !sources[domain.QuoteSourceProvider] || !sources[domain.QuoteSourceAgent] {
		t.Fatal("source provenance dropped")
	}
	if _, e = db.SQL.ExecContext(t.Context(), `UPDATE instrument_quotes SET unit_price='5.00000001' WHERE id=?`, providerID); e != nil {
		t.Fatal(e)
	}
	r, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{item}})
	if e != nil || r.Results[0].Status != "conflict" || r.Matches[0].ActiveEqual != 1 || r.Matches[0].ActiveDifferent != 1 {
		t.Fatal("equal evidence hid competing conflict", e, r)
	}
	insert(uuid.NewString(), "manual", "manual", "manual", "", "", "5", "")
	latest := item
	latest.ClientItemID = "manual-latest"
	latest.Kind = "latest"
	latest.Date = ""
	latest.QuotedAt = "2026-09-28T12:00:00Z"
	if _, e = db.SQL.ExecContext(t.Context(), `PRAGMA query_only=ON`); e != nil {
		t.Fatal(e)
	}
	r, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{latest}})
	if e != nil || r.Results[0].Status != "same" {
		t.Fatal("manual latest exact value unavailable or comparison wrote data", e, r)
	}
	if _, e = db.SQL.ExecContext(t.Context(), `PRAGMA query_only=OFF`); e != nil {
		t.Fatal(e)
	}
	latest.QuotedAt = "2026-09-28T12:00:00.000000001Z"
	r, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{latest}})
	if e != nil || r.Results[0].Status != "unavailable" || r.Results[0].Reason != "submillisecond_timestamp_not_preserved_by_storage" {
		t.Fatal("timestamp precision falsely rounded or called missing", e, r)
	}
	insert(uuid.NewString(), "provider", "synthetic_converted", "close", domain.YahooRawClosePriceBasis, "2026-09-27", "9", `{"rawCurrency":"USD","displayOnly":true}`)
	item.Date = "2026-09-27"
	r, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{item}})
	if e != nil || r.Results[0].Status != "unavailable" {
		t.Fatal("converted basis falsely compared", e, r)
	}
}

func TestMarketComparisonFinalCaptureBudgetEscapedEvidence(t *testing.T) {
	db, e := sqlite.Open(filepath.Join(t.TempDir(), "comparison-escaped-budget.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	repo := sqlite.NewRepository(db)
	s := NewService(repo)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	if e = s.CompleteOnboarding(t.Context(), OnboardingInput{HouseholdName: "Synthetic byte budget", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); e != nil {
		t.Fatal(e)
	}
	i, e := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic budget", Type: "stock", QuoteCurrency: "USD", QuoteSource: "agent"})
	if e != nil {
		t.Fatal(e)
	}
	// SQL input is about 1 MiB; its JSON escaped expansion exceeds 4 MiB. The
	// final serialized budget must fail before an apparently complete capture.
	_, e = db.SQL.ExecContext(t.Context(), `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,delayed,observation_kind,effective_date,price_basis) VALUES(?,?,'1','USD','agent',?,'2026-09-28T00:00:00Z','2026-09-28T00:00:00Z',0,'close','2026-09-28','agent_unit_nav_v1')`, uuid.NewString(), i.ID.String(), strings.Repeat("\x01", 1<<20))
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.CompareMarketData(t.Context(), MarketComparisonRequest{Items: []MarketComparisonItem{{ClientItemID: "one", InstrumentID: i.ID.String(), Kind: "nav", Currency: "USD", Value: "1", Date: "2026-09-28"}}})
	var de *domain.Error
	if !errors.As(e, &de) || de.Code != "too_large" {
		t.Fatal("serialized capture expansion did not fail closed", e)
	}
}
