package mcpserver

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const toolGroupsMeta = "nestworth.toolGroups"

// These are discovery categories, never permission/scoping tokens. Every call
// still goes through the unchanged mode-specific registered SDK allowlist.
func discoveryGroup(name string) string {
	switch name {
	case "get_read_page", "get_read_item", "get_operation", "commit_plan":
		return "shared"
	case "get_context", "get_catalog":
		return "context"
	case "get_financial_context", "get_financial_context_item", "compare_financial_context", "compare_financial_attribution":
		return "summary"
	case "get_overview", "get_account_valuations", "get_account_snapshot", "get_historical_overview", "get_historical_overview_series", "analyze_period", "list_contributions", "get_contribution_item", "get_return_day", "get_asset_driver_detail", "list_activities", "get_activity":
		return "analysis"
	case "preview_change", "preview_batch", "preview_correction", "preview_reconciliation":
		return "ledger"
	}
	switch {
	case strings.Contains(name, "investment_review"):
		return "review"
	case strings.Contains(name, "reconciliation_session") || name == "get_reconciliation_item":
		return "reconciliation"
	case strings.Contains(name, "data_health") || strings.Contains(name, "data_repair"):
		return "health"
	case strings.Contains(name, "market_data") || name == "set_fx_source" || name == "search_market_instruments":
		return "market"
	case strings.Contains(name, "product") || strings.Contains(name, "liquidity") || name == "get_upcoming_actions":
		return "products"
	case strings.Contains(name, "member") || strings.Contains(name, "institution") || strings.Contains(name, "group") || strings.Contains(name, "account") || strings.Contains(name, "instrument") || strings.Contains(name, "holding"):
		return "directory"
	}
	return ""
}

type toolGroupCursor struct {
	Groups string `json:"groups"`
	Offset int    `json:"offset"`
}

func taskToolDiscovery(set *toolSet) mcp.Middleware {
	// Build immutable metadata once, not per page or business call.
	all := []*mcp.Tool{}
	for _, d := range set.Definitions() {
		all = append(all, sdkTool(d))
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic(err)
	}
	invalid := func() error {
		return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "invalid task tool discovery metadata or cursor; restart tools/list with matching nestworth.toolGroups"}
	}
	sign := func(payload []byte) []byte {
		mac := hmac.New(sha256.New, key[:])
		mac.Write(payload)
		return mac.Sum(nil)
	}
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			if method != "tools/list" {
				return next(ctx, method, request)
			}
			params, ok := request.GetParams().(*mcp.ListToolsParams)
			if !ok || params == nil {
				return next(ctx, method, request)
			}
			raw, present := params.Meta[toolGroupsMeta]
			if !present {
				return next(ctx, method, request)
			}
			known := map[string]bool{"context": true, "summary": true, "analysis": true, "directory": true, "ledger": true, "market": true, "products": true, "review": true, "reconciliation": true, "health": true}
			values, ok := raw.([]any)
			if !ok || len(values) < 1 || len(values) > 10 {
				return nil, invalid()
			}
			selected := map[string]bool{}
			for _, v := range values {
				g, ok := v.(string)
				if !ok || !known[g] {
					return nil, invalid()
				}
				selected[g] = true
			}
			groups := []string{}
			for g := range selected {
				groups = append(groups, g)
			}
			sort.Strings(groups)
			groupKey := strings.Join(groups, ",")
			tools := []*mcp.Tool{}
			for _, tool := range all {
				g := discoveryGroup(tool.Name)
				if g == "shared" || selected[g] {
					tools = append(tools, tool)
				}
			}
			offset := 0
			if params.Cursor != "" {
				if len(params.Cursor) > 2048 {
					return nil, invalid()
				}
				parts := strings.Split(params.Cursor, ".")
				if len(parts) != 2 {
					return nil, invalid()
				}
				payload, e := base64.RawURLEncoding.DecodeString(parts[0])
				if e != nil {
					return nil, invalid()
				}
				signature, e := base64.RawURLEncoding.DecodeString(parts[1])
				if e != nil || !hmac.Equal(signature, sign(payload)) {
					return nil, invalid()
				}
				var cursor toolGroupCursor
				if jsoncodec.Unmarshal(payload, &cursor) != nil || cursor.Groups != groupKey || cursor.Offset < 1 || cursor.Offset >= len(tools) {
					return nil, invalid()
				}
				offset = cursor.Offset
			}
			end := min(offset+16, len(tools))
			result := &mcp.ListToolsResult{Tools: tools[offset:end], Meta: mcp.Meta{toolGroupsMeta: groups}}
			if end < len(tools) {
				payload, _ := jsoncodec.Marshal(toolGroupCursor{Groups: groupKey, Offset: end})
				result.NextCursor = base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sign(payload))
			}
			return result, nil
		}
	}
}
