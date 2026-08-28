// Package wire holds serialization helpers shared by every
// internal/wailsapi service: canonical string conversions for the domain's
// decimal-wrapping value types, a
// shared timestamp format, and the read-model view DTOs
// (MoneyView/SignedMoneyView) that recur across services. Per-service DTOs
// still live in each service's own dto.go; this package exists so those
// files do not each redefine the same primitives.
package wire

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// timestampLayout matches the RFC 3339 millisecond-precision, UTC format
// already required at every existing UI boundary (see
// docs/architecture/data-and-ipc-contracts.md, "Serialization and view
// models").
const timestampLayout = "2006-01-02T15:04:05.000Z"

// FormatTime renders a time.Time as a UTC RFC 3339 string with millisecond
// precision. The zero time formats as an empty string so optional-time DTO
// fields can round-trip through a plain string when the caller does not
// need a pointer.
func FormatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Truncate(time.Millisecond).Format(timestampLayout)
}

// FormatTimePtr is FormatTime for a *time.Time, returning nil for a nil
// input so optional timestamps serialize as an explicit null rather than
// an empty string.
func FormatTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := FormatTime(*value)
	return &formatted
}

// ParseTime parses a UTC RFC 3339 timestamp produced by FormatTime (or any
// RFC 3339 string, for input leniency).
func ParseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "timestamp", Message: "must be an RFC 3339 timestamp"}
	}
	return parsed.UTC(), nil
}

// MoneyView is the wire shape of domain.MoneyView: a canonical decimal
// string amount plus its three-letter currency code. Every monetary DTO
// field in internal/wailsapi is either this shape, a plain string produced
// by a value type's Canonical()/CanonicalAmount() method, or a pointer to
// one of these when the value can be legitimately absent.
type MoneyView struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// SignedMoneyView is MoneyView's counterpart for values that may be
// negative (realized/unrealized gain, currency and instrument movement).
type SignedMoneyView struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func FromMoneyView(value *domain.MoneyView) *MoneyView {
	if value == nil {
		return nil
	}
	return &MoneyView{Amount: value.Amount, Currency: value.Currency.String()}
}

func FromSignedMoneyView(value *domain.SignedMoneyView) *SignedMoneyView {
	if value == nil {
		return nil
	}
	return &SignedMoneyView{Amount: value.Amount, Currency: value.Currency.String()}
}

func FromMoney(value domain.Money) MoneyView {
	return MoneyView{Amount: value.CanonicalAmount(), Currency: value.Currency().String()}
}

func FromMoneyPtr(value *domain.Money) *MoneyView {
	if value == nil {
		return nil
	}
	converted := FromMoney(*value)
	return &converted
}

func FromSignedMoney(value domain.SignedMoney) SignedMoneyView {
	return SignedMoneyView{Amount: value.CanonicalAmount(), Currency: value.Currency().String()}
}

// StringPtr returns nil for an empty string and a pointer to the value
// otherwise, matching the "explicit optional pointer" rule (technical
// for DTO fields sourced from an optional domain string.
func StringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// StringFromPtr is StringPtr's inverse for request DTOs: a nil pointer and
// an empty string are both treated as "not provided."
func StringFromPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// HouseholdDTO, MemberDTO, InstitutionDTO, and GroupDTO are shared read
// models: both the household service's Bootstrap and the directory
// service's list/create/update/archive methods return them, so they are
// defined once here rather than duplicated per service.
type HouseholdDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BaseCurrency string `json:"baseCurrency"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func FromHousehold(value domain.Household) HouseholdDTO {
	return HouseholdDTO{
		ID:           value.ID.String(),
		Name:         value.Name,
		BaseCurrency: value.BaseCurrency.String(),
		CreatedAt:    FormatTime(value.CreatedAt),
		UpdatedAt:    FormatTime(value.UpdatedAt),
	}
}

type MemberDTO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	IconKey    string  `json:"iconKey"`
	Note       *string `json:"note,omitempty"`
	SortOrder  int     `json:"sortOrder"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
	ArchivedAt *string `json:"archivedAt,omitempty"`
}

func FromMember(value domain.Member) MemberDTO {
	dto := MemberDTO{
		ID: value.ID.String(), Name: value.Name, IconKey: iconValue(value.IconKey, domain.DefaultMemberIcon), Note: value.Note, SortOrder: value.SortOrder,
		CreatedAt: FormatTime(value.CreatedAt), UpdatedAt: FormatTime(value.UpdatedAt),
		ArchivedAt: FormatTimePtr(value.ArchivedAt),
	}
	return dto
}

func FromMembers(values []domain.Member) []MemberDTO {
	result := make([]MemberDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromMember(value))
	}
	return result
}

