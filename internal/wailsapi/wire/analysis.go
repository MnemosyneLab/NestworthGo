package wire

// Asset Changes response DTOs. These live in the shared wire
// package so all analysis surfaces use the same canonical signed amount and
// completeness vocabulary at the IPC boundary.

type AssetChangeDTO struct {
	Summary            AssetChangeSummaryDTO `json:"summary"`
	Waterfall          []AssetChangeRowDTO   `json:"waterfall"`
	Groups             []AssetChangeGroupDTO `json:"groups"`
	ResidualIssueCount int                   `json:"residualIssueCount"`
	Available          bool                  `json:"available"`
	Status             string                `json:"status"`
	MissingReason      string                `json:"missingReason,omitempty"`
	ValuationForced    *string               `json:"valuationForced"`
}

type AssetChangeSummaryDTO struct {
	BeginningValue *SignedMoneyView `json:"beginningValue,omitempty"`
	EndingValue    *SignedMoneyView `json:"endingValue,omitempty"`
	Change         *SignedMoneyView `json:"change,omitempty"`
}

type AssetChangeRowDTO struct {
	Key    string           `json:"key"`
	Label  string           `json:"label"`
	Bucket string           `json:"bucket"`
	Amount *SignedMoneyView `json:"amount,omitempty"`
}

type AssetChangeGroupDTO struct {
	Key    string              `json:"key"`
	Label  string              `json:"label"`
	Amount *SignedMoneyView    `json:"amount,omitempty"`
	Rows   []AssetChangeRowDTO `json:"rows"`
}

type AnalysisDimensionAmountDTO struct {
	Key          string           `json:"key"`
	Label        string           `json:"label"`
	AccountID    string           `json:"accountId,omitempty"`
	InstrumentID string           `json:"instrumentId,omitempty"`
	Amount       *SignedMoneyView `json:"amount,omitempty"`
}

type AssetDriverDetailDTO struct {
	DriverKey       string                       `json:"driverKey"`
	ByInstrument    []AnalysisDimensionAmountDTO `json:"byInstrument"`
	ByAccount       []AnalysisDimensionAmountDTO `json:"byAccount"`
	ResidualDetails []AssetResidualDetailDTO     `json:"residualDetails"`
	Available       bool                         `json:"available"`
	Status          string                       `json:"status"`
	MissingReason   string                       `json:"missingReason,omitempty"`
	ValuationForced *string                      `json:"valuationForced"`
}

type AssetResidualDetailDTO struct {
	Date         string           `json:"date"`
	ComponentKey string           `json:"componentKey"`
	AccountID    string           `json:"accountId,omitempty"`
	HoldingID    string           `json:"holdingId,omitempty"`
	InstrumentID string           `json:"instrumentId,omitempty"`
	Amount       *SignedMoneyView `json:"amount,omitempty"`
}

type AssetTrendPointDTO struct {
	Period          string           `json:"period"`
	Value           *SignedMoneyView `json:"value,omitempty"`
	Rate            *string          `json:"rate"`
	RatedDays       int              `json:"ratedDays"`
	TotalDays       int              `json:"totalDays"`
	Available       bool             `json:"available"`
	Status          string           `json:"status"`
	MissingReason   string           `json:"missingReason,omitempty"`
	ValuationForced *string          `json:"valuationForced"`
}

type AssetTrendDTO struct {
	Points          []AssetTrendPointDTO `json:"points"`
	Summary         *SignedMoneyView     `json:"summary,omitempty"`
	Rate            *string              `json:"rate"`
	RatedDays       int                  `json:"ratedDays"`
	TotalDays       int                  `json:"totalDays"`
	Available       bool                 `json:"available"`
	Status          string               `json:"status"`
	MissingReason   string               `json:"missingReason,omitempty"`
	ValuationForced *string              `json:"valuationForced"`
}

type CategoryRowDTO struct {
	Key          string           `json:"key"`
	Label        string           `json:"label"`
	AccountID    string           `json:"accountId,omitempty"`
	InstrumentID string           `json:"instrumentId,omitempty"`
	AssetClass   string           `json:"assetClass,omitempty"`
	Amount       *SignedMoneyView `json:"amount,omitempty"`
}

type CategoriesDTO struct {
	Total           *SignedMoneyView `json:"total,omitempty"`
	Rows            []CategoryRowDTO `json:"rows"`
	Available       bool             `json:"available"`
	Status          string           `json:"status"`
	MissingReason   string           `json:"missingReason,omitempty"`
	ValuationForced *string          `json:"valuationForced"`
}

type CategoryChildDTO struct {
	Key          string           `json:"key"`
	Label        string           `json:"label"`
	AccountID    string           `json:"accountId,omitempty"`
	InstrumentID string           `json:"instrumentId,omitempty"`
	Amount       *SignedMoneyView `json:"amount,omitempty"`
}

type CategoryActivityRefDTO struct {
	Date         string           `json:"date"`
	ActivityID   string           `json:"activityId,omitempty"`
	AccountID    string           `json:"accountId,omitempty"`
	HoldingID    string           `json:"holdingId,omitempty"`
	InstrumentID string           `json:"instrumentId,omitempty"`
	Amount       *SignedMoneyView `json:"amount,omitempty"`
}

type CategoryDetailDTO struct {
	Children        []CategoryChildDTO       `json:"children"`
	ActivityRefs    []CategoryActivityRefDTO `json:"activityRefs"`
	Available       bool                     `json:"available"`
	Status          string                   `json:"status"`
	MissingReason   string                   `json:"missingReason,omitempty"`
	ValuationForced *string                  `json:"valuationForced"`
}
