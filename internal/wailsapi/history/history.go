// Package history adapts internal/application.Service's History/Timeline,
// Starting Point, Record change, Undo/Fix, and daily-snapshot surface for
// the Wails IPC boundary. The change-command union (command.go) is the
// typed boundary between frontend requests and domain commands.
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

// resolveHistoryContext re-derives both the current Household ID and the
// immutable Origin timezone from server-side state. The frontend may submit a
// local wall-clock pair, but it never gets to choose the timezone used to turn
// that pair into an instant.
func (s *Service) resolveHistoryContext(ctx context.Context) (domain.HouseholdID, string, error) {
	origin, err := s.app.HistoryOrigin(ctx)
	if err != nil {
		return "", "", err
	}
	if origin == nil {
		return "", "", &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before recording a change"}
	}
	household, err := s.app.Household(ctx)
	if err != nil {
		return "", "", err
	}
	return household.ID, origin.Timezone, nil
}

// resolveHouseholdID is used by the daily-snapshot maintenance endpoints,
// which do not accept local change timestamps and therefore do not need the
// Origin timezone.
func (s *Service) resolveHouseholdID(ctx context.Context) (domain.HouseholdID, error) {
	household, err := s.app.Household(ctx)
	if err != nil {
		return "", err
	}
	return household.ID, nil
}

