package airuntime

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

// Context is supplied by Wails; cancellation aborts the provider upload.
func (s *Service) Transcribe(ctx context.Context, q assistant.AudioInput) (string, error) {
	if s.runtime == nil {
		return "", assistant.ErrUnavailable
	}
	return s.runtime.Transcribe(ctx, q)
}
