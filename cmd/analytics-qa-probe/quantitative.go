package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type quantitativeCase struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Status      string         `json:"status"`
	Query       map[string]any `json:"query"`
	Expected    map[string]any `json:"expected"`
	Actual      map[string]any `json:"actual"`
	Delta       map[string]any `json:"delta,omitempty"`
	Tolerance   string         `json:"tolerance"`
	ErrorReason string         `json:"errorReason,omitempty"`
	Evidence    []string       `json:"evidence,omitempty"`
}

type quantitativeOut struct {
	CodeVersion    string             `json:"codeVersion"`
	FixtureVersion string             `json:"fixtureVersion"`
	Scenario       string             `json:"scenario"`
	DBPath         string             `json:"dbPath"`
	Cases          []quantitativeCase `json:"cases"`
	AllPass        bool               `json:"allPass"`
}

type snapshotValue struct {
	AccountID    string
	InstrumentID string
	Amount       decimal.Decimal
}

type snapshotTotals struct {
	Total        decimal.Decimal
	ByAccount    map[string]decimal.Decimal
	ByInstrument map[string]decimal.Decimal
	Cash         decimal.Decimal
}

// withoutTransferRepository provides the counterfactual input for C8 without
// changing the fixture database. All persistence stays delegated to the real
// repository; only the selected in-range cash_transfer activities are
// omitted from the analysis read.
type withoutTransferRepository struct {
	application.Repository
	from string
	to   string
}

func (r withoutTransferRepository) ListActivitiesUntil(ctx context.Context, householdID domain.HouseholdID, cutoff time.Time) ([]domain.Activity, error) {
	activities, err := r.Repository.ListActivitiesUntil(ctx, householdID, cutoff)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.Activity, 0, len(activities))
	for _, activity := range activities {
		if activity.Kind == domain.ActivityCashTransfer && activity.EffectiveLocalDate >= r.from && activity.EffectiveLocalDate <= r.to {
			continue
		}
		filtered = append(filtered, activity)
	}
	return filtered, nil
}

type transferLeg struct {
	accountID string
	currency  domain.CurrencyCode
	amount    decimal.Decimal
}

func runQuantitativeProbe() error {
	dbPath := envOr("NESTWORTH_DATABASE_PATH", "/workspace/nestworth-analytics-qa/data/nestworth.db")
	outPath := envOr("NESTWORTH_PROBE_OUT", "/workspace/nestworth-analytics-qa/probes/quantitative.json")
	from := envOr("NESTWORTH_PROBE_FROM", "2026-08-01")
	to := envOr("NESTWORTH_PROBE_TO", "2026-08-31")
	scenario := envOr("NESTWORTH_QA_SCENARIO", "complete")
	out := quantitativeOut{CodeVersion: envOr("NESTWORTH_CODE_VERSION", "working-tree"), FixtureVersion: envOr("NESTWORTH_FIXTURE_VERSION", "analytics-qa-seed-v3"), Scenario: scenario, DBPath: dbPath, Cases: []quantitativeCase{}}

	database, _, _, err := openProbeDB(dbPath)
	if err != nil {
		return writeQuantitative(outPath, out, err)
	}
	defer database.Close()
	ctx := context.Background()
	repository := sqlite.NewRepository(database)
	portfolio, err := repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil || portfolio.Household == nil {
		if err == nil {
			err = fmt.Errorf("household is missing")
		}
		return writeQuantitative(outPath, out, err)
	}
	service := application.NewService(repository)
	query := quantitativeQuery(from, to, domain.ScopeHousehold, "", domain.ValuationBase)
	beginningDate, err := dayBefore(from)
	if err != nil {
		return writeQuantitative(outPath, out, err)
	}
	beginning, err := loadSnapshotTotals(database.SQL, portfolio.Household.ID.String(), beginningDate)
	if err != nil {
		return writeQuantitative(outPath, out, err)
	}
	ending, err := loadSnapshotTotals(database.SQL, portfolio.Household.ID.String(), to)
	if err != nil {
		return writeQuantitative(outPath, out, err)
	}
	household, err := service.AssetChange(ctx, query)
	if err != nil {
		return writeQuantitative(outPath, out, err)
	}
	out.Cases = append(out.Cases, quantitativeScopeCase(ctx, service, portfolio, query, beginning, ending, household, beginningDate))
	out.Cases = append(out.Cases, quantitativeNativeCase(ctx, service, portfolio, from, to))
	out.Cases = append(out.Cases, quantitativeSourcesCase(ctx, service, query))
	out.Cases = append(out.Cases, quantitativeTransferCase(ctx, service, repository, portfolio, from, to))

	out.AllPass = true
	for _, probe := range out.Cases {
		if probe.Status != "PASS" {
			out.AllPass = false
		}
	}
	return writeQuantitative(outPath, out, nil)
}

