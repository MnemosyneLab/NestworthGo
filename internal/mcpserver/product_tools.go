package mcpserver

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type ProductListInput struct {
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
	readTool(server, "list_products", "Read managed deposit/locked contracts, filtered compact rows and same-capture get_read_item details/reservations. Frozen mcp-read/1; follow relevant cursors. Currency, cost/value, revision and policies remain authoritative. GUI actions do not grant MCP permission. Ordinary NAV holdings use list_holdings.", func(ctx context.Context, in ProductListInput) (any, error) {
		if in.AccountID != nil && strings.TrimSpace(*in.AccountID) == "" {
			return nil, fail("validation", "accountId must be an existing account UUID when supplied")
		}
		return s.pagedProducts(ctx, generation, in, nil)
	})
	readTool(server, "get_product", "Read one managed product with full contract/terms/policy/revision and separately paged reservations. Frozen mcp-read/1. due_unconfirmed does not record cash or forecast interest as net worth. GUI actions do not grant permission; writes require dedicated preview/commit pairs with ledger_write.", func(ctx context.Context, in ProductGetInput) (any, error) {
		id, err := domain.ParseProductContractID(in.ID)
		if err != nil {
			return nil, err
		}
		return s.pagedProducts(ctx, generation, ProductListInput{Limit: in.Limit}, &id)
	})
	readTool(server, "list_product_operations", "Page live managed-product operations by productId; default 25/max 100, byte-aware. Follow page.nextCursor with get_read_page; refresh after a changed latestRecordedOperationId. Includes reversals and current undo availability, which is not a plan or permission. get_activity shows ledger effects; get_operation uses a different MCP receipt UUID.", func(ctx context.Context, in liquidity.ListOperationsRequest) (any, error) {
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
	readTool(server, "get_liquidity_overview", "Read current household liquidity routes, exact values/FX, forecasts, fees and reservations. fullAvailable/knownAvailableSubtotal precede reservations; fullUnreserved/knownUnreservedSubtotal follow them. Unknown inputs keep totals incomplete; forecasts are not cash/net worth. Frozen sources and unresolvedReservations use independent cursors. Filters affect detail only; household totals stay unchanged. hasRule means saved rule; availableBase sorting requires horizonOn. No provider refresh or financial writes.", func(ctx context.Context, in LiquidityOverviewInput) (any, error) {
		return s.pagedLiquidity(ctx, generation, in)
	})
}

func isProductReadTool(name string) bool {
	switch name {
	case "list_products", "get_product", "list_product_operations", "get_liquidity_overview":
		return true
	default:
		return false
	}
}

func isProductTool(name string) bool {
	switch name {
	case "preview_save_liquidity_policy", "commit_save_liquidity_policy", "preview_reset_liquidity_policy", "commit_reset_liquidity_policy", "preview_product_operation", "commit_product_operation", "preview_product_terms", "commit_product_terms", "preview_product_valuation", "commit_product_valuation":
		return true
	default:
		return isProductReadTool(name)
	}
}
