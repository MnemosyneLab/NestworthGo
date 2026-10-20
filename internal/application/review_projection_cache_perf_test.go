//go:build perf

package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// BenchmarkReviewProjectionIncremental100x20 measures one new closed date
// against the design's typical dimensions: 100 positive holdings, 20 rules,
// one year of source history, 100k quote facts, and 50k unrelated activities.
// It reuses the trailing 30 published dates; financial source capture, SQLite
// publication, and report-page reads are outside this application-layer run.
func BenchmarkReviewProjectionIncremental100x20(b *testing.B) {
	capture, now := reviewWarmReplayBenchmarkCapture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		publication, err := PrepareReviewPublication(context.Background(), capture, now, "synthetic-fx")
		if err != nil {
			b.Fatal(err)
		}
		if len(publication.Rules) != 100*20 {
			b.Fatalf("published %d rule/instrument pairs, want 2000", len(publication.Rules))
		}
	}
}

func reviewWarmReplayBenchmarkCapture(b *testing.B) (domain.ReviewProjectionCapture, time.Time) {
	b.Helper()
	household := domain.NewHouseholdID()
	accountID := domain.NewAccountID()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	start := time.Date(2025, 10, 16, 0, 0, 0, 0, time.UTC)
	previousLatest := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	origin, err := domain.NewHistoryOrigin(household, "UTC", start, start)
	if err != nil {
		b.Fatal(err)
	}
	quantity, err := domain.NewQuantity(decimal.NewFromInt(1))
	if err != nil {
		b.Fatal(err)
	}
	currency := domain.CurrencyCode("CNY")
	cashAmount, err := domain.ParseMoney("1", currency)
	if err != nil {
		b.Fatal(err)
	}
	quotePrice, err := domain.ParseUnitPrice("100")
	if err != nil {
		b.Fatal(err)
	}
	house := domain.Household{ID: household, Name: "Synthetic benchmark", BaseCurrency: currency, CreatedAt: start, UpdatedAt: start}
	account := domain.Account{ID: accountID, HouseholdID: household, Name: "Synthetic account", AccountType: domain.TypeBrokerage, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingHoldings, DefaultCurrency: currency, IncludeInPortfolio: true, IncludeInNetWorth: true, CreatedAt: start, UpdatedAt: start}
	instruments := make([]domain.Instrument, 100)
	holdings := make([]domain.Holding, 100)
	components := make([]domain.HistoryOriginComponent, 100)
	ids := make([]domain.InstrumentID, 100)
	for index := range instruments {
		instrumentID := domain.NewInstrumentID()
		holdingID := domain.NewHoldingID()
		ids[index] = instrumentID
		instruments[index] = domain.Instrument{ID: instrumentID, HouseholdID: household, Name: fmt.Sprintf("Synthetic %03d", index), Type: domain.InstrumentStock, QuoteCurrency: currency, CreatedAt: start.AddDate(-1, 0, 0), UpdatedAt: start.AddDate(-1, 0, 0)}
		holdings[index] = domain.Holding{ID: holdingID, AccountID: accountID, InstrumentID: instrumentID, Quantity: quantity, CreatedAt: start.Add(-time.Hour), UpdatedAt: start.Add(-time.Hour)}
		a, h, i, q := accountID, holdingID, instrumentID, quantity
		components[index] = domain.HistoryOriginComponent{ID: domain.NewHistoryOriginComponentID(), OriginID: origin.ID, Kind: domain.HistoryOriginHoldingQuantity, AccountID: &a, HoldingID: &h, InstrumentID: &i, Quantity: &q, CreatedAt: start}
	}
	activities := make([]domain.Activity, 50_000)
	for index := range activities {
		id := domain.NewActivityID()
		on := start.AddDate(0, 0, index%365)
		account := accountID
		activities[index] = domain.Activity{
			ID: id, HouseholdID: household, Kind: domain.ActivityCashDividend,
			EffectiveAt: on.Add(time.Hour), EffectiveLocalDate: on.Format(time.DateOnly), CreatedAt: on.Add(time.Hour),
			Effects: []domain.ActivityEffect{{
				ID: domain.NewActivityEffectID(), ActivityID: id, Sequence: 1, Role: domain.EffectRoleAmount,
				Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash,
				Classification: domain.ClassificationIncome, AccountID: &account, Money: &cashAmount,
			}},
		}
	}
	quoteFacts := make([]domain.InstrumentQuote, 100_000)
	for index := range quoteFacts {
		instrumentIndex := index % len(ids)
		dayOffset := (index / len(ids)) % 365
		on := start.AddDate(0, 0, dayOffset)
		quoteFacts[index] = domain.InstrumentQuote{
			ID:           domain.InstrumentQuoteID(fmt.Sprintf("synthetic-quote-%06d", index)),
			InstrumentID: ids[instrumentIndex], UnitPrice: quotePrice, Currency: currency,
			SourceKind: domain.QuoteSourceManual, SourceKey: "synthetic", QuotedAt: on.Add(16 * time.Hour),
			CreatedAt: on.Add(17 * time.Hour), ObservationKind: "close", EffectiveDate: on.Format(time.DateOnly),
			BindingRevision: 0, PriceBasis: domain.TiingoRawClosePriceBasis, TimestampBasis: "session_close",
		}
	}
	history := domain.HistoricalSnapshotBatch{
		Origin: origin, OriginData: domain.HistoryOriginData{Origin: origin, Components: components},
		Portfolio:  domain.PortfolioSnapshot{Household: &house, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: instruments, Holdings: holdings},
		Activities: activities, InstrumentQuoteFacts: quoteFacts,
	}
	rules := make([]domain.ReviewRule, 20)
	for index := range rules {
		spec := syntheticHistoricalReviewSpec()
		spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 30, MinHits: 10}
		rules[index] = domain.ReviewRule{ID: domain.NewReviewRuleID(), HouseholdID: household, Name: fmt.Sprintf("Synthetic rule %02d", index), Enabled: true, Severity: "notice", Evaluator: domain.ReviewEvaluator, RowRevision: 1, CurrentConditionVersion: 1, Spec: spec}
	}
	capture := domain.ReviewProjectionCapture{
		HouseholdID: household, Historical: domain.ReviewHistoricalBatch{History: history},
		Control: domain.ReviewProjectionControl{Enabled: true, ProjectionStatus: "complete"},
		Rules:   rules, OpenEpisodes: []domain.ReviewStoredEpisode{}, EvaluatedInstruments: map[domain.ReviewRuleID][]domain.InstrumentID{},
		ReusableDailyEvidence: make(map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence, 100*20*30),
	}
	truth := domain.ReviewTrue
	typeOfInstrument := domain.InstrumentStock
	quantityText := "1"
	for _, rule := range rules {
		timezoneVersion := rule.Spec.Schedule.Timezone + "/" + rule.Spec.Schedule.CutoffVersion
		for _, instrumentID := range ids {
			for dayIndex := 29; dayIndex >= 0; dayIndex-- {
				on := previousLatest.AddDate(0, 0, -dayIndex).Format(time.DateOnly)
				key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: instrumentID, TimezoneVersion: timezoneVersion, On: on}
				capture.ReusableDailyEvidence[key] = domain.ReviewReusableDailyEvidence{
					Input:           domain.ReviewDailyInput{On: on, InstrumentID: instrumentID, FXProviderKey: "synthetic-fx", Quantity: &quantityText, InstrumentType: &typeOfInstrument, NativeCurrency: &currency},
					CoverageMissing: []string{}, InputFingerprint: "stable-synthetic-fingerprint",
					Result: domain.ReviewDailyResult{On: on, Truth: truth}, HasResult: true,
				}
			}
		}
	}
	return capture, now
}
