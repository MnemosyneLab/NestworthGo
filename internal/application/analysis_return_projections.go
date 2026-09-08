package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReturnCalendarResult, ReturnDayResult, and ReturnTrendResult are application
// read models. They intentionally contain domain money and decimal values; the
// Wails layer owns serialization, just as it does for the Phase 2a models.
type ReturnComponentAmount struct {
	Component domain.ReturnComponent
	Amount    *domain.SignedMoney
}

type ReturnIssue struct {
	Date          domain.LocalDate
	Status        domain.Completeness
	MissingReason string
}

type ReturnDayResult struct {
	AnalysisAvailability
	Date                   domain.LocalDate
	BeginningInvestedValue *domain.SignedMoney
	EndingInvestedValue    *domain.SignedMoney
	ReturnAmount           *domain.SignedMoney
	ReturnRate             *decimal.Decimal
	Composition            []ReturnComponentAmount
	Contributors           []ReturnContributor
	Coverage               domain.RateCoverage
	Issues                 []ReturnIssue
}

type ReturnCalendarSummary struct {
	BeginningInvestedValue *domain.SignedMoney
	EndingInvestedValue    *domain.SignedMoney
	ReturnAmount           *domain.SignedMoney
	ReturnRate             *decimal.Decimal
	Coverage               domain.RateCoverage
}

type ReturnCalendarResult struct {
	AnalysisAvailability
	Summary         ReturnCalendarSummary
	Days            []ReturnDayResult
	TopContributors []ReturnContributor
	Issues          []ReturnIssue
}

type ReturnContributor struct {
	Key      string
	Label    string
	Amount   *domain.SignedMoney
	Rate     *decimal.Decimal
	Coverage domain.RateCoverage
}

// ReturnSource is the component-level breakdown shown by Return Trend. It is
// deliberately separate from ReturnContributor: the calendar's contributors
// answer "which holding/account?", while trend sources answer "which return
// component?".
type ReturnSource struct {
	Key    string
	Label  string
	Amount *domain.SignedMoney
	Share  *decimal.Decimal
}

type ReturnTrendDisplay string

const (
	ReturnTrendCumulativeAmount   ReturnTrendDisplay = "cumulative_amount"
	ReturnTrendLinkedRate         ReturnTrendDisplay = "linked_rate"
	ReturnTrendPeriodReturnAmount ReturnTrendDisplay = "period_return_amount"
)

type ReturnTrendPoint struct {
	Date   domain.LocalDate
	Amount *domain.SignedMoney
	// Rate is the daily Modified Dietz rate. Value is intentionally empty for
	// linked_rate; the period-linked rate lives on ReturnTrendResult.Rate.
	Rate     *decimal.Decimal
	Value    *domain.SignedMoney
	Coverage domain.RateCoverage
	AnalysisAvailability
}

type ReturnTrendResult struct {
	AnalysisAvailability
	Display ReturnTrendDisplay
	Points  []ReturnTrendPoint
	// Sources is the return-component breakdown, not the calendar's holding
	// contributor list. It is folded from ReturnComponents in the same result.
	Sources  []ReturnSource
	Amount   *domain.SignedMoney
	Rate     *decimal.Decimal
	Coverage domain.RateCoverage
}

type ContributionReturnType string

const (
	ContributionTotalReturn      ContributionReturnType = "total_return"
	ContributionRealized         ContributionReturnType = "realized"
	ContributionUnrealized       ContributionReturnType = "unrealized"
	ContributionDividendInterest ContributionReturnType = "dividend_interest"
)

type ContributionGroupBy string

const (
	ContributionGroupInstrument ContributionGroupBy = "instrument"
	ContributionGroupAccount    ContributionGroupBy = "account"
	ContributionGroupCurrency   ContributionGroupBy = "currency"
	ContributionGroupAssetClass ContributionGroupBy = "asset_class"
)

type ContributionSort string

const (
	ContributionSortAmountDesc ContributionSort = "amount_desc"
	ContributionSortAmountAsc  ContributionSort = "amount_asc"
	ContributionSortRateDesc   ContributionSort = "rate_desc"
	ContributionSortRateAsc    ContributionSort = "rate_asc"
	ContributionSortNameAsc    ContributionSort = "name_asc"
)

type ContributionItemResult struct {
	Key         string
	Label       string
	Amount      *domain.SignedMoney
	Rate        *decimal.Decimal
	Coverage    domain.RateCoverage
	Components  []ContributionComponent
	ByAccount   []ContributionComponent
	HistoryHint ContributionHistoryHint
	AnalysisAvailability
}

