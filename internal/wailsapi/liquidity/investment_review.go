package liquidity

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// The review contract contains exact decimal strings and actual date labels.
// It is separate from scheduled liquidity actions, which remain read-only.
func (s *Service) InvestmentReviews(ctx context.Context) (application.InvestmentReviewReport, error) {
	report, err := s.app.InvestmentReviews(ctx)
	return report, apierror.Wrap(err)
}

type SaveInvestmentReviewPolicyRequest struct {
	Policy           domain.InvestmentReviewPolicy `json:"policy"`
	ExpectedRevision int                           `json:"expectedRevision"`
}

func (s *Service) SaveInvestmentReviewPolicy(ctx context.Context, request SaveInvestmentReviewPolicyRequest) (application.InvestmentReviewReport, error) {
	report, err := s.app.SaveInvestmentReviewPolicy(ctx, request.Policy, request.ExpectedRevision)
	return report, apierror.Wrap(err)
}
func (s *Service) AcknowledgeInvestmentReview(ctx context.Context, episodeID string) (application.InvestmentReviewReport, error) {
	report, err := s.app.AcknowledgeInvestmentReview(ctx, episodeID)
	return report, apierror.Wrap(err)
}

type SetInvestmentReviewEnrollmentRequest struct {
	InstrumentID     string `json:"instrumentId"`
	Enabled          bool   `json:"enabled"`
	ExpectedRevision int    `json:"expectedRevision"`
}

func (s *Service) SetInvestmentReviewEnrollment(ctx context.Context, request SetInvestmentReviewEnrollmentRequest) (application.InvestmentReviewReport, error) {
	report, err := s.app.SetInvestmentReviewEnrollment(ctx, request.InstrumentID, request.Enabled, request.ExpectedRevision)
	return report, apierror.Wrap(err)
}
