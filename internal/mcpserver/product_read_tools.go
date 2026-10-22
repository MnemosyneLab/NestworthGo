package mcpserver

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type ProductReadFilter struct {
	ProductKinds []string `json:"productKinds,omitempty"`
	States       []string `json:"states,omitempty"`
	Query        string   `json:"query,omitempty"`
}
type ProductGetInput struct {
	ID    string `json:"id"`
	Limit int    `json:"limit,omitempty"`
}
type productReadSummary struct {
	ID               string `json:"id"`
	AccountID        string `json:"accountId"`
	HoldingID        string `json:"holdingId"`
	InstrumentID     string `json:"instrumentId"`
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	ShortName        string `json:"shortName"`
	Symbol           string `json:"symbol"`
	Currency         string `json:"currency"`
	Principal        any    `json:"principal"`
	CurrentValue     any    `json:"currentValue"`
	CurrentCostBasis any    `json:"currentCostBasis"`
	State            string `json:"state"`
	DisplayState     string `json:"displayState"`
	Revision         int    `json:"revision"`
	ReservationCount int    `json:"reservationCount"`
}

func compareReadText(a, b string, direction string) int {
	if a == "" && b != "" {
		return 1
	}
	if b == "" && a != "" {
		return -1
	}
	v := strings.Compare(a, b)
	if direction == "desc" {
		v = -v
	}
	return v
}
func normalizedReadName(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func (s *Service) pagedProducts(ctx context.Context, g uint64, in ProductListInput, id *domain.ProductContractID) (ReadEnvelope, error) {
	if id != nil {
		normalized, err := domain.ParseProductContractID(id.String())
		if err != nil {
			return ReadEnvelope{}, err
		}
		id = &normalized
	}
	f := ProductReadFilter{}
	if in.Filter != nil {
		f = *in.Filter
	}
	if utf8.RuneCountInString(f.Query) > 200 {
		return ReadEnvelope{}, fail("validation", "query supports at most 200 characters")
	}
	if err := validateReadValues(f.ProductKinds, []string{"term_deposit", "locked_product"}, false); err != nil {
		return ReadEnvelope{}, err
	}
	if err := validateReadValues(f.States, []string{"open", "settled", "cancelled"}, false); err != nil {
		return ReadEnvelope{}, err
	}
	order := ReadSort{}
	if in.Sort != nil {
		order = *in.Sort
	}
	if err := validateReadSort(&order, []string{"startOn", "maturityOn", "displayName", "state"}, "startOn", "desc"); err != nil {
		return ReadEnvelope{}, err
	}
	if order.HorizonOn != "" {
		return ReadEnvelope{}, fail("validation", "product sort does not accept horizonOn")
	}
	return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
		adapter := liquidity.NewService(s.app)
		var products []liquidity.ProductDetailDTO
		if id != nil {
			v, err := adapter.Product(ctx, id.String())
			if err != nil {
				return readCapture{}, err
			}
			products = []liquidity.ProductDetailDTO{v}
		} else {
			v, err := adapter.ListProducts(ctx, liquidity.ListProductsRequest{AccountID: in.AccountID, IncludeClosed: in.IncludeClosed})
			if err != nil {
				return readCapture{}, err
			}
			products = v
		}
		total := len(products)
		selected := make([]liquidity.ProductDetailDTO, 0, len(products))
		for _, v := range products {
			p := v.Product
			if hasString(f.ProductKinds, p.Kind) && hasString(f.States, p.State) && matches(f.Query, p.Name, p.ShortName, p.Symbol) {
				selected = append(selected, v)
			}
		}
		sort.SliceStable(selected, func(i, j int) bool {
			a, b := selected[i].Product, selected[j].Product
			av, bv := a.StartOn, b.StartOn
			switch order.Field {
			case "maturityOn":
				av, bv = "", ""
				if a.MaturityOn != nil {
					av = *a.MaturityOn
				}
				if b.MaturityOn != nil {
					bv = *b.MaturityOn
				}
			case "displayName":
				av, bv = normalizedReadName(a.Name), normalizedReadName(b.Name)
				if a.ShortName != "" {
					av = normalizedReadName(a.ShortName)
				}
				if b.ShortName != "" {
					bv = normalizedReadName(b.ShortName)
				}
			case "state":
				av, bv = a.State, b.State
			}
			cmp := compareReadText(av, bv, order.Direction)
			if cmp == 0 {
				return a.ID < b.ID
			}
			return cmp < 0
		})
		rows := make([]productReadSummary, 0, len(selected))
		details := map[string]any{}
		sections := map[string][]json.RawMessage{}
		for _, v := range selected {
			p := v.Product
			rows = append(rows, productReadSummary{p.ID, p.AccountID, p.HoldingID, p.InstrumentID, p.Kind, p.Name, p.ShortName, p.Symbol, p.Currency, p.Principal, p.CurrentValue, p.CurrentCostBasis, p.State, p.DisplayState, p.Revision, len(v.Reservations)})
			sort.SliceStable(v.Reservations, func(i, j int) bool {
				a, b := v.Reservations[i], v.Reservations[j]
				at, _ := time.Parse(time.RFC3339Nano, a.CreatedAt)
				bt, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
				if !at.Equal(bt) {
					return at.Before(bt)
				}
				return a.ID < b.ID
			})
			raw, err := readRows(v.Reservations)
			if err != nil {
				return readCapture{}, err
			}
			sections["reservations/"+p.ID] = raw
			details[p.ID] = map[string]any{"product": p, "permittedActions": v.PermittedActions, "disabledReasons": v.DisabledReasons, "reservationCount": len(v.Reservations)}
		}
		raw, err := readRows(rows)
		if err != nil {
			return readCapture{}, err
		}
		sections["products"] = raw
		summary := map[string]any{"totalProductCount": total, "matchedProductCount": len(rows), "productsScope": "filtered", "details": "get_read_item returns full detail from this capture; reservations are separate sections"}
		if id != nil {
			summary["detail"] = details[id.String()]
		}
		initial := []string{"products"}
		if id != nil {
			initial = append(initial, "reservations/"+id.String())
		}
		return readCapture{InitialSections: initial, Kind: "products", Query: map[string]any{"accountId": in.AccountID, "includeClosed": in.IncludeClosed, "filter": f, "sort": order, "id": id}, Summary: summary, Sections: sections, Details: details}, nil
	})
}
