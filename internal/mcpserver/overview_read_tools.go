package mcpserver

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
)

type OverviewReadInput struct {
	account.AccountFilterRequest
	Limit int `json:"limit,omitempty" jsonschema:"Paged initial default 5 per section, maximum 100; continuation default 25"`
}

// Preserve the complete current GUI overview, including all breakdowns,
// missing inputs, recent activities and identity labels. Only its collections
// move out of the summary; no total or completeness calculation is repeated.
func (s *Service) pagedOverview(ctx context.Context, g uint64, in OverviewReadInput) (ReadEnvelope, error) {
	limit := in.Limit
	if limit == 0 {
		limit = 5
	}
	return s.buildRead(ctx, g, limit, func(ctx context.Context) (readCapture, error) {
		value, err := portfolio.NewService(s.app).Overview(ctx, in.AccountFilterRequest)
		if err != nil {
			return readCapture{}, err
		}
		capture, err := sectionedReadCapture("current_overview", in, value, "missingInputs", "assetsByType", "liabilitiesByType", "byMember", "byInstitution", "byGroup", "byAccountType", "recentActivities", "accountLabels", "instrumentLabels", "holdingLabels")
		if err != nil {
			return capture, err
		}
		capture.Summary.(map[string]any)["totalsScope"] = "request_account_filters"
		return capture, nil
	})
}
