package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historicalQuoteCacheKey struct{}
type historicalSnapshotBatchKey struct{}

type historicalQuoteCache struct {
	instrumentQuotes map[domain.InstrumentID][]domain.InstrumentQuote
	fxQuotes         []domain.FXQuote
}

// BuildDailyValuationSnapshot reconstructs a closed local day from the
// Starting point plus immutable Activities, then evaluates it with only quote
// observations at or before that day's cutoff.
func (s *Service) BuildDailyValuationSnapshot(ctx context.Context, localDate string) (domain.DailyValuationSnapshot, bool, error) {
	var origin *domain.HistoryOrigin
	var err error
	if batch, ok := ctx.Value(historicalSnapshotBatchKey{}).(*domain.HistoricalSnapshotBatch); ok && batch != nil {
		origin = &batch.Origin
	} else {
		origin, err = s.HistoryOrigin(ctx)
		if err != nil {
			return domain.DailyValuationSnapshot{}, false, err
		}
		if origin == nil {
			return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before building a snapshot"}
		}
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	parsedDate, err := time.ParseInLocation("2006-01-02", localDate, location)
	if err != nil || parsedDate.Format("2006-01-02") != localDate {
		return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrValidation, Field: "localDate", Message: "local date must use YYYY-MM-DD"}
	}
	if localDate >= s.clock().In(location).Format("2006-01-02") {
		return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "localDate", Message: "today is not a closed day"}
	}
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	if localDate < originDate {
		return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "localDate", Message: "snapshot date precedes the Starting point"}
	}
	nextLocal := parsedDate.AddDate(0, 0, 1).Format("2006-01-02")
	nextMidnight, err := domain.ResolveLocalDateTime(nextLocal, "00:00", origin.Timezone)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	cutoff := nextMidnight.Add(-time.Millisecond)
	portfolio, err := s.historicalPortfolioSnapshot(ctx, origin, cutoff)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	// PortfolioSnapshot intentionally reports investment totals only. For a
	// daily balance-sheet snapshot, value every account and sign liabilities.
	valuation := NewValuationService(s.repository, func() time.Time { return cutoff })
	valuation.SetFXProviderKey(s.FXProviderKey)
	valuedAccounts, missing, err := valuation.ValueAccounts(portfolio)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	assets, liabilities := decimal.Zero, decimal.Zero
	complete := true
	items := make([]domain.DailyValuationSnapshotItem, 0)
	eligibleMissing := 0
	baseCurrency := portfolio.Household.BaseCurrency
	for _, account := range valuedAccounts {
		if !domain.AccountEligibleForNetWorth(account.Account) {
			continue
		}
		if !account.Complete {
			complete = false
		}
		eligibleMissing += len(account.MissingInputs)
		for _, component := range account.Components {
			item := domain.DailyValuationSnapshotItem{ID: domain.NewDailyValuationSnapshotItemID(), AccountID: account.Account.ID, HoldingID: component.HoldingID, NativeAmount: component.NativeAmount, NativeCurrency: component.NativeCurrency, Complete: component.Available, InstrumentID: component.InstrumentID, StateObservationID: component.StateObservationID, PreferenceObservationID: component.PreferenceObservationID, FXPreferenceObservationID: component.FXPreferenceObservationID}
			if err := item.ValidateNativeAmount(); err != nil {
				return domain.DailyValuationSnapshot{}, false, err
			}
			if component.PriceEvidence != nil && component.PriceEvidence.ObservationID != "" {
				quoteID := component.PriceEvidence.ObservationID
				item.QuoteID = &quoteID
			}
			if component.FXEvidence != nil && component.FXEvidence.ObservationID != "" {
				fxQuoteID := component.FXEvidence.ObservationID
				item.FXQuoteID = &fxQuoteID
			}
			if component.BaseAmountExact != "" {
				exact, parseErr := domain.ParseNativeAmount(component.BaseAmountExact)
				if parseErr != nil {
					return domain.DailyValuationSnapshot{}, false, parseErr
				}
				item.BaseAmountExact = exact
				if err := item.ValidateBaseAmountExact(); err != nil {
					return domain.DailyValuationSnapshot{}, false, err
				}
				amount, amountErr := decimal.NewFromString(exact)
				if amountErr != nil {
					return domain.DailyValuationSnapshot{}, false, &domain.Error{Code: domain.ErrIntegrity, Message: "historical base amount is invalid"}
				}
				rounded, roundErr := domain.NewMoney(amount, baseCurrency)
				if roundErr != nil {
					return domain.DailyValuationSnapshot{}, false, roundErr
				}
				item.BaseAmount = &rounded
				if component.Available {
					if account.Account.IsLiability() {
						liabilities = liabilities.Add(amount)
					} else {
						assets = assets.Add(amount)
					}
				}
			}
			if !component.Available {
				reason := missingReason(component, missing)
				item.MissingReason = &reason
			}
			if account.Account.TrackingMode != domain.TrackingHoldings {
				item.ClassificationBasis = domain.ClassificationCurrentMetadataDerived
			}
			items = append(items, item)
		}
	}
	assetsMoney, err := domain.NewMoney(assets, baseCurrency)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	liabilitiesMoney, err := domain.NewMoney(liabilities, baseCurrency)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	netWorthMoney, err := domain.NewSignedMoney(assets.Sub(liabilities), baseCurrency)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	hash := snapshotContentHash(localDate, cutoff, assetsMoney, liabilitiesMoney, netWorthMoney, items)
	snapshot := domain.DailyValuationSnapshot{ID: domain.NewDailyValuationSnapshotID(), HouseholdID: portfolio.Household.ID, LocalDate: localDate, CutoffAt: cutoff, ContentHash: hash, AssetsAmount: &assetsMoney, LiabilitiesAmount: &liabilitiesMoney, NetWorthAmount: &netWorthMoney, Currency: baseCurrency, Complete: complete && eligibleMissing == 0, ComponentCount: len(items), MissingCount: eligibleMissing, GenerationReason: "manual", CreatedAt: s.clock(), Items: items}
	var appended bool
	ctx, unlock, err := s.beginWrite(ctx)
	if err != nil {
		return domain.DailyValuationSnapshot{}, false, err
	}
	appended, err = s.repository.SaveDailyValuationSnapshotAndMarkCompleted(ctx, snapshot, s.clock())
	unlock()
	if err == nil && appended {
		s.invalidateAnalysis()
	}
	return snapshot, appended, err
}