func quantitativeQuery(from, to string, kind domain.ScopeKind, id string, valuation domain.Valuation) domain.AnalysisQuery {
	return domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: kind, ID: id}, From: domain.LocalDate(from), To: domain.LocalDate(to), Valuation: valuation, IncludeCash: true, Basis: domain.ReturnBasisInvestment}
}

func quantitativeScopeCase(ctx context.Context, service *application.Service, portfolio domain.PortfolioSnapshot, query domain.AnalysisQuery, beginning, ending snapshotTotals, household application.AssetChangeResult, beginningDate string) quantitativeCase {
	c := quantitativeCase{ID: "C5", Name: "Scope triangulation", Status: "PASS", Query: map[string]any{"scope": "household/account/instrument+cash", "from": query.From, "to": query.To, "valuation": string(query.Valuation), "includeCash": true}, Expected: map[string]any{}, Actual: map[string]any{}, Delta: map[string]any{}, Tolerance: "0"}
	c.Expected["boundaryBeginningDate"] = beginningDate
	c.Expected["householdBeginning"] = beginning.Total.String()
	c.Expected["householdEnding"] = ending.Total.String()
	c.Expected["householdChange"] = ending.Total.Sub(beginning.Total).String()
	c.Actual["householdBeginning"] = moneyAmount(household.Summary.BeginningValue)
	c.Actual["householdEnding"] = moneyAmount(household.Summary.EndingValue)
	c.Actual["householdChange"] = moneyAmount(household.Summary.Change)
	c.Delta["householdChange"] = moneyDelta(household.Summary.Change, ending.Total.Sub(beginning.Total))

	accountBeginning, accountEnding, accountActual := decimal.Zero, decimal.Zero, decimal.Zero
	for _, record := range portfolio.Accounts {
		id := record.Account.ID.String()
		accountBeginning = accountBeginning.Add(beginning.ByAccount[id])
		accountEnding = accountEnding.Add(ending.ByAccount[id])
		accountQuery := quantitativeQuery(string(query.From), string(query.To), domain.ScopeAccount, id, query.Valuation)
		result, err := service.AssetChange(ctx, accountQuery)
		if err != nil {
			return failQuantitative(c, fmt.Sprintf("account %s: %v", id, err))
		}
		if result.Summary.Change != nil {
			accountActual = accountActual.Add(result.Summary.Change.Amount())
		}
	}
	c.Expected["accountPartitionChange"] = accountEnding.Sub(accountBeginning).String()
	c.Actual["accountPartitionChange"] = accountActual.String()
	c.Delta["accountPartitionChange"] = accountActual.Sub(accountEnding.Sub(accountBeginning)).String()

	instrumentBeginning, instrumentEnding, instrumentActual := decimal.Zero, decimal.Zero, decimal.Zero
	seen := map[string]bool{}
	for item := range beginning.ByInstrument {
		if item != "" {
			seen[item] = true
		}
	}
	for item := range ending.ByInstrument {
		if item != "" {
			seen[item] = true
		}
	}
	for item := range seen {
		if item == "" {
			continue
		}
		instrumentBeginning = instrumentBeginning.Add(beginning.ByInstrument[item])
		instrumentEnding = instrumentEnding.Add(ending.ByInstrument[item])
		instrumentQuery := quantitativeQuery(string(query.From), string(query.To), domain.ScopeInstrument, item, query.Valuation)
		result, err := service.AssetChange(ctx, instrumentQuery)
		if err != nil {
			return failQuantitative(c, fmt.Sprintf("instrument %s: %v", item, err))
		}
		if result.Summary.Change != nil {
			instrumentActual = instrumentActual.Add(result.Summary.Change.Amount())
		}
	}
	c.Expected["instrumentChange"] = instrumentEnding.Sub(instrumentBeginning).String()
	c.Expected["cashChange"] = ending.Cash.Sub(beginning.Cash).String()
	c.Expected["instrumentPlusCashChange"] = instrumentEnding.Sub(instrumentBeginning).Add(ending.Cash.Sub(beginning.Cash)).String()
	c.Actual["instrumentChange"] = instrumentActual.String()
	c.Actual["cashPlusInstrumentChange"] = instrumentActual.Add(householdChangeForCash(household, instrumentActual)).String()
	c.Delta["instrumentChange"] = instrumentActual.Sub(instrumentEnding.Sub(instrumentBeginning)).String()
	if !decimalEqual(moneyDecimal(household.Summary.Change), ending.Total.Sub(beginning.Total)) || !decimalEqual(accountActual, ending.Total.Sub(beginning.Total)) || !decimalEqual(instrumentActual, instrumentEnding.Sub(instrumentBeginning)) {
		return failQuantitative(c, "scope partition does not reconcile to independent snapshot totals")
	}
	c.Evidence = []string{"expected totals come from latest daily_valuation_snapshot_items at query boundaries", "instrument scope is checked separately and cash is added explicitly; rates are not summed"}
	return c
}

