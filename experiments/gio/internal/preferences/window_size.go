package preferences

import (
	"context"
	"errors"
	"image"
	"log/slog"

	"github.com/waltwang/nestworth-go/internal/settings"
)

func ValidWindowSize(size image.Point) bool {
	return size.X >= settings.MinWindowWidth && size.X <= settings.MaxWindowWidth &&
		size.Y >= settings.MinWindowHeight && size.Y <= settings.MaxWindowHeight
}

// WindowSize returns the startup dimensions in logical pixels.
func (s *Service) WindowSize() image.Point {
	current, err := s.store.Load()
	if err == nil {
		size := image.Pt(int(current.WindowWidth), int(current.WindowHeight))
		if ValidWindowSize(size) {
			return size
		}
	}
	return image.Pt(settings.DefaultWindowWidth, settings.DefaultWindowHeight)
}

// RememberWindowSize saves the geometry without interrupting a window event.
func (s *Service) RememberWindowSize(ctx context.Context, size image.Point) {
	if err := s.saveWindowSize(ctx, size); err != nil {
		slog.Warn("could not persist window size")
	}
}

// saveWindowSize patches the latest profile under the same gate as Settings
// saves and resets. Geometry alone does not invalidate financial previews.
func (s *Service) saveWindowSize(ctx context.Context, size image.Point) error {
	if !ValidWindowSize(size) {
		return errors.New("window size is outside the persisted bounds")
	}
	return s.app.WithPrivateConfiguration(ctx, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, status, err := s.store.LoadWithStatus()
		if err != nil {
			return err
		}
		if status == settings.LoadStatusRecovered {
			return errors.New("settings require explicit recovery before saving the window size")
		}
		next := current
		next.WindowWidth, next.WindowHeight = float32(size.X), float32(size.Y)
		return s.persist(current, next)
	})
}
