// Package airuntime is the bounded desktop conversation adapter. Credential
// mutation, approval and financial writes are not exported through IPC. Mobile
// is not registered here and cannot obtain a household-wide host principal.
package airuntime

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

type Service struct{ runtime *assistant.Runtime }

func NewService(r *assistant.Runtime) *Service { return &Service{runtime: r} }
func (s *Service) Status() assistant.Snapshot {
	if s.runtime == nil {
		return assistant.Snapshot{Version: 1, Reason: "secure_runtime_unavailable"}
	}
	return s.runtime.Snapshot()
}
func (s *Service) List(epoch string) ([]assistant.Conversation, error) {
	if s.runtime == nil {
		return nil, assistant.ErrUnavailable
	}
	return s.runtime.List(context.Background(), epoch)
}
func (s *Service) Create(epoch string) (assistant.Conversation, error) {
	if s.runtime == nil {
		return assistant.Conversation{}, assistant.ErrUnavailable
	}
	return s.runtime.Create(context.Background(), epoch)
}
func (s *Service) Inspect(epoch, id string) (assistant.Conversation, error) {
	if s.runtime == nil {
		return assistant.Conversation{}, assistant.ErrUnavailable
	}
	return s.runtime.Inspect(context.Background(), epoch, id)
}
func (s *Service) Start(q assistant.StartRequest) (assistant.Run, error) {
	if s.runtime == nil {
		return assistant.Run{}, assistant.ErrUnavailable
	}
	return s.runtime.Start(context.Background(), q)
}
func (s *Service) Cancel(epoch, id, runID string) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.Cancel(context.Background(), epoch, id, runID)
}
func (s *Service) Events(epoch, id string, after uint64) (assistant.EventPage, error) {
	if s.runtime == nil {
		return assistant.EventPage{}, assistant.ErrUnavailable
	}
	return s.runtime.Events(context.Background(), epoch, id, after)
}
func (s *Service) Delete(epoch, id string) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.Delete(context.Background(), epoch, id)
}

func (s *Service) Messages(epoch, id string, offset int) (assistant.Conversation, error) {
	if s.runtime == nil {
		return assistant.Conversation{}, assistant.ErrUnavailable
	}
	return s.runtime.Messages(context.Background(), epoch, id, offset)
}
