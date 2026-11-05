package builtinai

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
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
	if c.Kind != history.ChangeMoneyAdded && c.Kind != history.ChangeMoneyRemoved {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	v := assistant.ChangeReview{ID: r.Binding.ID, Digest: r.Digest, State: r.State, ExpiresAt: r.Binding.Preview.ExpiresAt, Kind: string(c.Kind), Amount: c.Amount, Currency: c.Currency, Reason: c.Reason, EffectiveAt: c.EffectiveAt}
	if c.Note != nil {
		v.Note = *c.Note
	}
	for _, e := range r.Binding.Preview.Preview.Resulting {
		if e.AccountID != nil && *e.AccountID == c.AccountID && e.Currency == c.Currency {
			v.Account = e.Name
			v.ResultingAmount = e.Amount
		}
	}
	if v.Account == "" || v.ResultingAmount == "" {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	return v, nil
}
func (s reviewSession) Preview(ctx context.Context, p assistant.CashProposal) (assistant.ChangeReview, error) {
	if p.Kind != "money_added" && p.Kind != "money_removed" {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	command := history.ChangeCommandRequest{Kind: history.ChangeCommandKind(p.Kind), AccountID: p.AccountID, Amount: p.Amount, Currency: p.Currency, Reason: p.Reason, EffectiveAt: p.EffectiveAt}
	if p.Note != "" {
		command.Note = &p.Note
	}
	r, err := s.session.PreviewHostChange(ctx, command)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(r)
}
func (s reviewSession) Inspect(ctx context.Context, id string) (assistant.ChangeReview, error) {
	r, err := s.session.InspectHostChange(ctx, id)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(r)
}
func (s reviewSession) Confirm(ctx context.Context, id, digest string) (assistant.ChangeReview, error) {
	r, err := s.session.ConfirmHostChange(ctx, id, digest)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(r)
}
func (s reviewSession) Cancel(ctx context.Context, id string) error {
	return s.session.CancelHostChange(ctx, id)
}
func (s reviewSession) Repreview(ctx context.Context, id string) (assistant.ChangeReview, error) {
	r, err := s.session.InspectHostChange(ctx, id)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	if r.State != mcpserver.ReviewNeedsPreview {
		return assistant.ChangeReview{}, assistant.ErrConflict
	}
	if _, err := reviewView(r); err != nil {
		return assistant.ChangeReview{}, err
	}
	if err := s.session.CancelHostChange(ctx, id); err != nil {
		return assistant.ChangeReview{}, err
	}
	next, err := s.session.PreviewHostChange(ctx, r.Binding.Preview.Command)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(next)
}
