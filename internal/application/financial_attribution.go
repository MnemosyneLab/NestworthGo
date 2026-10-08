package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// A new comparison is captured after snapshot maintenance. This link never
// certifies an older comparisonId, or joins a live analysis to a frozen package.
type FinancialAttributionLink struct {
	SchemaVersion       string                        `json:"schemaVersion"`
	Status              string                        `json:"status"`
	AssetStatus         string                        `json:"assetStatus"`
	Available           bool                          `json:"available"`
	MissingReason       string                        `json:"missingReason,omitempty"`
	ResidualIssueCount  int                           `json:"residualIssueCount"`
	MismatchReasons     []string                      `json:"mismatchReasons"`
	Basis               string                        `json:"basis"`
	BasisHash           string                        `json:"basisHash,omitempty"`
	Period              *FinancialAttributionPeriod   `json:"period"`
	Scope               FinancialContextScope         `json:"scope"`
	ScopeBasis          string                        `json:"scopeBasis"`
	LeftScope           FinancialContextScope         `json:"leftScope"`
	RightScope          FinancialContextScope         `json:"rightScope"`
	BeginningValue      FinancialContextAmount        `json:"beginningValue"`
	EndingValue         FinancialContextAmount        `json:"endingValue"`
	AnalysisDelta       FinancialContextAmount        `json:"analysisDelta"`
	ExplainedDelta      FinancialContextAmount        `json:"explainedDelta"`
	Residual            FinancialContextAmount        `json:"residual"`
	PrecisionAdjustment FinancialContextAmount        `json:"precisionAdjustment"`
	Precision           FinancialAttributionPrecision `json:"precision"`
	Drivers             []FinancialAttributionDriver  `json:"drivers"`
	InvestmentReturn    *FinancialAttributionReturn   `json:"investmentReturn"`
}
type FinancialAttributionPrecision struct {
	AmountScale        int                    `json:"amountScale"`
	Rounding           string                 `json:"rounding"`
	BoundaryBasis      string                 `json:"boundaryBasis"`
	DriverBasis        string                 `json:"driverBasis"`
	BoundaryAdjustment FinancialContextAmount `json:"boundaryAdjustment"`
	DriverAdjustment   FinancialContextAmount `json:"driverAdjustment"`
}
type FinancialAttributionPeriod struct {
	From           string `json:"from"`
	To             string `json:"to"`
	Timezone       string `json:"timezone"`
	StartExclusive string `json:"startExclusive"`
	EndInclusive   string `json:"endInclusive"`
}
type FinancialAttributionDriver struct {
	Key    string                 `json:"key"`
	Amount FinancialContextAmount `json:"amount"`
}
type FinancialAttributionReturn struct {
	Basis         string                       `json:"basis"`
	IncludeCash   bool                         `json:"includeCash"`
	Amount        FinancialContextAmount       `json:"amount"`
	Rate          *string                      `json:"rate"`
	Status        string                       `json:"status"`
	Available     bool                         `json:"available"`
	MissingReason string                       `json:"missingReason,omitempty"`
	RatedDays     int                          `json:"ratedDays"`
	TotalDays     int                          `json:"totalDays"`
	Sources       []FinancialAttributionDriver `json:"sources"`
}

