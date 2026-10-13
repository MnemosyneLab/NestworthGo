package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type InvestmentReviewRepository interface {
	ReadInvestmentReviewSnapshot(context.Context) (domain.InvestmentReviewSnapshot, error)
}
type InvestmentReviewConfiguration interface {
	LoadConfiguration(string) ([]byte, bool, error)
	SaveConfiguration(string, []byte) error
}
type InvestmentReviewReport struct {
	SchemaVersion string                        `json:"schemaVersion"`
	AsOf          string                        `json:"asOf"`
	LocalDate     string                        `json:"localDate"`
	Timezone      string                        `json:"timezone"`
	Revision      int                           `json:"revision"`
	Policy        domain.InvestmentReviewPolicy `json:"policy"`
	Items         []domain.InvestmentReviewItem `json:"items"`
}
type investmentReviewState struct {
	Enrolled     map[string]bool               `json:"enrolled"`
	Revision     int                           `json:"revision"`
	Policy       domain.InvestmentReviewPolicy `json:"policy"`
	Acknowledged map[string]bool               `json:"acknowledged"`
}

func (s *Service) SetInvestmentReviewConfiguration(store InvestmentReviewConfiguration) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	s.reviewStore = store
}
func (s *Service) investmentReviews(ctx context.Context) (InvestmentReviewReport, investmentReviewState, string, error) {
	empty := InvestmentReviewReport{SchemaVersion: "investment-review/1", Policy: domain.DefaultInvestmentReviewPolicy(), Items: []domain.InvestmentReviewItem{}}
	reader, ok := s.repository.(InvestmentReviewRepository)
	if !ok || s.reviewStore == nil {
		return empty, investmentReviewState{}, "", &domain.Error{Code: domain.ErrUnavailable, Message: "investment review storage is unavailable"}
	}
	snapshot, err := reader.ReadInvestmentReviewSnapshot(ctx)
	if err != nil {
		return empty, investmentReviewState{}, "", err
	}
	if snapshot.Gain.Portfolio.Household == nil {
		return empty, investmentReviewState{}, "", nil
	}
	key := "investment_review/" + snapshot.Gain.Portfolio.Household.ID.String()
	state := investmentReviewState{Policy: domain.DefaultInvestmentReviewPolicy(), Acknowledged: map[string]bool{}, Enrolled: map[string]bool{}}
	raw, found, err := s.reviewStore.LoadConfiguration(key)
	if err != nil {
		return empty, state, key, err
	}
	if found {
		if json.Unmarshal(raw, &state) != nil || state.Policy.Validate() != nil || state.Revision < 0 || state.Acknowledged == nil || state.Enrolled == nil {
			return empty, state, key, &domain.Error{Code: domain.ErrIntegrity, Message: "investment review configuration is invalid"}
		}
	}
	state.Policy = state.Policy.Canonical()
	now := s.clock()
	location := time.UTC
	if origin := snapshot.Gain.Portfolio.Origin; origin != nil {
		location, err = time.LoadLocation(origin.Timezone)
		if err != nil {
			return empty, state, key, err
		}
	}
	report := InvestmentReviewReport{SchemaVersion: "investment-review/1", AsOf: now.UTC().Format(time.RFC3339Nano), LocalDate: now.In(location).Format("2006-01-02"), Timezone: location.String(), Revision: state.Revision, Policy: state.Policy, Items: []domain.InvestmentReviewItem{}}
	report.Items = domain.ProjectInvestmentReviews(snapshot, state.Policy, report.LocalDate)
	for n := range report.Items {
		report.Items[n].Monitored = state.Enrolled[report.Items[n].InstrumentID]
		if !report.Items[n].Monitored {
			report.Items[n].EpisodeID = ""
		}
		report.Items[n].Acknowledged = state.Acknowledged[report.Items[n].EpisodeID]
	}
	return report, state, key, nil
}
func (s *Service) InvestmentReviews(ctx context.Context) (InvestmentReviewReport, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	report, _, _, err := s.investmentReviews(ctx)
	return report, err
}
func (s *Service) SaveInvestmentReviewPolicy(ctx context.Context, p domain.InvestmentReviewPolicy, expectedRevision int) (InvestmentReviewReport, error) {
	if err := p.Validate(); err != nil {
		return InvestmentReviewReport{}, err
	}
	p = p.Canonical()
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	defer unlock()
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	_, state, key, err := s.investmentReviews(ctx)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	if key == "" || expectedRevision != state.Revision {
		return InvestmentReviewReport{}, &domain.Error{Code: domain.ErrConflict, Message: "investment review settings changed; reload before saving"}
	}
	state.Policy, state.Revision = p, state.Revision+1
	raw, err := json.Marshal(state)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	if err = s.reviewStore.SaveConfiguration(key, raw); err != nil {
		return InvestmentReviewReport{}, err
	}
	report, _, _, err := s.investmentReviews(ctx)
	return report, err
}
func (s *Service) AcknowledgeInvestmentReview(ctx context.Context, episodeID string) (InvestmentReviewReport, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	defer unlock()
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	report, state, key, err := s.investmentReviews(ctx)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	valid := false
	for _, item := range report.Items {
		if item.EpisodeID != "" && item.EpisodeID == episodeID {
			valid = true
		}
	}
	if !valid {
		return InvestmentReviewReport{}, &domain.Error{Code: domain.ErrConflict, Message: "investment review episode is no longer current"}
	}
	state.Acknowledged[episodeID] = true
	raw, err := json.Marshal(state)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	if err = s.reviewStore.SaveConfiguration(key, raw); err != nil {
		return InvestmentReviewReport{}, err
	}
	for n := range report.Items {
		if report.Items[n].EpisodeID == episodeID {
			report.Items[n].Acknowledged = true
		}
	}
	return report, nil
}

// Enrollment is explicit for each instrument; no instrument starts monitored.
func (s *Service) SetInvestmentReviewEnrollment(ctx context.Context, instrumentID string, enabled bool, expectedRevision int) (InvestmentReviewReport, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	defer unlock()
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	report, state, key, err := s.investmentReviews(ctx)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	valid := false
	for _, item := range report.Items {
		if item.InstrumentID == instrumentID {
			valid = true
		}
	}
	if !valid || expectedRevision != state.Revision {
		return InvestmentReviewReport{}, &domain.Error{Code: domain.ErrConflict, Message: "investment review settings changed; reload before saving"}
	}
	state.Enrolled[instrumentID] = enabled
	state.Revision++
	raw, err := json.Marshal(state)
	if err != nil {
		return InvestmentReviewReport{}, err
	}
	if err = s.reviewStore.SaveConfiguration(key, raw); err != nil {
		return InvestmentReviewReport{}, err
	}
	report, _, _, err = s.investmentReviews(ctx)
	return report, err
}