type InstitutionDTO struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	IconKey         string  `json:"iconKey"`
	InstitutionType string  `json:"institutionType"`
	CountryCode     *string `json:"countryCode,omitempty"`
	Website         *string `json:"website,omitempty"`
	Note            *string `json:"note,omitempty"`
	SortOrder       int     `json:"sortOrder"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	ArchivedAt      *string `json:"archivedAt,omitempty"`
}

func FromInstitution(value domain.Institution) InstitutionDTO {
	dto := InstitutionDTO{
		ID: value.ID.String(), Name: value.Name, IconKey: iconValue(value.IconKey, domain.DefaultIconForInstitutionType(value.InstitutionType)), InstitutionType: string(value.InstitutionType),
		CountryCode: value.CountryCode, Website: value.Website, Note: value.Note, SortOrder: value.SortOrder,
		CreatedAt: FormatTime(value.CreatedAt), UpdatedAt: FormatTime(value.UpdatedAt),
		ArchivedAt: FormatTimePtr(value.ArchivedAt),
	}
	return dto
}

func FromInstitutions(values []domain.Institution) []InstitutionDTO {
	result := make([]InstitutionDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromInstitution(value))
	}
	return result
}

type GroupDTO struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	IconKey     string  `json:"iconKey"`
	Color       *string `json:"color,omitempty"`
	Description *string `json:"description,omitempty"`
	SortOrder   int     `json:"sortOrder"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	ArchivedAt  *string `json:"archivedAt,omitempty"`
}

func FromGroup(value domain.Group) GroupDTO {
	dto := GroupDTO{
		ID: value.ID.String(), Name: value.Name, IconKey: iconValue(value.IconKey, domain.DefaultGroupIcon), Color: value.Color,
		Description: value.Description, SortOrder: value.SortOrder,
		CreatedAt: FormatTime(value.CreatedAt), UpdatedAt: FormatTime(value.UpdatedAt),
		ArchivedAt: FormatTimePtr(value.ArchivedAt),
	}
	return dto
}

func FromGroups(values []domain.Group) []GroupDTO {
	result := make([]GroupDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromGroup(value))
	}
	return result
}

// OwnershipShareDTO mirrors domain.OwnershipShare. It is shared by the
// account service (Account create/update/read) and the history service
// (activity ownership context), so it lives here rather than in either.
type OwnershipShareDTO struct {
	MemberID string `json:"memberId"`
	ShareBPS int    `json:"shareBps"`
}

func FromOwnershipShare(value domain.OwnershipShare) OwnershipShareDTO {
	return OwnershipShareDTO{MemberID: value.MemberID.String(), ShareBPS: value.ShareBPS}
}

func FromOwnershipShares(values []domain.OwnershipShare) []OwnershipShareDTO {
	result := make([]OwnershipShareDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromOwnershipShare(value))
	}
	return result
}

func (s OwnershipShareDTO) ToDomain() (domain.OwnershipShare, error) {
	memberID, err := domain.ParseMemberID(s.MemberID)
	if err != nil {
		return domain.OwnershipShare{}, err
	}
	return domain.OwnershipShare{MemberID: memberID, ShareBPS: s.ShareBPS}, nil
}

// ToOwnershipShares converts a request DTO slice to domain shares, failing
// on the first invalid member ID.
func ToOwnershipShares(values []OwnershipShareDTO) ([]domain.OwnershipShare, error) {
	result := make([]domain.OwnershipShare, 0, len(values))
	for _, value := range values {
		share, err := value.ToDomain()
		if err != nil {
			return nil, err
		}
		result = append(result, share)
	}
	return result, nil
}

// AccountDTO mirrors domain.Account. It is shared by the account service
// (owns Account CRUD) and the portfolio/analytics services (which embed it
// inside read models such as AccountValuationDTO/AccountGainDTO), so it
// lives here rather than being duplicated per service.
type AccountDTO struct {
	ID                    string  `json:"id"`
	HouseholdID           string  `json:"householdId"`
	InstitutionID         *string `json:"institutionId,omitempty"`
	GroupID               *string `json:"groupId,omitempty"`
	Name                  string  `json:"name"`
	AccountType           string  `json:"accountType"`
	BalanceSheetRole      string  `json:"balanceSheetRole"`
	TrackingMode          string  `json:"trackingMode"`
	DefaultCurrency       string  `json:"defaultCurrency"`
	Note                  *string `json:"note,omitempty"`
	IconKey               string  `json:"iconKey"`
	IncludeInNetWorth     bool    `json:"includeInNetWorth"`
	IncludeInPortfolio    bool    `json:"includeInPortfolio"`
	IncludeInLiquidAssets bool    `json:"includeInLiquidAssets"`
	OpenedOn              *string `json:"openedOn,omitempty"`
	ClosedOn              *string `json:"closedOn,omitempty"`
	SortOrder             int     `json:"sortOrder"`
	CreatedAt             string  `json:"createdAt"`
	UpdatedAt             string  `json:"updatedAt"`
	ArchivedAt            *string `json:"archivedAt,omitempty"`
}

func FromAccount(value domain.Account) AccountDTO {
	dto := AccountDTO{
		ID: value.ID.String(), HouseholdID: value.HouseholdID.String(), Name: value.Name,
		AccountType: value.AccountType.String(), BalanceSheetRole: string(value.BalanceSheetRole),
		TrackingMode: string(value.TrackingMode), DefaultCurrency: value.DefaultCurrency.String(),
		Note: value.Note, IconKey: iconValue(value.IconKey, domain.DefaultAccountIcon(value.AccountType)), IncludeInNetWorth: value.IncludeInNetWorth,
		IncludeInPortfolio: value.IncludeInPortfolio, IncludeInLiquidAssets: value.IncludeInLiquidAssets,
		OpenedOn: value.OpenedOn, ClosedOn: value.ClosedOn, SortOrder: value.SortOrder,
		CreatedAt: FormatTime(value.CreatedAt), UpdatedAt: FormatTime(value.UpdatedAt),
		ArchivedAt: FormatTimePtr(value.ArchivedAt),
	}
	if value.InstitutionID != nil {
		id := value.InstitutionID.String()
		dto.InstitutionID = &id
	}
	if value.GroupID != nil {
		id := value.GroupID.String()
		dto.GroupID = &id
	}
	return dto
}

