package desktop

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type directoryCreateSheet struct {
	analysisSheet
	overlay ui.Overlay
}

type directoryCreateDraft struct {
	name, icon string
	kind       domain.InstitutionType
	customIcon bool
}

func newDirectoryCreateSheet() *directoryCreateSheet {
	sheet := newAnalysisSheet()
	sheet.width, sheet.modal, sheet.focus = 420, true, false
	return &directoryCreateSheet{analysisSheet: sheet}
}

func (p *directoryList) layoutCreateSheet(gtx layout.Context, view ui.View) {
	sheet := p.createSheet
	view.Overlay = &sheet.overlay
	sheet.overlay.BeginFrame(gtx)
	if sheet.overlay.Trigger == nil {
		dismiss := sheet.dismissed(gtx, &p.draft.Editor, &p.typePicker.Trigger, &p.iconPicker.Trigger, &p.save)
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			dismiss = dismiss || e.(key.Event).State == key.Press
		}
		if dismiss && !p.busy {
			p.createDraft = directoryCreateDraft{name: p.draft.Text(), icon: p.icon, kind: p.kind, customIcon: p.iconCustomized}
			p.endEditor()
			gtx.Execute(op.InvalidateCmd{})
			return
		}
		for {
			e, ok := gtx.Event(key.Filter{Focus: &sheet.close, Name: key.NameTab, Required: key.ModShift},
				key.Filter{Focus: &p.save, Name: key.NameTab})
			if !ok {
				break
			}
			if e.(key.Event).State == key.Press && !p.busy {
				if gtx.Focused(&sheet.close) {
					gtx.Execute(key.FocusCmd{Tag: &p.save})
				} else {
					gtx.Execute(key.FocusCmd{Tag: &sheet.close})
				}
			}
		}
	}
	sheet.layoutPanel(gtx, view, view.Language.Text(p.copy.add), func(gtx layout.Context, view ui.View) layout.Dimensions {
		bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
		view = view.WithScrollAnchors(&sheet.list)
		sheet.overlay.ScrollOutsidePanel(gtx, &sheet.list, bounds)
		dims := view.List(&sheet.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return p.createEditor(gtx, view)
		})
		sheet.overlay.OffsetScrollAnchor(&sheet.list, bounds)
		return dims
	}, nil, !p.busy)
	sheet.overlay.Layout(gtx, view)
}

func (p *directoryList) createEditor(gtx layout.Context, view ui.View) layout.Dimensions {
	if p.busy {
		gtx = gtx.Disabled()
	}
	submit := gtx.Enabled() && p.save.Clicked(gtx)
	for {
		e, ok := gtx.Event(key.Filter{Focus: &p.draft.Editor, Name: key.NameReturn}, key.Filter{Focus: &p.draft.Editor, Name: key.NameEnter})
		if !ok {
			break
		}
		submit = submit || e.(key.Event).State == key.Press
	}
	l := view.Language
	gap := gtx.Dp(ui.Space12)
	origin := view.Origin
	fields := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		dims := p.nameEditor(gtx, view, p.copy.nameHint)
		origin.Y += dims.Size.Y + gap
		return dims
	})}
	if p.isInstitutions() {
		fields = append(fields, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			field := ui.PickerField{
				Label: i18n.InstitutionsType, Origin: origin, Disabled: !gtx.Enabled(),
				Previous: &p.draft.Editor, Next: &p.iconPicker.Trigger,
			}
			p.typePicker.Layout(gtx, view, field, ui.SelectOptions{Items: institutionChoices(view, directoryInstitutionTypeLabel)}, string(p.kind), func(value string) {
				p.kind = domain.InstitutionType(value)
				if !p.iconCustomized {
					p.icon = p.fallbackIcon(p.kind)
				}
			})
			height := gtx.Dp(60)
			origin.Y += height + gap
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
		}))
	}
	fields = append(fields, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		field := ui.PickerField{
			Label: i18n.DirectoryIcon, Origin: origin, Disabled: !gtx.Enabled(), Previous: &p.draft.Editor, Next: &p.save,
		}
		if p.isInstitutions() {
			field.Previous = &p.typePicker.Trigger
		}
		p.layoutIconPicker(gtx, view, field)
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
	}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		label := i18n.InstitutionsCreate
		if p.busy {
			label = i18n.DirectorySaving
		}
		return view.PrimaryButton(&p.save, l.Text(label))(gtx)
	}))
	dims := layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx, fields...)
	if submit {
		p.submitEditor(gtx)
	}
	return dims
}