func householdChangeForCash(household application.AssetChangeResult, instrumentChange decimal.Decimal) decimal.Decimal {
	if household.Summary.Change == nil {
		return decimal.Zero
	}
	return household.Summary.Change.Amount().Sub(instrumentChange)
}

func quantitativeNativeCase(ctx context.Context, service *application.Service, portfolio domain.PortfolioSnapshot, from, to string) quantitativeCase {
	c := quantitativeCase{ID: "C6", Name: "Native versus Base", Status: "PASS", Query: map[string]any{"from": from, "to": to, "includeCash": true}, Expected: map[string]any{}, Actual: map[string]any{}, Delta: map[string]any{}, Tolerance: "0"}
	currencies := map[domain.CurrencyCode]bool{}
	for _, account := range portfolio.Accounts {
		currencies[account.Account.DefaultCurrency] = true
	}
	baseQuery := quantitativeQuery(from, to, domain.ScopeHousehold, "", domain.ValuationBase)
	nativeQuery := baseQuery
	nativeQuery.Valuation = domain.ValuationNative
	base, baseErr := service.AssetChange(ctx, baseQuery)
	native, nativeErr := service.AssetChange(ctx, nativeQuery)
	if baseErr != nil || nativeErr != nil {
		return failQuantitative(c, fmt.Sprintf("base=%v native=%v", baseErr, nativeErr))
	}
	expectsFallback := len(currencies) > 1
	c.Expected["mixedCurrencies"] = expectsFallback
	c.Expected["nativeValuationForced"] = expectsFallback
	c.Actual["nativeCurrency"] = moneyCurrency(native.Summary.Change)
	c.Actual["nativeValuationForced"] = stringPointerValue(native.ValuationForced)
	c.Actual["baseCurrency"] = moneyCurrency(base.Summary.Change)
	if expectsFallback && (native.ValuationForced == nil || *native.ValuationForced != string(domain.ValuationBase)) {
		return failQuantitative(c, "mixed-currency Native query did not force Base")
	}
	if !expectsFallback && native.ValuationForced != nil {
		return failQuantitative(c, "single-currency Native query unexpectedly forced Base")
	}
	for _, instrument := range portfolio.Instruments {
		instrumentQuery := quantitativeQuery(from, to, domain.ScopeInstrument, instrument.ID.String(), domain.ValuationNative)
		result, err := service.AssetChange(ctx, instrumentQuery)
		if err != nil {
			return failQuantitative(c, fmt.Sprintf("instrument Native %s: %v", instrument.ID, err))
		}
		if result.Summary.Change != nil && result.Summary.Change.Currency() != instrument.QuoteCurrency {
			return failQuantitative(c, fmt.Sprintf("instrument Native %s currency=%s want=%s", instrument.ID, result.Summary.Change.Currency(), instrument.QuoteCurrency))
		}
	}
	c.Evidence = []string{"currency cardinality comes from portfolio account metadata", "instrument Native currency is checked against instrument quote currency", "missing-FX behavior remains represented by the service availability/forced fields"}
	return c
}

