package assistant

import (
	"bytes"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"io"
)

func decodeProposal(raw []byte, proposal *CashProposal) error {
	d := jsoncodec.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(proposal); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrUnavailable
	}
	if (proposal.Kind != "money_added" && proposal.Kind != "money_removed") || !canonicalID(proposal.AccountID) || proposal.Amount == "" || proposal.Currency == "" || proposal.Reason == "" || proposal.EffectiveAt == "" {
		return ErrUnavailable
	}
	return nil
}
