package desktop

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountCreator interface {
	Bootstrap(context.Context) (application.Bootstrap, error)
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	CreateAccount(context.Context, application.AccountInput) (domain.AccountRecord, error)
}

type accountCreateSource struct {
	bootstrap application.Bootstrap
	opening   ui.DateOptions
	err       error
}

type accountCreateResult struct {
	record domain.AccountRecord
	err    error
}

type balanceAccountReview struct {
	input  application.AccountInput
	money  domain.Money
	shares []domain.OwnershipShare
}

type balanceAccountDraft struct {
	kind                         domain.AccountType
	role                         domain.BalanceSheetRole
	mode                         domain.TrackingMode
	scoped                       bool
	institution                  domain.Institution
	institutions                 []domain.Institution
	groups                       []domain.Group
	group                        string
	openedOn                     string
	openingDates                 ui.DateOptions
	showOpeningOptions           bool
	available                    bool
	members                      []domain.Member
	owners                       []domain.MemberID
	ownerButtons                 map[domain.MemberID]*widget.Clickable
	ownerList                    widget.List
	name, opening                ui.TextField
	currency                     domain.CurrencyCode
	currencySelect, typeSelect   ui.Select
	modeSelect, groupSelect      ui.Select
	datePicker                   ui.DatePicker
	institutionSelect            ui.Select
	included                     bool
	include, next, cancel, retry widget.Clickable
	options                      widget.Clickable
	problem                      i18n.MessageID
	loadFailed                   bool
	loadedOnce                   bool
	focusName                    bool
	refs                         <-chan accountCreateSource
	cancelRefs                   context.CancelFunc
}

func (d *balanceAccountDraft) liability() bool {
	return d.role == domain.RoleLiability
}

func (d *balanceAccountDraft) shell() bool {
	return d.mode != domain.TrackingBalance
}

func (d *balanceAccountDraft) selectMode(mode domain.TrackingMode) {
	if mode == d.mode {
		return
	}
	d.mode = mode
	if !domain.IsValidAccountCombination(d.kind, d.role, mode) {
		d.kind = domain.TypeBrokerage
	}
	d.opening.Editor.SetText("")
	d.included = domain.SuggestedInclusion(d.kind, mode).IncludeInNetWorth
	d.problem = i18n.NoMessage
}

// Account creation is transient. Navigation retains the scoped browser and
// created object, never the form or a submit intent.
type balanceAccountCreation struct {
	service      accountCreator
	draft        *balanceAccountDraft
	review       *balanceAccountReview
	submission   <-chan accountCreateResult
	confirmation *ui.Confirmation
}

func (p *accountsPage) canCreateBankAccount() bool {
	if !p.canCreateAccount() || p.target.institution == "" {
		return false
	}
	institution := p.current.institutions[p.target.institution]
	return institution.ID == p.target.institution && institution.ArchivedAt == nil
}

func (p *accountsPage) canCreateAccount() bool {
	return p.creation != nil && p.target.account == "" && p.current != nil && !p.current.unavailable && p.request == nil && p.err == nil
}

func (p *accountsPage) canCreateUnscoped() bool {
	return p.canCreateAccount() && p.target.institution == ""
}

func (p *accountsPage) startBankAccount() {
	if p.canCreateBankAccount() {
		p.startBalanceAccount(domain.TypeBankAccount, domain.RoleAsset, p.current.institutions[p.target.institution])
	}
}

func (p *accountsPage) startBalanceAccount(kind domain.AccountType, role domain.BalanceSheetRole, institution domain.Institution) {
	p.startAccount(kind, role, domain.TrackingBalance, institution)
}

func (p *accountsPage) startAccount(kind domain.AccountType, role domain.BalanceSheetRole, mode domain.TrackingMode, institution domain.Institution) {
	if p.creation.draft != nil {
		return
	}
	p.creation.draft = &balanceAccountDraft{
		kind: kind, role: role, mode: mode, scoped: institution.ID != "", institution: institution,
		name: ui.NewTextField(true), opening: ui.NewTextField(true),
		included:     domain.SuggestedInclusion(kind, mode).IncludeInNetWorth,
		ownerButtons: make(map[domain.MemberID]*widget.Clickable),
		ownerList:    widget.List{List: layout.List{Axis: layout.Vertical}}, focusName: true,
	}
	p.loadAccountCreationReferences()
}

