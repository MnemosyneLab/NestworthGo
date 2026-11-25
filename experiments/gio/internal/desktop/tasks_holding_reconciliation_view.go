package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/statement"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *tasksPage) holdingReconciliationRows(view ui.View) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	if c == nil || len(c.directory.accounts) == 0 && len(c.directory.sessions) == 0 {
		return nil
	}
	if c.sessionButtons == nil {
		c.sessionButtons = make(map[string]*widget.Clickable)
	}
	rows := []layout.Widget{tradeTitle(view, i18n.ReconciliationSessions)}
	for _, session := range c.directory.sessions {
		button := portfolioButton(c.sessionButtons, session.ID)
		name := strings.Join(session.AccountNames, " · ")
		status := l.Text(statementStatus(session))
		when := statementRecordedTime(view, session.StartedAt)
		openLabel := i18n.ReconciliationOpen
		if len(session.AccountIDs) == 1 {
			if a, ok := c.directory.account(session.AccountIDs[0]); ok && a.TrackingMode == domain.TrackingHoldings {
				openLabel = i18n.HoldingReconOpen
			}
		}
		rows = append(rows, fundsCard(view, name,
			view.Message(status+" · "+when, view.Colors.SecondaryInk),
			c.link(view, button, fmt.Sprintf(l.Text(openLabel), name), func() {
				p.openHoldingReconciliation(session.ID, button)
			})))
	}
	if len(c.directory.accounts) == 0 {
		return rows
	}
	return append(rows, c.link(view, &c.start, l.Text(i18n.ReconciliationStart), func() {
		c.draft = newHoldingReconDraft(&c.start)
		c.draft.action = statement.Start
	}))
}

func (c *holdingReconciliation) link(view ui.View, button *widget.Clickable, label string, open func()) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		c.restoreFocus(gtx, button)
		return taskLink(view, button, label, func() {
			offset := view.PageScrollOffset()
			view.RestorePageScroll(0)
			open()
			if c.draft != nil || c.dated != nil || c.cashDated != nil {
				c.editScroll = offset
			}
		})(gtx)
	}
}

func (c *holdingReconciliation) restoreFocus(gtx layout.Context, button *widget.Clickable) {
	if c.restore == button {
		gtx.Execute(key.FocusCmd{Tag: button})
		c.restore = nil
	}
}

func (p *tasksPage) layoutHoldingReconciliation(gtx layout.Context, view ui.View) layout.Dimensions {
	gtx, view = view.Measure(gtx, 690)
	c, l := p.holdingRecon, view.Language
	if c.draft != nil {
		return p.layoutHoldingReconDraft(gtx, view)
	}
	if c.dated != nil {
		return p.layoutHoldingDatedChange(gtx, view)
	}
	if c.cashDated != nil {
		return p.layoutDatedCheckpointCash(gtx, view)
	}
	if c.restoreEditScroll && c.loading == nil {
		view.RestorePageScroll(c.editScroll)
		c.restoreEditScroll = false
		gtx.Execute(op.InvalidateCmd{})
	}
	title := i18n.TasksReconciliationTab
	if c.session != nil && len(c.session.Accounts) == 1 && c.session.Accounts[0].Mode == string(domain.TrackingHoldings) {
		title = i18n.HoldingReconTitle
	}
	rows := []layout.Widget{
		c.link(view, &c.back, l.Text(i18n.HoldingReconBack), func() {
			entry := c.sessionEntry
			c.discard()
			c.restore = entry
		}),
		tradeTitle(view, title),
	}
	switch {
	case c.loading != nil:
		rows = append(rows, view.Message(l.Text(i18n.TasksChecksLoading), view.Colors.SecondaryInk))
	case c.readFailed:
		rows = append(rows, view.Message(l.Text(i18n.TasksChecksReadFailed), view.Colors.ErrorInk),
			c.link(view, &c.reload, l.Text(i18n.HistoryRetry), p.readHoldingReconciliation))
	case c.session != nil:
		rows = append(rows, p.holdingSessionRows(view)...)
	}
	return holdingReconColumn(gtx, rows)
}

