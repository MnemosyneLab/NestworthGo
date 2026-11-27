package desktop

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountCreateStep uint8

const (
	accountType accountCreateStep = iota
	accountInstitution
	accountTracking
	accountDetails
)

type accountCreateSheet struct {
	analysisSheet
	overlay ui.Overlay
	step    accountCreateStep
	source  *widget.Clickable
	reset   bool
}

func newAccountCreateSheet(step accountCreateStep, source *widget.Clickable) *accountCreateSheet {
	sheet := newAnalysisSheet()
	sheet.width, sheet.modal = 680, true
	return &accountCreateSheet{analysisSheet: sheet, step: step, source: source}
}

func (s *accountCreateSheet) resetPosition() {
	s.reset = true
}

func (s *accountCreateSheet) move(step accountCreateStep) {
	s.step, s.reset = step, true
}

func (s *shell) accountCreationOpen() bool {
	return s.activeRoute() == accountsRoute && s.accounts != nil && s.accounts.creation != nil && s.accounts.creation.sheet != nil
}

func (p *accountsPage) updateAccountCreateTrigger(gtx layout.Context) {
	if !gtx.Enabled() || p.creation == nil || p.creation.sheet != nil {
		return
	}
	switch {
	case p.canCreateUnscoped() && p.createAccount.Clicked(gtx):
		p.startAccountChooser()
	case p.canCreateBankAccount() && p.createBank.Clicked(gtx):
		p.startBankAccount()
	case p.canCreateBankAccount() && p.createShell.Clicked(gtx):
		p.startAccount(domain.TypeProperty, domain.RoleAsset, domain.TrackingManualValue, p.current.institutions[p.target.institution])
	default:
		return
	}
	gtx.Execute(op.InvalidateCmd{})
}

func (p *accountsPage) cancelAccountCreate(gtx layout.Context) {
	f := p.creation
	p.focusCreate = f.sheet.source
	f.discard()
	gtx.Execute(op.InvalidateCmd{})
}

func (p *accountsPage) layoutAccountCreateSheet(gtx layout.Context, view ui.View) {
	f := p.creation
	sheet := f.sheet
	view.Overlay = &sheet.overlay
	sheet.overlay.BeginFrame(gtx)
	canClose := f.submission == nil && !f.creatingInstitution()
	if sheet.overlay.Trigger == nil {
		dismiss := sheet.dismissed(gtx)
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			dismiss = dismiss || e.(key.Event).State == key.Press
		}
		if dismiss && canClose {
			p.cancelAccountCreate(gtx)
			return
		}
	}
	if sheet.reset {
		sheet.list.Position, sheet.reset = layout.Position{}, false
	}
	sheet.layoutPanel(gtx, view, view.Language.Text(i18n.AccountCreateWizardTitle), func(gtx layout.Context, view ui.View) layout.Dimensions {
		bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
		view = view.WithScrollAnchors(&sheet.list)
		sheet.overlay.ScrollOutsidePanel(gtx, &sheet.list, bounds)
		dims := view.List(&sheet.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			switch sheet.step {
			case accountType:
				return p.layoutAccountTypeChooser(gtx, view)
			case accountInstitution:
				return p.layoutAccountInstitutionChooser(gtx, view)
			case accountTracking:
				return p.layoutAccountTrackingChooser(gtx, view)
			default:
				if f.review != nil {
					return p.layoutAccountCreationReview(gtx, view)
				}
				return p.layoutAccountCreation(gtx, view)
			}
		})
		sheet.overlay.OffsetScrollAnchor(&sheet.list, bounds)
		return dims
	}, nil, canClose)
	if f.sheet != nil {
		sheet.overlay.Layout(gtx, view)
	}
}
