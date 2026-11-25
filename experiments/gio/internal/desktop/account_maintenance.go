package desktop

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

type accountMaintainer interface {
	UpdateAccount(context.Context, domain.AccountID, application.AccountInput) (domain.AccountRecord, error)
	ArchiveAccount(context.Context, domain.AccountID, bool) error
}

type accountSettingsDraft struct {
	accountOwnershipDraft
	record                                     domain.AccountRecord
	kind                                       domain.AccountType
	name                                       ui.TextField
	institution, group, icon                   string
	included, focusName                        bool
	institutionPicker, groupPicker, iconPicker ui.Select
	typePicker                                 ui.Select
	include, save, cancel                      widget.Clickable
}

type accountArchiveReview struct {
	archive bool
	name    string
	dialog  ui.Confirmation
}

type accountMaintenanceResult struct {
	message i18n.MessageID
	err     error
}

type accountMaintenance struct {
	service        accountMaintainer
	draft          *accountSettingsDraft
	review         *accountArchiveReview
	pending        <-chan accountMaintenanceResult
	edit, archive  widget.Clickable
	focus          *widget.Clickable
	success, issue i18n.MessageID
}

func accountMaintenanceSupported(account domain.Account) bool {
	return domain.IsValidAccountCombination(account.AccountType, account.BalanceSheetRole, account.TrackingMode)
}

func compatibleAccountType(account domain.Account, kind domain.AccountType) bool {
	if account.TrackingMode == domain.TrackingHoldings && (kind == domain.TypeCashOnHand) != (account.AccountType == domain.TypeCashOnHand) {
		return false
	}
	return domain.IsValidAccountCombination(kind, account.BalanceSheetRole, account.TrackingMode)
}

func canEditAccount(record domain.AccountRecord) bool {
	return record.Account.ArchivedAt == nil && (record.LatestValue != nil || record.Account.TrackingMode == domain.TrackingHoldings)
}

func (p *accountsPage) freshDetail() *accountRow {
	if p.target.account == "" || p.current == nil || p.current.unavailable || len(p.current.rows) != 1 || p.request != nil || p.err != nil {
		return nil
	}
	return &p.current.rows[0]
}

func refValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func (m *accountMaintenance) startEdit(record domain.AccountRecord) {
	a := record.Account
	d := &accountSettingsDraft{
		record: record, kind: a.AccountType, name: ui.NewTextField(true), focusName: true, institution: refValue(a.InstitutionID),
		group: refValue(a.GroupID), icon: refValue(a.IconKey), included: a.IncludeInNetWorth,
		accountOwnershipDraft: newAccountOwnershipDraft(),
	}
	d.name.Editor.SetText(a.Name)
	shares := record.Ownership.Shares()
	for _, share := range shares {
		d.owners = append(d.owners, share.MemberID)
		d.owner(share.MemberID).percent.Editor.SetText(format.ShareBPS(share.ShareBPS))
	}
	equal, err := domain.EqualOwnership(d.owners)
	d.custom = err == nil && !slices.Equal(shares, equal.Shares())
	m.draft, m.issue, m.success = d, i18n.NoMessage, i18n.NoMessage
}

func (d *accountSettingsDraft) input() (application.AccountInput, i18n.MessageID) {
	a := d.record.Account
	if !compatibleAccountType(a, d.kind) {
		return application.AccountInput{}, i18n.AccountTypeIncompatible
	}
	name := strings.TrimSpace(d.name.Text())
	if name == "" {
		d.focusName = true
		return application.AccountInput{}, i18n.AccountCreateNameRequired
	}
	if utf8.RuneCountInString(name) > 120 {
		d.focusName = true
		return application.AccountInput{}, i18n.AccountCreateNameTooLong
	}
	ownership, issue := d.ownership()
	if issue != i18n.NoMessage {
		return application.AccountInput{}, issue
	}
	input := application.AccountInput{
		InstitutionID: d.institution, InstitutionIDSet: d.institution != refValue(a.InstitutionID),
		GroupID: d.group, GroupIDSet: d.group != refValue(a.GroupID),
		IconKey: d.icon, IconKeySet: d.icon != refValue(a.IconKey),
		IncludeInNetWorth: d.included, IncludeInNetWorthSet: d.included != a.IncludeInNetWorth,
	}
	if name != a.Name {
		input.Name = name
	}
	if d.kind != a.AccountType {
		input.AccountType = d.kind.String()
	}
	if shares := ownership.Shares(); !slices.Equal(shares, d.record.Ownership.Shares()) {
		input.Ownership = shares
	}
	return input, i18n.NoMessage
}

func (m *accountMaintenance) startArchive(account domain.Account) {
	m.review = &accountArchiveReview{archive: account.ArchivedAt == nil, name: account.Name}
	m.review.dialog.Open()
	m.issue, m.success = i18n.NoMessage, i18n.NoMessage
}

