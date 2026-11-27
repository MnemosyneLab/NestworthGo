package desktop

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/settings"
)

var (
	languageIcon   = ui.Glyph(lucide.Languages)
	monitorIcon    = ui.Glyph(lucide.Monitor)
	sunIcon        = ui.Glyph(lucide.Sun)
	moonIcon       = ui.Glyph(lucide.Moon)
	collapseIcon   = ui.Glyph(lucide.PanelLeftClose)
	expandRailIcon = ui.Glyph(lucide.PanelLeftOpen)
)

func (s *shell) collapsed() bool {
	if s.narrow {
		return !s.narrowExpanded
	}
	return s.wideCollapsed
}

func (s *shell) sidebarWidth() unit.Dp {
	if s.collapsed() {
		return sidebarRailWidth
	}
	return sidebarWidth
}

func sidebarIcon(icon ui.Glyph, ink color.NRGBA, size unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		side := gtx.Dp(size)
		gtx.Constraints = layout.Exact(image.Pt(side, side))
		return icon.Layout(gtx, ink)
	}
}

func sidebarButton(gtx layout.Context, view ui.View, click *widget.Clickable, label, description string, picker bool, body layout.Widget) layout.Dimensions {
	return accessibility.Button(gtx, click, label, description, picker, func(gtx layout.Context) layout.Dimensions {
		return layoutSidebarButton(gtx, view, click, label, description, body)
	})
}

func layoutSidebarButton(gtx layout.Context, view ui.View, click *widget.Clickable, label, description string, body layout.Widget) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.LabelOp(label).Add(gtx.Ops)
		if description != "" {
			semantic.DescriptionOp(description).Add(gtx.Ops)
		}
		if gtx.Enabled() {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		box := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}.Inset(1), gtx.Dp(6))
		if click.Hovered() && gtx.Enabled() {
			paint.FillShape(gtx.Ops, view.Colors.HoverSurface, box.Op(gtx.Ops))
		}
		if gtx.Focused(click) {
			paint.FillShape(gtx.Ops, view.Colors.Accent, clip.Stroke{Path: box.Path(gtx.Ops), Width: 1}.Op())
		}
		return layout.Center.Layout(gtx, body)
	})
}

func (s *shell) sidebarBrand(gtx layout.Context, view ui.View) layout.Dimensions {
	compact := s.collapsed()
	axis, icon := layout.Horizontal, collapseIcon
	if compact {
		axis, icon = layout.Vertical, expandRailIcon
	}
	mark := func(gtx layout.Context) layout.Dimensions {
		side := gtx.Dp(27)
		gtx.Constraints = layout.Exact(image.Pt(side, side))
		return s.mark.Layout(gtx)
	}
	toggle := func(gtx layout.Context) layout.Dimensions {
		side := gtx.Dp(30)
		gtx.Constraints = layout.Exact(image.Pt(side, side))
		return sidebarButton(gtx, view, &s.railButton, view.Language.Text(i18n.SidebarToggle), "", false, sidebarIcon(icon, view.Colors.SecondaryInk, 18))
	}
	return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		parts := []layout.FlexChild{layout.Rigid(mark)}
		if !compact {
			parts = append(parts, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(min(gtx.Dp(116), gtx.Constraints.Max.X), gtx.Dp(30)))
					return s.wordmark.Layout(gtx)
				})
			}))
		}
		if !s.titlebar.active {
			parts = append(parts, layout.Rigid(toggle))
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: axis, Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx, parts...)
		})
	})
}

