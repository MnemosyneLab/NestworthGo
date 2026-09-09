package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// AnalysisAvailability is shared by the analysis projections. Available is
// deliberately separate from Status: a partial result contains usable
// values, but the caller must not mistake it for a complete period.
type AnalysisAvailability struct {
	Available       bool
	Status          domain.Completeness
	MissingReason   string
	ValuationForced *string
}

type AssetChangeRow struct {
	Key    string
	Label  string
	Bucket domain.AttributionBucket
	Amount *domain.SignedMoney
}

type AssetChangeGroup struct {
	Key    string
	Label  string
	Amount *domain.SignedMoney
	Rows   []AssetChangeRow
}

type AssetChangeSummary struct {
	BeginningValue *domain.SignedMoney
	EndingValue    *domain.SignedMoney
	Change         *domain.SignedMoney
}

type AssetChangeResult struct {
	AnalysisAvailability
	Summary            AssetChangeSummary
	Waterfall          []AssetChangeRow
	Groups             []AssetChangeGroup
	ResidualIssueCount int
}

type AnalysisDimensionAmount struct {
	Key          string
	Label        string
	AccountID    string
	InstrumentID string
	Amount       *domain.SignedMoney
}

type AssetDriverDetailResult struct {
	AnalysisAvailability
	DriverKey       string
	ByInstrument    []AnalysisDimensionAmount
	ByAccount       []AnalysisDimensionAmount
	ResidualDetails []AssetResidualDetail
}

// AssetResidualDetail keeps the component/day identity of an unexplained
// difference. Residuals are not an aggregate-only driver: the user needs to
// know which valued component and closed day requires attention.
type AssetResidualDetail struct {
	Date         domain.LocalDate
	ComponentKey string
	AccountID    string
	HoldingID    string
	InstrumentID string
	Amount       *domain.SignedMoney
}

type AssetTrendGranularity string

const (
	TrendDay   AssetTrendGranularity = "day"
	TrendWeek  AssetTrendGranularity = "week"
	TrendMonth AssetTrendGranularity = "month"
)

type AssetTrendMetric string

const (
	TrendNetWorth         AssetTrendMetric = "net_worth"
	TrendAssets           AssetTrendMetric = "assets"
	TrendLiabilities      AssetTrendMetric = "liabilities"
	TrendIncome           AssetTrendMetric = "income"
	TrendSpending         AssetTrendMetric = "spending"
	TrendFees             AssetTrendMetric = "fees"
	TrendDividendInterest AssetTrendMetric = "dividend_interest"
	TrendExternalFlow     AssetTrendMetric = "external_flow"
	TrendPriceChange      AssetTrendMetric = "price_change"
	TrendFXImpact         AssetTrendMetric = "fx_impact"
	TrendInvestmentReturn AssetTrendMetric = "investment_return"
	TrendReturnRate       AssetTrendMetric = "return_rate"
	TrendNetChange        AssetTrendMetric = "net_change"
	TrendResidual         AssetTrendMetric = "residual"
)

type AssetTrendPoint struct {
	Period   string
	Value    *domain.SignedMoney
	Rate     *decimal.Decimal
	Coverage domain.RateCoverage
	AnalysisAvailability
}

type AssetTrendResult struct {
	AnalysisAvailability
	Points   []AssetTrendPoint
	Summary  *domain.SignedMoney
	Rate     *decimal.Decimal
	Coverage domain.RateCoverage
}

type AnalysisCategoryType string

const (
	CategoryIncome           AnalysisCategoryType = "income"
	CategorySpending         AnalysisCategoryType = "spending"
	CategoryFees             AnalysisCategoryType = "fees"
	CategoryDividendInterest AnalysisCategoryType = "dividend_interest"
	CategoryInvestmentReturn AnalysisCategoryType = "investment_return"
)

type CategoryRow struct {
	Key          string
	Label        string
	AccountID    string
	InstrumentID string
	AssetClass   string
	Amount       *domain.SignedMoney
}

type CategoriesResult struct {
	AnalysisAvailability
	Total *domain.SignedMoney
	Rows  []CategoryRow
}

type CategoryChild struct {
	Key          string
	Label        string
	AccountID    string
	InstrumentID string
	Amount       *domain.SignedMoney
}

type CategoryActivityRef struct {
	Date         domain.LocalDate
	ActivityID   string
	AccountID    string
	HoldingID    string
	InstrumentID string
	Amount       *domain.SignedMoney
}

type CategoryDetailResult struct {
	AnalysisAvailability
	Children     []CategoryChild
	ActivityRefs []CategoryActivityRef
}