func (p *accountsPage) submitAccountMaintenance(archive *bool, input application.AccountInput) {
	m := p.maintenance
	if p.freshDetail() == nil || m.pending != nil || p.ctx.Err() != nil {
		return
	}
	id := p.target.account
	result := make(chan accountMaintenanceResult, 1)
	m.pending, m.issue = result, i18n.NoMessage
	p.workers.Go(func() {
		result <- p.writeAccountMaintenance(id, archive, input)
		p.invalidate()
	})
}

func (p *accountsPage) writeAccountMaintenance(id domain.AccountID, archive *bool, input application.AccountInput) accountMaintenanceResult {
	// The detail may be older than the draft. Check eligibility before the
	// single shared mutation, then let the service validate its references.
	records, err := p.service.ListAccounts(p.ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{id}, IncludeArchived: true})
	if err != nil {
		return accountMaintenanceResult{err: err}
	}
	if len(records) != 1 || records[0].Account.ID != id || !accountMaintenanceSupported(records[0].Account) {
		return accountMaintenanceResult{message: i18n.AccountMaintenanceChanged}
	}
	record := records[0]
	archived := record.Account.ArchivedAt != nil
	if archive != nil {
		message := i18n.AccountMaintenanceRestored
		if *archive {
			message = i18n.AccountMaintenanceArchived
		}
		if archived != *archive {
			err = p.maintenance.service.ArchiveAccount(p.ctx, id, *archive)
		}
		return accountMaintenanceResult{message: message, err: err}
	}
	if !canEditAccount(record) {
		return accountMaintenanceResult{message: i18n.AccountMaintenanceChanged}
	}
	if input.AccountType != "" && !compatibleAccountType(record.Account, domain.AccountType(input.AccountType)) {
		return accountMaintenanceResult{message: i18n.AccountTypeIncompatible}
	}
	if input.AccountType != "" || input.Name != "" || input.InstitutionIDSet || input.GroupIDSet || input.IconKeySet || input.IncludeInNetWorthSet || input.Ownership != nil {
		_, err = p.maintenance.service.UpdateAccount(p.ctx, id, input)
	}
	return accountMaintenanceResult{message: i18n.AccountMaintenanceSaved, err: err}
}

func maintenanceProblem(err error) i18n.MessageID {
	var problem *domain.Error
	if errors.As(err, &problem) {
		switch problem.Code {
		case domain.ErrManagedPosition:
			return i18n.AccountMaintenanceArchiveBlocked
		case domain.ErrNotFound:
			return i18n.AccountMaintenanceChanged
		}
		switch problem.Field {
		case "accountType", "balanceSheetRole", "trackingMode":
			return i18n.AccountTypeIncompatible
		case "institutionId":
			return i18n.AccountCreateInstitutionUnavailable
		case "groupId":
			return i18n.AccountMaintenanceGroupUnavailable
		case "ownership":
			return i18n.AccountCreateOwnersChanged
		}
	}
	return i18n.AccountMaintenanceWriteFailed
}

func (p *accountsPage) pollAccountMaintenance() bool {
	m := p.maintenance
	if m == nil {
		return false
	}
	select {
	case result := <-m.pending:
		m.pending = nil
		if m.review != nil {
			m.focus, m.review = &m.archive, nil
		}
		if result.err != nil {
			m.issue = maintenanceProblem(result.err)
			switch m.issue {
			case i18n.AccountMaintenanceChanged:
				m.draft = nil
				p.read()
			case i18n.AccountCreateInstitutionUnavailable, i18n.AccountMaintenanceGroupUnavailable, i18n.AccountCreateOwnersChanged:
				p.read()
			}
			return false
		}
		if result.message == i18n.AccountMaintenanceChanged {
			m.draft, m.issue = nil, result.message
			p.read()
			return false
		}
		if result.message == i18n.AccountTypeIncompatible {
			m.issue = result.message
			return false
		}
		if m.draft != nil {
			m.focus, m.draft = &m.edit, nil
		}
		if p.createdID == p.target.account {
			p.createdID = ""
		}
		m.success = result.message
		return true
	default:
		return false
	}
}

func (p *accountsPage) refreshAccountSettings() {
	m := p.maintenance
	if m == nil || m.draft == nil {
		return
	}
	row := p.freshDetail()
	if row == nil || !accountMaintenanceSupported(row.record.Account) || !canEditAccount(row.record) {
		m.draft, m.issue = nil, i18n.AccountMaintenanceChanged
		return
	}
	d := m.draft
	retained := row.record.Ownership.Shares()
	d.owners = slices.DeleteFunc(d.owners, func(id domain.MemberID) bool {
		member := p.current.members[id]
		return (member.ID == "" || member.ArchivedAt != nil) && !slices.ContainsFunc(retained, func(share domain.OwnershipShare) bool {
			return share.MemberID == id
		})
	})
}