func (s *shell) sidebarPreferences(gtx layout.Context, view ui.View, height int) layout.Dimensions {
	paint.FillShape(gtx.Ops, view.Colors.Divider, clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X-1, 1)).Op())
	return layout.Inset{Left: 12, Right: 12, Top: 8, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.prefs == nil || !s.prefs.ready || s.prefs.err == nil || s.activeRoute() == settingsRoute {
					return layout.Dimensions{}
				}
				return view.Message(view.Language.Text(i18n.SidebarSaveError), view.Colors.ErrorInk)(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.prefs == nil || !s.prefs.ready || s.prefs.busy {
					gtx = gtx.Disabled()
				}
				// The footer is pinned to the bottom; the menu anchor is in window coordinates.
				width, side := gtx.Constraints.Max.X, gtx.Dp(32)
				gap := gtx.Dp(4)
				y := height - gtx.Dp(8) - side
				x := gtx.Dp(12)
				languageWidth := width - side - gap
				axis := layout.Horizontal
				if s.collapsed() {
					axis = layout.Vertical
					languageWidth = side
					x += (width - side) / 2
					y -= gap + side
				}
				size := image.Pt(languageWidth, side)
				anchor := image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x, y).Add(size)}
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: axis, Alignment: layout.Middle, Gap: gap}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(size)
							return s.quickLanguage(gtx, view, anchor)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(side, side))
							return s.quickAppearance(gtx, view)
						}),
					)
				})
			}),
		)
	})
}

func (s *shell) quickLanguage(gtx layout.Context, view ui.View, bounds image.Rectangle) layout.Dimensions {
	l := view.Language
	choices := []ui.Choice{
		{ID: string(i18n.English), Label: l.Text(i18n.SettingsLanguageEn)},
		{ID: string(i18n.SimplifiedChinese), Label: l.Text(i18n.SettingsLanguageZhCN)},
		{ID: string(i18n.TraditionalChinese), Label: l.Text(i18n.SettingsLanguageZhTW)},
		{ID: string(i18n.Japanese), Label: l.Text(i18n.SettingsLanguageJa)},
	}
	value := string(l)
	picker := &s.languagePicker
	last := s.groups[len(s.groups)-1].items
	picker.Menu(gtx, view, bounds, choices, l.Text(i18n.SettingsLanguage), value, &last[len(last)-1].button, &s.themeButton, 0, func(id string) {
		if id == value {
			return
		}
		before := s.prefs.saved
		after := before
		after.Language = settings.Language(id)
		s.prefs.save(before, after, nil)
	})

	current := ui.ChoiceLabel(view, choices, value)
	s.languageHint.Layout(gtx, view, s.collapsed() && picker.Trigger.Hovered(), current)
	return sidebarButton(gtx, view, &picker.Trigger, l.Text(i18n.SettingsLanguage), current, true, func(gtx layout.Context) layout.Dimensions {
		labelIcon := sidebarIcon(languageIcon, view.Colors.SecondaryInk, 16)
		if s.collapsed() {
			return labelIcon(gtx)
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(6)}.Layout(gtx,
			layout.Rigid(labelIcon),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, current, 12, font.Medium, view.Colors.SecondaryInk)
			}),
			layout.Rigid(sidebarIcon(ui.ExpandIcon, view.Colors.SecondaryInk, 16)),
		)
	})
}

func (s *shell) quickAppearance(gtx layout.Context, view ui.View) layout.Dimensions {
	current := settings.AppearanceSystem
	if s.prefs != nil && s.prefs.ready {
		current = s.prefs.saved.Appearance
	}
	icon, next, name := monitorIcon, settings.AppearanceLight, i18n.SettingsAppearanceSystem
	switch current {
	case settings.AppearanceLight:
		icon, next, name = sunIcon, settings.AppearanceDark, i18n.SettingsAppearanceLight
	case settings.AppearanceDark:
		icon, next, name = moonIcon, settings.AppearanceSystem, i18n.SettingsAppearanceDark
	}
	if s.themeButton.Clicked(gtx) {
		before := s.prefs.saved
		after := before
		after.Appearance = next
		s.prefs.save(before, after, nil)
	}
	s.appearanceHint.Layout(gtx, view, s.themeButton.Hovered(), view.Language.Text(name))
	return sidebarButton(gtx, view, &s.themeButton, view.Language.Text(i18n.SidebarToggleAppearance), view.Language.Text(name), false, sidebarIcon(icon, view.Colors.SecondaryInk, 16))
}