func (s *Service) AssetChange(ctx context.Context, query domain.AnalysisQuery) (AssetChangeResult, error) {
	result, forced, err := s.analyzeAssetProjection(ctx, query)
	if err != nil {
		return AssetChangeResult{}, err
	}
	return foldAssetChange(result, forced), nil
}

func (s *Service) AssetDriverDetail(ctx context.Context, query domain.AnalysisQuery, driverKey string) (AssetDriverDetailResult, error) {
	if !isAssetBucket(domain.AttributionBucket(strings.TrimSpace(driverKey))) {
		return AssetDriverDetailResult{}, &domain.Error{Code: domain.ErrValidation, Field: "driverKey", Message: "asset driver is invalid"}
	}
	result, forced, err := s.analyzeAssetProjection(ctx, query)
	if err != nil {
		return AssetDriverDetailResult{}, err
	}
	return foldAssetDriverDetail(result, forced, driverKey), nil
}

func (s *Service) AssetTrend(ctx context.Context, query domain.AnalysisQuery, granularity AssetTrendGranularity, metric AssetTrendMetric) (AssetTrendResult, error) {
	result, forced, err := s.analyzeAssetProjection(ctx, query)
	if err != nil {
		return AssetTrendResult{}, err
	}
	return foldAssetTrend(result, forced, granularity, metric)
}

func (s *Service) Categories(ctx context.Context, query domain.AnalysisQuery, categoryType AnalysisCategoryType) (CategoriesResult, error) {
	if err := validateCategoryType(categoryType); err != nil {
		return CategoriesResult{}, &domain.Error{Code: domain.ErrValidation, Field: "categoryType", Message: "analysis category is invalid"}
	}
	result, forced, err := s.analyzeAssetProjection(ctx, query)
	if err != nil {
		return CategoriesResult{}, err
	}
	return foldCategories(result, forced, categoryType), nil
}

func (s *Service) CategoryDetail(ctx context.Context, query domain.AnalysisQuery, categoryType AnalysisCategoryType, rowKey string) (CategoryDetailResult, error) {
	if err := validateCategoryType(categoryType); err != nil {
		return CategoryDetailResult{}, &domain.Error{Code: domain.ErrValidation, Field: "categoryType", Message: "analysis category is invalid"}
	}
	result, forced, err := s.analyzeAssetProjection(ctx, query)
	if err != nil {
		return CategoryDetailResult{}, err
	}
	return foldCategoryDetail(result, forced, categoryType, rowKey), nil
}

func (s *Service) analyzeAssetProjection(ctx context.Context, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, string, error) {
	if s == nil {
		return domain.PeriodAnalysisResult{}, "", &domain.Error{Code: domain.ErrUnavailable, Message: "application service is not configured"}
	}
	return s.analysisService().ComputeWithValuationFallback(ctx, query)
}

func (s *Service) analyzeReturnProjection(ctx context.Context, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, string, error) {
	if s == nil {
		return domain.PeriodAnalysisResult{}, "", &domain.Error{Code: domain.ErrUnavailable, Message: "application service is not configured"}
	}
	return s.analysisService().ComputeWithInvestmentValuationFallback(ctx, query)
}

func availability(result domain.PeriodAnalysisResult, forced string) AnalysisAvailability {
	hasUsableDay := false
	hasIncompleteDay := false
	for _, day := range result.Days {
		switch day.Status {
		case domain.CompletenessOK:
			hasUsableDay = true
		case domain.CompletenessPartial:
			hasUsableDay = true
			hasIncompleteDay = true
		case domain.CompletenessUnavailable:
			hasIncompleteDay = true
		}
	}
	status := domain.CompletenessUnavailable
	missingReason := "no usable asset days are available for this period"
	if hasUsableDay {
		status = domain.CompletenessOK
		missingReason = ""
		if hasIncompleteDay {
			status = domain.CompletenessPartial
			missingReason = "one or more asset days are incomplete"
		}
	}
	return AnalysisAvailability{Available: hasUsableDay, Status: status, MissingReason: missingReason, ValuationForced: forcedPointer(forced)}
}

func forcedPointer(forced string) *string {
	if forced == "" {
		return nil
	}
	value := forced
	return &value
}

