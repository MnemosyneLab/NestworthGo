package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func (p *onboardingPage) layout(gtx layout.Context, view ui.View, prefs *preferenceState, brand widget.Image) layout.Dimensions {
	size := gtx.Constraints.Max
	width := min(size.X-gtx.Dp(48), gtx.Dp(690))
	origin := image.Pt((size.X-width)/2, gtx.Dp(16))
	stack := op.Offset(origin).Push(gtx.Ops)
	defer stack.Pop()
	gtx.Constraints.Max = image.Pt(width, size.Y-2*origin.Y)
	gtx.Constraints.Min = image.Pt(width, 0)
	gap := gtx.Dp(10)
	layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(7)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(146), gtx.Dp(29)))
					return brand.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, view.Language.Text(i18n.OnboardingTitle), 23, font.Medium, view.Colors.Ink)
				}),
			)
			origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !p.form || !prefs.ready {
				return p.layoutStatus(gtx, view, prefs)
			}
			return p.layoutForm(gtx, view, prefs, origin)
		}),
	)
	return layout.Dimensions{Size: size}
}

func (p *onboardingPage) layoutStatus(gtx layout.Context, view ui.View, prefs *preferenceState) layout.Dimensions {
	l := view.Language
	id, ink := i18n.OnboardingLoading, view.Colors.SecondaryInk
	failed := p.readFailed || (!prefs.ready && prefs.err != nil)
	if failed {
		id, ink = i18n.OnboardingReadFailed, view.Colors.ErrorInk
	}
	children := []layout.FlexChild{}
	if p.confirmed {
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.OnboardingSaved), view.Colors.Ink)))
	}
	children = append(children, layout.Rigid(view.Message(l.Text(id), ink)))
	if failed {
		if p.retry.Clicked(gtx) {
			if p.readFailed {
				p.read()
			}
			if !prefs.ready && prefs.err != nil {
				prefs.reload()
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		children = append(children, layout.Rigid(view.Button(&p.retry, l.Text(i18n.HistoryRetry))))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(12)}.Layout(gtx, children...)
}

func (p *onboardingPage) layoutForm(gtx layout.Context, view ui.View, prefs *preferenceState, origin image.Point) layout.Dimensions {
	l := view.Language
	intro := view.Message(l.Text(i18n.OnboardingIntro), view.Colors.SecondaryInk)
	submit := false
	dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return fieldPosition(gtx, &origin, intro) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if p.writing {
				gtx = gtx.Disabled()
			}
			columnGap := gtx.Dp(22)
			width := (gtx.Constraints.Max.X - columnGap) / 2
			return layout.Flex{Gap: columnGap}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					return p.layoutHousehold(gtx, view, origin)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.layoutInitialMembers(gtx, view, prefs, origin.Add(image.Pt(width+columnGap, 0)))
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			id, ink := p.problem, view.Colors.ErrorInk
			if p.writing {
				id, ink = i18n.OnboardingSaving, view.Colors.SecondaryInk
			} else if prefs.err != nil {
				id = i18n.SettingsSaveError
			}
			if id == i18n.NoMessage {
				return layout.Dimensions{}
			}
			return view.Message(l.Text(id), ink)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.writing || prefs.busy {
				gtx = gtx.Disabled()
			}
			submit = p.create.Clicked(gtx)
			return view.PrimaryButton(&p.create, l.Text(i18n.OnboardingCreate))(gtx)
		}),
	)
	if submit {
		p.submit(view.TimeLocation)
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

func fieldPosition(gtx layout.Context, origin *image.Point, draw layout.Widget) layout.Dimensions {
	dims := draw(gtx)
	origin.Y += dims.Size.Y + gtx.Dp(8)
	return dims
}

func (p *onboardingPage) layoutHousehold(gtx layout.Context, view ui.View, origin image.Point) layout.Dimensions {
	l := view.Language
	day := p.day
	today := p.today(view.TimeLocation)
	if day == "" {
		day = today
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.focusName && gtx.Enabled() {
				p.name.Focus(gtx)
				p.focusName = false
			}
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				dims := p.name.Layout(gtx, view, ui.FieldOptions{Label: i18n.OnboardingName})
				if p.problem == i18n.OnboardingNameRequired && strings.TrimSpace(p.name.Text()) != "" {
					p.problem = i18n.NoMessage
					gtx.Execute(op.InvalidateCmd{})
				}
				return dims
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			p.currencyPicker.Layout(gtx, view, ui.PickerField{Label: i18n.SettingsCurrency, Origin: origin, Disabled: !gtx.Enabled()},
				ui.SelectOptions{Items: supportedCurrencyChoices}, p.currency, func(id string) { p.currency = id })
			origin.Y += gtx.Dp(68)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.toggle.Clicked(gtx) {
				p.historical = !p.historical
				gtx.Execute(op.InvalidateCmd{})
			}
			return fieldPosition(gtx, &origin, view.TabButton(&p.toggle, l.Text(i18n.OnboardingHistorical), p.historical))
		}),
	}
	if p.historical {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				weekStart := time.Monday
				if view.WeekStart == "sunday" {
					weekStart = time.Sunday
				}
				p.calendar.Layout(gtx, view, ui.PickerField{Label: i18n.OnboardingDate, Origin: origin, Disabled: !gtx.Enabled()},
					ui.DateOptions{Today: today, Max: today, WeekStart: weekStart, Format: view.DateFormat}, day, func(value string) { p.day = value })
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
			}),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.OnboardingTimezone), view.TimeLocation.String()), view.Colors.SecondaryInk)),
		)
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (p *onboardingPage) layoutInitialMembers(gtx layout.Context, view ui.View, prefs *preferenceState, origin image.Point) layout.Dimensions {
	l := view.Language
	added := false
	var remove *onboardingMember
	dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			p.languagePicker.Layout(gtx, view, ui.PickerField{Label: i18n.SettingsLanguage, Origin: origin, Disabled: !gtx.Enabled() || prefs.busy},
				ui.SelectOptions{Items: []ui.Choice{
					{ID: string(settings.LanguageSystem), Label: l.Text(i18n.SettingsLanguageSystem)},
					{ID: string(i18n.English), Label: l.Text(i18n.SettingsLanguageEn)},
					{ID: string(i18n.SimplifiedChinese), Label: l.Text(i18n.SettingsLanguageZhCN)},
					{ID: string(i18n.TraditionalChinese), Label: l.Text(i18n.SettingsLanguageZhTW)},
					{ID: string(i18n.Japanese), Label: l.Text(i18n.SettingsLanguageJa)},
				}}, string(prefs.saved.Language), func(id string) {
					after := prefs.saved
					after.Language = settings.Language(id)
					prefs.save(prefs.saved, after, nil)
				})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(5)}.Layout(gtx,
				layout.Flexed(1, view.Message(l.Text(i18n.OnboardingMembers), view.Colors.Ink)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					added = p.add.Clicked(gtx)
					return view.Button(&p.add, l.Text(i18n.OnboardingAddMember))(gtx)
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return view.List(&p.membersList).Layout(gtx, len(p.members), func(gtx layout.Context, i int) layout.Dimensions {
				m := p.members[i]
				return layout.Inset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.End, Gap: gtx.Dp(6)}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if p.focusMember == m && gtx.Enabled() {
								m.name.Focus(gtx)
								p.focusMember = nil
							}
							dims := m.name.Layout(gtx, view, ui.FieldOptions{Label: i18n.OnboardingMemberName})
							if p.problem == i18n.OnboardingMemberRequired && strings.TrimSpace(m.name.Text()) != "" {
								p.problem = i18n.NoMessage
								gtx.Execute(op.InvalidateCmd{})
							}
							return dims
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if len(p.members) == 1 {
								return layout.Dimensions{}
							}
							if m.remove.Clicked(gtx) {
								remove = m
							}
							return view.Button(&m.remove, l.Text(i18n.OnboardingRemoveMember))(gtx)
						}),
					)
				})
			})
		}),
	)
	if added && strings.TrimSpace(p.members[len(p.members)-1].name.Text()) != "" {
		p.focusMember = &onboardingMember{name: ui.NewTextField(true)}
		p.members = append(p.members, p.focusMember)
		p.membersList.ScrollTo(len(p.members) - 1)
		gtx.Execute(op.InvalidateCmd{})
	}
	if remove != nil {
		p.members = slices.DeleteFunc(p.members, func(m *onboardingMember) bool { return m == remove })
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}
