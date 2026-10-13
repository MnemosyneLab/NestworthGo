package liquidity

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
	"time"
)

type UpcomingActionsDTO struct {
	SchemaVersion string                 `json:"schemaVersion"`
	AsOf          string                 `json:"asOf"`
	LocalDate     string                 `json:"localDate"`
	Timezone      string                 `json:"timezone"`
	WindowStartOn string                 `json:"windowStartOn"`
	WindowEndOn   string                 `json:"windowEndOn"`
	BaseCurrency  string                 `json:"baseCurrency"`
	Completeness  string                 `json:"completeness"`
	Counts        UpcomingCountsDTO      `json:"counts"`
	Items         []UpcomingActionDTO    `json:"items"`
	DataChecks    []UpcomingDataCheckDTO `json:"dataChecks"`
	ScopeWarnings []string               `json:"scopeWarnings"`
}
type UpcomingCountsDTO struct {
	Total             int `json:"total"`
	Attention         int `json:"attention"`
	ReceiptDue        int `json:"receiptDue"`
	ReservationIssues int `json:"reservationIssues"`
	Scheduled         int `json:"scheduled"`
	Eligible          int `json:"eligible"`
	DataChecks        int `json:"dataChecks"`
	ScopeNotes        int `json:"scopeNotes"`
}
type UpcomingTargetDTO struct {
	Kind          string        `json:"kind"`
	ProductID     *string       `json:"productId"`
	SourceRef     *SourceRefDTO `json:"sourceRef"`
	ReservationID *string       `json:"reservationId"`
}
type UpcomingActionDTO struct {
	ID                 string              `json:"id"`
	OccurrenceKey      string              `json:"occurrenceKey"`
	Kind               string              `json:"kind"`
	Phase              string              `json:"phase"`
	Group              string              `json:"group"`
	Priority           int                 `json:"priority"`
	SortOn             *string             `json:"sortOn"`
	AccountID          string              `json:"accountId"`
	AccountName        *string             `json:"accountName"`
	ProductID          *string             `json:"productId"`
	ProductKind        *string             `json:"productKind"`
	SourceKey          *string             `json:"sourceKey"`
	SourceRef          *SourceRefDTO       `json:"sourceRef"`
	ReservationIDs     []string            `json:"reservationIds"`
	ReservationLabels  []string            `json:"reservationLabels"`
	DisplayName        string              `json:"displayName"`
	AnchorKind         string              `json:"anchorKind"`
	AnchorOn           *string             `json:"anchorOn"`
	MaturityOn         *string             `json:"maturityOn"`
	EligibilityOn      *string             `json:"eligibilityOn"`
	EstimatedReceiptOn *string             `json:"estimatedReceiptOn"`
	ReceiptBasis       string              `json:"receiptBasis"`
	ReferenceOn        *string             `json:"referenceOn"`
	Amount             *wire.MoneyView     `json:"amount"`
	AmountRole         string              `json:"amountRole"`
	RequestedReserve   *wire.MoneyView     `json:"requestedReserve"`
	AmountStatus       string              `json:"amountStatus"`
	CoverageStatus     string              `json:"coverageStatus"`
	ReasonCodes        []string            `json:"reasonCodes"`
	Assumptions        []string            `json:"assumptions"`
	PrimaryTarget      UpcomingTargetDTO   `json:"primaryTarget"`
	Targets            []UpcomingTargetDTO `json:"targets"`
	DaysFromToday      *int                `json:"daysFromToday"`
	PolicyRevision     int                 `json:"policyRevision"`
}
type UpcomingDataCheckDTO struct {
	ID              string              `json:"id"`
	SourceKey       string              `json:"sourceKey"`
	Category        string              `json:"category"`
	ProductID       *string             `json:"productId"`
	DisplayName     string              `json:"displayName"`
	ReasonCodes     []string            `json:"reasonCodes"`
	RelatedActionID *string             `json:"relatedActionId"`
	AnchorOn        *string             `json:"anchorOn"`
	CoverageStatus  string              `json:"coverageStatus"`
	Target          UpcomingTargetDTO   `json:"target"`
	Targets         []UpcomingTargetDTO `json:"targets"`
	ReferenceOn     *string             `json:"referenceOn"`
}

