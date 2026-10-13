package mcpserver

import (
	"testing"
	"time"
)

func TestAgentReadTimestampTiesUseRecordIdentity(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	for _, direction := range []string{"asc", "desc"} {
		rows := []AgentDataRecordDTO{{RecordID: "b", Date: "2026-09-30"}, {RecordID: "a", Date: "2026-09-01"}, {RecordID: "c", Date: ""}}
		sortAgentReadRecords(rows, map[string]time.Time{"a": now, "b": now, "c": now}, ReadSort{Field: "createdAt", Direction: direction})
		for i, id := range []string{"a", "b", "c"} {
			if rows[i].RecordID != id {
				t.Fatalf("createdAt %s introduced observation-date tie key: %+v", direction, rows)
			}
		}
	}
}