// AccountValueDTO mirrors domain.AccountValue.
type AccountValueDTO struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"accountId"`
	ValueKind   string    `json:"valueKind"`
	Amount      MoneyView `json:"amount"`
	EffectiveAt string    `json:"effectiveAt"`
	CreatedAt   string    `json:"createdAt"`
}

func FromAccountValue(value domain.AccountValue) AccountValueDTO {
	return AccountValueDTO{
		ID: value.ID.String(), AccountID: value.AccountID.String(), ValueKind: string(value.ValueKind),
		Amount: FromMoney(value.Amount), EffectiveAt: FormatTime(value.EffectiveAt), CreatedAt: FormatTime(value.CreatedAt),
	}
}

// AccountRecordDTO mirrors domain.AccountRecord.
type AccountRecordDTO struct {
	Account         AccountDTO          `json:"account"`
	Ownership       []OwnershipShareDTO `json:"ownership"`
	LatestValue     *AccountValueDTO    `json:"latestValue,omitempty"`
	InstitutionName string              `json:"institutionName,omitempty"`
	GroupName       string              `json:"groupName,omitempty"`
}

func FromAccountRecord(value domain.AccountRecord) AccountRecordDTO {
	dto := AccountRecordDTO{
		Account: FromAccount(value.Account), Ownership: FromOwnershipShares(value.Ownership.Shares()),
		InstitutionName: value.InstitutionName, GroupName: value.GroupName,
	}
	if value.LatestValue != nil {
		latest := FromAccountValue(*value.LatestValue)
		dto.LatestValue = &latest
	}
	return dto
}

func FromAccountRecords(values []domain.AccountRecord) []AccountRecordDTO {
	result := make([]AccountRecordDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromAccountRecord(value))
	}
	return result
}

// QuoteEvidenceDTO mirrors domain.QuoteEvidenceView.
type QuoteEvidenceDTO struct {
	ObservationID string `json:"observationId"`
	Source        string `json:"source"`
	SourceKey     string `json:"sourceKey"`
	QuotedAt      string `json:"quotedAt"`
	Freshness     string `json:"freshness"`
	Delayed       bool   `json:"delayed"`
}

func FromQuoteEvidence(value *domain.QuoteEvidenceView) *QuoteEvidenceDTO {
	if value == nil {
		return nil
	}
	return &QuoteEvidenceDTO{
		ObservationID: value.ObservationID, Source: string(value.Source), SourceKey: value.SourceKey,
		QuotedAt: FormatTime(value.QuotedAt), Freshness: string(value.Freshness), Delayed: value.Delayed,
	}
}

// ValuationComponentDTO mirrors domain.ValuationComponent.
type ValuationComponentDTO struct {
	AccountID        string            `json:"accountId"`
	HoldingID        *string           `json:"holdingId,omitempty"`
	InstrumentID     *string           `json:"instrumentId,omitempty"`
	InstrumentName   string            `json:"instrumentName,omitempty"`
	InstrumentSymbol string            `json:"instrumentSymbol,omitempty"`
	NativeAmount     string            `json:"nativeAmount"`
	NativeCurrency   string            `json:"nativeCurrency"`
	BaseAmount       *MoneyView        `json:"baseAmount,omitempty"`
	BaseAmountExact  string            `json:"baseAmountExact,omitempty"`
	PriceEvidence    *QuoteEvidenceDTO `json:"priceEvidence,omitempty"`
	FXEvidence       *QuoteEvidenceDTO `json:"fxEvidence,omitempty"`
	Available        bool              `json:"available"`
}

func FromValuationComponent(value domain.ValuationComponent) ValuationComponentDTO {
	dto := ValuationComponentDTO{
		AccountID: value.AccountID.String(), InstrumentName: value.InstrumentName, InstrumentSymbol: value.InstrumentSymbol,
		NativeAmount: value.NativeAmount, NativeCurrency: value.NativeCurrency.String(),
		BaseAmount: FromMoneyView(value.BaseAmount), BaseAmountExact: value.BaseAmountExact,
		PriceEvidence: FromQuoteEvidence(value.PriceEvidence), FXEvidence: FromQuoteEvidence(value.FXEvidence),
		Available: value.Available,
	}
	if value.HoldingID != nil {
		id := value.HoldingID.String()
		dto.HoldingID = &id
	}
	if value.InstrumentID != nil {
		id := value.InstrumentID.String()
		dto.InstrumentID = &id
	}
	return dto
}

func FromValuationComponents(values []domain.ValuationComponent) []ValuationComponentDTO {
	result := make([]ValuationComponentDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromValuationComponent(value))
	}
	return result
}

// MissingInputDTO mirrors domain.MissingInputView.
type MissingInputDTO struct {
	Kind             string  `json:"kind"`
	AccountID        string  `json:"accountId"`
	InstrumentID     *string `json:"instrumentId,omitempty"`
	InstrumentName   string  `json:"instrumentName,omitempty"`
	InstrumentSymbol string  `json:"instrumentSymbol,omitempty"`
	BaseCurrency     string  `json:"baseCurrency,omitempty"`
	QuoteCurrency    string  `json:"quoteCurrency,omitempty"`
}

func FromMissingInput(value domain.MissingInputView) MissingInputDTO {
	dto := MissingInputDTO{
		Kind: string(value.Kind), AccountID: value.AccountID.String(), InstrumentName: value.InstrumentName,
		InstrumentSymbol: value.InstrumentSymbol, BaseCurrency: value.BaseCurrency.String(), QuoteCurrency: value.QuoteCurrency.String(),
	}
	if value.InstrumentID != nil {
		id := value.InstrumentID.String()
		dto.InstrumentID = &id
	}
	return dto
}

func FromMissingInputs(values []domain.MissingInputView) []MissingInputDTO {
	result := make([]MissingInputDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromMissingInput(value))
	}
	return result
}

// AccountValuationDTO mirrors domain.AccountValuation.
type AccountValuationDTO struct {
	Account         AccountDTO              `json:"account"`
	Ownership       []OwnershipShareDTO     `json:"ownership"`
	InstitutionName string                  `json:"institutionName,omitempty"`
	GroupName       string                  `json:"groupName,omitempty"`
	BaseValue       *MoneyView              `json:"baseValue,omitempty"`
	Complete        bool                    `json:"complete"`
	Components      []ValuationComponentDTO `json:"components"`
	MissingInputs   []MissingInputDTO       `json:"missingInputs"`
}

func FromAccountValuation(value domain.AccountValuation) AccountValuationDTO {
	return AccountValuationDTO{
		Account: FromAccount(value.Account), Ownership: FromOwnershipShares(value.Ownership.Shares()),
		InstitutionName: value.InstitutionName, GroupName: value.GroupName, BaseValue: FromMoneyView(value.BaseValue),
		Complete: value.Complete, Components: FromValuationComponents(value.Components), MissingInputs: FromMissingInputs(value.MissingInputs),
	}
}

func FromAccountValuations(values []domain.AccountValuation) []AccountValuationDTO {
	result := make([]AccountValuationDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromAccountValuation(value))
	}
	return result
}

// AllocationDTO mirrors domain.AllocationView.
type AllocationDTO struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Amount   MoneyView `json:"amount"`
	ShareBPS int       `json:"shareBps"`
}

func FromAllocation(value domain.AllocationView) AllocationDTO {
	return AllocationDTO{Key: value.Key, Label: value.Label, Amount: MoneyView{Amount: value.Amount.Amount, Currency: value.Amount.Currency.String()}, ShareBPS: value.ShareBPS}
}

func FromAllocations(values []domain.AllocationView) []AllocationDTO {
	result := make([]AllocationDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromAllocation(value))
	}
	return result
}

// BreakdownDTO mirrors domain.BreakdownItem (an exact decimal.Decimal
// amount, canonicalized here to a string).
type BreakdownDTO struct {
	Key                 string `json:"key"`
	Label               string `json:"label"`
	Amount              string `json:"amount"`
	ShareBPS            int    `json:"shareBps"`
	ClassificationBasis string `json:"classificationBasis,omitempty"`
}

func FromBreakdown(value domain.BreakdownItem) BreakdownDTO {
	amount := value.Amount.String()
	if value.Amount.IsZero() {
		amount = "0"
	}
	return BreakdownDTO{Key: value.Key, Label: value.Label, Amount: amount, ShareBPS: value.ShareBPS, ClassificationBasis: value.ClassificationBasis.String()}
}

func FromBreakdowns(values []domain.BreakdownItem) []BreakdownDTO {
	result := make([]BreakdownDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromBreakdown(value))
	}
	return result
}

// InstrumentDTO mirrors domain.Instrument. Shared by the instrument,
// holding, quote, analytics, and history services.
type InstrumentDTO struct {
	ID             string  `json:"id"`
	HouseholdID    string  `json:"householdId"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	QuoteCurrency  string  `json:"quoteCurrency"`
	Symbol         *string `json:"symbol,omitempty"`
	MarketCode     *string `json:"marketCode,omitempty"`
	CountryCode    *string `json:"countryCode,omitempty"`
	ISIN           *string `json:"isin,omitempty"`
	Note           *string `json:"note,omitempty"`
	IconKey        string  `json:"iconKey"`
	SortOrder      int     `json:"sortOrder"`
	QuoteSource    string  `json:"quoteSource"`
	ProviderKey    *string `json:"providerKey,omitempty"`
	ProviderSymbol *string `json:"providerSymbol,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	ArchivedAt     *string `json:"archivedAt,omitempty"`
}

func FromInstrument(value domain.Instrument) InstrumentDTO {
	dto := InstrumentDTO{
		ID: value.ID.String(), HouseholdID: value.HouseholdID.String(), Name: value.Name, Type: string(value.Type),
		QuoteCurrency: value.QuoteCurrency.String(), Symbol: value.Symbol, MarketCode: value.MarketCode,
		CountryCode: value.CountryCode, ISIN: value.ISIN, Note: value.Note, IconKey: iconValue(value.IconKey, domain.DefaultInstrumentIcon(value.Type)), SortOrder: value.SortOrder,
		QuoteSource: string(value.QuoteSource), ProviderKey: value.ProviderKey, ProviderSymbol: value.ProviderSymbol,
		CreatedAt: FormatTime(value.CreatedAt), UpdatedAt: FormatTime(value.UpdatedAt), ArchivedAt: FormatTimePtr(value.ArchivedAt),
	}
	return dto
}

func iconValue(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

func FromInstruments(values []domain.Instrument) []InstrumentDTO {
	result := make([]InstrumentDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromInstrument(value))
	}
	return result
}

// HoldingDTO mirrors domain.Holding.
type HoldingDTO struct {
	ID           string  `json:"id"`
	AccountID    string  `json:"accountId"`
	InstrumentID string  `json:"instrumentId"`
	Quantity     string  `json:"quantity"`
	Note         *string `json:"note,omitempty"`
	SortOrder    int     `json:"sortOrder"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
	ArchivedAt   *string `json:"archivedAt,omitempty"`
}

