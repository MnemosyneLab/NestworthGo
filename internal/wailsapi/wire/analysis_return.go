package wire

// Phase 2b return projection DTOs. Rates remain optional canonical decimal
// strings at the IPC boundary; a nil rate is meaningful and is not encoded as
// zero.

type ReturnComponentAmountDTO struct {
	Component string           `json:"component"`
	Amount    *SignedMoneyView `json:"amount,omitempty"`
}

type ReturnContributorDTO struct {
	Key       string           `json:"key"`
	Label     string           `json:"label"`
	Amount    *SignedMoneyView `json:"amount,omitempty"`
	Rate      *string          `json:"rate"`
	RatedDays int              `json:"ratedDays"`
	TotalDays int              `json:"totalDays"`
}

type ReturnSourceDTO struct {
	Key    string           `json:"key"`
	Label  string           `json:"label"`
	Amount *SignedMoneyView `json:"amount,omitempty"`
	Share  *string          `json:"share"`
}

type ReturnIssueDTO struct {
	Date          string `json:"date"`
	Status        string `json:"status"`
	MissingReason string `json:"missingReason,omitempty"`
}

type ReturnDayDTO struct {
	Date                   string                     `json:"date"`
	BeginningInvestedValue *SignedMoneyView           `json:"beginningInvestedValue,omitempty"`
	EndingInvestedValue    *SignedMoneyView           `json:"endingInvestedValue,omitempty"`
	ReturnAmount           *SignedMoneyView           `json:"returnAmount,omitempty"`
	ReturnRate             *string                    `json:"returnRate"`
	Composition            []ReturnComponentAmountDTO `json:"composition"`
	Contributors           []ReturnContributorDTO     `json:"contributors"`
	RatedDays              int                        `json:"ratedDays"`
	TotalDays              int                        `json:"totalDays"`
	Issues                 []ReturnIssueDTO           `json:"issues"`
	Available              bool                       `json:"available"`
	Status                 string                     `json:"status"`
	MissingReason          string                     `json:"missingReason,omitempty"`
	ValuationForced        *string                    `json:"valuationForced"`
}

type ReturnCalendarSummaryDTO struct {
	BeginningInvestedValue *SignedMoneyView `json:"beginningInvestedValue,omitempty"`
	EndingInvestedValue    *SignedMoneyView `json:"endingInvestedValue,omitempty"`
	ReturnAmount           *SignedMoneyView `json:"returnAmount,omitempty"`
	ReturnRate             *string          `json:"returnRate"`
	RatedDays              int              `json:"ratedDays"`
	TotalDays              int              `json:"totalDays"`
}

type ReturnCalendarDTO struct {
	Summary         ReturnCalendarSummaryDTO `json:"summary"`
	Cells           []ReturnDayDTO           `json:"cells"`
	TopContributors []ReturnContributorDTO   `json:"topContributors"`
	Issues          []ReturnIssueDTO         `json:"issues"`
	Available       bool                     `json:"available"`
	Status          string                   `json:"status"`
	MissingReason   string                   `json:"missingReason,omitempty"`
	ValuationForced *string                  `json:"valuationForced"`
}

type ReturnTrendPointDTO struct {
	Date   string           `json:"date"`
	Amount *SignedMoneyView `json:"amount,omitempty"`
	// Rate is the daily Modified Dietz rate. The period-linked rate is on the
	// parent ReturnTrendDTO; Value stays empty for linked_rate.
	Rate            *string          `json:"rate"`
	Value           *SignedMoneyView `json:"value,omitempty"`
	RatedDays       int              `json:"ratedDays"`
	TotalDays       int              `json:"totalDays"`
	Available       bool             `json:"available"`
	Status          string           `json:"status"`
	MissingReason   string           `json:"missingReason,omitempty"`
	ValuationForced *string          `json:"valuationForced"`
}

type ReturnTrendDTO struct {
	Display         string                `json:"display"`
	Points          []ReturnTrendPointDTO `json:"points"`
	Sources         []ReturnSourceDTO     `json:"sources"`
	Amount          *SignedMoneyView      `json:"amount,omitempty"`
	Rate            *string               `json:"rate"`
	RatedDays       int                   `json:"ratedDays"`
	TotalDays       int                   `json:"totalDays"`
	Available       bool                  `json:"available"`
	Status          string                `json:"status"`
	MissingReason   string                `json:"missingReason,omitempty"`
	ValuationForced *string               `json:"valuationForced"`
}

type ContributionComponentDTO struct {
	Key          string           `json:"key"`
	AccountID    string           `json:"accountId,omitempty"`
	InstrumentID string           `json:"instrumentId,omitempty"`
	Currency     string           `json:"currency,omitempty"`
	Amount       *SignedMoneyView `json:"amount,omitempty"`
}

type ContributionHistoryHintDTO struct {
	Kinds        []string `json:"kinds"`
	AccountID    string   `json:"accountId,omitempty"`
	InstrumentID string   `json:"instrumentId,omitempty"`
	From         string   `json:"from"`
	To           string   `json:"to"`
}

type ContributionRowDTO struct {
	Key             string           `json:"key"`
	Label           string           `json:"label"`
	Amount          *SignedMoneyView `json:"amount,omitempty"`
	Rate            *string          `json:"rate"`
	RatedDays       int              `json:"ratedDays"`
	TotalDays       int              `json:"totalDays"`
	Available       bool             `json:"available"`
	Status          string           `json:"status"`
	MissingReason   string           `json:"missingReason,omitempty"`
	ValuationForced *string          `json:"valuationForced"`
}

type ContributionDTO struct {
	ReturnType      string               `json:"returnType"`
	GroupBy         string               `json:"groupBy"`
	Rows            []ContributionRowDTO `json:"rows"`
	RatedDays       int                  `json:"ratedDays"`
	TotalDays       int                  `json:"totalDays"`
	Available       bool                 `json:"available"`
	Status          string               `json:"status"`
	MissingReason   string               `json:"missingReason,omitempty"`
	ValuationForced *string              `json:"valuationForced"`
}

type ContributionItemDTO struct {
	Key             string                     `json:"key"`
	Label           string                     `json:"label"`
	Amount          *SignedMoneyView           `json:"amount,omitempty"`
	Rate            *string                    `json:"rate"`
	RatedDays       int                        `json:"ratedDays"`
	TotalDays       int                        `json:"totalDays"`
	Components      []ContributionComponentDTO `json:"components"`
	ByAccount       []ContributionComponentDTO `json:"byAccount"`
	HistoryHint     ContributionHistoryHintDTO `json:"historyHint"`
	Available       bool                       `json:"available"`
	Status          string                     `json:"status"`
	MissingReason   string                     `json:"missingReason,omitempty"`
	ValuationForced *string                    `json:"valuationForced"`
}