func foldAssetChange(result domain.PeriodAnalysisResult, forced string) AssetChangeResult {
	status := availability(result, forced)
	waterfall := make(map[domain.AttributionBucket]decimal.Decimal)
	var currency domain.CurrencyCode
	for _, day := range result.Days {
		if day.BeginningValue.Currency() != "" {
			currency = day.BeginningValue.Currency()
		}
		if len(day.AssetBucketExact) > 0 {
			for bucket, amount := range day.AssetBucketExact {
				waterfall[bucket] = waterfall[bucket].Add(amount)
			}
		}
		for bucket, amount := range day.AssetBuckets {
			if len(day.AssetBucketExact) == 0 {
				waterfall[bucket] = waterfall[bucket].Add(amount.Amount())
			}
			if currency == "" {
				currency = amount.Currency()
			}
		}
	}

	beginning, beginningOK := periodBeginning(result)
	ending, endingOK := periodEnding(result)
	summary := AssetChangeSummary{}
	if beginningOK {
		summary.BeginningValue = signedPointer(beginning, currency)
	}
	if endingOK {
		summary.EndingValue = signedPointer(ending, currency)
	}
	if beginningOK && endingOK {
		summary.Change = signedPointer(ending.Sub(beginning), currency)
	}

	rows := make([]AssetChangeRow, 0, len(waterfall))
	for _, bucket := range orderedBuckets() {
		amount, ok := waterfall[bucket]
		if !ok || amount.IsZero() {
			continue
		}
		rows = append(rows, AssetChangeRow{Key: string(bucket), Label: assetBucketLabel(bucket), Bucket: bucket, Amount: signedPointer(amount, currency)})
	}
	groups := foldAssetGroups(waterfall, currency)
	return AssetChangeResult{AnalysisAvailability: status, Summary: summary, Waterfall: rows, Groups: groups, ResidualIssueCount: countResidualIssues(result)}
}

func countResidualIssues(result domain.PeriodAnalysisResult) int {
	count := 0
	for _, day := range result.Days {
		amount, ok := day.AssetBuckets[domain.BucketResidual]
		if !ok || amount.IsZero() {
			continue
		}
		count++
	}
	return count
}

func foldAssetGroups(waterfall map[domain.AttributionBucket]decimal.Decimal, currency domain.CurrencyCode) []AssetChangeGroup {
	type groupDef struct {
		key     string
		label   string
		buckets []domain.AttributionBucket
	}
	defs := []groupDef{
		{key: "cash", label: "Cash flows", buckets: []domain.AttributionBucket{domain.BucketExternalFlow, domain.BucketIncome, domain.BucketSpending}},
		{key: "market", label: "Market & investment", buckets: []domain.AttributionBucket{domain.BucketDividendInterest, domain.BucketPriceChange, domain.BucketFXImpact, domain.BucketFee}},
		{key: "other", label: "Other", buckets: []domain.AttributionBucket{domain.BucketLiabilityImpact, domain.BucketAdjustment, domain.BucketResidual}},
	}
	groups := make([]AssetChangeGroup, 0, len(defs))
	for _, def := range defs {
		amount := decimal.Zero
		rows := make([]AssetChangeRow, 0, len(def.buckets))
		for _, bucket := range def.buckets {
			value := waterfall[bucket]
			amount = amount.Add(value)
			if value.IsZero() {
				continue
			}
			rows = append(rows, AssetChangeRow{Key: string(bucket), Label: assetBucketLabel(bucket), Bucket: bucket, Amount: signedPointer(value, currency)})
		}
		if len(rows) == 0 {
			continue
		}
		groups = append(groups, AssetChangeGroup{Key: def.key, Label: def.label, Amount: signedPointer(amount, currency), Rows: rows})
	}
	return groups
}

func foldAssetDriverDetail(result domain.PeriodAnalysisResult, forced, driverKey string) AssetDriverDetailResult {
	status := availability(result, forced)
	bucket := domain.AttributionBucket(strings.TrimSpace(driverKey))
	byInstrument := make(map[string]AnalysisDimensionAmount)
	byAccount := make(map[string]AnalysisDimensionAmount)
	residualDetails := make([]AssetResidualDetail, 0)
	var currency domain.CurrencyCode
	for _, day := range result.Days {
		amount, ok := day.AssetBuckets[bucket]
		if !ok || amount.IsZero() {
			continue
		}
		currency = amount.Currency()
		component := day.Component
		if bucket == domain.BucketResidual {
			detail := AssetResidualDetail{
				Date:         day.Date,
				ComponentKey: component.Key(),
				AccountID:    component.AccountID.String(),
				Amount:       signedPointer(amount.Amount(), amount.Currency()),
			}
			if component.HoldingID != nil {
				detail.HoldingID = component.HoldingID.String()
			}
			if component.InstrumentID != nil {
				detail.InstrumentID = component.InstrumentID.String()
			}
			residualDetails = append(residualDetails, detail)
		}
		accountKey := component.AccountID.String()
		accountRow := byAccount[accountKey]
		accountRow.Key, accountRow.Label, accountRow.AccountID = accountKey, accountKey, accountKey
		accountRow.Amount = signedPointer(decimalValue(accountRow.Amount).Add(amount.Amount()), currency)
		byAccount[accountKey] = accountRow

		instrumentKey := "cash"
		if component.InstrumentID != nil {
			instrumentKey = component.InstrumentID.String()
		}
		instrumentRow := byInstrument[instrumentKey]
		instrumentRow.Key, instrumentRow.Label, instrumentRow.InstrumentID = instrumentKey, instrumentKey, instrumentKey
		instrumentRow.Amount = signedPointer(decimalValue(instrumentRow.Amount).Add(amount.Amount()), currency)
		byInstrument[instrumentKey] = instrumentRow
	}
	sort.Slice(residualDetails, func(i, j int) bool {
		if residualDetails[i].Date != residualDetails[j].Date {
			return residualDetails[i].Date < residualDetails[j].Date
		}
		return residualDetails[i].ComponentKey < residualDetails[j].ComponentKey
	})
	return AssetDriverDetailResult{AnalysisAvailability: status, DriverKey: string(bucket), ByInstrument: sortedDimensionAmounts(byInstrument), ByAccount: sortedDimensionAmounts(byAccount), ResidualDetails: residualDetails}
}

