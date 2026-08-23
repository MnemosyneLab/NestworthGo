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
	operation  refreshOperation
	instrument domain.InstrumentID
	currencyA  string
	currencyB  string
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
	if c.service == nil || c.refreshPending {
		return
	}
	if c.refreshCancel != nil {
		c.refreshCancel()
	}
	c.refreshGeneration++
	generation := c.refreshGeneration
	ctx, cancel := context.WithCancel(context.Background())
	c.refreshCancel = cancel
	c.refreshPending = true
	c.refreshProgress = c.translator.T("portfolio.refreshing")
	c.refreshResult = nil
	c.refreshError = nil
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
	if generation != c.refreshGeneration {
		return false
	}
	if c.refreshCancel != nil {
		c.refreshCancel()
	}
	c.refreshCancel = nil
	c.refreshPending = false
	c.refreshProgress = ""
	c.refreshError = err
	if err != nil {
		c.refreshResult = nil
		c.retryRefresh = func() { c.startRefreshInternal(request, c.refreshRebuild, c.refreshObserver) }
	} else {
		c.refreshResult = &result
		if refreshResultNeedsAttention(result) {
			c.retryRefresh = func() { c.startRefreshInternal(request, c.refreshRebuild, c.refreshObserver) }
		} else {
			c.retryRefresh = nil
		}
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

// cancelRefresh invalidates the current generation before cancelling its
// context. The order makes cancellation safe even if the worker completes at
// the same time as a route change.
func (c *Controller) cancelRefresh() {
	c.refreshGeneration++
	if c.refreshCancel != nil {
		c.refreshCancel()
	}
	c.refreshCancel = nil
	c.refreshPending = false
	c.refreshProgress = ""
	c.retryRefresh = nil
	c.refreshObserver = nil
	c.refreshRebuild = false
}

func refreshFeedback(c *Controller) fyne.CanvasObject {
	if c.refreshPending {
		label := widget.NewLabel(c.refreshProgress)
		label.Importance = widget.WarningImportance
		return container.NewVBox(label)
	}
	if c.refreshError != nil {
		label := widget.NewLabel(c.translator.TranslateError(c.refreshError))
		label.Importance = widget.DangerImportance
		if c.retryRefresh == nil {
			return label
		}
		retry := widget.NewButton(c.translator.T("common.retry"), c.retryRefresh)
		retry.Importance = widget.LowImportance
		return container.NewBorder(nil, nil, nil, retry, label)
	}
	if c.refreshResult == nil {
		return container.NewWithoutLayout()
	}
	label := widget.NewLabel(refreshResultText(c))
	if refreshResultNeedsAttention(*c.refreshResult) {
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

func refreshResultText(c *Controller) string {
	if c.refreshResult == nil {
		return ""
	}
	return refreshResultTextFor(c, *c.refreshResult)
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
