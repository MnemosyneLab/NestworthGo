package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"time"
)

type ReviewCertifiedRepository interface {
	CertifiedReviewEnabled() bool
	ConfirmCertifiedReviewNoChange(context.Context, domain.HouseholdID, time.Time, func() time.Time, domain.ReviewProviderSelection) (domain.ReviewNoChangeResult, bool, error)
	PublishCertifiedReviewProjection(context.Context, domain.ReviewPublication, func() time.Time, func(func() error) error) error
}

func (s *Service) reviewProviderSnapshot() domain.ReviewProviderSelection {
	s.reviewProviderMu.RLock()
	defer s.reviewProviderMu.RUnlock()
	return s.reviewProviderSnapshotLocked()
}
func (s *Service) reviewProviderSnapshotLocked() domain.ReviewProviderSelection {
	s.stateMu.RLock()
	pinned := s.fxProviderKey != "" && !s.reviewProviderExhausted
	s.stateMu.RUnlock()
	return domain.ReviewProviderSelection{Key: s.FXProviderKey(), Revision: s.reviewProviderRevision, Pinned: pinned}
}

// A setter (including ABA and registry replacement) must wait until COMMIT.
// Default-provider registry internals cannot establish reuse authority.
func (s *Service) withReviewProviderLease(expected domain.ReviewProviderSelection, fn func() error) error {
	s.reviewProviderMu.RLock()
	defer s.reviewProviderMu.RUnlock()
	if s.reviewProviderSnapshotLocked() != expected {
		return &domain.Error{Code: domain.ErrConflict, Message: "review provider changed during preparation"}
	}
	return fn()
}

func (s *Service) advanceReviewProviderLocked() {
	if s.reviewProviderRevision == ^uint64(0) {
		s.reviewProviderExhausted = true
		return
	}
	s.reviewProviderRevision++
}

func (s *Service) notifyReviewProviderSelection() {
	if observer, ok := s.repository.(interface {
		CertifiedReviewEnabled() bool
		InvalidateReviewProviderSelection()
	}); ok && observer.CertifiedReviewEnabled() {
		observer.InvalidateReviewProviderSelection()
	}
}