func (p *tasksPage) holdingSessionRows(view ui.View) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	s := c.session
	rows := []layout.Widget{view.Message(l.Text(statementStatus(*s)), view.Colors.Ink)}
	if c.saved != i18n.NoMessage {
		rows = append(rows, view.Message(l.Text(c.saved), view.Colors.Ink))
	}
	open := func(button *widget.Clickable, action statement.Action) func() {
		return func() {
			c.draft = newHoldingReconDraft(button)
			c.draft.action = action
			c.saved = i18n.NoMessage
		}
	}
	var controls []layout.Widget
	if s.Status == "paused" {
		controls = append(controls, c.link(view, &c.resume, l.Text(i18n.HoldingReconResume), open(&c.resume, statement.Resume)))
	} else if c.ledger != nil && s.Status == "in_progress" {
		controls = append(controls, c.link(view, &c.pause, l.Text(i18n.HoldingReconPause), open(&c.pause, statement.Pause)))
	}
	if c.ledger != nil && s.Status != "completed" {
		controls = append(controls, c.link(view, &c.finish, l.Text(i18n.HoldingReconFinish), open(&c.finish, statement.Finish)))
	}
	if len(controls) != 0 {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: 8}).Layout(gtx, len(controls), func(gtx layout.Context, i int) layout.Dimensions {
				return controls[i](gtx)
			})
		})
	}
	if c.ledger != nil {
		rows = append(rows, holdingReconCounts(view, *s))
	}
	if s.CompletedAt != "" {
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.StatementCompletedAt), statementRecordedTime(view, s.CompletedAt)), view.Colors.SecondaryInk))
		if s.ClosingNote != "" {
			rows = append(rows, view.Message(s.ClosingNote, view.Colors.Ink))
		}
	}
	found := false
	for _, account := range s.Accounts {
		activeAccount, active := c.directory.account(account.AccountID)
		if !slices.ContainsFunc(c.directory.accounts, func(a domain.Account) bool { return a.ID.String() == account.AccountID }) {
			continue
		}
		rows = append(rows, view.Message(account.Name, view.Colors.Ink))
		if c.ledger != nil && s.Status == "in_progress" && slices.ContainsFunc(account.Items, uncheckedHoldingItem) {
			if at := slices.IndexFunc(account.Items, func(item domain.ReconciliationItem) bool { return item.Kind == "scope" }); at >= 0 {
				scope := account.Items[at]
				button := &c.checkpointActions(account.AccountID, scope.ID).checkAccount
				rows = append(rows, c.link(view, button, l.Text(i18n.HoldingReconCheckAccount), func() {
					p.openHoldingAccountCheck(account, scope, button)
				}))
			}
		}
		for _, item := range account.Items {
			if item.Kind != "holding" || item.HoldingID == "" {
				continue
			}
			found = true
			fields := holdingPositionEvidence(view, item)
			buttons := c.checkpointActions(account.AccountID, item.ID)
			if s.Status == "in_progress" && item.ProductID == "" && !item.ArchivedNonzero {
				open := func(button *widget.Clickable) {
					c.saved = i18n.NoMessage
					c.draft = newHoldingReconDraft(button)
					c.draft.accountID, c.draft.itemID = account.AccountID, item.ID
				}
				fields = append(fields, c.link(view, &buttons.quantity, l.Text(i18n.HoldingReconAdjustQuantity), func() {
					open(&buttons.quantity)
					target := &holdingReconQuantity{quantity: ui.NewTextField(true), unitCost: ui.NewTextField(true)}
					target.quantity.Editor.SetText(item.Value)
					c.draft.target = target
				}), c.link(view, &buttons.cost, l.Text(i18n.HoldingReconCorrect), func() {
					open(&buttons.cost)
				}))
				if c.ledger != nil {
					fields = append(fields, c.link(view, &buttons.record, l.Text(i18n.HoldingReconRecordChange), func() {
						p.startHoldingDatedChange(account, item, &buttons.record)
					}))
				}
			}
			if c.ledger != nil {
				fields = append(fields, p.holdingCheckpointRows(view, account, item, buttons)...)
			}
			fields = append(fields, p.holdingReceiptLinks(view, account.AccountID, item.ID)...)
			rows = append(rows, fundsCard(view, item.Label, fields...))
		}
		if c.ledger != nil {
			for _, item := range account.Items {
				if item.Kind == "holding" || !holdingCheckable(item) {
					continue
				}
				fields := holdingCheckpointFacts(view, item)
				buttons := c.checkpointActions(account.AccountID, item.ID)
				if s.Status == "in_progress" && active && (item.Kind == "cash" || item.Kind == "scope" || item.Kind == "balance") {
					label := l.Text(i18n.HoldingReconRecordCash)
					if activeAccount.TrackingMode != domain.TrackingHoldings {
						label = l.Text(i18n.LiabilityRecord)
						if item.Kind == "scope" {
							label += " · " + l.Text(i18n.StatementScope)
						}
					} else if item.Currency != "" {
						label += " · " + item.Currency
					}
					fields = append(fields, c.link(view, &buttons.cash, label, func() {
						p.startDatedCheckpointCash(activeAccount, item, &buttons.cash)
					}))
					if item.Kind == "scope" && history.TradeAccount(activeAccount) {
						fields = append(fields, c.link(view, &buttons.importPosition, l.Text(i18n.OpeningPositionOpen), func() {
							p.startHoldingDatedChange(account, item, &buttons.importPosition)
						}))
						if domain.ProductAccountEligible(activeAccount) == nil {
							fields = append(fields, c.link(view, &buttons.addProduct, l.Text(i18n.ProductAddAction), func() {
								p.openHoldingProductAdd(account, item, &buttons.addProduct, l.Text(i18n.ProductAddDeposit))
							}))
						}
					}
				}
				if contract, ok := managedProductCheckpoint(item); s.Status == "in_progress" && ok {
					fields = append(fields, c.link(view, &buttons.terms, l.Text(i18n.ProductEditTerms), func() {
						p.openHoldingProductCorrection(account, item, contract, &buttons.terms, holdingEditProductTerms)
					}))
					if contract.State == domain.ProductStateOpen {
						if contract.Kind == domain.ProductLockedProduct {
							fields = append(fields, c.link(view, &buttons.valuation, l.Text(i18n.HoldingReconProductValuation), func() {
								p.openHoldingProductCorrection(account, item, contract, &buttons.valuation, holdingObserveProductValue)
							}))
						}
						fields = append(fields,
							c.link(view, &buttons.interest, l.Text(i18n.HoldingReconProductInterest), func() {
								p.openHoldingProductCorrection(account, item, contract, &buttons.interest, holdingReceiveProductInterest)
							}),
							c.link(view, &buttons.settle, l.Text(i18n.ProductReceiptAction), func() {
								p.openHoldingProductCorrection(account, item, contract, &buttons.settle, holdingSettleProduct)
							}),
							c.link(view, &buttons.renew, l.Text(i18n.ProductRenewAction), func() {
								p.openHoldingProductCorrection(account, item, contract, &buttons.renew, holdingRenewProduct)
							}))
					}
				}
				fields = append(fields, p.holdingCheckpointRows(view, account, item, buttons)...)
				fields = append(fields, p.holdingReceiptLinks(view, account.AccountID, item.ID)...)
				rows = append(rows, fundsCard(view, holdingCheckLabel(view, item), fields...))
			}
		}
	}
	if !found && slices.ContainsFunc(s.Accounts, func(a domain.ReconciliationAccount) bool { return a.Mode == string(domain.TrackingHoldings) }) {
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconEmpty), view.Colors.SecondaryInk))
	}
	if c.ledger != nil {
		rows = append(rows, p.reconciliationJournalRows(view)...)
	}
	return rows
}

