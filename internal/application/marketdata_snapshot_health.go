package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// incompleteSnapshotHealth reads both stored results and current local inputs.
// A completed build cursor says nothing about the completeness of its values.
func (s *Service) incompleteSnapshotHealth(ctx context.Context, origin *domain.HistoryOrigin, plan HistoryRepairPlan, names map[domain.InstrumentID]domain.Instrument) ([]HealthIssue, error) {
	until, err := time.Parse("2006-01-02", plan.YesterdayLocal)
	if err != nil {
		return nil, err
	}
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, origin.HouseholdID, time.Time{}, until)
	if err != nil {
		return nil, err
	}
	needs := map[domain.InstrumentID]InstrumentRepairNeed{}
	for _, need := range plan.Instruments {
		needs[need.InstrumentID] = need
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return nil, err
	}
	var issues []HealthIssue
	for _, snapshot := range snapshots {
		if snapshot.Complete || snapshot.LocalDate < plan.OriginLocalDate {
			continue
		}
		fresh, err := s.valueHistoricalSnapshot(ctx, origin, snapshot.CutoffAt, snapshot.LocalDate)
		if err != nil {
			return nil, err
		}
		if fresh.Complete {
			issues = append(issues, HealthIssue{ID: "snapshot-incomplete-" + snapshot.LocalDate, Kind: HealthKindSnapshotOutdated, Severity: HealthSeverityBlocking, TargetKey: "snapshot", GroupKey: "snapshot", RangeStart: snapshot.LocalDate, RangeEnd: snapshot.LocalDate, RangeCount: 1, Code: "snapshot_outdated", Reason: "snapshot_inputs_ready", Action: HealthActionRepair, Executable: true})
			continue
		}
		before := len(issues)
		for index, item := range fresh.Items {
			if item.Complete {
				continue
			}
			issue := HealthIssue{ID: fmt.Sprintf("snapshot-incomplete-%s-%d", snapshot.LocalDate, index), Kind: HealthKindSnapshotIncomplete, Severity: HealthSeverityWarning, TargetKey: "snapshot", GroupKey: "snapshot", AccountID: item.AccountID.String(), RangeStart: snapshot.LocalDate, RangeEnd: snapshot.LocalDate, RangeCount: 1, Code: "snapshot_incomplete", Reason: "snapshot_incomplete", Action: HealthActionNone}
			if item.InstrumentID != nil {
				id := *item.InstrumentID
				issue.InstrumentID = id.String()
				issue.TargetKey = instrumentTargetKey(id)
				issue.GroupKey = issue.TargetKey
				instrument := names[id]
				issue.Label = instrument.Name
				need := needs[id]
				issue.Provider = need.ProviderKey
				if item.MissingReason != nil && strings.Contains(*item.MissingReason, "coverage") && domain.UsesMetalFuturesHistory(need.InstrumentType, need.Market) && snapshot.LocalDate > need.LastFinalizedMarketDate && need.LastFinalizedMarketDate != "" {
					issue.Kind = HealthKindHistoryPending
					issue.Code = "daily_reference_pending"
					issue.Reason = "daily_reference_pending"
					eligible, err := domain.MetalDailyBarEligibleAt(snapshot.LocalDate)
					if err != nil {
						return nil, err
					}
					// Finalization advances on the next calendar day, after the final millisecond.
					issue.NextCheckAt = eligible.Add(time.Millisecond).In(location).Format(time.RFC3339)
				}
			}
			issues = append(issues, issue)
		}
		if len(issues) == before {
			issues = append(issues, HealthIssue{ID: "snapshot-incomplete-" + snapshot.LocalDate, Kind: HealthKindSnapshotIncomplete, Severity: HealthSeverityWarning, TargetKey: "snapshot", GroupKey: "snapshot", RangeStart: snapshot.LocalDate, RangeEnd: snapshot.LocalDate, RangeCount: 1, Code: "snapshot_incomplete", Reason: "snapshot_incomplete", Action: HealthActionNone})
		}
	}
	return issues, nil
}

func readySnapshotDates(issues []HealthIssue) []string {
	seen := map[string]bool{}
	var dates []string
	for _, issue := range issues {
		if issue.Reason == "snapshot_inputs_ready" && !seen[issue.RangeStart] {
			seen[issue.RangeStart] = true
			dates = append(dates, issue.RangeStart)
		}
	}
	return dates
}