type ContributionComponent struct {
	Key          string
	AccountID    string
	InstrumentID string
	Currency     string
	Amount       *domain.SignedMoney
}

type ContributionHistoryHint struct {
	Kinds        []string
	AccountID    string
	InstrumentID string
	From         domain.LocalDate
	To           domain.LocalDate
}

type ContributionRow struct {
	Key      string
	Label    string
	Amount   *domain.SignedMoney
	Rate     *decimal.Decimal
	Coverage domain.RateCoverage
	AnalysisAvailability
}

type ContributionResult struct {
	AnalysisAvailability
	ReturnType ContributionReturnType
	GroupBy    ContributionGroupBy
	Rows       []ContributionRow
	Coverage   domain.RateCoverage
}

func (s *Service) ReturnCalendar(ctx context.Context, query domain.AnalysisQuery, cursor, granularity string) (ReturnCalendarResult, error) {
	if value := strings.TrimSpace(granularity); value != "" && value != "day" {
		return ReturnCalendarResult{}, &domain.Error{Code: domain.ErrValidation, Field: "granularity", Message: "return calendar granularity is invalid"}
	}
	if _, err := returnCalendarMonth(cursor); err != nil {
		return ReturnCalendarResult{}, err
	}
	result, forced, err := s.analyzeReturnProjection(ctx, query)
	if err != nil {
		return ReturnCalendarResult{}, err
	}
	return projectReturnCalendar(result, forced, cursor)
}

func (s *Service) ReturnDay(ctx context.Context, query domain.AnalysisQuery, date domain.LocalDate) (ReturnDayResult, error) {
	result, forced, err := s.analyzeReturnProjection(ctx, query)
	if err != nil {
		return ReturnDayResult{}, err
	}
	for _, day := range projectReturnDays(result, forced) {
		if day.Date == date {
			return day, nil
		}
	}
	return ReturnDayResult{}, &domain.Error{Code: domain.ErrNotFound, Field: "date", Message: "return day was not found"}
}

func (s *Service) ReturnTrend(ctx context.Context, query domain.AnalysisQuery, display ReturnTrendDisplay) (ReturnTrendResult, error) {
	if display == "" {
		display = ReturnTrendPeriodReturnAmount
	}
	if display != ReturnTrendCumulativeAmount && display != ReturnTrendLinkedRate && display != ReturnTrendPeriodReturnAmount {
		return ReturnTrendResult{}, &domain.Error{Code: domain.ErrValidation, Field: "display", Message: "return trend display is invalid"}
	}
	result, forced, err := s.analyzeReturnProjection(ctx, query)
	if err != nil {
		return ReturnTrendResult{}, err
	}
	return projectReturnTrend(result, forced, display)
}

func (s *Service) Contribution(ctx context.Context, query domain.AnalysisQuery, returnType ContributionReturnType, groupBy ContributionGroupBy, ordering ContributionSort) (ContributionResult, error) {
	if err := validateContribution(returnType, groupBy, ordering); err != nil {
		return ContributionResult{}, err
	}
	if err := query.Validate(); err != nil {
		return ContributionResult{}, err
	}
	if returnType == ContributionUnrealized {
		return ContributionResult{AnalysisAvailability: unavailableReturn("unrealized range-end cost basis is not available from the analysis result"), ReturnType: returnType, GroupBy: groupBy}, nil
	}
	if returnType == ContributionRealized {
		return s.projectRealizedContribution(ctx, query, "", groupBy, ordering)
	}
	result, forced, err := s.analyzeReturnProjection(ctx, query)
	if err != nil {
		return ContributionResult{}, err
	}
	if returnType == ContributionDividendInterest {
		return projectDividendContribution(result, forced, groupBy, ordering)
	}
	if !hasNonZeroReturnAmount(result) {
		return ContributionResult{AnalysisAvailability: returnAvailability(result, forced), ReturnType: returnType, GroupBy: groupBy, Coverage: result.Coverage, Rows: []ContributionRow{}}, nil
	}
	groups, err := FoldReturnGroups(result, analysisGroupBy(groupBy))
	if err != nil {
		return ContributionResult{}, err
	}
	rows := make([]ContributionRow, 0, len(groups))
	currency := resultCurrency(result)
	for _, group := range groups {
		rows = append(rows, ContributionRow{Key: group.Key, Label: group.Key, Amount: signedPointer(group.ReturnAmount, currency), Rate: group.ReturnRate, Coverage: group.Coverage, AnalysisAvailability: groupAvailability(group.Status, group.Coverage, forced)})
	}
	sortContributionRows(rows, ordering, true)
	return ContributionResult{AnalysisAvailability: returnAvailability(result, forced), ReturnType: returnType, GroupBy: groupBy, Rows: rows, Coverage: result.Coverage}, nil
}

