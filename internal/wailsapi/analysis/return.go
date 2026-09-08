package analysis

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type ReturnCalendarDTO = wire.ReturnCalendarDTO
type ReturnDayDTO = wire.ReturnDayDTO
type ReturnTrendDTO = wire.ReturnTrendDTO
type ContributionDTO = wire.ContributionDTO
type ContributionItemDTO = wire.ContributionItemDTO

func (s *Service) ReturnCalendar(ctx context.Context, request AnalysisQueryRequest, cursor, granularity string) (ReturnCalendarDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return ReturnCalendarDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.ReturnCalendar(ctx, query, strings.TrimSpace(cursor), strings.TrimSpace(granularity))
	if err != nil {
		return ReturnCalendarDTO{}, apierror.Wrap(err)
	}
	return fromReturnCalendar(result), nil
}

func (s *Service) ReturnDay(ctx context.Context, request AnalysisQueryRequest, date string) (ReturnDayDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return ReturnDayDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.ReturnDay(ctx, query, domain.LocalDate(strings.TrimSpace(date)))
	if err != nil {
		return ReturnDayDTO{}, apierror.Wrap(err)
	}
	return fromReturnDay(result), nil
}

func (s *Service) ReturnTrend(ctx context.Context, request AnalysisQueryRequest, display string) (ReturnTrendDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return ReturnTrendDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.ReturnTrend(ctx, query, application.ReturnTrendDisplay(strings.TrimSpace(display)))
	if err != nil {
		return ReturnTrendDTO{}, apierror.Wrap(err)
	}
	return fromReturnTrend(result), nil
}

func (s *Service) Contribution(ctx context.Context, request AnalysisQueryRequest, returnType, groupBy, ordering string) (ContributionDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return ContributionDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.Contribution(ctx, query, application.ContributionReturnType(strings.TrimSpace(returnType)), application.ContributionGroupBy(strings.TrimSpace(groupBy)), application.ContributionSort(strings.TrimSpace(ordering)))
	if err != nil {
		return ContributionDTO{}, apierror.Wrap(err)
	}
	return fromContribution(result), nil
}

func (s *Service) ContributionItem(ctx context.Context, request AnalysisQueryRequest, returnType, groupBy, groupKey string) (ContributionItemDTO, error) {
	query, err := request.ToDomain()
	if err != nil {
		return ContributionItemDTO{}, apierror.Wrap(err)
	}
	result, err := s.app.ContributionItem(ctx, query, application.ContributionReturnType(strings.TrimSpace(returnType)), application.ContributionGroupBy(strings.TrimSpace(groupBy)), strings.TrimSpace(groupKey))
	if err != nil {
		return ContributionItemDTO{}, apierror.Wrap(err)
	}
	return fromContributionItem(result), nil
}

func fromReturnCalendar(value application.ReturnCalendarResult) wire.ReturnCalendarDTO {
	result := wire.ReturnCalendarDTO{
		Summary:         fromReturnCalendarSummary(value.Summary),
		Cells:           make([]wire.ReturnDayDTO, 0, len(value.Days)),
		TopContributors: fromReturnContributors(value.TopContributors),
		Issues:          fromReturnIssues(value.Issues),
	}
	for _, day := range value.Days {
		result.Cells = append(result.Cells, fromReturnDay(day))
	}
	setAvailability(&result.Available, &result.Status, &result.MissingReason, &result.ValuationForced, value.AnalysisAvailability)
	return result
}

func fromReturnCalendarSummary(value application.ReturnCalendarSummary) wire.ReturnCalendarSummaryDTO {
	return wire.ReturnCalendarSummaryDTO{
		BeginningInvestedValue: wire.FromSignedMoneyPtr(value.BeginningInvestedValue),
		EndingInvestedValue:    wire.FromSignedMoneyPtr(value.EndingInvestedValue),
		ReturnAmount:           wire.FromSignedMoneyPtr(value.ReturnAmount),
		ReturnRate:             decimalStringPtr(value.ReturnRate),
		RatedDays:              value.Coverage.RatedDays,
		TotalDays:              value.Coverage.TotalDays,
	}
}

func fromReturnDay(value application.ReturnDayResult) wire.ReturnDayDTO {
	result := wire.ReturnDayDTO{
		Date:                   string(value.Date),
		BeginningInvestedValue: wire.FromSignedMoneyPtr(value.BeginningInvestedValue),
		EndingInvestedValue:    wire.FromSignedMoneyPtr(value.EndingInvestedValue),
		ReturnAmount:           wire.FromSignedMoneyPtr(value.ReturnAmount),
		ReturnRate:             decimalStringPtr(value.ReturnRate),
		Composition:            fromReturnComposition(value.Composition),
		Contributors:           fromReturnContributors(value.Contributors),
		RatedDays:              value.Coverage.RatedDays,
		TotalDays:              value.Coverage.TotalDays,
		Issues:                 fromReturnIssues(value.Issues),
	}
	setAvailability(&result.Available, &result.Status, &result.MissingReason, &result.ValuationForced, value.AnalysisAvailability)
	return result
}

