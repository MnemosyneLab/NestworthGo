package application

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/csvcodec"
)

func (s *Service) BuildCSVImportPlan(ctx context.Context, profile string, table csvcodec.Table, mapping map[string]string, options CSVParseOptions, seed *CSVImportPlan) (CSVImportPlan, error) {
	if err := validateCSVParseOptions(options); err != nil {
		return CSVImportPlan{}, err
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return CSVImportPlan{}, err
	}
	origin, err := s.repository.HistoryOrigin(ctx, household.ID)
	if err != nil {
		return CSVImportPlan{}, err
	}
	now := s.clock()
	plan := CSVImportPlan{Profile: profile, Headers: table.Headers, PreviewRows: previewRows(table.Rows), Batch: domain.CSVImportBatch{AsOf: now}}
	if seed != nil {
		plan.Batch = seed.Batch
		plan.Batch.AsOf = now
		plan.Stats = seed.Stats
		plan.Errors = append([]CSVRowError{}, seed.Errors...)
		plan.Warnings = append([]CSVWarning{}, seed.Warnings...)
		plan.Unresolved = append([]CSVUnresolvedName{}, seed.Unresolved...)
	}
	column, err := resolveMapping(table.Headers, mapping, profile)
	if err != nil {
		return CSVImportPlan{}, err
	}
	members, err := s.repository.ListMembers(ctx, false)
	if err != nil {
		return CSVImportPlan{}, err
	}
	institutions, err := s.repository.ListInstitutions(ctx, false)
	if err != nil {
		return CSVImportPlan{}, err
	}
	groups, err := s.repository.ListGroups(ctx, false)
	if err != nil {
		return CSVImportPlan{}, err
	}
	accounts, err := s.repository.ListAccountRecords(ctx, household.ID, domain.AccountFilter{IncludeArchived: false})
	if err != nil {
		return CSVImportPlan{}, err
	}
	instruments, err := s.repository.ListInstruments(ctx, household.ID, false)
	if err != nil {
		return CSVImportPlan{}, err
	}
	switch profile {
	case CSVProfileAccounts:
		planAccountsCSV(&plan, household, origin, now, table, column, options, members, institutions, groups, accounts)
	case CSVProfileHoldings:
		existingHoldings := map[string]struct{}{}
		for _, record := range accounts {
			if record.Account.TrackingMode != domain.TrackingHoldings {
				continue
			}
			holdings, listErr := s.repository.ListHoldings(ctx, record.Account.ID, false)
			if listErr != nil {
				return CSVImportPlan{}, listErr
			}
			for _, holding := range holdings {
				name := holding.InstrumentID.String()
				for _, instrument := range instruments {
					if instrument.ID == holding.InstrumentID {
						name = instrument.Name
						break
					}
				}
				existingHoldings[record.Account.Name+"\x00"+name] = struct{}{}
			}
		}
		planHoldingsCSV(&plan, household, origin, now, table, column, options, accounts, instruments, existingHoldings)
	default:
		return CSVImportPlan{}, &domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "CSV profile is not supported"}
	}
	plan.Stats.Errors = len(plan.Errors)
	plan.Stats.Warnings = len(plan.Warnings)
	return plan, nil
}

func resolveMapping(headers []string, mapping map[string]string, profile string) (map[string]int, error) {
	required := AccountsCSVHeaders
	if profile == CSVProfileHoldings {
		required = HoldingsCSVHeaders
	}
	used := map[int]string{}
	column := map[string]int{}
	for _, target := range required {
		source := target
		if mapping != nil {
			if mapped, ok := mapping[target]; ok && strings.TrimSpace(mapped) != "" {
				source = mapped
			}
		}
		index := csvcodec.HeaderIndex(headers, source)
		if index < 0 {
			if isOptionalCSVField(target) {
				continue
			}
			return nil, &domain.Error{Code: domain.ErrCSVMappingRequired, Field: target, Message: "required CSV field is not mapped"}
		}
		if previous, taken := used[index]; taken && previous != target {
			return nil, &domain.Error{Code: domain.ErrCSVMappingConflict, Field: target, Message: "a source column is mapped to more than one field"}
		}
		used[index] = target
		column[target] = index
	}
	return column, nil
}

func isOptionalCSVField(name string) bool {
	switch name {
	case "current_value", "value_date", "include_in_net_worth", "institution_name", "institution_type", "group_name",
		"include_in_portfolio", "include_in_liquid_assets", "icon_key", "note",
		"instrument_type", "quote_currency", "unit_price", "quote_date", "symbol", "market_code", "country_code", "isin":
		return true
	default:
		return false
	}
}

