package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type InstrumentReadFilter struct {
	Types []string `json:"types,omitempty"`
	IDs   []string `json:"ids,omitempty"`
	Query string   `json:"query,omitempty"`
}
type InstrumentReadInput struct {
	IncludeArchived bool                  `json:"includeArchived,omitempty"`
	Query           string                `json:"query,omitempty"`
	ResponseMode    string                `json:"responseMode,omitempty"`
	Filter          *InstrumentReadFilter `json:"filter,omitempty"`
	Sort            *ReadSort             `json:"sort,omitempty"`
	Limit           int                   `json:"limit,omitempty"`
}
type instrumentReadRow struct {
	wire.InstrumentDTO
	MetadataVersion string `json:"metadataVersion"`
	Managed         bool   `json:"managed"`
}

func (s *Service) pagedInstruments(ctx context.Context, g uint64, in InstrumentReadInput) (ReadEnvelope, error) {
	f := InstrumentReadFilter{}
	if in.Filter != nil {
		f = *in.Filter
	}
	if in.Query != "" && f.Query != "" {
		return ReadEnvelope{}, fail("validation", "choose query or filter.query")
	}
	if in.Query != "" {
		f.Query = in.Query
	}
	if err := validateReadQuery(f.Query); err != nil {
		return ReadEnvelope{}, err
	}
	types := []string{}
	for _, v := range domain.AllInstrumentTypes() {
		types = append(types, string(v))
	}
	if err := validateReadValues(f.Types, types, false); err != nil {
		return ReadEnvelope{}, err
	}
	if err := validateReadValues(f.IDs, nil, true); err != nil {
		return ReadEnvelope{}, err
	}
	order := ReadSort{}
	if in.Sort != nil {
		order = *in.Sort
	}
	if err := validateReadSort(&order, []string{"displayName", "name", "type", "updatedAt", "id"}, "displayName", "asc"); err != nil {
		return ReadEnvelope{}, err
	}
	if order.HorizonOn != "" {
		return ReadEnvelope{}, fail("validation", "instrument sort does not accept horizonOn")
	}
	return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
		snapshot, err := s.app.FilteredInstrumentMetadata(ctx, domain.InstrumentMetadataReadQuery{IncludeArchived: in.IncludeArchived, IDs: f.IDs, Types: f.Types})
		if err != nil {
			return readCapture{}, err
		}
		items := snapshot.Items
		total := snapshot.TotalCount
		selected := items[:0]
		for _, v := range items {
			i := v.Instrument
			if hasString(f.Types, string(i.Type)) && hasString(f.IDs, i.ID.String()) && matches(f.Query, i.Name, i.ShortName, ptrString(i.Symbol)) {
				selected = append(selected, v)
			}
		}
		sort.SliceStable(selected, func(i, j int) bool {
			a, b := selected[i].Instrument, selected[j].Instrument
			av, bv := normalizedReadName(a.DisplayLabel()), normalizedReadName(b.DisplayLabel())
			switch order.Field {
			case "name":
				av, bv = normalizedReadName(a.Name), normalizedReadName(b.Name)
			case "type":
				av, bv = string(a.Type), string(b.Type)
			case "updatedAt":
				if a.UpdatedAt.Equal(b.UpdatedAt) {
					return a.ID.String() < b.ID.String()
				}
				if order.Direction == "desc" {
					return a.UpdatedAt.After(b.UpdatedAt)
				}
				return a.UpdatedAt.Before(b.UpdatedAt)
			case "id":
				av, bv = a.ID.String(), b.ID.String()
			}
			cmp := compareReadText(av, bv, order.Direction)
			if cmp == 0 {
				return a.ID.String() < b.ID.String()
			}
			return cmp < 0
		})
		rows := make([]instrumentReadRow, 0, len(selected))
		for _, v := range selected {
			rows = append(rows, instrumentReadRow{wire.FromInstrument(v.Instrument), domain.InstrumentMetadataVersion(v.Instrument, v.Managed), v.Managed})
		}
		raw, err := readRows(rows)
		return readCapture{Kind: "instruments", Query: map[string]any{"includeArchived": in.IncludeArchived, "filter": f, "sort": order}, Summary: map[string]any{"totalInstrumentCount": total, "matchedInstrumentCount": len(rows)}, Sections: map[string][]json.RawMessage{"instruments": raw}}, err
	})
}
func (s *Service) instrumentBatchTools(server *mcp.Server, mode string) {
	if mode == ReadOnly {
		return
	}
	writeTool(s, server, "update_instruments_batch", "Atomically update metadata for 1 to 100 verified instrument IDs using expectedMetadataVersion from paged list_instruments. Only name/shortName/note; omission preserves, empty shortName clears, note=null clears. Managed or archived instruments and any stale version reject the whole batch. No ledger or quote changes. Short receipt; pending/unknown outcomes require get_operation and current metadata before another intention.", func(ctx context.Context, in application.InstrumentMetadataBatchInput) (any, error) {
		if len(in.Items) < 1 || len(in.Items) > 100 {
			return nil, fail("validation", "batch supports 1 to 100 items")
		}
		if ok, err := metadataReceiptBudget(); err != nil {
			return nil, err
		} else if !ok {
			return nil, fail("too_large", "batch receipt exceeds wire budget")
		}
		return s.app.UpdateInstrumentsBatch(ctx, in)
	}, instrumentMetadataBatchSchema())
}

func instrumentMetadataBatchSchema() any {
	str := map[string]any{"type": "string"}
	changes := map[string]any{"type": "object", "additionalProperties": false, "minProperties": 1, "properties": map[string]any{"name": str, "shortName": str, "note": map[string]any{"type": []string{"string", "null"}}}}
	item := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "expectedMetadataVersion", "changes"}, "properties": map[string]any{"id": str, "expectedMetadataVersion": str, "changes": changes}}
	input := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"items"}, "properties": map[string]any{"items": map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": item}}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"operationId", "input"}, "properties": map[string]any{"operationId": str, "input": input}}
}

func metadataReceiptBudget() (bool, error) {
	receipt := domain.InstrumentMetadataReceipt{RequestedCount: 100, ChangedCount: 100, Items: make([]domain.InstrumentMetadataReceiptItem, 100)}
	for i := range receipt.Items {
		receipt.Items[i] = domain.InstrumentMetadataReceiptItem{ID: strings.Repeat("a", 36), Changed: true, MetadataVersion: strings.Repeat("a", 64)}
	}
	raw, err := jsoncodec.Marshal(receipt)
	if err != nil {
		return false, err
	}
	return readFits(Operation{ID: strings.Repeat("a", 36), Tool: "update_instruments_batch", Hash: strings.Repeat("a", 64), Status: "succeeded", CreatedAt: "2026-10-13T12:00:00.123456789Z", Result: raw})
}
