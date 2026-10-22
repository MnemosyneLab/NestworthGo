package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sort"
	"strings"
	"testing"
)

// Reassemble all frozen sections only as a GUI parity oracle. These helpers
// exercise the public page/item contract; production never returns this shape.
func sectionEvidence(t *testing.T, c *mcp.ClientSession, name string, args any) map[string]any {
	cap := scaleCapture(t, c, name, args)
	out := cap["summary"].(map[string]any)
	for section := range cap["sections"].(map[string]any) {
		rows := scaleSection(t, c, cap, section)
		target, key := out, section
		if name == "analyze_period" {
			parts := strings.SplitN(section, "/", 2)
			target = out[parts[0]].(map[string]any)
			key = parts[1]
		}
		target[key] = rows
		delete(target, key+"Count")
		if name == "preview_data_repair" && len(rows) == 0 {
			delete(target, key)
		}
	}
	if name == "get_liquidity_overview" {
		for _, key := range []string{"totalsScope", "sourcesScope", "unresolvedReservationsScope", "totalSourceCount", "matchedSourceCount", "filteredOutSourceCount", "unresolvedReservationCount", "sortBasis", "sortHorizonOn"} {
			delete(out, key)
		}
		for _, row := range out["sources"].([]any) {
			for _, key := range []string{"instrumentId", "instrumentType", "productKind", "hasRule", "policyEditable", "policyReason"} {
				delete(row.(map[string]any), key)
			}
		}
	}
	if name == "get_upcoming_actions" {
		for _, key := range []string{"countsScope", "itemsScope", "totalItemCount", "matchedItemCount", "matchedDataCheckCount", "dataChecksFilterBasis"} {
			delete(out, key)
		}
	}
	return out
}
func productEvidence(t *testing.T, c *mcp.ClientSession, args any) map[string]any {
	cap := scaleCapture(t, c, "get_product", args)
	detail := cap["summary"].(map[string]any)["detail"].(map[string]any)
	id := detail["product"].(map[string]any)["id"].(string)
	detail["reservations"] = scaleSection(t, c, cap, "reservations/"+id)
	delete(detail, "reservationCount")
	return detail
}
func productListEvidence(t *testing.T, c *mcp.ClientSession, args any) []any {
	cap := scaleCapture(t, c, "list_products", args)
	out := []any{}
	for _, row := range scaleSection(t, c, cap, "products") {
		id := row.(map[string]any)["id"].(string)
		item := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "products", ID: id})
		detail := item["summary"].(map[string]any)
		detail["reservations"] = scaleSection(t, c, item, "reservations/"+id)
		delete(detail, "reservationCount")
		out = append(out, detail)
	}
	return out
}
func repairJobEvidence(t *testing.T, c *mcp.ClientSession, args any) map[string]any {
	cap := scaleCapture(t, c, "get_data_repair_job", args)
	out := cap["summary"].(map[string]any)
	health := out["health"].(map[string]any)
	health["issues"] = scaleSection(t, c, cap, "issues")
	delete(health, "issuesCount")
	job := out["job"].(map[string]any)
	for _, field := range []string{"items", "blockers", "prerequisites"} {
		job[field] = scaleSection(t, c, cap, "job_"+field)
		delete(job, field+"Count")
	}
	return out
}

// The bounded contract's global order differs from GUI display order. Compare
// complete identity sets without weakening any per-row field assertions.
func canonicalEvidence(v any) any {
	switch x := v.(type) {
	case map[string]any:
		// GUI excludes policy evaluation for excluded sources. The read model
		// additionally exposes saved/effective policy for management; filter and
		// policy round-trip tests assert those fields independently.
		if x["excluded"] == true && x["sourceKey"] != nil {
			x["policy"] = nil
			x["policyOrigin"] = ""
		}
		for k, v := range x {
			x[k] = canonicalEvidence(v)
		}
	case []any:
		for i, v := range x {
			x[i] = canonicalEvidence(v)
		}
		key := func(v any) string {
			m, ok := v.(map[string]any)
			if !ok {
				return ""
			}
			if p, ok := m["product"].(map[string]any); ok {
				return p["id"].(string)
			}
			if id, ok := m["sourceKey"].(string); ok {
				return id
			}
			return ""
		}
		if len(x) > 0 && key(x[0]) != "" {
			sort.Slice(x, func(i, j int) bool { return key(x[i]) < key(x[j]) })
		}
	}
	return v
}