func FromHolding(value domain.Holding) HoldingDTO {
	return HoldingDTO{
		ID: value.ID.String(), AccountID: value.AccountID.String(), InstrumentID: value.InstrumentID.String(),
		Quantity: value.Quantity.Canonical(), Note: value.Note, SortOrder: value.SortOrder,
		CreatedAt: FormatTime(value.CreatedAt), UpdatedAt: FormatTime(value.UpdatedAt), ArchivedAt: FormatTimePtr(value.ArchivedAt),
	}
}

func FromHoldings(values []domain.Holding) []HoldingDTO {
	result := make([]HoldingDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromHolding(value))
	}
	return result
}

// AccountCashValueDTO mirrors domain.AccountCashValue.
type AccountCashValueDTO struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"accountId"`
	Amount      MoneyView `json:"amount"`
	EffectiveAt string    `json:"effectiveAt"`
	CreatedAt   string    `json:"createdAt"`
}

func FromAccountCashValue(value domain.AccountCashValue) AccountCashValueDTO {
	return AccountCashValueDTO{
		ID: value.ID.String(), AccountID: value.AccountID.String(), Amount: FromMoney(value.Amount),
		EffectiveAt: FormatTime(value.EffectiveAt), CreatedAt: FormatTime(value.CreatedAt),
	}
}

func FromAccountCashValues(values []domain.AccountCashValue) []AccountCashValueDTO {
	result := make([]AccountCashValueDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromAccountCashValue(value))
	}
	return result
}

// InstrumentQuoteDTO mirrors domain.InstrumentQuote.
type InstrumentQuoteDTO struct {
	ID           string `json:"id"`
	InstrumentID string `json:"instrumentId"`
	UnitPrice    string `json:"unitPrice"`
	Currency     string `json:"currency"`
	SourceKind   string `json:"sourceKind"`
	SourceKey    string `json:"sourceKey"`
	QuotedAt     string `json:"quotedAt"`
	CreatedAt    string `json:"createdAt"`
	Delayed      bool   `json:"delayed"`
}

func FromInstrumentQuote(value domain.InstrumentQuote) InstrumentQuoteDTO {
	return InstrumentQuoteDTO{
		ID: value.ID.String(), InstrumentID: value.InstrumentID.String(), UnitPrice: value.UnitPrice.Canonical(),
		Currency: value.Currency.String(), SourceKind: string(value.SourceKind), SourceKey: value.SourceKey,
		QuotedAt: FormatTime(value.QuotedAt), CreatedAt: FormatTime(value.CreatedAt), Delayed: value.Delayed,
	}
}

func FromInstrumentQuotePtr(value *domain.InstrumentQuote) *InstrumentQuoteDTO {
	if value == nil {
		return nil
	}
	dto := FromInstrumentQuote(*value)
	return &dto
}

func FromInstrumentQuotes(values []domain.InstrumentQuote) []InstrumentQuoteDTO {
	result := make([]InstrumentQuoteDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromInstrumentQuote(value))
	}
	return result
}

// FXQuoteDTO mirrors domain.FXQuote.
type FXQuoteDTO struct {
	ID            string `json:"id"`
	HouseholdID   string `json:"householdId"`
	BaseCurrency  string `json:"baseCurrency"`
	QuoteCurrency string `json:"quoteCurrency"`
	Rate          string `json:"rate"`
	SourceKind    string `json:"sourceKind"`
	SourceKey     string `json:"sourceKey"`
	QuotedAt      string `json:"quotedAt"`
	CreatedAt     string `json:"createdAt"`
	Delayed       bool   `json:"delayed"`
}

func FromFXQuote(value domain.FXQuote) FXQuoteDTO {
	return FXQuoteDTO{
		ID: value.ID.String(), HouseholdID: value.HouseholdID.String(), BaseCurrency: value.BaseCurrency.String(),
		QuoteCurrency: value.QuoteCurrency.String(), Rate: value.Rate.Canonical(), SourceKind: string(value.SourceKind),
		SourceKey: value.SourceKey, QuotedAt: FormatTime(value.QuotedAt), CreatedAt: FormatTime(value.CreatedAt), Delayed: value.Delayed,
	}
}

func FromFXQuotePtr(value *domain.FXQuote) *FXQuoteDTO {
	if value == nil {
		return nil
	}
	dto := FromFXQuote(*value)
	return &dto
}

func FromFXQuotes(values []domain.FXQuote) []FXQuoteDTO {
	result := make([]FXQuoteDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromFXQuote(value))
	}
	return result
}

// FXPreferenceDTO mirrors domain.FXPreference.
type FXPreferenceDTO struct {
	HouseholdID string `json:"householdId"`
	CurrencyA   string `json:"currencyA"`
	CurrencyB   string `json:"currencyB"`
	SourceKind  string `json:"sourceKind"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func FromFXPreference(value domain.FXPreference) FXPreferenceDTO {
	return FXPreferenceDTO{
		HouseholdID: value.HouseholdID.String(), CurrencyA: value.CurrencyA.String(), CurrencyB: value.CurrencyB.String(),
		SourceKind: string(value.SourceKind), CreatedAt: FormatTime(value.CreatedAt), UpdatedAt: FormatTime(value.UpdatedAt),
	}
}

