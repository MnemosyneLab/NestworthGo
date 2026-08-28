package domain

import "strings"

// InstitutionType is the closed set of organizations an account can belong to.
type InstitutionType string

const (
	InstitutionBank       InstitutionType = "bank"
	InstitutionBrokerage  InstitutionType = "brokerage"
	InstitutionInsurer    InstitutionType = "insurer"
	InstitutionExchange   InstitutionType = "exchange"
	InstitutionEmployer   InstitutionType = "employer"
	InstitutionGovernment InstitutionType = "government"
	InstitutionOther      InstitutionType = "other"
)

func ParseInstitutionType(value string) (InstitutionType, error) {
	parsed := InstitutionType(strings.TrimSpace(value))
	switch parsed {
	case InstitutionBank, InstitutionBrokerage, InstitutionInsurer, InstitutionExchange,
		InstitutionEmployer, InstitutionGovernment, InstitutionOther:
		return parsed, nil
	default:
		return "", validation("institutionType", "is not supported")
	}
}

func AllInstitutionTypes() []InstitutionType {
	return []InstitutionType{InstitutionBank, InstitutionBrokerage, InstitutionInsurer, InstitutionExchange, InstitutionEmployer, InstitutionGovernment, InstitutionOther}
}