func (p *accountsPage) loadAccountCreationReferences() {
	f := p.creation
	d := f.draft
	if d.refs != nil || p.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	readOpening := d.shell()
	results := make(chan accountCreateSource, 1)
	d.refs, d.cancelRefs, d.loadFailed = results, cancel, false
	p.workers.Go(func() {
		core, err := f.service.Bootstrap(ctx)
		source := accountCreateSource{bootstrap: core, err: err}
		if err == nil && readOpening && core.Household != nil {
			origin, err := f.service.HistoryOrigin(ctx)
			source.err = err
			if err == nil && origin != nil {
				loc, err := time.LoadLocation(origin.Timezone)
				source.err = err
				if err == nil {
					today := p.clock().In(loc).Format(time.DateOnly)
					source.opening = ui.DateOptions{Today: today, Min: origin.StartedAt.In(loc).Format(time.DateOnly), Max: today, Clearable: true}
				}
			}
		}
		if ctx.Err() == nil {
			results <- source
			p.invalidate()
		}
	})
}

func (d *balanceAccountDraft) applyReferences(source accountCreateSource) {
	d.loadFailed = source.err != nil
	if d.loadFailed {
		return
	}
	d.available = false
	core := source.bootstrap
	d.openingDates = source.opening
	d.groups = slices.DeleteFunc(slices.Clone(core.Groups), func(group domain.Group) bool { return group.ArchivedAt != nil })
	d.institutions = slices.DeleteFunc(slices.Clone(core.Institutions), func(item domain.Institution) bool { return item.ArchivedAt != nil })
	if core.Household != nil {
		d.available = !d.scoped
		if index := d.institutionIndex(); index >= 0 {
			d.institution, d.available = d.institutions[index], true
		}
	}
	d.members = slices.DeleteFunc(slices.Clone(core.Members), func(member domain.Member) bool { return member.ArchivedAt != nil })
	d.owners = slices.DeleteFunc(d.owners, func(id domain.MemberID) bool {
		return !slices.ContainsFunc(d.members, func(member domain.Member) bool { return member.ID == id })
	})
	if !d.loadedOnce {
		if core.Household != nil {
			d.currency = core.Household.BaseCurrency
		}
		if len(d.members) == 1 {
			d.owners = []domain.MemberID{d.members[0].ID}
		}
		if d.shell() && len(d.groups) != 0 {
			first := slices.MinFunc(d.groups, func(a, b domain.Group) int {
				return cmp.Or(cmp.Compare(a.SortOrder, b.SortOrder), cmp.Compare(a.ID, b.ID))
			})
			d.group = first.ID.String()
		}
		d.loadedOnce = true
	}
}

func (d *balanceAccountDraft) institutionIndex() int {
	return slices.IndexFunc(d.institutions, func(item domain.Institution) bool { return item.ID == d.institution.ID })
}

func (f *balanceAccountCreation) discard() {
	if f == nil || f.draft == nil {
		return
	}
	if f.draft.cancelRefs != nil {
		f.draft.cancelRefs()
	}
	f.draft, f.review, f.confirmation = nil, nil, nil
}

func (p *accountsPage) pollAccountCreation() domain.AccountRecord {
	f := p.creation
	if f == nil || f.draft == nil {
		return domain.AccountRecord{}
	}
	d := f.draft
	select {
	case source := <-d.refs:
		d.cancelRefs()
		d.refs, d.cancelRefs = nil, nil
		d.applyReferences(source)
	default:
	}
	select {
	case result := <-f.submission:
		f.submission, f.review = nil, nil
		if result.err == nil {
			f.discard()
			return result.record
		}
		d.problem = accountCreationProblem(result.err)
		if d.shell() && d.problem == i18n.AccountCreateWriteFailed {
			d.problem = i18n.AccountCreateAssetWriteFailed
		}
		if d.problem == i18n.AccountCreateGroupUnavailable || d.problem == i18n.AccountCreateDateInvalid {
			d.showOpeningOptions = true
		}
		if d.problem == i18n.AccountCreateInstitutionUnavailable || d.problem == i18n.AccountCreateOwnersChanged || d.problem == i18n.AccountCreateGroupUnavailable || d.problem == i18n.AccountCreateDateInvalid {
			p.loadAccountCreationReferences()
		}
		if !d.scoped && !d.shell() {
			switch d.problem {
			case i18n.AccountCreateWriteFailed:
				d.problem = i18n.AccountCreateAssetWriteFailed
				if d.liability() {
					d.problem = i18n.AccountCreateLiabilityWriteFailed
				}
			case i18n.AccountCreateInstitutionUnavailable:
				d.problem = i18n.AccountCreateChooseInstitution
			}
		}
	default:
	}
	return domain.AccountRecord{}
}