func FromFXPreferences(values []domain.FXPreference) []FXPreferenceDTO {
	result := make([]FXPreferenceDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromFXPreference(value))
	}
	return result
}

// HoldingGainDTO mirrors domain.HoldingGainView.
type HoldingGainDTO struct {
	HoldingID          string           `json:"holdingId"`
	AccountID          string           `json:"accountId"`
	InstrumentID       string           `json:"instrumentId"`
	InstrumentName     string           `json:"instrumentName"`
	InstrumentSymbol   string           `json:"instrumentSymbol,omitempty"`
	Quantity           string           `json:"quantity"`
	AverageCost        MoneyView        `json:"averageCost"`
	TotalCost          MoneyView        `json:"totalCost"`
	TotalCostBase      *MoneyView       `json:"totalCostBase,omitempty"`
	CurrentValue       *MoneyView       `json:"currentValue,omitempty"`
	CurrentValueBase   *MoneyView       `json:"currentValueBase,omitempty"`
	RealizedGain       SignedMoneyView  `json:"realizedGain"`
	UnrealizedGain     *SignedMoneyView `json:"unrealizedGain,omitempty"`
	UnrealizedGainBase *SignedMoneyView `json:"unrealizedGainBase,omitempty"`
	InstrumentMovement *SignedMoneyView `json:"instrumentMovement,omitempty"`
	CurrencyMovement   *SignedMoneyView `json:"currencyMovement,omitempty"`
	Available          bool             `json:"available"`
	MissingReason      string           `json:"missingReason,omitempty"`
}