func (s *Service) ContributionItem(ctx context.Context, query domain.AnalysisQuery, returnType ContributionReturnType, groupBy ContributionGroupBy, groupKey string) (ContributionItemResult, error) {
	if err := validateContribution(returnType, groupBy, ContributionSortAmountDesc); err != nil {
		return ContributionItemResult{}, err
	}
	if err := query.Validate(); err != nil {
		return ContributionItemResult{}, err
	}
	if returnType == ContributionUnrealized {
		return ContributionItemResult{Key: groupKey, Label: groupKey, AnalysisAvailability: unavailableReturn("unrealized range-end cost basis is not available from the analysis result")}, nil
	}
	if returnType == ContributionRealized {
		contribution, err := s.projectRealizedContribution(ctx, query, "", groupBy, ContributionSortAmountDesc)
		if err != nil {
			return ContributionItemResult{}, err
		}
		item := ContributionItemResult{Key: groupKey, Label: groupKey, AnalysisAvailability: contribution.AnalysisAvailability, HistoryHint: ContributionHistoryHint{From: query.From, To: query.To}}
		var matched *ContributionRow
		for _, row := range contribution.Rows {
			if row.Key == groupKey {
				rowCopy := row
				matched = &rowCopy
				break
			}
		}
		if matched == nil {
			return ContributionItemResult{}, contributionGroupNotFound(groupKey)
		}
		item.Label, item.Amount, item.Rate, item.Coverage = matched.Label, matched.Amount, matched.Rate, matched.Coverage
		component := ContributionComponent{Key: groupKey, Amount: matched.Amount}
		if matched.Amount != nil {
			component.Currency = matched.Amount.Currency().String()
		}
		if groupBy == ContributionGroupInstrument {
			component.InstrumentID = groupKey
			item.Components = []ContributionComponent{component}
		} else if groupBy == ContributionGroupAccount {
			component.AccountID = groupKey
			item.ByAccount = []ContributionComponent{component}
		}
		return item, nil
	}
	result, forced, err := s.analyzeReturnProjection(ctx, query)
	if err != nil {
		return ContributionItemResult{}, err
	}
	return projectContributionItem(result, forced, query, returnType, groupBy, groupKey)
}

func projectContributionItem(result domain.PeriodAnalysisResult, forced string, query domain.AnalysisQuery, returnType ContributionReturnType, groupBy ContributionGroupBy, groupKey string) (ContributionItemResult, error) {
	item := ContributionItemResult{Key: groupKey, Label: groupKey, AnalysisAvailability: returnAvailability(result, forced), HistoryHint: ContributionHistoryHint{From: query.From, To: query.To}}
	componentAmounts := make(map[domain.ReturnComponent]decimal.Decimal)
	accountAmounts := make(map[string]decimal.Decimal)
	accountCurrency := make(map[string]string)
	itemAmount := decimal.Zero
	foundGroup := false
	for _, day := range result.Days {
		if !returnEligibleComponent(day.Component, query.IncludeCash) || returnGroupKey(day.Component, analysisGroupBy(groupBy)) != groupKey {
			continue
		}
		amount := decimal.Zero
		if returnType == ContributionTotalReturn {
			foundGroup = true
			for component, value := range day.ReturnComponents {
				if value.Currency() == "" {
					continue
				}
				amount = amount.Add(value.Amount())
				componentAmounts[component] = componentAmounts[component].Add(value.Amount())
			}
		} else {
			value, ok := day.ReturnComponents[domain.ReturnDividendInterest]
			if !ok || value.Currency() == "" {
				continue
			}
			foundGroup = true
			amount = value.Amount()
			componentAmounts[domain.ReturnDividendInterest] = componentAmounts[domain.ReturnDividendInterest].Add(amount)
		}
		itemAmount = itemAmount.Add(amount)
		if amount.IsZero() {
			continue
		}
		accountKey := day.Component.AccountID.String()
		accountAmounts[accountKey] = accountAmounts[accountKey].Add(amount)
		accountCurrency[accountKey] = day.Component.Currency.String()
		if item.HistoryHint.AccountID == "" {
			item.HistoryHint.AccountID = accountKey
		}
		if item.HistoryHint.InstrumentID == "" && day.Component.InstrumentID != nil {
			item.HistoryHint.InstrumentID = day.Component.InstrumentID.String()
		}
	}
	// ComponentDay intentionally retains attribution, not the parent
	// ActivityKind. Leave Kinds empty rather than passing return-component keys
	// (price_change, fx_impact, ...) to History as if they were activity kinds.
	// The date/account/instrument filters still take the user to the relevant
	// history slice without silently filtering every record out.
	item.Components = contributionComposition(componentAmounts, resultCurrency(result))
	item.ByAccount = contributionByAccount(accountAmounts, accountCurrency, resultCurrency(result))
	if returnType == ContributionTotalReturn {
		groups, err := FoldReturnGroups(result, analysisGroupBy(groupBy))
		if err != nil {
			return ContributionItemResult{}, err
		}
		for _, group := range groups {
			if group.Key == groupKey {
				foundGroup = true
				item.Amount = signedPointer(group.ReturnAmount, resultCurrency(result))
				item.Rate = group.ReturnRate
				item.Coverage = group.Coverage
				break
			}
		}
	} else {
		item.Amount = signedPointer(itemAmount, resultCurrency(result))
	}
	if !foundGroup {
		return ContributionItemResult{}, contributionGroupNotFound(groupKey)
	}
	return item, nil
}

