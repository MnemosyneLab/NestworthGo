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

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const noReconciliationInstitution = "-"

type reconciliationSelection struct {
	selected                                                  map[string]bool
	checks                                                    map[domain.AccountID]*widget.Bool
	evidence                                                  map[string]*holdingEvidenceDisclosure
	owner, kind, institution                                  string
	ownerPick, typePick, institutionPick                      ui.Select
	clear, selectVisible, deselectVisible, deselectAll, retry widget.Clickable
}

func (s *reconciliationSelection) includes(id domain.AccountID) bool {
	return s.selected == nil || s.selected[id.String()]
}

func (s *reconciliationSelection) chosen(accounts []domain.AccountRecord) []string {
	var ids []string
	for _, row := range accounts {
		if s.includes(row.Account.ID) {
			ids = append(ids, row.Account.ID.String())
		}
	}
	return ids
}

func (s *reconciliationSelection) freeze(ids []string) {
	s.selected = make(map[string]bool, len(ids))
	for _, id := range ids {
		s.selected[id] = true
	}
}

func (s *reconciliationSelection) visible(accounts []domain.AccountRecord) []domain.AccountRecord {
	var rows []domain.AccountRecord
	for _, row := range accounts {
		a := row.Account
		institution := noReconciliationInstitution
		if a.InstitutionID != nil {
			institution = a.InstitutionID.String()
		}
		if s.kind != "" && a.AccountType.String() != s.kind || s.institution != "" && institution != s.institution {
			continue
		}
		if s.owner != "" && !slices.ContainsFunc(row.Ownership.Shares(), func(share domain.OwnershipShare) bool {
			return share.MemberID.String() == s.owner
		}) {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func reconciliationScopeOverlaps(sessions []domain.ReconciliationSession, ids []string) bool {
	for _, session := range sessions {
		if session.Status == "completed" {
			continue
		}
		for _, id := range ids {
			if slices.Contains(session.AccountIDs, id) {
				return true
			}
		}
	}
	return false
}

func (p *tasksPage) reconciliationSelectionRows(view ui.View, origin *image.Point, d *holdingReconDraft, ready bool) []layout.Widget {
	s, directory, l := d.selection, p.holdingRecon.directory, view.Language
	var rows []layout.Widget
	if directory.ownersFailed {
		rows = append(rows, view.Message(l.Text(i18n.DirectoryLoadError), view.Colors.SecondaryInk),
			reconciliationSelectionButton(view, &s.retry, i18n.HistoryRetry, ready, p.read))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		if !ready {
			gtx = gtx.Disabled()
		}
		if d.initialFocus && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: &s.ownerPick.Trigger})
			d.initialFocus = false
		}
		view.Origin = *origin
		return s.filters(gtx, view, directory)
	}, func(gtx layout.Context) layout.Dimensions {
		rows := s.visible(directory.accounts)
		chosen, visibleChosen := s.chosen(directory.accounts), s.chosen(rows)
		items := []layout.Widget{
			reconciliationSelectionButton(view, &s.clear, i18n.ReconciliationClearFilters, ready && (s.owner != "" || s.kind != "" || s.institution != ""), func() {
				s.owner, s.kind, s.institution = "", "", ""
			}),
			reconciliationSelectionButton(view, &s.selectVisible, i18n.ReconciliationSelectVisible, ready && len(visibleChosen) < len(rows), func() {
				s.freeze(chosen)
				for _, row := range rows {
					s.selected[row.Account.ID.String()] = true
				}
			}),
			reconciliationSelectionButton(view, &s.deselectVisible, i18n.ReconciliationDeselectVisible, ready && len(visibleChosen) != 0, func() {
				s.freeze(chosen)
				for _, row := range rows {
					delete(s.selected, row.Account.ID.String())
				}
			}),
			reconciliationSelectionButton(view, &s.deselectAll, i18n.ReconciliationDeselectAll, ready && len(chosen) != 0, func() {
				s.freeze(nil)
			}),
		}
		return (ui.Wrap{Gap: 6}).Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
			return items[i](gtx)
		})
	}, func(gtx layout.Context) layout.Dimensions {
		visible := s.visible(directory.accounts)
		chosen := s.chosen(directory.accounts)
		visibleChosen := len(s.chosen(visible))
		summary := fmt.Sprintf(l.Text(i18n.ReconciliationSelectionSummary), len(chosen), len(directory.accounts), visibleChosen, len(visible), len(chosen)-visibleChosen)
		return view.Message(summary, view.Colors.SecondaryInk)(gtx)
	}, func(gtx layout.Context) layout.Dimensions {
		visible := s.visible(directory.accounts)
		if len(visible) == 0 {
			return view.Message(l.Text(i18n.ReconciliationNoMatches), view.Colors.SecondaryInk)(gtx)
		}
		if !ready {
			gtx = gtx.Disabled()
		}
		return (ui.Grid{MinColumn: 245, MaxColumns: 2, Gap: 8}).Layout(gtx, view, len(visible), func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			return s.accountCheck(gtx, view, directory, visible[i])
		})
	}, func(gtx layout.Context) layout.Dimensions {
		if !reconciliationScopeOverlaps(directory.sessions, s.chosen(directory.accounts)) {
			return layout.Dimensions{}
		}
		return view.Message(l.Text(i18n.ReconciliationOverlap), view.Colors.SecondaryInk)(gtx)
	})
	return rows
}

