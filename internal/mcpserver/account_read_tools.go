package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type AccountValuationReadInput struct {
	account.AccountFilterRequest
	AccountIDs []string `json:"accountIds,omitempty"`
	Detail     string   `json:"detail,omitempty" jsonschema:"summary (default) or holdings; holdings includes the first page of components and missing inputs."`
	Limit      int      `json:"limit,omitempty"`
}

func (s *Service) pagedAccountValuations(ctx context.Context, g uint64, in AccountValuationReadInput) (ReadEnvelope, error) {
	if in.Detail != "" && in.Detail != "summary" && in.Detail != "holdings" {
		return ReadEnvelope{}, fail("validation", "detail must be summary or holdings")
	}
	if err := validateReadValues(in.AccountIDs, nil, true); err != nil {
		return ReadEnvelope{}, err
	}
	return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
		capturedAt := time.Now()
		filter, err := in.AccountFilterRequest.ToDomain()
		if err != nil {
			return readCapture{}, err
		}
		for _, raw := range in.AccountIDs {
			id, e := domain.ParseAccountID(raw)
			if e != nil {
				return readCapture{}, e
			}
			filter.AccountIDs = append(filter.AccountIDs, id)
		}
		valued, err := s.app.AccountValuations(ctx, filter)
		values := wire.FromAccountValuations(valued)
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
			header := map[string]any{"id": id, "name": v.Account.Name, "aggregation": "rollup", "complete": v.Complete, "baseValue": full["baseValue"], "displayValue": detail["displayValue"], "componentCount": len(v.Components), "missingInputCount": len(v.MissingInputs)}
			for _, key := range []string{"defaultCurrency", "trackingMode", "balanceSheetRole", "includeInNetWorth", "includeInLiquidAssets", "includeInPortfolio"} {
				header[key] = full["account"].(map[string]any)[key]
			}
			header["institutionName"], header["groupName"] = v.InstitutionName, v.GroupName
			header["ownership"] = v.Ownership
			header["priceFreshness"] = accountPriceFreshness(v.Components)
			if v.Composition != nil {
				header["assetTypes"] = accountAssetTypes(v.Composition)
				header["compositionStatus"] = v.Composition.Status
			}
			// Exact base/display totals; GUI composition totals/layouts stay
			// available through explicit evidence drilldown, not repeated here.
			if in.Detail == "holdings" {
				c.InitialSections = append(c.InitialSections, "components/"+id, "missingInputs/"+id)
			}
			rows = append(rows, header)
		}
		c.Summary = map[string]any{"totalAccountCount": len(values), "matchedAccountCount": len(rows), "countsScope": "request_account_filters_and_accountIds", "itemsScope": "accountIds within request filters", "aggregation": "account rollups; do not add components again"}
		c.Sections["accounts"], err = readRows(rows)
		return c, err
	})
}

// Summarize quote age without guessing freshness from wall-clock age. Keep
// manual, missing, delayed and carried evidence separate from completeness.
func accountPriceFreshness(components []wire.ValuationComponentDTO) map[string]any {
	counts := map[string]int{}
	carried, delayed, without := 0, 0, 0
	oldest, newest := "", ""
	for _, component := range components {
		e := component.PriceEvidence
		if e == nil {
			without++
			continue
		}
		counts[e.Freshness]++
		if e.Carried {
			carried++
		}
		if e.Delayed {
			delayed++
		}
		if e.EffectiveDate != nil && *e.EffectiveDate != "" {
			d := *e.EffectiveDate
			if oldest == "" || d < oldest {
				oldest = d
			}
			if d > newest {
				newest = d
			}
		}
	}
	out := map[string]any{"counts": counts, "carriedCount": carried, "delayedCount": delayed, "withoutPriceEvidenceCount": without}
	if oldest != "" {
		out["oldestEffectiveDate"], out["newestEffectiveDate"] = oldest, newest
	}
	return out
}

func accountAssetTypes(composition *wire.AccountCompositionDTO) []map[string]any {
	rows := make([]map[string]any, 0, len(composition.Types))
	for _, v := range composition.Types {
		var amount any
		if v.BaseAmountExact != "" {
			amount = v.BaseAmountExact
		}
		rows = append(rows, map[string]any{"type": v.TypeKey, "amountExact": amount, "currency": composition.Currency, "shareBps": v.ShareBPS, "valuedCount": v.ValuedCount, "missingCount": v.MissingCount})
	}
	return rows
}
