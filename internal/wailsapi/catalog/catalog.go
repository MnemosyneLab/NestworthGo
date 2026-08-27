package catalog

import (
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

type AccountCombinationDTO struct {
	AccountType           string `json:"accountType"`
	BalanceSheetRole      string `json:"balanceSheetRole"`
	TrackingMode          string `json:"trackingMode"`
	RoleLocked            bool   `json:"roleLocked"`
	IncludeInNetWorth     bool   `json:"includeInNetWorth"`
	IncludeInPortfolio    bool   `json:"includeInPortfolio"`
	IncludeInLiquidAssets bool   `json:"includeInLiquidAssets"`
	WholeAccountWarning   bool   `json:"wholeAccountWarning"`
}

type CatalogDTO struct {
	Currencies                 []string                `json:"currencies"`
	InstrumentTypes            []string                `json:"instrumentTypes"`
	QuoteSources               []string                `json:"quoteSources"`
	InstrumentProviders        []string                `json:"instrumentProviders"`
	AccountTypes               []string                `json:"accountTypes"`
	BalanceSheetRoles          []string                `json:"balanceSheetRoles"`
	TrackingModes              []string                `json:"trackingModes"`
	AccountCombinations        []AccountCombinationDTO `json:"accountCombinations"`
	TrackingModesByAccountType map[string][]string     `json:"trackingModesByAccountType"`
	TrendRanges                []string                `json:"trendRanges"`
	Appearances                []string                `json:"appearances"`
	Languages                  []string                `json:"languages"`
	Accents                    []string                `json:"accents"`
	MoneyInReasons             []string                `json:"moneyInReasons"`
	MoneyOutReasons            []string                `json:"moneyOutReasons"`
	ValueUpdateReasons         []string                `json:"valueUpdateReasons"`
	TradeSides                 []string                `json:"tradeSides"`
}

func (s *Service) Catalog() CatalogDTO {
	combinations := make([]AccountCombinationDTO, 0, len(domain.LegalAccountCombinations()))
	trackingByType := map[string][]string{}
	seenTracking := map[string]map[string]struct{}{}
	for _, combination := range domain.LegalAccountCombinations() {
		defaults := domain.SuggestedInclusion(combination.AccountType, combination.TrackingMode)
		_, roleLocked := domain.DefaultBalanceSheetRole(combination.AccountType)
		item := AccountCombinationDTO{
			AccountType:           combination.AccountType.String(),
			BalanceSheetRole:      combination.BalanceSheetRole.String(),
			TrackingMode:          string(combination.TrackingMode),
			RoleLocked:            roleLocked,
			IncludeInNetWorth:     defaults.IncludeInNetWorth,
			IncludeInPortfolio:    defaults.IncludeInPortfolio,
			IncludeInLiquidAssets: defaults.IncludeInLiquidAssets,
			WholeAccountWarning:   combination.TrackingMode == domain.TrackingHoldings,
		}
		combinations = append(combinations, item)
		typeKey := combination.AccountType.String()
		if seenTracking[typeKey] == nil {
			seenTracking[typeKey] = map[string]struct{}{}
		}
		mode := string(combination.TrackingMode)
		if _, exists := seenTracking[typeKey][mode]; exists {
			continue
		}
		seenTracking[typeKey][mode] = struct{}{}
		trackingByType[typeKey] = append(trackingByType[typeKey], mode)
	}
	return CatalogDTO{
		Currencies:                 settings.SupportedCurrencies(),
		InstrumentTypes:            stringSlice(domain.AllInstrumentTypes()),
		QuoteSources:               stringSlice(domain.AllQuoteSourceKinds()),
		InstrumentProviders:        application.InstrumentProviderKeys(),
		AccountTypes:               stringSlice(domain.AllAccountTypes()),
		BalanceSheetRoles:          stringSlice(domain.AllBalanceSheetRoles()),
		TrackingModes:              stringSlice(domain.AllTrackingModes()),
		AccountCombinations:        combinations,
		TrackingModesByAccountType: trackingByType,
		TrendRanges:                stringSlice(domain.AllTrendRanges()),
		Appearances:                stringSlice(settings.AllAppearances()),
		Languages:                  stringSlice(settings.AllLanguages()),
		Accents:                    stringSlice(settings.AllAccents()),
		MoneyInReasons:             stringSlice(domain.MoneyInReasons()),
		MoneyOutReasons:            stringSlice(domain.MoneyOutReasons()),
		ValueUpdateReasons:         stringSlice(domain.ValueUpdateReasons()),
		TradeSides:                 stringSlice(domain.AllTradeSides()),
	}
}

func stringSlice[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}
