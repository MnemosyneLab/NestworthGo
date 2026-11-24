package desktop

import (
	"context"
	"image"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
)

// Run serves the native window and joins pending reads before returning to the database owner.
func Run(ctx context.Context, service *application.Service, prefs *preferences.Service, wordmark, mark image.Image) error {
	ctx, cancel := context.WithCancel(ctx)
	window := new(app.Window)
	state := newPreferenceState(ctx, prefs, window.Invalidate)
	state.setLanguage = service.SetUILanguage
	stopWatching := preferences.WatchSystemChanges(state.systemUpdate)
	state.reload()
	directory := newInstitutionsPage(ctx, service, window.Invalidate)
	activities := history.New(ctx, service, window.Invalidate)
	timeline := newHistoryPage(activities)
	defer func() {
		stopWatching()
		cancel()
		state.wait()
		directory.workers.Wait()
		activities.Wait()
	}()
	return newShell(wordmark, mark, ledgerPages{directory: directory.layout, history: timeline.layout, prefs: state}).run(window)
}

func (s *shell) run(window *app.Window) error {
	window.Option(app.Title(pilot.DisplayName), app.Size(1120, 840), app.MinSize(700, 460))
	ax := accessibility.New(window)
	defer ax.Close()
	var ops op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.ViewEvent:
			ax.View(event)
		case app.ConfigEvent:
			if !event.Config.Focused && s.overlay.Trigger != nil {
				s.overlay.Close()
				window.Invalidate()
			}
		case app.FrameEvent:
			gtx := ax.Begin(app.NewContext(&ops, event))
			s.layout(gtx)
			event.Frame(gtx.Ops)
			ax.Publish(gtx)
		}
	}
}
