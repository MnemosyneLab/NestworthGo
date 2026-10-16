package liquidity

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type ListReviewAlertsV2Request struct {
	Filter   domain.ReviewAlertFilter `json:"filter"`
	Cursor   string                   `json:"cursor"`
	PageSize int                      `json:"pageSize"`
}

type ListReviewCoverageV2Request struct {
	Filter   domain.ReviewCoverageFilter `json:"filter"`
	Cursor   string                      `json:"cursor"`
	PageSize int                         `json:"pageSize"`
}

type ListReviewDailyEvidenceV2Request struct {
	EpisodeID string `json:"episodeId"`
	Cursor    string `json:"cursor"`
	PageSize  int    `json:"pageSize"`
}

type ListReviewEpisodeHistoryV2Request struct {
	RuleID   string `json:"ruleId"`
	Cursor   string `json:"cursor"`
	PageSize int    `json:"pageSize"`
}

func (s *Service) ListReviewEpisodeHistoryV2(ctx context.Context, request ListReviewEpisodeHistoryV2Request) (domain.ReviewEpisodeHistoryPage, error) {
	page, err := s.app.ListReviewEpisodeHistoryV2(ctx, request.RuleID, request.Cursor, request.PageSize)
	return page, apierror.Wrap(err)
}

type ListReviewEpisodeEventsV2Request struct {
	EpisodeID string `json:"episodeId"`
	Cursor    string `json:"cursor"`
	PageSize  int    `json:"pageSize"`
}

func (s *Service) ListReviewEpisodeEventsV2(ctx context.Context, request ListReviewEpisodeEventsV2Request) (domain.ReviewEpisodeEventPage, error) {
	page, err := s.app.ListReviewEpisodeEventsV2(ctx, request.EpisodeID, request.Cursor, request.PageSize)
	return page, apierror.Wrap(err)
}

func (s *Service) ListReviewDailyEvidenceV2(ctx context.Context, request ListReviewDailyEvidenceV2Request) (domain.ReviewDailyEvidencePage, error) {
	page, err := s.app.ListReviewDailyEvidenceV2(ctx, request.EpisodeID, request.Cursor, request.PageSize)
	return page, apierror.Wrap(err)
}

func (s *Service) ListReviewCoverageV2(ctx context.Context, request ListReviewCoverageV2Request) (domain.ReviewCoveragePage, error) {
	page, err := s.app.ListReviewCoverageV2(ctx, request.Filter, request.Cursor, request.PageSize)
	return page, apierror.Wrap(err)
}

func (s *Service) ListReviewAlertsV2(ctx context.Context, request ListReviewAlertsV2Request) (domain.ReviewAlertPage, error) {
	page, err := s.app.ListReviewAlertGroupsV2(ctx, request.Filter, request.Cursor, request.PageSize)
	return page, apierror.Wrap(err)
}

type ListReviewRulesV2Request struct {
	Cursor   string `json:"cursor"`
	PageSize int    `json:"pageSize"`
}

func (s *Service) ListReviewRulesV2(ctx context.Context, request ListReviewRulesV2Request) ([]domain.ReviewRule, error) {
	rules, err := s.app.ListReviewRulesV2(ctx, request.Cursor, request.PageSize)
	return rules, apierror.Wrap(err)
}

func (s *Service) CreateReviewRuleV2(ctx context.Context, request application.SaveReviewRuleV2Input) (domain.ReviewRule, error) {
	rule, err := s.app.CreateReviewRuleV2(ctx, request)
	return rule, apierror.Wrap(err)
}

func (s *Service) UpdateReviewRuleV2(ctx context.Context, request application.SaveReviewRuleV2Input) (domain.ReviewRule, error) {
	rule, err := s.app.UpdateReviewRuleV2(ctx, request)
	return rule, apierror.Wrap(err)
}

