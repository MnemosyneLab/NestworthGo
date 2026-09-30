package sqlite

import "strings"

// Historical migration fixtures start from the embedded current schema and
// then remove the Agent quote additions to represent their older versions.
func legacyV14QuoteSourceSchema(schema string) string {
	start := strings.Index(schema, "CREATE TABLE agent_quote_batches (")
	end := strings.Index(schema, "PRAGMA user_version = 15;")
	if start >= 0 && end > start {
		schema = schema[:start] + schema[end:]
	}
	schema = strings.ReplaceAll(schema, "CHECK(source_kind IN ('manual','provider','agent'))", "CHECK(source_kind IN ('manual','provider'))")
	schema = strings.ReplaceAll(schema, "CHECK(quote_source IN ('manual','provider','agent'))", "CHECK(quote_source IN ('manual','provider'))")
	schema = strings.ReplaceAll(schema, "CHECK(quote_source <> 'provider' OR (provider_key IS NOT NULL AND provider_symbol IS NOT NULL))", "CHECK(quote_source = 'manual' OR (provider_key IS NOT NULL AND provider_symbol IS NOT NULL))")
	return schema
}
