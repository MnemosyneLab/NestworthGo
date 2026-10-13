package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
)

type AgentReadFilter struct {
	InstrumentIDs    []string                       `json:"instrumentIds,omitempty"`
	FXPairs          []application.MarketReadTarget `json:"fxPairs,omitempty"`
	Operations       []string                       `json:"operations,omitempty"`
	Active           *bool                          `json:"active,omitempty"`
	ObservationKinds []string                       `json:"observationKinds,omitempty"`
	FromDate         string                         `json:"fromDate,omitempty"`
	ToDate           string                         `json:"toDate,omitempty"`
	CreatedFrom      string                         `json:"createdFrom,omitempty"`
	CreatedTo        string                         `json:"createdTo,omitempty"`
}

func (s *Service) pagedAgentRecords(ctx context.Context, g uint64, in AgentDataListInput) (ReadEnvelope, error) {
	if in.Offset != 0 {
		return ReadEnvelope{}, fail("validation", "offset cannot be used with responseMode=paged")
	}
	f := AgentReadFilter{}
	if in.Filter != nil {
		f = *in.Filter
	}
	if err := validateReadValues(f.InstrumentIDs, nil, true); err != nil {
		return ReadEnvelope{}, err
	}
	if err := validateReadValues(f.Operations, []string{"append", "correct", "retract"}, false); err != nil {
		return ReadEnvelope{}, err
	}
	if err := validateReadValues(f.ObservationKinds, []string{"latest", "close", "nav", "daily_reference"}, false); err != nil {
		return ReadEnvelope{}, err
	}
	for _, v := range []string{f.FromDate, f.ToDate} {
		if v != "" {
			if _, err := time.Parse(time.DateOnly, v); err != nil {
				return ReadEnvelope{}, fail("validation", "observation dates must be YYYY-MM-DD")
			}
		}
	}
	if f.FromDate != "" && f.ToDate != "" && f.FromDate > f.ToDate {
		return ReadEnvelope{}, fail("validation", "observation date range must be forward")
	}
	var from, to *time.Time
	for i, v := range []string{f.CreatedFrom, f.CreatedTo} {
		if v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return ReadEnvelope{}, fail("validation", "created bounds must be RFC3339 timestamps")
			}
			if i == 0 {
				from = &t
			} else {
				to = &t
			}
		}
	}
	if from != nil && to != nil && from.After(*to) {
		return ReadEnvelope{}, fail("validation", "created range must be forward")
	}
	pairs := map[string]bool{}
	if len(f.FXPairs) > 100 {
		return ReadEnvelope{}, fail("validation", "fxPairs supports at most 100 values")
	}
	for n, p := range f.FXPairs {
		if p.InstrumentID != "" {
			return ReadEnvelope{}, fail("validation", "fxPairs requires oriented currencies only")
		}
		normalized, err := application.NormalizeMarketRead(application.MarketReadRequest{Targets: []application.MarketReadTarget{p}})
		if err != nil {
			return ReadEnvelope{}, err
		}
		f.FXPairs[n] = normalized.Targets[0]
		pairs[normalized.Targets[0].Key()] = true
	}
	order := ReadSort{}
	if in.Sort != nil {
		order = *in.Sort
	}
	if err := validateReadSort(&order, []string{"createdAt", "observationDate", "operation"}, "createdAt", "desc"); err != nil {
		return ReadEnvelope{}, err
	}
	if order.HorizonOn != "" {
		return ReadEnvelope{}, fail("validation", "Agent sort does not accept horizonOn")
	}
	return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
		records, err := s.app.AgentMarketDataRecords(ctx)
		if err != nil {
			return readCapture{}, err
		}
		inactive := map[string]bool{}
		for _, v := range records {
			if v.TargetQuoteID != "" {
				inactive[v.TargetQuoteID] = true
			}
		}
		identities := map[string]AgentDataRecordDTO{}
		all := make([]AgentDataRecordDTO, 0, len(records))
		created := map[string]time.Time{}
		for _, v := range records {
			dto := agentRecordDTO(v, inactive)
			all = append(all, dto)
			created[dto.RecordID] = v.CreatedAt
			if dto.QuoteID != "" {
				identities[dto.QuoteID] = dto
			}
		}
		rows := []AgentDataRecordDTO{}
		for _, v := range all {
			identity := v
			if identity.InstrumentID == "" && identity.BaseCurrency == "" && v.TargetQuoteID != "" {
				if target, ok := identities[v.TargetQuoteID]; ok {
					identity = target
					v.InstrumentID, v.BaseCurrency, v.QuoteCurrency, v.Currency, v.Kind, v.Date, v.QuotedAt = target.InstrumentID, target.BaseCurrency, target.QuoteCurrency, target.Currency, target.Kind, target.Date, target.QuotedAt
				}
			}
			if !hasString(f.InstrumentIDs, identity.InstrumentID) || !hasString(f.Operations, v.Operation) || !hasString(f.ObservationKinds, identity.Kind) || (f.Active != nil && v.Active != *f.Active) || (len(pairs) > 0 && !pairs[application.MarketReadTarget{BaseCurrency: identity.BaseCurrency, QuoteCurrency: identity.QuoteCurrency}.Key()]) || (f.FromDate != "" && (identity.Date == "" || identity.Date < f.FromDate)) || (f.ToDate != "" && (identity.Date == "" || identity.Date > f.ToDate)) || (from != nil && created[v.RecordID].Before(*from)) || (to != nil && created[v.RecordID].After(*to)) {
				continue
			}
			rows = append(rows, v)
		}
		sortAgentReadRecords(rows, created, order)
		raw, err := readRows(rows)
		return readCapture{Kind: "agent_market_data", Query: map[string]any{"filter": f, "sort": order}, Summary: map[string]any{"totalRecordCount": len(records), "matchedRecordCount": len(rows), "activeBasis": "not corrected or retracted; independent of current valuation selection"}, Sections: map[string][]json.RawMessage{"records": raw}}, err
	})
}

func sortAgentReadRecords(rows []AgentDataRecordDTO, created map[string]time.Time, order ReadSort) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if order.Field == "createdAt" {
			av, bv := created[a.RecordID], created[b.RecordID]
			if !av.Equal(bv) {
				if order.Direction == "desc" {
					return av.After(bv)
				}
				return av.Before(bv)
			}
			return a.RecordID < b.RecordID
		}
		av, bv := a.Date, b.Date
		if order.Field == "operation" {
			av, bv = a.Operation, b.Operation
		}
		cmp := compareReadText(av, bv, order.Direction)
		if cmp == 0 {
			return a.RecordID < b.RecordID
		}
		return cmp < 0
	})
}
