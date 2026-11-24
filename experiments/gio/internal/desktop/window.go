package desktop

import (
	"context"
	"image"

	"gioui.org/app"
	"gioui.org/op"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/internal/application"
)

// Run serves the native window and joins pending reads before returning to the database owner.
func Run(ctx context.Context, service *application.Service, wordmark, mark image.Image) error {
	ctx, cancel := context.WithCancel(ctx)
	window := new(app.Window)
	directory := newInstitutionsPage(ctx, service, window.Invalidate)
	activities := history.New(ctx, service, window.Invalidate)
	timeline := newHistoryPage(activities)
	defer func() {
		cancel()
		directory.workers.Wait()
		activities.Wait()
	}()
	return newShell(wordmark, mark, ledgerPages{directory: directory.layout, history: timeline.layout}).run(window)
}

func (s *shell) run(window *app.Window) error {
	window.Option(app.Title("Nestworth"), app.Size(1120, 840), app.MinSize(700, 460))
	var ops op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.ConfigEvent:
			if !event.Config.Focused && s.overlay.trigger != nil {
				s.overlay.close()
				window.Invalidate()
			}
		case app.FrameEvent:
			gtx := app.NewContext(&ops, event)
			s.layout(gtx)
			event.Frame(gtx.Ops)
		}
	}
}
