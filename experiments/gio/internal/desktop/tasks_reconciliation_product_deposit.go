package desktop

import (
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type holdingDepositTerms struct {
	mode, yearBasis        ui.Select
	rate, maturityInterest ui.TextField
	paidThrough            ui.DatePicker
	lastPaidThrough        *string
}

func newHoldingDepositTerms() *holdingDepositTerms {
	return &holdingDepositTerms{rate: ui.NewTextField(true), maturityInterest: ui.NewTextField(true)}
}

var productDepositModes = []productFormChoice{
	{string(domain.InterestNone), i18n.ProductDepositNone}, {string(domain.InterestSimpleAct365), i18n.ProductDepositAnnualRate},
	{string(domain.InterestManualMaturityAmount), i18n.ProductDepositMaturityInterest},
}

var productDepositBases = []productFormChoice{
	{string(domain.InterestSimpleAct365), i18n.HoldingReconInterest365},
	{string(domain.InterestSimpleAct360), i18n.HoldingReconInterest360},
}

func depositSimple(mode string) bool {
	return mode == string(domain.InterestSimpleAct365) || mode == string(domain.InterestSimpleAct360)
}

func depositAccess(access string) string {
	if access == string(domain.AccessUnknown) || access == string(domain.AccessExcluded) {
		return access
	}
	return string(domain.AccessOnDate)
}

func (f *holdingProductTerms) changeDepositMode(mode string) {
	f.terms.InterestMode = mode
	f.terms.AnnualRate, f.terms.AnnualRatePercent, f.terms.MaturityInterest, f.terms.InterestPaidThroughOn = nil, nil, nil, nil
	f.deposit.rate.Editor.SetText("")
	f.deposit.maturityInterest.Editor.SetText("")
	if depositSimple(mode) {
		f.terms.InterestPaidThroughOn = f.deposit.lastPaidThrough
	}
}

func (f *holdingProductDraft) depositPrincipal() string {
	if f.add != nil {
		return f.add.principal.Text()
	}
	if f.renewal != nil {
		return f.renewal.principal.Text()
	}
	return f.terms.current.Principal.Amount
}
