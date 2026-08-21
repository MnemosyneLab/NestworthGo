package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrorCode is a stable category for errors crossing the application boundary.
type ErrorCode string

const (
	ErrValidation      ErrorCode = "validation"
	ErrNotFound        ErrorCode = "not_found"
	ErrConflict        ErrorCode = "conflict"
	ErrUnsupportedDB   ErrorCode = "unsupported_database"
	ErrMigration       ErrorCode = "migration_failed"
	ErrIntegrity       ErrorCode = "integrity_failed"
	ErrUnavailable     ErrorCode = "unavailable"
	ErrDecimalOverflow ErrorCode = "decimal_overflow"
)

// Error is safe to expose to the UI; database details stay below this boundary.
type Error struct {
	Code    ErrorCode
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func validation(field, message string) error {
	return &Error{Code: ErrValidation, Field: field, Message: message}
}

func notFound(entity string) error {
	return &Error{Code: ErrNotFound, Message: entity + " was not found"}
}

func conflict(message string) error {
	return &Error{Code: ErrConflict, Message: message}
}

func newID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New().String()
	}
	return id.String()
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func validID(value string) bool {
	return uuidPattern.MatchString(strings.ToLower(value))
}

// Typed UUID identities prevent IDs from unrelated aggregates being mixed.
type HouseholdID string
type MemberID string
type InstitutionID string
type GroupID string
type AccountID string
type AccountValueID string
type MediaAssetID string

func NewHouseholdID() HouseholdID       { return HouseholdID(newID()) }
func NewMemberID() MemberID             { return MemberID(newID()) }
func NewInstitutionID() InstitutionID   { return InstitutionID(newID()) }
func NewGroupID() GroupID               { return GroupID(newID()) }
func NewAccountID() AccountID           { return AccountID(newID()) }
func NewAccountValueID() AccountValueID { return AccountValueID(newID()) }
func NewMediaAssetID() MediaAssetID     { return MediaAssetID(newID()) }

func (id HouseholdID) String() string    { return string(id) }
func (id MemberID) String() string       { return string(id) }
func (id InstitutionID) String() string  { return string(id) }
func (id GroupID) String() string        { return string(id) }
func (id AccountID) String() string      { return string(id) }
func (id AccountValueID) String() string { return string(id) }
func (id MediaAssetID) String() string   { return string(id) }

func ParseHouseholdID(value string) (HouseholdID, error) {
	return parseID[HouseholdID](value, "householdId")
}
func ParseMemberID(value string) (MemberID, error) { return parseID[MemberID](value, "memberId") }
func ParseInstitutionID(value string) (InstitutionID, error) {
	return parseID[InstitutionID](value, "institutionId")
}
func ParseGroupID(value string) (GroupID, error)     { return parseID[GroupID](value, "groupId") }
func ParseAccountID(value string) (AccountID, error) { return parseID[AccountID](value, "accountId") }
func ParseAccountValueID(value string) (AccountValueID, error) {
	return parseID[AccountValueID](value, "accountValueId")
}
func ParseMediaAssetID(value string) (MediaAssetID, error) {
	return parseID[MediaAssetID](value, "mediaAssetId")
}

func parseID[T ~string](value, field string) (T, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !validID(value) {
		return "", validation(field, "must be a lowercase UUID")
	}
	return T(value), nil
}

var timestampLayout = "2006-01-02T15:04:05.000Z07:00"

func normalizeTime(value time.Time) time.Time { return value.UTC().Truncate(time.Millisecond) }
func formatTime(value time.Time) string       { return normalizeTime(value).Format(timestampLayout) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, validation("timestamp", "must be an RFC 3339 timestamp")
	}
	return normalizeTime(parsed), nil
}

func validateName(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", validation(field, "must not be empty")
	}
	if len([]rune(value)) > 120 {
		return "", validation(field, "must be 120 characters or fewer")
	}
	return value, nil
}

func validateNote(field string, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if len([]rune(trimmed)) > 2000 {
		return nil, validation(field, "must be 2000 characters or fewer")
	}
	return &trimmed, nil
}

const dateLayout = "2006-01-02"

func normalizeDate(field string, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	parsed, err := time.Parse(dateLayout, trimmed)
	if err != nil {
		return nil, validation(field, "must use YYYY-MM-DD")
	}
	canonical := parsed.Format(dateLayout)
	return &canonical, nil
}

