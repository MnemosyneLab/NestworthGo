package desktop

import (
	"image"
	"strconv"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func (s *shell) recentButton(gtx layout.Context, view ui.View, anchorX int) layout.Dimensions {
	size := image.Pt(min(gtx.Dp(96), gtx.Constraints.Max.X), gtx.Dp(31))
	gtx.Constraints = layout.Exact(size)
	title := view.Language.Text(i18n.NavigationRecent)
	if len(s.trail.recent) == 0 {
		gtx = gtx.Disabled()
	}
	if gtx.Enabled() {
		choices := make([]ui.Choice, 0, len(s.trail.recent))
		for i := range s.trail.recent {
			choices = append(choices, ui.Choice{ID: strconv.Itoa(i), Label: s.visitLabel(view.Language, &s.trail.recent[i])})
		}
		y := (gtx.Dp(headerHeight) - size.Y) / 2
		bounds := image.Rectangle{Min: image.Pt(anchorX, y), Max: image.Pt(anchorX+size.X, y+size.Y)}
		previous := &s.forward
		if s.trail.adjacent(1) == nil {
			previous = &s.back
		}
		s.recentPicker.Menu(gtx, view, bounds, choices, "", previous, &s.headingFocus, 340, func(id string) {
			index, err := strconv.Atoi(id)
			if err != nil || index < 0 || index >= len(s.trail.recent) {
				return
			}
			entry := s.trail.recent[index]
			s.pendingNavigation = &navigationRequest{target: entry.target, recent: &entry}
			gtx.Execute(op.InvalidateCmd{})
		})
	}
	return view.Button(&s.recentPicker.Trigger, title)(gtx)
}