func projectReturnCalendar(result domain.PeriodAnalysisResult, forced, cursor string) (ReturnCalendarResult, error) {
	month, err := returnCalendarMonth(cursor)
	if err != nil {
		return ReturnCalendarResult{}, err
	}
	allDays := projectReturnDays(result, forced)
	days := allDays
	if month != "" {
		days = make([]ReturnDayResult, 0, len(allDays))
		for _, day := range allDays {
			if strings.HasPrefix(string(day.Date), month) {
				days = append(days, day)
			}
		}
	}
	topContributors, err := topReturnContributors(result, forced)
	if err != nil {
		return ReturnCalendarResult{}, err
	}
	calendar := ReturnCalendarResult{AnalysisAvailability: returnAvailability(result, forced), Days: days, Issues: make([]ReturnIssue, 0), TopContributors: topContributors}
	for _, day := range allDays {
		calendar.Issues = append(calendar.Issues, day.Issues...)
	}
	calendar.Summary = ReturnCalendarSummary{BeginningInvestedValue: result.InvestedCapital, ReturnAmount: result.ReturnAmount, ReturnRate: result.ReturnRate, Coverage: result.Coverage}
	calendar.Summary.EndingInvestedValue = periodEndingValue(result)
	return calendar, nil
}

// ReturnCalendar's 2b cursor is a visible-month selector. It filters cells
// while keeping summary, period issues, and top contributors for the full
// query window. Phase 3 can therefore request a visible month without
// changing the period summary contract.
func returnCalendarMonth(cursor string) (string, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return "", nil
	}
	if len(cursor) != len("2006-01") {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "return calendar cursor must use YYYY-MM"}
	}
	parsed, err := time.Parse("2006-01", cursor)
	if err != nil || parsed.Format("2006-01") != cursor {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "cursor", Message: "return calendar cursor must use YYYY-MM"}
	}
	return cursor, nil
}