func foldAssetTrend(result domain.PeriodAnalysisResult, forced string, granularity AssetTrendGranularity, metric AssetTrendMetric) (AssetTrendResult, error) {
	if granularity != TrendDay && granularity != TrendWeek && granularity != TrendMonth {
		return AssetTrendResult{}, &domain.Error{Code: domain.ErrValidation, Field: "granularity", Message: "asset trend granularity is invalid"}
	}
	if !validTrendMetric(metric) {
		return AssetTrendResult{}, &domain.Error{Code: domain.ErrValidation, Field: "metric", Message: "asset trend metric is invalid"}
	}
	if metric == TrendReturnRate {
		return foldAssetTrendRates(result, forced, granularity)
	}
	type trendBucket struct {
		amount        decimal.Decimal
		currency      domain.CurrencyCode
		status        domain.Completeness
		hasValue      bool
		levelByDate   map[string]decimal.Decimal
		levelCurrency domain.CurrencyCode
	}
	periods := map[string]*trendBucket{}
	for _, day := range result.Days {
		period, err := assetTrendPeriod(day.Date, result.AnalysisDayTimezone, granularity)
		if err != nil {
			return AssetTrendResult{}, err
		}
		bucket := periods[period]
		if bucket == nil {
			bucket = &trendBucket{status: day.Status, levelByDate: make(map[string]decimal.Decimal)}
			periods[period] = bucket
		} else {
			bucket.status = mergeCompleteness(bucket.status, day.Status)
		}
		if day.Status == domain.CompletenessUnavailable {
			continue
		}
		value, ok := trendDayValue(day, metric)
		if !ok {
			if day.BeginningValue.Currency() == "" {
				continue
			}
			zero, zeroErr := domain.NewSignedMoney(decimal.Zero, day.BeginningValue.Currency())
			if zeroErr != nil {
				continue
			}
			value = zero
		}
		if bucket.currency == "" {
			bucket.currency = value.Currency()
		}
		if isTrendLevel(metric) {
			// A level is a point-in-time total. First sum all components for
			// each local day, then select that period's last day. This avoids
			// both averaging levels and overwriting one component with another.
			date := string(day.Date)
			bucket.levelByDate[date] = bucket.levelByDate[date].Add(value.Amount())
			bucket.levelCurrency = value.Currency()
		} else {
			bucket.amount = bucket.amount.Add(value.Amount())
			bucket.hasValue = true
		}
	}
	keys := make([]string, 0, len(periods))
	for key := range periods {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	points := make([]AssetTrendPoint, 0, len(keys))
	var summary *domain.SignedMoney
	var flowSummary decimal.Decimal
	var flowCurrency domain.CurrencyCode
	hasFlowSummary := false
	for _, key := range keys {
		bucket := periods[key]
		if isTrendLevel(metric) {
			lastDate := ""
			for date := range bucket.levelByDate {
				if lastDate == "" || date > lastDate {
					lastDate = date
				}
			}
			if lastDate != "" {
				bucket.amount = bucket.levelByDate[lastDate]
				bucket.currency = bucket.levelCurrency
				bucket.hasValue = true
			}
		}
		point := AssetTrendPoint{Period: key, AnalysisAvailability: AnalysisAvailability{Available: bucket.status != domain.CompletenessUnavailable, Status: bucket.status, ValuationForced: forcedPointer(forced)}}
		if bucket.status == domain.CompletenessPartial {
			point.MissingReason = "one or more analysis days are incomplete"
		} else if bucket.status == domain.CompletenessUnavailable {
			point.MissingReason = "no complete analysis inputs are available for this period"
		}
		if bucket.hasValue {
			point.Value = signedPointer(bucket.amount, bucket.currency)
			if isTrendLevel(metric) {
				summary = point.Value
			} else {
				flowSummary = flowSummary.Add(bucket.amount)
				flowCurrency = bucket.currency
				hasFlowSummary = true
			}
		}
		points = append(points, point)
	}
	if !isTrendLevel(metric) && hasFlowSummary {
		summary = signedPointer(flowSummary, flowCurrency)
	}
	status := availability(result, forced)
	return AssetTrendResult{AnalysisAvailability: status, Points: points, Summary: summary, Coverage: result.Coverage}, nil
}

func foldAssetTrendRates(result domain.PeriodAnalysisResult, forced string, granularity AssetTrendGranularity) (AssetTrendResult, error) {
	type rateBucket struct {
		factor    decimal.Decimal
		ratedDays int
		totalDays int
		status    domain.Completeness
	}
	periods := map[string]*rateBucket{}
	for _, daily := range result.DailyReturns {
		period, err := assetTrendPeriod(daily.Date, result.AnalysisDayTimezone, granularity)
		if err != nil {
			return AssetTrendResult{}, err
		}
		bucket := periods[period]
		if bucket == nil {
			bucket = &rateBucket{factor: decimal.NewFromInt(1), status: daily.Status}
			periods[period] = bucket
		} else {
			bucket.status = mergeCompleteness(bucket.status, daily.Status)
		}
		bucket.totalDays++
		if daily.Rate != nil && daily.Status == domain.CompletenessOK {
			bucket.factor = bucket.factor.Mul(decimal.NewFromInt(1).Add(*daily.Rate))
			bucket.ratedDays++
		}
	}
	keys := make([]string, 0, len(periods))
	for key := range periods {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	points := make([]AssetTrendPoint, 0, len(keys))
	for _, key := range keys {
		bucket := periods[key]
		if bucket.ratedDays < bucket.totalDays && bucket.status == domain.CompletenessOK {
			bucket.status = domain.CompletenessPartial
		}
		point := AssetTrendPoint{Period: key, Coverage: domain.RateCoverage{RatedDays: bucket.ratedDays, TotalDays: bucket.totalDays}, AnalysisAvailability: AnalysisAvailability{Available: bucket.status != domain.CompletenessUnavailable, Status: bucket.status, ValuationForced: forcedPointer(forced)}}
		if bucket.ratedDays > 0 && bucket.ratedDays*2 >= bucket.totalDays {
			rate := bucket.factor.Sub(decimal.NewFromInt(1))
			point.Rate = &rate
		}
		if bucket.status == domain.CompletenessPartial {
			point.MissingReason = "rate coverage is partial"
		} else if bucket.status == domain.CompletenessUnavailable {
			point.MissingReason = "no complete analysis inputs are available for this period"
		}
		points = append(points, point)
	}
	return AssetTrendResult{AnalysisAvailability: availability(result, forced), Points: points, Rate: result.ReturnRate, Coverage: result.Coverage}, nil
}

func foldCategories(result domain.PeriodAnalysisResult, forced string, categoryType AnalysisCategoryType) CategoriesResult {
	bucketSet := categoryBuckets(categoryType)
	rows := make(map[string]CategoryRow)
	var total decimal.Decimal
	var currency domain.CurrencyCode
	useReturnEffects := categoryType == CategoryDividendInterest && hasReturnComponent(result, domain.ReturnDividendInterest)
	for _, day := range result.Days {
		if useReturnEffects {
			for _, attributed := range day.AttributedEffects {
				if attributed.ReturnComponent == nil || *attributed.ReturnComponent != domain.ReturnDividendInterest {
					continue
				}
				amount := attributed.Amount.Amount()
				currency = attributed.Amount.Currency()
				total = total.Add(amount)
				rowKey, row := categoryRowForEffect(day.Component, categoryType, attributed)
				row.Key, row.Label = rowKey, rowKey
				row.Amount = signedPointer(decimalValue(row.Amount).Add(amount), currency)
				rows[rowKey] = row
			}
			continue
		}
		amount := decimal.Zero
		for bucket, value := range day.AssetBuckets {
			if bucketSet[bucket] {
				amount = amount.Add(value.Amount())
				currency = value.Currency()
			}
		}
		if amount.IsZero() {
			continue
		}
		total = total.Add(amount)
		rowKey, row := categoryRowFor(day.Component, categoryType)
		row.Key, row.Label = rowKey, rowKey
		row.Amount = signedPointer(decimalValue(row.Amount).Add(amount), currency)
		rows[rowKey] = row
	}
	resultRows := make([]CategoryRow, 0, len(rows))
	for _, row := range rows {
		resultRows = append(resultRows, row)
	}
	sort.SliceStable(resultRows, func(i, j int) bool { return signedAmountGreater(resultRows[i].Amount, resultRows[j].Amount) })
	return CategoriesResult{AnalysisAvailability: availability(result, forced), Total: signedPointerIf(total, currency), Rows: resultRows}
}

func foldCategoryDetail(result domain.PeriodAnalysisResult, forced string, categoryType AnalysisCategoryType, rowKey string) CategoryDetailResult {
	bucketSet := categoryBuckets(categoryType)
	children := make(map[string]decimal.Decimal)
	refs := make([]CategoryActivityRef, 0)
	var currency domain.CurrencyCode
	useReturnEffects := categoryType == CategoryDividendInterest && hasReturnComponent(result, domain.ReturnDividendInterest)
	for _, day := range result.Days {
		if useReturnEffects {
			for _, attributed := range day.AttributedEffects {
				if attributed.ReturnComponent == nil || *attributed.ReturnComponent != domain.ReturnDividendInterest {
					continue
				}
				key, _ := categoryRowForEffect(day.Component, categoryType, attributed)
				if key != rowKey {
					continue
				}
				currency = attributed.Amount.Currency()
				children[day.Component.Key()] = children[day.Component.Key()].Add(attributed.Amount.Amount())
				refs = append(refs, categoryActivityRef(day, attributed))
			}
			continue
		}
		key, _ := categoryRowFor(day.Component, categoryType)
		if key != rowKey {
			continue
		}
		for bucket, amount := range day.AssetBuckets {
			if !bucketSet[bucket] {
				continue
			}
			currency = amount.Currency()
			children[day.Component.Key()] = children[day.Component.Key()].Add(amount.Amount())
		}
		for _, attributed := range day.AttributedEffects {
			if attributed.AssetBucket == nil || !bucketSet[*attributed.AssetBucket] {
				continue
			}
			refs = append(refs, categoryActivityRef(day, attributed))
		}
	}
	childRows := make([]CategoryChild, 0, len(children))
	for key, amount := range children {
		accountID, instrumentID := categoryChildDimensions(result, key)
		childRows = append(childRows, CategoryChild{Key: key, Label: key, AccountID: accountID, InstrumentID: instrumentID, Amount: signedPointer(amount, currency)})
	}
	sort.SliceStable(childRows, func(i, j int) bool { return signedAmountGreater(childRows[i].Amount, childRows[j].Amount) })
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Date != refs[j].Date {
			return refs[i].Date < refs[j].Date
		}
		return refs[i].ActivityID < refs[j].ActivityID
	})
	return CategoryDetailResult{AnalysisAvailability: availability(result, forced), Children: childRows, ActivityRefs: refs}
}

