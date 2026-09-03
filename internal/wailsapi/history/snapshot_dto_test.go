package history

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestDailyValuationSnapshotDTOMarksSimpleItemsNotTotals(t *testing.T) {
	currency, err := domain.ParseCurrency("CNY")
	if err != nil {
		t.Fatal(err)
	}
	amount, err := domain.ParseMoney("800", currency)
	if err != nil {
		t.Fatal(err)
	}
	netWorth, err := domain.ParseSignedMoney("800", currency)
	if err != nil {
		t.Fatal(err)
	}
	simpleID := domain.NewAccountID()
	holdingsID := domain.NewAccountID()
	holdingID := domain.NewHoldingID()
	instrumentID := domain.NewInstrumentID()
	snapshot := domain.DailyValuationSnapshot{
		ID:                domain.NewDailyValuationSnapshotID(),
		HouseholdID:       domain.NewHouseholdID(),
		LocalDate:         "2026-08-01",
		CutoffAt:          time.Date(2026, 8, 1, 23, 59, 59, 0, time.UTC),
		Currency:          currency,
		AssetsAmount:      &amount,
		LiabilitiesAmount: &amount,
		NetWorthAmount:    &netWorth,
		Items: []domain.DailyValuationSnapshotItem{
			{
				ID:                  domain.NewDailyValuationSnapshotItemID(),
				AccountID:           simpleID,
				NativeAmount:        "800",
				NativeCurrency:      currency,
				BaseAmount:          &amount,
				Complete:            true,
				ClassificationBasis: domain.ClassificationCurrentMetadataDerived,
			},
			{
				ID:             domain.NewDailyValuationSnapshotItemID(),
				AccountID:      holdingsID,
				HoldingID:      &holdingID,
				InstrumentID:   &instrumentID,
				NativeAmount:   "100",
				NativeCurrency: currency,
				BaseAmount:     &amount,
				Complete:       true,
			},
		},
	}
	dto := fromDailyValuationSnapshot(snapshot)
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["classificationBasis"]; ok {
		t.Fatalf("snapshot DTO JSON still has classificationBasis: %s", raw)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v", payload["items"])
	}
	simple, _ := items[0].(map[string]any)
	composite, _ := items[1].(map[string]any)
	if simple["classificationBasis"] != domain.ClassificationCurrentMetadataDerived.String() {
		t.Fatalf("simple item = %#v", simple)
	}
	if _, ok := composite["classificationBasis"]; ok {
		t.Fatalf("composite item still has classificationBasis: %#v", composite)
	}
}