// BuildFinancialAttribution reuses the application write coordinator and the
// existing snapshot/replay/analysis engines. MCP holds this same reentrant
// permit through cache publication, so restore cannot republish a stale build.
func (s *Service) BuildFinancialAttribution(ctx context.Context, in FinancialComparisonRequest) (result FinancialComparisonResult, err error) {
	err = s.WithWrite(ctx, func(ctx context.Context) error {
		var buildErr error
		result, buildErr = s.buildFinancialAttribution(ctx, in)
		return buildErr
	})
	return
}
func (s *Service) buildFinancialAttribution(ctx context.Context, in FinancialComparisonRequest) (FinancialComparisonResult, error) {
	if err := ctx.Err(); err != nil {
		return FinancialComparisonResult{}, err
	}
	if in.LeftAsOf == "" || in.LeftAsOf == "current" || in.RightAsOf == "" {
		return FinancialComparisonResult{}, contextValidation("leftAsOf requires a closed date; rightAsOf requires a closed date or current")
	}
	left, ids, err := normalizeFinancialContextRequest(FinancialContextRequest{AsOf: in.LeftAsOf, Scope: in.Scope, Disclosure: in.Disclosure})
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	right, _, err := normalizeFinancialContextRequest(FinancialContextRequest{AsOf: in.RightAsOf, Scope: in.Scope, Disclosure: in.Disclosure})
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	if in.RightAsOf != "current" && in.LeftAsOf >= in.RightAsOf {
		return FinancialComparisonResult{}, contextValidation("attribution requires leftAsOf before rightAsOf")
	}
	// Validate history and cutoffs before any maintenance. Unsupported scopes and
	// current endpoints still return a comparison, but never fabricated returns.
	inputs, now, provider, ttl, err := s.captureFinancialContext(ctx, true, ids)
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	for _, request := range []FinancialContextRequest{left, right} {
		if _, err := s.prepareFinancialContextSide(ctx, request, ids, inputs, now, provider, ttl); err != nil {
			return FinancialComparisonResult{}, err
		}
	}
	unsupported := len(ids) > 1
	current := right.AsOf == "current"
	boundaryUnsupported := !current && !attributionBoundariesSupported(left.AsOf, right.AsOf, inputs.History.Origin.Timezone)
	if !unsupported && !current && !boundaryUnsupported {
		if err := s.ensureAttributionSnapshots(ctx, inputs.History.Origin.HouseholdID, left.AsOf, right.AsOf); err != nil {
			return FinancialComparisonResult{}, err
		}
		// Maintenance can invalidate analysis memos. Only now capture the immutable
		// facts/configuration used by both projections; do not use a previous memo.
		inputs, now, provider, ttl, err = s.captureFinancialContext(ctx, true, ids)
		if err != nil {
			return FinancialComparisonResult{}, err
		}
	}
	comparison, err := s.buildFinancialComparison(ctx, left, right, ids, inputs, now, provider, ttl)
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	c := &comparison.Content
	currency := c.Left.Basis.BaseCurrency
	link := &FinancialAttributionLink{
		SchemaVersion: "financial-attribution/1", Status: "unavailable", AssetStatus: "unavailable", MismatchReasons: []string{},
		Basis: "fresh_recapture_after_derived_snapshot_maintenance; currently_retained_corrected_facts; current_metadata; historical_inclusion_must_match_current_analysis_universe; base_valuation; analysis_money_4_place_half_even; explainedDelta_excludes_residual; precision_adjustment_is_not_return_or_residual; net_worth_change_is_not_investment_return",
		Scope: financialAttributionScope(*c), ScopeBasis: "endpoint_account_union; included_when_either_endpoint_included; shared_comparison_refs; endpoint_scopes_preserved", LeftScope: c.Left.Scope, RightScope: c.Right.Scope,
		BeginningValue: contextAmount(nil, currency), EndingValue: contextAmount(nil, currency), AnalysisDelta: contextAmount(nil, currency), ExplainedDelta: contextAmount(nil, currency), Residual: contextAmount(nil, currency), PrecisionAdjustment: contextAmount(nil, currency), Drivers: []FinancialAttributionDriver{},
		Precision: FinancialAttributionPrecision{AmountScale: assetProjectionPrecision, Rounding: "half_even", BoundaryBasis: "signed_component_daily_boundary_rounded_then_summed", DriverBasis: "exact_component_day_buckets_aggregated_by_period_driver_then_existing_waterfall_rounding_and_reconciliation", BoundaryAdjustment: contextAmount(nil, currency), DriverAdjustment: contextAmount(nil, currency)},
	}
	c.Attribution = link
	finish := func(status string, reasons ...string) (FinancialComparisonResult, error) {
		link.Status, link.MismatchReasons = status, reasons
		if len(reasons) == 0 {
			link.MismatchReasons = []string{}
		}
		return hashFinancialComparison(now, *c)
	}
	if unsupported {
		return finish("unavailable", "unsupported_account_set")
	}
	if current {
		return finish("unavailable", "right_endpoint_not_closed")
	}
	day, _ := time.Parse("2006-01-02", left.AsOf)
	from := day.AddDate(0, 0, 1).Format("2006-01-02")
	link.Period = &FinancialAttributionPeriod{From: from, To: right.AsOf, Timezone: inputs.History.Origin.Timezone, StartExclusive: c.Left.AsOf.CutoffAt, EndInclusive: c.Right.AsOf.CutoffAt}
	if boundaryUnsupported {
		return finish("incompatible", "historical_boundary_unsupported")
	}
	if len(ids) == 1 {
		for _, position := range c.Positions {
			if position.Kind == "account" && (position.Left == nil || position.Right == nil || position.Left.Status == "not_created" || position.Right.Status == "not_created") {
				return finish("unavailable", "account_endpoint_absent")
			}
		}
	}
	query := domain.AnalysisQuery{From: from, To: right.AsOf, Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment}
	if len(ids) == 1 {
		query.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: ids[0].String()}
	}
	start, _ := time.Parse("2006-01-02", left.AsOf)
	end, _ := time.Parse("2006-01-02", right.AsOf)
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, inputs.History.Origin.HouseholdID, start, end)
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	// Fresh replay evidence, not equality of aggregate amounts, proves the stored
	// snapshot basis. Also fence external repository writes that bypass this app.
	state, err := s.repository.DailySnapshotState(ctx, inputs.History.Origin.HouseholdID)
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	if state.InputGeneration != inputs.History.InputGeneration {
		return finish("incompatible", "source_revision_changed")
	}
	basisHash, reason, err := s.validateAttributionSnapshots(ctx, inputs, snapshots, start, end, ids, now, provider, ttl)
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	if reason != "" {
		if reason == "missing_snapshot_boundary" {
			return finish("unavailable", reason)
		}
		return finish("incompatible", reason)
	}
	link.BasisHash = basisHash
	input := AnalysisInputs{Origin: inputs.History.Origin, Portfolio: inputs.Portfolio, Snapshots: snapshots, Activities: inputs.History.Activities, InstrumentQuotes: inputs.History.InstrumentQuoteFacts, FXQuotes: inputs.History.FXQuoteFacts}
	result, err := ComputeAnalysis(input, query)
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	status, reasons, err := projectFinancialAttribution(link, c, result, input)
	if err != nil {
		return FinancialComparisonResult{}, err
	}
	return finish(status, reasons...)
}