// CurrencyCode is exactly three uppercase ASCII letters.
type CurrencyCode string

func ParseCurrency(value string) (CurrencyCode, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "", validation("currency", "must contain exactly three uppercase letters")
	}
	for _, char := range value {
		if char < 'A' || char > 'Z' {
			return "", validation("currency", "must contain exactly three uppercase letters")
		}
	}
	return CurrencyCode(value), nil
}

func (c CurrencyCode) String() string { return string(c) }

var moneySyntax = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,4})?$`)

// Money is a non-negative exact decimal amount in one currency.
type Money struct {
	amount   decimal.Decimal
	currency CurrencyCode
}

func ParseMoney(amount string, currency CurrencyCode) (Money, error) {
	canonicalCurrency, err := ParseCurrency(currency.String())
	if err != nil {
		return Money{}, err
	}
	if !moneySyntax.MatchString(amount) {
		return Money{}, validation("amount", "must be a canonical non-negative decimal with up to four fractional digits")
	}
	value, err := decimal.NewFromString(amount)
	if err != nil || value.IsNegative() || value.GreaterThan(decimal.RequireFromString("999999999999.9999")) {
		return Money{}, validation("amount", "is outside the supported range")
	}
	return Money{amount: value, currency: canonicalCurrency}, nil
}

func NewMoney(value decimal.Decimal, currency CurrencyCode) (Money, error) {
	if value.IsNegative() || value.GreaterThan(decimal.RequireFromString("999999999999.9999")) {
		return Money{}, &Error{Code: ErrDecimalOverflow, Field: "amount", Message: "amount is outside the supported range"}
	}
	value = value.RoundBank(4)
	return ParseMoney(value.StringFixed(4), currency)
}

func (m Money) Amount() decimal.Decimal { return m.amount }
func (m Money) Currency() CurrencyCode  { return m.currency }
func (m Money) CanonicalAmount() string { return m.amount.String() }
func (m Money) IsZero() bool            { return m.amount.IsZero() }

func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, validation("currency", "money currencies must match")
	}
	return NewMoney(m.amount.Add(other.amount), m.currency)
}

// PrimaryCategory controls the balance-sheet sign and allowed tracking mode.
type PrimaryCategory string

const (
	CategoryCashEquivalent PrimaryCategory = "cash_equivalent"
	CategoryInvestment     PrimaryCategory = "investment"
	CategoryProperty       PrimaryCategory = "property"
	CategoryReceivable     PrimaryCategory = "receivable"
	CategoryLiability      PrimaryCategory = "liability"
)

func (c PrimaryCategory) IsLiability() bool { return c == CategoryLiability }
func (c PrimaryCategory) String() string    { return string(c) }

func ParsePrimaryCategory(value string) (PrimaryCategory, error) {
	category := PrimaryCategory(strings.TrimSpace(value))
	switch category {
	case CategoryCashEquivalent, CategoryInvestment, CategoryProperty, CategoryReceivable, CategoryLiability:
		return category, nil
	default:
		return "", validation("primaryCategory", "is not supported")
	}
}

// SecondaryCategory is intentionally a string enum to keep SQLite and UI values stable.
type SecondaryCategory string

const (
	SecondaryCash                  SecondaryCategory = "cash"
	SecondaryBankAccount           SecondaryCategory = "bank_account"
	SecondaryDigitalWallet         SecondaryCategory = "digital_wallet"
	SecondaryBrokerCash            SecondaryCategory = "broker_cash"
	SecondaryOtherCashEquivalent   SecondaryCategory = "other_cash_equivalent"
	SecondaryBrokerageAccount      SecondaryCategory = "brokerage_account"
	SecondaryInvestmentFundAccount SecondaryCategory = "investment_fund_account"
	SecondaryBankInvestmentProduct SecondaryCategory = "bank_investment_product"
	SecondaryInsurance             SecondaryCategory = "insurance"
	SecondaryManualInvestment      SecondaryCategory = "manual_investment"
	SecondaryOtherInvestment       SecondaryCategory = "other_investment"
	SecondaryRealEstate            SecondaryCategory = "real_estate"
	SecondaryVehicle               SecondaryCategory = "vehicle"
	SecondaryCollectible           SecondaryCategory = "collectible"
	SecondaryOtherProperty         SecondaryCategory = "other_property"
	SecondaryLoanReceivable        SecondaryCategory = "loan_receivable"
	SecondaryOtherReceivable       SecondaryCategory = "other_receivable"
	SecondaryCreditCard            SecondaryCategory = "credit_card"
	SecondaryMortgage              SecondaryCategory = "mortgage"
	SecondaryAutoLoan              SecondaryCategory = "auto_loan"
	SecondaryConsumerLoan          SecondaryCategory = "consumer_loan"
	SecondaryPersonalDebt          SecondaryCategory = "personal_debt"
	SecondaryOtherLiability        SecondaryCategory = "other_liability"
)

var secondaryByPrimary = map[PrimaryCategory][]SecondaryCategory{
	CategoryCashEquivalent: {SecondaryCash, SecondaryBankAccount, SecondaryDigitalWallet, SecondaryBrokerCash, SecondaryOtherCashEquivalent},
	CategoryInvestment:     {SecondaryBrokerageAccount, SecondaryInvestmentFundAccount, SecondaryBankInvestmentProduct, SecondaryInsurance, SecondaryManualInvestment, SecondaryOtherInvestment},
	CategoryProperty:       {SecondaryRealEstate, SecondaryVehicle, SecondaryCollectible, SecondaryOtherProperty},
	CategoryReceivable:     {SecondaryLoanReceivable, SecondaryOtherReceivable},
	CategoryLiability:      {SecondaryCreditCard, SecondaryMortgage, SecondaryAutoLoan, SecondaryConsumerLoan, SecondaryPersonalDebt, SecondaryOtherLiability},
}

func ParseSecondaryCategory(value string) (SecondaryCategory, error) {
	candidate := SecondaryCategory(strings.TrimSpace(value))
	for _, values := range secondaryByPrimary {
		for _, value := range values {
			if value == candidate {
				return candidate, nil
			}
		}
	}
	return "", validation("secondaryCategory", "is not supported")
}

func (c SecondaryCategory) BelongsTo(primary PrimaryCategory) bool {
	for _, value := range secondaryByPrimary[primary] {
		if c == value {
			return true
		}
	}
	return false
}

func (c PrimaryCategory) DefaultTrackingMode() TrackingMode {
	switch c {
	case CategoryInvestment:
		return TrackingManualValue
	case CategoryProperty, CategoryReceivable:
		return TrackingManualValue
	default:
		return TrackingBalance
	}
}

// TrackingMode is immutable after an Account is created.
type TrackingMode string

const (
	TrackingBalance     TrackingMode = "balance"
	TrackingManualValue TrackingMode = "manual_value"
	TrackingHoldings    TrackingMode = "holdings"
)

func ParseTrackingMode(value string) (TrackingMode, error) {
	mode := TrackingMode(strings.TrimSpace(value))
	switch mode {
	case TrackingBalance, TrackingManualValue, TrackingHoldings:
		return mode, nil
	default:
		return "", validation("trackingMode", "is not supported")
	}
}

func (m TrackingMode) AllowedFor(primary PrimaryCategory) bool {
	switch primary {
	case CategoryCashEquivalent, CategoryLiability:
		return m == TrackingBalance
	case CategoryInvestment:
		return m == TrackingHoldings || m == TrackingManualValue
	case CategoryProperty, CategoryReceivable:
		return m == TrackingManualValue
	default:
		return false
	}
}

// OwnershipShare is an exact integer basis-point allocation.
type OwnershipShare struct {
	MemberID MemberID
	ShareBPS int
}

const TotalOwnershipBPS = 10000

type Ownership struct{ shares []OwnershipShare }

func ParseOwnership(shares []OwnershipShare) (Ownership, error) {
	if len(shares) == 0 {
		return Ownership{}, validation("ownership", "at least one owner is required")
	}
	seen := make(map[MemberID]struct{}, len(shares))
	total := 0
	canonical := make([]OwnershipShare, 0, len(shares))
	for _, share := range shares {
		memberID, err := ParseMemberID(share.MemberID.String())
		if err != nil {
			return Ownership{}, validation("ownership", "owner ID is invalid")
		}
		if share.ShareBPS < 1 || share.ShareBPS > TotalOwnershipBPS {
			return Ownership{}, validation("ownership", "each share must be between 1 and 10000 basis points")
		}
		if _, exists := seen[memberID]; exists {
			return Ownership{}, validation("ownership", "an owner may appear only once")
		}
		seen[memberID] = struct{}{}
		canonical = append(canonical, OwnershipShare{MemberID: memberID, ShareBPS: share.ShareBPS})
		total += share.ShareBPS
	}
	if total != TotalOwnershipBPS {
		return Ownership{}, validation("ownership", "shares must total exactly 10000 basis points")
	}
	return Ownership{shares: canonical}, nil
}

func EqualOwnership(memberIDs []MemberID) (Ownership, error) {
	if len(memberIDs) == 0 {
		return Ownership{}, validation("members", "at least one member is required")
	}
	shares := make([]OwnershipShare, len(memberIDs))
	base, remainder := TotalOwnershipBPS/len(memberIDs), TotalOwnershipBPS%len(memberIDs)
	for index, memberID := range memberIDs {
		shares[index] = OwnershipShare{MemberID: memberID, ShareBPS: base}
		if index < remainder {
			shares[index].ShareBPS++
		}
	}
	return ParseOwnership(shares)
}

func (o Ownership) Shares() []OwnershipShare { return append([]OwnershipShare(nil), o.shares...) }
func (o Ownership) IsShared() bool           { return len(o.shares) > 1 }

var percentPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,2})(\.[0-9]{1,2})?%?$`)