func categoryChildDimensions(result domain.PeriodAnalysisResult, key string) (string, string) {
	for _, day := range result.Days {
		if day.Component.Key() != key {
			continue
		}
		accountID := day.Component.AccountID.String()
		instrumentID := ""
		if day.Component.InstrumentID != nil {
			instrumentID = day.Component.InstrumentID.String()
		}
		return accountID, instrumentID
	}
	return "", ""
}

func hasReturnComponent(result domain.PeriodAnalysisResult, wanted domain.ReturnComponent) bool {
	for _, day := range result.Days {
		for _, attributed := range day.AttributedEffects {
			if attributed.ReturnComponent != nil && *attributed.ReturnComponent == wanted {
				return true
			}
		}
	}
	return false
}

func categoryRowForEffect(component domain.ComponentID, categoryType AnalysisCategoryType, attributed domain.AttributedEffect) (string, CategoryRow) {
	if categoryType == CategoryDividendInterest && attributed.RelatedInstrumentID != nil {
		key := attributed.RelatedInstrumentID.String()
		return key, CategoryRow{InstrumentID: key, AssetClass: component.AssetClass}
	}
	return categoryRowFor(component, categoryType)
}

func categoryActivityRef(day domain.ComponentDay, attributed domain.AttributedEffect) CategoryActivityRef {
	ref := CategoryActivityRef{Date: day.Date, ActivityID: attributed.SourceEffect.ActivityID.String(), AccountID: day.Component.AccountID.String(), Amount: &attributed.Amount}
	if attributed.RelatedHoldingID != nil {
		ref.HoldingID = attributed.RelatedHoldingID.String()
	} else if day.Component.HoldingID != nil {
		ref.HoldingID = day.Component.HoldingID.String()
	}
	if attributed.RelatedInstrumentID != nil {
		ref.InstrumentID = attributed.RelatedInstrumentID.String()
	} else if day.Component.InstrumentID != nil {
		ref.InstrumentID = day.Component.InstrumentID.String()
	}
	return ref
}

