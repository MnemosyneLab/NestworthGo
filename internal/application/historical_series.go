package application

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const historicalSeriesTimeout = 10 * time.Second

var historicalSeriesLimits = domain.HistoricalSeriesLimits{MaxLeaves: 1000, MaxFacts: 100000, MaxBytes: 32 << 20}

type historicalSeriesRepository interface {
	LoadHistoricalOverviewSeriesBatch(context.Context, time.Time, domain.HistoricalSeriesLimits) (domain.HistoricalSnapshotBatch, domain.HistoricalSeriesWork, error)
}

type HistoricalSeriesCoverage struct {
	Included int `json:"included"`
	Known    int `json:"known"`
	Missing  int `json:"missing"`
}
type HistoricalSeriesMissing struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Role    string   `json:"role"`
	Reasons []string `json:"reasons"`
}
type HistoricalSeriesPoint struct {
	Date                string                    `json:"date"`
	CutoffAt            string                    `json:"cutoffAt"`
	Available           bool                      `json:"available"`
	GapReason           string                    `json:"gapReason"`
	Assets              *string                   `json:"assets"`
	Liabilities         *string                   `json:"liabilities"`
	NetWorth            *string                   `json:"netWorth"`
	KnownAssets         *string                   `json:"knownAssets"`
	KnownLiabilities    *string                   `json:"knownLiabilities"`
	AssetsComplete      bool                      `json:"assetsComplete"`
	LiabilitiesComplete bool                      `json:"liabilitiesComplete"`
	AssetCoverage       HistoricalSeriesCoverage  `json:"assetCoverage"`
	LiabilityCoverage   HistoricalSeriesCoverage  `json:"liabilityCoverage"`
	Missing             []HistoricalSeriesMissing `json:"missing"`
	ScopeChanged        bool                      `json:"scopeChanged"`
	BasisChanged        bool                      `json:"basisChanged"`
	ConnectAssets       bool                      `json:"connectAssets"`
	ConnectLiabilities  bool                      `json:"connectLiabilities"`
	ConnectNetWorth     bool                      `json:"connectNetWorth"`
}
type HistoricalOverviewSeriesResult struct {
	Overview HistoricalOverviewResult    `json:"overview"`
	Points   []HistoricalSeriesPoint     `json:"points"`
	From     string                      `json:"from"`
	To       string                      `json:"to"`
	Work     domain.HistoricalSeriesWork `json:"work"`
}

type seriesIndex struct {
	quotes     *historicalQuoteCache
	activities map[domain.AccountID][]domain.Activity
	fxPairs    map[string][]domain.FXQuote
	coverage   map[domain.InstrumentID][]domain.InstrumentHistoryCoverage
}

func indexHistoricalSeries(ctx context.Context, b *domain.HistoricalSnapshotBatch) (seriesIndex, error) {
	index := seriesIndex{quotes: &historicalQuoteCache{instrumentQuotes: map[domain.InstrumentID][]domain.InstrumentQuote{}, fxQuotes: b.FXQuoteFacts}, activities: map[domain.AccountID][]domain.Activity{}, fxPairs: map[string][]domain.FXQuote{}, coverage: map[domain.InstrumentID][]domain.InstrumentHistoryCoverage{}}
	for _, q := range b.InstrumentQuoteFacts {
		if err := ctx.Err(); err != nil {
			return index, err
		}
		index.quotes.instrumentQuotes[q.InstrumentID] = append(index.quotes.instrumentQuotes[q.InstrumentID], q)
	}
	for _, q := range b.FXQuoteFacts {
		if err := ctx.Err(); err != nil {
			return index, err
		}
		key := seriesFXPair(q.BaseCurrency, q.QuoteCurrency)
		index.fxPairs[key] = append(index.fxPairs[key], q)
	}
	for _, c := range b.InstrumentHistoryCoverage {
		index.coverage[c.InstrumentID] = append(index.coverage[c.InstrumentID], c)
	}
	holdingAccounts := map[domain.HoldingID]domain.AccountID{}
	for _, h := range b.Portfolio.Holdings {
		holdingAccounts[h.ID] = h.AccountID
	}
	for _, a := range b.Activities {
		if err := ctx.Err(); err != nil {
			return index, err
		}
		ids := map[domain.AccountID]bool{}
		for _, e := range a.Effects {
			if e.AccountID != nil {
				ids[*e.AccountID] = true
			}
			if e.HoldingID != nil {
				if id, ok := holdingAccounts[*e.HoldingID]; ok {
					ids[id] = true
				}
			}
		}
		for _, e := range a.Resulting {
			if e.AccountID != nil {
				ids[*e.AccountID] = true
			}
		}
		for id := range ids {
			index.activities[id] = append(index.activities[id], a)
		}
	}
	return index, nil
}

