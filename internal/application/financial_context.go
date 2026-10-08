package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Fixed whitelist DTOs are also the canonical hash encoding. Domain models,
// names/notes and conversion_json are never serialized through this boundary.
type FinancialContextRequest struct {
	AsOf       string                       `json:"asOf,omitempty"`
	Scope      FinancialContextScopeRequest `json:"scope,omitempty"`
	Disclosure string                       `json:"disclosure,omitempty"`
}
type FinancialContextScopeRequest struct {
	Kind       string   `json:"kind,omitempty"`
	AccountIDs []string `json:"accountIds,omitempty"`
}
type FinancialContextContent struct {
	SchemaVersion      string                     `json:"schemaVersion"`
	CalculationVersion string                     `json:"calculationVersion"`
	ResolverPolicy     string                     `json:"resolverPolicy"`
	Disclosure         string                     `json:"disclosure"`
	AsOf               FinancialContextAsOf       `json:"asOf"`
	Scope              FinancialContextScope      `json:"scope"`
	Basis              FinancialContextBasis      `json:"basis"`
	Summary            FinancialContextSummary    `json:"summary"`
	Coverage           FinancialContextCoverage   `json:"coverage"`
	DataAsOf           FinancialContextDataAsOf   `json:"dataAsOf"`
	Positions          []FinancialContextPosition `json:"positions"`
	Gaps               []FinancialContextGap      `json:"gaps"`
	Evidence           []FinancialContextEvidence `json:"evidence"`
}
type FinancialContextAsOf struct {
	Mode      string  `json:"mode"`
	LocalDate string  `json:"localDate,omitempty"`
	CutoffAt  string  `json:"cutoffAt,omitempty"`
	Timezone  *string `json:"timezone"`
}
type FinancialContextScope struct {
	Kind                 string   `json:"kind"`
	AccountRefs          []string `json:"accountRefs"`
	AccountCount         int      `json:"accountCount"`
	IncludedAccountCount int      `json:"includedAccountCount"`
	InclusionRule        string   `json:"inclusionRule"`
}
type FinancialContextBasis struct {
	BaseCurrency    string `json:"baseCurrency"`
	Valuation       string `json:"valuation"`
	HistoryEvidence string `json:"historyEvidence"`
	MetadataBasis   string `json:"metadataBasis"`
	QuoteTTLSeconds int64  `json:"quoteTtlSeconds"`
}
type FinancialContextAmount struct {
	Value    *string `json:"value"`
	Currency string  `json:"currency"`
	Status   string  `json:"status"`
}
type FinancialContextSummary struct {
	Assets           FinancialContextAmount `json:"assets"`
	Liabilities      FinancialContextAmount `json:"liabilities"`
	NetWorth         FinancialContextAmount `json:"netWorth"`
	KnownAssets      string                 `json:"knownAssets"`
	KnownLiabilities string                 `json:"knownLiabilities"`
}
type FinancialContextCoverage struct {
	IncludedComponents int      `json:"includedComponents"`
	ValuedComponents   int      `json:"valuedComponents"`
	MissingComponents  int      `json:"missingComponents"`
	ValuationComplete  bool     `json:"valuationComplete"`
	SnapshotHealth     string   `json:"snapshotHealth"`
	NotAssessed        []string `json:"notAssessed"`
}
type FinancialContextDataAsOf struct {
	Basis               string  `json:"basis"`
	EarliestObservation *string `json:"earliestObservation"`
	LatestObservation   *string `json:"latestObservation"`
	UnknownTimeCount    int     `json:"unknownTimeCount"`
	DateLabelCount      int     `json:"dateLabelCount"`
	StaleCount          int     `json:"staleCount"`
}
type FinancialContextPosition struct {
	Ref             string   `json:"ref"`
	ParentRef       string   `json:"parentRef,omitempty"`
	Kind            string   `json:"kind"`
	Name            string   `json:"name,omitempty"`
	Status          string   `json:"status"`
	Included        bool     `json:"included"`
	ExclusionReason string   `json:"exclusionReason,omitempty"`
	Complete        bool     `json:"complete"`
	Role            string   `json:"role"`
	Currency        string   `json:"currency,omitempty"`
	AssetClass      string   `json:"assetClass,omitempty"`
	NativeAmount    *string  `json:"nativeAmount"`
	BaseAmount      *string  `json:"baseAmount"`
	Quantity        *string  `json:"quantity"`
	Missing         []string `json:"missing"`
	EvidenceRefs    []string `json:"evidenceRefs"`
}
type FinancialContextGap struct {
	Code            string   `json:"code"`
	Severity        string   `json:"severity"`
	EntityRef       string   `json:"entityRef"`
	DependencyRef   string   `json:"dependencyRef"`
	AffectedMetrics []string `json:"affectedMetrics"`
	SuggestedAction string   `json:"suggestedAction"`
}
type FinancialContextConversion struct {
	Policy      string  `json:"policy"`
	RawPrice    string  `json:"rawPrice"`
	RawCurrency string  `json:"rawCurrency"`
	RawUnit     string  `json:"rawUnit"`
	RawQuotedAt string  `json:"rawQuotedAt"`
	FXRate      string  `json:"fxRate"`
	FXQuotedAt  *string `json:"fxQuotedAt"`
	Currency    string  `json:"currency"`
	Unit        string  `json:"unit"`
}
type FinancialContextEvidence struct {
	Ref             string                      `json:"ref"`
	Kind            string                      `json:"kind"`
	Status          string                      `json:"status"`
	SourceKind      string                      `json:"sourceKind"`
	ObservationKind string                      `json:"observationKind"`
	EffectiveAt     *string                     `json:"effectiveAt"`
	EffectiveDate   string                      `json:"effectiveDate,omitempty"`
	TimestampBasis  string                      `json:"timestampBasis"`
	Freshness       string                      `json:"freshness"`
	Value           *string                     `json:"value"`
	Currency        string                      `json:"currency,omitempty"`
	BaseCurrency    string                      `json:"baseCurrency,omitempty"`
	QuoteCurrency   string                      `json:"quoteCurrency,omitempty"`
	Conversion      *FinancialContextConversion `json:"conversion"`
}
type FinancialContextResult struct {
	CapturedAt  time.Time
	ContentHash string
	Content     FinancialContextContent
}