func projectReturnDays(result domain.PeriodAnalysisResult, forced string) []ReturnDayResult {
	byDate := make(map[domain.LocalDate][]domain.ComponentDay)
	contributorsByDate := make(map[domain.LocalDate]map[string]decimal.Decimal)
	for _, day := range result.Days {
		if returnEligibleComponent(day.Component, result.Query.IncludeCash) {
			byDate[day.Date] = append(byDate[day.Date], day)
			if len(day.ReturnComponents) > 0 {
				byInstrument := contributorsByDate[day.Date]
				if byInstrument == nil {
					byInstrument = make(map[string]decimal.Decimal)
					contributorsByDate[day.Date] = byInstrument
				}
				key := returnGroupKey(day.Component, GroupByInstrument)
				for _, value := range day.ReturnComponents {
					byInstrument[key] = byInstrument[key].Add(value.Amount())
				}
			}
		}
	}
	days := make([]ReturnDayResult, 0, len(result.DailyReturns))
	for _, daily := range result.DailyReturns {
		var beginning, ending decimal.Decimal
		components := map[domain.ReturnComponent]decimal.Decimal{}
		for _, day := range byDate[daily.Date] {
			beginning = beginning.Add(day.BeginningValue.Amount())
			ending = ending.Add(day.EndingValue.Amount())
			for component, value := range day.ReturnComponents {
				components[component] = components[component].Add(value.Amount())
			}
		}
		currency := resultCurrency(result)
		coverage := dailyCoverage(daily)
		cell := ReturnDayResult{AnalysisAvailability: dailyAvailability(daily, forced), Date: daily.Date, ReturnAmount: daily.Amount, ReturnRate: daily.Rate, Coverage: coverage, BeginningInvestedValue: signedPointer(beginning, currency), EndingInvestedValue: signedPointer(ending, currency), Contributors: returnContributorsFromAmounts(contributorsByDate[daily.Date], currency, coverage)}
		keys := make([]string, 0, len(components))
		for component := range components {
			keys = append(keys, string(component))
		}
		sort.Strings(keys)
		for _, key := range keys {
			cell.Composition = append(cell.Composition, ReturnComponentAmount{Component: domain.ReturnComponent(key), Amount: signedPointer(components[domain.ReturnComponent(key)], currency)})
		}
		if daily.Status != domain.CompletenessOK {
			cell.Issues = []ReturnIssue{{Date: daily.Date, Status: daily.Status, MissingReason: "daily return is incomplete"}}
		}
		days = append(days, cell)
	}
	return days
}

func projectReturnTrend(result domain.PeriodAnalysisResult, forced string, display ReturnTrendDisplay) (ReturnTrendResult, error) {
	points := make([]ReturnTrendPoint, 0, len(result.DailyReturns))
	cumulative := decimal.Zero
	currency := resultCurrency(result)
	for _, daily := range result.DailyReturns {
		if daily.Amount != nil {
			cumulative = cumulative.Add(daily.Amount.Amount())
		}
		var value *domain.SignedMoney
		switch display {
		case ReturnTrendCumulativeAmount:
			value = signedPointer(cumulative, currency)
		case ReturnTrendPeriodReturnAmount:
			value = daily.Amount
		}
		points = append(points, ReturnTrendPoint{Date: daily.Date, Amount: daily.Amount, Rate: daily.Rate, Value: value, Coverage: dailyCoverage(daily), AnalysisAvailability: dailyAvailability(daily, forced)})
	}
	return ReturnTrendResult{AnalysisAvailability: returnAvailability(result, forced), Display: display, Points: points, Sources: returnSources(result), Amount: result.ReturnAmount, Rate: result.ReturnRate, Coverage: result.Coverage}, nil
}

func topReturnContributors(result domain.PeriodAnalysisResult, forced string) ([]ReturnContributor, error) {
	if !hasNonZeroReturnAmount(result) {
		return []ReturnContributor{}, nil
	}
	groups, err := FoldReturnGroups(result, GroupByInstrument)
	if err != nil {
		return nil, err
	}
	currency := resultCurrency(result)
	contributors := make([]ReturnContributor, 0, len(groups))
	for _, group := range groups {
		contributors = append(contributors, ReturnContributor{Key: group.Key, Label: group.Key, Amount: signedPointer(group.ReturnAmount, currency), Rate: group.ReturnRate, Coverage: group.Coverage})
	}
	sort.SliceStable(contributors, func(i, j int) bool {
		return signedAmountGreater(contributors[i].Amount, contributors[j].Amount)
	})
	if len(contributors) > 5 {
		contributors = contributors[:5]
	}
	return contributors, nil
}

func contributionComposition(amounts map[domain.ReturnComponent]decimal.Decimal, currency domain.CurrencyCode) []ContributionComponent {
	keys := make([]string, 0, len(amounts))
	for component, amount := range amounts {
		if amount.IsZero() {
			continue
		}
		keys = append(keys, string(component))
	}
	sort.Strings(keys)
	components := make([]ContributionComponent, 0, len(keys))
	for _, key := range keys {
		components = append(components, ContributionComponent{Key: key, Currency: currency.String(), Amount: signedPointer(amounts[domain.ReturnComponent(key)], currency)})
	}
	return components
}

