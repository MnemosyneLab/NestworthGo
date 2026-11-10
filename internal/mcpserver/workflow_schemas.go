package mcpserver

import (
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"strings"
)

// Publish common structural dependencies before business validation. Decimal,
// date, identity, permission, source, and CAS validation remain authoritative.
func workflowSchema[I any](name string) *jsonschema.Schema {
	switch name {
	case "create_investment_review_rule", "update_investment_review_rule", "preview_investment_review_rule", "list_activities", "preview_batch", "preview_correction", "preview_product_terms", "preview_save_liquidity_policy", "get_data_repair_job", "get_market_data":
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
	case "create_investment_review_rule", "update_investment_review_rule":
		addReviewRuleSchema(s, s.Properties["input"].Properties)
	case "preview_investment_review_rule":
		addReviewRuleSchema(s, s.Properties)
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
	case "preview_product_terms":
		productTermsSchema(s.Properties["terms"], false)
		productPolicySchema(s.Properties["policy"], true)
	case "preview_save_liquidity_policy":
		productPolicySchema(s.Properties["policy"], false)
	case "preview_correction":
		setSchemaEnum(s.Properties["action"], []string{"fix", "undo"})
		s.OneOf = []*jsonschema.Schema{{Properties: map[string]*jsonschema.Schema{"action": {Enum: []any{"fix"}}}, Required: []string{"replacement"}}, {Properties: map[string]*jsonschema.Schema{"action": {Enum: []any{"undo"}}}, Not: &jsonschema.Schema{Required: []string{"replacement"}}}}
		addLedgerSchema(s.Properties["replacement"], true)
		slimLedgerCommandSchema(s.Properties["replacement"])
	case "preview_batch":
		commands := s.Properties["commands"]
		commands.Type, commands.Types = "array", nil
		min, max := 1, 100
		commands.MinItems, commands.MaxItems = &min, &max
		item := commands.Items
		addLedgerSchema(item, false)
		slimLedgerCommandSchema(item)
	}
	return s
}

func addLedgerSchema(item *jsonschema.Schema, correction bool) {
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
		if correction && v.kind == "position_import" {
			continue
		}
		if correction && v.kind == "trade" {
			branch.Required = append(branch.Required, "holdingId")
		}
		allowed := strings.Fields("kind note " + ledgerFields[history.ChangeCommandKind(v.kind)])
		if !correction {
			allowed = append(allowed, "effectiveAt", "effectiveLocalDate", "effectiveLocalTime")
		}
		branch.PropertyNames = &jsonschema.Schema{}
		setSchemaEnum(branch.PropertyNames, allowed)
		if v.kind == "position_transfer" {
			branch.OneOf = []*jsonschema.Schema{{Required: []string{"toHoldingId"}}, {Required: []string{"toAccountId"}}}
		}
		item.OneOf = append(item.OneOf, branch)
	}
	item.DependentRequired = map[string][]string{"fee": {"feeCurrency"}, "feeCurrency": {"fee"}, "effectiveLocalDate": {"effectiveLocalTime"}, "effectiveLocalTime": {"effectiveLocalDate"}, "interestOrFee": {"interestOrFeeCurrency"}, "interestOrFeeCurrency": {"interestOrFee"}}
	reasons := []string{}
	for _, reason := range []domain.ActivityReason{domain.ReasonIncome, domain.ReasonContribution, domain.ReasonGift, domain.ReasonOther, domain.ReasonExpense, domain.ReasonCardPurchase, domain.ReasonCardRefund, domain.ReasonFee, domain.ReasonTax, domain.ReasonReconciliation, domain.ReasonPrincipal, domain.ReasonInterest} {
		reasons = append(reasons, string(reason))
	}
	setSchemaEnum(item.Properties["reason"], reasons)
	setSchemaEnum(item.Properties["side"], []string{"buy", "sell"})
	item.Not = &jsonschema.Schema{Required: []string{"effectiveAt"}, AnyOf: []*jsonschema.Schema{{Required: []string{"effectiveLocalDate"}}, {Required: []string{"effectiveLocalTime"}}}}
}
