package desktop

import (
	"fmt"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

var supportedCurrencyChoices = func() []ui.Choice {
	var choices []ui.Choice
	for _, currency := range domain.SupportedCurrencies() {
		code := currency.String()
		choices = append(choices, ui.Choice{ID: code, Label: code})
	}
	return choices
}()

func accountTypeChoices(l i18n.Language, role domain.BalanceSheetRole, mode domain.TrackingMode) []ui.Choice {
	var result []ui.Choice
	for _, option := range domain.LegalAccountCombinations() {
		if option.BalanceSheetRole == role && option.TrackingMode == mode {
			result = append(result, ui.Choice{ID: option.AccountType.String(), Label: l.Text(accountTypes[option.AccountType])})
		}
	}
	return result
}

func accountTrackingLabel(mode domain.TrackingMode, kind domain.AccountType) i18n.MessageID {
	if mode == domain.TrackingBalance {
		return i18n.AccountTrackingBalance
	}
	if mode == domain.TrackingManualValue {
		return i18n.AccountTrackingManual
	}
	if kind == domain.TypeCashOnHand {
		return i18n.AccountTrackingCashOnly
	}
	return i18n.AccountTrackingHoldings
}

func (p *accountsPage) layoutAccountCreationReview(gtx layout.Context, view ui.View) {
	f, l := p.creation, view.Language
	r, d := f.review, f.draft
	owners := make([]string, 0, min(3, len(r.shares)))
	for _, share := range r.shares[:min(3, len(r.shares))] {
		if i := slices.IndexFunc(d.members, func(member domain.Member) bool { return member.ID == share.MemberID }); i >= 0 {
			owners = append(owners, d.members[i].Name+" · "+format.ShareBPS(share.ShareBPS))
		}
	}
	if len(r.shares) > 3 {
		owners = append(owners, fmt.Sprintf(l.Text(i18n.AccountCreateMoreOwners), len(r.shares)-3))
	}
	include := i18n.AccountCreateExcluded
	if r.input.IncludeInNetWorth {
		include = i18n.AccountCreateIncluded
	}
	title, confirm, amount, saving := i18n.AccountCreateReviewTitle, i18n.AccountCreateConfirm, i18n.AccountCreateReviewBalance, i18n.AccountCreateSaving
	institution := fmt.Sprintf(l.Text(i18n.AccountInstitution), d.institution.Name)
	lines := []string{r.input.Name}
	if d.shell() {
		title, confirm, amount, saving = i18n.AccountCreateAssetReview, i18n.AccountCreateAssetConfirm, i18n.AccountCreateReviewValuation, i18n.SettingsPending
	} else if d.liability() {
		title, confirm, amount, saving = i18n.AccountCreateLiabilityReview, i18n.AccountCreateLiabilityConfirm, i18n.AccountCreateReviewOwed, i18n.SettingsPending
	} else if !d.scoped {
		title, confirm, saving = i18n.AccountCreateAssetReview, i18n.AccountCreateAssetConfirm, i18n.SettingsPending
	}
	if !d.scoped || d.shell() {
		lines = append(lines, l.Text(accountTypes[d.kind]))
		if d.institution.ID == "" {
			institution = l.Text(i18n.AccountNoInstitution)
		}
	}
	lines = append(lines, institution)
	if d.shell() {
		lines = append(lines, l.Text(accountTrackingLabel(d.mode, d.kind)))
	}
	if i := slices.IndexFunc(d.groups, func(group domain.Group) bool { return group.ID.String() == r.input.GroupID }); i >= 0 {
		lines = append(lines, fmt.Sprintf(l.Text(i18n.AccountGroup), d.groups[i].Name))
	}
	if day, valid := ui.ParseCivilDay(refValue(r.input.OpenedOn)); valid {
		lines = append(lines, fmt.Sprintf(l.Text(i18n.AccountCreateReviewDate), ui.FormatDay(day, l, view.DateFormat)))
	}
	if r.input.IconKey != "" {
		options, value := accountIconOptions(view, r.input.IconKey)
		label := options.ValueLabel
		if label == "" {
			label = ui.ChoiceLabel(view, options.Items, value)
		}
		lines = append(lines, l.Text(i18n.DirectoryIcon)+" · "+label)
	}
	if d.mode == domain.TrackingHoldings {
		lines = append(lines, fmt.Sprintf(l.Text(i18n.AccountCreateReviewCurrency), r.input.DefaultCurrency), l.Text(i18n.AccountCreateHoldingsEmpty))
	} else {
		lines = append(lines, fmt.Sprintf(l.Text(amount), r.money.CanonicalAmount()+" "+r.money.Currency().String()))
	}
	body := strings.Join(append(lines,
		fmt.Sprintf(l.Text(i18n.AccountOwners), strings.Join(owners, " / ")),
		l.Text(include),
	), "\n")
	problem := ""
	if f.submission != nil {
		problem = l.Text(saving)
	}
	f.confirmation.Layout(gtx, view, ui.ConfirmationOptions{
		Title: l.Text(title), Body: body, Busy: f.submission != nil, Problem: problem,
		Cancel: l.Text(i18n.AccountCreateBackToDetails), Confirm: l.Text(confirm),
	}, func() {
		f.review = nil
		gtx.Execute(op.InvalidateCmd{})
	}, p.submitNewAccount)
}
