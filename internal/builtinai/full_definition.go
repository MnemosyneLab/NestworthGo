package builtinai

import "github.com/waltwang/nestworth-go/internal/agentcapability"

func fullDefinition() agentcapability.Definition {
	props := map[string]any{}
	for _, key := range []string{"accountId", "amount", "currency", "reason", "fromAccountId", "toAccountId", "sent", "sentCurrency", "received", "receivedCurrency", "fee", "feeCurrency", "effectiveAt", "note"} {
		props[key] = map[string]any{"type": "string"}
	}
	props["kind"] = map[string]any{"type": "string", "enum": []string{"money_added", "money_removed", "cash_transfer"}}
	closed := false
	return agentcapability.Definition{Name: "execute_ledger_change", Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{OpenWorldHint: &closed}, Description: "Full permission: directly record one completed cash deposit/withdrawal or account cash transfer, without a separate confirmation. Host previews and commits with domain validation and a stable durable operation identity. Require explicit exact amounts/currencies, account IDs from authoritative reads and RFC3339 effectiveAt. Cash changes require accountId, amount, currency, reason. Transfers require fromAccountId, toAccountId, sent/sentCurrency, received/receivedCurrency, optional fee/feeCurrency. Ask for missing business details. No batches, directory, positions, reconciliation, future/incomplete financial events or guessed values. Never retry a failed/interrupted/unknown write or recreate it with different arguments; inspect the original desktop operation. Success is authoritative only when status is succeeded.", InputSchema: map[string]any{"type": "object", "properties": props, "required": []string{"kind", "effectiveAt"}, "additionalProperties": false}}
}