func (c *holdingReconciliation) checkpointActions(accountID, itemID string) *holdingReconItemActions {
	at := accountID + "/" + itemID
	if c.itemButtons[at] == nil {
		c.itemButtons[at] = new(holdingReconItemActions)
	}
	return c.itemButtons[at]
}

func (p *tasksPage) holdingCheckpointRows(view ui.View, account domain.ReconciliationAccount, item domain.ReconciliationItem, buttons *holdingReconItemActions) []layout.Widget {
	c := p.holdingRecon
	rows := holdingCheckEvidence(view, item)
	if len(item.Attempts) != 0 {
		l := view.Language
		label := fmt.Sprintf(l.Text(i18n.HoldingReconShowAttempts), len(item.Attempts))
		rows = append(rows, buttons.attempts.toggle(view, label, l.Text(i18n.HoldingReconHideAttempts)))
		if buttons.attempts.open {
			rows = append(rows, holdingAttemptRows(view, item, c.zone)...)
		}
	}
	if c.session.Status == "in_progress" {
		rows = append(rows, c.link(view, &buttons.check, fmt.Sprintf(view.Language.Text(i18n.HoldingReconCheck), holdingCheckLabel(view, item)), func() {
			p.openHoldingCheckpoint(account, item, &buttons.check)
		}))
	}
	return rows
}