func cell(row []string, column map[string]int, name string) string {
	index, ok := column[name]
	if !ok || index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func planAccountsCSV(plan *CSVImportPlan, household domain.Household, origin *domain.HistoryOrigin, now time.Time, table csvcodec.Table, column map[string]int, options CSVParseOptions, members []domain.Member, institutions []domain.Institution, groups []domain.Group, accounts []domain.AccountRecord) {
	memberByName := uniqueNameIndex(len(members), func(i int) string { return members[i].Name })
	institutionByName := uniqueNameIndex(len(institutions), func(i int) string { return institutions[i].Name })
	groupByName := uniqueNameIndex(len(groups), func(i int) string { return groups[i].Name })
	seenFile := map[string]int{}
	existingKeys := map[string]struct{}{}
	for _, record := range accounts {
		existingKeys[accountConflictKey(record.Account.Name, record.Account.DefaultCurrency.String(), record.InstitutionName)] = struct{}{}
	}
	createdMembers := map[string]domain.Member{}
	createdInstitutions := map[string]domain.Institution{}
	createdGroups := map[string]domain.Group{}
	for i, row := range table.Rows {
		rowNumber := i + 2
		name := cell(row, column, "account_name")
		accountType, typeErr := domain.ParseAccountType(cell(row, column, "account_type"))
		role, roleErr := domain.ParseBalanceSheetRole(cell(row, column, "balance_sheet_role"))
		mode, modeErr := domain.ParseTrackingMode(cell(row, column, "tracking_mode"))
		currency, currencyErr := domain.ParseSupportedCurrency(strings.ToUpper(cell(row, column, "currency")))
		if name == "" {
			appendCSVError(plan, rowNumber, "account_name", cell(row, column, "account_name"), domain.ErrCSVRowInvalid, "account name is required")
			continue
		}
		if typeErr != nil {
			appendCSVError(plan, rowNumber, "account_type", cell(row, column, "account_type"), domain.ErrCSVRowInvalid, "account type is not valid")
			continue
		}
		if roleErr != nil {
			appendCSVError(plan, rowNumber, "balance_sheet_role", cell(row, column, "balance_sheet_role"), domain.ErrCSVRowInvalid, "balance sheet role is not valid")
			continue
		}
		if modeErr != nil {
			appendCSVError(plan, rowNumber, "tracking_mode", cell(row, column, "tracking_mode"), domain.ErrCSVRowInvalid, "tracking mode is not valid")
			continue
		}
		if currencyErr != nil {
			appendCSVError(plan, rowNumber, "currency", cell(row, column, "currency"), domain.ErrCSVRowInvalid, "currency is not valid")
			continue
		}
		valueText := normalizeNumber(cell(row, column, "current_value"), options)
		valueDate := cell(row, column, "value_date")
		if mode == domain.TrackingHoldings {
			if valueText != "" || valueDate != "" {
				appendCSVError(plan, rowNumber, "current_value", valueText, domain.ErrCSVRowInvalid, "holdings accounts must leave current value empty")
				continue
			}
		} else if valueText == "" || valueDate == "" {
			appendCSVError(plan, rowNumber, "current_value", valueText, domain.ErrCSVRowInvalid, "balance and manual value accounts require current value and date")
			continue
		}
		institutionName := cell(row, column, "institution_name")
		conflictKey := accountConflictKey(name, currency.String(), institutionName)
		if previous, exists := seenFile[conflictKey]; exists {
			appendCSVError(plan, rowNumber, "account_name", name, domain.ErrCSVDuplicate, "this account already appears in the file")
			plan.Stats.Duplicates++
			_ = previous
			continue
		}
		if _, exists := existingKeys[conflictKey]; exists {
			appendCSVError(plan, rowNumber, "account_name", name, domain.ErrCSVDuplicate, "an account with this name, currency, and institution already exists")
			plan.Stats.Duplicates++
			continue
		}
		seenFile[conflictKey] = rowNumber
		var institutionID *domain.InstitutionID
		if institutionName != "" {
			if ids, ok := institutionByName[institutionName]; ok {
				if len(ids) != 1 {
					appendCSVError(plan, rowNumber, "institution_name", institutionName, domain.ErrCSVReferenceUnresolved, "institution name is ambiguous")
					continue
				}
				id := institutions[ids[0]].ID
				institutionID = &id
				plan.Stats.References++
			} else if created, ok := createdInstitutions[institutionName]; ok {
				institutionID = &created.ID
				plan.Stats.References++
			} else {
				id, ok := resolveNewInstitution(plan, household, now, options, institutions, institutionByName, createdInstitutions, rowNumber, institutionName, cell(row, column, "institution_type"))
				if !ok {
					continue
				}
				institutionID = id
			}
		}
		var groupID *domain.GroupID
		groupName := cell(row, column, "group_name")
		if groupName != "" {
			if ids, ok := groupByName[groupName]; ok {
				if len(ids) != 1 {
					appendCSVError(plan, rowNumber, "group_name", groupName, domain.ErrCSVReferenceUnresolved, "group name is ambiguous")
					continue
				}
				id := groups[ids[0]].ID
				groupID = &id
				plan.Stats.References++
			} else if created, ok := createdGroups[groupName]; ok {
				groupID = &created.ID
				plan.Stats.References++
			} else {
				id, ok := resolveNewGroup(plan, household, now, options, groups, groupByName, createdGroups, rowNumber, groupName)
				if !ok {
					continue
				}
				groupID = id
			}
		}
		shares, shareErr := parseOwnershipCSV(cell(row, column, "ownership"), household, now, options, members, memberByName, createdMembers, plan, rowNumber)
		if shareErr != nil {
			continue
		}
		defaults := domain.SuggestedInclusion(accountType, mode)
		includeNet, netSet, netErr := parseOptionalBool(cell(row, column, "include_in_net_worth"))
		includePort, portSet, portErr := parseOptionalBool(cell(row, column, "include_in_portfolio"))
		includeLiq, liqSet, liqErr := parseOptionalBool(cell(row, column, "include_in_liquid_assets"))
		if netErr != nil || portErr != nil || liqErr != nil {
			appendCSVError(plan, rowNumber, "include_in_net_worth", cell(row, column, "include_in_net_worth"), domain.ErrCSVRowInvalid, "inclusion flag is not valid")
			continue
		}
		if !netSet {
			includeNet = defaults.IncludeInNetWorth
		}
		if !portSet {
			includePort = defaults.IncludeInPortfolio
		}
		if !liqSet {
			includeLiq = defaults.IncludeInLiquidAssets
		}
		initialAmount := valueText
		if mode == domain.TrackingHoldings {
			initialAmount = ""
		}
		var iconKey *string
		if icon := cell(row, column, "icon_key"); icon != "" {
			iconKey = &icon
		}
		var note *string
		if noteText := cell(row, column, "note"); noteText != "" {
			note = &noteText
		}
		historyInitialNonZero := false
		if origin != nil && mode != domain.TrackingHoldings && initialAmount != "" {
			parsed, parseErr := domain.ParseMoney(initialAmount, currency)
			if parseErr == nil && !parsed.IsZero() {
				historyInitialNonZero = true
				initialAmount = "0"
			}
		}
		account, ownership, initial, accountErr := domain.NewAccount(domain.AccountInput{
			HouseholdID: household.ID, InstitutionID: institutionID, GroupID: groupID, Name: name,
			AccountType: accountType, BalanceSheetRole: role, TrackingMode: mode, DefaultCurrency: currency,
			Note: note, IconKey: iconKey, IncludeInNetWorth: includeNet, IncludeInPortfolio: includePort,
			IncludeInLiquidAssets: includeLiq, Ownership: shares, InitialAmount: initialAmount,
		}, now)
		if accountErr != nil {
			appendCSVError(plan, rowNumber, "account_name", name, domain.ErrCSVRowInvalid, accountErr.Error())
			continue
		}
		imported := domain.CSVImportedAccount{Account: account, Ownership: ownership}
		if mode != domain.TrackingHoldings {
			effectiveAt, dateErr := observationTime(valueDate, options.DateFormat, origin, now)
			if dateErr != nil {
				appendCSVError(plan, rowNumber, "value_date", valueDate, domain.ErrCSVRowInvalid, dateErr.Error())
				continue
			}
			amount := *initial
			if historyInitialNonZero {
				parsed, parseErr := domain.ParseMoney(valueText, currency)
				if parseErr != nil {
					appendCSVError(plan, rowNumber, "current_value", valueText, domain.ErrCSVRowInvalid, "current value is not valid")
					continue
				}
				amount = parsed
			}
			if historyInitialNonZero {
				zero, _ := domain.ParseMoney("0", currency)
				created, valueErr := domain.NewAccountValue(account, zero, effectiveAt, now)
				if valueErr != nil {
					appendCSVError(plan, rowNumber, "current_value", valueText, domain.ErrCSVRowInvalid, valueErr.Error())
					continue
				}
				imported.Value = &created
				state := domain.ChangeState{HouseholdID: household.ID, OriginAt: origin.StartedAt, Timezone: origin.Timezone, Now: now, Accounts: map[domain.AccountID]domain.ChangeAccountState{account.ID: {ID: account.ID, Name: account.Name, Currency: account.DefaultCurrency, Mode: account.TrackingMode, Liability: account.IsLiability(), Current: zero}}, Cash: make(map[domain.AccountID]map[domain.CurrencyCode]domain.Money), Holdings: make(map[domain.HoldingID]domain.ChangeHoldingState)}
				preview, previewErr := domain.PreviewChange(state, domain.MoneyAddedInput{HouseholdID: household.ID, AccountID: account.ID, Amount: amount, Reason: domain.ReasonContribution, EffectiveAt: effectiveAt})
				if previewErr != nil {
					appendCSVError(plan, rowNumber, "value_date", valueDate, domain.ErrCSVRowInvalid, previewErr.Error())
					continue
				}
				commit := domain.ActivityCommit{Activity: preview.Activity, Effects: preview.Effects, Resulting: preview.Resulting}
				imported.Activity = &commit
			} else {
				created, valueErr := domain.NewAccountValue(account, amount, effectiveAt, now)
				if valueErr != nil {
					appendCSVError(plan, rowNumber, "current_value", valueText, domain.ErrCSVRowInvalid, valueErr.Error())
					continue
				}
				imported.Value = &created
			}
		}
		if origin != nil {
			observation := domain.AccountStateObservation{
				ID: domain.NewAccountStateObservationID(), AccountID: account.ID, EffectiveAt: account.CreatedAt,
				ArchivedAt: account.ArchivedAt, IncludeInNetWorth: account.IncludeInNetWorth, IncludeInPortfolio: account.IncludeInPortfolio,
				IncludeInLiquidAssets: account.IncludeInLiquidAssets, CreatedAt: account.CreatedAt, Ownership: ownership.Shares(),
			}
			imported.Observation = &observation
		}
		plan.Batch.Accounts = append(plan.Batch.Accounts, imported)
		plan.Stats.CreateAccounts++
	}
	for _, member := range createdMembers {
		plan.Batch.Members = append(plan.Batch.Members, member)
		plan.Stats.CreateMembers++
	}
}

func planHoldingsCSV(plan *CSVImportPlan, household domain.Household, origin *domain.HistoryOrigin, now time.Time, table csvcodec.Table, column map[string]int, options CSVParseOptions, accounts []domain.AccountRecord, instruments []domain.Instrument, existingHoldings map[string]struct{}) {
	accountByName := map[string][]domain.AccountRecord{}
	for _, record := range accounts {
		accountByName[record.Account.Name] = append(accountByName[record.Account.Name], record)
	}
	for _, imported := range plan.Batch.Accounts {
		accountByName[imported.Account.Name] = append(accountByName[imported.Account.Name], domain.AccountRecord{Account: imported.Account})
	}
	instrumentByName := uniqueNameIndex(len(instruments), func(i int) string { return instruments[i].Name })
	createdInstruments := map[string]domain.Instrument{}
	seenFile := map[string]struct{}{}
	for i, row := range table.Rows {
		rowNumber := i + 2
		accountName := cell(row, column, "account_name")
		instrumentName := cell(row, column, "instrument_name")
		quantityText := normalizeNumber(cell(row, column, "quantity"), options)
		if accountName == "" || instrumentName == "" || quantityText == "" {
			appendCSVError(plan, rowNumber, "account_name", accountName, domain.ErrCSVRowInvalid, "account, instrument, and quantity are required")
			continue
		}
		candidates := accountByName[accountName]
		if len(candidates) == 0 {
			appendCSVError(plan, rowNumber, "account_name", accountName, domain.ErrCSVReferenceUnresolved, "account was not found")
			continue
		}
		if len(candidates) != 1 {
			appendCSVError(plan, rowNumber, "account_name", accountName, domain.ErrCSVReferenceUnresolved, "account name is ambiguous")
			continue
		}
		account := candidates[0].Account
		if account.TrackingMode != domain.TrackingHoldings {
			appendCSVError(plan, rowNumber, "account_name", accountName, domain.ErrCSVRowInvalid, "holdings require a holdings account")
			continue
		}
		quantity, quantityErr := domain.ParseQuantity(quantityText)
		if quantityErr != nil {
			appendCSVError(plan, rowNumber, "quantity", quantityText, domain.ErrCSVRowInvalid, "quantity is not valid")
			continue
		}
		dupKey := accountName + "\x00" + instrumentName
		if _, exists := seenFile[dupKey]; exists {
			appendCSVError(plan, rowNumber, "instrument_name", instrumentName, domain.ErrCSVDuplicate, "this holding already appears in the file")
			plan.Stats.Duplicates++
			continue
		}
		if _, exists := existingHoldings[dupKey]; exists {
			appendCSVError(plan, rowNumber, "instrument_name", instrumentName, domain.ErrCSVDuplicate, "this holding already exists")
			plan.Stats.Duplicates++
			continue
		}
		seenFile[dupKey] = struct{}{}
		var instrument domain.Instrument
		if ids, ok := instrumentByName[instrumentName]; ok {
			if len(ids) != 1 {
				appendCSVError(plan, rowNumber, "instrument_name", instrumentName, domain.ErrCSVReferenceUnresolved, "instrument name is ambiguous")
				continue
			}
			instrument = instruments[ids[0]]
			plan.Stats.References++
		} else if created, ok := createdInstruments[instrumentName]; ok {
			instrument = created
			plan.Stats.References++
		} else {
			created, ok := resolveNewInstrument(plan, household, origin, now, options, instruments, instrumentByName, createdInstruments, rowNumber, instrumentName, cell(row, column, "instrument_type"), cell(row, column, "quote_currency"), cell(row, column, "symbol"), cell(row, column, "market_code"), cell(row, column, "country_code"), cell(row, column, "isin"))
			if !ok {
				continue
			}
			instrument = created
		}
		var note *string
		if noteText := cell(row, column, "note"); noteText != "" {
			note = &noteText
		}
		unitPriceText := normalizeNumber(cell(row, column, "unit_price"), options)
		quoteDate := cell(row, column, "quote_date")
		if (unitPriceText == "") != (quoteDate == "") {
			appendCSVError(plan, rowNumber, "unit_price", unitPriceText, domain.ErrCSVRowInvalid, "unit price and quote date must both be provided or both be empty")
			continue
		}
		var quote *domain.InstrumentQuote
		if unitPriceText != "" {
			price, priceErr := domain.ParseUnitPrice(unitPriceText)
			if priceErr != nil {
				appendCSVError(plan, rowNumber, "unit_price", unitPriceText, domain.ErrCSVRowInvalid, "unit price is not valid")
				continue
			}
			quotedAt, dateErr := observationTime(quoteDate, options.DateFormat, origin, now)
			if dateErr != nil {
				appendCSVError(plan, rowNumber, "quote_date", quoteDate, domain.ErrCSVRowInvalid, dateErr.Error())
				continue
			}
			createdQuote, quoteErr := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, Currency: instrument.QuoteCurrency, SourceKind: domain.QuoteSourceManual, QuotedAt: quotedAt}, now)
			if quoteErr != nil {
				appendCSVError(plan, rowNumber, "unit_price", unitPriceText, domain.ErrCSVRowInvalid, quoteErr.Error())
				continue
			}
			quote = &createdQuote
		}
		holding, holdingErr := domain.NewHoldingForAccount(account, instrument, quantity, note, 0, now)
		if holdingErr != nil {
			appendCSVError(plan, rowNumber, "instrument_name", instrumentName, domain.ErrCSVRowInvalid, holdingErr.Error())
			continue
		}
		plan.Batch.Holdings = append(plan.Batch.Holdings, holding)
		plan.Stats.CreateHoldings++
		if quote != nil {
			plan.Batch.Quotes = append(plan.Batch.Quotes, *quote)
		} else {
			plan.Warnings = append(plan.Warnings, CSVWarning{Row: rowNumber, Code: "missing_quote", Message: "no quote was provided; valuation will be incomplete"})
		}
		if origin != nil {
			plan.Warnings = append(plan.Warnings, CSVWarning{Row: rowNumber, Code: "missing_cost", Message: "no cost evidence was imported; analytics stay incomplete"})
		}
	}
}

