package application

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/csvcodec"
)

const (
	CSVProfileAccounts = "accounts"
	CSVProfileHoldings = "holdings"
	CSVDateISO         = "iso"
	CSVDateDayFirst    = "day-first"
	CSVDateMonthFirst  = "month-first"
)

var AccountsCSVHeaders = []string{
	"account_name", "account_type", "balance_sheet_role", "tracking_mode", "currency",
	"current_value", "value_date", "ownership", "include_in_net_worth", "institution_name",
	"institution_type", "group_name", "include_in_portfolio", "include_in_liquid_assets",
	"icon_key", "note",
}

var HoldingsCSVHeaders = []string{
	"account_name", "instrument_type", "instrument_name", "quantity", "quote_currency",
	"unit_price", "quote_date", "symbol", "market_code", "country_code", "isin", "note",
}

type CSVParseOptions struct {
	Delimiter   rune
	DateFormat  string
	DecimalSep  string
	GroupingSep string
	Unresolved  []CSVUnresolvedAction
}

const (
	CSVUnresolvedBlock  = "block"
	CSVUnresolvedCreate = "create"
	CSVUnresolvedMap    = "map"
)

type CSVUnresolvedAction struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Action string `json:"action"`
	MapTo  string `json:"mapTo,omitempty"`
}

type CSVUnresolvedName struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Field string `json:"field"`
	Row   int    `json:"row"`
}

type CSVRowError struct {
	Row        int    `json:"row"`
	Field      string `json:"field"`
	Source     string `json:"source,omitempty"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type CSVPreviewStats struct {
	CreateAccounts     int `json:"createAccounts"`
	CreateHoldings     int `json:"createHoldings"`
	CreateInstruments  int `json:"createInstruments"`
	CreateMembers      int `json:"createMembers"`
	CreateInstitutions int `json:"createInstitutions"`
	CreateGroups       int `json:"createGroups"`
	References         int `json:"references"`
	Warnings           int `json:"warnings"`
	Errors             int `json:"errors"`
	Duplicates         int `json:"duplicates"`
}

type CSVWarning struct {
	Row     int    `json:"row"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CSVImportPlan struct {
	Profile     string
	Stats       CSVPreviewStats
	Errors      []CSVRowError
	Warnings    []CSVWarning
	Unresolved  []CSVUnresolvedName
	PreviewRows [][]string
	Headers     []string
	Batch       domain.CSVImportBatch
}

type BackupSidePreview struct {
	HouseholdName string
	BaseCurrency  string
	Accounts      int
	Holdings      int
	Activities    int
}

func (s *Service) ExportAccountsCSV(ctx context.Context, includeArchived bool) ([]byte, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: includeArchived})
	if err != nil {
		return nil, err
	}
	if snapshot.Household == nil {
		return nil, onboardingRequired()
	}
	memberNames := map[domain.MemberID]string{}
	for _, member := range snapshot.Members {
		memberNames[member.ID] = member.Name
	}
	institutionTypes := map[domain.InstitutionID]string{}
	for _, institution := range snapshot.Institutions {
		institutionTypes[institution.ID] = string(institution.InstitutionType)
	}
	records := append([]domain.AccountRecord(nil), snapshot.Accounts...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].Account.Name != records[j].Account.Name {
			return records[i].Account.Name < records[j].Account.Name
		}
		return records[i].Account.ID.String() < records[j].Account.ID.String()
	})
	rows := make([][]string, 0, len(records))
	for _, record := range records {
		value, valueDate := "", ""
		if record.Account.TrackingMode != domain.TrackingHoldings && record.LatestValue != nil {
			value = record.LatestValue.Amount.CanonicalAmount()
			valueDate = calendarDate(record.LatestValue.EffectiveAt, snapshot.Origin)
		}
		institutionType := ""
		if record.Account.InstitutionID != nil {
			institutionType = institutionTypes[*record.Account.InstitutionID]
		}
		rows = append(rows, []string{
			record.Account.Name,
			record.Account.AccountType.String(),
			string(record.Account.BalanceSheetRole),
			string(record.Account.TrackingMode),
			record.Account.DefaultCurrency.String(),
			value,
			valueDate,
			formatOwnershipCSV(record.Ownership, memberNames),
			boolCSV(record.Account.IncludeInNetWorth),
			record.InstitutionName,
			institutionType,
			record.GroupName,
			boolCSV(record.Account.IncludeInPortfolio),
			boolCSV(record.Account.IncludeInLiquidAssets),
			deref(record.Account.IconKey),
			deref(record.Account.Note),
		})
	}
	return csvcodec.Encode(AccountsCSVHeaders, rows)
}

