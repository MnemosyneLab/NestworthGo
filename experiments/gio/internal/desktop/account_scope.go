package desktop

import (
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountDimension uint8

const (
	accountByInstitution accountDimension = iota + 1
	accountByType
	accountByMember
	accountByGroup
)

type accountListScope struct {
	dimension accountDimension
	key       string
}

func (s accountListScope) active() bool { return s != (accountListScope{}) }

func (s accountListScope) unassigned() bool {
	return s.key == "" && (s.dimension == accountByInstitution || s.dimension == accountByGroup)
}

func breakdownAccountScope(dimension accountDimension, key string) accountListScope {
	if key == "unassigned" && (dimension == accountByInstitution || dimension == accountByGroup) {
		key = ""
	}
	return accountListScope{dimension: dimension, key: key}
}

func (s accountListScope) filter(filter *domain.AccountFilter) bool {
	switch s.dimension {
	case accountByInstitution:
		return s.unassigned()
	case accountByType:
		kind, err := domain.ParseAccountType(s.key)
		filter.AccountType = &kind
		return err == nil
	case accountByMember:
		member, err := domain.ParseMemberID(s.key)
		filter.MemberID = &member
		return err == nil
	case accountByGroup:
		if s.unassigned() {
			return true
		}
		group, err := domain.ParseGroupID(s.key)
		filter.GroupID = &group
		return err == nil
	default:
		return !s.active()
	}
}

func (s accountListScope) name(l i18n.Language, fallback string) string {
	switch {
	case s.dimension == accountByInstitution && s.unassigned():
		return l.Text(i18n.OverviewAllocationUnassigned)
	case s.dimension == accountByGroup && s.unassigned():
		return l.Text(i18n.AccountNoGroup)
	case s.dimension == accountByType:
		if id := accountTypes[domain.AccountType(s.key)]; id != i18n.NoMessage {
			return l.Text(id)
		}
	}
	return fallback
}

func (s accountListScope) includes(record domain.AccountRecord) bool {
	switch s.dimension {
	case accountByInstitution:
		return record.Account.InstitutionID == nil
	case accountByGroup:
		return !s.unassigned() || record.Account.GroupID == nil
	default:
		return true
	}
}