func quantitativeSourcesCase(ctx context.Context, service *application.Service, query domain.AnalysisQuery) quantitativeCase {
	c := quantitativeCase{ID: "C7", Name: "Return sources", Status: "PASS", Query: map[string]any{"from": query.From, "to": query.To, "valuation": string(query.Valuation), "basis": string(query.Basis), "includeCash": query.IncludeCash}, Expected: map[string]any{}, Actual: map[string]any{}, Delta: map[string]any{}, Tolerance: "0"}
	trend, err := service.ReturnTrend(ctx, query, application.ReturnTrendPeriodReturnAmount)
	if err != nil {
		return failQuantitative(c, err.Error())
	}
	sum := decimal.Zero
	keys := make([]string, 0, len(trend.Sources))
	allowed := map[string]bool{string(domain.ReturnPriceChange): true, string(domain.ReturnFXImpact): true, string(domain.ReturnDividendInterest): true, string(domain.ReturnInvestmentFee): true}
	for _, source := range trend.Sources {
		if source.Amount == nil || !allowed[source.Key] {
			return failQuantitative(c, "sources contain an unknown or unavailable return component")
		}
		sum = sum.Add(source.Amount.Amount())
		keys = append(keys, source.Key)
	}
	sort.Strings(keys)
	c.Expected["sourceKeys"] = keys
	c.Expected["sourceSumEqualsPeriodReturn"] = true
	c.Actual["sourceSum"] = sum.String()
	c.Actual["periodReturn"] = moneyAmount(trend.Amount)
	c.Actual["coverage"] = map[string]int{"ratedDays": trend.Coverage.RatedDays, "totalDays": trend.Coverage.TotalDays}
	delta := sum.Sub(moneyDecimal(trend.Amount))
	c.Delta["sourceSumMinusPeriodReturn"] = delta.String()
	if trend.Amount == nil || !delta.IsZero() {
		return failQuantitative(c, "Price/FX/Dividend/Fee sources do not sum to period return")
	}
	c.Evidence = []string{"realized gain is intentionally excluded; only total-return source components are compared", "coverage is emitted alongside the source sum"}
	return c
}