type ArchiveReviewRuleV2Request struct {
	RuleID           string `json:"ruleId"`
	MutationID       string `json:"mutationId"`
	ExpectedRevision int    `json:"expectedRevision"`
}

func (s *Service) ArchiveReviewRuleV2(ctx context.Context, request ArchiveReviewRuleV2Request) (domain.ReviewRule, error) {
	rule, err := s.app.ArchiveReviewRuleV2(ctx, request.RuleID, request.MutationID, request.ExpectedRevision)
	return rule, apierror.Wrap(err)
}

type PreviewReviewRuleV2Request struct {
	Spec                domain.ReviewRuleSpec `json:"spec"`
	InstrumentID        string                `json:"instrumentId"`
	LatestClosedOn      string                `json:"latestClosedOn"`
	ExpectedSourceToken string                `json:"expectedSourceToken"`
}

func (s *Service) PreviewReviewRuleV2(ctx context.Context, request PreviewReviewRuleV2Request) (application.ReviewRulePreview, error) {
	preview, err := s.app.PreviewReviewRuleV2(ctx, request.Spec, request.InstrumentID, request.LatestClosedOn, request.ExpectedSourceToken)
	return preview, apierror.Wrap(err)
}

func (s *Service) PreviewReviewLegacyConversionV2(ctx context.Context, request application.ReviewLegacyConversionDraft) (application.ReviewLegacyConversionPreview, error) {
	preview, err := s.app.PreviewReviewLegacyConversionV2(ctx, request)
	return preview, apierror.Wrap(err)
}

func (s *Service) ConvertReviewLegacyRuleV2(ctx context.Context, request application.ConvertReviewLegacyRuleV2Input) ([]domain.ReviewRule, error) {
	rules, err := s.app.ConvertReviewLegacyRuleV2(ctx, request)
	return rules, apierror.Wrap(err)
}

type AcknowledgeReviewEpisodeV2Request struct {
	EpisodeID        string `json:"episodeId"`
	MutationID       string `json:"mutationId"`
	ExpectedRevision int    `json:"expectedRevision"`
}

func (s *Service) AcknowledgeReviewEpisodeV2(ctx context.Context, request AcknowledgeReviewEpisodeV2Request) (domain.ReviewAcknowledgement, error) {
	acknowledged, err := s.app.AcknowledgeReviewEpisodeV2(ctx, request.EpisodeID, request.MutationID, request.ExpectedRevision)
	return acknowledged, apierror.Wrap(err)
}

type ListReviewSuppressionsV2Request struct {
	Cursor   string `json:"cursor"`
	PageSize int    `json:"pageSize"`
}

func (s *Service) ListReviewSuppressionsV2(ctx context.Context, request ListReviewSuppressionsV2Request) ([]domain.ReviewSuppression, error) {
	items, err := s.app.ListReviewSuppressionsV2(ctx, request.Cursor, request.PageSize)
	return items, apierror.Wrap(err)
}

func (s *Service) SetReviewSuppressionV2(ctx context.Context, request application.SetReviewSuppressionV2Input) (domain.ReviewSuppression, error) {
	item, err := s.app.SetReviewSuppressionV2(ctx, request)
	return item, apierror.Wrap(err)
}

type ClearReviewSuppressionV2Request struct {
	SuppressionID    string `json:"suppressionId"`
	MutationID       string `json:"mutationId"`
	ExpectedRevision int    `json:"expectedRevision"`
}

func (s *Service) ClearReviewSuppressionV2(ctx context.Context, request ClearReviewSuppressionV2Request) error {
	return apierror.Wrap(s.app.ClearReviewSuppressionV2(ctx, request.SuppressionID, request.MutationID, request.ExpectedRevision))
}

func (s *Service) ReconcileInvestmentReviewsV2(ctx context.Context) (application.ReviewReconcileResult, error) {
	result, err := s.app.ReconcileInvestmentReviewsV2(ctx)
	return result, apierror.Wrap(err)
}