// One bounded transaction supplies every point and the full endpoint overview.
// It never calls the public overview, analysis or snapshot/provider services.
func (s *Service) HistoricalOverviewSeries(ctx context.Context, from, to string) (out HistoricalOverviewSeriesResult, err error) {
	a, ea := time.Parse("2006-01-02", from)
	b, eb := time.Parse("2006-01-02", to)
	if ea != nil || eb != nil || a.Format("2006-01-02") != from || b.Format("2006-01-02") != to || b.Before(a) || b.Sub(a) > 30*24*time.Hour {
		return out, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "range", Message: "select 1 to 31 forward closed civil dates"}
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, historicalSeriesTimeout)
	defer cancel()
	defer func() {
		if parentCtx.Err() != nil {
			out = HistoricalOverviewSeriesResult{}
			err = parentCtx.Err()
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			out = HistoricalOverviewSeriesResult{}
			err = &domain.Error{Code: domain.ErrHistoricalSeriesBudget, Message: "local trend exceeded its computation deadline"}
		}
	}()
	reader, ok := s.repository.(historicalSeriesRepository)
	if !ok {
		return out, &domain.Error{Code: domain.ErrUnavailable, Message: "bounded historical series reads are not supported by this repository"}
	}
	now := s.clock()
	provider, ttl := s.FXProviderKey(), s.QuoteCacheTTL()
	batch, work, err := reader.LoadHistoricalOverviewSeriesBatch(ctx, now, historicalSeriesLimits)
	if err != nil {
		return out, err
	}
	location, err := time.LoadLocation(batch.Origin.Timezone)
	if err != nil {
		return out, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "invalid history timezone"}
	}
	today := now.In(location).Format("2006-01-02")
	originDate := batch.Origin.StartedAt.In(location).Format("2006-01-02")
	if from < originDate || to >= today {
		return out, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "range", Message: "series endpoints must be closed dates on or after the history origin"}
	}
	index, err := indexHistoricalSeries(ctx, &batch)
	if err != nil {
		return out, err
	}
	out = HistoricalOverviewSeriesResult{From: from, To: to, Work: work, Points: []HistoricalSeriesPoint{}}
	out.Overview = HistoricalOverviewResult{Timezone: batch.Origin.Timezone, OriginDate: originDate, LastClosedDate: time.Date(now.In(location).Year(), now.In(location).Month(), now.In(location).Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1).Format("2006-01-02"), CapturedAt: now.UTC().Format(time.RFC3339Nano), InputGeneration: batch.InputGeneration, ResolverPolicy: domain.MarketDataResolverPolicy}
	var leftRows, rightRows []HistoricalOverviewRow
	var previousScope, previousBasis string
	for day := a; !day.After(b); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return HistoricalOverviewSeriesResult{}, err
		}
		date := day.Format("2006-01-02")
		cutoff, cutoffErr := historicalOverviewDayCutoff(day, location)
		if cutoffErr != nil {
			var problem *domain.Error
			if date != from && date != to && errors.As(cutoffErr, &problem) && problem.Field == "localDate" {
				out.Points = append(out.Points, HistoricalSeriesPoint{Date: date, GapReason: "nonexistent_civil_date", Missing: []HistoricalSeriesMissing{}})
				previousScope, previousBasis = "", ""
				continue
			}
			return HistoricalOverviewSeriesResult{}, cutoffErr
		}
		portfolio, err := (HistoricalReplay{repository: s.repository, batch: &batch, quotes: index.quotes}).Snapshot(ctx, &batch.Origin, cutoff)
		if err != nil {
			return HistoricalOverviewSeriesResult{}, err
		}
		point, state, scope, basis, selected, err := s.historicalSeriesPoint(ctx, portfolio, index, date, cutoff, provider, ttl)
		if err != nil {
			return HistoricalOverviewSeriesResult{}, err
		}
		point.ScopeChanged = previousScope != "" && previousScope != scope
		point.BasisChanged = previousBasis != "" && previousBasis != basis
		if len(out.Points) > 0 {
			prev := out.Points[len(out.Points)-1]
			same := prev.Available && !point.ScopeChanged && !point.BasisChanged
			point.ConnectAssets = same && prev.Assets != nil && point.Assets != nil
			point.ConnectLiabilities = same && prev.Liabilities != nil && point.Liabilities != nil
			point.ConnectNetWorth = same && prev.NetWorth != nil && point.NetWorth != nil
		}
		out.Points = append(out.Points, point)
		previousScope, previousBasis = scope, basis
		if date == from || date == to {
			rows, err := s.historicalSeriesEndpoint(ctx, &batch, selected, index, date, cutoff, provider, ttl)
			if err != nil {
				return HistoricalOverviewSeriesResult{}, err
			}
			// Assets-only allocations retain exactly the P1 sum/share convention.
			classes, currencies := map[string]decimal.Decimal{}, map[string]decimal.Decimal{}
			for _, row := range rows {
				cell := row.Left
				if row.Kind != "account" && row.Role == "asset" && cell.Included && cell.BaseAmount != nil {
					value, _ := decimal.NewFromString(*cell.BaseAmount)
					classes[cell.AssetClass] = classes[cell.AssetClass].Add(value)
					currencies[cell.Currency] = currencies[cell.Currency].Add(value)
				}
			}
			total, _ := decimal.NewFromString(state.KnownAssets)
			state.ByClass, state.ByCurrency = historicalAllocations(classes, total), historicalAllocations(currencies, total)
			if date == from {
				out.Overview.Left, leftRows = state, rows
			}
			if date == to {
				out.Overview.Right = &state
				rightRows = rows
			}
		}
	}
	out.Overview.Change = &HistoricalOverviewTotalsChange{Assets: historicalDifference(out.Overview.Left.Assets, out.Overview.Right.Assets), Liabilities: historicalDifference(out.Overview.Left.Liabilities, out.Overview.Right.Liabilities), NetWorth: historicalDifference(out.Overview.Left.NetWorth, out.Overview.Right.NetWorth)}
	out.Overview.Rows = alignHistoricalOverview(leftRows, rightRows, true)
	out.Overview.Categories = historicalCategoryComparisons(out.Overview)
	if err := ctx.Err(); err != nil {
		return HistoricalOverviewSeriesResult{}, err
	}
	return out, nil
}