func parseOwnershipCSV(raw string, household domain.Household, now time.Time, options CSVParseOptions, members []domain.Member, memberByName map[string][]int, created map[string]domain.Member, plan *CSVImportPlan, rowNumber int) ([]domain.OwnershipShare, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		appendCSVError(plan, rowNumber, "ownership", raw, domain.ErrCSVRowInvalid, "ownership is required")
		return nil, &domain.Error{Code: domain.ErrCSVRowInvalid, Field: "ownership"}
	}
	parts := splitOwnershipParts(raw)
	shares := make([]domain.OwnershipShare, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		colon := ownershipColonIndex(part)
		if colon < 0 {
			appendCSVError(plan, rowNumber, "ownership", part, domain.ErrCSVRowInvalid, "ownership must use name:percent pairs")
			return nil, &domain.Error{Code: domain.ErrCSVRowInvalid, Field: "ownership"}
		}
		name := strings.TrimSpace(unescapeOwnershipName(part[:colon]))
		percent := strings.TrimSpace(part[colon+1:])
		bps, err := domain.PercentToBasisPoints(normalizeNumber(percent, options))
		if err != nil {
			appendCSVError(plan, rowNumber, "ownership", part, domain.ErrCSVRowInvalid, "ownership percentage is not valid")
			return nil, err
		}
		var memberID domain.MemberID
		if ids, exists := memberByName[name]; exists {
			if len(ids) != 1 {
				appendCSVError(plan, rowNumber, "ownership", name, domain.ErrCSVReferenceUnresolved, "member name is ambiguous")
				return nil, &domain.Error{Code: domain.ErrCSVReferenceUnresolved, Field: "ownership"}
			}
			memberID = members[ids[0]].ID
			plan.Stats.References++
		} else if existing, exists := created[name]; exists {
			memberID = existing.ID
		} else {
			id, ok := resolveNewMember(plan, household, now, options, members, memberByName, created, rowNumber, name)
			if !ok {
				return nil, &domain.Error{Code: domain.ErrCSVReferenceUnresolved, Field: "ownership"}
			}
			memberID = id
		}
		shares = append(shares, domain.OwnershipShare{MemberID: memberID, ShareBPS: bps})
	}
	if _, err := domain.ParseOwnership(shares); err != nil {
		appendCSVError(plan, rowNumber, "ownership", raw, domain.ErrCSVRowInvalid, "ownership must total 100%")
		return nil, err
	}
	return shares, nil
}