func contributionByAccount(amounts map[string]decimal.Decimal, currencies map[string]string, currency domain.CurrencyCode) []ContributionComponent {
	accounts := make([]ContributionComponent, 0, len(amounts))
	for key, amount := range amounts {
		if amount.IsZero() {
			continue
		}
		accountCurrency := currencies[key]
		if accountCurrency == "" {
			accountCurrency = currency.String()
		}
		accounts = append(accounts, ContributionComponent{Key: key, AccountID: key, Currency: accountCurrency, Amount: signedPointer(amount, currency)})
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		return signedAmountGreater(accounts[i].Amount, accounts[j].Amount)
	})
	return accounts
}

func returnSources(result domain.PeriodAnalysisResult) []ReturnSource {
	amounts := make(map[domain.ReturnComponent]decimal.Decimal)
	for _, day := range result.Days {
		if !returnEligibleComponent(day.Component, result.Query.IncludeCash) {
			continue
		}
		for component, amount := range day.ReturnComponents {
			amounts[component] = amounts[component].Add(amount.Amount())
		}
	}
	keys := make([]string, 0, len(amounts))
	for component, amount := range amounts {
		if amount.IsZero() {
			continue
		}
		keys = append(keys, string(component))
	}
	sort.Strings(keys)
	currency := resultCurrency(result)
	var total decimal.Decimal
	if result.ReturnAmount != nil {
		total = result.ReturnAmount.Amount()
	}
	sources := make([]ReturnSource, 0, len(keys))
	for _, key := range keys {
		amount := amounts[domain.ReturnComponent(key)]
		var share *decimal.Decimal
		if !total.IsZero() {
			value := amount.Div(total)
			share = &value
		}
		sources = append(sources, ReturnSource{Key: key, Label: key, Amount: signedPointer(amount, currency), Share: share})
	}
	return sources
}

func hasNonZeroReturnAmount(result domain.PeriodAnalysisResult) bool {
	for _, day := range result.Days {
		if day.ReturnAmount != nil && !day.ReturnAmount.Amount().IsZero() {
			return true
		}
	}
	return false
}

func returnContributorsFromAmounts(amounts map[string]decimal.Decimal, currency domain.CurrencyCode, coverage domain.RateCoverage) []ReturnContributor {
	contributors := make([]ReturnContributor, 0, len(amounts))
	for key, amount := range amounts {
		contributors = append(contributors, ReturnContributor{Key: key, Label: key, Amount: signedPointer(amount, currency), Coverage: coverage})
	}
	sort.SliceStable(contributors, func(i, j int) bool {
		return signedAmountGreater(contributors[i].Amount, contributors[j].Amount)
	})
	return contributors
}

func projectDividendContribution(result domain.PeriodAnalysisResult, forced string, groupBy ContributionGroupBy, ordering ContributionSort) (ContributionResult, error) {
	amounts := map[string]decimal.Decimal{}
	for _, day := range result.Days {
		if !returnEligibleComponent(day.Component, result.Query.IncludeCash) {
			continue
		}
		key := returnGroupKey(day.Component, analysisGroupBy(groupBy))
		value := day.ReturnComponents[domain.ReturnDividendInterest]
		if value.Currency() != "" {
			amounts[key] = amounts[key].Add(value.Amount())
		}
	}
	rows := make([]ContributionRow, 0, len(amounts))
	currency := resultCurrency(result)
	for key, amount := range amounts {
		rows = append(rows, ContributionRow{Key: key, Label: key, Amount: signedPointer(amount, currency), AnalysisAvailability: returnAvailability(result, forced)})
	}
	sortContributionRows(rows, ordering, false)
	return ContributionResult{AnalysisAvailability: returnAvailability(result, forced), ReturnType: ContributionDividendInterest, GroupBy: groupBy, Rows: rows, Coverage: result.Coverage}, nil
}

