package builtinai

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"strings"
)

type reviewSession struct{ session *mcpserver.InProcessSession }

func (p *Pool) AcquireReview(ctx context.Context, id string) (assistant.ReviewSession, error) {
	if p.repository == nil {
		return nil, assistant.ErrUnavailable
	}
	s, err := p.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return reviewSession{s}, nil
}
func reviewView(r mcpserver.HostChangeReview) (assistant.ChangeReview, error) {
	c := r.Binding.Preview.Command
	if r.Binding.Source == "ai_full" && c.Kind == history.ChangeCashTransfer {
		lines := []string{c.Sent + " " + c.SentCurrency + " → " + c.Received + " " + c.ReceivedCurrency}
		if c.Fee != "" {
			lines = append(lines, "Fee: "+c.Fee+" "+c.FeeCurrency)
		}
		for _, e := range r.Binding.Preview.Preview.Resulting {
			lines = append(lines, e.Name+": "+e.Amount+" "+e.Currency)
		}
		return assistant.ChangeReview{Summary: strings.Join(lines, "\n"), Source: "ai_full", ID: r.Binding.ID, Digest: r.Digest, State: r.State, Kind: string(c.Kind), Amount: c.Sent, Currency: c.SentCurrency, EffectiveAt: c.EffectiveAt}, nil
	}
	if c.Kind != history.ChangeMoneyAdded && c.Kind != history.ChangeMoneyRemoved {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	currency, err := domain.ParseSupportedCurrency(c.Currency)
	if err != nil {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	v := assistant.ChangeReview{Source: r.Binding.Source, ID: r.Binding.ID, Digest: r.Digest, State: r.State, ExpiresAt: r.Binding.Preview.ExpiresAt, Kind: string(c.Kind), Amount: c.Amount, Currency: currency.String(), Reason: c.Reason, EffectiveAt: c.EffectiveAt}
	if c.Note != nil {
		v.Note = *c.Note
	}
	for _, e := range r.Binding.Preview.Preview.Resulting {
		if e.AccountID != nil && *e.AccountID == c.AccountID && e.Currency == currency.String() {
			v.Account = e.Name
			v.ResultingAmount = e.Amount
		}
	}
	if identity := r.Binding.CashAccount; identity != nil {
		v.Account = identity.Name
		v.AccountInstitution = identity.Institution
		v.AccountOwners = append([]string{}, identity.Owners...)
		v.AccountType = identity.AccountType
		v.AccountCurrency = identity.DefaultCurrency
	} else if v.State == mcpserver.ReviewAwaitingConfirmation {
		// Legacy reviews can still recover receipts, but cannot authorize a write
		// until a new preview captures readable identity.
		v.State = mcpserver.ReviewNeedsPreview
	}
	if v.Account == "" || v.ResultingAmount == "" {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	return v, nil
}
func (s reviewSession) Inspect(ctx context.Context, id string) (assistant.ChangeReview, error) {
	r, err := s.session.InspectHostChange(ctx, id)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(r)
}

func (s reviewSession) RetireUnsubmitted(ctx context.Context, id string) (assistant.ChangeReview, error) {
	if err := s.session.CancelHostChange(ctx, id); err != nil {
		return assistant.ChangeReview{}, err
	}
	return s.Inspect(ctx, id)
}
