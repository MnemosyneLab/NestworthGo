// Package history adapts internal/application.Service's History/Timeline,
// Starting Point, Record change, Undo/Fix, and daily-snapshot surface for
// the Wails IPC boundary. The change-command union (command.go) is the
// hardest single mapping in this migration (technical design Sec6).
package history

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

// HistoryOriginDTO mirrors domain.HistoryOrigin.
type HistoryOriginDTO struct {
	ID          string `json:"id"`
	HouseholdID string `json:"householdId"`
	Timezone    string `json:"timezone"`
	StartedAt   string `json:"startedAt"`
	CreatedAt   string `json:"createdAt"`
}

func fromHistoryOrigin(value domain.HistoryOrigin) HistoryOriginDTO {
	return HistoryOriginDTO{
		ID: value.ID.String(), HouseholdID: value.HouseholdID.String(), Timezone: value.Timezone,
		StartedAt: wire.FormatTime(value.StartedAt), CreatedAt: wire.FormatTime(value.CreatedAt),
	}
}

func (s *Service) HistoryOrigin(ctx context.Context) (*HistoryOriginDTO, error) {
	origin, err := s.app.HistoryOrigin(ctx)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	if origin == nil {
		return nil, nil
	}
	dto := fromHistoryOrigin(*origin)
	return &dto, nil
}

func (s *Service) HistoryStarted(ctx context.Context) (bool, error) {
	started, err := s.app.HistoryStarted(ctx)
	return started, apierror.Wrap(err)
}

func (s *Service) StartHistory(ctx context.Context, timezone string) (HistoryOriginDTO, error) {
	origin, err := s.app.StartHistory(ctx, timezone)
	if err != nil {
		return HistoryOriginDTO{}, apierror.Wrap(err)
	}
	return fromHistoryOrigin(origin), nil
}

// StartHistoryWithCosts's costOverrides map is keyed by domain.HoldingID in
// Go; JSON object keys must be strings, so the wire shape is a plain
// map[string]string keyed by the Holding ID string, parsed here.
func (s *Service) StartHistoryWithCosts(ctx context.Context, timezone string, costOverrides map[string]string) (HistoryOriginDTO, error) {
	parsed := make(map[domain.HoldingID]string, len(costOverrides))
	for id, cost := range costOverrides {
		holdingID, err := domain.ParseHoldingID(id)
		if err != nil {
			return HistoryOriginDTO{}, apierror.Wrap(err)
		}
		parsed[holdingID] = cost
	}
	origin, err := s.app.StartHistoryWithCosts(ctx, timezone, parsed)
	if err != nil {
		return HistoryOriginDTO{}, apierror.Wrap(err)
	}
	return fromHistoryOrigin(origin), nil
}

func (s *Service) StartingPointDraft(ctx context.Context) ([]wire.StartingPointHoldingDTO, error) {
	draft, err := s.app.StartingPointDraft(ctx)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromStartingPointHoldings(draft), nil
}

func (s *Service) HistoryMutationAllowed(ctx context.Context) error {
	return apierror.Wrap(s.app.HistoryMutationAllowed(ctx))
}

// resolveHouseholdID re-derives the current Household ID from Bootstrap
// rather than trusting a client-submitted value, matching the rule that a
// change command's HouseholdID always comes from server-side context
// (command.go's ToCommand doc comment).
func (s *Service) resolveHouseholdID(ctx context.Context) (domain.HouseholdID, error) {
	bootstrap, err := s.app.Bootstrap(ctx)
	if err != nil {
		return "", err
	}
	if bootstrap.Household == nil {
		return "", &domain.Error{Code: domain.ErrConflict, Message: "complete onboarding first"}
	}
	return bootstrap.Household.ID, nil
}

