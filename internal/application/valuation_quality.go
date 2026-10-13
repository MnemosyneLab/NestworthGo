package application

import (
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"sort"
	"time"
)

// One entry per included leaf, irrespective of missing-reason count. Project
// the existing exact amounts; never select quotes or feed the return kernel.
type displayAccumulator struct {
	quality   domain.ValuationQuality
	sum       decimal.Decimal
	uncertain bool
}

func (a *displayAccumulator) reason(r domain.ValuationReason) {
	for _, old := range a.quality.Reasons {
		if old == r {
			return
		}
	}
	a.quality.Reasons = append(a.quality.Reasons, r)
}
func (a *displayAccumulator) add(c domain.ValuationComponent, missing []domain.MissingInputView, target string, current bool) error {
	a.quality.IncludedCount++
	available := c.Available && c.BaseAmountExact != ""
	provisional, carried, stale := false, false, false
	for _, m := range missing {
		switch m.Kind {
		case domain.MissingHistoryCoverage:
			provisional = true
			a.reason(domain.ReasonHistoryUnverified)
		case domain.MissingInstrumentPrice, domain.MissingInstrument:
			available = false
			a.reason(domain.ReasonPriceMissing)
		case domain.MissingFXRate:
			available = false
			a.reason(domain.ReasonFXMissing)
		case domain.MissingAccountValue:
			available = false
			a.reason(domain.ReasonAccountValueMissing)
		}
	}
	for n, e := range []*domain.QuoteEvidenceView{c.PriceEvidence, c.FXEvidence} {
		if e == nil {
			continue
		}
		if current && e.Source != domain.QuoteSourceManual {
			provisional = true
		}
		date := e.EffectiveDate
		if date == "" && !e.ValueEffectiveAt.IsZero() {
			date = e.ValueEffectiveAt.Format("2006-01-02")
		}
		if date == "" && !e.QuotedAt.IsZero() {
			date = e.QuotedAt.Format("2006-01-02")
		}
		if date != "" {
			if a.quality.OldestMarketDate == nil || date < *a.quality.OldestMarketDate {
				d := date
				a.quality.OldestMarketDate = &d
			}
			if a.quality.NewestMarketDate == nil || date > *a.quality.NewestMarketDate {
				d := date
				a.quality.NewestMarketDate = &d
			}
			if target != "" && date < target && e.Source != domain.QuoteSourceManual {
				carried = true
				if n == 0 {
					a.reason(domain.ReasonCarriedPrice)
					if c.InstrumentType == domain.InstrumentMutualFund || c.InstrumentType == domain.InstrumentBankInvestmentProduct {
						a.reason(domain.ReasonCarriedNAV)
					}
				} else {
					a.reason(domain.ReasonCarriedFX)
				}
			}
		}
		stale = stale || e.Freshness == domain.FreshnessStale
	}
	if available {
		value, err := decimal.NewFromString(c.BaseAmountExact)
		if err != nil {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "display valuation amount is invalid"}
		}
		a.sum = a.sum.Add(value)
		a.quality.ValuedCount++
	} else {
		a.quality.UnpricedCount++
		if len(missing) == 0 {
			a.reason(domain.ReasonEvidenceUnavailable)
		}
	}
	if provisional {
		a.quality.ProvisionalCount++
	}
	if carried {
		a.quality.CarriedCount++
	}
	if stale {
		a.quality.StaleCount++
	}
	return nil
}
func (a displayAccumulator) amount(currency domain.CurrencyCode, signed bool) (domain.ValuationAmount, error) {
	q := a.quality
	q.Availability = domain.ValuationFull
	q.Coverage = domain.CoverageVerified
	if q.UnpricedCount > 0 {
		q.Availability = domain.ValuationPartial
		q.Coverage = domain.CoverageProvisional
	}
	if q.IncludedCount > 0 && q.ValuedCount == 0 {
		q.Availability = domain.ValuationNone
		q.Coverage = domain.CoverageUnavailable
	}
	if q.ProvisionalCount > 0 && q.Coverage == domain.CoverageVerified {
		q.Coverage = domain.CoverageProvisional
	}
	q.Reasons = append([]domain.ValuationReason{}, q.Reasons...)
	sort.Slice(q.Reasons, func(i, j int) bool { return q.Reasons[i] < q.Reasons[j] })
	if a.uncertain && q.Availability == domain.ValuationFull {
		q.Availability = domain.ValuationPartial
		q.Coverage = domain.CoverageProvisional
		q.Reasons = append(q.Reasons, domain.ReasonEvidenceUnavailable)
	}
	out := domain.ValuationAmount{Currency: currency, Quality: q}
	if q.ValuedCount > 0 || q.IncludedCount == 0 {
		var value domain.MoneyView
		if signed {
			m, err := domain.NewSignedMoney(a.sum, currency)
			if err != nil {
				return out, err
			}
			value = domain.MoneyView{Amount: m.CanonicalAmount(), Currency: currency}
		} else {
			m, err := domain.NewMoney(a.sum, currency)
			if err != nil {
				return out, err
			}
			value = domain.MoneyView{Amount: m.CanonicalAmount(), Currency: currency}
		}
		out.KnownSubtotal = &value
		if q.Availability == domain.ValuationFull {
			out.Total = &value
		}
	}
	return out, nil
}
func householdDisplay(assets, liabilities displayAccumulator, currency domain.CurrencyCode) (domain.HouseholdDisplayValuation, error) {
	a, err := assets.amount(currency, false)
	if err != nil {
		return domain.HouseholdDisplayValuation{}, err
	}
	l, err := liabilities.amount(currency, false)
	if err != nil {
		return domain.HouseholdDisplayValuation{}, err
	}
	n := assets
	n.uncertain = assets.uncertain || liabilities.uncertain
	n.sum = assets.sum.Sub(liabilities.sum)
	n.quality.IncludedCount += liabilities.quality.IncludedCount
	n.quality.ValuedCount += liabilities.quality.ValuedCount
	n.quality.UnpricedCount += liabilities.quality.UnpricedCount
	n.quality.ProvisionalCount += liabilities.quality.ProvisionalCount
	n.quality.CarriedCount += liabilities.quality.CarriedCount
	n.quality.StaleCount += liabilities.quality.StaleCount
	n.quality.Reasons = append([]domain.ValuationReason{}, assets.quality.Reasons...)
	for _, r := range liabilities.quality.Reasons {
		n.reason(r)
	}
	if l.Quality.OldestMarketDate != nil && (n.quality.OldestMarketDate == nil || *l.Quality.OldestMarketDate < *n.quality.OldestMarketDate) {
		n.quality.OldestMarketDate = l.Quality.OldestMarketDate
	}
	if l.Quality.NewestMarketDate != nil && (n.quality.NewestMarketDate == nil || *l.Quality.NewestMarketDate > *n.quality.NewestMarketDate) {
		n.quality.NewestMarketDate = l.Quality.NewestMarketDate
	}
	net, err := n.amount(currency, true)
	return domain.HouseholdDisplayValuation{Assets: a, Liabilities: l, NetWorth: net}, err
}
func displayDate(p domain.PortfolioSnapshot, now time.Time) string {
	if p.Origin != nil {
		if loc, err := time.LoadLocation(p.Origin.Timezone); err == nil {
			return now.In(loc).Format("2006-01-02")
		}
	}
	return now.Format("2006-01-02")
}