func (s *Service) projectRealizedContribution(ctx context.Context, query domain.AnalysisQuery, forced string, groupBy ContributionGroupBy, ordering ContributionSort) (ContributionResult, error) {
	if s.gain == nil {
		return ContributionResult{AnalysisAvailability: unavailableReturn("realized gain service is unavailable"), ReturnType: ContributionRealized, GroupBy: groupBy}, nil
	}
	scope, ok := realizedScope(query)
	if !ok {
		return ContributionResult{AnalysisAvailability: unavailableReturn("realized gain grouping does not support the requested currency or asset-class filter"), ReturnType: ContributionRealized, GroupBy: groupBy}, nil
	}
	view, err := s.gain.RealizedGainInRange(ctx, scope, query.From, query.To)
	if err != nil {
		return ContributionResult{}, err
	}
	groups := view.ByInstrument
	if groupBy == ContributionGroupAccount {
		groups = view.ByAccount
	}
	if groupBy != ContributionGroupInstrument && groupBy != ContributionGroupAccount {
		return ContributionResult{AnalysisAvailability: unavailableReturn("realized gain grouping is available by account or instrument only"), ReturnType: ContributionRealized, GroupBy: groupBy}, nil
	}
	rows := make([]ContributionRow, 0, len(groups))
	currency := view.Currency
	for _, group := range groups {
		amount, parseErr := decimal.NewFromString(group.Gain.Amount)
		if parseErr != nil {
			return ContributionResult{}, parseErr
		}
		rows = append(rows, ContributionRow{Key: group.Key, Label: group.Label, Amount: signedPointer(amount, currency), AnalysisAvailability: gainAvailability(view.Available && group.Available, view.MissingReason+group.MissingReason, forced)})
	}
	sortContributionRows(rows, ordering, false)
	return ContributionResult{AnalysisAvailability: gainAvailability(view.Available, view.MissingReason, forced), ReturnType: ContributionRealized, GroupBy: groupBy, Rows: rows}, nil
}

func validateContribution(returnType ContributionReturnType, groupBy ContributionGroupBy, ordering ContributionSort) error {
	if returnType != ContributionTotalReturn && returnType != ContributionRealized && returnType != ContributionUnrealized && returnType != ContributionDividendInterest {
		return &domain.Error{Code: domain.ErrValidation, Field: "returnType", Message: "contribution return type is invalid"}
	}
	if groupBy != ContributionGroupInstrument && groupBy != ContributionGroupAccount && groupBy != ContributionGroupCurrency && groupBy != ContributionGroupAssetClass {
		return &domain.Error{Code: domain.ErrValidation, Field: "groupBy", Message: "contribution group is invalid"}
	}
	if ordering != ContributionSortAmountDesc && ordering != ContributionSortAmountAsc && ordering != ContributionSortRateDesc && ordering != ContributionSortRateAsc && ordering != ContributionSortNameAsc {
		return &domain.Error{Code: domain.ErrValidation, Field: "sort", Message: "contribution sort is invalid"}
	}
	return nil
}

func contributionGroupNotFound(groupKey string) error {
	return &domain.Error{Code: domain.ErrNotFound, Field: "groupKey", Message: "contribution group was not found"}
}

func analysisGroupBy(groupBy ContributionGroupBy) AnalysisGroupBy { return AnalysisGroupBy(groupBy) }

func sortContributionRows(rows []ContributionRow, ordering ContributionSort, hasRate bool) {
	if !hasRate && (ordering == ContributionSortRateAsc || ordering == ContributionSortRateDesc) {
		ordering = ContributionSortAmountDesc
	}
	sort.SliceStable(rows, func(i, j int) bool {
		switch ordering {
		case ContributionSortNameAsc:
			return strings.ToLower(rows[i].Label) < strings.ToLower(rows[j].Label)
		case ContributionSortAmountAsc:
			return signedAmountLess(rows[i].Amount, rows[j].Amount)
		case ContributionSortRateAsc:
			return rateValue(rows[i].Rate).LessThan(rateValue(rows[j].Rate))
		case ContributionSortRateDesc:
			return rateValue(rows[i].Rate).GreaterThan(rateValue(rows[j].Rate))
		case ContributionSortAmountDesc:
			return signedAmountGreater(rows[i].Amount, rows[j].Amount)
		default:
			return signedAmountGreater(rows[i].Amount, rows[j].Amount)
		}
	})
}

func signedAmountGreater(left, right *domain.SignedMoney) bool {
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	return left.Amount().GreaterThan(right.Amount())
}

func signedAmountLess(left, right *domain.SignedMoney) bool {
	if left == nil {
		return right != nil
	}
	if right == nil {
		return false
	}
	return left.Amount().LessThan(right.Amount())
}

func rateValue(rate *decimal.Decimal) decimal.Decimal {
	if rate == nil {
		return decimal.Zero
	}
	return *rate
}

