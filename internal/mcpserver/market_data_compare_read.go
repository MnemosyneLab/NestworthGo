package mcpserver

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Detailed rows remain stored exactly once. Summary rows and item sections are
// bounded projections over the same immutable capture, not new captures.
func comparisonSummary(e readCapture) bool {
	if e.Kind != "market_import_comparison" {
		return false
	}
	q, _ := e.Query.(map[string]any)
	return q["view"] != "full"
}
func readSectionRows(e readCapture, section string) ([]json.RawMessage, bool, error) {
	if rows, ok := e.Sections[section]; ok {
		return rows, true, nil
	}
	if e.Kind != "market_import_comparison" {
		return nil, false, nil
	}
	if section == "result_details" {
		return e.Sections["results"], true, nil
	}
	parts := strings.SplitN(section, "/", 4)
	if len(parts) != 3 || parts[0] != "comparison_item" {
		return nil, false, nil
	}
	index, err := strconv.Atoi(parts[1])
	if err != nil || strconv.Itoa(index) != parts[1] || index < 0 || index >= len(e.Sections["results"]) {
		return nil, false, nil
	}
	var row application.MarketComparisonRow
	if err = decodeReadValue(e.Sections["results"][index], &row); err != nil {
		return nil, false, err
	}
	var match application.MarketComparisonMatch
	var found bool
	for _, raw := range e.Sections["matches"] {
		var candidate application.MarketComparisonMatch
		if err = decodeReadValue(raw, &candidate); err != nil {
			return nil, false, err
		}
		if candidate.ID == row.MatchID {
			match = candidate
			found = true
			break
		}
	}
	out := []json.RawMessage{}
	if !found {
		return out, true, nil
	}
	switch parts[2] {
	case "matches":
		raw, err := jsoncodec.Marshal(match)
		if err != nil {
			return nil, false, err
		}
		var metadata map[string]json.RawMessage
		if err = decodeReadValue(raw, &metadata); err != nil {
			return nil, false, err
		}
		delete(metadata, "relations")
		b, err := jsoncodec.Marshal(map[string]any{"match": metadata, "relationsSection": "comparison_item/" + parts[1] + "/relations", "relationsDelivery": "see_relations_page"})
		return []json.RawMessage{b}, true, err
	case "relations":
		rows, err := readRows(match.Relations)
		return rows, true, err
	case "targets":
		for _, raw := range e.Sections["targets"] {
			var key struct {
				Key string `json:"key"`
			}
			if err = decodeReadValue(raw, &key); err != nil {
				return nil, false, err
			}
			if key.Key == match.TargetKey {
				out = append(out, raw)
			}
		}
		return out, true, nil
	case "evidence", "audits":
		ids := map[string]bool{}
		for _, relation := range match.Relations {
			ids[relation.EvidenceID] = true
		}
		audits := map[string]bool{}
		for _, raw := range e.Sections["evidence"] {
			var ev struct {
				ID       string   `json:"id"`
				AuditIDs []string `json:"auditIds"`
			}
			if err = decodeReadValue(raw, &ev); err != nil {
				return nil, false, err
			}
			if !ids[ev.ID] {
				continue
			}
			if parts[2] == "evidence" {
				out = append(out, raw)
			}
			for _, id := range ev.AuditIDs {
				audits[id] = true
			}
		}
		if parts[2] == "audits" {
			for _, raw := range e.Sections["audits"] {
				var a struct {
					ID string `json:"recordId"`
				}
				if err = decodeReadValue(raw, &a); err != nil {
					return nil, false, err
				}
				if audits[a.ID] {
					out = append(out, raw)
				}
			}
		}
		return out, true, nil
	default:
		return nil, false, nil
	}
}
func comparisonCompactRows(rows []json.RawMessage) ([]json.RawMessage, error) {
	type summaryRow struct {
		Ordinal        int    `json:"ordinal"`
		ClientItemID   string `json:"clientItemId"`
		Status         string `json:"status"`
		Reason         string `json:"reason,omitempty"`
		DuplicateGroup string `json:"duplicateGroup,omitempty"`
		BatchConflict  bool   `json:"batchConflict,omitempty"`
	}
	out := make([]json.RawMessage, 0, len(rows))
	for _, raw := range rows {
		var row application.MarketComparisonRow
		if err := decodeReadValue(raw, &row); err != nil {
			return nil, err
		}
		b, err := jsoncodec.Marshal(summaryRow{row.Ordinal, row.ClientItemID, row.Status, row.Reason, row.DuplicateGroup, row.BatchConflict})
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}
func (c *boundedReadCache) comparisonItem(id string, e readCapture, in ReadItemInput) (ReadEnvelope, error) {
	if in.Section != "results" {
		return ReadEnvelope{}, fail("validation", "comparison item section must be results; id is clientItemId")
	}
	var row application.MarketComparisonRow
	index := -1
	for j, raw := range e.Sections["results"] {
		var candidate application.MarketComparisonRow
		if err := decodeReadValue(raw, &candidate); err != nil {
			return ReadEnvelope{}, err
		}
		if candidate.ClientItemID == in.ID {
			row = candidate
			index = j
			break
		}
	}
	if index < 0 {
		return ReadEnvelope{}, fail("not_found", "client item not found in capture")
	}
	r := c.base(id, e)
	r.Section = "results"
	r.Summary = map[string]any{"result": row, "scope": "one_client_item", "evidenceDelivery": "see_child_pages", "globalEvidenceDelivered": false}
	r.Sections = map[string]ReadSection{}
	names := []string{}
	sections := map[string][]json.RawMessage{}
	for _, kind := range []string{"targets", "matches", "relations", "evidence", "audits"} {
		name := fmt.Sprintf("comparison_item/%d/%s", index, kind)
		rows, ok, err := readSectionRows(e, name)
		if err != nil {
			return r, err
		}
		if !ok {
			return r, fail("not_found", "item evidence section unavailable")
		}
		names = append(names, name)
		sections[name] = rows
		r.Sections[name] = ReadSection{Items: []json.RawMessage{}, Page: c.pageInfoSize(id, name, e, len(rows), 0, 0)}
	}
	if ok, err := readFits(r); err != nil {
		return r, err
	} else if !ok {
		return r, fail("too_large", "single_item comparison input exceeds read wire budget")
	}
	for _, name := range names {
		rows := sections[name]
		for n := 0; n < min(25, len(rows)); n++ {
			old := r.Sections[name]
			r.Sections[name] = ReadSection{Items: rows[:n+1], Page: c.pageInfoSize(id, name, e, len(rows), 0, n+1)}
			ok, err := readFits(r)
			if err != nil {
				return r, err
			}
			if !ok {
				r.Sections[name] = old
				break
			}
		}
	}
	return r, nil
}
