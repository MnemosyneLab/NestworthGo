package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// The type order follows the Wails create wizard: the ten common types come
// first and the rest sit behind "More account types".
var (
	primaryAccountTypes = []domain.AccountType{
		domain.TypeBankAccount, domain.TypeBrokerage, domain.TypeInvestmentAccount,
		domain.TypeCryptoExchange, domain.TypeDigitalWallet, domain.TypeProperty,
		domain.TypeVehicle, domain.TypeCreditCard, domain.TypeLoan, domain.TypeOther,
	}
	moreAccountTypes = []domain.AccountType{
		domain.TypeCashOnHand, domain.TypePension, domain.TypeInsurancePolicy,
		domain.TypeCollectible, domain.TypeReceivable,
	}
)

// accountTypeChooser is the first step of account creation. It holds only the
// selection; the form draft starts when the user continues.
type accountTypeChooser struct {
	selected             domain.AccountType
	more                 bool
	cards                map[domain.AccountType]*widget.Clickable
	toggle, next, cancel widget.Clickable
}

func newAccountTypeChooser() *accountTypeChooser {
	c := &accountTypeChooser{selected: domain.TypeBankAccount, cards: map[domain.AccountType]*widget.Clickable{}}
	for _, kind := range slices.Concat(primaryAccountTypes, moreAccountTypes) {
		c.cards[kind] = new(widget.Clickable)
	}
	return c
}

// accountTypeSetup returns the role and tracking mode a type starts with, as
// the Wails wizard defaults them: the first legal role, the balance mode when
// the type offers it and otherwise the first legal mode.
func accountTypeSetup(kind domain.AccountType) (domain.BalanceSheetRole, domain.TrackingMode) {
	role, mode := domain.RoleAsset, domain.TrackingBalance
	found := false
	for _, option := range domain.LegalAccountCombinations() {
		if option.AccountType != kind {
			continue
		}
		if !found {
			role, mode, found = option.BalanceSheetRole, option.TrackingMode, true
			continue
		}
		if option.BalanceSheetRole == role && option.TrackingMode == domain.TrackingBalance {
			mode = option.TrackingMode
		}
	}
	if kind == domain.TypeBrokerage || kind == domain.TypeInvestmentAccount || kind == domain.TypeCryptoExchange || kind == domain.TypePension {
		mode = domain.TrackingHoldings
	}
	return role, mode
}

// accountTypeModes lists the tracking modes the creation form can start or
// switch to for a type, so a card promises nothing the form cannot do.
func accountTypeModes(kind domain.AccountType) []domain.TrackingMode {
	role, mode := accountTypeSetup(kind)
	if mode == domain.TrackingBalance {
		return []domain.TrackingMode{domain.TrackingBalance}
	}
	var modes []domain.TrackingMode
	for _, candidate := range []domain.TrackingMode{domain.TrackingHoldings, domain.TrackingManualValue} {
		if domain.IsValidAccountCombination(kind, role, candidate) {
			modes = append(modes, candidate)
		}
	}
	return modes
}

func (p *accountsPage) startAccountChooser() {
	if p.creation.draft != nil || p.creation.chooser != nil {
		return
	}
	p.creation.chooser = newAccountTypeChooser()
}

func (p *accountsPage) layoutAccountTypeChooser(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.creation, view.Language
	c := f.chooser
	if c.cancel.Clicked(gtx) {
		f.chooser, p.focusCreate = nil, true
		view.RestorePageScroll(0)
		p.read()
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	for kind, card := range c.cards {
		if card.Clicked(gtx) {
			c.selected = kind
		}
	}
	if c.toggle.Clicked(gtx) {
		c.more = !c.more
	}
	if c.next.Clicked(gtx) {
		kind := c.selected
		role, mode := accountTypeSetup(kind)
		f.chooser = nil
		view.RestorePageScroll(0)
		p.startAccount(kind, role, mode, domain.Institution{})
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	types := primaryAccountTypes
	if c.more {
		types = slices.Concat(primaryAccountTypes, moreAccountTypes)
	}
	toggle := i18n.AccountTypeMore
	if c.more {
		toggle = i18n.AccountTypeFewer
	}
	rows := []func(gtx layout.Context, view ui.View) layout.Dimensions{
		func(gtx layout.Context, view ui.View) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return accountStepper(gtx, view, 0) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return view.Line(gtx, ui.TextHeading, l.Text(i18n.AccountTypeStepTitle), view.Colors.Ink)
						}),
						layout.Rigid(view.Body(ui.TextCaption, l.Text(i18n.AccountTypeStepHelp), view.Colors.SecondaryInk)),
					)
				}),
			)
		},
		func(gtx layout.Context, view ui.View) layout.Dimensions {
			return (ui.Grid{MinColumn: 260, MaxColumns: 2, Gap: ui.Space12}).Layout(gtx, view, len(types),
				func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
					return accountTypeCard(gtx, view, c.cards[types[i]], types[i], c.selected == types[i])
				})
		},
		func(gtx layout.Context, view ui.View) layout.Dimensions {
			return view.ButtonOf(&c.toggle, ui.ButtonSpec{Variant: ui.ButtonGhost, Title: l.Text(toggle), Name: l.Text(toggle)})(gtx)
		},
		func(gtx layout.Context, view ui.View) layout.Dimensions {
			return layout.Flex{Spacing: layout.SpaceStart, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Rigid(view.Button(&c.cancel, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(view.PrimaryButton(&c.next, l.Text(i18n.AccountTypeContinue))),
			)
		},
	}
	gtx, view = view.Measure(gtx, 720)
	children := make([]ui.ColumnChild, len(rows))
	for i, row := range rows {
		children[i] = ui.Rigid(func(gtx layout.Context) layout.Dimensions { return row(gtx, view) })
	}
	return ui.Column{Gap: ui.Space16}.Layout(gtx, children...)
}