func observationTime(value, layout string, origin *domain.HistoryOrigin, now time.Time) (time.Time, error) {
	parsed, err := parseCSVDate(value, layout)
	if err != nil {
		return time.Time{}, err
	}
	location := time.UTC
	if origin != nil {
		loaded, locErr := time.LoadLocation(origin.Timezone)
		if locErr == nil {
			location = loaded
		}
	}
	observed := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, location)
	if observed.After(now) {
		return time.Time{}, &domain.Error{Code: domain.ErrCSVRowInvalid, Field: "value_date", Message: "observation date cannot be in the future"}
	}
	if origin != nil && observed.Before(origin.StartedAt) {
		return time.Time{}, &domain.Error{Code: domain.ErrCSVRowInvalid, Field: "value_date", Message: "observation date is earlier than history origin"}
	}
	return observed, nil
}

func parseCSVDate(value, layout string) (time.Time, error) {
	value = strings.TrimSpace(value)
	formats := []string{"2006-01-02"}
	switch layout {
	case CSVDateDayFirst:
		formats = []string{"02/01/2006", "2/1/2006", "2006-01-02"}
	case CSVDateMonthFirst:
		formats = []string{"01/02/2006", "1/2/2006", "2006-01-02"}
	}
	for _, format := range formats {
		if parsed, err := time.Parse(format, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, &domain.Error{Code: domain.ErrCSVRowInvalid, Field: "value_date", Message: "date is not valid"}
}

func normalizeNumber(value string, options CSVParseOptions) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	grouping := options.GroupingSep
	if grouping == "space" {
		grouping = " "
	}
	if grouping != "" && grouping != "none" {
		value = strings.ReplaceAll(value, grouping, "")
	}
	if options.DecimalSep == "," {
		if strings.Contains(value, ".") && strings.Contains(value, ",") {
			return value
		}
		value = strings.ReplaceAll(value, ",", ".")
	}
	return value
}

func validateCSVParseOptions(options CSVParseOptions) error {
	decimal := options.DecimalSep
	if decimal == "" {
		decimal = "."
	}
	grouping := options.GroupingSep
	if grouping == "" {
		grouping = "none"
	}
	if decimal != "." && decimal != "," {
		return &domain.Error{Code: domain.ErrCSVInvalidFormat, Field: "number_format", Message: "decimal separator must be a dot or comma"}
	}
	if grouping != "none" && grouping != "," && grouping != "." && grouping != "space" {
		return &domain.Error{Code: domain.ErrCSVInvalidFormat, Field: "number_format", Message: "grouping separator is not supported"}
	}
	if grouping != "none" && grouping != "space" && decimal == grouping {
		return &domain.Error{Code: domain.ErrCSVInvalidFormat, Field: "number_format", Message: "decimal and grouping separators must be different"}
	}
	return nil
}

func parseOptionalBool(value string) (bool, bool, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false, false, nil
	}
	switch value {
	case "true", "1", "yes", "y":
		return true, true, nil
	case "false", "0", "no", "n":
		return false, true, nil
	default:
		return false, false, &domain.Error{Code: domain.ErrCSVRowInvalid, Message: "boolean is not valid"}
	}
}

