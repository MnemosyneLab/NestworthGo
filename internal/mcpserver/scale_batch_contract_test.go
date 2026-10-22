package mcpserver

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

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
			raw, _ := jsoncodec.Marshal(alias)
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
	capture := scaleCapture(t, c, "list_instruments", map[string]any{})
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
