package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/waltwang/nestworth-go/internal/application"
)

type MarketComparisonInput struct {
	View         string                             `json:"view,omitempty" jsonschema:"summary (default): compact results, details deferred in the same capture; full: upfront bulk evidence for complete review"`
	Items        []application.MarketComparisonItem `json:"items" jsonschema:"1 to 500 ordered rows; unique clientItemId; maximum 100 distinct targets and 128 KiB input"`
	DisplayScale *int                               `json:"displayScale,omitempty" jsonschema:"0 to 12 display decimal places, default instrument 4 / FX 6. Never a tolerance: any exact difference remains conflict."`
	Limit        int                                `json:"limit,omitempty" jsonschema:"Page rows, default 25, maximum 100; final byte budget may reduce this"`
}

func (s *Service) marketComparisonTools(server *toolSet, g uint64) {
	readTool(server, "compare_market_data_import", "Compare 1-500 proposed quote rows against local data (up to 100 targets). Use unique clientItemId values, source dates for daily rows and quotedAt for latest rows. close is raw close; nav is unit NAV. Classifications use exact decimals, original FX direction and provenance. Missing means locally absent. Nothing is imported or authorized. Default summary pages compact results; use full for bulk evidence, or get_read_item with section=results and id=clientItemId for one row. Comparison pages default to 25 rows, maximum 100; continue each section with get_read_page({cursor,limit?}).", func(ctx context.Context, in MarketComparisonInput) (any, error) {
		if in.Limit == 0 {
			in.Limit = 25
		}
		view := in.View
		if view == "" {
			view = "summary"
		}
		if view != "summary" && view != "full" {
			return nil, fail("validation", "view must be summary or full")
		}
		return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
			r, err := s.app.CompareMarketData(ctx, application.MarketComparisonRequest{Items: in.Items, DisplayScale: in.DisplayScale})
			if err != nil {
				return readCapture{}, err
			}
			statusCounts := map[string]int{}
			batchConflicts := 0
			for _, row := range r.Results {
				statusCounts[row.Status]++
				if row.BatchConflict {
					batchConflicts++
				}
			}
			sections := map[string][]json.RawMessage{}
			if sections["targets"], err = readRows(r.Targets); err != nil {
				return readCapture{}, err
			}
			if sections["results"], err = readRows(r.Results); err != nil {
				return readCapture{}, err
			}
			if sections["matches"], err = readRows(r.Matches); err != nil {
				return readCapture{}, err
			}
			if sections["evidence"], err = readRows(r.Evidence); err != nil {
				return readCapture{}, err
			}
			if sections["audits"], err = readRows(r.Audits); err != nil {
				return readCapture{}, err
			}
			// Hash the typed ordered input before the generic set normalizer; do not
			// duplicate it in Query. Original rows appear exactly once, in results.
			return readCapture{
				Kind: "market_import_comparison", CapturedAt: r.CapturedAt,
				Query:    map[string]any{"inputHash": r.InputHash, "itemCount": len(in.Items), "view": view},
				Summary:  map[string]any{"statusCounts": statusCounts, "batchConflictRowCount": batchConflicts, "writeAuthorized": false},
				Sections: sections,
			}, nil
		})
	})
}