func quantitativeTransferCase(ctx context.Context, service *application.Service, repository application.Repository, portfolio domain.PortfolioSnapshot, from, to string) quantitativeCase {
	c := quantitativeCase{ID: "C8", Name: "Transfer neutrality", Status: "PASS", Query: map[string]any{"from": from, "to": to, "basis": string(domain.ReturnBasisInvestment), "scope": "household", "includeCash": true, "counterfactual": "same snapshots and quotes with in-range cash_transfer activities removed"}, Expected: map[string]any{"householdChangeDeltaWithVsWithoutTransfer": "0", "householdExternalFlowDeltaWithVsWithoutTransfer": "0", "marketFXFeeDeltaWithVsWithoutTransfer": "0", "accountEndpointDeltaMatchesTransferLeg": true, "returnSourceDeltaWithVsWithoutTransfer": "0"}, Actual: map[string]any{}, Delta: map[string]any{}, Tolerance: "0"}
	activities, err := service.ListActivities(ctx, 1000)
	if err != nil {
		return failQuantitative(c, err.Error())
	}
	transferDays := map[string]map[string]transferLeg{}
	for _, activity := range activities {
		if activity.Kind != domain.ActivityCashTransfer || activity.EffectiveLocalDate < from || activity.EffectiveLocalDate > to {
			continue
		}
		if transferDays[activity.EffectiveLocalDate] == nil {
			transferDays[activity.EffectiveLocalDate] = map[string]transferLeg{}
		}
		for _, effect := range activity.Effects {
			if effect.AccountID == nil || effect.Money == nil || (effect.Role != domain.EffectRoleTransferFrom && effect.Role != domain.EffectRoleTransferTo) {
				continue
			}
			amount := effect.Money.Amount()
			if effect.Direction == domain.EffectRemoved {
				amount = amount.Neg()
			}
			accountID := effect.AccountID.String()
			leg := transferDays[activity.EffectiveLocalDate][accountID]
			leg.accountID = accountID
			leg.currency = effect.Money.Currency()
			leg.amount = leg.amount.Add(amount)
			transferDays[activity.EffectiveLocalDate][accountID] = leg
		}
	}
	if len(transferDays) == 0 {
		return failQuantitative(c, "no cash_transfer activity in selected range")
	}
	days := make([]string, 0, len(transferDays))
	for date := range transferDays {
		days = append(days, date)
	}
	sort.Strings(days)
	withoutTransfers := application.NewService(withoutTransferRepository{Repository: repository, from: from, to: to})
	for _, date := range days {
		query := quantitativeQuery(date, date, domain.ScopeHousehold, "", domain.ValuationBase)
		withResult, err := service.AssetChange(ctx, query)
		if err != nil {
			return failQuantitative(c, fmt.Sprintf("transfer day %s: %v", date, err))
		}
		withoutResult, err := withoutTransfers.AssetChange(ctx, query)
		if err != nil {
			return failQuantitative(c, fmt.Sprintf("transfer counterfactual day %s: %v", date, err))
		}
		changeDelta := moneyDecimal(withResult.Summary.Change).Sub(moneyDecimal(withoutResult.Summary.Change))
		externalDelta := assetChangeBucket(withResult, domain.BucketExternalFlow).Sub(assetChangeBucket(withoutResult, domain.BucketExternalFlow))
		dayActual := map[string]any{
			"householdChangeWithTransfer":          moneyAmount(withResult.Summary.Change),
			"householdChangeWithoutTransfer":       moneyAmount(withoutResult.Summary.Change),
			"householdExternalFlowWithTransfer":    assetChangeBucket(withResult, domain.BucketExternalFlow).String(),
			"householdExternalFlowWithoutTransfer": assetChangeBucket(withoutResult, domain.BucketExternalFlow).String(),
		}
		dayDelta := map[string]any{"householdChange": changeDelta.String(), "householdExternalFlow": externalDelta.String()}
		for _, bucket := range []domain.AttributionBucket{domain.BucketPriceChange, domain.BucketFXImpact, domain.BucketFee} {
			delta := assetChangeBucket(withResult, bucket).Sub(assetChangeBucket(withoutResult, bucket))
			dayActual[string(bucket)+"WithTransfer"] = assetChangeBucket(withResult, bucket).String()
			dayActual[string(bucket)+"WithoutTransfer"] = assetChangeBucket(withoutResult, bucket).String()
			dayDelta[string(bucket)] = delta.String()
			if !delta.IsZero() {
				return failQuantitative(c, fmt.Sprintf("transfer day %s changed %s attribution by %s", date, bucket, delta))
			}
		}
		accountActual := map[string]any{}
		accountDelta := map[string]string{}
		for accountID, leg := range transferDays[date] {
			if leg.amount.IsZero() {
				continue
			}
			accountQuery := quantitativeQuery(date, date, domain.ScopeAccount, accountID, domain.ValuationBase)
			withAccount, accountErr := service.AssetChange(ctx, accountQuery)
			if accountErr != nil {
				return failQuantitative(c, fmt.Sprintf("transfer account %s on %s: %v", accountID, date, accountErr))
			}
			withoutAccount, accountErr := withoutTransfers.AssetChange(ctx, accountQuery)
			if accountErr != nil {
				return failQuantitative(c, fmt.Sprintf("transfer counterfactual account %s on %s: %v", accountID, date, accountErr))
			}
			observed := assetChangeBucket(withAccount, domain.BucketExternalFlow).Sub(assetChangeBucket(withoutAccount, domain.BucketExternalFlow))
			changeDifference := moneyDecimal(withAccount.Summary.Change).Sub(moneyDecimal(withoutAccount.Summary.Change))
			accountActual[accountID] = map[string]string{"leg": leg.amount.String(), "currency": leg.currency.String(), "externalFlowDelta": observed.String(), "changeDelta": changeDifference.String()}
			accountDelta[accountID] = observed.Sub(leg.amount).String()
			if leg.currency != portfolio.Household.BaseCurrency {
				return failQuantitative(c, fmt.Sprintf("transfer account %s on %s uses %s; C8 expected base-currency endpoint legs", accountID, date, leg.currency))
			}
			if !observed.Equal(leg.amount) || !changeDifference.IsZero() {
				return failQuantitative(c, fmt.Sprintf("transfer account %s on %s endpoint mismatch: expected %s, observed external %s, change delta %s", accountID, date, leg.amount, observed, changeDifference))
			}
		}
		if !changeDelta.IsZero() || !externalDelta.IsZero() {
			return failQuantitative(c, fmt.Sprintf("transfer day %s changed household change/external flow: change=%s external=%s", date, changeDelta, externalDelta))
		}
		dayActual["accounts"] = accountActual
		dayDelta["accounts"] = accountDelta
		c.Actual[date] = dayActual
		c.Delta[date] = dayDelta
	}
	withReturn, err := service.ReturnTrend(ctx, quantitativeQuery(from, to, domain.ScopeHousehold, "", domain.ValuationBase), application.ReturnTrendPeriodReturnAmount)
	if err != nil {
		return failQuantitative(c, fmt.Sprintf("transfer return with activities: %v", err))
	}
	withoutReturn, err := withoutTransfers.ReturnTrend(ctx, quantitativeQuery(from, to, domain.ScopeHousehold, "", domain.ValuationBase), application.ReturnTrendPeriodReturnAmount)
	if err != nil {
		return failQuantitative(c, fmt.Sprintf("transfer return without activities: %v", err))
	}
	returnDelta := moneyDecimal(withReturn.Amount).Sub(moneyDecimal(withoutReturn.Amount))
	c.Actual["return"] = map[string]any{"withTransfer": moneyAmount(withReturn.Amount), "withoutTransfer": moneyAmount(withoutReturn.Amount), "sourcesWithTransfer": returnSourceAmounts(withReturn.Sources), "sourcesWithoutTransfer": returnSourceAmounts(withoutReturn.Sources)}
	c.Delta["returnAmount"] = returnDelta.String()
	if !returnDelta.IsZero() {
		return failQuantitative(c, fmt.Sprintf("transfer changed household return amount by %s", returnDelta))
	}
	withSources := returnSourceAmounts(withReturn.Sources)
	withoutSources := returnSourceAmounts(withoutReturn.Sources)
	for _, key := range []string{string(domain.ReturnPriceChange), string(domain.ReturnFXImpact), string(domain.ReturnInvestmentFee)} {
		delta := withSources[key].Sub(withoutSources[key])
		c.Delta["return_"+key] = delta.String()
		if !delta.IsZero() {
			return failQuantitative(c, fmt.Sprintf("transfer changed return source %s by %s", key, delta))
		}
	}
	c.Expected["transferDays"] = days
	c.Evidence = []string{"transfer days are discovered from persisted cash_transfer activities", "the counterfactual keeps snapshots and quotes fixed while removing only in-range cash_transfer activities", "household change and external flow are compared day by day", "price_change, fx_impact, fee, and return sources are compared independently", "both transfer endpoint accounts are checked against their signed transfer legs", fmt.Sprintf("accounts=%d instruments=%d", len(portfolio.Accounts), len(portfolio.Instruments))}
	return c
}

