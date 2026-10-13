package mcpserver

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func seedScaleReviewTargets(t *testing.T, fx ledgerFixture) []string {
	t.Helper()
	fx.app.SetInvestmentReviewConfiguration(&memoryInvestmentReviewStore{})
	ids := []string{}
	for range 2 {
		i, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Synthetic review target", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: i.ID.String(), Quantity: "1", UnitCost: "1"}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, i.ID.String())
	}
	return ids
}
func TestScaleReviewCompetingGUIRevisionIsAtomic(t *testing.T) {
	fx := newLedgerFixture(t)
	ids := seedScaleReviewTargets(t, fx)
	start := make(chan struct{})
	type outcome struct {
		batch bool
		err   error
	}
	results := make(chan outcome, 2)
	batch := application.ReviewEnrollmentBatchInput{ExpectedRevision: 0, Items: []application.ReviewEnrollmentItem{{InstrumentID: ids[0], Enabled: true}, {InstrumentID: ids[1], Enabled: true}}}
	go func() {
		<-start
		_, err := fx.app.SetInvestmentReviewEnrollment(t.Context(), ids[0], true, 0)
		results <- outcome{err: err}
	}()
	go func() {
		<-start
		_, err := fx.app.SetInvestmentReviewEnrollmentBatch(t.Context(), batch)
		results <- outcome{batch: true, err: err}
	}()
	close(start)
	success, conflict := 0, 0
	batchWon := false
	for range 2 {
		result := <-results
		err := result.err
		if err == nil {
			if result.batch {
				batchWon = true
			}
			success++
		} else if safeError(err).Code == "conflict" {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	report, err := fx.app.InvestmentReviews(t.Context())
	if err != nil || report.Revision != 1 {
		t.Fatal(report, err)
	}
	for _, item := range report.Items {
		if item.InstrumentID == ids[1] && item.Monitored != batchWon {
			t.Fatal("batch winner did not change both targets atomically")
		}
		if item.InstrumentID == ids[0] && !item.Monitored {
			t.Fatal("winner target absent")
		}
	}
}
func TestScaleReviewBatchReceiptRecoveryAndOneNotification(t *testing.T) {
	fx := newLedgerFixture(t)
	ids := seedScaleReviewTargets(t, fx)
	var notices atomic.Int32
	fx.service.changed = func() { notices.Add(1) }
	c := ledgerSession(t, fx)
	items := []any{map[string]any{"instrumentId": ids[0], "enabled": true}, map[string]any{"instrumentId": ids[1], "enabled": true}}
	args := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"expectedRevision": 0, "items": items}}
	first := call(t, c, "set_investment_review_enrollment_batch", args, false)["data"]
	if notices.Load() != 1 {
		t.Fatal("batch emitted multiple notifications")
	}
	if !reflect.DeepEqual(first, call(t, c, "set_investment_review_enrollment_batch", args, false)["data"]) {
		t.Fatal("successful replay changed receipt")
	}
	if notices.Load() != 1 {
		t.Fatal("replay emitted notification")
	}
	before, _ := fx.app.InvestmentReviews(t.Context())
	items[1].(map[string]any)["enabled"] = false
	call(t, c, "set_investment_review_enrollment_batch", args, true)
	items[1].(map[string]any)["enabled"] = true
	for _, status := range []string{"pending", "unknown"} {
		op, err := fx.service.GetOperation(args["operationId"].(string))
		if err != nil {
			t.Fatal(err)
		}
		op.Status = status
		path, _ := fx.service.operationPath(op.ID)
		if err := fx.service.writePrivate(path, op); err != nil {
			t.Fatal(err)
		}
		call(t, c, "set_investment_review_enrollment_batch", args, true)
	}
	after, _ := fx.app.InvestmentReviews(t.Context())
	if !reflect.DeepEqual(before, after) || after.Revision != 1 || notices.Load() != 1 {
		t.Fatal("conflict/unknown changed state, revision or notification")
	}
}
func TestScaleReviewBatchInvalidRequestsLeaveStateUnchanged(t *testing.T) {
	fx := newLedgerFixture(t)
	ids := seedScaleReviewTargets(t, fx)
	c := ledgerSession(t, fx)
	before, _ := fx.app.InvestmentReviews(t.Context())
	valid := map[string]any{"instrumentId": ids[0], "enabled": true}
	oversized := make([]any, 101)
	for i := range oversized {
		oversized[i] = valid
	}
	for _, items := range [][]any{{}, {valid, valid}, {valid, map[string]any{"instrumentId": "invalid", "enabled": true}}, {valid, map[string]any{"instrumentId": uuid.NewString(), "enabled": true}}, oversized} {
		call(t, c, "set_investment_review_enrollment_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"expectedRevision": 0, "items": items}}, true)
		after, _ := fx.app.InvestmentReviews(t.Context())
		if !reflect.DeepEqual(before, after) {
			t.Fatal("invalid enrollment batch partially saved")
		}
	}
	call(t, c, "set_investment_review_snooze_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"expectedRevision": 0, "items": []any{map[string]any{"instrumentId": ids[0], "days": 7}, map[string]any{"instrumentId": ids[1], "days": 1}}}}, true)
	after, _ := fx.app.InvestmentReviews(t.Context())
	if !reflect.DeepEqual(before, after) {
		t.Fatal("invalid last snooze partially saved")
	}
}
func TestScaleMetadataCompetingVersionsHaveOneWinner(t *testing.T) {
	fx := newLedgerFixture(t)
	snapshot, err := fx.app.FilteredInstrumentMetadata(t.Context(), domain.InstrumentMetadataReadQuery{IDs: []string{fx.instrument}})
	if err != nil {
		t.Fatal(err)
	}
	version := domain.InstrumentMetadataVersion(snapshot.Items[0].Instrument, false)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, alias := range []string{"A", "B"} {
		go func(alias string) {
			<-start
			raw, _ := json.Marshal(alias)
			_, err := fx.app.UpdateInstrumentsBatch(t.Context(), application.InstrumentMetadataBatchInput{Items: []domain.InstrumentMetadataUpdate{{ID: fx.instrument, ExpectedMetadataVersion: version, Changes: domain.InstrumentMetadataChanges{ShortName: raw}}}})
			results <- err
		}(alias)
	}
	close(start)
	success, conflict := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if safeError(err).Code == "conflict" {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("CAS winners=%d conflicts=%d", success, conflict)
	}
}
func TestScaleMetadataInvalidAndManagedLastTargetsAreAtomic(t *testing.T) {
	fx := newLedgerFixture(t)
	product := guiProductPost(t, fx, liquidity.ProductCommandRequest{Kind: "record_existing", RecordExisting: &application.RecordExistingProductCommand{AccountID: fx.brokerage, Currency: "USD", Principal: "100", TotalCostBasis: "100", CurrentValue: "100", CashExcludesProduct: true, Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Managed", StartOn: "2026-09-01", MaturityOn: productString("2026-12-01"), InterestMode: "none"}, Policy: productPolicy("2026-12-01")}})
	detail, err := fx.app.Product(t.Context(), domain.ProductContractID(product.ProductIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	capture := scaleCapture(t, c, "list_instruments", map[string]any{"responseMode": "paged"})
	versions := map[string]any{}
	for _, r := range scaleSection(t, c, capture, "instruments") {
		row := r.(map[string]any)
		versions[row["id"].(string)] = row["metadataVersion"]
	}
	before, _ := fx.app.ListInstruments(t.Context(), true)
	valid := map[string]any{"id": fx.instrument, "expectedMetadataVersion": versions[fx.instrument], "changes": map[string]any{"shortName": "Must roll back"}}
	managed := map[string]any{"id": detail.Contract.InstrumentID.String(), "expectedMetadataVersion": versions[detail.Contract.InstrumentID.String()], "changes": map[string]any{"name": "Forbidden"}}
	stale := map[string]any{"id": detail.Contract.InstrumentID.String(), "expectedMetadataVersion": strings.Repeat("0", 64), "changes": map[string]any{"name": "Forbidden"}}
	oversized := make([]any, 101)
	for i := range oversized {
		oversized[i] = valid
	}
	for _, items := range [][]any{{}, {valid, valid}, {valid, map[string]any{"id": "invalid", "expectedMetadataVersion": strings.Repeat("0", 64), "changes": map[string]any{"name": "Forbidden"}}}, {valid, managed}, {valid, stale}, oversized} {
		call(t, c, "update_instruments_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": items}}, true)
		after, _ := fx.app.ListInstruments(t.Context(), true)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("invalid/managed final item changed earlier metadata")
		}
	}
}

func TestScaleReviewStaleFinalAcknowledgementDoesNotSaveEarlierItem(t *testing.T) {
	fx := newLedgerFixture(t)
	ids := seedScaleReviewTargets(t, fx)
	seedScaleNAVSeries(t, fx, ids)
	c := ledgerSession(t, fx)
	mutate(t, c, "set_investment_review_enrollment_batch", map[string]any{"expectedRevision": 0, "items": []any{map[string]any{"instrumentId": ids[0], "enabled": true}, map[string]any{"instrumentId": ids[1], "enabled": true}}})
	before, err := fx.app.InvestmentReviews(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	items := []any{}
	for _, item := range before.Items {
		if item.EpisodeID == "" {
			t.Fatal("synthetic current episode missing")
		}
		items = append(items, map[string]any{"instrumentId": item.InstrumentID, "episodeId": item.EpisodeID})
	}
	items[1].(map[string]any)["episodeId"] = "stale-episode"
	call(t, c, "acknowledge_investment_reviews_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"expectedRevision": 1, "items": items}}, true)
	after, err := fx.app.InvestmentReviews(t.Context())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("stale final acknowledgement saved earlier item", err)
	}
}

func seedScaleNAVSeries(t *testing.T, fx ledgerFixture, ids []string) {
	t.Helper()
	fx.app.SetClock(func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) })
	quotes := []application.AgentMarketDataItem{}
	for _, id := range ids {
		for n := 0; n < 43; n++ {
			quotes = append(quotes, application.AgentMarketDataItem{InstrumentID: id, Currency: "USD", Value: "1", Kind: "nav", Date: time.Date(2026, 9, 29+n, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), Timezone: "UTC", SourceTitle: "Synthetic NAV"})
		}
	}
	if _, err := fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: quotes}); err != nil {
		t.Fatal(err)
	}
}
