package wire

import "github.com/waltwang/nestworth-go/internal/domain"

// Additive display fields do not change the strict amount/Complete contract.
type ValuationAmountDTO struct {
	Currency      string                  `json:"currency"`
	Total         *MoneyView              `json:"total"`
	KnownSubtotal *MoneyView              `json:"knownSubtotal"`
	Quality       domain.ValuationQuality `json:"quality"`
}
type HouseholdDisplayValuationDTO struct {
	Assets      ValuationAmountDTO `json:"assets"`
	Liabilities ValuationAmountDTO `json:"liabilities"`
	NetWorth    ValuationAmountDTO `json:"netWorth"`
}

func FromValuationAmount(v domain.ValuationAmount) ValuationAmountDTO {
	return ValuationAmountDTO{Currency: v.Currency.String(), Total: FromMoneyView(v.Total), KnownSubtotal: FromMoneyView(v.KnownSubtotal), Quality: v.Quality}
}
func FromHouseholdDisplay(v domain.HouseholdDisplayValuation) HouseholdDisplayValuationDTO {
	return HouseholdDisplayValuationDTO{Assets: FromValuationAmount(v.Assets), Liabilities: FromValuationAmount(v.Liabilities), NetWorth: FromValuationAmount(v.NetWorth)}
}
