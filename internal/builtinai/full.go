package builtinai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

// This bounded stage intentionally excludes directory composites and batches.
// Both dispatch and discovery use the same closed command fields.
type fullCommand struct {
	Kind             history.ChangeCommandKind `json:"kind"`
	AccountID        string                    `json:"accountId,omitempty"`
	Amount           string                    `json:"amount,omitempty"`
	Currency         string                    `json:"currency,omitempty"`
	Reason           string                    `json:"reason,omitempty"`
	FromAccountID    string                    `json:"fromAccountId,omitempty"`
	ToAccountID      string                    `json:"toAccountId,omitempty"`
	Sent             string                    `json:"sent,omitempty"`
	SentCurrency     string                    `json:"sentCurrency,omitempty"`
	Received         string                    `json:"received,omitempty"`
	ReceivedCurrency string                    `json:"receivedCurrency,omitempty"`
	Fee              string                    `json:"fee,omitempty"`
	FeeCurrency      string                    `json:"feeCurrency,omitempty"`
	EffectiveAt      string                    `json:"effectiveAt"`
	Note             string                    `json:"note,omitempty"`
}

func (s reviewSession) PreviewFull(ctx context.Context, raw json.RawMessage) (assistant.ChangeReview, error) {
	var in fullCommand
	d := jsoncodec.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 4096 || d.Decode(&in) != nil || in.EffectiveAt == "" {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	canonical, err := jsoncodec.Marshal(in)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	var command history.ChangeCommandRequest
	if jsoncodec.Unmarshal(canonical, &command) != nil {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	r, err := s.session.PreviewFullChange(ctx, command)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(r)
}
func (s reviewSession) ExecuteFull(ctx context.Context, id string) (assistant.ChangeReview, error) {
	r, err := s.session.ExecuteFullChange(ctx, id)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(r)
}
func fullDefinition() agentcapability.Definition {
	props := map[string]any{}
	for _, key := range []string{"accountId", "amount", "currency", "reason", "fromAccountId", "toAccountId", "sent", "sentCurrency", "received", "receivedCurrency", "fee", "feeCurrency", "effectiveAt", "note"} {
		props[key] = map[string]any{"type": "string"}
	}
	props["kind"] = map[string]any{"type": "string", "enum": []string{"money_added", "money_removed", "cash_transfer"}}
	closed := false
	return agentcapability.Definition{Name: "execute_ledger_change", Effect: agentcapability.Write, Annotations: &agentcapability.Annotations{OpenWorldHint: &closed}, Description: "Full permission: directly record one completed cash deposit/withdrawal or account cash transfer, without a separate confirmation. Host previews and commits with domain validation and a stable durable operation identity. Require explicit exact amounts/currencies, account IDs from authoritative reads and RFC3339 effectiveAt. Cash changes require accountId, amount, currency, reason. Transfers require fromAccountId, toAccountId, sent/sentCurrency, received/receivedCurrency, optional fee/feeCurrency. Ask for missing business details. No batches, directory, positions, reconciliation, future/incomplete financial events or guessed values. Never retry a failed/interrupted/unknown write or recreate it with different arguments; inspect the original desktop operation. Success is authoritative only when status is succeeded.", InputSchema: map[string]any{"type": "object", "properties": props, "required": []string{"kind", "effectiveAt"}, "additionalProperties": false}}
}