func (s *Service) ExportHoldingsCSV(ctx context.Context, includeArchived bool) ([]byte, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: includeArchived})
	if err != nil {
		return nil, err
	}
	if snapshot.Household == nil {
		return nil, onboardingRequired()
	}
	records := snapshot.Accounts
	instrumentByID := map[domain.InstrumentID]domain.Instrument{}
	for _, instrument := range snapshot.Instruments {
		if !includeArchived && instrument.ArchivedAt != nil {
			continue
		}
		instrumentByID[instrument.ID] = instrument
	}
	quotesByInstrument := map[domain.InstrumentID][]domain.InstrumentQuote{}
	for _, quote := range snapshot.InstrumentQuotes {
		quotesByInstrument[quote.InstrumentID] = append(quotesByInstrument[quote.InstrumentID], quote)
	}
	type holdingRow struct {
		accountName string
		holding     domain.Holding
		instrument  domain.Instrument
	}
	var items []holdingRow
	for _, record := range records {
		if record.Account.TrackingMode != domain.TrackingHoldings {
			continue
		}
		for _, holding := range snapshot.Holdings {
			if holding.AccountID != record.Account.ID || (!includeArchived && holding.ArchivedAt != nil) {
				continue
			}
			instrument, ok := instrumentByID[holding.InstrumentID]
			if !ok {
				continue
			}
			items = append(items, holdingRow{accountName: record.Account.Name, holding: holding, instrument: instrument})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].accountName != items[j].accountName {
			return items[i].accountName < items[j].accountName
		}
		if items[i].instrument.Name != items[j].instrument.Name {
			return items[i].instrument.Name < items[j].instrument.Name
		}
		return items[i].holding.ID.String() < items[j].holding.ID.String()
	})
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		unitPrice, quoteDate := "", ""
		if quote := latestManualQuote(quotesByInstrument[item.instrument.ID]); quote != nil {
			unitPrice = quote.UnitPrice.Canonical()
			quoteDate = calendarDate(quote.QuotedAt, snapshot.Origin)
		}
		rows = append(rows, []string{
			item.accountName,
			string(item.instrument.Type),
			item.instrument.Name,
			item.holding.Quantity.Canonical(),
			item.instrument.QuoteCurrency.String(),
			unitPrice,
			quoteDate,
			deref(item.instrument.Symbol),
			deref(item.instrument.MarketCode),
			deref(item.instrument.CountryCode),
			deref(item.instrument.ISIN),
			deref(item.holding.Note),
		})
	}
	return csvcodec.Encode(HoldingsCSVHeaders, rows)
}

func (s *Service) CommitCSVImport(ctx context.Context, plan CSVImportPlan) (CSVPreviewStats, error) {
	return s.CommitCSVImportBuilt(ctx, func() (CSVImportPlan, error) { return plan, nil })
}

func (s *Service) CommitCSVImportBuilt(ctx context.Context, build func() (CSVImportPlan, error)) (CSVPreviewStats, error) {
	if err := s.BeginExclusiveOperation(); err != nil {
		return CSVPreviewStats{}, err
	}
	defer s.EndExclusiveOperation()
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	plan, err := build()
	if err != nil {
		return CSVPreviewStats{}, err
	}
	if len(plan.Errors) > 0 {
		return CSVPreviewStats{}, &domain.Error{Code: domain.ErrCSVRowInvalid, Message: "CSV has blocking errors"}
	}
	if _, err := s.requireHousehold(ctx); err != nil {
		return CSVPreviewStats{}, err
	}
	if err := s.repository.CommitCSVImport(ctx, plan.Batch); err != nil {
		return CSVPreviewStats{}, err
	}
	return plan.Stats, nil
}

