package application

import (
	"slices"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// fundingCashAfterProductStart is the stable analytics sentence mapped by the
// frontend reason table for a product recorded before the cash that funds it.
const fundingCashAfterProductStart = "funding cash is recorded after the product start"

// historicalFundingGapReason is the historical-overview missing code for the
// same gap. Totals stay nil; the known subtotal is kept.
const historicalFundingGapReason = "funding_cash_after_product_start"

func annotateReplayFundingGaps(items []domain.DailyValuationSnapshotItem, gaps []domain.ReplayFundingGap) ([]domain.DailyValuationSnapshotItem, int) {
	extraMissing := 0
	for _, gap := range gaps {
		reason := fundingCashAfterProductStart
		for index := range items {
			item := &items[index]
			if item.HoldingID == nil || item.AccountID != gap.AccountID || !item.Complete || !slices.Contains(gap.HoldingIDs, *item.HoldingID) {
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

func markDisplayIncomplete(value *domain.ValuationAmount) {
	if value == nil {
		return
	}
	value.Total = nil
	quality := &value.Quality
	if quality.Availability == "" || quality.Availability == domain.ValuationFull {
		quality.Availability = domain.ValuationPartial
		if quality.Coverage == "" || quality.Coverage == domain.CoverageVerified {
			quality.Coverage = domain.CoverageProvisional
		}
	}
	quality.IncludedCount++
	quality.UnpricedCount++
	for _, reason := range quality.Reasons {
		if reason == domain.ReasonAccountValueMissing {
			return
		}
	}
	quality.Reasons = append(quality.Reasons, domain.ReasonAccountValueMissing)
}

// applyOverviewFundingGaps turns an omitted cash sleeve into an incomplete
// leaf. Household and account totals stay unpublished; known component amounts
// remain available as a subtotal.
func applyOverviewFundingGaps(portfolio domain.PortfolioSnapshot, accounts map[domain.AccountID]domain.AccountRecord, rows *[]HistoricalOverviewRow, assets, debts *displayAccumulator, assetComplete, debtComplete *bool) {
	for _, gap := range portfolio.ReplayFundingGaps {
		record, ok := accounts[gap.AccountID]
		if !ok {
			continue
		}
		account := record.Account
		parentKey := account.ID.String()
		parentIndex := -1
		for index, row := range *rows {
			if row.Kind == "account" && row.Key == parentKey {
				parentIndex = index
				break
			}
		}
		if parentIndex < 0 || (*rows)[parentIndex].Left == nil {
			continue
		}
		currency := gap.Currency.String()
		childKey := parentKey + ":cash:" + currency
		kind := "cash"
		if account.TrackingMode != domain.TrackingHoldings {
			childKey = parentKey + ":balance"
			kind = "balance"
		}
		childIndex := -1
		for index, row := range *rows {
			if row.Key == childKey {
				childIndex = index
				break
			}
		}
		reason := historicalFundingGapReason
		if childIndex < 0 {
			included := domain.AccountEligibleForNetWorth(account) && account.ArchivedAt == nil
			cell := HistoricalOverviewCell{
				Status: "unknown", Included: included, Complete: false, Currency: currency,
				AssetClass: string(domain.BucketCash), Missing: []string{reason},
				MissingDetails: []HistoricalMissingDetail{{Kind: reason, QuoteCurrency: currency}},
			}
			*rows = append(*rows, HistoricalOverviewRow{
				Key: childKey, AccountID: parentKey, ParentKey: parentKey, Kind: kind,
				Name: currency, Role: string(account.BalanceSheetRole), Left: &cell,
			})
			if included {
				if account.IsLiability() {
					debts.omitUnknown()
				} else {
					assets.omitUnknown()
				}
			}
		} else if cell := (*rows)[childIndex].Left; cell != nil {
			cell.Complete = false
			cell.Status = "unknown"
			cell.NativeAmount = nil
			cell.BaseAmount = nil
			cell.Missing = appendHistoricalReason(cell.Missing, reason)
			cell.MissingDetails = append(cell.MissingDetails, HistoricalMissingDetail{Kind: reason, QuoteCurrency: currency})
			markDisplayIncomplete(&cell.DisplayValuation)
		}
		parent := (*rows)[parentIndex].Left
		parent.Complete = false
		parent.BaseAmount = nil
		parent.Missing = appendHistoricalReason(parent.Missing, reason)
		parent.MissingDetails = append(parent.MissingDetails, HistoricalMissingDetail{Kind: reason, QuoteCurrency: currency})
		markDisplayIncomplete(&parent.DisplayValuation)
		if domain.AccountEligibleForNetWorth(account) && account.ArchivedAt == nil {
			if account.IsLiability() {
				*debtComplete = false
			} else {
				*assetComplete = false
			}
		}
	}
}

// applySeriesFundingGaps records the same omitted sleeve on a trend point.
// Review projections do not read these cash sleeves, so a gap must not turn a
// review quantity into zero.
func applySeriesFundingGaps(point *HistoricalSeriesPoint, portfolio domain.PortfolioSnapshot, assets, debts *displayAccumulator) {
	accounts := make(map[domain.AccountID]domain.Account, len(portfolio.Accounts))
	for _, record := range portfolio.Accounts {
		accounts[record.Account.ID] = record.Account
	}
	for _, gap := range portfolio.ReplayFundingGaps {
		account, ok := accounts[gap.AccountID]
		if !ok {
			continue
		}
		currency := gap.Currency.String()
		point.Missing = append(point.Missing, HistoricalSeriesMissing{
			Key: account.ID.String() + ":cash:" + currency, Name: account.Name + " · " + currency,
			Role: string(account.BalanceSheetRole), Reasons: []string{historicalFundingGapReason},
		})
		if !domain.AccountEligibleForNetWorth(account) || account.ArchivedAt != nil {
			continue
		}
		coverage := &point.AssetCoverage
		target := assets
		if account.IsLiability() {
			coverage = &point.LiabilityCoverage
			target = debts
		}
		coverage.Included++
		coverage.Missing++
		target.omitUnknown()
	}
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
