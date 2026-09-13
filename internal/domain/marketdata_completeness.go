package domain

import (
	"strings"
	"time"
)

type HistoryCompleteness struct {
	VerifiedRanges  []InclusiveDateRange
	PendingRanges   []InclusiveDateRange
	UncertainRanges []InclusiveDateRange
	Status          string
	Reason          string
}

type InclusiveDateRange struct {
	Start string
	End   string
}

// ClassifyHistoryCompleteness assigns verified, pending, and uncertain
// subranges of an inclusive requested market-date range. HTTP 200 is not
// evidence of completeness. Only verified finalized subranges may produce
// expiring no-data; pending and uncertain remain retryable.
func ClassifyHistoryCompleteness(requestedStart, requestedEnd, lastFinalized string, pendingDates []string, truncated, malformed bool) (HistoryCompleteness, error) {
	if malformed {
		return HistoryCompleteness{Status: "invalid", Reason: "invalid_batch"}, nil
	}
	dates, err := InclusiveMarketDates(requestedStart, requestedEnd)
	if err != nil {
		return HistoryCompleteness{}, err
	}
	pendingSet := make(map[string]struct{}, len(pendingDates))
	for _, date := range pendingDates {
		parsed, parseErr := ParseMarketDate(date)
		if parseErr != nil {
			return HistoryCompleteness{}, parseErr
		}
		pendingSet[parsed] = struct{}{}
	}
	if truncated {
		return HistoryCompleteness{
			UncertainRanges: []InclusiveDateRange{{Start: requestedStart, End: requestedEnd}},
			Status:          "uncertain",
			Reason:          "truncated_response",
		}, nil
	}
	if strings.TrimSpace(lastFinalized) == "" {
		return HistoryCompleteness{
			PendingRanges: []InclusiveDateRange{{Start: requestedStart, End: requestedEnd}},
			Status:        "pending",
			Reason:        "publication_not_ready",
		}, nil
	}
	finalized, err := ParseMarketDate(lastFinalized)
	if err != nil {
		return HistoryCompleteness{}, err
	}
	var verified, pending, uncertain []string
	for _, date := range dates {
		switch {
		case dateInSet(date, pendingSet):
			pending = append(pending, date)
		case date <= finalized:
			verified = append(verified, date)
		default:
			uncertain = append(uncertain, date)
		}
	}
	return HistoryCompleteness{
		VerifiedRanges:  collapseDates(verified),
		PendingRanges:   collapseDates(pending),
		UncertainRanges: collapseDates(uncertain),
		Status:          "classified",
	}, nil
}

func dateInSet(date string, set map[string]struct{}) bool {
	_, ok := set[date]
	return ok
}

func collapseDates(dates []string) []InclusiveDateRange {
	if len(dates) == 0 {
		return nil
	}
	ranges := make([]InclusiveDateRange, 0, 1)
	start, prev := dates[0], dates[0]
	for _, date := range dates[1:] {
		next, err := addMarketDays(prev, 1)
		if err != nil || date != next {
			ranges = append(ranges, InclusiveDateRange{Start: start, End: prev})
			start = date
		}
		prev = date
	}
	return append(ranges, InclusiveDateRange{Start: start, End: prev})
}

func addMarketDays(date string, days int) (string, error) {
	parsed, err := ParseMarketDate(date)
	if err != nil {
		return "", err
	}
	value, err := time.Parse("2006-01-02", parsed)
	if err != nil {
		return "", validation("marketDate", "must use YYYY-MM-DD")
	}
	return value.AddDate(0, 0, days).Format("2006-01-02"), nil
}
