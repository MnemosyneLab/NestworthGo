package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
)

type AccountValuationReadInput struct {
	account.AccountFilterRequest
	AccountIDs []string `json:"accountIds,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

func (s *Service) pagedAccountValuations(ctx context.Context, g uint64, in AccountValuationReadInput) (ReadEnvelope, error) {
	if err := validateReadValues(in.AccountIDs, nil, true); err != nil {
		return ReadEnvelope{}, err
	}
	return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
		capturedAt := time.Now()
		values, err := account.NewService(s.app).AccountValuations(ctx, in.AccountFilterRequest)
		if err != nil {
			return readCapture{}, err
		}
		c := readCapture{Kind: "account_valuations", CapturedAt: capturedAt, Query: in, InitialSections: []string{"accounts"}, Sections: map[string][]json.RawMessage{}, Details: map[string]any{}, DetailSections: map[string][]string{}}
		sort.Slice(values, func(i, j int) bool { return values[i].Account.ID < values[j].Account.ID })
		rows := []map[string]any{}
		for _, v := range values {
			id := v.Account.ID
			if !hasString(in.AccountIDs, id) {
				continue
			}
			full, e := readSummary(v)
			if e != nil {
				return c, e
			}
			// Complete GUI layouts remain independently retrievable. The default row
			// carries their counts and exact totals, not repeated ranked detail arrays.
			detail, e := readSummary(v, "components", "missingInputs")
			if e != nil {
				return c, e
			}
			sections := map[string]any{"components": v.Components, "missingInputs": v.MissingInputs}
			if composition, ok := detail["composition"].(map[string]any); ok {
				for _, key := range []string{"types", "details", "detailSlices"} {
					sections["composition_"+key] = composition[key]
					delete(composition, key)
				}
			}
			if display, ok := detail["displayValue"].(map[string]any); ok {
				for _, key := range []string{"missingInputs", "fxEvidence"} {
					sections["display_"+key] = display[key]
					delete(display, key)
				}
			}
			keys := []string{}
			for name := range sections {
				keys = append(keys, name)
			}
			sort.Strings(keys)
			for _, name := range keys {
				raw, e := readSummary(map[string]any{"items": sections[name]})
				if e != nil {
					return c, e
				}
				items, ok := raw["items"].([]any)
				if !ok {
					items = []any{}
				}
				section := name + "/" + id
				c.Sections[section], err = readRows(items)
				if err != nil {
					return c, err
				}
				c.DetailSections["accounts/"+id] = append(c.DetailSections["accounts/"+id], section)
			}
			c.Details["accounts/"+id] = detail
			header := map[string]any{"id": id, "name": v.Account.Name, "aggregation": "rollup", "complete": v.Complete, "baseValue": full["baseValue"], "displayValue": detail["displayValue"], "composition": detail["composition"], "componentCount": len(v.Components), "missingInputCount": len(v.MissingInputs)}
			for _, key := range []string{"defaultCurrency", "trackingMode", "balanceSheetRole", "includeInNetWorth"} {
				header[key] = full["account"].(map[string]any)[key]
			}
			rows = append(rows, header)
		}
		c.Summary = map[string]any{"totalAccountCount": len(values), "matchedAccountCount": len(rows), "countsScope": "request_account_filters", "itemsScope": "accountIds within request filters", "aggregation": "account rollups; do not add components again"}
		c.Sections["accounts"], err = readRows(rows)
		return c, err
	})
}