func PercentToBasisPoints(value string) (int, error) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "%") {
		value = strings.TrimSuffix(value, "%")
	}
	if !percentPattern.MatchString(value) {
		return 0, validation("ownership", "percentage must be between 0 and 100 with at most two decimals")
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil || parsed.IsNegative() || parsed.GreaterThan(decimal.NewFromInt(100)) {
		return 0, validation("ownership", "percentage must be between 0 and 100")
	}
	bps := parsed.Mul(decimal.NewFromInt(100)).IntPart()
	if parsed.Mul(decimal.NewFromInt(100)).Truncate(0).Cmp(parsed.Mul(decimal.NewFromInt(100))) != 0 {
		return 0, validation("ownership", "percentage supports at most two decimals")
	}
	return int(bps), nil
}

// Household is the singleton root of the balance sheet.
type Household struct {
	ID           HouseholdID
	Name         string
	BaseCurrency CurrencyCode
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewHousehold(name string, baseCurrency CurrencyCode, now time.Time) (Household, error) {
	name, err := validateName("name", name)
	if err != nil {
		return Household{}, err
	}
	canonicalCurrency, err := ParseCurrency(baseCurrency.String())
	if err != nil {
		return Household{}, err
	}
	now = normalizeTime(now)
	return Household{ID: NewHouseholdID(), Name: name, BaseCurrency: canonicalCurrency, CreatedAt: now, UpdatedAt: now}, nil
}

// Member, Institution, and Group are archived references retained for history.
type Member struct {
	ID            MemberID
	HouseholdID   HouseholdID
	Name          string
	AvatarAssetID *MediaAssetID
	Note          *string
	SortOrder     int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ArchivedAt    *time.Time
}

type Institution struct {
	ID              InstitutionID
	HouseholdID     HouseholdID
	Name            string
	IconKey         *string
	InstitutionType *string
	CountryCode     *string
	Website         *string
	Note            *string
	LogoAssetID     *MediaAssetID
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ArchivedAt      *time.Time
}

type Group struct {
	ID          GroupID
	HouseholdID HouseholdID
	Name        string
	IconKey     *string
	Color       *string
	LogoAssetID *MediaAssetID
	Description *string
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
}

func NewMember(householdID HouseholdID, name string, now time.Time) (Member, error) {
	name, err := validateName("name", name)
	if err != nil {
		return Member{}, err
	}
	now = normalizeTime(now)
	return Member{ID: NewMemberID(), HouseholdID: householdID, Name: name, CreatedAt: now, UpdatedAt: now}, nil
}

func NewInstitution(householdID HouseholdID, name string, now time.Time) (Institution, error) {
	name, err := validateName("name", name)
	if err != nil {
		return Institution{}, err
	}
	now = normalizeTime(now)
	iconKey := DefaultInstitutionIcon
	return Institution{ID: NewInstitutionID(), HouseholdID: householdID, Name: name, IconKey: &iconKey, CreatedAt: now, UpdatedAt: now}, nil
}

func NewGroup(householdID HouseholdID, name string, now time.Time) (Group, error) {
	name, err := validateName("name", name)
	if err != nil {
		return Group{}, err
	}
	now = normalizeTime(now)
	iconKey := DefaultGroupIcon
	return Group{ID: NewGroupID(), HouseholdID: householdID, Name: name, IconKey: &iconKey, CreatedAt: now, UpdatedAt: now}, nil
}

// Account is the balance-sheet unit. Its current observations live separately.
type Account struct {
	ID                    AccountID
	HouseholdID           HouseholdID
	InstitutionID         *InstitutionID
	GroupID               *GroupID
	Name                  string
	PrimaryCategory       PrimaryCategory
	SecondaryCategory     SecondaryCategory
	TrackingMode          TrackingMode
	DefaultCurrency       CurrencyCode
	Note                  *string
	IconKey               *string
	LogoAssetID           *MediaAssetID
	IncludeInNetWorth     bool
	IncludeInInvestment   bool
	IncludeInLiquidAssets bool
	OpenedOn              *string
	ClosedOn              *string
	SortOrder             int
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ArchivedAt            *time.Time
}

func NewAccount(input AccountInput, now time.Time) (Account, Ownership, *Money, error) {
	name, err := validateName("name", input.Name)
	if err != nil {
		return Account{}, Ownership{}, nil, err
	}
	iconKey, err := normalizedIconKey(input.IconKey, DefaultAccountIcon)
	if err != nil {
		return Account{}, Ownership{}, nil, err
	}
	if !input.SecondaryCategory.BelongsTo(input.PrimaryCategory) {
		return Account{}, Ownership{}, nil, validation("secondaryCategory", "does not belong to primary category")
	}
	if !input.TrackingMode.AllowedFor(input.PrimaryCategory) {
		return Account{}, Ownership{}, nil, validation("trackingMode", "is not allowed for this category")
	}
	if input.TrackingMode == TrackingHoldings {
		return Account{}, Ownership{}, nil, validation("trackingMode", "holdings accounts are planned for v0.1.2")
	}
	canonicalCurrency, err := ParseCurrency(input.DefaultCurrency.String())
	if err != nil {
		return Account{}, Ownership{}, nil, err
	}
	openedOn, err := normalizeDate("openedOn", input.OpenedOn)
	if err != nil {
		return Account{}, Ownership{}, nil, err
	}
	closedOn, err := normalizeDate("closedOn", input.ClosedOn)
	if err != nil {
		return Account{}, Ownership{}, nil, err
	}
	if openedOn != nil && closedOn != nil && *closedOn < *openedOn {
		return Account{}, Ownership{}, nil, validation("closedOn", "cannot be earlier than openedOn")
	}
	ownership, err := ParseOwnership(input.Ownership)
	if err != nil {
		return Account{}, Ownership{}, nil, err
	}
	note, err := validateNote("note", input.Note)
	if err != nil {
		return Account{}, Ownership{}, nil, err
	}
	var initial *Money
	if input.InitialAmount != "" {
		value, parseErr := ParseMoney(input.InitialAmount, canonicalCurrency)
		if parseErr != nil {
			return Account{}, Ownership{}, nil, parseErr
		}
		initial = &value
	}
	if initial == nil {
		return Account{}, Ownership{}, nil, validation("initialAmount", "is required for Balance and Manual Value accounts")
	}
	now = normalizeTime(now)
	return Account{
		ID: NewAccountID(), HouseholdID: input.HouseholdID, InstitutionID: input.InstitutionID, GroupID: input.GroupID,
		Name: name, PrimaryCategory: input.PrimaryCategory, SecondaryCategory: input.SecondaryCategory,
		TrackingMode: input.TrackingMode, DefaultCurrency: canonicalCurrency, Note: note,
		IconKey:           iconKey,
		IncludeInNetWorth: input.IncludeInNetWorth, IncludeInInvestment: input.IncludeInInvestment,
		IncludeInLiquidAssets: input.IncludeInLiquidAssets, OpenedOn: openedOn, ClosedOn: closedOn,
		SortOrder: input.SortOrder, CreatedAt: now, UpdatedAt: now,
	}, ownership, initial, nil
}

type AccountInput struct {
	HouseholdID              HouseholdID
	InstitutionID            *InstitutionID
	InstitutionIDSet         bool
	GroupID                  *GroupID
	GroupIDSet               bool
	Name                     string
	PrimaryCategory          PrimaryCategory
	SecondaryCategory        SecondaryCategory
	TrackingMode             TrackingMode
	DefaultCurrency          CurrencyCode
	Note                     *string
	NoteSet                  bool
	IconKey                  *string
	IncludeInNetWorth        bool
	IncludeInNetWorthSet     bool
	IncludeInInvestment      bool
	IncludeInInvestmentSet   bool
	IncludeInLiquidAssets    bool
	IncludeInLiquidAssetsSet bool
	OpenedOn                 *string
	OpenedOnSet              bool
	ClosedOn                 *string
	ClosedOnSet              bool
	SortOrder                int
	Ownership                []OwnershipShare
	OwnerIDs                 []MemberID
	OwnershipPercentages     []string
	InitialAmount            string
}

type AccountValue struct {
	ID          AccountValueID
	AccountID   AccountID
	ValueKind   TrackingMode
	Amount      Money
	EffectiveAt time.Time
	CreatedAt   time.Time
}

func NewAccountValue(account Account, amount Money, effectiveAt, createdAt time.Time) (AccountValue, error) {
	if account.TrackingMode != TrackingBalance && account.TrackingMode != TrackingManualValue {
		return AccountValue{}, validation("trackingMode", "account values are not valid for this tracking mode")
	}
	if amount.Currency() != account.DefaultCurrency {
		return AccountValue{}, validation("amount", "currency must match account currency")
	}
	return AccountValue{ID: NewAccountValueID(), AccountID: account.ID, ValueKind: account.TrackingMode, Amount: amount, EffectiveAt: normalizeTime(effectiveAt), CreatedAt: normalizeTime(createdAt)}, nil
}

func (a Account) SignedAmount(value Money) (decimal.Decimal, error) {
	if value.Currency() != a.DefaultCurrency {
		return decimal.Zero, validation("amount", "currency must match account currency")
	}
	if !a.IncludeInNetWorth || a.ArchivedAt != nil {
		return decimal.Zero, nil
	}
	if a.PrimaryCategory.IsLiability() {
		return value.Amount().Neg(), nil
	}
	return value.Amount(), nil
}

func (o Ownership) sorted() []OwnershipShare {
	result := o.Shares()
	sort.Slice(result, func(i, j int) bool { return result[i].MemberID < result[j].MemberID })
	return result
}

// OverviewTotals is the backend-owned authoritative result.
type OverviewTotals struct {
	AccountCount int
	Assets       Money
	Liabilities  Money
	NetWorth     decimal.Decimal
}

func (a Account) ValidateValue(value Money) error {
	if value.Currency() != a.DefaultCurrency {
		return validation("amount", "currency must match account currency")
	}
	return nil
}

// AccountRecord combines an Account with the immutable ownership and latest
// current-value observation needed by application read models.
type AccountRecord struct {
	Account         Account
	Ownership       Ownership
	LatestValue     *AccountValue
	InstitutionName string
	GroupName       string
}

type BreakdownItem struct {
	Key      string
	Label    string
	Amount   decimal.Decimal
	ShareBPS int
}

type OverviewResult struct {
	Currency      CurrencyCode
	AccountCount  int
	Assets        decimal.Decimal
	Liabilities   decimal.Decimal
	NetWorth      decimal.Decimal
	ByCategory    []BreakdownItem
	ByMember      []BreakdownItem
	ByInstitution []BreakdownItem
	ByGroup       []BreakdownItem
}

type OwnershipScope string

const (
	OwnershipAny    OwnershipScope = "any"
	OwnershipSole   OwnershipScope = "sole"
	OwnershipShared OwnershipScope = "shared"
)

type AccountFilter struct {
	IncludeArchived bool
	MemberID        *MemberID
	InstitutionID   *InstitutionID
	GroupID         *GroupID
	Category        *PrimaryCategory
	OwnershipScope  OwnershipScope
}

type ReadSnapshot struct {
	Household    *Household
	Members      []Member
	Institutions []Institution
	Groups       []Group
	Accounts     []AccountRecord
}
type MediaAsset struct {
	ID          MediaAssetID
	HouseholdID HouseholdID
	MimeType    string
	Data        []byte
	CreatedAt   time.Time
}
