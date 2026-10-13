package application

import (
	"encoding/json"
	"github.com/waltwang/nestworth-go/internal/domain"
	"sort"
	"strconv"
	"time"
)

func compatibilityKeys(scope, basis [][]string) (string, string) {
	sort.Slice(scope, func(i, j int) bool { return scope[i][0] < scope[j][0] })
	sort.Slice(basis, func(i, j int) bool { return basis[i][0] < basis[j][0] })
	s, _ := json.Marshal(scope)
	b, _ := json.Marshal(basis)
	return string(s), string(b)
}
func priceCompatibility(q domain.InstrumentQuote) []string {
	return []string{q.Currency.String(), string(q.SourceKind), q.SourceKey, q.ObservationKind, q.PriceBasis, q.TimestampBasis, q.SourcePolicyVersion, strconv.Itoa(q.BindingRevision), q.SplitFactor, conversionCompatibility(q.ConversionJSON)}
}
func fxCompatibility(q domain.FXQuote) []string {
	return []string{q.BaseCurrency.String(), q.QuoteCurrency.String(), string(q.SourceKind), q.SourceKey, q.ObservationKind, q.TimestampBasis, q.SourcePolicyVersion}
}
func historicalValuationCompatibility(rows []HistoricalOverviewRow, p domain.PortfolioSnapshot) (string, string) {
	var scope, basis [][]string
	type accountRecords struct{ scope, basis [][]string }
	parents := map[string]*accountRecords{}
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for _, i := range p.Instruments {
		instruments[i.ID] = i
	}
	prices, fx := map[string]domain.InstrumentQuote{}, map[string]domain.FXQuote{}
	for _, q := range p.InstrumentQuotes {
		prices[q.ID.String()] = q
	}
	for _, q := range p.FXQuotes {
		fx[q.ID.String()] = q
	}
	for _, row := range rows {
		c := row.Left
		if row.Kind == "account" || c == nil || !c.Included {
			continue
		}
		identity := []string{row.Key, row.Role, row.InstrumentID, c.Currency}
		scope = append(scope, identity)
		b := []string{row.Key, c.Currency, c.AssetClass}
		if i, ok := instruments[domain.InstrumentID(row.InstrumentID)]; ok {
			b = append(b, string(i.Type), i.QuantityUnit, i.QuoteCurrency.String(), string(i.QuoteSource), strconv.Itoa(i.ProviderBindingRevision))
		}
		if c.Price != nil {
			b = append(b, priceCompatibility(prices[c.Price.ID])...)
		}
		if c.FX != nil {
			b = append(b, fxCompatibility(fx[c.FX.ID])...)
		}
		basis = append(basis, b)
		c.displayScope, c.displayBasis = compatibilityKeys([][]string{identity}, [][]string{b})
		if row.ParentKey != "" {
			group := parents[row.ParentKey]
			if group == nil {
				group = &accountRecords{}
				parents[row.ParentKey] = group
			}
			group.scope = append(group.scope, identity)
			group.basis = append(group.basis, b)
		}
	}
	for _, row := range rows {
		if row.Kind != "account" || row.Left == nil {
			continue
		}
		group := parents[row.Key]
		if group == nil {
			group = &accountRecords{}
		}
		row.Left.displayScope, row.Left.displayBasis = compatibilityKeys(group.scope, group.basis)
	}
	return compatibilityKeys(scope, basis)
}
func snapshotValuationCompatibility(s domain.DailyValuationSnapshot, e snapshotDisplayEvidence) (string, string) {
	var scope, basis [][]string
	for _, i := range s.Items {
		key := i.AccountID.String() + ":" + i.NativeCurrency.String()
		if i.HoldingID != nil {
			key += "/" + i.HoldingID.String()
		}
		instrument := ""
		if i.InstrumentID != nil {
			instrument = i.InstrumentID.String()
		}
		scope = append(scope, []string{key, instrument, strconv.FormatBool(e.liabilities[i.AccountID])})
		b := []string{key, i.NativeCurrency.String()}
		if i.QuoteID != nil {
			b = append(b, e.priceBasis[*i.QuoteID]...)
		}
		if i.FXQuoteID != nil {
			b = append(b, e.fxBasis[*i.FXQuoteID]...)
		}
		if i.PreferenceObservationID != nil {
			b = append(b, i.PreferenceObservationID.String())
		}
		if i.FXPreferenceObservationID != nil {
			b = append(b, i.FXPreferenceObservationID.String())
		}
		basis = append(basis, b)
	}
	return compatibilityKeys(scope, basis)
}

func currentValuationCompatibility(accounts []domain.AccountValuation, p domain.PortfolioSnapshot) (string, string) {
	snapshot := domain.DailyValuationSnapshot{}
	for _, a := range accounts {
		if !domain.AccountEligibleForNetWorth(a.Account) {
			continue
		}
		for _, c := range a.Components {
			i := domain.DailyValuationSnapshotItem{AccountID: c.AccountID, HoldingID: c.HoldingID, InstrumentID: c.InstrumentID, NativeCurrency: c.NativeCurrency, PreferenceObservationID: c.PreferenceObservationID, FXPreferenceObservationID: c.FXPreferenceObservationID}
			if c.PriceEvidence != nil {
				id := c.PriceEvidence.ObservationID
				i.QuoteID = &id
			}
			if c.FXEvidence != nil {
				id := c.FXEvidence.ObservationID
				i.FXQuoteID = &id
			}
			snapshot.Items = append(snapshot.Items, i)
		}
	}
	return snapshotValuationCompatibility(snapshot, newSnapshotDisplayEvidence(domain.HistoricalSnapshotBatch{Portfolio: p, InstrumentQuoteFacts: p.InstrumentQuotes, FXQuoteFacts: p.FXQuotes}, time.Time{}, 0))
}

// Conversion values and observation times change without changing units/basis.
func conversionCompatibility(raw string) string {
	if raw == "" {
		return ""
	}
	var e metalConversionEvidence
	if json.Unmarshal([]byte(raw), &e) != nil {
		return "unknown"
	}
	value, _ := json.Marshal([]string{e.Policy, e.Symbol, e.RawCurrency, e.RawUnit, e.FXSource, e.Currency, e.Unit})
	return string(value)
}
