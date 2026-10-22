package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func TestMarketHistoryDateNullTieAcrossPageBoundaries(t *testing.T) {
	c := newBoundedReadCache()
	c.activate(1)
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	for _, direction := range []string{"asc", "desc"} {
		rows, err := historyRows([]domain.QuoteSeriesPoint{{ID: "b", EffectiveDate: "2026-10-01", QuotedAt: now, CreatedAt: now}, {ID: "z", QuotedAt: now, CreatedAt: now}, {ID: "a", EffectiveDate: "2026-10-01", QuotedAt: now, CreatedAt: now}}, ReadSort{Field: "effectiveDate", Direction: direction}, "asc")
		if err != nil {
			t.Fatal(err)
		}
		r, err := c.store(1, readCapture{Kind: "test", Sections: map[string][]json.RawMessage{"history": rows}}, 1)
		if err != nil {
			t.Fatal(err)
		}
		all := append([]json.RawMessage{}, r.Sections["history"].Items...)
		page := r.Sections["history"].Page
		for page.HasMore {
			next, err := c.page(1, ReadPageInput{CaptureID: r.CaptureID, Section: "history", Cursor: *page.NextCursor, Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			all = append(all, next.Items...)
			page = *next.Page
		}
		for i, id := range []string{"a", "b", "z"} {
			var row marketHistoryPoint
			if jsoncodec.Unmarshal(all[i], &row) != nil || row.QuoteID != id {
				t.Fatalf("date %s null/tie order wrong", direction)
			}
		}
	}
}
func TestAgentCorrectionRetractionTargetDateFilter(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	item := map[string]any{"instrumentId": fx.instrument, "currency": "USD", "value": "1", "kind": "nav", "date": "2026-09-27", "sourceTitle": "Synthetic NAV"}
	first := mutate(t, c, "import_market_data", map[string]any{"items": []any{item}})
	original := first["quoteIds"].([]any)[0]
	item["operation"], item["targetQuoteId"], item["date"], item["value"] = "correct", original, "2026-09-28", "2"
	corrected := mutate(t, c, "import_market_data", map[string]any{"items": []any{item}})
	target := corrected["quoteIds"].([]any)[0]
	mutate(t, c, "import_market_data", map[string]any{"items": []any{map[string]any{"operation": "retract", "targetQuoteId": target, "sourceTitle": "Synthetic withdrawal"}}})
	for _, direction := range []string{"asc", "desc"} {
		args := map[string]any{"filter": map[string]any{"instrumentIds": []string{fx.instrument}, "operations": []string{"correct", "retract"}, "fromDate": "2026-09-28", "toDate": "2026-09-28"}, "sort": map[string]any{"field": "observationDate", "direction": direction}, "limit": 1}
		cap := scaleCapture(t, c, "list_agent_market_data", args)
		rows := scaleSection(t, c, cap, "records")
		if len(rows) != 2 {
			t.Fatal("date filter dropped correction/retraction target")
		}
		if rows[0].(map[string]any)["recordId"].(string) > rows[1].(map[string]any)["recordId"].(string) {
			t.Fatal("audit same-date tie not by record ID")
		}
		for _, r := range rows {
			row := r.(map[string]any)
			if row["instrumentId"] != fx.instrument || row["date"] != "2026-09-28" || row["active"] != false {
				t.Fatal("audit target identity/date/active wrong")
			}
		}
	}
	empty := scaleCapture(t, c, "list_agent_market_data", map[string]any{"filter": map[string]any{"operations": []string{"retract"}, "instrumentIds": []string{fx.instrument}, "fromDate": "2026-09-27", "toDate": "2026-09-27"}})
	if len(scaleSection(t, c, empty, "records")) != 0 {
		t.Fatal("retraction incorrectly filtered by original superseded date")
	}
	call(t, c, "list_agent_market_data", map[string]any{"filter": map[string]any{"instrumentIds": []string{uuid.NewString()}}}, false)
}

type scaleForbiddenProvider struct{ calls atomic.Int32 }

func (p *scaleForbiddenProvider) Key() string { return application.YahooFinanceProviderKey }
func (p *scaleForbiddenProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestInstrument: true, LatestFX: true}
}
func (p *scaleForbiddenProvider) LatestInstrument(context.Context, application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	p.calls.Add(1)
	return application.LatestInstrumentQuote{}, errors.New("provider must not be called by a frozen read")
}
func (p *scaleForbiddenProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	p.calls.Add(1)
	return application.LatestFXQuote{}, errors.New("provider must not be called by a frozen read")
}
func TestMarketBatchReadNeverCallsProvider(t *testing.T) {
	provider := &scaleForbiddenProvider{}
	app := wailstest.NewService(t, application.NewMarketDataRegistry(provider))
	fx := newLedgerFixtureWithApp(t, app, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	before, err := fx.app.ListActivities(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"current", "coverage", "history"} {
		cap := scaleCapture(t, c, "get_market_data_batch", map[string]any{"targets": []any{map[string]any{"instrumentId": fx.instrument}, map[string]any{"baseCurrency": "USD", "quoteCurrency": "SGD"}}, "view": view})
		if len(scaleSection(t, c, cap, "targets")) != 2 {
			t.Fatal("read lost missing targets")
		}
	}
	after, err := fx.app.ListActivities(t.Context(), 100)
	if err != nil || !reflect.DeepEqual(before, after) || provider.calls.Load() != 0 {
		t.Fatal("market read performed a provider or ledger mutation", err)
	}
}

func TestUpcomingSortNullTieAcrossPageBoundaries(t *testing.T) {
	fx := newLedgerFixture(t)
	ids := []string{}
	for range 2 {
		policy := productPolicy("2026-10-10")
		policy.NormalExitFee = productString("0")
		r := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "100", TotalCostBasis: "100", CurrentValue: "100", CashExcludesProduct: true, Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Same", StartOn: "2026-09-01", MaturityOn: productString("2026-10-10"), InterestMode: "none"}, Policy: policy}})
		ids = append(ids, r.ProductIDs[0])
	}
	detail, err := fx.app.Product(t.Context(), domain.ProductContractID(ids[0]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.SaveLiquidityReservation(t.Context(), application.SaveReservationInput{Source: domain.HoldingSourceRef(detail.Contract.AccountID, detail.Contract.HoldingID), Label: "Oversubscribed synthetic reserve", Amount: "200"}); err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	for _, field := range []string{"priority", "sortOn", "displayName"} {
		for _, direction := range []string{"asc", "desc"} {
			args := map[string]any{"sort": map[string]any{"field": field, "direction": direction}, "limit": 1}
			small := scaleCapture(t, c, "get_upcoming_actions", args)
			a := scaleSection(t, c, small, "items")
			args["limit"] = 100
			large := scaleCapture(t, c, "get_upcoming_actions", args)
			b := scaleSection(t, c, large, "items")
			if len(a) < 3 || !reflect.DeepEqual(a, b) {
				t.Fatal("upcoming page boundary changed order")
			}
			if field == "sortOn" {
				if a[len(a)-1].(map[string]any)["sortOn"] != nil {
					t.Fatal("null action sortOn not last")
				}
				for i := 1; i < len(a); i++ {
					prev, next := a[i-1].(map[string]any), a[i].(map[string]any)
					if reflect.DeepEqual(prev["sortOn"], next["sortOn"]) && prev["id"].(string) > next["id"].(string) {
						t.Fatal("upcoming equal dates not tied by ID")
					}
				}
			}
		}
	}
}
