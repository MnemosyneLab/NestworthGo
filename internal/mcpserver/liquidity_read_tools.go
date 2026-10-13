package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type ReadSort struct {
	Field     string `json:"field,omitempty"`
	Direction string `json:"direction,omitempty"`
	HorizonOn string `json:"horizonOn,omitempty"`
}
type LiquidityReadFilter struct {
	AccountIDs      []string                 `json:"accountIds,omitempty"`
	SourceRefs      []liquidity.SourceRefDTO `json:"sourceRefs,omitempty"`
	SourceKinds     []string                 `json:"sourceKinds,omitempty"`
	InstrumentIDs   []string                 `json:"instrumentIds,omitempty"`
	InstrumentTypes []string                 `json:"instrumentTypes,omitempty"`
	AccessKinds     []string                 `json:"accessKinds,omitempty"`
	PolicyOrigins   []string                 `json:"policyOrigins,omitempty"`
	HasRule         *bool                    `json:"hasRule,omitempty"`
	EarlyKinds      []string                 `json:"earlyKinds,omitempty"`
	Currencies      []string                 `json:"currencies,omitempty"`
	Excluded        *bool                    `json:"excluded,omitempty"`
	Query           string                   `json:"query,omitempty"`
}
type liquidityReadRow struct {
	liquidity.LiquiditySourceDTO
	InstrumentID   *string `json:"instrumentId"`
	InstrumentType *string `json:"instrumentType"`
	ProductKind    *string `json:"productKind"`
	HasRule        *bool   `json:"hasRule"`
	PolicyEditable *bool   `json:"policyEditable"`
	PolicyReason   string  `json:"policyReason,omitempty"`
}

