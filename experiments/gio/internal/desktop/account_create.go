package desktop

import (
	"context"
	"errors"
	"slices"
	"strings"
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
	CreateAccount(context.Context, application.AccountInput) (domain.AccountRecord, error)
}

type accountCreateSource struct {
	bootstrap application.Bootstrap
	err       error
}

type accountCreateResult struct {
	record domain.AccountRecord
	err    error
}

type bankAccountReview struct {
	input  application.AccountInput
	money  domain.Money
	shares []domain.OwnershipShare
}

type bankAccountDraft struct {
	institution                  domain.Institution
	available                    bool
	members                      []domain.Member
	owners                       []domain.MemberID
	ownerButtons                 map[domain.MemberID]*widget.Clickable
	ownerList                    widget.List
	name, opening                ui.TextField
	currency                     domain.CurrencyCode
	currencySelect               ui.Select
	included                     bool
	include, next, cancel, retry widget.Clickable
	problem                      i18n.MessageID
	loadFailed                   bool
	loadedOnce                   bool
	focusName                    bool
	refs                         <-chan accountCreateSource
	cancelRefs                   context.CancelFunc
}

// Account creation is transient. Navigation retains the scoped browser and
// created object, never the form or a submit intent.
type bankAccountCreation struct {
	service      accountCreator
	draft        *bankAccountDraft
	review       *bankAccountReview
	submission   <-chan accountCreateResult
	confirmation *ui.Confirmation
}

func (p *accountsPage) canCreateBankAccount() bool {
	if p.creation == nil || p.target.institution == "" || p.current == nil || p.current.unavailable || p.request != nil || p.err != nil {
		return false
	}
	institution := p.current.institutions[p.target.institution]
	return institution.ID == p.target.institution && institution.ArchivedAt == nil
}

func (p *accountsPage) startBankAccount() {
	if !p.canCreateBankAccount() || p.creation.draft != nil {
		return
	}
	p.creation.draft = &bankAccountDraft{
		institution: p.current.institutions[p.target.institution],
		name:        ui.NewTextField(true), opening: ui.NewTextField(true),
		included:     domain.SuggestedInclusion(domain.TypeBankAccount, domain.TrackingBalance).IncludeInNetWorth,
		ownerButtons: make(map[domain.MemberID]*widget.Clickable),
		ownerList:    widget.List{List: layout.List{Axis: layout.Vertical}}, focusName: true,
	}
	p.loadBankAccountReferences()
}

func (p *accountsPage) loadBankAccountReferences() {
	f := p.creation
	d := f.draft
	if d.refs != nil || p.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	results := make(chan accountCreateSource, 1)
	d.refs, d.cancelRefs, d.loadFailed = results, cancel, false
	p.workers.Go(func() {
		core, err := f.service.Bootstrap(ctx)
		if ctx.Err() == nil {
			results <- accountCreateSource{bootstrap: core, err: err}
			p.invalidate()
		}
	})
}

func (d *bankAccountDraft) applyReferences(source accountCreateSource) {
	d.loadFailed = source.err != nil
	if d.loadFailed {
		return
	}
	d.available = false
	core := source.bootstrap
	for _, institution := range core.Institutions {
		if institution.ID == d.institution.ID && institution.ArchivedAt == nil {
			d.institution, d.available = institution, core.Household != nil
			break
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
		d.loadedOnce = true
	}
}

func (f *bankAccountCreation) discard() {
	if f == nil || f.draft == nil {
		return
	}
	if f.draft.cancelRefs != nil {
		f.draft.cancelRefs()
	}
	f.draft, f.review, f.confirmation = nil, nil, nil
}

func (p *accountsPage) pollBankAccount() domain.AccountRecord {
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
		d.problem = bankAccountProblem(result.err)
		if d.problem == i18n.AccountCreateInstitutionUnavailable || d.problem == i18n.AccountCreateOwnersChanged {
			p.loadBankAccountReferences()
		}
	default:
	}
	return domain.AccountRecord{}
}

func bankAccountProblem(err error) i18n.MessageID {
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
		case "institutionId":
			return i18n.AccountCreateInstitutionUnavailable
		case "ownership", "memberId":
			return i18n.AccountCreateOwnersChanged
		}
	}
	return i18n.AccountCreateWriteFailed
}

func (f *bankAccountCreation) prepareReview() {
	d := f.draft
	if d.refs != nil || d.loadFailed || !d.available || f.review != nil {
		return
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
	if amount == "" {
		amount = "0"
	}
	money, err := domain.ParseMoney(amount, currency)
	if err != nil {
		d.problem = i18n.AccountCreateAmountInvalid
		return
	}
	owners, err := domain.EqualOwnership(d.owners)
	if err != nil {
		d.problem = i18n.AccountCreateOwnersRequired
		return
	}
	defaults := domain.SuggestedInclusion(domain.TypeBankAccount, domain.TrackingBalance)
	f.review = &bankAccountReview{
		money: money, shares: owners.Shares(),
		input: application.AccountInput{
			Name: name, AccountType: domain.TypeBankAccount.String(), BalanceSheetRole: domain.RoleAsset.String(), TrackingMode: string(domain.TrackingBalance),
			InstitutionID: d.institution.ID.String(), InstitutionIDSet: true,
			DefaultCurrency: currency.String(), OwnerIDs: slices.Clone(d.owners), InitialAmount: money.CanonicalAmount(),
			IncludeInNetWorth: d.included, IncludeInNetWorthSet: true,
			IncludeInPortfolio: defaults.IncludeInPortfolio, IncludeInPortfolioSet: true,
			IncludeInLiquidAssets: defaults.IncludeInLiquidAssets, IncludeInLiquidAssetsSet: true,
		},
	}
	d.problem = i18n.NoMessage
	f.confirmation = new(ui.Confirmation)
	f.confirmation.Open()
}

func (p *accountsPage) submitBankAccount() {
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
