package domain

import (
	"slices"
	"strings"
)

// AccountType is the closed set of real-world account/container identities.
type AccountType string

const (
	TypeCashOnHand        AccountType = "cash_on_hand"
	TypeBankAccount       AccountType = "bank_account"
	TypeBrokerage         AccountType = "brokerage"
	TypeInvestmentAccount AccountType = "investment_account"
	TypeCryptoExchange    AccountType = "crypto_exchange"
	TypeDigitalWallet     AccountType = "digital_wallet"
	TypePension           AccountType = "pension"
	TypeInsurancePolicy   AccountType = "insurance_policy"
	TypeProperty          AccountType = "property"
	TypeVehicle           AccountType = "vehicle"
	TypeCollectible       AccountType = "collectible"
	TypeReceivable        AccountType = "receivable"
	TypeCreditCard        AccountType = "credit_card"
	TypeLoan              AccountType = "loan"
	TypeOther             AccountType = "other"
)

func (t AccountType) String() string { return string(t) }

func ParseAccountType(value string) (AccountType, error) {
	parsed := AccountType(strings.TrimSpace(value))
	for _, candidate := range AllAccountTypes() {
		if parsed == candidate {
			return parsed, nil
		}
	}
	return "", validation("accountType", "is not supported")
}

func AllAccountTypes() []AccountType {
	return []AccountType{
		TypeCashOnHand, TypeBankAccount, TypeBrokerage, TypeInvestmentAccount,
		TypeCryptoExchange, TypeDigitalWallet, TypePension, TypeInsurancePolicy,
		TypeProperty, TypeVehicle, TypeCollectible, TypeReceivable,
		TypeCreditCard, TypeLoan, TypeOther,
	}
}

// BalanceSheetRole is the persistent asset/liability side of an Account.
type BalanceSheetRole string

const (
	RoleAsset     BalanceSheetRole = "asset"
	RoleLiability BalanceSheetRole = "liability"
)

func (r BalanceSheetRole) String() string    { return string(r) }
func (r BalanceSheetRole) IsLiability() bool { return r == RoleLiability }

func ParseBalanceSheetRole(value string) (BalanceSheetRole, error) {
	parsed := BalanceSheetRole(strings.TrimSpace(value))
	switch parsed {
	case RoleAsset, RoleLiability:
		return parsed, nil
	default:
		return "", validation("balanceSheetRole", "is not supported")
	}
}

func AllBalanceSheetRoles() []BalanceSheetRole {
	return []BalanceSheetRole{RoleAsset, RoleLiability}
}

// ClassificationBasis explains how a historical bucket name was derived.
type ClassificationBasis string

const (
	ClassificationCurrentMetadataDerived ClassificationBasis = "current-metadata-derived"
)

func (b ClassificationBasis) String() string { return string(b) }

// AssetClass is the single classification decision used by Overview,
// Portfolio, Account detail, and historical breakdown.
type AssetClass struct {
	Role                BalanceSheetRole
	Bucket              string
	Incomplete          bool
	MissingInstrument   bool
	ClassificationBasis ClassificationBasis
}

const (
	BucketCash                   = "cash"
	BucketStock                  = "stock"
	BucketETF                    = "etf"
	BucketMutualFund             = "mutual_fund"
	BucketBond                   = "bond"
	BucketBankInvestmentProduct  = "bank_investment_product"
	BucketPreciousMetal          = "precious_metal"
	BucketCrypto                 = "crypto"
	BucketProperty               = "property"
	BucketVehicle                = "vehicle"
	BucketReceivable             = "receivable"
	BucketUnclassifiedInvestment = "unclassified_investment"
	BucketInsurance              = "insurance"
	BucketCollectible            = "collectible"
	BucketOtherAsset             = "other_asset"
	BucketCreditCard             = "credit_card"
	BucketLoan                   = "loan"
	BucketOtherLiability         = "other_liability"
	BucketPension                = "pension"
)

// AccountCombination is one legal (type, role, tracking) row.
type AccountCombination struct {
	AccountType      AccountType
	BalanceSheetRole BalanceSheetRole
	TrackingMode     TrackingMode
}

// InclusionDefaults are catalog suggestions only. Saved user choices win.
type InclusionDefaults struct {
	IncludeInNetWorth     bool
	IncludeInPortfolio    bool
	IncludeInLiquidAssets bool
}

type accountCombinationKey struct {
	accountType AccountType
	role        BalanceSheetRole
	tracking    TrackingMode
}

var legalAccountCombinations = []AccountCombination{
	{TypeCashOnHand, RoleAsset, TrackingBalance},
	{TypeCashOnHand, RoleAsset, TrackingHoldings},
	{TypeBankAccount, RoleAsset, TrackingBalance},
	{TypeBankAccount, RoleAsset, TrackingHoldings},
	{TypeBrokerage, RoleAsset, TrackingHoldings},
	{TypeBrokerage, RoleAsset, TrackingManualValue},
	{TypeInvestmentAccount, RoleAsset, TrackingHoldings},
	{TypeInvestmentAccount, RoleAsset, TrackingManualValue},
	{TypeCryptoExchange, RoleAsset, TrackingHoldings},
	{TypeDigitalWallet, RoleAsset, TrackingBalance},
	{TypeDigitalWallet, RoleAsset, TrackingHoldings},
	{TypePension, RoleAsset, TrackingHoldings},
	{TypePension, RoleAsset, TrackingManualValue},
	{TypeInsurancePolicy, RoleAsset, TrackingManualValue},
	{TypeProperty, RoleAsset, TrackingManualValue},
	{TypeVehicle, RoleAsset, TrackingManualValue},
	{TypeCollectible, RoleAsset, TrackingManualValue},
	{TypeReceivable, RoleAsset, TrackingBalance},
	{TypeReceivable, RoleAsset, TrackingManualValue},
	{TypeCreditCard, RoleLiability, TrackingBalance},
	{TypeLoan, RoleLiability, TrackingBalance},
	{TypeOther, RoleAsset, TrackingBalance},
	{TypeOther, RoleAsset, TrackingManualValue},
	{TypeOther, RoleAsset, TrackingHoldings},
	{TypeOther, RoleLiability, TrackingBalance},
}

