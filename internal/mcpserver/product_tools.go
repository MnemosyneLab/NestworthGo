package mcpserver

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type ProductReadInput struct {
	ID            *string            `json:"id,omitempty" jsonschema:"Committed product UUID; omit for a filtered list"`
	View          string             `json:"view,omitempty" jsonschema:"summary or detail; defaults to detail with id, summary otherwise. detail requires id"`
	Filter        *ProductReadFilter `json:"filter,omitempty"`
	Sort          *ReadSort          `json:"sort,omitempty"`
	Limit         int                `json:"limit,omitempty"`
	AccountID     *string            `json:"accountId,omitempty"`
	IncludeClosed bool               `json:"includeClosed,omitempty"`
}

type LiquidityOverviewInput struct {
	Filter                 *LiquidityReadFilter `json:"filter,omitempty"`
	Sort                   *ReadSort            `json:"sort,omitempty"`
	Limit                  int                  `json:"limit,omitempty"`
	CustomHorizonOn        *string              `json:"customHorizonOn,omitempty"`
	IncludeEarlyWithdrawal bool                 `json:"includeEarlyWithdrawal,omitempty"`
}

// This extension intentionally exposes only the GUI's read models. Product
// writes need dedicated stored plans and atomic business receipts; directory
// permission and the generic trade/quote tools must never substitute for them.
func (s *Service) productTools(server *toolSet) {
	products := liquidity.NewService(s.app)
	generation := s.contextGeneration
	readTool(server, "get_products", "Read managed deposit and locked contracts. Omit id for compact filtered rows; supply a committed product id for full terms, policy, revision and paged reservations (view=summary selects a compact row). List filters and sorting require omitting id. Use get_read_item for details from a list capture. due_unconfirmed does not record cash or forecast interest as net worth. Ordinary NAV holdings use list_holdings.", func(ctx context.Context, in ProductReadInput) (any, error) {
		if in.AccountID != nil && strings.TrimSpace(*in.AccountID) == "" {
			return nil, fail("validation", "accountId must be an existing account UUID when supplied")
		}
		var id *domain.ProductContractID
		if in.ID != nil {
			if in.AccountID != nil || in.IncludeClosed || in.Filter != nil || in.Sort != nil {
				return nil, fail("validation", "id cannot be combined with product list filters or sorting")
			}
			parsed, err := domain.ParseProductContractID(*in.ID)
			if err != nil {
				return nil, err
			}
			id = &parsed
		}
		if in.View == "" {
			in.View = "summary"
			if id != nil {
				in.View = "detail"
			}
		}
		if in.View != "summary" && (in.View != "detail" || id == nil) {
			return nil, fail("validation", "view must be summary, or detail with a product id")
		}
		return s.pagedProducts(ctx, generation, in, id)
	})
	readTool(server, "list_product_operations", "Page live managed-product operations by productId, including reversals and current undo availability (which is not a plan or permission). Follow page.nextCursor with get_read_page and refresh after latestRecordedOperationId changes. get_activity shows ledger effects; get_operation uses a different MCP receipt UUID.", func(ctx context.Context, in liquidity.ListOperationsRequest) (any, error) {
		if in.Limit < 0 || in.Limit > 100 {
			return nil, fail("validation", "limit must be 1 to 100, or omitted/zero for 25")
		}
		if len(in.Cursor) > 128 {
			return nil, fail("validation", "cursor must come from this product's operation page")
		}
		size := in.Limit
		if size == 0 {
			size = 25
		}
		return fitLiveReadPage(ctx, s.app, size, func(ctx context.Context, size int) (liquidity.ProductOperationPageDTO, error) {
			query := in
			query.Limit = size
			return products.ListOperations(ctx, query)
		})
	})
	readTool(server, "get_liquidity_overview", "Current household liquidity routes with exact values and FX, forecasts, fees and reservations. fullAvailable and knownAvailableSubtotal come before reservations; fullUnreserved and knownUnreservedSubtotal after them. Unknown inputs keep totals incomplete; forecasts are not cash or net worth. sources and unresolvedReservations are separate sections; filters narrow detail only and household totals stay unchanged. hasRule means a saved rule; sorting by availableBase needs horizonOn. No provider refresh or writes.", func(ctx context.Context, in LiquidityOverviewInput) (any, error) {
		return s.pagedLiquidity(ctx, generation, in)
	})
}

func isProductReadTool(name string) bool {
	switch name {
	case "get_products", "list_product_operations", "get_liquidity_overview":
		return true
	default:
		return false
	}
}

func isProductTool(name string) bool {
	switch name {
	case "preview_save_liquidity_policy", "preview_reset_liquidity_policy", "preview_product_operation", "preview_product_terms", "preview_product_valuation":
		return true
	default:
		return isProductReadTool(name)
	}
}