func holdingPositionEvidence(view ui.View, item domain.ReconciliationItem) []layout.Widget {
	l := view.Language
	basis := l.Text(i18n.OverviewNoValue)
	if item.Cost != "" {
		basis = portfolioDecimal(view, item.Cost) + " " + item.Currency
	}
	return []layout.Widget{
		view.Message(fmt.Sprintf(l.Text(i18n.AccountQuantity), portfolioDecimal(view, item.Value)), view.Colors.Ink),
		view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconBasis), basis), view.Colors.Ink),
	}
}

func (p *tasksPage) layoutHoldingReconDraft(gtx layout.Context, view ui.View) layout.Dimensions {
	c, l := p.holdingRecon, view.Language
	d := c.draft
	if !c.unresolved() && d.cancel.Clicked(gtx) {
		c.draft, c.restore, c.restoreEditScroll = nil, d.entry, c.id != ""
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.review != nil {
		return p.layoutHoldingReconReview(gtx, view)
	}
	title := i18n.HoldingReconCorrect
	if d.target != nil {
		title = i18n.HoldingReconAdjustQuantity
	}
	switch d.action {
	case statement.Start:
		title = i18n.ReconciliationStart
	case statement.Resume:
		title = i18n.HoldingReconResume
	case statement.Pause:
		title = i18n.HoldingReconPause
	case statement.CheckItem:
		title = i18n.HoldingReconCheckTitle
	case statement.Check:
		title = i18n.HoldingReconCheckAccount
	case statement.Finish:
		title = i18n.HoldingReconFinish
	}
	if d.product != nil {
		title = d.product.title()
	}
	origin := view.Origin
	rows := []layout.Widget{tradeTitle(view, title)}
	ready := c.loading == nil && !c.readFailed && d.reading == nil
	if d.check != nil {
		rows = append(rows, c.holdingCheckFields(view, d, &origin, ready)...)
	} else if d.product != nil && d.product.add != nil {
		rows = append(rows, c.holdingProductAddFields(view, &origin, d, ready)...)
	} else if d.product != nil {
		rows = append(rows, p.holdingProductFields(view, d, ready)...)
		productReady := ready && d.product.loading == nil && !d.product.loadFailed
		if d.product.terms != nil {
			rows = append(rows, c.productTermsFields(view, &origin, d, d.product.terms, productReady, true)...)
		} else if d.product.interest != nil {
			rows = append(rows, c.holdingProductInterestFields(view, &origin, d, productReady)...)
		} else if d.product.settlement != nil {
			rows = append(rows, c.holdingProductSettlementFields(view, &origin, d, d.product.settlement, productReady)...)
		} else if d.product.renewal != nil {
			rows = append(rows, c.holdingProductRenewalFields(view, &origin, d, productReady)...)
		}
	} else if d.itemID != "" {
		if a, item, ok := c.item(d); ok {
			rows = append(rows, view.Message(a.Name+" · "+item.Label, view.Colors.Ink))
			rows = append(rows, holdingPositionEvidence(view, item)...)
			totalLabel := i18n.HoldingReconTotal
			if d.target != nil {
				totalLabel = i18n.HoldingReconOptionalTotal
				rows = append(rows, func(gtx layout.Context) layout.Dimensions {
					d.focusAdjustmentField(gtx, &d.target.quantity)
					return d.target.quantity.Layout(gtx, view, ui.FieldOptions{Label: i18n.HoldingReconTargetQuantity, Disabled: !ready})
				}, func(gtx layout.Context) layout.Dimensions {
					return d.target.unitCost.LayoutCurrency(gtx, view, ui.FieldOptions{
						Label: i18n.HoldingReconAcquisitionCost, Disabled: !ready,
					}, item.Currency)
				})
			}
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				if d.target == nil {
					d.focusAdjustmentField(gtx, &d.total)
				}
				return d.total.LayoutCurrency(gtx, view, ui.FieldOptions{
					Label: totalLabel, Disabled: !ready,
				}, item.Currency)
			})
		}
	} else if d.action == statement.Start {
		choices := make([]ui.Choice, 0, len(c.directory.accounts))
		for _, a := range c.directory.accounts {
			choices = append(choices, ui.Choice{ID: a.ID.String(), Label: a.Name})
		}
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if d.initialFocus {
				gtx.Execute(key.FocusCmd{Tag: &d.accountPick.Trigger})
				d.initialFocus = false
			}
			d.accountPick.Layout(gtx, view, ui.PickerField{Label: i18n.AnalysisAccount, Origin: origin, Next: &d.note.Editor, Disabled: !ready},
				ui.SelectOptions{Items: choices, Searchable: true}, d.accountID, func(id string) {
					d.accountID, d.problem = id, i18n.NoMessage
				})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		})
	} else if c.session != nil {
		rows = append(rows, view.Message(holdingReconciliationAccountNames(c.session), view.Colors.Ink))
	}
	if d.action == statement.Finish {
		if c.session != nil {
			rows = append(rows, holdingReconCounts(view, *c.session))
		}
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconFinishHelp), view.Colors.SecondaryInk),
			checkpointCheckbox(view, &d.partial, i18n.HoldingReconPartial, ready))
	}
	if d.product == nil && d.action != statement.Resume && d.action != statement.Pause {
		label := i18n.PreviewNotes
		if d.action == statement.Finish {
			label = i18n.HoldingReconClosingNote
		}
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return d.note.Layout(gtx, view, ui.FieldOptions{Label: label, MaxBytes: 2000, Disabled: !ready})
		})
	}
	if d.problem != i18n.NoMessage {
		rows = append(rows, view.Message(l.Text(d.problem), view.Colors.ErrorInk))
	}
	if c.readFailed {
		rows = append(rows, c.link(view, &c.reload, l.Text(i18n.HistoryRetry), p.readHoldingReconciliation))
	}
	if !ready && !c.readFailed {
		rows = append(rows, view.Message(l.Text(i18n.TasksChecksLoading), view.Colors.SecondaryInk))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		canReview := ready && c.canReview(d)
		if canReview && d.preview.Clicked(gtx) {
			p.previewHoldingReconciliation()
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(taskPagerButton(view, &d.cancel, i18n.InstitutionsCancel, true)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !canReview {
					gtx = gtx.Disabled()
				}
				view.Focus.Watch(gtx, &d.preview)
				return view.PrimaryButton(&d.preview, l.Text(i18n.HoldingReconReview))(gtx)
			}))
	})
	// The same origin calculation places the picker correctly after scrolling.
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, row)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}

