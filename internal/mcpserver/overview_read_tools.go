package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type OverviewReadInput struct {
	account.AccountFilterRequest
	Sections []string `json:"sections,omitempty" jsonschema:"Optional breakdown, evidence or label sections to include; omitted returns totals and quality"`
	Limit    int      `json:"limit,omitempty" jsonschema:"Initial rows per selected section, maximum 100; continuation default 25"`
}

var overviewSections = []string{"missingInputs", "assetsByType", "liabilitiesByType", "byMember", "byInstitution", "byGroup", "byAccountType", "recentActivities", "accountLabels", "instrumentLabels", "holdingLabels"}

type overviewSummary struct {
	DisplayValuation   wire.HouseholdDisplayValuationDTO `json:"displayValuation"`
	Currency           string                            `json:"currency"`
	AccountCount       int                               `json:"accountCount"`
	Complete           bool                              `json:"complete"`
	MissingInputsCount int                               `json:"missingInputsCount"`
	HistoryStarted     bool                              `json:"historyStarted"`
	TotalsScope        string                            `json:"totalsScope"`
}

// Keep the GUI's authoritative totals and quality, then serialize only the
// evidence requested by the caller.
func (s *Service) pagedOverview(ctx context.Context, g uint64, in OverviewReadInput) (ReadEnvelope, error) {
	if err := validateReadValues(in.Sections, overviewSections, false); err != nil {
		return ReadEnvelope{}, err
	}
	return s.buildRead(ctx, g, autoInlineWhenUnset(in.Limit), func(ctx context.Context) (readCapture, error) {
		value, err := portfolio.NewService(s.app).Overview(ctx, in.AccountFilterRequest)
		if err != nil {
			return readCapture{}, err
		}
		capture := readCapture{
			Kind: "current_overview", Query: in, AvailableSections: overviewSections,
			Summary: overviewSummary{
				DisplayValuation: value.DisplayValuation, Currency: value.Currency,
				AccountCount: value.AccountCount, Complete: value.Complete, MissingInputsCount: len(value.MissingInputs),
				HistoryStarted: value.HistoryStarted, TotalsScope: "request_account_filters",
			},
			Sections: map[string][]json.RawMessage{},
		}
		for _, section := range in.Sections {
			if _, exists := capture.Sections[section]; exists {
				continue
			}
			var rows []json.RawMessage
			switch section {
			case "missingInputs":
				rows, err = readRows(value.MissingInputs)
			case "assetsByType":
				rows, err = readRows(value.AssetsByType)
			case "liabilitiesByType":
				rows, err = readRows(value.LiabilitiesByType)
			case "byMember":
				rows, err = readRows(value.ByMember)
			case "byInstitution":
				rows, err = readRows(value.ByInstitution)
			case "byGroup":
				rows, err = readRows(value.ByGroup)
			case "byAccountType":
				rows, err = readRows(value.ByAccountType)
			case "recentActivities":
				rows, err = readRows(value.RecentActivities)
			case "accountLabels":
				rows, err = readRows(value.AccountLabels)
			case "instrumentLabels":
				rows, err = readRows(value.InstrumentLabels)
			case "holdingLabels":
				rows, err = readRows(value.HoldingLabels)
			}
			if err != nil {
				return readCapture{}, err
			}
			capture.Sections[section] = rows
		}
		return capture, nil
	})
}