func hasString(values []string, v string) bool {
	if len(values) == 0 {
		return true
	}
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
func validateReadValues(values []string, allowed []string, ids bool) error {
	if len(values) > 100 {
		return fail("validation", "filter arrays support at most 100 values")
	}
	for i, v := range values {
		if v == "" || (ids && uuid.Validate(v) != nil) || (allowed != nil && !hasString(allowed, v)) {
			return fail("validation", "invalid filter value; inspect tool schema")
		}
		if ids {
			parsed, _ := uuid.Parse(v)
			values[i] = parsed.String()
		}
	}
	return nil
}
func validateReadSort(s *ReadSort, fields []string, field, direction string) error {
	if s.Field == "" {
		s.Field = field
	}
	if s.Direction == "" {
		s.Direction = direction
	}
	if !hasString(fields, s.Field) || (s.Direction != "asc" && s.Direction != "desc") {
		return fail("validation", "invalid sort field or direction")
	}
	return nil
}
func validateResponseMode(mode string, hasOptions bool) error {
	if mode != "" && mode != "paged" {
		return fail("validation", "responseMode must be paged or omitted")
	}
	if mode == "" && hasOptions {
		return fail("validation", "filter, sort and paging options require responseMode=paged")
	}
	return nil
}
func (s *Service) pagedLiquidity(ctx context.Context, generation uint64, in LiquidityOverviewInput) (ReadEnvelope, error) {
	f := in.Filter
	if f == nil {
		f = &LiquidityReadFilter{}
	}
	if utf8.RuneCountInString(f.Query) > 200 {
		return ReadEnvelope{}, fail("validation", "query supports at most 200 characters")
	}
	for _, v := range [][]string{f.AccountIDs, f.InstrumentIDs} {
		if err := validateReadValues(v, nil, true); err != nil {
			return ReadEnvelope{}, err
		}
	}
	enums := []struct{ v, a []string }{{f.SourceKinds, []string{"account_value", "account_cash", "holding"}}, {f.InstrumentTypes, []string{"stock", "etf", "mutual_fund", "crypto", "bond", "precious_metal", "bank_investment_product", "other"}}, {f.AccessKinds, []string{"on_request", "on_date", "unknown", "excluded"}}, {f.PolicyOrigins, []string{"assumed", "explicit", "contract"}}, {f.EarlyKinds, []string{"allowed", "not_allowed", "unknown"}}}
	for _, v := range enums {
		if err := validateReadValues(v.v, v.a, false); err != nil {
			return ReadEnvelope{}, err
		}
	}
	if err := validateReadValues(f.Currencies, nil, false); err != nil {
		return ReadEnvelope{}, err
	}
	for n, v := range f.Currencies {
		currency, err := domain.ParseCurrency(v)
		if err != nil {
			return ReadEnvelope{}, err
		}
		f.Currencies[n] = currency.String()
	}
	if len(f.SourceRefs) > 100 {
		return ReadEnvelope{}, fail("validation", "sourceRefs supports at most 100 values")
	}
	keys := []string{}
	for n, ref := range f.SourceRefs {
		v, e := ref.ToDomain()
		if e != nil {
			return ReadEnvelope{}, e
		}
		canonical := liquidity.SourceRefDTO{Kind: string(v.Kind), AccountID: v.AccountID.String()}
		if v.HoldingID != nil {
			id := v.HoldingID.String()
			canonical.HoldingID = &id
		}
		if v.Currency != nil {
			currency := v.Currency.String()
			canonical.Currency = &currency
		}
		f.SourceRefs[n] = canonical
		keys = append(keys, v.Key())
	}
	sort := ReadSort{}
	if in.Sort != nil {
		sort = *in.Sort
	}
	if err := validateReadSort(&sort, []string{"sourceKey", "displayName", "instrumentType", "accessKind", "policyOrigin", "normalReceiptOn", "currentValueBase", "availableBase"}, "sourceKey", "asc"); err != nil {
		return ReadEnvelope{}, err
	}
	if sort.Field != "availableBase" && sort.HorizonOn != "" {
		return ReadEnvelope{}, fail("validation", "horizonOn is only valid for availableBase")
	}
	return s.buildRead(ctx, generation, in.Limit, func(ctx context.Context) (readCapture, error) {
		result, err := s.app.LiquidityRead(ctx, application.LiquidityOverviewQuery{CustomHorizonOn: in.CustomHorizonOn, IncludeEarlyWithdrawal: in.IncludeEarlyWithdrawal})
		if err != nil {
			return readCapture{}, err
		}
		if sort.Field == "availableBase" {
			valid := false
			for _, v := range result.Overview.Buckets {
				valid = valid || v.HorizonOn == sort.HorizonOn
			}
			if !valid {
				return readCapture{}, fail("validation", "availableBase requires horizonOn belonging to captured buckets")
			}
		}
		total := len(result.Sources)
		matched := []application.LiquidityReadSource{}
		for _, v := range result.Sources {
			r := v.Result
			id, kind := "", ""
			if v.InstrumentID != nil {
				id = *v.InstrumentID
			}
			if v.InstrumentType != nil {
				kind = *v.InstrumentType
			}
			access, early := "", ""
			if r.Policy != nil {
				access = string(r.Policy.AccessKind)
				early = string(r.Policy.EarlyKind)
			}
			if !hasString(f.AccountIDs, r.AccountID.String()) || !hasString(keys, r.SourceKey) || !hasString(f.SourceKinds, string(r.Ref.Kind)) || !hasString(f.InstrumentIDs, id) || !hasString(f.InstrumentTypes, kind) || !hasString(f.AccessKinds, access) || !hasString(f.PolicyOrigins, string(r.PolicyOrigin)) || !hasString(f.EarlyKinds, early) || !hasString(f.Currencies, r.NativeCurrency.String()) || !matches(f.Query, v.SearchText) || (f.HasRule != nil && (v.HasRule == nil || *v.HasRule != *f.HasRule)) || (f.Excluded != nil && r.Excluded != *f.Excluded) {
				continue
			}
			matched = append(matched, v)
		}
		application.SortLiquidityReadSources(matched, sort.Field, sort.Direction, sort.HorizonOn)
		overview := result.Overview
		overview.Sources = []domain.LiquiditySourceResult{}
		for _, v := range matched {
			overview.Sources = append(overview.Sources, v.Result)
		}
		dto := liquidity.FromOverview(overview)
		rows := make([]liquidityReadRow, 0, len(matched))
		for i, v := range matched {
			rows = append(rows, liquidityReadRow{dto.Sources[i], v.InstrumentID, v.InstrumentType, v.ProductKind, v.HasRule, v.PolicyEditable, v.PolicyReason})
		}
		sourceRows, err := readRows(rows)
		if err != nil {
			return readCapture{}, err
		}
		reserveRows, err := readRows(dto.UnresolvedReservations)
		if err != nil {
			return readCapture{}, err
		}
		summary := map[string]any{"asOf": dto.AsOf, "localDate": dto.LocalDate, "timezone": dto.Timezone, "baseCurrency": dto.BaseCurrency, "assumptions": dto.Assumptions, "buckets": dto.Buckets, "totalsScope": "household", "sourcesScope": "filtered", "unresolvedReservationsScope": "household", "totalSourceCount": total, "matchedSourceCount": len(rows), "filteredOutSourceCount": total - len(rows), "unresolvedReservationCount": len(reserveRows), "sortBasis": "same_capture_exact_base_currency", "sortHorizonOn": sort.HorizonOn}
		f.Query = strings.ToLower(strings.TrimSpace(f.Query))
		return readCapture{Kind: "liquidity", CapturedAt: overview.AsOf, Query: map[string]any{"filter": f, "sort": sort, "customHorizonOn": in.CustomHorizonOn, "includeEarlyWithdrawal": in.IncludeEarlyWithdrawal}, Summary: summary, Sections: map[string][]json.RawMessage{"sources": sourceRows, "unresolvedReservations": reserveRows}}, nil
	})
}
