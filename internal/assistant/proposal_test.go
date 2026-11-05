package assistant

import "testing"

func TestCashProposalClosedInput(t *testing.T) {
	valid := `{"kind":"money_added","accountId":"f07f6130-239e-48d6-bd6c-341fb6cf0751","amount":"12.3456789","currency":"USD","reason":"income","effectiveAt":"2026-09-28T12:00:00Z"}`
	var proposal CashProposal
	if err := decodeProposal([]byte(valid), &proposal); err != nil || proposal.Amount != "12.3456789" {
		t.Fatal("valid exact proposal refused")
	}
	for _, raw := range []string{`null`, `{}`, valid + `{}`, valid[:len(valid)-1] + `,"approved":true}`, valid[:len(valid)-1] + `,"mutationId":"f07f6130-239e-48d6-bd6c-341fb6cf0751"}`, `{"kind":"cash_transfer","accountId":"f07f6130-239e-48d6-bd6c-341fb6cf0751","amount":"12","currency":"USD","reason":"income","effectiveAt":"2026-09-28T12:00:00Z"}`} {
		var out CashProposal
		if decodeProposal([]byte(raw), &out) == nil {
			t.Fatal("accepted unsupported proposal shape")
		}
	}
}
