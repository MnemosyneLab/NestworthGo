package desktop

import (
	"context"
	"errors"
	"slices"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type fundsPolicyLedger interface {
	SaveLiquidityPolicy(context.Context, application.SavePolicyInput) (domain.LiquidityPolicy, error)
	ResetLiquidityPolicy(context.Context, domain.LiquiditySourceRef, int) error
}

type fundsPolicyDraft struct {
	source                      domain.LiquiditySourceRef
	title                       string
	currency                    string
	id                          domain.LiquidityPolicyID
	revision                    int
	entry                       *widget.Clickable
	fields                      fundsPolicyFields
	list                        widget.List
	openingFocus                bool
	footerFocus                 *widget.Clickable
	problem                     i18n.MessageID
	latest                      *application.LiquidityReadSource
	write                       <-chan historyResult[domain.LiquidityPolicy]
	ack                         domain.LiquidityPolicy
	writeErr                    error
	checking                    bool
	resetWrite                  bool
	confirming                  bool
	confirmation                ui.Confirmation
	save, reset, cancel, reload widget.Clickable
}

func editableFundsPolicy(row application.LiquidityReadSource) bool {
	return row.Result.ProductID == nil && row.PolicyEditable != nil && *row.PolicyEditable
}

func fundsPolicyVersion(row *application.LiquidityReadSource) (domain.LiquidityPolicyID, int) {
	if row != nil && row.Result.Policy != nil && row.Result.PolicyOrigin == domain.PolicyOriginExplicit {
		return row.Result.Policy.ID, row.Result.Policy.Revision
	}
	return "", 0
}

func (p *availableFundsPage) policyButton(view ui.View, row application.LiquidityReadSource, title string, missing bool) layout.Widget {
	button := &p.sourceButtons(row.Result.SourceKey).policy
	name := i18n.FundsPolicyEdit
	if missing {
		name = i18n.FundsPolicyComplete
	}
	return func(gtx layout.Context) layout.Dimensions {
		if gtx.Enabled() && button.Clicked(gtx) {
			p.startPolicy(row, title, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		if gtx.Enabled() && p.policyFocus == button {
			p.policyFocus = nil
			gtx.Execute(key.FocusCmd{Tag: button})
		}
		label := view.Language.Text(name)
		return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Title: label, Name: label + " · " + title})(gtx)
	}
}

func (p *availableFundsPage) startPolicy(row application.LiquidityReadSource, title string, entry *widget.Clickable) {
	id, revision := fundsPolicyVersion(&row)
	p.policyNotice = i18n.NoMessage
	p.policy = &fundsPolicyDraft{source: row.Result.Ref, title: title, currency: row.Result.NativeCurrency.String(), id: id, revision: revision,
		entry: entry, fields: newFundsPolicyFields(row.Result.Policy), list: widget.List{List: layout.List{Axis: layout.Vertical}}, openingFocus: true}
}

func (p *availableFundsPage) policyUnresolved() bool {
	d := p.policy
	return d != nil && (d.write != nil || d.checking || d.confirming)
}

func (p *availableFundsPage) policyConfirming() bool {
	return p.policy != nil && p.policy.confirming
}

func (p *availableFundsPage) policySource(ref domain.LiquiditySourceRef) *application.LiquidityReadSource {
	if p.current == nil {
		return nil
	}
	rows := p.current.report.Sources
	i := slices.IndexFunc(rows, func(row application.LiquidityReadSource) bool { return row.Result.Ref.Key() == ref.Key() })
	if i < 0 {
		return nil
	}
	return &rows[i]
}

func (p *availableFundsPage) cancelPolicy(refresh bool) {
	d := p.policy
	p.policy = nil
	if row := p.policySource(d.source); row != nil && editableFundsPolicy(*row) && fundsMatchesSourceFilter(row.Result, p.selectedFundsBucket().HorizonOn, p.state.filter) {
		p.policyFocus = d.entry
	} else {
		p.state.focusSources = true
	}
	if refresh {
		p.read()
	}
}

func (p *availableFundsPage) postPolicy(reset bool) {
	d := p.policy
	if d.write != nil || d.checking || d.problem == i18n.FundsPolicyConflict {
		return
	}
	input := application.SavePolicyInput{Source: d.source, ExpectedRevision: d.revision}
	if !reset {
		var valid bool
		input.Policy, valid = d.fields.input()
		if !valid {
			d.problem = i18n.ProductEditValidDays
			return
		}
	}
	d.problem, d.resetWrite = i18n.NoMessage, reset
	d.write = runHistoryRead(&p.workers, p.invalidate, func() (domain.LiquidityPolicy, error) {
		if reset {
			return domain.LiquidityPolicy{}, p.policyLedger.ResetLiquidityPolicy(p.ctx, input.Source, input.ExpectedRevision)
		}
		return p.policyLedger.SaveLiquidityPolicy(p.ctx, input)
	})
}

func (p *availableFundsPage) pollPolicy() {
	d := p.policy
	select {
	case result := <-d.write:
		d.write, d.confirming = nil, false
		var invalid *domain.Error
		if errors.As(result.err, &invalid) && (invalid.Code == domain.ErrValidation || invalid.Code == domain.ErrDecimalOverflow) {
			d.problem, d.footerFocus = fundsPolicyValidation(invalid.Field), &d.save
			return
		}
		d.ack, d.writeErr, d.checking = result.value, result.err, true
		d.footerFocus = &d.reload
		p.read()
	default:
	}
	if !d.checking || p.request != nil || p.err != nil {
		return
	}
	d.checking = false
	d.latest = p.policySource(d.source)
	id, revision := fundsPolicyVersion(d.latest)
	available := d.latest != nil && editableFundsPolicy(*d.latest)
	var issue *domain.Error
	if available && d.writeErr == nil && (d.resetWrite && revision == 0 ||
		!d.resetWrite && id != "" && id == d.ack.ID && revision == d.ack.Revision && revision == d.revision+1) {
		p.policyNotice = i18n.FundsPolicySaved
		if d.resetWrite {
			p.policyNotice = i18n.FundsPolicyResetSaved
		}
		p.cancelPolicy(false)
	} else if available && id == d.id && revision == d.revision && d.writeErr != nil &&
		!(errors.As(d.writeErr, &issue) && (issue.Code == domain.ErrRevisionConflict || issue.Code == domain.ErrNotFound)) {
		d.problem, d.footerFocus = i18n.FundsPolicySaveFailed, &d.save
	} else {
		d.problem, d.footerFocus = i18n.FundsPolicyConflict, &d.reload
		if !available {
			d.latest, d.footerFocus = nil, &d.cancel
		}
	}
}

func fundsPolicyValidation(field string) i18n.MessageID {
	switch field {
	case "unlockOn":
		return i18n.FundsPolicyDateRequired
	case "earlyKind", "earlyGrossAmount":
		return i18n.FundsPolicyEarlyRequired
	case "settlementDays", "earlySettlementDays":
		return i18n.ProductEditValidDays
	default:
		return i18n.FundsPolicyInvalid
	}
}

func (p *availableFundsPage) layoutPolicyConfirmation(gtx layout.Context, view ui.View) {
	d, l := p.policy, view.Language
	d.confirmation.Layout(gtx, view, ui.ConfirmationOptions{Title: l.Text(i18n.FundsPolicyResetTitle), Body: l.Text(i18n.FundsPolicyResetBody),
		Cancel: l.Text(i18n.InstitutionsCancel), Confirm: l.Text(i18n.FundsPolicyResetConfirm), Busy: d.write != nil}, func() {
		d.confirming, d.footerFocus = false, &d.reset
		gtx.Execute(op.InvalidateCmd{})
	}, func() {
		p.postPolicy(true)
		gtx.Execute(op.InvalidateCmd{})
	})
}