func uniqueNameIndex(count int, nameAt func(int) string) map[string][]int {
	index := map[string][]int{}
	for i := 0; i < count; i++ {
		name := nameAt(i)
		index[name] = append(index[name], i)
	}
	return index
}

func accountConflictKey(name, currency, institution string) string {
	return name + "\x00" + currency + "\x00" + institution
}

func appendCSVError(plan *CSVImportPlan, row int, field, source string, code domain.ErrorCode, message string) {
	plan.Errors = append(plan.Errors, CSVRowError{Row: row, Field: field, Source: source, Code: string(code), Message: message, Suggestion: csvRepairSuggestion(code, field)})
}

func csvRepairSuggestion(code domain.ErrorCode, field string) string {
	switch code {
	case domain.ErrCSVReferenceUnresolved:
		return "Choose an existing mapping, create a new record, or keep this import blocked."
	case domain.ErrCSVMappingRequired, domain.ErrCSVMappingConflict:
		return "Update the field mapping and preview again."
	case domain.ErrCSVDuplicate:
		return "Remove the duplicate or resolve the existing record before previewing again."
	default:
		if field != "" {
			return "Correct this source value and preview again."
		}
		return "Correct this source row and preview again."
	}
}

func unresolvedChoice(options CSVParseOptions, kind, name string) CSVUnresolvedAction {
	for _, item := range options.Unresolved {
		if item.Kind == kind && item.Name == name {
			return item
		}
	}
	return CSVUnresolvedAction{Kind: kind, Name: name, Action: CSVUnresolvedBlock}
}

