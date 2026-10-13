package application

import (
	"context"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// LiquidityReadSource augments the evaluated source with facts from the same
// snapshot. Saved-policy presence is independent of eligibility and exclusion.
type LiquidityReadSource struct {
	Result           domain.LiquiditySourceResult
	InstrumentID     *string
	InstrumentType   *string
	HasRule          *bool
	PolicyEditable   *bool
	CurrentValueBase *decimal.Decimal
	SearchText       string
	PolicyReason     string
}
type LiquidityReadResult struct {
	Overview domain.LiquidityOverview
	Sources  []LiquidityReadSource
}

func (s *Service) LiquidityRead(ctx context.Context, q LiquidityOverviewQuery) (LiquidityReadResult, error) {
	overview, snapshot, sources, err := s.prepareLiquidity(ctx, q, s.clock())
	if err != nil {
		return LiquidityReadResult{}, err
	}
	byKey := map[string]domain.LiquiditySource{}
	for _, v := range sources {
		byKey[v.Ref.Key()] = v
	}
	holdings := map[domain.HoldingID]domain.Holding{}
	for _, v := range snapshot.Portfolio.Holdings {
		holdings[v.ID] = v
	}
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for _, v := range snapshot.Portfolio.Instruments {
		instruments[v.ID] = v
	}
	out := LiquidityReadResult{Overview: overview, Sources: make([]LiquidityReadSource, 0, len(overview.Sources))}
	for _, r := range overview.Sources {
		source := byKey[r.SourceKey]
		row := LiquidityReadSource{Result: r, SearchText: r.DisplayName}
		if source.Ref.HoldingID != nil {
			if h, ok := holdings[*source.Ref.HoldingID]; ok {
				if i, ok := instruments[h.InstrumentID]; ok {
					id, kind := i.ID.String(), string(i.Type)
					row.InstrumentID = &id
					row.InstrumentType = &kind
					row.SearchText += " " + i.Name + " " + i.ShortName + " " + pointerText(i.Symbol)
				}
			}
		}
		policy, origin, _, e := domain.ResolveLiquidityPolicy(source)
		if e == nil {
			has := origin == domain.PolicyOriginExplicit || origin == domain.PolicyOriginContract
			editable := !source.Managed && !source.Archived && !source.Liability && !source.QuantityZero
			row.HasRule = &has
			row.PolicyEditable = &editable
			row.Result.PolicyOrigin = origin
			row.Result.Policy = &policy
		} else {
			row.Result.PolicyOrigin = origin
			row.PolicyReason = "policy facts unavailable"
			editable := false
			if source.Managed || source.Archived || source.Liability || source.QuantityZero {
				row.PolicyEditable = &editable
			}
		}
		row.CurrentValueBase = source.CurrentBaseValue
		out.Sources = append(out.Sources, row)
	}
	return out, nil
}

// SortLiquidityReadSources compares exact same-base-currency valuation and
// domain NetBase, never display-rounded money or binary floating point.
func SortLiquidityReadSources(rows []LiquidityReadSource, field, direction, horizon string) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		var cmp int
		var an, bn bool
		switch field {
		case "currentValueBase", "availableBase":
			av, bv := a.CurrentValueBase, b.CurrentValueBase
			if field == "availableBase" {
				av, bv = nil, nil
				for _, v := range a.Result.BucketResults {
					if v.HorizonOn == horizon {
						av = v.NetBase
					}
				}
				for _, v := range b.Result.BucketResults {
					if v.HorizonOn == horizon {
						bv = v.NetBase
					}
				}
			}
			an, bn = av == nil, bv == nil
			if !an && !bn {
				cmp = av.Cmp(*bv)
			}
		default:
			var av, bv string
			switch field {
			case "displayName":
				av, bv = normalizeReadName(a.Result.DisplayName), normalizeReadName(b.Result.DisplayName)
			case "instrumentType":
				if a.InstrumentType != nil {
					av = *a.InstrumentType
				}
				if b.InstrumentType != nil {
					bv = *b.InstrumentType
				}
			case "accessKind":
				if a.Result.Policy != nil {
					av = string(a.Result.Policy.AccessKind)
				}
				if b.Result.Policy != nil {
					bv = string(b.Result.Policy.AccessKind)
				}
			case "policyOrigin":
				av, bv = string(a.Result.PolicyOrigin), string(b.Result.PolicyOrigin)
			case "normalReceiptOn":
				if a.Result.NormalRoute != nil && a.Result.NormalRoute.ReceiptOn != nil {
					av = *a.Result.NormalRoute.ReceiptOn
				}
				if b.Result.NormalRoute != nil && b.Result.NormalRoute.ReceiptOn != nil {
					bv = *b.Result.NormalRoute.ReceiptOn
				}
			default:
				av, bv = a.Result.SourceKey, b.Result.SourceKey
			}
			an, bn = av == "", bv == ""
			cmp = strings.Compare(av, bv)
		}
		if an != bn {
			return !an
		}
		if cmp != 0 {
			if direction == "desc" {
				return cmp > 0
			}
			return cmp < 0
		}
		return a.Result.SourceKey < b.Result.SourceKey
	})
}
func normalizeReadName(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