func returnAvailability(result domain.PeriodAnalysisResult, forced string) AnalysisAvailability {
	if len(result.DailyReturns) == 0 {
		return unavailableReturn("return days are unavailable")
	}
	usable, partial := false, false
	for _, day := range result.DailyReturns {
		if day.Status == domain.CompletenessOK {
			usable = true
		}
		if day.Status == domain.CompletenessPartial {
			usable, partial = true, true
		}
	}
	status := domain.CompletenessUnavailable
	reason := "return days are unavailable"
	if usable {
		status, reason = domain.CompletenessOK, ""
		if partial || result.Coverage.RatedDays < result.Coverage.TotalDays {
			status, reason = domain.CompletenessPartial, "one or more return days are incomplete"
		}
	}
	return AnalysisAvailability{Available: usable, Status: status, MissingReason: reason, ValuationForced: forcedPointer(forced)}
}

func dailyAvailability(daily domain.DailyReturn, forced string) AnalysisAvailability {
	reason := ""
	if daily.Status != domain.CompletenessOK {
		reason = "daily return is incomplete"
	}
	return AnalysisAvailability{Available: daily.Amount != nil, Status: daily.Status, MissingReason: reason, ValuationForced: forcedPointer(forced)}
}
func unavailableReturn(reason string) AnalysisAvailability {
	return AnalysisAvailability{Available: false, Status: domain.CompletenessUnavailable, MissingReason: reason}
}
func groupAvailability(status domain.Completeness, coverage domain.RateCoverage, forced string) AnalysisAvailability {
	return AnalysisAvailability{Available: status != domain.CompletenessUnavailable, Status: status, MissingReason: func() string {
		if status == domain.CompletenessOK {
			return ""
		}
		return "group has no complete rated days"
	}(), ValuationForced: forcedPointer(forced)}
}
func gainAvailability(available bool, reason, forced string) AnalysisAvailability {
	if !available && reason == "" {
		reason = "gain is unavailable"
	}
	status := domain.CompletenessOK
	if !available {
		status = domain.CompletenessUnavailable
	}
	return AnalysisAvailability{Available: available, Status: status, MissingReason: reason, ValuationForced: forcedPointer(forced)}
}

func dailyCoverage(daily domain.DailyReturn) domain.RateCoverage {
	coverage := domain.RateCoverage{TotalDays: 1}
	if daily.Rate != nil && daily.Status == domain.CompletenessOK {
		coverage.RatedDays = 1
	}
	return coverage
}

func resultCurrency(result domain.PeriodAnalysisResult) domain.CurrencyCode {
	for _, daily := range result.DailyReturns {
		if daily.Amount != nil {
			return daily.Amount.Currency()
		}
	}
	if result.InvestedCapital != nil {
		return result.InvestedCapital.Currency()
	}
	for _, day := range result.Days {
		if !returnEligibleComponent(day.Component, result.Query.IncludeCash) {
			continue
		}
		if day.ReturnAmount != nil {
			return day.ReturnAmount.Currency()
		}
		if day.BeginningValue.Currency() != "" {
			return day.BeginningValue.Currency()
		}
		if day.EndingValue.Currency() != "" {
			return day.EndingValue.Currency()
		}
	}
	return ""
}
func periodEndingValue(result domain.PeriodAnalysisResult) *domain.SignedMoney {
	if len(result.DailyReturns) == 0 {
		return nil
	}
	date := result.DailyReturns[len(result.DailyReturns)-1].Date
	total := decimal.Zero
	for _, day := range result.Days {
		if day.Date == date && returnEligibleComponent(day.Component, result.Query.IncludeCash) {
			total = total.Add(day.EndingValue.Amount())
		}
	}
	if resultCurrency(result) == "" {
		return nil
	}
	return signedPointer(total, resultCurrency(result))
}
func realizedScope(query domain.AnalysisQuery) (domain.GainScope, bool) {
	scope := domain.GainScope{}
	if query.Filters.Currency != nil || query.Filters.AssetClass != "" || query.Filters.MemberID != nil {
		return scope, false
	}
	if query.Scope.Kind == domain.ScopeAccount {
		id, err := domain.ParseAccountID(query.Scope.ID)
		if err != nil {
			return scope, false
		}
		scope.AccountID = &id
	}
	if query.Scope.Kind == domain.ScopeInstrument {
		id, err := domain.ParseInstrumentID(query.Scope.ID)
		if err != nil {
			return scope, false
		}
		scope.InstrumentID = &id
	}
	if query.Filters.AccountID != nil {
		scope.AccountID = query.Filters.AccountID
	}
	if query.Filters.InstrumentID != nil {
		scope.InstrumentID = query.Filters.InstrumentID
	}
	return scope, true
}
