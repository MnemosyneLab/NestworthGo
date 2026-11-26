package desktop

import (
	"context"
	"errors"
	"image"
	"sync/atomic"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type fxSourceTestWriter struct {
	marketFXSourceWriter
	calls   atomic.Int32
	started chan struct{}
	fail    chan struct{}
}

func (w *fxSourceTestWriter) SetFXPreference(ctx context.Context, a, b, source string) (domain.FXPreference, error) {
	if w.calls.Add(1) == 1 {
		close(w.started)
		select {
		case <-ctx.Done():
			return domain.FXPreference{}, ctx.Err()
		case <-w.fail:
			return domain.FXPreference{}, errors.New("synthetic local storage failure")
		}
	}
	return w.marketFXSourceWriter.SetFXPreference(ctx, a, b, source)
}

func TestMarketDataFXSourceRetainsDraftAndRefreshesOnlyProvider(t *testing.T) {
	provider := &refreshTestProvider{}
	h, page, service, _ := newMarketOperationsHarness(t, provider, true)
	writer := &fxSourceTestWriter{marketFXSourceWriter: service, started: make(chan struct{}), fail: make(chan struct{})}
	page.fxSourceWriter = writer
	h.size = image.Pt(700, 500)
	h.frame()
	h.click("Change FX source", false)
	draft := page.fxSourceDraft
	if draft == nil || draft.pair != (marketFXPair{"JPY", "USD"}) || draft.selected != domain.QuoteSourceProvider {
		t.Fatalf("wrong saved source or pair: %+v", draft)
	}
	h.click("Manual quote", false)
	h.press(key.NameEscape)
	h.frame()
	h.focused(draft.from)
	if writer.calls.Load() != 0 || provider.latestCalls.Load() != 0 {
		t.Fatal("cancel wrote a preference or started a refresh")
	}
	h.press(key.NameReturn)
	h.frame()
	h.click("Agent quote", false)
	save := semanticBounds(t, &h.router, "Save FX source", false)
	h.click("Save FX source", false)
	select {
	case <-writer.started:
	case <-t.Context().Done():
		t.Fatal("source save did not start")
	}
	h.clickAt(save.Min.Add(save.Max).Div(2))
	h.press(key.NameEscape)
	if page.fxSourceDraft == nil || page.fxSourceDraft.request == nil || writer.calls.Load() != 1 {
		t.Fatal("a second submit or Escape interrupted the pending write")
	}
	close(writer.fail)
	waitMarketOperations(t, h, func() bool { return page.fxSourceDraft != nil && page.fxSourceDraft.err != nil })
	if page.fxSourceDraft.selected != domain.QuoteSourceAgent {
		t.Fatal("save failure discarded the selected source")
	}
	assertPageLabel(t, &h.router, "Could not save the FX source.")
	h.click("Save FX source", false)
	waitMarketOperations(t, h, func() bool { return page.fxSourceDraft == nil && page.request == nil })
	h.focused(draft.from)
	if page.current[marketFX].fx[0].source != domain.QuoteSourceAgent || provider.latestCalls.Load() != 0 {
		t.Fatal("Agent did not persist on just the selected pair, or triggered a provider")
	}
	prefs, err := service.ListFXPreferences(t.Context())
	if err != nil || len(prefs) != 2 {
		t.Fatalf("persisted FX preferences = %+v, %v", prefs, err)
	}
	for _, pref := range prefs {
		if pref.CurrencyA == "SGD" && pref.SourceKind != domain.QuoteSourceProvider {
			t.Fatalf("unrelated pair was altered: %+v", pref)
		}
	}
	h.press(key.NameReturn)
	h.frame()
	h.click("Provider quote", false)
	h.click("Save FX source", false)
	waitMarketOperations(t, h, func() bool {
		return page.fxSourceDraft == nil && page.latest.result != nil && page.request == nil
	})
	quote, err := service.CurrentFXQuote(t.Context(), "JPY", "USD")
	if err != nil || quote == nil || quote.Rate.Canonical() != "140.4321" ||
		page.current[marketFX].fx[0].source != domain.QuoteSourceProvider || writer.calls.Load() != 3 {
		t.Fatalf("Provider source or exact refreshed quote failed to persist: %+v, %v", quote, err)
	}
}