func periodBeginning(result domain.PeriodAnalysisResult) (decimal.Decimal, bool) {
	amount := decimal.Zero
	has := false
	for _, day := range result.Days {
		if day.Date != result.Query.From || day.Status == domain.CompletenessUnavailable || day.BeginningValue.Currency() == "" {
			continue
		}
		amount = amount.Add(day.BeginningValue.Amount())
		has = true
	}
	return amount, has
}

func periodEnding(result domain.PeriodAnalysisResult) (decimal.Decimal, bool) {
	amount := decimal.Zero
	has := false
	for _, day := range result.Days {
		if day.Date != result.Query.To || day.Status == domain.CompletenessUnavailable {
			continue
		}
		value := day.EndingValue.Amount()
		if day.EndingValue.Currency() == "" {
			// Compatibility for deterministic callers that construct the
			// original analysis kernel shape by hand.
			value = day.BeginningValue.Amount()
			for _, bucket := range day.AssetBuckets {
				value = value.Add(bucket.Amount())
			}
		}
		amount = amount.Add(value)
		has = true
	}
	return amount, has
}

func trendDayValue(day domain.ComponentDay, metric AssetTrendMetric) (domain.SignedMoney, bool) {
	if isTrendLevel(metric) {
		value := day.EndingValue.Amount()
		currency := day.EndingValue.Currency()
		if day.EndingValue.Currency() == "" {
			value = day.BeginningValue.Amount()
			for _, bucket := range day.AssetBuckets {
				value = value.Add(bucket.Amount())
			}
			currency = day.BeginningValue.Currency()
		}
		if metric == TrendAssets || metric == TrendLiabilities {
			if metric == TrendAssets && value.IsNegative() {
				return domain.SignedMoney{}, false
			}
			if metric == TrendLiabilities {
				if !value.IsNegative() {
					return domain.SignedMoney{}, false
				}
				value = value.Neg()
			}
		}
		signed, err := domain.NewSignedMoney(value, currency)
		if err != nil {
			return domain.SignedMoney{}, false
		}
		return signed, true
	}
	if metric == TrendInvestmentReturn {
		return sumDayBuckets(day, map[domain.AttributionBucket]bool{domain.BucketPriceChange: true, domain.BucketFXImpact: true, domain.BucketDividendInterest: true})
	}
	if metric == TrendNetChange {
		return sumDayBuckets(day, allAssetBuckets())
	}
	bucket, ok := trendMetricBucket(metric)
	if !ok {
		return domain.SignedMoney{}, false
	}
	value, exists := day.AssetBuckets[bucket]
	return value, exists
}