func (s *Service) PreviewChange(ctx context.Context, request ChangeCommandRequest) (wire.ChangePreviewDTO, error) {
	householdID, originTimezone, err := s.resolveHistoryContext(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := request.ToCommand(householdID, originTimezone)
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
	householdID, originTimezone, err := s.resolveHistoryContext(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := request.ToCommand(householdID, originTimezone)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.RecordChange(ctx, command)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	return wire.FromChangePreview(preview), nil
}

// CommitChange is kept as a wire alias of RecordChange so the frontend's
// "preview, then confirm" UX has a stable name to call. Application code
// has a single RecordChange write path.
func (s *Service) CommitChange(ctx context.Context, request ChangeCommandRequest) (wire.ChangePreviewDTO, error) {
	householdID, originTimezone, err := s.resolveHistoryContext(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := request.ToCommand(householdID, originTimezone)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.RecordChange(ctx, command)
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
// form the same "preview, then confirm" UX every other change kind gets:
// it returns the replacement ChangePreview
// FixChange would commit, computed by inverting the original Activity's
// effects first, so the previewed number is not double-counted against
// the original Activity that Confirm will actually remove.
func (s *Service) PreviewFixChange(ctx context.Context, activityID string, replacement ChangeCommandRequest) (wire.ChangePreviewDTO, error) {
	id, err := domain.ParseActivityID(activityID)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	householdID, originTimezone, err := s.resolveHistoryContext(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := replacement.ToCommand(householdID, originTimezone)
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
	householdID, originTimezone, err := s.resolveHistoryContext(ctx)
	if err != nil {
		return wire.ChangePreviewDTO{}, apierror.Wrap(err)
	}
	command, err := replacement.ToCommand(householdID, originTimezone)
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

func (s *Service) Activity(ctx context.Context, activityID string) (wire.ActivityDTO, error) {
	id, err := domain.ParseActivityID(activityID)
	if err != nil {
		return wire.ActivityDTO{}, apierror.Wrap(err)
	}
	activity, err := s.app.Activity(ctx, id)
	if err != nil {
		return wire.ActivityDTO{}, apierror.Wrap(err)
	}
	return wire.FromActivity(activity), nil
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
		parsed, err := domain.ParseActivityKind(kind)
		if err != nil {
			return domain.ActivityQuery{}, err
		}
		query.Kinds = append(query.Kinds, parsed)
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

func (s *Service) DailySnapshotState(ctx context.Context, _ string) (DailySnapshotStateDTO, error) {
	id, err := s.resolveHouseholdID(ctx)
	if err != nil {
		return DailySnapshotStateDTO{}, apierror.Wrap(err)
	}
	state, err := s.app.DailySnapshotState(ctx, id)
	if err != nil {
		return DailySnapshotStateDTO{}, apierror.Wrap(err)
	}
	return DailySnapshotStateDTO{HouseholdID: state.HouseholdID.String(), DirtyFrom: state.DirtyFrom, LastCompletedClosedOn: state.LastCompletedClosedOn}, nil
}

func (s *Service) CompleteDailySnapshotRange(ctx context.Context, _, targetDate string) error {
	id, err := s.resolveHouseholdID(ctx)
	if err != nil {
		return apierror.Wrap(err)
	}
	return apierror.Wrap(s.app.CompleteDailySnapshotRange(ctx, id, targetDate))
}

// DailyValuationSnapshotItemDTO is one reconstructed component. Simple
// Account items carry classificationBasis=current-metadata-derived because
// the bucket name comes from current account_type, not a historical type
// observation. Composite cash/instrument items omit that field.
type DailyValuationSnapshotItemDTO struct {
	ID                  string          `json:"id"`
	AccountID           string          `json:"accountId"`
	HoldingID           *string         `json:"holdingId,omitempty"`
	InstrumentID        *string         `json:"instrumentId,omitempty"`
	NativeAmount        string          `json:"nativeAmount,omitempty"`
	NativeCurrency      string          `json:"nativeCurrency,omitempty"`
	BaseAmount          *wire.MoneyView `json:"baseAmount,omitempty"`
	Complete            bool            `json:"complete"`
	MissingReason       *string         `json:"missingReason,omitempty"`
	ClassificationBasis string          `json:"classificationBasis,omitempty"`
}

// DailyValuationSnapshotDTO mirrors domain.DailyValuationSnapshot totals
// plus the per-item results that carry Simple classification provenance.
type DailyValuationSnapshotDTO struct {
	ID                string                          `json:"id"`
	HouseholdID       string                          `json:"householdId"`
	LocalDate         string                          `json:"localDate"`
	CutoffAt          string                          `json:"cutoffAt"`
	Revision          int                             `json:"revision"`
	AssetsAmount      *wire.MoneyView                 `json:"assetsAmount,omitempty"`
	LiabilitiesAmount *wire.MoneyView                 `json:"liabilitiesAmount,omitempty"`
	NetWorthAmount    *wire.MoneyView                 `json:"netWorthAmount,omitempty"`
	Currency          string                          `json:"currency"`
	Complete          bool                            `json:"complete"`
	ComponentCount    int                             `json:"componentCount"`
	MissingCount      int                             `json:"missingCount"`
	GenerationReason  string                          `json:"generationReason"`
	CreatedAt         string                          `json:"createdAt"`
	Items             []DailyValuationSnapshotItemDTO `json:"items"`
}

func fromDailyValuationSnapshotItem(item domain.DailyValuationSnapshotItem) DailyValuationSnapshotItemDTO {
	dto := DailyValuationSnapshotItemDTO{
		ID: item.ID.String(), AccountID: item.AccountID.String(), NativeAmount: item.NativeAmount,
		Complete: item.Complete, MissingReason: item.MissingReason, ClassificationBasis: item.ClassificationBasis.String(),
		BaseAmount: wire.FromMoneyPtr(item.BaseAmount),
	}
	if item.NativeCurrency != "" {
		dto.NativeCurrency = item.NativeCurrency.String()
	}
	if item.HoldingID != nil {
		id := item.HoldingID.String()
		dto.HoldingID = &id
	}
	if item.InstrumentID != nil {
		id := item.InstrumentID.String()
		dto.InstrumentID = &id
	}
	return dto
}

func fromDailyValuationSnapshot(value domain.DailyValuationSnapshot) DailyValuationSnapshotDTO {
	items := make([]DailyValuationSnapshotItemDTO, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, fromDailyValuationSnapshotItem(item))
	}
	return DailyValuationSnapshotDTO{
		ID: value.ID.String(), HouseholdID: value.HouseholdID.String(), LocalDate: value.LocalDate,
		CutoffAt: wire.FormatTime(value.CutoffAt), Revision: value.Revision,
		AssetsAmount: wire.FromMoneyPtr(value.AssetsAmount), LiabilitiesAmount: wire.FromMoneyPtr(value.LiabilitiesAmount),
		NetWorthAmount: wire.FromMoneyPtr(value.NetWorthAmount), Currency: value.Currency.String(), Complete: value.Complete,
		ComponentCount: value.ComponentCount, MissingCount: value.MissingCount, GenerationReason: value.GenerationReason,
		CreatedAt: wire.FormatTime(value.CreatedAt), Items: items,
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