func (s *Service) historicalPortfolioSnapshot(ctx context.Context, origin *domain.HistoryOrigin, cutoff time.Time) (domain.PortfolioSnapshot, error) {
	var batch *domain.HistoricalSnapshotBatch
	if value, ok := ctx.Value(historicalSnapshotBatchKey{}).(*domain.HistoricalSnapshotBatch); ok {
		batch = value
	}
	quotes, _ := ctx.Value(historicalQuoteCacheKey{}).(*historicalQuoteCache)
	return HistoricalReplay{repository: s.repository, batch: batch, quotes: quotes}.Snapshot(ctx, origin, cutoff)
}

const snapshotContentHashVersion = "v2"

func snapshotContentHash(localDate string, cutoff time.Time, assets, liabilities domain.Money, netWorth domain.SignedMoney, items []domain.DailyValuationSnapshotItem) string {
	sort.Slice(items, func(i, j int) bool {
		return snapshotItemSortKey(items[i]) < snapshotItemSortKey(items[j])
	})
	hash := sha256.New()
	fmt.Fprintf(hash, "%s|%s|%s|%s|%s|%s", localDate, cutoff.UTC().Format(time.RFC3339Nano), assets.CanonicalAmount(), liabilities.CanonicalAmount(), netWorth.CanonicalAmount(), assets.Currency().String())
	for _, item := range items {
		fmt.Fprintf(hash, "|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%t|%s", item.AccountID.String(), snapshotItemHoldingIDString(item), snapshotItemInstrumentIDString(item), snapshotItemStateObservationIDString(item), snapshotItemPreferenceObservationIDString(item), snapshotItemFXPreferenceObservationIDString(item), item.NativeAmount, item.NativeCurrency.String(), snapshotItemBaseAmountString(item), snapshotItemQuoteIDString(item), snapshotItemFXQuoteIDString(item), item.Complete, snapshotItemMissingReasonString(item))
	}
	return snapshotContentHashVersion + ":" + hex.EncodeToString(hash.Sum(nil))
}

func snapshotHashNeedsRebuild(contentHash string) bool {
	return !strings.HasPrefix(contentHash, snapshotContentHashVersion+":")
}

func snapshotItemSortKey(item domain.DailyValuationSnapshotItem) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%t|%s", item.AccountID.String(), snapshotItemHoldingIDString(item), snapshotItemInstrumentIDString(item), snapshotItemStateObservationIDString(item), snapshotItemPreferenceObservationIDString(item), snapshotItemFXPreferenceObservationIDString(item), item.NativeAmount, item.NativeCurrency.String(), snapshotItemBaseAmountString(item), snapshotItemQuoteIDString(item), snapshotItemFXQuoteIDString(item), item.Complete, snapshotItemMissingReasonString(item))
}