// Translate legacy persisted text at this boundary only.
func coverageOnlySnapshotItem(item domain.DailyValuationSnapshotItem) bool {
	return !item.Complete && snapshotItemBaseAmountString(item) != "" && item.MissingReason != nil && *item.MissingReason == "historical market-data coverage is unverified"
}

func currentHouseholdDisplay(accounts []domain.AccountValuation, p domain.PortfolioSnapshot, now time.Time) (domain.HouseholdDisplayValuation, error) {
	var assets, liabilities displayAccumulator
	for _, account := range accounts {
		if !domain.AccountEligibleForNetWorth(account.Account) {
			continue
		}
		for _, c := range account.Components {
			target := &assets
			if account.Account.IsLiability() {
				target = &liabilities
			}
			if err := target.add(c, c.MissingInputs, displayDate(p, now), true); err != nil {
				return domain.HouseholdDisplayValuation{}, err
			}
		}
	}
	return householdDisplay(assets, liabilities, p.Household.BaseCurrency)
}
func currentPortfolioDisplay(accounts []domain.AccountValuation, p domain.PortfolioSnapshot, now time.Time) (domain.ValuationAmount, error) {
	var a displayAccumulator
	for _, account := range accounts {
		for _, c := range account.Components {
			if c.InstrumentID == nil {
				continue
			}
			if err := a.add(c, c.MissingInputs, displayDate(p, now), true); err != nil {
				return domain.ValuationAmount{}, err
			}
		}
	}
	return a.amount(p.Household.BaseCurrency, false)
}

type balanceDisplayKey struct {
	currency  domain.CurrencyCode
	amount    string
	liability bool
}
type balanceDisplayValue struct {
	view  domain.ValuationAmount
	exact decimal.Decimal
}
type snapshotDisplayEvidence struct {
	balances            map[balanceDisplayKey]balanceDisplayValue
	priceBasis, fxBasis map[string][]string
	prices, fx          map[string]domain.QuoteEvidenceView
	liabilities         map[domain.AccountID]bool
	instruments         map[domain.InstrumentID]domain.InstrumentType
}

