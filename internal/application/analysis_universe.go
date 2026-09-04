package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// AnalysisInputs is the immutable read set consumed by the Phase 1b kernel.
// Keeping the pure input type public makes the calculation independently
// testable without opening a database, while AnalysisService supplies it from
// the repository in one application-level read flow.
type AnalysisInputs struct {
	Origin           domain.HistoryOrigin
	Portfolio        domain.PortfolioSnapshot
	Snapshots        []domain.DailyValuationSnapshot
	Activities       []domain.Activity
	InstrumentQuotes []domain.InstrumentQuote
	FXQuotes         []domain.FXQuote
}

type analysisUniverse struct {
	context     domain.ResolvedAnalysisContext
	accounts    map[domain.AccountID]domain.Account
	ownership   map[domain.AccountID]domain.Ownership
	instruments map[domain.InstrumentID]domain.Instrument
	holdings    map[domain.HoldingID]domain.Holding
}

func resolveAnalysisUniverse(input AnalysisInputs, query domain.AnalysisQuery) (analysisUniverse, error) {
	if input.Portfolio.Household == nil {
		return analysisUniverse{}, &domain.Error{Code: domain.ErrNotFound, Message: "household was not found"}
	}
	if err := query.Validate(); err != nil {
		return analysisUniverse{}, err
	}
	accounts := make(map[domain.AccountID]domain.Account, len(input.Portfolio.Accounts))
	ownership := make(map[domain.AccountID]domain.Ownership, len(input.Portfolio.Accounts))
	for _, record := range input.Portfolio.Accounts {
		accounts[record.Account.ID] = record.Account
		ownership[record.Account.ID] = record.Ownership
	}
	instruments := make(map[domain.InstrumentID]domain.Instrument, len(input.Portfolio.Instruments))
	for _, instrument := range input.Portfolio.Instruments {
		instruments[instrument.ID] = instrument
	}
	holdings := make(map[domain.HoldingID]domain.Holding, len(input.Portfolio.Holdings))
	for _, holding := range input.Portfolio.Holdings {
		holdings[holding.ID] = holding
	}

	componentsByKey := make(map[string]domain.ComponentID)
	for _, snapshot := range input.Snapshots {
		for _, item := range snapshot.Items {
			account, ok := accounts[item.AccountID]
			if !ok {
				// Historical snapshots can retain an archived account. Its
				// current record is still required for sign and scope matching;
				// an absent record is an integrity error rather than a guessed
				// asset.
				continue
			}
			if !domain.AccountEligibleForNetWorth(account) {
				continue
			}
			instrument, _ := instrumentForItem(item, instruments)
			if !analysisItemMatches(query, account, instrument, item, ownership[item.AccountID]) {
				continue
			}
			component := componentIDForItem(item, account)
			component.AssetClass = itemAssetClass(account, instrument, item)
			componentsByKey[component.Key()] = component
		}
	}
	components := make(domain.Components, 0, len(componentsByKey))
	for _, component := range componentsByKey {
		components = append(components, component)
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Key() < components[j].Key() })

	accountsIn := make([]domain.AccountID, 0)
	instrumentsIn := make([]domain.InstrumentID, 0)
	currenciesIn := make([]domain.CurrencyCode, 0)
	assetClassesIn := make([]string, 0)
	seenAccounts := map[domain.AccountID]bool{}
	seenInstruments := map[domain.InstrumentID]bool{}
	seenCurrencies := map[domain.CurrencyCode]bool{}
	seenClasses := map[string]bool{}
	for _, component := range components {
		if !seenAccounts[component.AccountID] {
			accountsIn = append(accountsIn, component.AccountID)
			seenAccounts[component.AccountID] = true
		}
		if component.InstrumentID != nil && !seenInstruments[*component.InstrumentID] {
			instrumentsIn = append(instrumentsIn, *component.InstrumentID)
			seenInstruments[*component.InstrumentID] = true
		}
		if component.Currency != "" && !seenCurrencies[component.Currency] {
			currenciesIn = append(currenciesIn, component.Currency)
			seenCurrencies[component.Currency] = true
		}
		class := componentAssetClass(component, accounts, instruments)
		if class != "" && !seenClasses[class] {
			assetClassesIn = append(assetClassesIn, class)
			seenClasses[class] = true
		}
	}
	sort.Slice(accountsIn, func(i, j int) bool { return accountsIn[i] < accountsIn[j] })
	sort.Slice(instrumentsIn, func(i, j int) bool { return instrumentsIn[i] < instrumentsIn[j] })
	sort.Slice(currenciesIn, func(i, j int) bool { return currenciesIn[i] < currenciesIn[j] })
	sort.Strings(assetClassesIn)
	context := domain.ResolvedAnalysisContext{
		Universe:            domain.AnalysisUniverse{Accounts: accountsIn, Instruments: instrumentsIn, Currencies: currenciesIn, AssetClasses: assetClassesIn, Components: components},
		InvestmentUniverse:  domain.InvestmentUniverse{Components: investmentComponents(components, query.IncludeCash, accounts)},
		AnalysisDayTimezone: input.Origin.Timezone,
	}
	return analysisUniverse{context: context, accounts: accounts, ownership: ownership, instruments: instruments, holdings: holdings}, nil
}