func (s *Service) PreviewChange(ctx context.Context, request ChangeCommandRequest) (wire.ChangePreviewDTO, error) {
	householdID, err := s.resolveHouseholdID(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := request.ToCommand(householdID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.PreviewChange(ctx, command)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	return wire.FromChangePreview(preview), nil
}

func (s *Service) RecordChange(ctx context.Context, request ChangeCommandRequest) (wire.ChangePreviewDTO, error) {
	householdID, err := s.resolveHouseholdID(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := request.ToCommand(householdID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.RecordChange(ctx, command)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	return wire.FromChangePreview(preview), nil
}

// CommitChange is kept distinct from RecordChange at the wire boundary even
// though application.Service.CommitChange is currently a thin alias for
// RecordChange, so the frontend's "preview, then confirm" UX (interaction
// brief Sec8.4) has a stable name to call regardless of how the Go layer
// evolves.
func (s *Service) CommitChange(ctx context.Context, request ChangeCommandRequest) (wire.ChangePreviewDTO, error) {
	householdID, err := s.resolveHouseholdID(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := request.ToCommand(householdID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.CommitChange(ctx, command)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	return wire.FromChangePreview(preview), nil
}

func (s *Service) UndoChange(ctx context.Context, activityID string) (wire.ChangePreviewDTO, error) {
	id, err := domain.ParseActivityID(activityID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.UndoChange(ctx, id)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	return wire.FromChangePreview(preview), nil
}

// PreviewFixChange is FixChange's read-only counterpart, giving the Fix
// form the same "preview, then confirm" UX every other change kind gets
// (technical design Sec6): it returns the replacement ChangePreview
// FixChange would commit, computed by inverting the original Activity's
// effects first, so the previewed number is not double-counted against
// the original Activity that Confirm will actually remove.
func (s *Service) PreviewFixChange(ctx context.Context, activityID string, replacement ChangeCommandRequest) (wire.ChangePreviewDTO, error) {
	id, err := domain.ParseActivityID(activityID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	householdID, err := s.resolveHouseholdID(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := replacement.ToCommand(householdID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.PreviewFixChange(ctx, id, command)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	return wire.FromChangePreview(preview), nil
}

func (s *Service) FixChange(ctx context.Context, activityID string, replacement ChangeCommandRequest) (wire.ChangePreviewDTO, error) {
	id, err := domain.ParseActivityID(activityID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	householdID, err := s.resolveHouseholdID(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := replacement.ToCommand(householdID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.FixChange(ctx, id, command)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	return wire.FromChangePreview(preview), nil
}

func (s *Service) ListActivities(ctx context.Context, limit int) ([]wire.ActivityDTO, error) {
	activities, err := s.app.ListActivities(ctx, limit)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromActivities(activities), nil
}

// ActivityQueryRequest mirrors domain.ActivityQuery.
type ActivityQueryRequest struct {
	AccountID        *string  `json:"accountId,omitempty"`
	Kinds            []string `json:"kinds,omitempty"`
	FromLocalDate    string   `json:"fromLocalDate,omitempty"`
	ToLocalDate      string   `json:"toLocalDate,omitempty"`
	AfterEffectiveAt string   `json:"afterEffectiveAt,omitempty"`
	AfterCreatedAt   string   `json:"afterCreatedAt,omitempty"`
	AfterID          string   `json:"afterId,omitempty"`
	Limit            int      `json:"limit,omitempty"`
}

func (r ActivityQueryRequest) toDomain() (domain.ActivityQuery, error) {
	query := domain.ActivityQuery{FromLocalDate: r.FromLocalDate, ToLocalDate: r.ToLocalDate, Limit: r.Limit}
	if r.AccountID != nil {
		id, err := domain.ParseAccountID(*r.AccountID)
		if err != nil {
			return domain.ActivityQuery{}, err
		}
		query.AccountID = &id
	}
	for _, kind := range r.Kinds {
		query.Kinds = append(query.Kinds, domain.ActivityKind(kind))
	}
	if r.AfterID != "" {
		id, err := domain.ParseActivityID(r.AfterID)
		if err != nil {
			return domain.ActivityQuery{}, err
		}
		effectiveAt, err := wire.ParseTime(r.AfterEffectiveAt)
		if err != nil {
			return domain.ActivityQuery{}, err
		}
		createdAt, err := wire.ParseTime(r.AfterCreatedAt)
		if err != nil {
			return domain.ActivityQuery{}, err
		}
		query.After = &domain.ActivityCursor{EffectiveAt: effectiveAt, CreatedAt: createdAt, ID: id}
	}
	return query, nil
}

func (s *Service) ListActivityPage(ctx context.Context, request ActivityQueryRequest) (wire.ActivityPageDTO, error) {
	query, err := request.toDomain()
	if err != nil {
		return wire.ActivityPageDTO{}, apierror.Wrap(err)
	}
	page, err := s.app.ListActivityPage(ctx, query)
	if err != nil {
		return wire.ActivityPageDTO{}, apierror.Wrap(err)
	}
	return wire.FromActivityPage(page), nil
}

// DailySnapshotStateDTO mirrors domain.DailySnapshotState.
type DailySnapshotStateDTO struct {
	HouseholdID           string  `json:"householdId"`
	DirtyFrom             *string `json:"dirtyFrom,omitempty"`
	LastCompletedClosedOn *string `json:"lastCompletedClosedOn,omitempty"`
}

func (s *Service) DailySnapshotState(ctx context.Context, householdID string) (DailySnapshotStateDTO, error) {
	id, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return DailySnapshotStateDTO{}, apierror.Wrap(err)
	}
	state, err := s.app.DailySnapshotState(ctx, id)
	if err != nil {
		return DailySnapshotStateDTO{}, apierror.Wrap(err)
	}
	return DailySnapshotStateDTO{HouseholdID: state.HouseholdID.String(), DirtyFrom: state.DirtyFrom, LastCompletedClosedOn: state.LastCompletedClosedOn}, nil
}

func (s *Service) CompleteDailySnapshotRange(ctx context.Context, householdID, targetDate string) error {
	id, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.CompleteDailySnapshotRange(ctx, id, targetDate))
}

// DailyValuationSnapshotDTO mirrors domain.DailyValuationSnapshot, minus its
// per-item list, which the frontend fetches separately if it needs
// component-level detail (list endpoints exist on internal/application but
// are not yet bound; add them here if a Phase 5 page needs them).
type DailyValuationSnapshotDTO struct {
	ID                string          `json:"id"`
	HouseholdID       string          `json:"householdId"`
	LocalDate         string          `json:"localDate"`
	CutoffAt          string          `json:"cutoffAt"`
	Revision          int             `json:"revision"`
	AssetsAmount      *wire.MoneyView `json:"assetsAmount,omitempty"`
	LiabilitiesAmount *wire.MoneyView `json:"liabilitiesAmount,omitempty"`
	NetWorthAmount    *wire.MoneyView `json:"netWorthAmount,omitempty"`
	Currency          string          `json:"currency"`
	Complete          bool            `json:"complete"`
	ComponentCount    int             `json:"componentCount"`
	MissingCount      int             `json:"missingCount"`
	GenerationReason  string          `json:"generationReason"`
	CreatedAt         string          `json:"createdAt"`
}

func fromDailyValuationSnapshot(value domain.DailyValuationSnapshot) DailyValuationSnapshotDTO {
	return DailyValuationSnapshotDTO{
		ID: value.ID.String(), HouseholdID: value.HouseholdID.String(), LocalDate: value.LocalDate,
		CutoffAt: wire.FormatTime(value.CutoffAt), Revision: value.Revision,
		AssetsAmount: wire.FromMoneyPtr(value.AssetsAmount), LiabilitiesAmount: wire.FromMoneyPtr(value.LiabilitiesAmount),
		NetWorthAmount: wire.FromMoneyPtr(value.NetWorthAmount), Currency: value.Currency.String(), Complete: value.Complete,
		ComponentCount: value.ComponentCount, MissingCount: value.MissingCount, GenerationReason: value.GenerationReason,
		CreatedAt: wire.FormatTime(value.CreatedAt),
	}
}

func (s *Service) BuildDailyValuationSnapshot(ctx context.Context, localDate string) (DailyValuationSnapshotDTO, bool, error) {
	snapshot, created, err := s.app.BuildDailyValuationSnapshot(ctx, localDate)
	if err != nil {
		return DailyValuationSnapshotDTO{}, false, apierror.Wrap(err)
	}
	return fromDailyValuationSnapshot(snapshot), created, nil
}

func (s *Service) RebuildHistoricalSnapshots(ctx context.Context, startDate, endDate string) (int, error) {
	count, err := s.app.RebuildHistoricalSnapshots(ctx, startDate, endDate)
	return count, apierror.Wrap(err)
}
