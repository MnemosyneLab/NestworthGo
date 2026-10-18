package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type UpcomingReadFilter struct {
	AccountIDs []string `json:"accountIds,omitempty"`
	ProductIDs []string `json:"productIds,omitempty"`
	Kinds      []string `json:"kinds,omitempty"`
	Groups     []string `json:"groups,omitempty"`
}
type UpcomingReadInput struct {
	ResponseMode string              `json:"responseMode,omitempty"`
	Filter       *UpcomingReadFilter `json:"filter,omitempty"`
	Sort         *ReadSort           `json:"sort,omitempty"`
	Limit        int                 `json:"limit,omitempty"`
}

func ptrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func readSummary(v any, remove ...string) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err = decodeReadValue(b, &out); err != nil {
		return nil, err
	}
	for _, k := range remove {
		delete(out, k)
	}
	return out, nil
}
func (s *Service) pagedUpcoming(ctx context.Context, g uint64, in UpcomingReadInput) (ReadEnvelope, error) {
	f := UpcomingReadFilter{}
	if in.Filter != nil {
		f = *in.Filter
	}
	for _, v := range [][]string{f.AccountIDs, f.ProductIDs} {
		if e := validateReadValues(v, nil, true); e != nil {
			return ReadEnvelope{}, e
		}
	}
	if e := validateReadValues(f.Kinds, []string{"reservation_unresolved", "reservation_shortfall", "source_access", "product_schedule"}, false); e != nil {
		return ReadEnvelope{}, e
	}
	if e := validateReadValues(f.Groups, []string{"attention", "scheduled", "eligible"}, false); e != nil {
		return ReadEnvelope{}, e
	}
	order := ReadSort{}
	if in.Sort != nil {
		order = *in.Sort
	}
	if e := validateReadSort(&order, []string{"priority", "sortOn", "displayName"}, "priority", "desc"); e != nil {
		return ReadEnvelope{}, e
	}
	if order.HorizonOn != "" {
		return ReadEnvelope{}, fail("validation", "upcoming sort does not accept horizonOn")
	}
	return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
		captured, err := s.app.UpcomingActionsRead(ctx)
		if err != nil {
			return readCapture{}, err
		}
		r := liquidity.FromUpcomingActions(captured.Actions)
		total := len(r.Items)
		selected := make([]liquidity.UpcomingActionDTO, 0, len(r.Items))
		for _, v := range r.Items {
			if hasString(f.AccountIDs, v.AccountID) && hasString(f.ProductIDs, ptrString(v.ProductID)) && hasString(f.Kinds, v.Kind) && hasString(f.Groups, v.Group) {
				selected = append(selected, v)
			}
		}
		sort.SliceStable(selected, func(i, j int) bool {
			a, b := selected[i], selected[j]
			cmp := 0
			switch order.Field {
			case "priority":
				cmp = a.Priority - b.Priority
				if order.Direction == "desc" {
					cmp = -cmp
				}
				if cmp == 0 {
					cmp = compareReadText(ptrString(a.SortOn), ptrString(b.SortOn), "asc")
				}
			case "sortOn":
				cmp = compareReadText(ptrString(a.SortOn), ptrString(b.SortOn), order.Direction)
			case "displayName":
				cmp = compareReadText(normalizedReadName(a.DisplayName), normalizedReadName(b.DisplayName), order.Direction)
			}
			if cmp == 0 {
				return a.ID < b.ID
			}
			return cmp < 0
		})
		checks := make([]liquidity.UpcomingDataCheckDTO, 0, len(r.DataChecks))
		for _, v := range r.DataChecks {
			account := captured.SourceAccountIDs[v.SourceKey]
			if v.Target.SourceRef != nil {
				account = v.Target.SourceRef.AccountID
			}
			if hasString(f.ProductIDs, ptrString(v.ProductID)) && hasString(f.AccountIDs, account) {
				checks = append(checks, v)
			}
		}
		sort.SliceStable(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
		rows, err := readRows(selected)
		if err != nil {
			return readCapture{}, err
		}
		checkRows, err := readRows(checks)
		if err != nil {
			return readCapture{}, err
		}
		summary, err := readSummary(r, "items", "dataChecks")
		if err != nil {
			return readCapture{}, err
		}
		summary["countsScope"], summary["itemsScope"], summary["totalItemCount"], summary["matchedItemCount"], summary["matchedDataCheckCount"] = "household", "filtered", total, len(rows), len(checks)
		summary["dataChecksFilterBasis"] = "accountIds/productIds only; kinds/groups select actions"
		capturedAt, _ := time.Parse(time.RFC3339Nano, r.AsOf)
		return readCapture{Kind: "upcoming_actions", CapturedAt: capturedAt, Query: map[string]any{"filter": f, "sort": order}, Summary: summary, Sections: map[string][]json.RawMessage{"items": rows, "dataChecks": checkRows}}, nil
	})
}

// validateReadQuery is shared by identity-discovery inputs.
func validateReadQuery(v string) error {
	if utf8.RuneCountInString(v) > 200 {
		return fail("validation", "query supports at most 200 characters")
	}
	return nil
}
