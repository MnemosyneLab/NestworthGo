package desktop

import (
	"errors"
	"slices"
	"strings"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// This table follows the Wails account wizard. The order gives the initial
// type for a new institution. Existing suggestions are ordered by directory
// sort order and ID across all of the matching types.
var accountInstitutionTypes = map[domain.AccountType][]domain.InstitutionType{
	domain.TypeBankAccount:       {domain.InstitutionBank},
	domain.TypeBrokerage:         {domain.InstitutionBrokerage, domain.InstitutionBank},
	domain.TypeInvestmentAccount: {domain.InstitutionBrokerage, domain.InstitutionBank, domain.InstitutionInsurer},
	domain.TypeCryptoExchange:    {domain.InstitutionExchange},
	domain.TypeDigitalWallet:     {domain.InstitutionBank, domain.InstitutionExchange},
	domain.TypeCreditCard:        {domain.InstitutionBank},
	domain.TypeLoan:              {domain.InstitutionBank, domain.InstitutionGovernment, domain.InstitutionEmployer},
	domain.TypePension:           {domain.InstitutionEmployer, domain.InstitutionGovernment, domain.InstitutionInsurer, domain.InstitutionBrokerage},
	domain.TypeInsurancePolicy:   {domain.InstitutionInsurer},
}

func (d *balanceAccountDraft) applyInstitutionDefault() {
	if d.scoped || d.institutionExplicit {
		return
	}
	d.institution = domain.Institution{}
	if i := slices.IndexFunc(d.institutions, func(item domain.Institution) bool {
		return slices.Contains(accountInstitutionTypes[d.kind], item.InstitutionType)
	}); i >= 0 {
		d.institution = d.institutions[i]
	}
}

func (d *balanceAccountDraft) chooseInstitution(id string) {
	d.institutionExplicit = true
	d.institution = domain.Institution{}
	for _, item := range d.institutions {
		if item.ID.String() == id {
			d.institution = item
			break
		}
	}
	if d.problem == i18n.AccountCreateChooseInstitution || d.problem == i18n.AccountCreateInstitutionUnavailable {
		d.problem = i18n.NoMessage
	}
}

type accountInstitutionResult struct {
	institution domain.Institution
	err         error
}

type accountInstitutionDraft struct {
	name                   ui.TextField
	kind                   domain.InstitutionType
	icon                   string
	customIcon, focusName  bool
	typePicker, iconPicker ui.Select
	save, cancel           widget.Clickable
	problem                i18n.MessageID
	nameLimit              int
	pending                <-chan accountInstitutionResult
}

func (d *balanceAccountDraft) startInstitution() {
	kind := domain.InstitutionOther
	if types := accountInstitutionTypes[d.kind]; len(types) > 0 {
		kind = types[0]
	}
	d.newInstitution = &accountInstitutionDraft{
		name: ui.NewTextField(true), kind: kind, icon: domain.DefaultIconForInstitutionType(kind), focusName: true,
	}
}

func (f *balanceAccountCreation) creatingInstitution() bool {
	return f != nil && f.draft != nil && f.draft.newInstitution != nil && f.draft.newInstitution.pending != nil
}

func (p *accountsPage) submitInlineInstitution() {
	f, d := p.creation, p.creation.draft.newInstitution
	if d == nil || d.pending != nil || p.ctx.Err() != nil {
		return
	}
	name := strings.TrimSpace(d.name.Text())
	if name == "" {
		d.problem, d.focusName = i18n.PreviewNameRequired, true
		return
	}
	kind, icon := d.kind, d.icon
	results := make(chan accountInstitutionResult, 1)
	d.pending, d.problem, d.nameLimit = results, i18n.NoMessage, 0
	p.workers.Go(func() {
		inst, err := f.service.CreateInstitution(p.ctx, name, kind, icon)
		results <- accountInstitutionResult{institution: inst, err: err}
		p.invalidate()
	})
}

// Once the institution writer succeeds, the account draft owns its returned
// ID. Refreshing references can fail, but the institution editor is finished.
func (p *accountsPage) pollInlineInstitution() bool {
	f := p.creation
	if !f.creatingInstitution() {
		return false
	}
	d, edit := f.draft, f.draft.newInstitution
	select {
	case result := <-edit.pending:
		edit.pending = nil
		if result.err != nil {
			edit.problem = i18n.InstitutionsSaveError
			var problem *domain.Error
			if errors.As(result.err, &problem) && problem.Field == "name" {
				edit.problem, edit.focusName = i18n.PreviewNameRequired, true
				if problem.MaxLength > 0 {
					edit.problem, edit.nameLimit = i18n.InstitutionsNameLength, problem.MaxLength
				}
			}
			return false
		}
		d.institution, d.institutionExplicit = result.institution, true
		d.savedInstitution = result.institution.Name
		d.newInstitution, d.focusAddInstitution = nil, true
		d.problem = i18n.NoMessage
		p.loadAccountCreationReferences()
		if f.sheet.step == accountInstitution {
			f.advanceFromInstitution()
		}
		return true
	default:
		return false
	}
}