// Project only already disclosed endpoint scopes/positions. No identity lookup
// or directory read is needed to describe an account entering the period.
func financialAttributionScope(c FinancialComparisonContent) FinancialContextScope {
	scope := FinancialContextScope{Kind: c.Left.Scope.Kind, InclusionRule: c.Left.Scope.InclusionRule, AccountRefs: []string{}}
	seen := map[string]bool{}
	for _, side := range []FinancialContextScope{c.Left.Scope, c.Right.Scope} {
		for _, ref := range side.AccountRefs {
			if !seen[ref] {
				seen[ref] = true
				scope.AccountRefs = append(scope.AccountRefs, ref)
			}
		}
	}
	sort.Strings(scope.AccountRefs)
	scope.AccountCount = len(scope.AccountRefs)
	for _, position := range c.Positions {
		if position.Kind == "account" && seen[position.Ref] && ((position.Left != nil && position.Left.Included) || (position.Right != nil && position.Right.Included)) {
			scope.IncludedAccountCount++
		}
	}
	return scope
}

func projectFinancialAttribution(link *FinancialAttributionLink, c *FinancialComparisonContent, result domain.PeriodAnalysisResult, input AnalysisInputs) (string, []string, error) {
	currency := c.Left.Basis.BaseCurrency
	finish := func(status string, reasons ...string) (string, []string, error) { return status, reasons, nil }
	asset := foldAssetChange(result, "")
	link.AssetStatus, link.Available, link.MissingReason, link.ResidualIssueCount = string(asset.Status), asset.Available, asset.MissingReason, asset.ResidualIssueCount
	link.BeginningValue = attributionAmount(asset.Summary.BeginningValue, currency, asset.Status)
	link.EndingValue = attributionAmount(asset.Summary.EndingValue, currency, asset.Status)
	for _, row := range asset.Waterfall {
		link.Drivers = append(link.Drivers, FinancialAttributionDriver{row.Key, attributionAmount(row.Amount, currency, asset.Status)})
	}
	returns, err := projectReturnTrend(result, "", ReturnTrendPeriodReturnAmount)
	if err != nil {
		return "", nil, err
	}
	link.InvestmentReturn = &FinancialAttributionReturn{Basis: "investment_universe; excludes_cash; geometric_linked_rate; not_net_worth_change", Amount: attributionAmount(returns.Amount, currency, returns.Status), Status: string(returns.Status), Available: returns.Available, MissingReason: returns.MissingReason, RatedDays: returns.Coverage.RatedDays, TotalDays: returns.Coverage.TotalDays, Sources: []FinancialAttributionDriver{}}
	if returns.Rate != nil {
		link.InvestmentReturn.Rate = historicalString(returns.Rate.String())
	}
	for _, source := range returns.Sources {
		link.InvestmentReturn.Sources = append(link.InvestmentReturn.Sources, FinancialAttributionDriver{source.Key, attributionAmount(source.Amount, currency, returns.Status)})
	}
	if !asset.Available && c.Left.Coverage.ValuationComplete && c.Right.Coverage.ValuationComplete {
		return finish("unavailable", "empty_analysis_universe")
	}
	for _, day := range result.Days {
		if day.BeginningValue.Currency() == "" || day.EndingValue.Currency() == "" {
			return finish("unavailable", "missing_valuation_evidence")
		}
	}
	if !c.Left.Coverage.ValuationComplete || !c.Right.Coverage.ValuationComplete || !asset.Available {
		return finish("unavailable", "missing_valuation_evidence")
	}
	proof, reason, err := proveAttributionPrecision(input, result, c)
	if err != nil {
		return "", nil, err
	}
	if reason == "" && (!sameAttributionAmount(link.BeginningValue, contextAmount(historicalString(proof.beginning.String()), currency)) || !sameAttributionAmount(link.EndingValue, contextAmount(historicalString(proof.ending.String()), currency))) {
		reason = "analysis_endpoint_mismatch"
	}
	if reason != "" {
		// The endpoint check is an additional guard after proof of all day inputs.
		link.Drivers, link.InvestmentReturn = []FinancialAttributionDriver{}, nil
		return finish("incompatible", reason)
	}
	explained, residual := decimal.Zero, decimal.Zero
	for _, row := range asset.Waterfall {
		if row.Amount == nil {
			return finish("unavailable", "missing_driver_evidence")
		}
		if row.Bucket == domain.BucketResidual {
			residual = residual.Add(row.Amount.Amount())
		} else {
			explained = explained.Add(row.Amount.Amount())
		}
	}
	// Prove the published driver values are the existing engine's deterministic
	// projection before attributing any difference to monetary precision.
	exactWaterfall, _ := aggregateAssetWaterfall(result)
	projectedWaterfall := reconcileAssetWaterfall(exactWaterfall, asset.Summary.Change)
	projectedTotal := decimal.Zero
	for _, amount := range projectedWaterfall {
		projectedTotal = projectedTotal.Add(amount)
	}
	if !explained.Add(residual).Equal(projectedTotal) || asset.Summary.Change == nil || !asset.Summary.Change.Amount().Equal(proof.ending.Sub(proof.beginning)) {
		link.Drivers, link.InvestmentReturn = []FinancialAttributionDriver{}, nil
		return finish("incompatible", "driver_reconciliation_mismatch")
	}
	analysisDelta := proof.ending.Sub(proof.beginning)
	boundaryAdjustment := proof.exactDelta.Sub(analysisDelta)
	driverAdjustment := analysisDelta.Sub(projectedTotal)
	precisionAdjustment := boundaryAdjustment.Add(driverAdjustment)
	if !explained.Add(residual).Add(precisionAdjustment).Equal(proof.exactDelta) {
		link.Drivers, link.InvestmentReturn = []FinancialAttributionDriver{}, nil
		return finish("incompatible", "driver_reconciliation_mismatch")
	}
	link.AnalysisDelta = contextAmount(historicalString(analysisDelta.String()), currency)
	link.ExplainedDelta, link.Residual = contextAmount(historicalString(explained.String()), currency), contextAmount(historicalString(residual.String()), currency)
	link.Precision.BoundaryAdjustment = contextAmount(historicalString(boundaryAdjustment.String()), currency)
	link.Precision.DriverAdjustment = contextAmount(historicalString(driverAdjustment.String()), currency)
	link.PrecisionAdjustment = contextAmount(historicalString(precisionAdjustment.String()), currency)
	return finish("compatible")
}

