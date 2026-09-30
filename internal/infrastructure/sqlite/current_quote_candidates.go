package sqlite

import "github.com/waltwang/nestworth-go/internal/domain"

// Keep separate daily, realtime and converted candidates. Cutting everything
// down to one timestamp per source discards data needed to resolve overlays.
func currentQuoteCandidateColumns(kind string, modern bool) string {
	if !modern {
		return `'latest' AS selection_kind, '' AS selection_group, q.quoted_at AS selection_at, '' AS selection_tie_at, 0 AS selection_revision`
	}
	dailyKind := "close"
	if kind == "fx" {
		dailyKind = "daily_reference"
	}
	daily := `q.observation_kind = '` + dailyKind + `' AND COALESCE(q.effective_date, '') <> ''`
	raw := `''`
	if kind == "instrument" {
		raw = metalRawQuoteKeySQL("q")
	}
	converted := `q.source_kind = 'provider' AND (` + raw + `) <> '' AND NOT (` + daily + `)`
	return `CASE WHEN ` + daily + ` THEN 'daily' WHEN ` + converted + ` THEN 'converted' ELSE 'latest' END AS selection_kind,
		CASE WHEN ` + converted + ` THEN ` + raw + ` ELSE '' END AS selection_group,
		CASE WHEN ` + daily + ` THEN q.effective_date WHEN ` + converted + ` THEN '' ELSE q.quoted_at END AS selection_at,
		CASE WHEN ` + daily + ` THEN q.quoted_at ELSE '' END AS selection_tie_at,
		CASE WHEN ` + converted + ` THEN COALESCE(q.revision, 1) ELSE 0 END AS selection_revision`
}

// CASE protects legacy malformed or empty conversion JSON from json_extract.
func metalRawQuoteKeySQL(alias string) string {
	return `CASE WHEN json_valid(` + alias + `.conversion_json) THEN CASE WHEN json_extract(` + alias + `.conversion_json, '$.policy') = '` + domain.MetalConversionPolicy + `' THEN COALESCE(json_extract(` + alias + `.conversion_json, '$.rawQuotedAt'), '') ELSE '' END ELSE '' END`
}

const currentQuoteCandidateLaterSQL = `newer.selection_at > q.selection_at
	OR (newer.selection_at = q.selection_at AND newer.selection_tie_at > q.selection_tie_at)
	OR (newer.selection_at = q.selection_at AND newer.selection_tie_at = q.selection_tie_at AND newer.selection_revision > q.selection_revision)
	OR (newer.selection_at = q.selection_at AND newer.selection_tie_at = q.selection_tie_at AND newer.selection_revision = q.selection_revision AND newer.created_at > q.created_at)
	OR (newer.selection_at = q.selection_at AND newer.selection_tie_at = q.selection_tie_at AND newer.selection_revision = q.selection_revision AND newer.created_at = q.created_at AND newer.id > q.id)`