// Resolve each instrument/pair once per day from the one prebuilt fact index.
// Account valuation then sees only the selected immutable observations.
func (s *Service) historicalSeriesPoint(ctx context.Context, p domain.PortfolioSnapshot, index seriesIndex, date string, cutoff time.Time, provider string, ttl time.Duration) (HistoricalSeriesPoint, HistoricalOverviewState, string, string, domain.PortfolioSnapshot, error) {
	point := HistoricalSeriesPoint{Date: date, CutoffAt: cutoff.UTC().Format(time.RFC3339Nano), Available: true, Missing: []HistoricalSeriesMissing{}}
	state := HistoricalOverviewState{Date: date, CutoffAt: point.CutoffAt, Currency: p.Household.BaseCurrency.String(), ByClass: []HistoricalOverviewAllocation{}, ByCurrency: []HistoricalOverviewAllocation{}}
	v := NewValuationService(s.repository, func() time.Time { return cutoff })
	v.readContext = ctx
	v.SetHistorical(true)
	v.SetHistoricalMarketDate(date)
	v.SetFXProviderKey(func() string { return provider })
	v.SetQuoteCacheTTL(func() time.Duration { return ttl })
	instruments := map[domain.InstrumentID]domain.Instrument{}
	p.InstrumentQuotes = nil
	var scope, basis []string
	for _, i := range p.Instruments {
		if err := ctx.Err(); err != nil {
			return point, state, "", "", p, err
		}
		instruments[i.ID] = i
		selection := selectHistoricalInstrumentQuoteWithCoverageAtMarketDate(i, index.quotes.instrumentQuotes[i.ID], index.coverage[i.ID], date, cutoff)
		if selection.quote != nil {
			p.InstrumentQuotes = append(p.InstrumentQuotes, *selection.quote)
		}
	}
	fx := []domain.FXQuote{}
	native := map[domain.CurrencyCode]bool{}
	for _, i := range p.Instruments {
		native[i.QuoteCurrency] = true
	}
	for _, a := range p.Accounts {
		native[a.Account.DefaultCurrency] = true
		if a.LatestValue != nil {
			native[a.LatestValue.Amount.Currency()] = true
		}
	}
	for _, c := range p.CashValues {
		native[c.Amount.Currency()] = true
	}
	for currency := range native {
		if err := ctx.Err(); err != nil {
			return point, state, "", "", p, err
		}
		if currency == p.Household.BaseCurrency {
			continue
		}
		if q := v.selectFXQuote(p, index.fxPairs[seriesFXPair(currency, p.Household.BaseCurrency)], currency, nil); q != nil {
			fx = append(fx, *q)
		}
	}
	p.FXQuotes = fx
	holdings, cash := map[domain.AccountID][]domain.Holding{}, map[domain.AccountID][]domain.AccountCashValue{}
	for _, h := range p.Holdings {
		holdings[h.AccountID] = append(holdings[h.AccountID], h)
	}
	for _, c := range p.CashValues {
		cash[c.AccountID] = append(cash[c.AccountID], c)
	}
	assets, debts := decimal.Zero, decimal.Zero
	for _, a := range p.Accounts {
		if err := ctx.Err(); err != nil {
			return point, state, "", "", p, err
		}
		if !domain.AccountEligibleForNetWorth(a.Account) {
			continue
		}
		scope = append(scope, "account:"+a.Account.ID.String())
		accountSnapshot := seriesAccountPortfolio(p, a, holdings[a.Account.ID], cash[a.Account.ID])
		item, err := v.valueAccount(accountSnapshot, a, instruments, append([]domain.Holding{}, holdings[a.Account.ID]...), cash[a.Account.ID])
		if err != nil {
			return point, state, "", "", p, err
		}
		for n, c := range item.model.Components {
			key := a.Account.ID.String() + ":balance"
			if c.HoldingID != nil {
				key = a.Account.ID.String() + ":holding:" + c.HoldingID.String()
			} else if a.Account.TrackingMode == domain.TrackingHoldings {
				key = a.Account.ID.String() + ":cash:" + c.NativeCurrency.String()
			}
			role := string(a.Account.BalanceSheetRole)
			coverage := &point.AssetCoverage
			if a.Account.IsLiability() {
				coverage = &point.LiabilityCoverage
			}
			coverage.Included++
			scope = append(scope, key+":"+role)
			basis = append(basis, key+":native:"+c.NativeCurrency.String()+":base:"+p.Household.BaseCurrency.String())
			if c.InstrumentID != nil {
				i := instruments[*c.InstrumentID]
				binding := ""
				if i.ProviderKey != nil {
					binding = *i.ProviderKey
				}
				basis = append(basis, key+":"+string(i.QuoteSource)+":"+binding+":"+i.QuoteCurrency.String()+":"+strconv.Itoa(i.ProviderBindingRevision))
			}
			preference := implicitFXPreference(p, c.NativeCurrency)
			basis = append(basis, key+":fx:"+string(preference.SourceKind))
			if c.BaseAmountExact != "" {
				value, err := decimal.NewFromString(c.BaseAmountExact)
				if err != nil {
					return point, state, "", "", p, err
				}
				coverage.Known++
				if a.Account.IsLiability() {
					debts = debts.Add(value)
				} else {
					assets = assets.Add(value)
				}
			}
			missing := item.componentMissing[n]
			if !c.Available || len(missing) > 0 || c.BaseAmountExact == "" {
				coverage.Missing++
				reasons := []string{}
				for _, m := range missing {
					reasons = appendHistoricalReason(reasons, string(m.Kind))
				}
				if len(reasons) == 0 {
					reasons = append(reasons, "account_value")
				}
				point.Missing = append(point.Missing, HistoricalSeriesMissing{Key: key, Name: a.Account.Name, Role: role, Reasons: reasons})
			}
		}
	}
	point.AssetsComplete = point.AssetCoverage.Missing == 0
	point.LiabilitiesComplete = point.LiabilityCoverage.Missing == 0
	state.KnownAssets, state.KnownLiabilities = assets.String(), debts.String()
	if point.AssetCoverage.Known > 0 || point.AssetCoverage.Included == 0 {
		point.KnownAssets = historicalString(assets.String())
	}
	if point.LiabilityCoverage.Known > 0 || point.LiabilityCoverage.Included == 0 {
		point.KnownLiabilities = historicalString(debts.String())
	}
	if point.AssetsComplete {
		point.Assets = historicalString(assets.String())
	}
	if point.LiabilitiesComplete {
		point.Liabilities = historicalString(debts.String())
	}
	if point.AssetsComplete && point.LiabilitiesComplete {
		point.NetWorth = historicalString(assets.Sub(debts).String())
	}
	state.Assets, state.Liabilities, state.NetWorth, state.Complete = point.Assets, point.Liabilities, point.NetWorth, point.AssetsComplete && point.LiabilitiesComplete
	sort.Strings(scope)
	sort.Strings(basis)
	sort.Slice(point.Missing, func(i, j int) bool { return point.Missing[i].Key < point.Missing[j].Key })
	return point, state, strings.Join(scope, "|"), strings.Join(basis, "|"), p, nil
}