func newSnapshotDisplayEvidence(b domain.HistoricalSnapshotBatch, now time.Time, ttl time.Duration) snapshotDisplayEvidence {
	e := snapshotDisplayEvidence{priceBasis: map[string][]string{}, fxBasis: map[string][]string{}, prices: map[string]domain.QuoteEvidenceView{}, fx: map[string]domain.QuoteEvidenceView{}, liabilities: map[domain.AccountID]bool{}, instruments: map[domain.InstrumentID]domain.InstrumentType{}}
	for _, i := range b.Portfolio.Instruments {
		e.instruments[i.ID] = i.Type
	}
	for _, a := range b.Portfolio.Accounts {
		e.liabilities[a.Account.ID] = a.Account.IsLiability()
	}
	for _, q := range b.InstrumentQuoteFacts {
		v := quoteEvidence(q.ID.String(), q.SourceKind, q.SourceKey, q.QuotedAt, q.Delayed, now, ttl)
		v.EffectiveDate = q.EffectiveDate
		v.ValueEffectiveAt = q.ValueEffectiveAt
		v.FetchedAt = q.FetchedAt
		e.prices[q.ID.String()] = v
		e.priceBasis[q.ID.String()] = priceCompatibility(q)
	}
	for _, q := range b.FXQuoteFacts {
		v := quoteEvidence(q.ID.String(), q.SourceKind, q.SourceKey, q.QuotedAt, q.Delayed, now, ttl)
		v.EffectiveDate = q.EffectiveDate
		v.ValueEffectiveAt = q.ValueEffectiveAt
		v.FetchedAt = q.FetchedAt
		e.fx[q.ID.String()] = v
		e.fxBasis[q.ID.String()] = fxCompatibility(q)
	}
	return e
}
func snapshotDisplayItem(item domain.DailyValuationSnapshotItem, e snapshotDisplayEvidence) (domain.ValuationComponent, []domain.MissingInputView, bool) {
	c := domain.ValuationComponent{AccountID: item.AccountID, HoldingID: item.HoldingID, InstrumentID: item.InstrumentID, NativeAmount: item.NativeAmount, NativeCurrency: item.NativeCurrency, BaseAmountExact: snapshotItemBaseAmountString(item), Available: snapshotItemBaseAmountString(item) != ""}
	if item.InstrumentID != nil {
		c.InstrumentType = e.instruments[*item.InstrumentID]
	}
	hydrated := true
	if item.QuoteID != nil {
		v, ok := e.prices[*item.QuoteID]
		if ok {
			c.PriceEvidence = &v
		} else {
			hydrated = false
		}
	}
	if item.FXQuoteID != nil {
		v, ok := e.fx[*item.FXQuoteID]
		if ok {
			c.FXEvidence = &v
		} else {
			hydrated = false
		}
	}
	if item.Complete {
		return c, nil, false
	}
	if coverageOnlySnapshotItem(item) && hydrated && (c.PriceEvidence != nil || c.FXEvidence != nil) {
		return c, []domain.MissingInputView{{Kind: domain.MissingHistoryCoverage}}, false
	}
	if c.Available {
		return c, []domain.MissingInputView{{Kind: domain.MissingHistoryCoverage}}, true
	}
	kind := domain.MissingAccountValue
	if item.InstrumentID != nil {
		kind = domain.MissingInstrumentPrice
	}
	if item.NativeAmount != "" {
		kind = domain.MissingFXRate
	}
	missing := []domain.MissingInputView{{Kind: kind}}
	if item.MissingReason != nil && *item.MissingReason == "historical market-data coverage is unverified" {
		missing = append(missing, domain.MissingInputView{Kind: domain.MissingHistoryCoverage})
	}
	return c, missing, false
}
func snapshotDisplay(snapshot domain.DailyValuationSnapshot, e snapshotDisplayEvidence) (domain.HouseholdDisplayValuation, domain.ValuationAmount, error) {
	var a, l, p displayAccumulator
	if snapshot.ID == "" {
		unknown := domain.ValuationAmount{Currency: snapshot.Currency, Quality: domain.ValuationQuality{Availability: domain.ValuationNone, Coverage: domain.CoverageUnavailable, Reasons: []domain.ValuationReason{domain.ReasonEvidenceUnavailable}}}
		return domain.HouseholdDisplayValuation{Assets: unknown, Liabilities: unknown, NetWorth: unknown}, unknown, nil
	}

	for _, item := range snapshot.Items {
		c, missing, uncertain := snapshotDisplayItem(item, e)
		target := &a
		if e.liabilities[item.AccountID] {
			target = &l
		}
		if err := target.add(c, missing, snapshot.LocalDate, false); err != nil {
			return domain.HouseholdDisplayValuation{}, domain.ValuationAmount{}, err
		}
		target.uncertain = target.uncertain || uncertain
		if item.InstrumentID != nil {
			if err := p.add(c, missing, snapshot.LocalDate, false); err != nil {
				return domain.HouseholdDisplayValuation{}, domain.ValuationAmount{}, err
			}
			p.uncertain = p.uncertain || uncertain
		}
	}
	// Legacy aggregate-only records do not prove an empty universe.
	if snapshot.ID != "" && len(snapshot.Items) == 0 && snapshot.ComponentCount > 0 {
		a.quality.IncludedCount = snapshot.ComponentCount
		a.quality.UnpricedCount = snapshot.ComponentCount
		a.reason(domain.ReasonEvidenceUnavailable)
		p = a
	}
	h, err := householdDisplay(a, l, snapshot.Currency)
	if err != nil {
		return h, domain.ValuationAmount{}, err
	}
	portfolio, err := p.amount(snapshot.Currency, false)
	return h, portfolio, err
}