func accountCreationProblem(err error) i18n.MessageID {
	var problem *domain.Error
	if errors.As(err, &problem) {
		switch problem.Field {
		case "name":
			if problem.MaxLength > 0 {
				return i18n.AccountCreateNameTooLong
			}
			return i18n.AccountCreateNameRequired
		case "currency", "defaultCurrency":
			return i18n.AccountCreateCurrencyRequired
		case "amount", "initialAmount":
			return i18n.AccountCreateAmountInvalid
		case "accountType":
			return i18n.AccountTypeIncompatible
		case "groupId":
			return i18n.AccountCreateGroupUnavailable
		case "openedOn", "effectiveAt":
			return i18n.AccountCreateDateInvalid
		case "institutionId":
			return i18n.AccountCreateInstitutionUnavailable
		case "ownership", "memberId":
			return i18n.AccountCreateOwnersChanged
		}
	}
	return i18n.AccountCreateWriteFailed
}

func (f *balanceAccountCreation) prepareReview() {
	d := f.draft
	if d.refs != nil || d.loadFailed || !d.available || f.review != nil {
		return
	}
	if !domain.IsValidAccountCombination(d.kind, d.role, d.mode) {
		d.problem = i18n.AccountTypeIncompatible
		return
	}
	if d.institution.ID != "" && d.institutionIndex() < 0 {
		d.problem = i18n.AccountCreateChooseInstitution
		return
	}
	if d.group != "" && !slices.ContainsFunc(d.groups, func(group domain.Group) bool { return group.ID.String() == d.group }) {
		d.showOpeningOptions, d.problem = true, i18n.AccountCreateGroupUnavailable
		return
	}
	var openedOn *string
	if d.openedOn != "" {
		if _, valid := ui.ParseCivilDay(d.openedOn); !valid || d.openingDates.Min == "" || d.openedOn < d.openingDates.Min || d.openedOn > d.openingDates.Max {
			d.showOpeningOptions, d.problem = true, i18n.AccountCreateDateInvalid
			return
		}
		day := d.openedOn
		openedOn = &day
	}
	name := strings.TrimSpace(d.name.Text())
	if name == "" {
		d.problem, d.focusName = i18n.AccountCreateNameRequired, true
		return
	}
	if utf8.RuneCountInString(name) > 120 {
		d.problem, d.focusName = i18n.AccountCreateNameTooLong, true
		return
	}
	currency, err := domain.ParseSupportedCurrency(d.currency.String())
	if err != nil {
		d.problem = i18n.AccountCreateCurrencyRequired
		return
	}
	amount := strings.TrimSpace(d.opening.Text())
	var money domain.Money
	if d.mode == domain.TrackingHoldings {
		if amount != "" {
			d.problem = i18n.AccountCreateHoldingsAmountInvalid
			return
		}
	} else {
		if amount == "" {
			amount = "0"
		}
		money, err = domain.ParseMoney(amount, currency)
		if err != nil {
			d.problem = i18n.AccountCreateAmountInvalid
			return
		}
		amount = money.CanonicalAmount()
	}
	owners, err := domain.EqualOwnership(d.owners)
	if err != nil {
		d.problem = i18n.AccountCreateOwnersRequired
		return
	}
	defaults := domain.SuggestedInclusion(d.kind, d.mode)
	f.review = &balanceAccountReview{
		money: money, shares: owners.Shares(),
		input: application.AccountInput{
			Name: name, AccountType: d.kind.String(), BalanceSheetRole: d.role.String(), TrackingMode: string(d.mode),
			InstitutionID: d.institution.ID.String(), InstitutionIDSet: d.institution.ID != "",
			GroupID: d.group, GroupIDSet: d.group != "", OpenedOn: openedOn, OpenedOnSet: openedOn != nil,
			DefaultCurrency: currency.String(), OwnerIDs: slices.Clone(d.owners), InitialAmount: amount,
			IncludeInNetWorth: d.included, IncludeInNetWorthSet: true,
			IncludeInPortfolio: defaults.IncludeInPortfolio, IncludeInPortfolioSet: true,
			IncludeInLiquidAssets: defaults.IncludeInLiquidAssets, IncludeInLiquidAssetsSet: true,
		},
	}
	d.problem = i18n.NoMessage
	f.confirmation = new(ui.Confirmation)
	f.confirmation.Open()
}

func (p *accountsPage) submitNewAccount() {
	f := p.creation
	if f.review == nil || f.submission != nil || p.ctx.Err() != nil {
		return
	}
	input := f.review.input
	results := make(chan accountCreateResult, 1)
	f.submission = results
	p.workers.Go(func() {
		record, err := f.service.CreateAccount(p.ctx, input)
		results <- accountCreateResult{record: record, err: err}
		p.invalidate()
	})
}
