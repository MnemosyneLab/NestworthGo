package application

import "github.com/waltwang/nestworth-go/internal/domain"

// fundingCashAfterProductStart is the stable analytics sentence mapped by the
// frontend reason table for a product recorded before the cash that funds it.
const fundingCashAfterProductStart = "funding cash is recorded after the product start"

func annotateReplayFundingGaps(items []domain.DailyValuationSnapshotItem, gaps []domain.ReplayFundingGap) ([]domain.DailyValuationSnapshotItem, int) {
	extraMissing := 0
	for _, gap := range gaps {
		reason := fundingCashAfterProductStart
		for index := range items {
			item := &items[index]
			if item.HoldingID == nil || item.AccountID != gap.AccountID || !item.Complete {
				continue
			}
			if item.NativeCurrency != "" && item.NativeCurrency != gap.Currency {
				continue
			}
			item.MissingReason = &reason
		}
		if fundingGapCashItemExists(items, gap) {
			continue
		}
		copied := reason
		items = append(items, domain.DailyValuationSnapshotItem{
			ID:             domain.NewDailyValuationSnapshotItemID(),
			AccountID:      gap.AccountID,
			NativeCurrency: gap.Currency,
			Complete:       false,
			MissingReason:  &copied,
		})
		extraMissing++
	}
	return items, extraMissing
}

func fundingGapCashItemExists(items []domain.DailyValuationSnapshotItem, gap domain.ReplayFundingGap) bool {
	for _, item := range items {
		if item.HoldingID != nil || item.AccountID != gap.AccountID {
			continue
		}
		if item.NativeCurrency == gap.Currency {
			return true
		}
	}
	return false
}

func fundingGapItemReason(items ...domain.DailyValuationSnapshotItem) string {
	for _, item := range items {
		if item.MissingReason != nil && *item.MissingReason == fundingCashAfterProductStart {
			return fundingCashAfterProductStart
		}
	}
	return ""
}

func fundingGapReasonFromDays(days []domain.ComponentDay) string {
	for _, day := range days {
		if day.MissingReason == fundingCashAfterProductStart {
			return fundingCashAfterProductStart
		}
	}
	return ""
}

func fundingGapReasonOnDate(days []domain.ComponentDay, date domain.LocalDate) string {
	for _, day := range days {
		if day.Date == date && day.MissingReason == fundingCashAfterProductStart {
			return fundingCashAfterProductStart
		}
	}
	return ""
}

func fundingGapReasonForGroup(result domain.PeriodAnalysisResult, groupKey string, groupBy ContributionGroupBy) string {
	for _, day := range result.Days {
		if day.MissingReason != fundingCashAfterProductStart {
			continue
		}
		if returnGroupKey(day.Component, analysisGroupBy(groupBy)) == groupKey {
			return fundingCashAfterProductStart
		}
	}
	return ""
}

// snapshotFundingGapAnnotated reports a closed snapshot whose only incompleteness
// is a funding gap, so a known holding can still open from its activity.
func snapshotFundingGapAnnotated(snapshot domain.DailyValuationSnapshot) bool {
	if snapshot.Complete {
		return true
	}
	if snapshot.LocalDate == "" {
		return false
	}
	sawGap := false
	for _, item := range snapshot.Items {
		if item.MissingReason != nil && *item.MissingReason == fundingCashAfterProductStart {
			sawGap = true
		}
		if !item.Complete && (item.MissingReason == nil || *item.MissingReason != fundingCashAfterProductStart) {
			return false
		}
	}
	return sawGap
}

func applyFundingGapMark(day *domain.ComponentDay, previous, current domain.DailyValuationSnapshotItem) {
	reason := fundingGapItemReason(previous, current)
	if reason == "" {
		return
	}
	day.MissingReason = reason
	if day.Status == domain.CompletenessOK {
		day.Status = domain.CompletenessPartial
	}
}