func assetChangeBucket(result application.AssetChangeResult, bucket domain.AttributionBucket) decimal.Decimal {
	for _, row := range result.Waterfall {
		if row.Bucket == bucket && row.Amount != nil {
			return row.Amount.Amount()
		}
	}
	return decimal.Zero
}

func returnSourceAmounts(sources []application.ReturnSource) map[string]decimal.Decimal {
	result := make(map[string]decimal.Decimal, len(sources))
	for _, source := range sources {
		if source.Amount != nil {
			result[source.Key] = source.Amount.Amount()
		}
	}
	return result
}

func loadSnapshotTotals(db *sql.DB, householdID, localDate string) (snapshotTotals, error) {
	totals := snapshotTotals{ByAccount: map[string]decimal.Decimal{}, ByInstrument: map[string]decimal.Decimal{}}
	rows, err := db.Query(`SELECT i.account_id, COALESCE(i.instrument_id, ''), i.base_amount
		FROM daily_valuation_snapshot_items i
		JOIN daily_valuation_snapshots s ON s.id = i.snapshot_id
		WHERE s.household_id = ? AND s.local_date = ?
		AND s.revision = (SELECT MAX(latest.revision) FROM daily_valuation_snapshots latest WHERE latest.household_id = s.household_id AND latest.local_date = s.local_date)
		AND i.base_amount IS NOT NULL AND i.base_amount != ''`, householdID, localDate)
	if err != nil {
		return totals, err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID, instrumentID, amount string
		if err := rows.Scan(&accountID, &instrumentID, &amount); err != nil {
			return totals, err
		}
		value, err := decimal.NewFromString(amount)
		if err != nil {
			return totals, fmt.Errorf("parse snapshot amount %q: %w", amount, err)
		}
		totals.Total = totals.Total.Add(value)
		totals.ByAccount[accountID] = totals.ByAccount[accountID].Add(value)
		if instrumentID == "" {
			totals.Cash = totals.Cash.Add(value)
		} else {
			totals.ByInstrument[instrumentID] = totals.ByInstrument[instrumentID].Add(value)
		}
	}
	return totals, rows.Err()
}