func (s *Service) UpcomingActions(ctx context.Context) (UpcomingActionsDTO, error) {
	result, err := s.app.UpcomingActions(ctx)
	if err != nil {
		return UpcomingActionsDTO{}, apierror.Wrap(err)
	}
	return fromUpcomingActions(result), nil
}
func fromUpcomingTarget(t domain.UpcomingTarget) UpcomingTargetDTO {
	out := UpcomingTargetDTO{Kind: t.Kind}
	if t.ProductID != nil {
		id := t.ProductID.String()
		out.ProductID = &id
	}
	if t.ReservationID != nil {
		id := t.ReservationID.String()
		out.ReservationID = &id
	}
	if t.SourceRef != nil {
		ref := fromSourceRef(*t.SourceRef)
		out.SourceRef = &ref
	}
	return out
}
func upcomingArray(v []string) []string { return append([]string{}, v...) }
func upcomingTargets(v []domain.UpcomingTarget) []UpcomingTargetDTO {
	out := make([]UpcomingTargetDTO, 0, len(v))
	for _, target := range v {
		out = append(out, fromUpcomingTarget(target))
	}
	return out
}
func fromUpcomingActions(v domain.UpcomingActions) UpcomingActionsDTO {
	c := v.Counts
	out := UpcomingActionsDTO{SchemaVersion: "upcoming-actions/1", AsOf: v.AsOf.UTC().Format(time.RFC3339Nano), LocalDate: v.LocalDate, Timezone: v.Timezone, WindowStartOn: v.WindowStartOn, WindowEndOn: v.WindowEndOn, BaseCurrency: v.BaseCurrency.String(), Completeness: string(v.Completeness),
		Counts: UpcomingCountsDTO{Total: c.Total, Attention: c.Attention, ReceiptDue: c.ReceiptDue, ReservationIssues: c.ReservationIssues, Scheduled: c.Scheduled, Eligible: c.Eligible, DataChecks: c.DataChecks, ScopeNotes: c.ScopeNotes}, Items: []UpcomingActionDTO{}, DataChecks: []UpcomingDataCheckDTO{}, ScopeWarnings: upcomingArray(v.ScopeWarnings)}
	for _, a := range v.Items {
		key := a.SourceKey
		item := UpcomingActionDTO{ID: a.ID, OccurrenceKey: a.OccurrenceKey, Kind: a.Kind, Phase: a.Phase, Group: a.Group, Priority: a.Priority, SortOn: a.SortOn, AccountID: a.AccountID.String(), AccountName: a.AccountName, SourceKey: &key,
			ReservationIDs: upcomingArray(a.ReservationIDs), ReservationLabels: upcomingArray(a.ReservationLabels), DisplayName: a.DisplayName, AnchorKind: a.AnchorKind, AnchorOn: a.AnchorOn, MaturityOn: a.MaturityOn, EligibilityOn: a.EligibilityOn, EstimatedReceiptOn: a.EstimatedReceiptOn, ReceiptBasis: a.ReceiptBasis, ReferenceOn: a.ReferenceOn,
			Amount: wire.FromMoneyPtr(a.Amount), RequestedReserve: wire.FromMoneyPtr(a.RequestedReserve), AmountRole: a.AmountRole, AmountStatus: a.AmountStatus, CoverageStatus: a.CoverageStatus, ReasonCodes: upcomingArray(a.ReasonCodes), Assumptions: upcomingArray(a.Assumptions), PrimaryTarget: fromUpcomingTarget(a.PrimaryTarget), Targets: upcomingTargets(a.Targets), DaysFromToday: a.DaysFromToday, PolicyRevision: a.PolicyRevision}
		if a.SourceRef != nil {
			ref := fromSourceRef(*a.SourceRef)
			item.SourceRef = &ref
		}
		if a.ProductID != nil {
			id := a.ProductID.String()
			item.ProductID = &id
		}
		if a.ProductKind != nil {
			kind := string(*a.ProductKind)
			item.ProductKind = &kind
		}
		out.Items = append(out.Items, item)
	}
	for _, c := range v.DataChecks {
		item := UpcomingDataCheckDTO{ID: c.ID, SourceKey: c.SourceKey, Category: c.Category, DisplayName: c.DisplayName, ReasonCodes: upcomingArray(c.ReasonCodes), RelatedActionID: c.RelatedActionID, AnchorOn: c.AnchorOn, CoverageStatus: c.CoverageStatus, ReferenceOn: c.ReferenceOn, Target: fromUpcomingTarget(c.Target), Targets: upcomingTargets(c.Targets)}
		if c.ProductID != nil {
			id := c.ProductID.String()
			item.ProductID = &id
		}
		out.DataChecks = append(out.DataChecks, item)
	}
	return out
}

// FromUpcomingActions adapts an already frozen projection without another read.
func FromUpcomingActions(value domain.UpcomingActions) UpcomingActionsDTO {
	return fromUpcomingActions(value)
}