var legalAccountCombinationSet = func() map[accountCombinationKey]struct{} {
	set := make(map[accountCombinationKey]struct{}, len(legalAccountCombinations))
	for _, combination := range legalAccountCombinations {
		set[accountCombinationKey{combination.AccountType, combination.BalanceSheetRole, combination.TrackingMode}] = struct{}{}
	}
	return set
}()

func LegalAccountCombinations() []AccountCombination {
	return slices.Clone(legalAccountCombinations)
}

func IsValidAccountCombination(accountType AccountType, role BalanceSheetRole, tracking TrackingMode) bool {
	_, ok := legalAccountCombinationSet[accountCombinationKey{accountType, role, tracking}]
	return ok
}

func DefaultBalanceSheetRole(accountType AccountType) (BalanceSheetRole, bool) {
	if accountType == TypeOther {
		return "", false
	}
	var found BalanceSheetRole
	for _, combination := range legalAccountCombinations {
		if combination.AccountType != accountType {
			continue
		}
		if found != "" && found != combination.BalanceSheetRole {
			return "", false
		}
		found = combination.BalanceSheetRole
	}
	return found, found != ""
}

func TrackingModesFor(accountType AccountType, role BalanceSheetRole) []TrackingMode {
	modes := make([]TrackingMode, 0, 3)
	seen := map[TrackingMode]struct{}{}
	for _, combination := range legalAccountCombinations {
		if combination.AccountType != accountType || combination.BalanceSheetRole != role {
			continue
		}
		if _, exists := seen[combination.TrackingMode]; exists {
			continue
		}
		seen[combination.TrackingMode] = struct{}{}
		modes = append(modes, combination.TrackingMode)
	}
	return modes
}

func SuggestedInclusion(accountType AccountType, tracking TrackingMode) InclusionDefaults {
	defaults := InclusionDefaults{IncludeInNetWorth: true}
	switch accountType {
	case TypeBrokerage, TypeInvestmentAccount, TypeCryptoExchange, TypePension:
		defaults.IncludeInPortfolio = true
	}
	if accountType == TypeCashOnHand || (tracking == TrackingBalance && (accountType == TypeBankAccount || accountType == TypeDigitalWallet)) {
		defaults.IncludeInLiquidAssets = true
	}
	return defaults
}

func ClassifySimpleAccount(accountType AccountType, role BalanceSheetRole, tracking TrackingMode) (AssetClass, error) {
	if !IsValidAccountCombination(accountType, role, tracking) {
		return AssetClass{}, validation("accountType", "is not a valid account combination")
	}
	if tracking == TrackingHoldings {
		return AssetClass{}, validation("trackingMode", "composite accounts must be classified from components")
	}
	class := AssetClass{Role: role, ClassificationBasis: ClassificationCurrentMetadataDerived}
	switch {
	case accountType == TypeCashOnHand || accountType == TypeBankAccount || accountType == TypeDigitalWallet:
		class.Bucket = BucketCash
	case accountType == TypeBrokerage || accountType == TypeInvestmentAccount:
		class.Bucket = BucketUnclassifiedInvestment
	case accountType == TypePension:
		class.Bucket = BucketPension
	case accountType == TypeInsurancePolicy:
		class.Bucket = BucketInsurance
	case accountType == TypeProperty:
		class.Bucket = BucketProperty
	case accountType == TypeVehicle:
		class.Bucket = BucketVehicle
	case accountType == TypeCollectible:
		class.Bucket = BucketCollectible
	case accountType == TypeReceivable:
		class.Bucket = BucketReceivable
	case accountType == TypeCreditCard:
		class.Bucket = BucketCreditCard
	case accountType == TypeLoan:
		class.Bucket = BucketLoan
	case accountType == TypeOther && role == RoleAsset:
		class.Bucket = BucketOtherAsset
	case accountType == TypeOther && role == RoleLiability:
		class.Bucket = BucketOtherLiability
	default:
		return AssetClass{}, validation("accountType", "is not a valid account combination")
	}
	return class, nil
}

func ClassifyCashBalance(account Account) AssetClass {
	return AssetClass{Role: account.BalanceSheetRole, Bucket: BucketCash}
}

func ClassifyHolding(account Account, instrument *Instrument) AssetClass {
	if instrument == nil {
		return AssetClass{Role: account.BalanceSheetRole, Incomplete: true, MissingInstrument: true}
	}
	return AssetClass{Role: account.BalanceSheetRole, Bucket: string(instrument.Type)}
}

func ClassifyAccountComponent(account Account, instrument *Instrument, cash bool) (AssetClass, error) {
	if account.TrackingMode == TrackingHoldings {
		if cash {
			return ClassifyCashBalance(account), nil
		}
		return ClassifyHolding(account, instrument), nil
	}
	return ClassifySimpleAccount(account.AccountType, account.BalanceSheetRole, account.TrackingMode)
}