func attributionAmount(money *domain.SignedMoney, currency string, status domain.Completeness) FinancialContextAmount {
	amount := contextAmount(nil, currency)
	if money != nil {
		amount = contextAmount(historicalString(money.Amount().String()), money.Currency().String())
		if status != domain.CompletenessOK {
			amount.Status = string(status)
		}
	}
	return amount
}
func sameAttributionAmount(a, b FinancialContextAmount) bool {
	if a.Value == nil || b.Value == nil || a.Currency != b.Currency {
		return false
	}
	x, e1 := decimal.NewFromString(*a.Value)
	y, e2 := decimal.NewFromString(*b.Value)
	return e1 == nil && e2 == nil && x.Equal(y)
}

// Uses the existing valuation/replay and canonical snapshot evidence helpers.
// Private observation IDs participate in the digest, but never enter the wire.
func (s *Service) validateAttributionSnapshots(ctx context.Context, inputs FinancialContextInputs, snapshots []domain.DailyValuationSnapshot, start, end time.Time, ids []domain.AccountID, now time.Time, provider string, ttl time.Duration) (string, string, error) {
	batch := inputs.History
	location, err := time.LoadLocation(batch.Origin.Timezone)
	if err != nil {
		return "", "", err
	}
	// Read-only builder has immutable configuration; it takes no separate gate.
	builder := &Service{repository: s.repository, now: func() time.Time { return now }, fxProviderKey: provider, quoteCacheTTL: ttl}
	ctx = context.WithValue(ctx, historicalSnapshotBatchKey{}, batch)
	quotes := &historicalQuoteCache{instrumentQuotes: map[domain.InstrumentID][]domain.InstrumentQuote{}, fxQuotes: batch.FXQuoteFacts}
	for _, quote := range batch.InstrumentQuoteFacts {
		quotes.instrumentQuotes[quote.InstrumentID] = append(quotes.instrumentQuotes[quote.InstrumentID], quote)
	}
	ctx = context.WithValue(ctx, historicalQuoteCacheKey{}, quotes)
	byDate := map[string]domain.DailyValuationSnapshot{}
	for _, snapshot := range snapshots {
		byDate[snapshot.LocalDate] = snapshot
	}
	selected := func(id domain.AccountID) bool { return len(ids) == 0 || id == ids[0] }
	currentEligibility := map[domain.AccountID]bool{}
	for _, record := range inputs.Portfolio.Accounts {
		if selected(record.Account.ID) {
			currentEligibility[record.Account.ID] = domain.AccountEligibleForNetWorth(record.Account)
		}
	}
	fingerprints := []string{}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		date := day.Format("2006-01-02")
		cutoff, err := historicalOverviewDayCutoff(day, location)
		if err != nil {
			return "", "historical_boundary_unsupported", nil
		}
		snapshot, ok := byDate[date]
		if !ok {
			return "", "missing_snapshot_boundary", nil
		}
		if !snapshot.CutoffAt.Equal(cutoff) {
			return "", "snapshot_cutoff_mismatch", nil
		}
		if snapshot.ResolverPolicyVersion != domain.MarketDataResolverPolicy || batch.ResolverPolicyVersion != domain.MarketDataResolverPolicy {
			return "", "resolver_policy_mismatch", nil
		}
		portfolio, err := builder.historicalPortfolioSnapshot(ctx, &batch.Origin, cutoff)
		if err != nil {
			return "", "", err
		}
		for _, record := range portfolio.Accounts {
			if selected(record.Account.ID) && domain.AccountEligibleForNetWorth(record.Account) != currentEligibility[record.Account.ID] {
				return "", "inclusion_mismatch", nil
			}
		}
		fresh, err := builder.valueHistoricalSnapshot(ctx, &batch.Origin, cutoff, date)
		if err != nil {
			return "", "", err
		}
		if snapshot.Complete != fresh.Complete {
			return "", "snapshot_evidence_mismatch", nil
		}
		fingerprint := scopedAttributionSnapshotHash(snapshot, selected)
		if fingerprint != scopedAttributionSnapshotHash(fresh, selected) {
			return "", "snapshot_evidence_mismatch", nil
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	encoded, err := json.Marshal(fingerprints)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), "", nil
}
func scopedAttributionSnapshotHash(snapshot domain.DailyValuationSnapshot, selected func(domain.AccountID) bool) string {
	items := []string{}
	for _, item := range snapshot.Items {
		if selected(item.AccountID) {
			items = append(items, snapshotItemSortKey(item)+"|"+string(item.ClassificationBasis))
		}
	}
	sort.Strings(items)
	encoded, _ := json.Marshal(struct {
		Date, Cutoff, Currency string
		Items                  []string
	}{snapshot.LocalDate, snapshot.CutoffAt.UTC().Format(time.RFC3339Nano), snapshot.Currency.String(), items})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// The comparison resolver supports civil closes at unusual IANA transitions.
// The existing analysis engine and materializer still need unambiguous local
// midnights. Refuse that unsupported overlap rather than inventing a period.
func attributionBoundariesSupported(left, right, timezone string) bool {
	start, _ := time.Parse("2006-01-02", left)
	end, _ := time.Parse("2006-01-02", right)
	for day := start; !day.After(end.AddDate(0, 0, 1)); day = day.AddDate(0, 0, 1) {
		if _, err := domain.ResolveLocalDateTime(day.Format("2006-01-02"), "00:00", timezone); err != nil {
			return false
		}
	}
	return true
}