// accountTypeCard is one selectable type: its icon tile, its name and the
// tracking methods the form supports for it.
func accountTypeCard(gtx layout.Context, view ui.View, button *widget.Clickable, kind domain.AccountType, selected bool) layout.Dimensions {
	l := view.Language
	name := l.Text(accountTypes[kind])
	var methods []string
	for _, mode := range accountTypeModes(kind) {
		methods = append(methods, l.Text(accountTrackingLabel(mode, kind)))
	}
	detail := fmt.Sprintf(l.Text(i18n.AccountTypeTracking), strings.Join(methods, " · "))
	variant := ui.ButtonOutline
	if selected {
		variant = ui.ButtonSoft
	}
	glyph := directoryGlyphs[domain.DefaultAccountIcon(kind)]
	return view.ButtonOf(button, ui.ButtonSpec{
		Variant: variant, Title: name, Name: name, Description: detail, Toggle: true, Selected: selected, Fill: true,
		Content: func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return directoryTile(gtx, view, kind.String(), glyph)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Line(gtx, ui.TextLabel, name, view.Colors.Ink) }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return view.Line(gtx, ui.TextCaption, detail, view.Colors.SecondaryInk)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					side := gtx.Dp(16)
					if selected {
						ui.LayoutGlyph(gtx, lucide.Check, 16, view.Colors.Accent)
					}
					return layout.Dimensions{Size: image.Pt(side, side)}
				}),
			)
		},
	})(gtx)
}

// accountStepper shows the three stages of creating an account. A finished
// stage carries a check, the current one is filled and later ones are quiet.
func accountStepper(gtx layout.Context, view ui.View, current int) layout.Dimensions {
	l := view.Language
	steps := []i18n.MessageID{i18n.AccountStepType, i18n.AccountStepDetails, i18n.AccountStepReview}
	var children []layout.FlexChild
	for i, step := range steps {
		if i > 0 {
			done := i <= current
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return stepConnector(gtx, view, done)
			}))
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return stepDot(gtx, view, i, current) }),
			layout.Rigid(layout.Spacer{Width: ui.Space8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				ink := view.Colors.SecondaryInk
				if i <= current {
					ink = view.Colors.Ink
				}
				return view.Line(gtx, ui.TextLabel, l.Text(step), ink)
			}),
		)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func stepDot(gtx layout.Context, view ui.View, index, current int) layout.Dimensions {
	colors := view.Colors
	side := gtx.Dp(24)
	bounds := image.Rectangle{Max: image.Pt(side, side)}
	fill, ink := colors.Accent, colors.OnAccent
	if index > current {
		fill, ink = colors.Surface, colors.SecondaryInk
	}
	ui.FillRoundRect(gtx, bounds, ui.RadiusFull, fill)
	if index > current {
		ui.StrokeRoundRect(gtx, bounds, ui.RadiusFull, 1, colors.Divider)
	}
	gtx.Constraints = layout.Exact(bounds.Size())
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if index < current {
			return ui.LayoutGlyph(gtx, lucide.Check, 14, ink)
		}
		return view.Line(gtx, ui.TextCaption, fmt.Sprint(index+1), ink)
	})
}

func stepConnector(gtx layout.Context, view ui.View, done bool) layout.Dimensions {
	inset := gtx.Dp(ui.Space12)
	width, height := max(0, gtx.Constraints.Max.X-2*inset), gtx.Dp(24)
	color := view.Colors.Divider
	if done {
		color = view.Colors.Accent
	}
	y := height / 2
	rect := image.Rect(inset, y, inset+width, y+max(1, gtx.Dp(2)))
	paint.FillShape(gtx.Ops, color, clip.Rect(rect).Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
}
