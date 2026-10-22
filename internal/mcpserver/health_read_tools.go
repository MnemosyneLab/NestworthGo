package mcpserver

import (
	"context"
	"encoding/json"
)

type HealthReadInput struct {
	ResponseMode string `json:"responseMode,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

// Health/repair arrays can grow with retained history, even for a modest number
// of holdings. Keep the complete issue/plan evidence independently pageable.
func sectionedReadCapture(kind string, query, value any, fields ...string) (readCapture, error) {
	summary, err := readSummary(value)
	if err != nil {
		return readCapture{}, err
	}
	c := readCapture{Kind: kind, Query: query, Summary: summary, Sections: map[string][]json.RawMessage{}}
	for _, field := range fields {
		rows, ok := summary[field].([]any)
		if !ok {
			rows = []any{}
		}
		c.Sections[field], err = readRows(rows)
		if err != nil {
			return c, err
		}
		summary[field+"Count"] = len(rows)
		delete(summary, field)
	}
	return c, nil
}
func (s *Service) pagedHealth(ctx context.Context, g uint64, in HealthReadInput, kind string, read func(context.Context) (any, error), fields ...string) (ReadEnvelope, error) {
	return s.buildRead(ctx, g, in.Limit, func(ctx context.Context) (readCapture, error) {
		value, err := read(ctx)
		if err != nil {
			return readCapture{}, err
		}
		return sectionedReadCapture(kind, in, value, fields...)
	})
}