func reconciliationStartEvidence(view ui.View, s *reconciliationSelection, accounts []domain.ReconciliationAccount) []layout.Widget {
	if s.evidence == nil {
		s.evidence = make(map[string]*holdingEvidenceDisclosure)
	}
	l := view.Language
	var rows []layout.Widget
	for _, account := range accounts {
		disclosure := s.evidence[account.AccountID]
		if disclosure == nil {
			disclosure = new(holdingEvidenceDisclosure)
			s.evidence[account.AccountID] = disclosure
		}
		show := fmt.Sprintf(l.Text(i18n.ReconciliationShowScope), account.Name)
		hide := fmt.Sprintf(l.Text(i18n.ReconciliationHideScope), account.Name)
		rows = append(rows, disclosure.toggle(view, show, hide))
		if disclosure.open {
			for _, item := range account.Items {
				rows = append(rows, holdingBatchItem(view, item))
			}
		}
	}
	return rows
}

func reconciliationSelectionButton(view ui.View, button *widget.Clickable, label i18n.MessageID, enabled bool, action func()) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if enabled && button.Clicked(gtx) {
			action()
			gtx.Execute(op.InvalidateCmd{})
		}
		return taskPagerButton(view, button, label, enabled)(gtx)
	}
}

func (s *reconciliationSelection) filters(gtx layout.Context, view ui.View, directory holdingReconDirectory) layout.Dimensions {
	l := view.Language
	all := ui.Choice{Label: l.Text(i18n.ReconciliationAllFilters)}
	owners, kinds, institutions := []ui.Choice{all}, []ui.Choice{all}, []ui.Choice{all}
	owned := make(map[domain.MemberID]bool)
	types := make(map[domain.AccountType]bool)
	banks := make(map[domain.InstitutionID]bool)
	for _, row := range directory.accounts {
		for _, share := range row.Ownership.Shares() {
			owned[share.MemberID] = true
		}
		a := row.Account
		types[a.AccountType] = true
		if a.InstitutionID != nil && !banks[*a.InstitutionID] {
			name := row.InstitutionName
			if name == "" {
				name = l.Text(i18n.AccountReferenceUnavailable)
			}
			banks[*a.InstitutionID] = true
			institutions = append(institutions, ui.Choice{ID: a.InstitutionID.String(), Label: name})
		}
	}
	for _, owner := range directory.members {
		if owned[owner.ID] {
			owners = append(owners, ui.Choice{ID: owner.ID.String(), Label: owner.Name})
		}
	}
	var orderedTypes []domain.AccountType
	for kind := range types {
		orderedTypes = append(orderedTypes, kind)
	}
	slices.Sort(orderedTypes)
	for _, kind := range orderedTypes {
		kinds = append(kinds, ui.Choice{ID: kind.String(), Label: l.Text(accountTypes[kind])})
	}
	institutions = append(institutions, ui.Choice{ID: noReconciliationInstitution, Label: l.Text(i18n.AccountNoInstitution)})
	fields := []struct {
		label   i18n.MessageID
		picker  *ui.Select
		value   *string
		options []ui.Choice
	}{
		{i18n.ReconciliationOwner, &s.ownerPick, &s.owner, owners},
		{i18n.PreviewAccountType, &s.typePick, &s.kind, kinds},
		{i18n.ReconciliationInstitution, &s.institutionPick, &s.institution, institutions},
	}
	return (ui.Grid{MinColumn: 175, MaxColumns: 3, Gap: 8}).Layout(gtx, view, len(fields), func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
		f := fields[i]
		f.picker.Layout(gtx, view, ui.PickerField{Label: f.label, Origin: view.Origin, Disabled: !gtx.Enabled()},
			ui.SelectOptions{Items: f.options}, *f.value, func(id string) {
				*f.value = id
				gtx.Execute(op.InvalidateCmd{})
			})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	})
}

func (s *reconciliationSelection) accountCheck(gtx layout.Context, view ui.View, directory holdingReconDirectory, row domain.AccountRecord) layout.Dimensions {
	a, l := row.Account, view.Language
	box := s.checks[a.ID]
	if box == nil {
		box = new(widget.Bool)
		s.checks[a.ID] = box
	}
	selected := s.includes(a.ID)
	box.Value = selected
	var detail []string
	for _, share := range row.Ownership.Shares() {
		at := slices.IndexFunc(directory.members, func(m domain.Member) bool { return m.ID == share.MemberID })
		name := l.Text(i18n.AccountReferenceUnavailable)
		if at >= 0 {
			name = directory.members[at].Name
		}
		detail = append(detail, name)
	}
	if row.InstitutionName != "" {
		detail = append(detail, row.InstitutionName)
	}
	detail = append(detail, l.Text(accountTypes[a.AccountType]))
	size := holdingReconColumn(gtx, []layout.Widget{
		view.Checkbox(box, a.Name+" · "+a.DefaultCurrency.String()),
		view.Message(strings.Join(detail, " · "), view.Colors.SecondaryInk),
	})
	if box.Value != selected {
		s.freeze(s.chosen(directory.accounts))
		s.selected[a.ID.String()] = box.Value
		gtx.Execute(op.InvalidateCmd{})
	}
	return size
}
