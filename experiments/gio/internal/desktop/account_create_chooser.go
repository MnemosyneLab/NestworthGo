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
	mode                 domain.TrackingMode
	modes                []domain.TrackingMode
	modeButtons          map[domain.TrackingMode]*widget.Clickable
	cards                map[domain.AccountType]*widget.Clickable
	institutionButtons   map[string]*widget.Clickable
	noInstitution        widget.Clickable
	toggle, next, cancel widget.Clickable
	back                 widget.Clickable
}

func newAccountTypeChooser() *accountTypeChooser {
	c := &accountTypeChooser{selected: domain.TypeBankAccount, cards: map[domain.AccountType]*widget.Clickable{},
		institutionButtons: map[string]*widget.Clickable{}}
	for _, kind := range slices.Concat(primaryAccountTypes, moreAccountTypes) {
		c.cards[kind] = new(widget.Clickable)
	}
	return c
}

// accountTypeSetup returns the role and the default tracking mode for a type,
// as the Wails wizard defaults them: the first legal role, balance when the
// type offers it, and otherwise the first legal mode. Brokerage-style types
// still start on holdings. The default is only the preselected choice; every
// legal mode for that role stays selectable.
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

// accountModes lists the tracking modes LegalAccountCombinations allows for one
// type and role, in catalog order. A type card and the creation form offer
// exactly this list.
func accountModes(kind domain.AccountType, role domain.BalanceSheetRole) []domain.TrackingMode {
	var modes []domain.TrackingMode
	seen := map[domain.TrackingMode]struct{}{}
	for _, option := range domain.LegalAccountCombinations() {
		if option.AccountType != kind || option.BalanceSheetRole != role {
			continue
		}
		if _, ok := seen[option.TrackingMode]; ok {
			continue
		}
		seen[option.TrackingMode] = struct{}{}
		modes = append(modes, option.TrackingMode)
	}
	return modes
}

// accountTypeModes lists the tracking modes the creation form can start or
// switch to for a type's default role, so a card promises nothing the form
// cannot do.
func accountTypeModes(kind domain.AccountType) []domain.TrackingMode {
	role, _ := accountTypeSetup(kind)
	return accountModes(kind, role)
}

func (p *accountsPage) startAccountChooser() {
	if p.creation.sheet != nil {
		return
	}
	p.creation.chooser = newAccountTypeChooser()
	p.creation.sheet = newAccountCreateSheet(accountType, &p.createAccount)
}

func (p *accountsPage) layoutAccountTypeChooser(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.creation, view.Language
	c := f.chooser
	if c.cancel.Clicked(gtx) {
		p.cancelAccountCreate(gtx)
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
		if d := f.draft; d == nil {
			p.startAccount(kind, role, mode, domain.Institution{})
		} else if d.kind != kind {
			if d.mode != mode {
				d.opening.Editor.SetText("")
			}
			d.kind, d.role, d.mode = kind, role, mode
			d.included = domain.SuggestedInclusion(kind, mode).IncludeInNetWorth
			d.institutionExplicit = false
			d.applyInstitutionDefault()
			d.problem = i18n.NoMessage
		}
		f.sheet.move(accountInstitution)
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

// layoutAccountTrackingChooser asks which legal tracking method to use when the
// chosen type has more than one. The account type stays the one already chosen.
func (p *accountsPage) layoutAccountTrackingChooser(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.creation, view.Language
	c := f.chooser
	if c.cancel.Clicked(gtx) {
		p.cancelAccountCreate(gtx)
		return layout.Dimensions{}
	}
	if c.back.Clicked(gtx) {
		f.sheet.move(accountInstitution)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	for mode, button := range c.modeButtons {
		if slices.Contains(c.modes, mode) && button.Clicked(gtx) {
			c.mode = mode
		}
	}
	if c.next.Clicked(gtx) {
		f.draft.selectMode(c.mode)
		f.draft.focusName = true
		f.sheet.move(accountDetails)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	typeName := l.Text(accountTypes[c.selected])
	rows := []func(gtx layout.Context, view ui.View) layout.Dimensions{
		func(gtx layout.Context, view ui.View) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return accountStepper(gtx, view, 1) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return view.Line(gtx, ui.TextHeading, l.Text(i18n.AccountTrackingStepTitle), view.Colors.Ink)
						}),
						layout.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.AccountTrackingStepHelp), typeName), view.Colors.SecondaryInk)),
					)
				}),
			)
		},
		func(gtx layout.Context, view ui.View) layout.Dimensions {
			_, recommended := accountTypeSetup(c.selected)
			children := make([]layout.FlexChild, len(c.modes))
			for i, mode := range c.modes {
				children[i] = layout.Rigid(accountTrackingChoice(view, c.modeButtons[mode], mode, c.selected, c.mode == mode, mode == recommended))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
		},
		func(gtx layout.Context, view ui.View) layout.Dimensions {
			back := l.Text(i18n.AccountCreateBack)
			return layout.Flex{Spacing: layout.SpaceStart, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Rigid(view.Button(&c.cancel, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(view.ButtonOf(&c.back, ui.ButtonSpec{Variant: ui.ButtonGhost, Title: back, Name: back})),
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

func accountTrackingChoice(view ui.View, button *widget.Clickable, mode domain.TrackingMode, kind domain.AccountType, selected, recommended bool) layout.Widget {
	label := view.Language.Text(accountTrackingLabel(mode, kind))
	help := i18n.AccountTrackingBalanceHelp
	switch {
	case mode == domain.TrackingManualValue:
		help = i18n.AccountTrackingManualHelp
	case mode == domain.TrackingHoldings && kind == domain.TypeCashOnHand:
		help = i18n.AccountTrackingCashHelp
	case mode == domain.TrackingHoldings:
		help = i18n.AccountTrackingHoldingsHelp
	}
	variant := ui.ButtonOutline
	if selected {
		variant = ui.ButtonSoft
	}
	description := view.Language.Text(help)
	if recommended {
		description = view.Language.Text(i18n.AccountTrackingRecommended) + " · " + description
	}
	return view.ButtonOf(button, ui.ButtonSpec{Variant: variant, Title: label, Name: label, Description: description,
		Toggle: true, Selected: selected, Fill: true, Content: func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space4}).Layout(gtx,
				ui.Rigid(view.Body(ui.TextLabel, label, view.Colors.Ink)),
				ui.Rigid(view.Body(ui.TextCaption, description, view.Colors.SecondaryInk)))
		}})
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
						layout.Rigid(view.Body(ui.TextLabel, name, view.Colors.Ink)),
						layout.Rigid(view.Body(ui.TextCaption, detail, view.Colors.SecondaryInk)),
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

// accountStepper shows the four stages of creating an account. A finished
// stage carries a check, the current one is filled and later ones are quiet.
func accountStepper(gtx layout.Context, view ui.View, current int) layout.Dimensions {
	l := view.Language
	steps := []i18n.MessageID{i18n.AccountStepType, i18n.AccountMaintenanceInstitution, i18n.AccountStepDetails, i18n.AccountStepReview}
	showLabels := ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 440)
	var children []layout.FlexChild
	for i, step := range steps {
		if i > 0 {
			done := i <= current
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return stepConnector(gtx, view, done)
			}))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return stepDot(gtx, view, i, current) }))
		if showLabels {
			children = append(children, layout.Rigid(layout.Spacer{Width: ui.Space8}.Layout), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				ink := view.Colors.SecondaryInk
				if i <= current {
					ink = view.Colors.Ink
				}
				return view.Line(gtx, ui.TextLabel, l.Text(step), ink)
			}))
		}
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
