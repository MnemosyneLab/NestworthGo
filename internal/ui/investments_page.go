package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

// NewInvestmentsPage is a local read view. Its construction never invokes a
// provider; refresh only begins from one of the explicit action buttons.
func NewInvestmentsPage(c *Controller) fyne.CanvasObject {
	t := c.translator
	portfolio, err := c.service.Portfolio(context.Background(), domain.AccountFilter{})
	if err != nil {
		return errorPanel(c, t.T("portfolio.loadError"), err)
	}

	refreshAll := widget.NewButton(t.T("portfolio.refreshAll"), func() {
		c.startRefresh(refreshRequest{operation: refreshAllOperation})
	})
	refreshFX := widget.NewButton(t.T("portfolio.refreshFX"), func() {
		c.startRefresh(refreshRequest{operation: refreshRequiredFXOperation})
	})
	if c.refreshPending {
		refreshAll.Disable()
		refreshFX.Disable()
	}
	actions := container.NewHBox(refreshAll, refreshFX)

	subtotal := t.T("portfolio.unavailable")
	if portfolio.ValuedSubtotal != nil {
		subtotal = format.Money(portfolio.ValuedSubtotal.Amount, portfolio.ValuedSubtotal.Currency.String(), c.preference)
	}
	positionCount := 0
	for _, account := range portfolio.Accounts {
		for _, component := range account.Components {
			if component.InstrumentID != nil {
				positionCount++
			}
		}
	}
	palette := PaletteFor(c.preference.Accent)
	metrics := container.NewGridWithColumns(3,
		metricCard(t.T("portfolio.valuedSubtotal"), subtotal, t.T("portfolio.valuedSubtotalDescription"), palette.Primary),
		metricCard(t.T("portfolio.positions"), fmt.Sprintf("%d", positionCount), t.T("portfolio.positionsDescription"), palette.Primary),
		metricCard(t.T("portfolio.accounts"), fmt.Sprintf("%d", len(portfolio.Accounts)), t.T("portfolio.accountsDescription"), palette.Primary),
	)

	rows := []fyne.CanvasObject{
		refreshFeedback(c),
		container.NewBorder(nil, nil, nil, actions, nil),
		metrics,
		fxProviderDisclaimer(c),
		investmentsStatusCard(c, portfolio),
		investmentPositionsCard(c, portfolio),
		investmentAccountsCard(c, portfolio),
		investmentAllocationsCard(c, portfolio),
	}
	return container.New(layout.NewCustomPaddedVBoxLayout(12), rows...)
}

func investmentsStatusCard(c *Controller, portfolio domain.PortfolioValuation) fyne.CanvasObject {
	t := c.translator
	rows := []fyne.CanvasObject{
		container.NewHBox(
			badge(t.T("portfolio.completeness"), PaletteFor(c.preference.Accent).Soft, PaletteFor(c.preference.Accent).Primary),
			completenessBadge(c, portfolio.Complete),
		),
	}
	if len(portfolio.MissingInputs) == 0 {
		rows = append(rows, mutedLabel(t.T("portfolio.noMissingInputs")))
	} else {
		missing := make([]fyne.CanvasObject, 0, len(portfolio.MissingInputs)+1)
		missing = append(missing, widget.NewLabelWithStyle(t.T("portfolio.missingInputs"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		for _, item := range portfolio.MissingInputs {
			missing = append(missing, widget.NewLabel(missingInputText(c, item, nil)))
		}
		rows = append(rows, settingsSurface(container.NewVBox(missing...), fyne.NewSize(1, 70)))
	}
	return sectionCard(t.T("portfolio.completeness"), t.T("portfolio.completenessDescription"), container.NewVBox(rows...))
}

func investmentPositionsCard(c *Controller, portfolio domain.PortfolioValuation) fyne.CanvasObject {
	t := c.translator
	rows := make([]fyne.CanvasObject, 0)
	for _, account := range portfolio.Accounts {
		for _, component := range account.Components {
			label := t.T("portfolio.cash")
			if component.InstrumentID != nil {
				label = component.InstrumentID.String()
			}
			rows = append(rows, valueEvidenceRow(c, label, component.NativeAmount, component.NativeCurrency, component, mutedLabel(account.Account.Name)))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, emptyPanel(t.T("portfolio.noPositions"), t.T("portfolio.noPositionsDescription")))
	}
	return sectionCard(t.T("portfolio.positions"), t.T("portfolio.positionsDescription"), container.NewVBox(rows...))
}

func investmentAccountsCard(c *Controller, portfolio domain.PortfolioValuation) fyne.CanvasObject {
	t := c.translator
	rows := make([]fyne.CanvasObject, 0, len(portfolio.Accounts))
	for _, account := range portfolio.Accounts {
		rows = append(rows, settingsSurface(container.NewBorder(nil, nil, nil,
			rowActionButton(t.T("accounts.details"), func(id domain.AccountID) func() {
				return func() { showAccountDetailDialog(c, id) }
			}(account.Account.ID)),
			container.NewVBox(
				widget.NewLabelWithStyle(account.Account.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				mutedLabel(accountValuationSummary(c, account)),
			)), fyne.NewSize(1, 68)))
	}
	if len(rows) == 0 {
		rows = append(rows, emptyPanel(t.T("portfolio.noInvestmentAccounts"), t.T("portfolio.noInvestmentAccountsDescription")))
	}
	return sectionCard(t.T("portfolio.accounts"), t.T("portfolio.accountsDescription"), container.NewVBox(rows...))
}

func investmentAllocationsCard(c *Controller, portfolio domain.PortfolioValuation) fyne.CanvasObject {
	t := c.translator
	if len(portfolio.ByCurrency) == 0 && len(portfolio.ByCountry) == 0 && len(portfolio.ByInstrumentType) == 0 {
		return sectionCard(t.T("portfolio.allocations"), t.T("portfolio.allocationsDescription"), emptyPanel(t.T("portfolio.noAllocations"), t.T("portfolio.noAllocationsDescription")))
	}
	return sectionCard(t.T("portfolio.allocations"), t.T("portfolio.allocationsDescription"), container.NewGridWithColumns(3,
		allocationGroup(c, t.T("portfolio.byCurrency"), portfolio.ByCurrency),
		allocationGroup(c, t.T("portfolio.byCountry"), portfolio.ByCountry),
		allocationGroup(c, t.T("portfolio.byInstrumentType"), portfolio.ByInstrumentType),
	))
}

func allocationGroup(c *Controller, title string, items []domain.AllocationView) fyne.CanvasObject {
	rows := []fyne.CanvasObject{widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})}
	for _, item := range items {
		rows = append(rows, keyValueRow(item.Label, format.Money(item.Amount.Amount, item.Amount.Currency.String(), c.preference)+" · "+format.ShareBPS(item.ShareBPS)))
	}
	if len(items) == 0 {
		rows = append(rows, mutedLabel(c.translator.T("portfolio.noAllocations")))
	}
	return settingsSurface(container.NewVBox(rows...), fyne.NewSize(1, 120))
}