func dayBefore(value string) (string, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", fmt.Errorf("parse query start date %q: %w", value, err)
	}
	return parsed.AddDate(0, 0, -1).Format("2006-01-02"), nil
}

func failQuantitative(c quantitativeCase, reason string) quantitativeCase {
	c.Status = "FAIL"
	c.ErrorReason = reason
	return c
}

func moneyAmount(value *domain.SignedMoney) string {
	if value == nil {
		return ""
	}
	return value.CanonicalAmount()
}

func moneyCurrency(value *domain.SignedMoney) string {
	if value == nil {
		return ""
	}
	return value.Currency().String()
}

func moneyDecimal(value *domain.SignedMoney) decimal.Decimal {
	if value == nil {
		return decimal.Zero
	}
	return value.Amount()
}

func moneyDelta(value *domain.SignedMoney, expected decimal.Decimal) string {
	return moneyDecimal(value).Sub(expected).String()
}

func decimalEqual(left, right decimal.Decimal) bool { return left.Equal(right) }

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func writeQuantitative(outPath string, out quantitativeOut, runErr error) error {
	if runErr != nil {
		out.AllPass = false
		out.Cases = append(out.Cases, quantitativeCase{ID: "probe", Name: "probe execution", Status: "FAIL", Query: map[string]any{}, Expected: map[string]any{}, Actual: map[string]any{}, Tolerance: "0", ErrorReason: runErr.Error()})
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return err
	}
	fmt.Println(string(raw))
	if runErr != nil {
		return runErr
	}
	if !out.AllPass {
		return fmt.Errorf("one or more quantitative probes failed")
	}
	return nil
}
