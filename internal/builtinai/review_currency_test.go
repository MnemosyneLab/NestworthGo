package builtinai

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/mcpserver"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestReviewProjectionNormalizesLegacyCurrencyAndPreservesUnknown(t *testing.T) {
	id := "synthetic-account"
	for _, currency := range []string{"usd", " USD ", "\tusd\n"} {
		review := mcpserver.HostChangeReview{State: mcpserver.ReviewUnknown, Binding: mcpserver.HostChangeReviewBinding{Preview: mcpserver.ChangePlanPreview{Command: history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: id, Amount: "12.34", Currency: currency}, Preview: wire.ChangePreviewDTO{Resulting: []wire.EndpointViewDTO{{AccountID: &id, Name: "Synthetic cash", Amount: "12.34", Currency: "USD"}}}}}}
		got, err := reviewView(review)
		if err != nil || got.Currency != "USD" || got.ResultingAmount != "12.34" || got.State != mcpserver.ReviewUnknown {
			t.Fatalf("legacy projection: %+v %v", got, err)
		}
		review.State = mcpserver.ReviewAwaitingConfirmation
		got, err = reviewView(review)
		if err != nil || got.State != mcpserver.ReviewNeedsPreview {
			t.Fatal("legacy identity enabled confirmation")
		}
	}
}