func sumDayBuckets(day domain.ComponentDay, wanted map[domain.AttributionBucket]bool) (domain.SignedMoney, bool) {
	var result domain.SignedMoney
	found := false
	for bucket, value := range day.AssetBuckets {
		if !wanted[bucket] {
			continue
		}
		if !found {
			result = value
			found = true
		} else {
			result, _ = domain.NewSignedMoney(result.Amount().Add(value.Amount()), result.Currency())
		}
	}
	return result, found
}

func assetTrendPeriod(date domain.LocalDate, timezone string, granularity AssetTrendGranularity) (string, error) {
	if granularity == TrendDay {
		return string(date), nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", err
	}
	parsed, err := time.ParseInLocation("2006-01-02", string(date), location)
	if err != nil {
		return "", err
	}
	if granularity == TrendMonth {
		return parsed.Format("2006-01"), nil
	}
	shift := (int(parsed.Weekday()) + 6) % 7
	monday := parsed.AddDate(0, 0, -shift)
	return monday.Format("2006-01-02"), nil
}

func isTrendLevel(metric AssetTrendMetric) bool {
	return metric == TrendNetWorth || metric == TrendAssets || metric == TrendLiabilities
}

func validTrendMetric(metric AssetTrendMetric) bool {
	if isTrendLevel(metric) || metric == TrendInvestmentReturn || metric == TrendReturnRate || metric == TrendNetChange {
		return true
	}
	_, ok := trendMetricBucket(metric)
	return ok
}

func trendMetricBucket(metric AssetTrendMetric) (domain.AttributionBucket, bool) {
	switch metric {
	case TrendIncome:
		return domain.BucketIncome, true
	case TrendSpending:
		return domain.BucketSpending, true
	case TrendFees:
		return domain.BucketFee, true
	case TrendDividendInterest:
		return domain.BucketDividendInterest, true
	case TrendExternalFlow:
		return domain.BucketExternalFlow, true
	case TrendPriceChange:
		return domain.BucketPriceChange, true
	case TrendFXImpact:
		return domain.BucketFXImpact, true
	case TrendResidual:
		return domain.BucketResidual, true
	default:
		return "", false
	}
}