func fromReturnTrend(value application.ReturnTrendResult) wire.ReturnTrendDTO {
	result := wire.ReturnTrendDTO{
		Display:   string(value.Display),
		Points:    make([]wire.ReturnTrendPointDTO, 0, len(value.Points)),
		Sources:   fromReturnSources(value.Sources),
		Amount:    wire.FromSignedMoneyPtr(value.Amount),
		Rate:      decimalStringPtr(value.Rate),
		RatedDays: value.Coverage.RatedDays,
		TotalDays: value.Coverage.TotalDays,
	}
	for _, point := range value.Points {
		pointDTO := wire.ReturnTrendPointDTO{
			Date:      string(point.Date),
			Amount:    wire.FromSignedMoneyPtr(point.Amount),
			Rate:      decimalStringPtr(point.Rate),
			Value:     wire.FromSignedMoneyPtr(point.Value),
			RatedDays: point.Coverage.RatedDays,
			TotalDays: point.Coverage.TotalDays,
		}
		setAvailability(&pointDTO.Available, &pointDTO.Status, &pointDTO.MissingReason, &pointDTO.ValuationForced, point.AnalysisAvailability)
		result.Points = append(result.Points, pointDTO)
	}
	setAvailability(&result.Available, &result.Status, &result.MissingReason, &result.ValuationForced, value.AnalysisAvailability)
	return result
}

func fromContribution(value application.ContributionResult) wire.ContributionDTO {
	result := wire.ContributionDTO{
		ReturnType: string(value.ReturnType),
		GroupBy:    string(value.GroupBy),
		Rows:       make([]wire.ContributionRowDTO, 0, len(value.Rows)),
		RatedDays:  value.Coverage.RatedDays,
		TotalDays:  value.Coverage.TotalDays,
	}
	for _, row := range value.Rows {
		rowDTO := wire.ContributionRowDTO{
			Key:       row.Key,
			Label:     row.Label,
			Amount:    wire.FromSignedMoneyPtr(row.Amount),
			Rate:      decimalStringPtr(row.Rate),
			RatedDays: row.Coverage.RatedDays,
			TotalDays: row.Coverage.TotalDays,
		}
		setAvailability(&rowDTO.Available, &rowDTO.Status, &rowDTO.MissingReason, &rowDTO.ValuationForced, row.AnalysisAvailability)
		result.Rows = append(result.Rows, rowDTO)
	}
	setAvailability(&result.Available, &result.Status, &result.MissingReason, &result.ValuationForced, value.AnalysisAvailability)
	return result
}

func fromContributionItem(value application.ContributionItemResult) wire.ContributionItemDTO {
	result := wire.ContributionItemDTO{
		Key:         value.Key,
		Label:       value.Label,
		Amount:      wire.FromSignedMoneyPtr(value.Amount),
		Rate:        decimalStringPtr(value.Rate),
		RatedDays:   value.Coverage.RatedDays,
		TotalDays:   value.Coverage.TotalDays,
		Components:  fromContributionComponents(value.Components),
		ByAccount:   fromContributionComponents(value.ByAccount),
		HistoryHint: fromContributionHistoryHint(value.HistoryHint),
	}
	setAvailability(&result.Available, &result.Status, &result.MissingReason, &result.ValuationForced, value.AnalysisAvailability)
	return result
}

func fromReturnComposition(values []application.ReturnComponentAmount) []wire.ReturnComponentAmountDTO {
	result := make([]wire.ReturnComponentAmountDTO, 0, len(values))
	for _, value := range values {
		result = append(result, wire.ReturnComponentAmountDTO{Component: string(value.Component), Amount: wire.FromSignedMoneyPtr(value.Amount)})
	}
	return result
}

func fromReturnContributors(values []application.ReturnContributor) []wire.ReturnContributorDTO {
	result := make([]wire.ReturnContributorDTO, 0, len(values))
	for _, value := range values {
		result = append(result, wire.ReturnContributorDTO{Key: value.Key, Label: value.Label, Amount: wire.FromSignedMoneyPtr(value.Amount), Rate: decimalStringPtr(value.Rate), RatedDays: value.Coverage.RatedDays, TotalDays: value.Coverage.TotalDays})
	}
	return result
}

func fromReturnSources(values []application.ReturnSource) []wire.ReturnSourceDTO {
	result := make([]wire.ReturnSourceDTO, 0, len(values))
	for _, value := range values {
		result = append(result, wire.ReturnSourceDTO{Key: value.Key, Label: value.Label, Amount: wire.FromSignedMoneyPtr(value.Amount), Share: decimalStringPtr(value.Share)})
	}
	return result
}

func fromReturnIssues(values []application.ReturnIssue) []wire.ReturnIssueDTO {
	result := make([]wire.ReturnIssueDTO, 0, len(values))
	for _, value := range values {
		result = append(result, wire.ReturnIssueDTO{Date: string(value.Date), Status: string(value.Status), MissingReason: value.MissingReason})
	}
	return result
}

func fromContributionComponents(values []application.ContributionComponent) []wire.ContributionComponentDTO {
	result := make([]wire.ContributionComponentDTO, 0, len(values))
	for _, value := range values {
		result = append(result, wire.ContributionComponentDTO{Key: value.Key, AccountID: value.AccountID, InstrumentID: value.InstrumentID, Currency: value.Currency, Amount: wire.FromSignedMoneyPtr(value.Amount)})
	}
	return result
}

func fromContributionHistoryHint(value application.ContributionHistoryHint) wire.ContributionHistoryHintDTO {
	return wire.ContributionHistoryHintDTO{Kinds: append([]string(nil), value.Kinds...), AccountID: value.AccountID, InstrumentID: value.InstrumentID, From: string(value.From), To: string(value.To)}
}

func decimalStringPtr(value *decimal.Decimal) *string {
	if value == nil {
		return nil
	}
	canonical := value.String()
	return &canonical
}

func setAvailability(available *bool, status *string, missingReason *string, valuationForced **string, value application.AnalysisAvailability) {
	*available = value.Available
	*status = string(value.Status)
	*missingReason = value.MissingReason
	*valuationForced = value.ValuationForced
}
