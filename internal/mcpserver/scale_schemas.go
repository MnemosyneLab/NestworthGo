package mcpserver

import (
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func scaleReadSchema[I any](name string) *jsonschema.Schema {
	fields := map[string][]string{
		"get_liquidity_overview": {"sourceKey", "displayName", "instrumentType", "accessKind", "policyOrigin", "normalReceiptOn", "currentValueBase", "availableBase"}, "list_products": {"startOn", "maturityOn", "displayName", "state"}, "get_product": {}, "list_instruments": {"displayName", "name", "type", "updatedAt", "id"}, "get_upcoming_actions": {"priority", "sortOn", "displayName"}, "list_agent_market_data": {"createdAt", "observationDate", "operation"}, "get_market_data_batch": {"targetKey", "displayName"}, "get_read_page": {}, "get_read_item": {}}
	sortFields, ok := fields[name]
	if !ok {
		return nil
	}
	schema, err := jsonschema.For[I](nil)
	if err != nil {
		panic(err)
	}
	if name == "get_product" {
		minimum := 1
		schema.Properties["id"].MinLength = &minimum
	}
	if p := schema.Properties["responseMode"]; p != nil {
		p.Enum = []any{"paged"}
	}
	if p := schema.Properties["limit"]; p != nil {
		min := float64(0)
		p.Minimum = &min
		max := float64(100)
		p.Maximum = &max
	}
	if p := schema.Properties["sort"]; p != nil {
		if v := p.Properties["field"]; v != nil {
			setSchemaEnum(v, sortFields)
		}
		if v := p.Properties["direction"]; v != nil {
			setSchemaEnum(v, []string{"asc", "desc"})
		}
	}
	pagingOptions := []string{"filter", "sort", "limit"}
	if name == "list_agent_market_data" {
		pagingOptions = []string{"filter", "sort"}
	}
	if schema.Properties["responseMode"] != nil {
		conditions := []*jsonschema.Schema{}
		for _, field := range pagingOptions {
			if schema.Properties[field] != nil {
				conditions = append(conditions, &jsonschema.Schema{Required: []string{field}})
			}
		}
		if len(conditions) > 0 {
			schema.If = &jsonschema.Schema{AnyOf: conditions}
			schema.Then = &jsonschema.Schema{Required: []string{"responseMode"}}
		}
	}
	walkScaleSchema(schema)
	if name == "list_instruments" {
		schema.Properties["query"].MaxLength = nil
	}
	filter := schema.Properties["filter"]
	if filter != nil {
		enums := map[string][]string{}
		switch name {
		case "get_liquidity_overview":
			types := []string{}
			for _, t := range domain.AllInstrumentTypes() {
				types = append(types, string(t))
			}
			enums = map[string][]string{"sourceKinds": {"account_value", "account_cash", "holding"}, "instrumentTypes": types, "accessKinds": {"on_request", "on_date", "unknown", "excluded"}, "policyOrigins": {"assumed", "explicit", "contract"}, "earlyKinds": {"allowed", "not_allowed", "unknown"}}
		case "list_products":
			kinds := []string{}
			for _, k := range domain.AllProductKinds() {
				kinds = append(kinds, string(k))
			}
			enums = map[string][]string{"productKinds": kinds, "states": {string(domain.ProductStateOpen), string(domain.ProductStateSettled), string(domain.ProductStateCancelled)}}
		case "list_instruments":
			types := []string{}
			for _, t := range domain.AllInstrumentTypes() {
				types = append(types, string(t))
			}
			enums = map[string][]string{"types": types}
		case "get_upcoming_actions":
			enums = map[string][]string{"kinds": {"reservation_unresolved", "reservation_shortfall", "source_access", "product_schedule"}, "groups": {"attention", "scheduled", "eligible"}}
		case "list_agent_market_data":
			enums = map[string][]string{"operations": {string(domain.AgentQuoteAppend), string(domain.AgentQuoteCorrect), string(domain.AgentQuoteRetract)}, "observationKinds": {"latest", "close", "nav", "daily_reference"}}
		}
		for k, values := range enums {
			if p := filter.Properties[k]; p != nil && p.Items != nil {
				setSchemaEnum(p.Items, values)
			}
		}
	}
	if name == "get_market_data_batch" {
		for k, values := range map[string][]string{"view": {"current", "coverage", "history"}, "range": {"30d", "ytd", "1y", "all"}, "sourceFilter": {"all", "manual", "provider", "agent"}} {
			setSchemaEnum(schema.Properties[k], values)
		}
		targets := schema.Properties["targets"]
		min := 1
		targets.MinItems = &min
		if h := schema.Properties["historySort"]; h != nil {
			setSchemaEnum(h.Properties["field"], []string{"quotedAt", "effectiveDate"})
			setSchemaEnum(h.Properties["direction"], []string{"asc", "desc"})
		}
	}
	return schema
}
func setSchemaEnum(p *jsonschema.Schema, values []string) {
	if p == nil {
		return
	}
	p.Enum = make([]any, 0, len(values))
	for _, v := range values {
		p.Enum = append(p.Enum, v)
	}
}
func walkScaleSchema(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if s.Type == "array" {
		max := 100
		s.MaxItems = &max
	}
	for k, p := range s.Properties {
		if k == "query" {
			max := 200
			p.MaxLength = &max
		}
		if k == "cursor" {
			max := 2048
			p.MaxLength = &max
		}
		walkScaleSchema(p)
	}
	walkScaleSchema(s.Items)
}