func seriesAccountPortfolio(p domain.PortfolioSnapshot, a domain.AccountRecord, h []domain.Holding, c []domain.AccountCashValue) domain.PortfolioSnapshot {
	p.Accounts = []domain.AccountRecord{a}
	p.Holdings = h
	p.CashValues = c
	ids := map[domain.InstrumentID]bool{}
	for _, holding := range h {
		ids[holding.InstrumentID] = true
	}
	quotes := []domain.InstrumentQuote{}
	for _, q := range p.InstrumentQuotes {
		if ids[q.InstrumentID] {
			quotes = append(quotes, q)
		}
	}
	p.InstrumentQuotes = quotes
	return p
}
func (s *Service) historicalSeriesEndpoint(ctx context.Context, b *domain.HistoricalSnapshotBatch, p domain.PortfolioSnapshot, index seriesIndex, date string, cutoff time.Time, provider string, ttl time.Duration) ([]HistoricalOverviewRow, error) {
	rows := []HistoricalOverviewRow{}
	holdings := map[domain.AccountID][]domain.Holding{}
	cash := map[domain.AccountID][]domain.AccountCashValue{}
	for _, h := range p.Holdings {
		holdings[h.AccountID] = append(holdings[h.AccountID], h)
	}
	for _, c := range p.CashValues {
		cash[c.AccountID] = append(cash[c.AccountID], c)
	}
	for _, a := range p.Accounts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		part := *b
		part.Activities = index.activities[a.Account.ID]
		_, accountRows, err := s.historicalOverviewPortfolio(ctx, &part, seriesAccountPortfolio(p, a, holdings[a.Account.ID], cash[a.Account.ID]), date, cutoff, false, provider, ttl)
		if err != nil {
			return nil, err
		}
		rows = append(rows, accountRows...)
	}
	return rows, nil
}

func seriesFXPair(a, b domain.CurrencyCode) string {
	if a > b {
		a, b = b, a
	}
	return a.String() + ":" + b.String()
}