func categoryBuckets(categoryType AnalysisCategoryType) map[domain.AttributionBucket]bool {
	switch categoryType {
	case CategoryIncome:
		return map[domain.AttributionBucket]bool{domain.BucketIncome: true}
	case CategorySpending:
		return map[domain.AttributionBucket]bool{domain.BucketSpending: true}
	case CategoryFees:
		return map[domain.AttributionBucket]bool{domain.BucketFee: true}
	case CategoryDividendInterest:
		return map[domain.AttributionBucket]bool{domain.BucketDividendInterest: true}
	case CategoryInvestmentReturn:
		return map[domain.AttributionBucket]bool{domain.BucketPriceChange: true, domain.BucketFXImpact: true}
	default:
		return map[domain.AttributionBucket]bool{}
	}
}

func categoryRowFor(component domain.ComponentID, categoryType AnalysisCategoryType) (string, CategoryRow) {
	row := CategoryRow{}
	if categoryType == CategoryIncome || categoryType == CategorySpending || categoryType == CategoryFees {
		key := component.AccountID.String()
		row.AccountID = key
		return key, row
	}
	if component.InstrumentID != nil {
		key := component.InstrumentID.String()
		row.InstrumentID = key
		row.AssetClass = component.AssetClass
		return key, row
	}
	key := component.AssetClass
	if key == "" {
		key = domain.BucketCash
	}
	row.AssetClass = key
	return key, row
}

func mergeCompleteness(left, right domain.Completeness) domain.Completeness {
	if left == domain.CompletenessUnavailable && right == domain.CompletenessUnavailable {
		return domain.CompletenessUnavailable
	}
	if left == domain.CompletenessPartial || right == domain.CompletenessPartial || left == domain.CompletenessUnavailable || right == domain.CompletenessUnavailable {
		return domain.CompletenessPartial
	}
	return domain.CompletenessOK
}

func allAssetBuckets() map[domain.AttributionBucket]bool {
	result := make(map[domain.AttributionBucket]bool)
	for _, bucket := range orderedBuckets() {
		result[bucket] = true
	}
	return result
}

func orderedBuckets() []domain.AttributionBucket {
	return []domain.AttributionBucket{domain.BucketExternalFlow, domain.BucketIncome, domain.BucketSpending, domain.BucketDividendInterest, domain.BucketPriceChange, domain.BucketFXImpact, domain.BucketFee, domain.BucketLiabilityImpact, domain.BucketAdjustment, domain.BucketResidual}
}

func isAssetBucket(bucket domain.AttributionBucket) bool {
	for _, candidate := range orderedBuckets() {
		if candidate == bucket {
			return true
		}
	}
	return false
}

func assetBucketLabel(bucket domain.AttributionBucket) string {
	labels := map[domain.AttributionBucket]string{
		domain.BucketExternalFlow: "External flows", domain.BucketIncome: "Income", domain.BucketSpending: "Spending", domain.BucketDividendInterest: "Dividend & interest", domain.BucketPriceChange: "Price change", domain.BucketFXImpact: "FX impact", domain.BucketFee: "Fees", domain.BucketLiabilityImpact: "Liability impact", domain.BucketAdjustment: "Adjustments", domain.BucketResidual: "Unexplained difference",
	}
	if label, ok := labels[bucket]; ok {
		return label
	}
	return string(bucket)
}

func sortedDimensionAmounts(values map[string]AnalysisDimensionAmount) []AnalysisDimensionAmount {
	result := make([]AnalysisDimensionAmount, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func decimalValue(value *domain.SignedMoney) decimal.Decimal {
	if value == nil {
		return decimal.Zero
	}
	return value.Amount()
}

func signedPointer(value decimal.Decimal, currency domain.CurrencyCode) *domain.SignedMoney {
	if currency == "" {
		return nil
	}
	signed, err := domain.NewSignedMoney(value, currency)
	if err != nil {
		return nil
	}
	return &signed
}

func signedPointerIf(value decimal.Decimal, currency domain.CurrencyCode) *domain.SignedMoney {
	if value.IsZero() && currency == "" {
		return nil
	}
	return signedPointer(value, currency)
}

func validateCategoryType(categoryType AnalysisCategoryType) error {
	if len(categoryBuckets(categoryType)) == 0 {
		return fmt.Errorf("unsupported analysis category: %s", categoryType)
	}
	return nil
}
