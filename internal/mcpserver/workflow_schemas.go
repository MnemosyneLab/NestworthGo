package mcpserver

import "github.com/google/jsonschema-go/jsonschema"

// Publish common structural dependencies before business validation. Decimal,
// date, identity, permission, source, and CAS validation remain authoritative.
func workflowSchema[I any](name string) *jsonschema.Schema {
	switch name {
	case "preview_investment_review_rule", "list_activities", "preview_batch", "get_data_repair_job", "get_market_data":
	default:
		return nil
	}
	s := scaleReadSchema[I](name)
	if s == nil {
		var err error
		s, err = jsonschema.For[I](nil)
		if err != nil {
			panic(err)
		}
	}
	switch name {
	case "preview_investment_review_rule":
		low, high := float64(0), float64(100)
		s.Properties["limit"].Minimum = &low
		s.Properties["limit"].Maximum = &high
	case "list_activities":
		s.DependentRequired = map[string][]string{
			"afterEffectiveAt": {"afterCreatedAt", "afterId"}, "afterCreatedAt": {"afterEffectiveAt", "afterId"}, "afterId": {"afterEffectiveAt", "afterCreatedAt"},
		}
		low, high := float64(0), float64(100)
		s.Properties["limit"].Minimum = &low
		s.Properties["limit"].Maximum = &high
	case "get_data_repair_job":
		setSchemaEnum(s.Properties["view"], []string{"progress", "evidence"})
	case "get_market_data":
		s.Properties["targets"].Items.OneOf = []*jsonschema.Schema{
			{Required: []string{"instrumentId"}, Not: &jsonschema.Schema{AnyOf: []*jsonschema.Schema{{Required: []string{"baseCurrency"}}, {Required: []string{"quoteCurrency"}}}}},
			{Required: []string{"baseCurrency", "quoteCurrency"}, Not: &jsonschema.Schema{Required: []string{"instrumentId"}}},
		}
		s.DependentRequired = map[string][]string{"fromDate": {"toDate"}, "toDate": {"fromDate"}}
		s.Not = &jsonschema.Schema{Required: []string{"range", "fromDate"}}
		s.If = &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"historyLayout": {Enum: []any{"records"}}}, Required: []string{"historyLayout"}}
		s.Then = &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"view": {Enum: []any{"history"}}}, Required: []string{"view"}}
		batchOnly := func() *jsonschema.Schema {
			return schemaAnyRequired("targets", "view", "range", "fromDate", "toDate", "historyLayout", "sourceFilter", "sort", "historySort", "limit")
		}
		s.OneOf = []*jsonschema.Schema{
			{Required: []string{"instrumentId"}, Not: &jsonschema.Schema{AnyOf: []*jsonschema.Schema{schemaAnyRequired("baseCurrency", "quoteCurrency"), batchOnly()}}},
			{Required: []string{"baseCurrency", "quoteCurrency"}, Not: &jsonschema.Schema{AnyOf: []*jsonschema.Schema{schemaAnyRequired("instrumentId"), batchOnly()}}},
			{Required: []string{"targets"}, Not: schemaAnyRequired("instrumentId", "baseCurrency", "quoteCurrency")},
		}
	case "preview_batch":
		commands := s.Properties["commands"]
		commands.Type, commands.Types = "array", nil
		min, max := 1, 100
		commands.MinItems, commands.MaxItems = &min, &max
		item := commands.Items
		variants := []struct {
			kind     string
			required []string
		}{
			{"position_import", []string{"accountId", "instrumentId", "quantity", "unitCost", "currency"}},
			{"money_added", []string{"accountId", "amount", "currency", "reason"}},
			{"money_removed", []string{"accountId", "amount", "currency", "reason"}},
			{"cash_dividend", []string{"holdingId", "amount", "currency"}},
			{"cash_transfer", []string{"fromAccountId", "toAccountId", "sent", "sentCurrency", "received", "receivedCurrency"}},
			{"fx_conversion", []string{"accountId", "sold", "soldCurrency", "bought", "boughtCurrency"}},
			{"position_transfer", []string{"fromHoldingId", "quantity"}},
			{"trade", []string{"settlementAccountId", "instrumentId", "side", "quantity", "gross", "grossCurrency"}},
			{"debt_draw", []string{"debtAccountId", "cashAccountId", "principal", "principalCurrency"}},
			{"debt_payment", []string{"debtAccountId", "cashAccountId", "principal", "principalCurrency"}},
		}
		for _, v := range variants {
			branch := &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"kind": {Enum: []any{v.kind}}}, Required: append([]string{"kind"}, v.required...)}
			if v.kind == "position_transfer" {
				branch.OneOf = []*jsonschema.Schema{{Required: []string{"toHoldingId"}}, {Required: []string{"toAccountId"}}}
			}
			item.OneOf = append(item.OneOf, branch)
		}
		item.DependentRequired = map[string][]string{"fee": {"feeCurrency"}, "effectiveLocalDate": {"effectiveLocalTime"}, "effectiveLocalTime": {"effectiveLocalDate"}, "interestOrFee": {"interestOrFeeCurrency"}}
		slimLedgerCommandSchema(item)
	}
	return s
}