func FromHoldingGain(value domain.HoldingGainView) HoldingGainDTO {
	return HoldingGainDTO{
		HoldingID: value.HoldingID.String(), AccountID: value.AccountID.String(), InstrumentID: value.InstrumentID.String(),
		InstrumentName: value.InstrumentName, InstrumentSymbol: value.InstrumentSymbol, Quantity: value.Quantity,
		AverageCost:   MoneyView{Amount: value.AverageCost.Amount, Currency: value.AverageCost.Currency.String()},
		TotalCost:     MoneyView{Amount: value.TotalCost.Amount, Currency: value.TotalCost.Currency.String()},
		TotalCostBase: FromMoneyView(value.TotalCostBase), CurrentValue: FromMoneyView(value.CurrentValue),
		CurrentValueBase: FromMoneyView(value.CurrentValueBase),
		RealizedGain:     SignedMoneyView{Amount: value.RealizedGain.Amount, Currency: value.RealizedGain.Currency.String()},
		UnrealizedGain:   FromSignedMoneyView(value.UnrealizedGain), UnrealizedGainBase: FromSignedMoneyView(value.UnrealizedGainBase),
		InstrumentMovement: FromSignedMoneyView(value.InstrumentMovement), CurrencyMovement: FromSignedMoneyView(value.CurrencyMovement),
		Available: value.Available, MissingReason: value.MissingReason,
	}
}

func FromHoldingGains(values []domain.HoldingGainView) []HoldingGainDTO {
	result := make([]HoldingGainDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromHoldingGain(value))
	}
	return result
}

// AccountGainDTO mirrors domain.AccountGainView.
type AccountGainDTO struct {
	AccountID      string           `json:"accountId"`
	Holdings       []HoldingGainDTO `json:"holdings"`
	TotalCost      *MoneyView       `json:"totalCost,omitempty"`
	CurrentValue   *MoneyView       `json:"currentValue,omitempty"`
	RealizedGain   *SignedMoneyView `json:"realizedGain,omitempty"`
	UnrealizedGain *SignedMoneyView `json:"unrealizedGain,omitempty"`
	Available      bool             `json:"available"`
	MissingReason  string           `json:"missingReason,omitempty"`
}

func FromAccountGain(value domain.AccountGainView) AccountGainDTO {
	return AccountGainDTO{
		AccountID: value.AccountID.String(), Holdings: FromHoldingGains(value.Holdings),
		TotalCost: FromMoneyView(value.TotalCost), CurrentValue: FromMoneyView(value.CurrentValue),
		RealizedGain: FromSignedMoneyView(value.RealizedGain), UnrealizedGain: FromSignedMoneyView(value.UnrealizedGain),
		Available: value.Available, MissingReason: value.MissingReason,
	}
}

// GainGroupDTO mirrors domain.GainGroupView.
type GainGroupDTO struct {
	Key           string          `json:"key"`
	Label         string          `json:"label"`
	Gain          SignedMoneyView `json:"gain"`
	Available     bool            `json:"available"`
	MissingReason string          `json:"missingReason,omitempty"`
}

