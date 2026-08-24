package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type refreshOperation string

const (
	refreshAllOperation        refreshOperation = "all"
	refreshRequiredFXOperation refreshOperation = "required_fx"
	refreshInstrumentOperation refreshOperation = "instrument"
	refreshFXOperation         refreshOperation = "fx"
)

type refreshRequest struct {
	operation         refreshOperation
	instrument        domain.InstrumentID
	currencyA         string
	currencyB         string
	persistFXProvider bool
}

func (r refreshRequest) run(ctx context.Context, service *application.Service) (application.RefreshResult, error) {
	switch r.operation {
	case refreshAllOperation:
		return service.RefreshAll(ctx)
	case refreshRequiredFXOperation:
		return service.RefreshRequiredFX(ctx)
	case refreshInstrumentOperation:
		return service.RefreshInstrument(ctx, r.instrument)
	case refreshFXOperation:
		if r.persistFXProvider {
			if _, err := service.SetFXPreference(ctx, r.currencyA, r.currencyB, string(domain.QuoteSourceProvider)); err != nil {
				return application.RefreshResult{}, err
			}
		}
		return service.RefreshFX(ctx, r.currencyA, r.currencyB)
	default:
		return application.RefreshResult{}, fmt.Errorf("unsupported refresh operation %q", r.operation)
	}
}

func (c *Controller) startRefresh(request refreshRequest) {
	c.startRefreshInternal(request, true, nil)
}

func (c *Controller) startRefreshInDialog(request refreshRequest, observer func(application.RefreshResult, error)) {
	c.startRefreshInternal(request, false, observer)
}

func (c *Controller) startRefreshInternal(request refreshRequest, rebuild bool, observer func(application.RefreshResult, error)) {
	if c.service == nil || c.refreshTask.pending {
		return
	}
	generation, ctx := c.refreshTask.begin(c.translator.T("portfolio.refreshing"))
	c.retryRefresh = nil
	c.refreshRebuild = rebuild
	c.refreshObserver = observer
	if rebuild {
		c.RefreshContent()
	}

	go func() {
		result, err := request.run(ctx, c.service)
		fyne.Do(func() {
			c.finishRefresh(generation, request, result, err)
		})
	}()
}

// finishRefresh is called only from fyne.Do. The generation check prevents a
// late completion from an abandoned request from changing the current page.
func (c *Controller) finishRefresh(generation uint64, request refreshRequest, result application.RefreshResult, err error) bool {
	if !c.refreshTask.finish(generation, result, err) {
		return false
	}
	if err != nil {
		c.retryRefresh = func() { c.startRefreshInternal(request, c.refreshRebuild, c.refreshObserver) }
	} else if refreshResultNeedsAttention(result) {
		c.retryRefresh = func() { c.startRefreshInternal(request, c.refreshRebuild, c.refreshObserver) }
	} else {
		c.retryRefresh = nil
		c.reloadBackend()
	}
	observer := c.refreshObserver
	if observer != nil {
		observer(result, err)
	}
	if c.refreshRebuild {
		c.RefreshContent()
	}
	return true
}

// cancelRefresh abandons the current refresh so a route change cannot be
// mutated by its completion.
func (c *Controller) cancelRefresh() {
	c.refreshTask.stop()
	c.retryRefresh = nil
	c.refreshObserver = nil
	c.refreshRebuild = false
}

func refreshFeedback(c *Controller) fyne.CanvasObject {
	if c.refreshTask.pending {
		label := widget.NewLabel(c.refreshTask.progress)
		label.Importance = widget.WarningImportance
		return container.NewVBox(label)
	}
	if c.refreshTask.err != nil {
		label := widget.NewLabel(c.translator.TranslateError(c.refreshTask.err))
		label.Importance = widget.DangerImportance
		if c.retryRefresh == nil {
			return label
		}
		retry := widget.NewButton(c.translator.T("common.retry"), c.retryRefresh)
		retry.Importance = widget.LowImportance
		return container.NewBorder(nil, nil, nil, retry, label)
	}
	if c.refreshTask.payload == nil {
		return container.NewWithoutLayout()
	}
	label := widget.NewLabel(refreshResultText(c))
	if refreshResultNeedsAttention(*c.refreshTask.payload) {
		label.Importance = widget.WarningImportance
	}
	if c.retryRefresh == nil {
		return label
	}
	retry := widget.NewButton(c.translator.T("common.retry"), c.retryRefresh)
	retry.Importance = widget.LowImportance
	return container.NewBorder(nil, nil, nil, retry, label)
}

func refreshResultNeedsAttention(result application.RefreshResult) bool {
	for _, item := range result.Items {
		if item.Status == application.RefreshFailed || item.Status == application.RefreshRateLimited {
			return true
		}
	}
	return false
}

func refreshSingleTargetNeedsAttention(result application.RefreshResult) bool {
	if len(result.Items) != 1 {
		return true
	}
	status := result.Items[0].Status
	return status != application.RefreshFetched && status != application.RefreshCached
}

func refreshResultText(c *Controller) string {
	if c.refreshTask.payload == nil {
		return ""
	}
	return refreshResultTextFor(c, *c.refreshTask.payload)
}

func refreshResultTextFor(c *Controller, result application.RefreshResult) string {
	var fetched, cached, skipped, failed, rateLimited int
	for _, item := range result.Items {
		switch item.Status {
		case application.RefreshFetched:
			fetched++
		case application.RefreshCached:
			cached++
		case application.RefreshSkipped:
			skipped++
		case application.RefreshFailed:
			failed++
		case application.RefreshRateLimited:
			rateLimited++
		}
	}
	return fmt.Sprintf(c.translator.T("portfolio.refreshSummary"), fetched, cached, skipped, failed, rateLimited)
}
