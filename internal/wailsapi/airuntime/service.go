// Package airuntime is the bounded desktop conversation adapter. Model
// configuration and explicit reviewed writes are desktop-only. Mobile
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
		return assistant.Snapshot{Version: 1, Credentials: "unavailable", Reason: "secure_runtime_unavailable"}
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

// SaveCredential accepts new input only. No method returns stored key material.
func (s *Service) SaveCredential(epoch, key string) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	value := []byte(key)
	defer clear(value)
	return s.runtime.SetCredential(context.Background(), epoch, value)
}
func (s *Service) DeleteCredential(epoch string) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.DeleteCredential(context.Background(), epoch)
}

// Configure requires a current host epoch/revision and explicit desktop opt-in.
// It never changes external MCP settings or mobile permissions.
func (s *Service) Configure(epoch string, revision uint64, config assistant.ModelConfig) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.Configure(context.Background(), epoch, revision, config)
}

func (s *Service) PublishPage(epoch string, revision uint64, page assistant.PageSelection) (assistant.DesktopPageLease, error) {
	if s.runtime == nil {
		return assistant.DesktopPageLease{}, assistant.ErrUnavailable
	}
	return s.runtime.PublishPage(context.Background(), epoch, revision, page)
}
func (s *Service) GrantNavigation(epoch string, revision uint64, conversationID, requestID, leaseID string) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.GrantNavigation(context.Background(), epoch, revision, conversationID, requestID, leaseID)
}
func (s *Service) ConsumeNavigation(epoch, conversationID, runID, intentID, leaseID string) (assistant.ModelNavigationTarget, error) {
	if s.runtime == nil {
		return assistant.ModelNavigationTarget{}, assistant.ErrUnavailable
	}
	return s.runtime.ConsumeNavigation(context.Background(), epoch, conversationID, runID, intentID, leaseID)
}

func (s *Service) PendingPageReads(epoch string) ([]assistant.PageReadRequest, error) {
	if s.runtime == nil {
		return nil, assistant.ErrUnavailable
	}
	return s.runtime.PendingPageReads(context.Background(), epoch)
}
func (s *Service) DeliverPageContent(epoch, requestID string, content assistant.PageContent) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.DeliverPageContent(context.Background(), epoch, requestID, content)
}

func (s *Service) SetWritePermission(epoch string, revision uint64, enabled bool) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.SetWritePermission(context.Background(), epoch, revision, enabled)
}
func (s *Service) Reviews(epoch, id string) ([]assistant.ChangeReview, error) {
	if s.runtime == nil {
		return nil, assistant.ErrUnavailable
	}
	return s.runtime.Reviews(context.Background(), epoch, id)
}
func (s *Service) ReviewAction(epoch, id, reviewID, digest, action string) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.ReviewAction(context.Background(), epoch, id, reviewID, digest, action)
}

// SetFullAccess is desktop-only explicit session authority; never model-callable.
func (s *Service) SetFullAccess(epoch string, revision uint64, enabled bool) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.SetFullAccess(context.Background(), epoch, revision, enabled)
}

// Rename gives explicit desktop naming precedence over pending generation.
func (s *Service) Rename(epoch, id, title string) error {
	if s.runtime == nil {
		return assistant.ErrUnavailable
	}
	return s.runtime.Rename(context.Background(), epoch, id, title)
}