func FromGainGroup(value domain.GainGroupView) GainGroupDTO {
	return GainGroupDTO{
		Key: value.Key, Label: value.Label, Gain: SignedMoneyView{Amount: value.Gain.Amount, Currency: value.Gain.Currency.String()},
		Available: value.Available, MissingReason: value.MissingReason,
	}
}

func FromGainGroups(values []domain.GainGroupView) []GainGroupDTO {
	result := make([]GainGroupDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromGainGroup(value))
	}
	return result
}

// RealizedGainDTO mirrors domain.RealizedGainView.
type RealizedGainDTO struct {
	From          string         `json:"from"`
	To            string         `json:"to"`
	Currency      string         `json:"currency,omitempty"`
	ByInstrument  []GainGroupDTO `json:"byInstrument"`
	ByAccount     []GainGroupDTO `json:"byAccount"`
	Available     bool           `json:"available"`
	MissingReason string         `json:"missingReason,omitempty"`
}

func FromRealizedGain(value domain.RealizedGainView) RealizedGainDTO {
	return RealizedGainDTO{
		From: value.From, To: value.To, Currency: value.Currency.String(),
		ByInstrument: FromGainGroups(value.ByInstrument), ByAccount: FromGainGroups(value.ByAccount),
		Available: value.Available, MissingReason: value.MissingReason,
	}
}

// StartingPointHoldingDTO mirrors domain.StartingPointHoldingView.
type StartingPointHoldingDTO struct {
	HoldingID      string `json:"holdingId"`
	InstrumentID   string `json:"instrumentId"`
	InstrumentName string `json:"instrumentName"`
	Currency       string `json:"currency"`
	Quantity       string `json:"quantity"`
	UnitCost       string `json:"unitCost,omitempty"`
}

func FromStartingPointHolding(value domain.StartingPointHoldingView) StartingPointHoldingDTO {
	return StartingPointHoldingDTO{
		HoldingID: value.HoldingID.String(), InstrumentID: value.InstrumentID.String(), InstrumentName: value.InstrumentName,
		Currency: value.Currency.String(), Quantity: value.Quantity, UnitCost: value.UnitCost,
	}
}

func FromStartingPointHoldings(values []domain.StartingPointHoldingView) []StartingPointHoldingDTO {
	result := make([]StartingPointHoldingDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromStartingPointHolding(value))
	}
	return result
}

// ActivityEffectDTO mirrors domain.ActivityEffect.
type ActivityEffectDTO struct {
	ID             string     `json:"id"`
	ActivityID     string     `json:"activityId"`
	Sequence       int        `json:"sequence"`
	Role           string     `json:"role"`
	Direction      string     `json:"direction"`
	Target         string     `json:"target"`
	Classification string     `json:"classification"`
	AccountID      *string    `json:"accountId,omitempty"`
	HoldingID      *string    `json:"holdingId,omitempty"`
	InstrumentID   *string    `json:"instrumentId,omitempty"`
	Money          *MoneyView `json:"money,omitempty"`
	Quantity       *string    `json:"quantity,omitempty"`
	CostUnitPrice  *string    `json:"costUnitPrice,omitempty"`
}

func FromActivityEffect(value domain.ActivityEffect) ActivityEffectDTO {
	dto := ActivityEffectDTO{
		ID: value.ID.String(), ActivityID: value.ActivityID.String(), Sequence: value.Sequence,
		Role: string(value.Role), Direction: string(value.Direction), Target: string(value.Target),
		Classification: string(value.Classification), Money: FromMoneyPtr(value.Money),
	}
	if value.AccountID != nil {
		id := value.AccountID.String()
		dto.AccountID = &id
	}
	if value.HoldingID != nil {
		id := value.HoldingID.String()
		dto.HoldingID = &id
	}
	if value.InstrumentID != nil {
		id := value.InstrumentID.String()
		dto.InstrumentID = &id
	}
	if value.Quantity != nil {
		quantity := value.Quantity.Canonical()
		dto.Quantity = &quantity
	}
	if value.CostUnitPrice != nil {
		cost := value.CostUnitPrice.Canonical()
		dto.CostUnitPrice = &cost
	}
	return dto
}

func FromActivityEffects(values []domain.ActivityEffect) []ActivityEffectDTO {
	result := make([]ActivityEffectDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromActivityEffect(value))
	}
	return result
}

// TradeDetailDTO mirrors domain.TradeDetail.
type TradeDetailDTO struct {
	Side         string     `json:"side"`
	InstrumentID string     `json:"instrumentId"`
	HoldingID    string     `json:"holdingId"`
	Quantity     string     `json:"quantity"`
	Gross        MoneyView  `json:"gross"`
	UnitPrice    string     `json:"unitPrice"`
	Fee          *MoneyView `json:"fee,omitempty"`
}

func FromTradeDetail(value *domain.TradeDetail) *TradeDetailDTO {
	if value == nil {
		return nil
	}
	return &TradeDetailDTO{
		Side: string(value.Side), InstrumentID: value.InstrumentID.String(), HoldingID: value.HoldingID.String(),
		Quantity: value.Quantity.Canonical(), Gross: FromMoney(value.Gross), UnitPrice: value.UnitPrice.Canonical(),
		Fee: FromMoneyPtr(value.Fee),
	}
}