func (s *Service) CurrentDatabasePreview(ctx context.Context) (BackupSidePreview, error) {
	household, err := s.repository.Household(ctx)
	if err != nil {
		return BackupSidePreview{}, err
	}
	preview := BackupSidePreview{}
	if household == nil {
		return preview, nil
	}
	preview.HouseholdName = household.Name
	preview.BaseCurrency = household.BaseCurrency.String()
	accounts, holdings, activities, err := s.repository.PreviewCounts(ctx)
	if err != nil {
		return BackupSidePreview{}, err
	}
	preview.Accounts = accounts
	preview.Holdings = holdings
	preview.Activities = activities
	return preview, nil
}

func calendarDate(value time.Time, origin *domain.HistoryOrigin) string {
	location := time.UTC
	if origin != nil {
		if loaded, err := time.LoadLocation(origin.Timezone); err == nil {
			location = loaded
		}
	}
	return value.In(location).Format("2006-01-02")
}

func boolCSV(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatOwnershipCSV(ownership domain.Ownership, names map[domain.MemberID]string) string {
	parts := make([]string, 0, len(ownership.Shares()))
	for _, share := range ownership.Shares() {
		name := names[share.MemberID]
		if name == "" {
			name = share.MemberID.String()
		}
		parts = append(parts, escapeOwnershipName(name)+":"+formatBPSPercent(share.ShareBPS))
	}
	return strings.Join(parts, ";")
}

func escapeOwnershipName(name string) string {
	var escaped strings.Builder
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '\\', ':', ';':
			escaped.WriteByte('\\')
		}
		escaped.WriteByte(name[i])
	}
	return escaped.String()
}

func splitOwnershipParts(value string) []string {
	parts := make([]string, 0, 1)
	var current strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && (value[i+1] == '\\' || value[i+1] == ':' || value[i+1] == ';') {
			current.WriteByte(value[i])
			current.WriteByte(value[i+1])
			i++
			continue
		}
		if value[i] == ';' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(value[i])
	}
	return append(parts, current.String())
}

func ownershipColonIndex(value string) int {
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && (value[i+1] == '\\' || value[i+1] == ':' || value[i+1] == ';') {
			i++
			continue
		}
		if value[i] == ':' {
			return i
		}
	}
	return -1
}

func unescapeOwnershipName(value string) string {
	var result strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && (value[i+1] == '\\' || value[i+1] == ':' || value[i+1] == ';') {
			result.WriteByte(value[i+1])
			i++
			continue
		}
		result.WriteByte(value[i])
	}
	return result.String()
}

func formatBPSPercent(bps int) string {
	whole := bps / 100
	frac := bps % 100
	if frac == 0 {
		return strconv.Itoa(whole) + "%"
	}
	if frac%10 == 0 {
		return strconv.Itoa(whole) + "." + strconv.Itoa(frac/10) + "%"
	}
	fracText := strconv.Itoa(frac)
	if frac < 10 {
		fracText = "0" + fracText
	}
	return strconv.Itoa(whole) + "." + fracText + "%"
}

func previewRows(rows [][]string) [][]string {
	if len(rows) <= csvcodec.PreviewRowCap {
		return rows
	}
	return rows[:csvcodec.PreviewRowCap]
}

func latestManualQuote(quotes []domain.InstrumentQuote) *domain.InstrumentQuote {
	var latest *domain.InstrumentQuote
	for i := range quotes {
		quote := quotes[i]
		if quote.SourceKind != domain.QuoteSourceManual {
			continue
		}
		if latest == nil || quote.QuotedAt.After(latest.QuotedAt) {
			copyQuote := quote
			latest = &copyQuote
		}
	}
	return latest
}