func noteUnresolved(plan *CSVImportPlan, row int, kind, field, name string) {
	seen := false
	for _, item := range plan.Unresolved {
		if item.Kind == kind && item.Name == name {
			seen = true
			break
		}
	}
	if !seen {
		plan.Unresolved = append(plan.Unresolved, CSVUnresolvedName{Kind: kind, Name: name, Field: field, Row: row})
	}
	appendCSVError(plan, row, field, name, domain.ErrCSVReferenceUnresolved, "this name is not in the current household; choose map, create, or keep blocked")
}

func resolveNewInstitution(plan *CSVImportPlan, household domain.Household, now time.Time, options CSVParseOptions, institutions []domain.Institution, byName map[string][]int, created map[string]domain.Institution, row int, name, typeText string) (*domain.InstitutionID, bool) {
	choice := unresolvedChoice(options, "institution", name)
	switch strings.ToLower(choice.Action) {
	case CSVUnresolvedMap:
		target := strings.TrimSpace(choice.MapTo)
		ids, ok := byName[target]
		if !ok || len(ids) != 1 {
			appendCSVError(plan, row, "institution_name", name, domain.ErrCSVReferenceUnresolved, "mapped institution was not found")
			return nil, false
		}
		id := institutions[ids[0]].ID
		plan.Stats.References++
		return &id, true
	case CSVUnresolvedCreate:
		institutionType, instTypeErr := domain.ParseInstitutionType(typeText)
		if instTypeErr != nil {
			appendCSVError(plan, row, "institution_type", typeText, domain.ErrCSVRowInvalid, "institution type is required when creating an institution")
			return nil, false
		}
		made, createErr := domain.NewInstitution(household.ID, name, institutionType, now)
		if createErr != nil {
			appendCSVError(plan, row, "institution_name", name, domain.ErrCSVRowInvalid, "institution could not be created")
			return nil, false
		}
		created[name] = made
		plan.Batch.Institutions = append(plan.Batch.Institutions, made)
		plan.Stats.CreateInstitutions++
		return &made.ID, true
	default:
		noteUnresolved(plan, row, "institution", "institution_name", name)
		return nil, false
	}
}