func snapshotItemHoldingIDString(item domain.DailyValuationSnapshotItem) string {
	if item.HoldingID == nil {
		return ""
	}
	return item.HoldingID.String()
}

func snapshotItemInstrumentIDString(item domain.DailyValuationSnapshotItem) string {
	if item.InstrumentID == nil {
		return ""
	}
	return item.InstrumentID.String()
}

func snapshotItemQuoteIDString(item domain.DailyValuationSnapshotItem) string {
	if item.QuoteID == nil {
		return ""
	}
	return *item.QuoteID
}

func snapshotItemStateObservationIDString(item domain.DailyValuationSnapshotItem) string {
	if item.StateObservationID == nil {
		return ""
	}
	return item.StateObservationID.String()
}

func snapshotItemPreferenceObservationIDString(item domain.DailyValuationSnapshotItem) string {
	if item.PreferenceObservationID == nil {
		return ""
	}
	return item.PreferenceObservationID.String()
}

func snapshotItemFXQuoteIDString(item domain.DailyValuationSnapshotItem) string {
	if item.FXQuoteID == nil {
		return ""
	}
	return *item.FXQuoteID
}

func snapshotItemFXPreferenceObservationIDString(item domain.DailyValuationSnapshotItem) string {
	if item.FXPreferenceObservationID == nil {
		return ""
	}
	return item.FXPreferenceObservationID.String()
}

func snapshotItemMissingReasonString(item domain.DailyValuationSnapshotItem) string {
	if item.MissingReason == nil {
		return ""
	}
	return *item.MissingReason
}

func missingReason(component domain.ValuationComponent, _ []domain.MissingInputView) string {
	if component.InstrumentID != nil {
		return "missing instrument price or FX rate"
	}
	return "missing account value or FX rate"
}

func snapshotItemBaseAmountString(item domain.DailyValuationSnapshotItem) string {
	if item.BaseAmountExact != "" {
		return item.BaseAmountExact
	}
	if item.BaseAmount == nil {
		return ""
	}
	return item.BaseAmount.CanonicalAmount()
}

func (s *Service) RebuildHistoricalSnapshots(ctx context.Context, startDate, endDate string) (int, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return 0, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "snapshot date range is invalid"}
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil || end.Before(start) {
		return 0, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "snapshot date range is invalid"}
	}
	if end.Sub(start) > 30*24*time.Hour {
		return 0, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "a snapshot rebuild is limited to 31 days"}
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return 0, err
	}
	if origin == nil {
		return 0, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before building snapshots"}
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return 0, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	endLocal, err := time.ParseInLocation("2006-01-02", endDate, location)
	if err != nil {
		return 0, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "snapshot date range is invalid"}
	}
	nextMidnight, err := domain.ResolveLocalDateTime(endLocal.AddDate(0, 0, 1).Format("2006-01-02"), "00:00", origin.Timezone)
	if err != nil {
		return 0, err
	}
	batch, err := s.repository.LoadHistoricalSnapshotBatch(ctx, origin.HouseholdID, nextMidnight.Add(-time.Millisecond))
	if err != nil {
		return 0, err
	}
	quoteCache := &historicalQuoteCache{instrumentQuotes: make(map[domain.InstrumentID][]domain.InstrumentQuote), fxQuotes: batch.FXQuoteFacts}
	for _, quote := range batch.InstrumentQuoteFacts {
		quoteCache.instrumentQuotes[quote.InstrumentID] = append(quoteCache.instrumentQuotes[quote.InstrumentID], quote)
	}
	ctx = context.WithValue(ctx, historicalQuoteCacheKey{}, quoteCache)
	ctx = context.WithValue(ctx, historicalSnapshotBatchKey{}, &batch)
	appended := 0
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return appended, err
		}
		_, changed, buildErr := s.BuildDailyValuationSnapshot(ctx, date.Format("2006-01-02"))
		if buildErr != nil {
			return appended, buildErr
		}
		if changed {
			appended++
		}
	}
	return appended, nil
}

func (s *Service) CompleteDailySnapshotRange(ctx context.Context, householdID domain.HouseholdID, targetDate string) error {
	err := s.WithWrite(ctx, func(ctx context.Context) error {
		return s.repository.CompleteDailySnapshotRange(ctx, householdID, targetDate, s.clock())
	})
	if err == nil {
		s.invalidateAnalysis()
	}
	return err
}

func (s *Service) DailySnapshotState(ctx context.Context, householdID domain.HouseholdID) (domain.DailySnapshotState, error) {
	return s.repository.DailySnapshotState(ctx, householdID)
}