func (d *holdingReconDraft) focusAdjustmentField(gtx layout.Context, field *ui.TextField) {
	if d.initialFocus {
		gtx.Execute(key.FocusCmd{Tag: &field.Editor})
		d.initialFocus = false
	}
}

func (p *tasksPage) layoutHoldingReconReview(gtx layout.Context, view ui.View) layout.Dimensions {
	c, l := p.holdingRecon, view.Language
	d := c.draft
	r := d.review
	rows := []layout.Widget{tradeTitle(view, i18n.HoldingReconReview)}
	if r.terms != nil && r.terms.Before != nil && r.terms.After != nil {
		rows = append(rows, holdingProductTermsReviewRows(view, r.terms.Before.Product, r.terms.After.Product)...)
	} else if r.operation != nil {
		rows = append(rows, holdingProductOperationReviewRows(view, d.product, r.operation)...)
	} else if r.valuation != nil {
		rows = append(rows, p.holdingProductValuationReviewRows(view, r.valuation)...)
	} else if r.session != nil {
		rows = append(rows, view.Message(holdingReconciliationAccountNames(r.session), view.Colors.Ink),
			view.Message(l.Text(statementStatus(*r.session)), view.Colors.Ink))
		if d.action == statement.Finish || d.check != nil {
			rows = append(rows, holdingReconCounts(view, *r.session))
		}
		if d.action == statement.Finish {
			rows = append(rows, view.Message(l.Text(i18n.HoldingReconFinishHelp), view.Colors.SecondaryInk))
		}
		if d.action == statement.Check {
			if account, _, ok := c.checkpoint(d.accountID, d.itemID, false); ok {
				for _, candidate := range account.Items {
					if uncheckedHoldingItem(candidate) {
						if _, checked, ok := checkpointInSession(r.session, account.AccountID, candidate.ID, false); ok {
							rows = append(rows, holdingBatchItem(view, checked))
						}
					}
				}
			}
		} else if d.check != nil {
			if _, item, ok := checkpointInSession(r.session, d.accountID, d.itemID, false); ok {
				rows = append(rows, view.Message(holdingCheckLabel(view, item), view.Colors.Ink))
				if item.Kind != "scope" {
					rows = append(rows, holdingCheckpointFacts(view, item)...)
				}
				rows = append(rows, holdingCheckEvidence(view, item)...)
			}
		}
	} else if a, item, ok := c.item(d); ok {
		rows = append(rows, view.Message(a.Name+" · "+item.Label, view.Colors.Ink))
		rows = append(rows, holdingPositionEvidence(view, item)...)
		for _, change := range r.changes {
			for _, result := range change.Resulting {
				if result.Target == string(domain.EffectTargetHoldingQuantity) && result.HoldingID != nil && *result.HoldingID == item.HoldingID {
					rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.QuantityAdjustAfter),
						portfolioDecimal(view, result.Quantity)), view.Colors.Ink))
				}
			}
			for _, effect := range change.Effects {
				if effect.HoldingID == nil || *effect.HoldingID != item.HoldingID {
					continue
				}
				if effect.Target == string(domain.EffectTargetHoldingQuantity) && effect.Quantity != nil {
					label := i18n.QuantityAdjustAdded
					if effect.Direction == string(domain.EffectRemoved) {
						label = i18n.QuantityAdjustRemoved
					}
					rows = append(rows, view.Message(fmt.Sprintf(l.Text(label), portfolioDecimal(view, *effect.Quantity)), view.Colors.Ink))
				}
				if effect.CostUnitPrice == nil {
					continue
				}
				label := i18n.HoldingReconNewBasis
				if effect.Target == string(domain.EffectTargetHoldingQuantity) {
					label = i18n.HoldingReconAcquisitionCostValue
				}
				rows = append(rows, view.Message(fmt.Sprintf(l.Text(label),
					portfolioDecimal(view, *effect.CostUnitPrice)+" "+item.Currency), view.Colors.Ink))
			}
		}
		if total := strings.TrimSpace(d.total.Text()); total != "" {
			rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconReviewedTotal),
				portfolioDecimal(view, total)+" "+item.Currency), view.Colors.Ink))
		}
	}
	if note := strings.TrimSpace(d.note.Text()); d.check == nil && note != "" {
		rows = append(rows, view.Message(note, view.Colors.SecondaryInk))
	}
	if !c.unresolved() && d.edit.Clicked(gtx) {
		d.review, d.initialFocus = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if d.writing == nil && d.confirm.Clicked(gtx) {
		p.confirmHoldingReconciliation()
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.uncertain {
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconRetry), view.Colors.ErrorInk))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		if d.reviewFocus {
			gtx.Execute(key.FocusCmd{Tag: &d.confirm})
			d.reviewFocus = false
		}
		for _, b := range []*widget.Clickable{&d.cancel, &d.edit, &d.confirm} {
			if !c.unresolved() || (d.uncertain && d.writing == nil && b == &d.confirm) {
				view.Focus.Watch(gtx, b)
			}
		}
		return layoutTargetBalanceConfirmation(gtx, view, d.writing != nil, d.uncertain, &d.cancel, &d.edit, &d.confirm, i18n.HoldingReconConfirm)
	})
	return holdingReconColumn(gtx, rows)
}

func holdingReconciliationAccountNames(session *domain.ReconciliationSession) string {
	names := make([]string, 0, len(session.Accounts))
	for _, account := range session.Accounts {
		names = append(names, account.Name)
	}
	return strings.Join(names, " · ")
}

func holdingReconColumn(gtx layout.Context, rows []layout.Widget) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, layout.Rigid(row))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
}