func resolveNewGroup(plan *CSVImportPlan, household domain.Household, now time.Time, options CSVParseOptions, groups []domain.Group, byName map[string][]int, created map[string]domain.Group, row int, name string) (*domain.GroupID, bool) {
	choice := unresolvedChoice(options, "group", name)
	switch strings.ToLower(choice.Action) {
	case CSVUnresolvedMap:
		target := strings.TrimSpace(choice.MapTo)
		ids, ok := byName[target]
		if !ok || len(ids) != 1 {
			appendCSVError(plan, row, "group_name", name, domain.ErrCSVReferenceUnresolved, "mapped group was not found")
			return nil, false
		}
		id := groups[ids[0]].ID
		plan.Stats.References++
		return &id, true
	case CSVUnresolvedCreate:
		made, createErr := domain.NewGroup(household.ID, name, now)
		if createErr != nil {
			appendCSVError(plan, row, "group_name", name, domain.ErrCSVRowInvalid, "group could not be created")
			return nil, false
		}
		created[name] = made
		plan.Batch.Groups = append(plan.Batch.Groups, made)
		plan.Stats.CreateGroups++
		return &made.ID, true
	default:
		noteUnresolved(plan, row, "group", "group_name", name)
		return nil, false
	}
}

func resolveNewInstrument(plan *CSVImportPlan, household domain.Household, origin *domain.HistoryOrigin, now time.Time, options CSVParseOptions, instruments []domain.Instrument, byName map[string][]int, created map[string]domain.Instrument, row int, name, typeText, currencyText, symbol, market, country, isin string) (domain.Instrument, bool) {
	choice := unresolvedChoice(options, "instrument", name)
	switch strings.ToLower(choice.Action) {
	case CSVUnresolvedMap:
		target := strings.TrimSpace(choice.MapTo)
		ids, ok := byName[target]
		if !ok || len(ids) != 1 {
			appendCSVError(plan, row, "instrument_name", name, domain.ErrCSVReferenceUnresolved, "mapped instrument was not found")
			return domain.Instrument{}, false
		}
		plan.Stats.References++
		return instruments[ids[0]], true
	case CSVUnresolvedCreate:
		instrumentType, typeErr := domain.ParseInstrumentType(typeText)
		currency, currencyErr := domain.ParseSupportedCurrency(strings.ToUpper(currencyText))
		if typeErr != nil || currencyErr != nil {
			appendCSVError(plan, row, "instrument_type", typeText, domain.ErrCSVRowInvalid, "instrument type and quote currency are required when creating an instrument")
			return domain.Instrument{}, false
		}
		made, createErr := domain.NewInstrument(domain.InstrumentInput{
			HouseholdID: household.ID, Name: name, Type: instrumentType, QuoteCurrency: currency,
			Symbol: optString(symbol), MarketCode: optString(market), CountryCode: optString(country), ISIN: optString(isin),
			QuoteSource: domain.QuoteSourceManual,
		}, now)
		if createErr != nil {
			appendCSVError(plan, row, "instrument_name", name, domain.ErrCSVRowInvalid, createErr.Error())
			return domain.Instrument{}, false
		}
		imported := domain.CSVImportedInstrument{Instrument: made}
		if origin != nil {
			observation := domain.InstrumentPreferenceObservation{ID: domain.NewInstrumentPreferenceObservationID(), InstrumentID: made.ID, SourceKind: made.QuoteSource, EffectiveAt: made.CreatedAt, CreatedAt: made.CreatedAt}
			imported.Observation = &observation
		}
		plan.Batch.Instruments = append(plan.Batch.Instruments, imported)
		plan.Stats.CreateInstruments++
		created[name] = made
		return made, true
	default:
		noteUnresolved(plan, row, "instrument", "instrument_name", name)
		return domain.Instrument{}, false
	}
}

func resolveNewMember(plan *CSVImportPlan, household domain.Household, now time.Time, options CSVParseOptions, members []domain.Member, byName map[string][]int, created map[string]domain.Member, row int, name string) (domain.MemberID, bool) {
	choice := unresolvedChoice(options, "member", name)
	switch strings.ToLower(choice.Action) {
	case CSVUnresolvedMap:
		target := strings.TrimSpace(choice.MapTo)
		ids, ok := byName[target]
		if !ok || len(ids) != 1 {
			appendCSVError(plan, row, "ownership", name, domain.ErrCSVReferenceUnresolved, "mapped member was not found")
			return "", false
		}
		plan.Stats.References++
		return members[ids[0]].ID, true
	case CSVUnresolvedCreate:
		made, createErr := domain.NewMember(household.ID, name, now)
		if createErr != nil {
			appendCSVError(plan, row, "ownership", name, domain.ErrCSVRowInvalid, "member could not be created")
			return "", false
		}
		created[name] = made
		return made.ID, true
	default:
		noteUnresolved(plan, row, "member", "ownership", name)
		return "", false
	}
}

func optString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}