func analysisItemMatches(query domain.AnalysisQuery, account domain.Account, instrument *domain.Instrument, item domain.DailyValuationSnapshotItem, ownership domain.Ownership) bool {
	filters := query.Filters
	if filters.AccountID != nil && account.ID != *filters.AccountID {
		return false
	}
	if filters.Currency != nil && item.NativeCurrency != *filters.Currency {
		return false
	}
	if filters.InstrumentID != nil && (item.InstrumentID == nil || *item.InstrumentID != *filters.InstrumentID) {
		return false
	}
	if filters.MemberID != nil && !ownershipContains(ownership, *filters.MemberID) {
		return false
	}
	if filters.AssetClass != "" && !strings.EqualFold(filters.AssetClass, itemAssetClass(account, instrument, item)) {
		return false
	}
	switch query.Scope.Kind {
	case domain.ScopeHousehold:
		return true
	case domain.ScopeAccount:
		return account.ID.String() == query.Scope.ID
	case domain.ScopeCurrency:
		return item.NativeCurrency.String() == strings.ToUpper(query.Scope.ID)
	case domain.ScopeInstrument:
		return item.InstrumentID != nil && item.InstrumentID.String() == query.Scope.ID
	case domain.ScopeAssetClass:
		return strings.EqualFold(query.Scope.ID, itemAssetClass(account, instrument, item))
	default:
		return false
	}
}

func ownershipContains(ownership domain.Ownership, memberID domain.MemberID) bool {
	for _, share := range ownership.Shares() {
		if share.MemberID == memberID {
			return true
		}
	}
	return false
}

func componentIDForItem(item domain.DailyValuationSnapshotItem, account domain.Account) domain.ComponentID {
	component := domain.ComponentID{AccountID: item.AccountID, Currency: item.NativeCurrency}
	if item.HoldingID != nil {
		component.HoldingID = item.HoldingID
		component.InstrumentID = item.InstrumentID
		return component
	}
	// Every account-value component without an instrument is cash/simple cash.
	// TrackingBalance accounts must use the same cash bridge as brokerage cash;
	// TrackingMode describes persistence, not whether FX attribution applies.
	component.Cash = true
	return component
}

func componentForEffect(effect domain.ActivityEffect, accounts map[domain.AccountID]domain.Account, holdings map[domain.HoldingID]domain.Holding, instruments map[domain.InstrumentID]domain.Instrument) (domain.ComponentID, bool) {
	if effect.HoldingID != nil {
		holding, ok := holdings[*effect.HoldingID]
		if !ok {
			return domain.ComponentID{}, false
		}
		if _, ok := accounts[holding.AccountID]; !ok {
			return domain.ComponentID{}, false
		}
		instrumentID := holding.InstrumentID
		instrument, ok := instruments[instrumentID]
		if !ok {
			return domain.ComponentID{AccountID: holding.AccountID, HoldingID: effect.HoldingID, InstrumentID: &instrumentID, AssetClass: "unknown"}, true
		}
		return domain.ComponentID{AccountID: holding.AccountID, HoldingID: effect.HoldingID, InstrumentID: &instrumentID, Currency: instrument.QuoteCurrency, AssetClass: string(instrument.Type)}, true
	}
	if effect.AccountID == nil || effect.Money == nil {
		return domain.ComponentID{}, false
	}
	account, ok := accounts[*effect.AccountID]
	if !ok {
		return domain.ComponentID{}, false
	}
	return domain.ComponentID{AccountID: account.ID, Currency: effect.Money.Currency(), AssetClass: itemAssetClass(account, nil, domain.DailyValuationSnapshotItem{NativeCurrency: effect.Money.Currency()}), Cash: true}, true
}

func instrumentForItem(item domain.DailyValuationSnapshotItem, instruments map[domain.InstrumentID]domain.Instrument) (*domain.Instrument, bool) {
	if item.InstrumentID == nil {
		return nil, false
	}
	instrument, ok := instruments[*item.InstrumentID]
	if !ok {
		return nil, false
	}
	return &instrument, true
}

func investmentComponents(components domain.Components, includeCash bool, accounts map[domain.AccountID]domain.Account) domain.Components {
	result := make(domain.Components, 0, len(components))
	for _, component := range components {
		account, accountOK := accounts[component.AccountID]
		if component.InstrumentID != nil || (includeCash && component.Cash && accountOK && !account.IsLiability()) {
			result = append(result, component)
		}
	}
	return result
}

func itemAssetClass(account domain.Account, instrument *domain.Instrument, item domain.DailyValuationSnapshotItem) string {
	if instrument != nil {
		return string(instrument.Type)
	}
	if item.HoldingID == nil {
		if account.IsLiability() {
			return string(account.AccountType)
		}
		return domain.BucketCash
	}
	return "unknown"
}

func componentAssetClass(component domain.ComponentID, accounts map[domain.AccountID]domain.Account, instruments map[domain.InstrumentID]domain.Instrument) string {
	if component.InstrumentID != nil {
		if instrument, ok := instruments[*component.InstrumentID]; ok {
			return string(instrument.Type)
		}
	}
	if account, ok := accounts[component.AccountID]; ok && account.IsLiability() {
		return string(account.AccountType)
	}
	if component.Cash || component.HoldingID == nil {
		return domain.BucketCash
	}
	return "unknown"
}

func (u analysisUniverse) componentInUniverse(component domain.ComponentID) bool {
	key := component.Key()
	for _, candidate := range u.context.Universe.Components {
		if candidate.Key() == key {
			return true
		}
	}
	return false
}

func (u analysisUniverse) ensureComponentCurrency(component domain.ComponentID) (domain.ComponentID, error) {
	if component.Currency == "" {
		return component, fmt.Errorf("component %s has no currency", component.Key())
	}
	return component, nil
}
