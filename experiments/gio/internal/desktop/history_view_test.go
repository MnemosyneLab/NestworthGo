package desktop

import (
	"context"
	"errors"
	"testing"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type activityReaderFunc func(context.Context, domain.ActivityQuery) (domain.ActivityPage, error)

func (f activityReaderFunc) ListActivityPage(ctx context.Context, q domain.ActivityQuery) (domain.ActivityPage, error) {
	return f(ctx, q)
}

func TestHistoryViewScrollLanguageAndRetry(t *testing.T) {
	note := "Statement adjustment"
	items := make([]domain.Activity, 40)
	for i := range items {
		items[i] = domain.Activity{ID: domain.NewActivityID(), Kind: domain.ActivityCostAdjustment, EffectiveLocalDate: "2026-10-20"}
	}
	items[0].Note = &note
	last := domain.Activity{ID: domain.NewActivityID(), Kind: "future_kind", EffectiveLocalDate: "2026-10-19"}
	attempts := 0
	reader := activityReaderFunc(func(_ context.Context, query domain.ActivityQuery) (domain.ActivityPage, error) {
		if query.After == nil {
			return domain.ActivityPage{Activities: items, HasMore: true, Next: &domain.ActivityCursor{ID: items[len(items)-1].ID}}, nil
		}
		attempts++
		if attempts == 1 {
			return domain.ActivityPage{}, errors.New("synthetic read failure")
		}
		return domain.ActivityPage{Activities: []domain.Activity{last}}, nil
	})
	model := history.New(t.Context(), reader, func() {})
	page := newHistoryPage(model)
	h := newShellHarnessWithPages(t, ledgerPages{history: page.layout})
	h.shell.preferences.language = i18n.English
	h.frame()
	h.click("Activity history", false)
	model.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Cost correction")
	assertPageLabel(t, &h.router, "2026-10-20")
	assertPageLabel(t, &h.router, note)
	h.click("繁體中文", false)
	assertPageLabel(t, &h.router, "成本修正")

	for range 48 {
		h.router.MoveFocus(key.FocusForward)
		h.frame()
		if h.router.Source().Focused(&page.list) {
			break
		}
	}
	h.focused(&page.list)
	h.press(key.NamePageDown)
	if page.list.Position.First == 0 {
		t.Fatal("Page Down did not scroll the focused history list")
	}
	h.press(key.NameHome)
	if page.list.Position.First != 0 {
		t.Fatal("Home did not return to the first recorded entry")
	}
	h.click("載入更多", false)
	model.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "無法載入帳務紀錄。")
	assertPageLabel(t, &h.router, note)
	h.click("重試", false)
	model.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "已載入 41 筆紀錄")
	h.router.Source().Execute(key.FocusCmd{Tag: &page.list})
	h.frame()
	h.press(key.NameEnd)
	assertPageLabel(t, &h.router, "帳務變更")
	assertPageLabel(t, &h.router, "2026-10-19")
}
