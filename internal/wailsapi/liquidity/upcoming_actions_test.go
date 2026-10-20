package liquidity

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"testing"
)

func TestUpcomingWireArraysNullAndMoney(t *testing.T) {
	zero, _ := domain.ParseMoney("0", "USD")
	v := domain.UpcomingActions{Items: []domain.UpcomingAction{{ID: "test", Amount: &zero, PrimaryTarget: domain.UpcomingTarget{Kind: "data_health"}}}}
	dto := fromUpcomingActions(v)
	b, e := jsoncodec.Marshal(dto)
	if e != nil {
		t.Fatal(e)
	}
	var wire map[string]any
	if e = jsoncodec.Unmarshal(b, &wire); e != nil {
		t.Fatal(e)
	}
	if wire["schemaVersion"] != "upcoming-actions/1" || wire["dataChecks"] == nil || wire["scopeWarnings"] == nil {
		t.Fatal(string(b))
	}
	item := wire["items"].([]any)[0].(map[string]any)
	for _, k := range []string{"reservationIds", "reservationLabels", "reasonCodes", "assumptions", "targets"} {
		if item[k] == nil {
			t.Fatalf("null %s", k)
		}
	}
	for _, k := range []string{"anchorOn", "maturityOn", "eligibilityOn", "estimatedReceiptOn", "referenceOn", "productId", "productKind", "sourceRef", "requestedReserve"} {
		if item[k] != nil {
			t.Fatalf("unknown %s was not null", k)
		}
	}
	if item["amount"].(map[string]any)["amount"] != "0" {
		t.Fatal("zero lost")
	}
}

type upcomingFailRepo struct{ application.Repository }

func (upcomingFailRepo) ReadLiquiditySnapshot(context.Context) (domain.LiquiditySnapshot, error) {
	return domain.LiquiditySnapshot{}, errors.New("synthetic read failure")
}
func TestUpcomingWirePropagatesReadFailure(t *testing.T) {
	s := NewService(application.NewService(upcomingFailRepo{}))
	_, e := s.UpcomingActions(context.Background())
	if e == nil {
		t.Fatal("failure became empty list")
	}
	var wire map[string]any
	if jsoncodec.Unmarshal([]byte(e.Error()), &wire) != nil || wire["code"] == nil {
		t.Fatalf("unwrapped error: %v", e)
	}
}