// ActivityDTO mirrors domain.Activity.
type ActivityDTO struct {
	ID                 string              `json:"id"`
	HouseholdID        string              `json:"householdId"`
	Kind               string              `json:"kind"`
	Reason             string              `json:"reason"`
	EffectiveAt        string              `json:"effectiveAt"`
	EffectiveLocalDate string              `json:"effectiveLocalDate"`
	CreatedAt          string              `json:"createdAt"`
	Note               *string             `json:"note,omitempty"`
	ReversesActivityID *string             `json:"reversesActivityId,omitempty"`
	CorrectionGroupID  *string             `json:"correctionGroupId,omitempty"`
	TransactionFXRate  *string             `json:"transactionFxRate,omitempty"`
	TradeDetail        *TradeDetailDTO     `json:"tradeDetail,omitempty"`
	Effects            []ActivityEffectDTO `json:"effects"`
}

func FromActivity(value domain.Activity) ActivityDTO {
	dto := ActivityDTO{
		ID: value.ID.String(), HouseholdID: value.HouseholdID.String(), Kind: string(value.Kind), Reason: string(value.Reason),
		EffectiveAt: FormatTime(value.EffectiveAt), EffectiveLocalDate: value.EffectiveLocalDate, CreatedAt: FormatTime(value.CreatedAt),
		Note: value.Note, TradeDetail: FromTradeDetail(value.TradeDetail), Effects: FromActivityEffects(value.Effects),
	}
	if value.ReversesActivityID != nil {
		id := value.ReversesActivityID.String()
		dto.ReversesActivityID = &id
	}
	if value.CorrectionGroupID != nil {
		id := value.CorrectionGroupID.String()
		dto.CorrectionGroupID = &id
	}
	if value.TransactionFXRate != nil {
		rate := value.TransactionFXRate.Canonical()
		dto.TransactionFXRate = &rate
	}
	return dto
}

func FromActivities(values []domain.Activity) []ActivityDTO {
	result := make([]ActivityDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromActivity(value))
	}
	return result
}

// EndpointViewDTO mirrors domain.EndpointView.
type EndpointViewDTO struct {
	Target    string  `json:"target"`
	AccountID *string `json:"accountId,omitempty"`
	HoldingID *string `json:"holdingId,omitempty"`
	Name      string  `json:"name"`
	Amount    string  `json:"amount,omitempty"`
	Quantity  string  `json:"quantity,omitempty"`
	Currency  string  `json:"currency,omitempty"`
}

func FromEndpointView(value domain.EndpointView) EndpointViewDTO {
	dto := EndpointViewDTO{Target: string(value.Target), Name: value.Name, Amount: value.Amount, Quantity: value.Quantity, Currency: value.Currency.String()}
	if value.AccountID != nil {
		id := value.AccountID.String()
		dto.AccountID = &id
	}
	if value.HoldingID != nil {
		id := value.HoldingID.String()
		dto.HoldingID = &id
	}
	return dto
}

func FromEndpointViews(values []domain.EndpointView) []EndpointViewDTO {
	result := make([]EndpointViewDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FromEndpointView(value))
	}
	return result
}

// ChangePreviewDTO mirrors domain.ChangePreview: the result of previewing,
// recording, undoing, or fixing a change.
type ChangePreviewDTO struct {
	Activity         ActivityDTO         `json:"activity"`
	Effects          []ActivityEffectDTO `json:"effects"`
	Resulting        []EndpointViewDTO   `json:"resulting"`
	DerivedRate      *string             `json:"derivedRate,omitempty"`
	DerivedUnitPrice *string             `json:"derivedUnitPrice,omitempty"`
}

func FromChangePreview(value domain.ChangePreview) ChangePreviewDTO {
	dto := ChangePreviewDTO{
		Activity: FromActivity(value.Activity), Effects: FromActivityEffects(value.Effects), Resulting: FromEndpointViews(value.Resulting),
	}
	if value.DerivedRate != nil {
		rate := value.DerivedRate.Canonical()
		dto.DerivedRate = &rate
	}
	if value.DerivedUnitPrice != nil {
		price := value.DerivedUnitPrice.Canonical()
		dto.DerivedUnitPrice = &price
	}
	return dto
}

// ActivityPageDTO mirrors domain.ActivityPage.
type ActivityCursorDTO struct {
	EffectiveAt string `json:"effectiveAt"`
	CreatedAt   string `json:"createdAt"`
	ID          string `json:"id"`
}

func FromActivityCursor(value *domain.ActivityCursor) *ActivityCursorDTO {
	if value == nil {
		return nil
	}
	return &ActivityCursorDTO{EffectiveAt: FormatTime(value.EffectiveAt), CreatedAt: FormatTime(value.CreatedAt), ID: value.ID.String()}
}

type ActivityPageDTO struct {
	Activities []ActivityDTO      `json:"activities"`
	Next       *ActivityCursorDTO `json:"next,omitempty"`
	HasMore    bool               `json:"hasMore"`
}

func FromActivityPage(value domain.ActivityPage) ActivityPageDTO {
	return ActivityPageDTO{Activities: FromActivities(value.Activities), Next: FromActivityCursor(value.Next), HasMore: value.HasMore}
}