func (s *Service) BuildFinancialContext(ctx context.Context, in FinancialContextRequest) (FinancialContextResult, error) {
	in, ids, err := normalizeFinancialContextRequest(in)
	if err != nil {
		return FinancialContextResult{}, err
	}
	inputs, now, fxProvider, ttl, err := s.captureFinancialContext(ctx, in.AsOf != "current", ids)
	if err != nil {
		return FinancialContextResult{}, err
	}
	side, err := s.prepareFinancialContextSide(ctx, in, ids, inputs, now, fxProvider, ttl)
	if err != nil {
		return FinancialContextResult{}, err
	}
	content, err := projectFinancialContext(ctx, in, side, ttl, financialContextRefs(side.rows, in.Disclosure))
	if err != nil {
		return FinancialContextResult{}, err
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return FinancialContextResult{}, err
	}
	digest := sha256.Sum256(encoded)
	return FinancialContextResult{CapturedAt: now, ContentHash: "sha256:" + hex.EncodeToString(digest[:]), Content: content}, nil
}
func normalizeFinancialContextRequest(in FinancialContextRequest) (FinancialContextRequest, []domain.AccountID, error) {
	if in.AsOf == "" {
		in.AsOf = "current"
	}
	if in.Disclosure == "" {
		in.Disclosure = "minimal"
	}
	if in.Disclosure != "minimal" && in.Disclosure != "named" {
		return in, nil, contextValidation("disclosure must be minimal or named")
	}
	if in.Scope.Kind == "" {
		in.Scope.Kind = "household"
	}
	if in.Scope.Kind != "household" && in.Scope.Kind != "accounts" {
		return in, nil, contextValidation("scope kind must be household or accounts")
	}
	if (in.Scope.Kind == "household" && len(in.Scope.AccountIDs) > 0) || (in.Scope.Kind == "accounts" && (len(in.Scope.AccountIDs) == 0 || len(in.Scope.AccountIDs) > 100)) {
		return in, nil, contextValidation("accounts scope requires 1 to 100 account IDs; household scope takes none")
	}
	ids := []domain.AccountID{}
	seen := map[domain.AccountID]bool{}
	for _, value := range in.Scope.AccountIDs {
		id, err := domain.ParseAccountID(value)
		if err != nil {
			return in, nil, err
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	if in.AsOf != "current" {
		d, err := time.Parse("2006-01-02", in.AsOf)
		if err != nil || d.Format("2006-01-02") != in.AsOf {
			return in, nil, contextValidation("asOf must be current or a YYYY-MM-DD closed date")
		}
	}
	return in, ids, nil
}
func (s *Service) captureFinancialContext(ctx context.Context, historical bool, ids []domain.AccountID) (FinancialContextInputs, time.Time, string, time.Duration, error) {
	repository, ok := s.repository.(FinancialContextRepository)
	if !ok {
		return FinancialContextInputs{}, time.Time{}, "", 0, &domain.Error{Code: domain.ErrUnavailable, Message: "financial context read is unavailable"}
	}
	// One configuration capture, independent of per-row getters. No provider I/O.
	s.stateMu.RLock()
	nowFn, fxProvider, ttl, registry := s.now, s.fxProviderKey, s.quoteCacheTTL, s.marketData
	s.stateMu.RUnlock()
	now := nowFn()
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	if fxProvider == "" && registry != nil {
		if p, err := registry.Default(); err == nil {
			fxProvider = strings.ToLower(strings.TrimSpace(p.Key()))
		}
	}
	inputs, err := repository.ReadFinancialContextInputs(ctx, historical, ids, now)
	if err != nil {
		return FinancialContextInputs{}, time.Time{}, "", 0, err
	}
	return inputs, now, fxProvider, ttl, nil
}

type financialContextSide struct {
	portfolio domain.PortfolioSnapshot
	state     HistoricalOverviewState
	rows      []HistoricalOverviewRow
	asOf      FinancialContextAsOf
}

func (s *Service) prepareFinancialContextSide(ctx context.Context, in FinancialContextRequest, ids []domain.AccountID, inputs FinancialContextInputs, now time.Time, fxProvider string, ttl time.Duration) (financialContextSide, error) {
	historical := in.AsOf != "current"
	selected := map[domain.AccountID]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	var err error
	portfolio := inputs.Portfolio
	asOf := FinancialContextAsOf{Mode: "current"}
	cutoff := now
	if portfolio.Origin != nil {
		tz := portfolio.Origin.Timezone
		asOf.Timezone = &tz
	}
	if historical {
		origin := inputs.History.Origin
		location, loadErr := time.LoadLocation(origin.Timezone)
		if loadErr != nil {
			return financialContextSide{}, loadErr
		}
		day, _ := time.Parse("2006-01-02", in.AsOf)
		if in.AsOf < origin.StartedAt.In(location).Format("2006-01-02") || in.AsOf >= now.In(location).Format("2006-01-02") {
			return financialContextSide{}, contextValidation("select a closed day on or after the history origin")
		}
		cutoff, err = historicalOverviewDayCutoff(day, location)
		if err != nil {
			return financialContextSide{}, err
		}
		portfolio, err = (HistoricalReplay{repository: s.repository, batch: inputs.History}).Snapshot(ctx, &origin, cutoff)
		if err != nil {
			return financialContextSide{}, err
		}
		asOf.Mode, asOf.LocalDate, asOf.CutoffAt = "closed_day", in.AsOf, cutoff.UTC().Format(time.RFC3339Nano)
	}
	// Replay is complete before scope projection: transfer effects are inseparable.
	if in.Scope.Kind == "accounts" {
		records := []domain.AccountRecord{}
		for _, a := range portfolio.Accounts {
			if selected[a.Account.ID] {
				records = append(records, a)
			}
		}
		portfolio.Accounts = records
	}
	state, rows, err := s.historicalOverviewPortfolio(ctx, inputs.History, portfolio, in.AsOf, cutoff, !historical, fxProvider, ttl)
	if err != nil {
		return financialContextSide{}, err
	}
	// Include requested, not-yet-created historical accounts explicitly.
	if in.Scope.Kind == "accounts" {
		for _, id := range ids {
			found := false
			for _, r := range rows {
				if r.Key == id.String() {
					found = true
					break
				}
			}
			if !found {
				rows = append(rows, HistoricalOverviewRow{Key: id.String(), Kind: "account", Left: &HistoricalOverviewCell{Status: "not_created", Complete: true, Missing: []string{}}})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	return financialContextSide{portfolio, state, rows, asOf}, nil
}
func financialContextRefs(rows []HistoricalOverviewRow, disclosure string) map[string]string {
	refs := map[string]string{}
	accounts, components := 0, 0
	for _, row := range rows {
		if _, ok := refs[row.Key]; ok {
			continue
		}
		ref := row.Key
		if row.Kind == "account" {
			accounts++
			if disclosure == "minimal" {
				ref = fmt.Sprintf("account-%d", accounts)
			}
		} else {
			components++
			if disclosure == "minimal" {
				ref = fmt.Sprintf("position-%d", components)
			}
		}
		refs[row.Key] = ref
	}
	return refs
}
func projectFinancialContext(ctx context.Context, in FinancialContextRequest, side financialContextSide, ttl time.Duration, refs map[string]string) (FinancialContextContent, error) {
	portfolio, state, rows, asOf := side.portfolio, side.state, side.rows, side.asOf
	content := FinancialContextContent{SchemaVersion: "financial-context/1", CalculationVersion: "valuation-context/1", ResolverPolicy: domain.MarketDataResolverPolicy, Disclosure: in.Disclosure, AsOf: asOf,
		Scope:    FinancialContextScope{Kind: in.Scope.Kind, AccountRefs: []string{}, InclusionRule: "includeInNetWorth && active; active components only"},
		Basis:    FinancialContextBasis{BaseCurrency: state.Currency, Valuation: "existing_deterministic_engine", HistoryEvidence: "current_database_state", MetadataBasis: "current", QuoteTTLSeconds: int64(ttl / time.Second)},
		Summary:  FinancialContextSummary{Assets: contextAmount(state.Assets, state.Currency), Liabilities: contextAmount(state.Liabilities, state.Currency), NetWorth: contextAmount(state.NetWorth, state.Currency), KnownAssets: state.KnownAssets, KnownLiabilities: state.KnownLiabilities},
		Coverage: FinancialContextCoverage{ValuationComplete: state.Complete, SnapshotHealth: "not_assessed", NotAssessed: []string{"persistent_snapshots", "full_history_health", "provider_configuration", "real_world_ledger_completeness", "period_returns"}},
		DataAsOf: FinancialContextDataAsOf{Basis: "mixed_scope_local_observations"}, Positions: []FinancialContextPosition{}, Gaps: []FinancialContextGap{}, Evidence: []FinancialContextEvidence{}}
	if in.AsOf != "current" {
		content.Basis.HistoryEvidence = "currently_retained_corrected_facts"
	}
	for _, row := range rows {
		if row.Kind == "account" {
			content.Scope.AccountRefs = append(content.Scope.AccountRefs, refs[row.Key])
		}
	}
	evidenceByKey := map[string]FinancialContextEvidence{}
	rowEvidence := map[string][]string{}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return FinancialContextContent{}, err
		}
		cell := row.Left
		if row.Kind == "account" {
			continue
		}
		add := func(key string, e FinancialContextEvidence) {
			evidenceByKey[key] = e
			rowEvidence[row.Key] = append(rowEvidence[row.Key], key)
		}
		if row.Kind == "balance" || row.Kind == "cash" {
			e := FinancialContextEvidence{Kind: "balance", Status: "unavailable", SourceKind: "local_record", ObservationKind: "account_value", TimestampBasis: "unknown", Freshness: "unavailable", Value: cell.NativeAmount, Currency: cell.Currency}
			if row.Kind == "cash" {
				e.ObservationKind = "account_cash"
			}
			if cell.ValueSourceAt != "" {
				e.Status = "available"
				e.EffectiveAt = historicalString(cell.ValueSourceAt)
				e.TimestampBasis = "recorded_effective_at"
				e.Freshness = "recorded"
			}
			if cell.Manual {
				e.Freshness = "manual"
				e.ObservationKind = "manual_value"
			}
			key := "balance:" + row.Key + ":" + cell.ValueSourceID
			add(key, e)
		}
		for _, kind := range []string{"price", "fx"} {
			old := cell.Price
			if kind == "fx" {
				old = cell.FX
			}
			if old == nil {
				continue
			}
			e := contextQuoteEvidence(kind, old, portfolio)
			add(kind+":"+old.ID, e)
		}
	}
	evidenceKeys := make([]string, 0, len(evidenceByKey))
	for k := range evidenceByKey {
		evidenceKeys = append(evidenceKeys, k)
	}
	sort.Strings(evidenceKeys)
	evidenceRefs := map[string]string{}
	for i, key := range evidenceKeys {
		e := evidenceByKey[key]
		e.Ref = fmt.Sprintf("evidence-%d", i+1)
		evidenceRefs[key] = e.Ref
		content.Evidence = append(content.Evidence, e)
		contextObserve(&content.DataAsOf, e)
	}
	for _, row := range rows {
		c := row.Left
		p := FinancialContextPosition{Ref: refs[row.Key], ParentRef: refs[row.ParentKey], Kind: row.Kind, Status: c.Status, Included: c.Included, Complete: c.Complete, Role: row.Role, Currency: c.Currency, AssetClass: c.AssetClass, NativeAmount: c.NativeAmount, BaseAmount: c.BaseAmount, Quantity: c.Quantity, Missing: append([]string{}, c.Missing...), EvidenceRefs: []string{}}
		sort.Strings(p.Missing)
		if in.Disclosure == "named" {
			p.Name = row.Name
		}
		if !c.Included {
			p.ExclusionReason = "not_in_net_worth"
			if c.Status == "archived" || c.Status == "not_created" {
				p.ExclusionReason = c.Status
			}
		}
		if row.Kind == "account" {
			content.Scope.AccountCount++
			if c.Included {
				content.Scope.IncludedAccountCount++
			}
		} else if c.Included {
			content.Coverage.IncludedComponents++
			if c.Complete && c.BaseAmount != nil {
				content.Coverage.ValuedComponents++
			} else {
				content.Coverage.MissingComponents++
			}
		}
		for _, key := range rowEvidence[row.Key] {
			p.EvidenceRefs = append(p.EvidenceRefs, evidenceRefs[key])
		}
		sort.Strings(p.EvidenceRefs)
		content.Positions = append(content.Positions, p)
		if row.Kind == "account" {
			continue
		}
		metrics := []string{}
		if c.Included {
			metrics = []string{"assets", "netWorth"}
			if row.Role == "liability" {
				metrics = []string{"liabilities", "netWorth"}
			}
		}
		for _, missing := range p.Missing {
			code, action := contextGapCode(missing)
			dep := p.Ref
			if missing == string(domain.MissingFXRate) {
				dep = "fx:" + c.Currency + "/" + state.Currency
				if c.NativeAmount == nil {
					for _, h := range portfolio.Holdings {
						if strings.Contains(row.Key, h.ID.String()) {
							for _, instrument := range portfolio.Instruments {
								if instrument.ID == h.InstrumentID && instrument.UsesMetalConversion() && c.Currency != "USD" {
									dep = "fx:USD/" + c.Currency
								}
							}
						}
					}
				}
			}
			content.Gaps = append(content.Gaps, FinancialContextGap{Code: code, Severity: "blocking", EntityRef: p.Ref, DependencyRef: dep, AffectedMetrics: metrics, SuggestedAction: action})
		}
		for _, key := range rowEvidence[row.Key] {
			e := evidenceByKey[key]
			if e.Freshness == "stale" || e.Freshness == "manual" {
				code := "stale_observation"
				if e.Freshness == "manual" {
					code = "manual_valuation"
				}
				action := "review_market_data"
				if e.Kind == "balance" {
					action = "review_account_value"
				}
				content.Gaps = append(content.Gaps, FinancialContextGap{Code: code, Severity: "info", EntityRef: p.Ref, DependencyRef: evidenceRefs[key], AffectedMetrics: metrics, SuggestedAction: action})
			}
		}
	}
	return content, nil
}

func contextValidation(message string) error {
	return &domain.Error{Code: domain.ErrValidation, Message: message}
}
func contextAmount(value *string, currency string) FinancialContextAmount {
	status := "complete"
	if value == nil {
		status = "incomplete"
	}
	return FinancialContextAmount{Value: value, Currency: currency, Status: status}
}
func contextGapCode(kind string) (string, string) {
	switch domain.MissingInputKind(kind) {
	case domain.MissingFXRate:
		return "missing_fx", "review_market_data"
	case domain.MissingInstrumentPrice:
		return "missing_price", "review_market_data"
	case domain.MissingAccountValue:
		return "missing_account_value", "review_account_value"
	case domain.MissingHistoryCoverage:
		return "missing_coverage", "open_data_health"
	default:
		return "missing_instrument", "review_instrument"
	}
}
func contextObserve(data *FinancialContextDataAsOf, e FinancialContextEvidence) {
	contextObserveTime(data, e.EffectiveAt)
	if e.Conversion != nil {
		contextObserveTime(data, &e.Conversion.RawQuotedAt)
		if e.Conversion.Currency != "USD" {
			contextObserveTime(data, e.Conversion.FXQuotedAt)
		}
	}
	if e.TimestampBasis == "date_label" {
		data.DateLabelCount++
	}
	if e.Freshness == "stale" {
		data.StaleCount++
	}
}
func contextObserveTime(data *FinancialContextDataAsOf, value *string) {
	if value == nil {
		data.UnknownTimeCount++
		return
	}
	at, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		data.UnknownTimeCount++
		return
	}
	earliest := time.Time{}
	if data.EarliestObservation != nil {
		earliest, _ = time.Parse(time.RFC3339Nano, *data.EarliestObservation)
	}
	latest := time.Time{}
	if data.LatestObservation != nil {
		latest, _ = time.Parse(time.RFC3339Nano, *data.LatestObservation)
	}
	if data.EarliestObservation == nil || at.Before(earliest) {
		v := *value
		data.EarliestObservation = &v
	}
	if data.LatestObservation == nil || at.After(latest) {
		v := *value
		data.LatestObservation = &v
	}
}

func contextQuoteEvidence(kind string, old *HistoricalOverviewEvidence, p domain.PortfolioSnapshot) FinancialContextEvidence {
	e := FinancialContextEvidence{Kind: kind, Status: "unavailable", SourceKind: old.Source, ObservationKind: "unavailable", TimestampBasis: "unknown", Freshness: old.Freshness}
	if kind == "price" {
		for _, q := range p.InstrumentQuotes {
			if q.ID.String() != old.ID {
				continue
			}
			e.Status = "available"
			e.Value = historicalString(q.UnitPrice.Canonical())
			e.Currency = q.Currency.String()
			e.ObservationKind = contextObservationKind(q.ObservationKind)
			e.EffectiveAt = contextEvidenceTime(old.EffectiveAt)
			e.EffectiveDate = q.EffectiveDate
			e.TimestampBasis = contextTimestampBasis(q.TimestampBasis)
			e.Conversion = contextConversion(q.ConversionJSON)
			break
		}
	} else {
		for _, q := range p.FXQuotes {
			if q.ID.String() != old.ID {
				continue
			}
			e.Status = "available"
			e.Value = historicalString(q.Rate.Canonical())
			e.BaseCurrency = q.BaseCurrency.String()
			e.QuoteCurrency = q.QuoteCurrency.String()
			e.ObservationKind = contextObservationKind(q.ObservationKind)
			e.EffectiveAt = contextEvidenceTime(old.EffectiveAt)
			e.EffectiveDate = q.EffectiveDate
			e.TimestampBasis = contextTimestampBasis(q.TimestampBasis)
			break
		}
	}
	return e
}
func contextObservationKind(v string) string {
	switch v {
	case string(InstrumentObservationManual), string(InstrumentObservationRealtime), string(InstrumentObservationClose), string(InstrumentObservationLegacy), string(FXObservationLatest), string(FXObservationDailyReference), "nav":
		return v
	default:
		return "unavailable"
	}
}
func contextTimestampBasis(v string) string {
	switch v {
	case "date_label", "provider", "session_close", "quoted_at", "actual", "timestamp", "source_timestamp", "policy_derived", "observed_publication":
		return v
	default:
		return "unknown"
	}
}
func contextConversion(raw string) *FinancialContextConversion {
	var e metalConversionEvidence
	if raw == "" || json.Unmarshal([]byte(raw), &e) != nil || e.Policy != domain.MetalConversionPolicy || e.RawCurrency != "USD" || e.RawUnit != "troy_oz" || e.RawQuotedAt.IsZero() {
		return nil
	}
	if _, err := domain.ParseCurrency(e.Currency); err != nil {
		return nil
	}
	switch e.Unit {
	case "g", "troy_oz":
	default:
		return nil
	}
	price, err := domain.ParseUnitPrice(e.RawPrice)
	if err != nil {
		return nil
	}
	rate, err := domain.ParseFxRate(e.FXRate)
	if err != nil {
		return nil
	}
	c := &FinancialContextConversion{Policy: domain.MetalConversionPolicy, RawPrice: price.Canonical(), RawCurrency: "USD", RawUnit: "troy_oz", RawQuotedAt: e.RawQuotedAt.UTC().Format(time.RFC3339Nano), FXRate: rate.Canonical(), Currency: e.Currency, Unit: e.Unit}
	if e.FXQuotedAt != nil {
		v := e.FXQuotedAt.UTC().Format(time.RFC3339Nano)
		c.FXQuotedAt = &v
	}
	return c
}

func contextEvidenceTime(value string) *string {
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || at.IsZero() {
		return nil
	}
	canonical := at.UTC().Format(time.RFC3339Nano)
	return &canonical
}
