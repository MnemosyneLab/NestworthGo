package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// ReviewLegacyEvaluator identifies migration-only compatibility rules. V2
// runtime queries intentionally select only ReviewEvaluator.
const ReviewLegacyEvaluator = "legacy_v1"

// InvestmentReviewPolicy is retained only to validate historical v1 preference
// JSON retained in immutable recovery records inside supported schema21/22.
type InvestmentReviewPolicy struct {
	Enabled              bool   `json:"enabled"`
	WindowDays           int    `json:"windowDays"`
	LowAnnualizedPercent string `json:"lowAnnualizedPercent"`
	LossPercent          string `json:"lossPercent"`
	SustainDays          int    `json:"sustainDays"`
	MaxAgeDays           int    `json:"maxAgeDays"`
}

var reviewThresholdSyntax = regexp.MustCompile(`^-?(0|[1-9][0-9]?)(\.[0-9]{1,8})?$`)

func (p InvestmentReviewPolicy) Validate() error {
	low, e1 := decimal.NewFromString(p.LowAnnualizedPercent)
	loss, e2 := decimal.NewFromString(p.LossPercent)
	if !reviewThresholdSyntax.MatchString(p.LowAnnualizedPercent) || !reviewThresholdSyntax.MatchString(p.LossPercent) || (p.WindowDays != 7 && p.WindowDays != 30) || p.SustainDays < 2 || p.SustainDays > 30 || p.MaxAgeDays < 1 || p.MaxAgeDays > 30 || e1 != nil || e2 != nil || low.IsNegative() || low.GreaterThan(decimal.NewFromInt(10)) || !loss.IsNegative() || loss.LessThan(decimal.NewFromInt(-50)) {
		return validation("investmentReviewPolicy", "use a 7/30-day window, 2–30 sustained days, 1–30 age days, low threshold 0–10%, and loss threshold -50% to below 0%")
	}
	return nil
}

// ReviewLegacyConfiguration names the exact v1 preference shape for migration
// validation and read-only recovery; live application writes are retired.
type ReviewLegacyConfiguration struct {
	PriorPolicies []InvestmentReviewPolicy `json:"priorPolicies,omitempty"`
	SnoozedUntil  map[string]string        `json:"snoozedUntil"`
	Enrolled      map[string]bool          `json:"enrolled"`
	Revision      int                      `json:"revision"`
	Policy        InvestmentReviewPolicy   `json:"policy"`
	Acknowledged  map[string]bool          `json:"acknowledged"`
}

type ReviewLegacySpec struct {
	SpecVersion   int                       `json:"specVersion"`
	Evaluator     string                    `json:"evaluator"`
	Configuration ReviewLegacyConfiguration `json:"configuration"`
}

func DecodeReviewLegacyConfiguration(raw []byte) (ReviewLegacyConfiguration, error) {
	var empty ReviewLegacyConfiguration
	if len(raw) == 0 || len(raw) > 1024*1024 {
		return empty, validation("legacyReview", "legacy configuration size is invalid")
	}
	var required map[string]json.RawMessage
	if err := jsoncodec.Unmarshal(raw, &required); err != nil || required == nil {
		return empty, validation("legacyReview", "legacy configuration must be an object")
	}
	for _, key := range []string{"revision", "policy", "enrolled", "acknowledged"} {
		if len(required[key]) == 0 || bytes.Equal(required[key], []byte("null")) {
			return empty, validation("legacyReview", fmt.Sprintf("missing %s", key))
		}
	}
	decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value ReviewLegacyConfiguration
	if err := decoder.Decode(&value); err != nil {
		return empty, validation("legacyReview", "legacy configuration fields are invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return empty, validation("legacyReview", "legacy configuration has trailing data")
	}
	if value.Revision < 0 || value.Policy.Validate() != nil || value.Enrolled == nil || value.Acknowledged == nil {
		return empty, validation("legacyReview", "legacy configuration values are invalid")
	}
	if value.SnoozedUntil == nil {
		value.SnoozedUntil = map[string]string{}
	}
	for id := range value.Enrolled {
		if _, err := ParseInstrumentID(id); err != nil {
			return empty, validation("legacyReview", "legacy enrollment instrument is invalid")
		}
	}
	for id, on := range value.SnoozedUntil {
		if _, err := ParseInstrumentID(id); err != nil {
			return empty, validation("legacyReview", "legacy snooze instrument is invalid")
		}
		if _, err := time.Parse(time.DateOnly, on); err != nil {
			return empty, validation("legacyReview", "legacy snooze date is invalid")
		}
	}
	for _, prior := range value.PriorPolicies {
		if prior.Validate() != nil {
			return empty, validation("legacyReview", "legacy prior policy is invalid")
		}
	}
	return value, nil
}

// DecodeReviewLegacySpec remains available to the read-only settings recovery
// path for existing legacy_v1 rules in supported schema21/22 databases.
func DecodeReviewLegacySpec(raw []byte) (ReviewLegacySpec, error) {
	var value ReviewLegacySpec
	decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.SpecVersion != 1 || value.Evaluator != ReviewLegacyEvaluator {
		return ReviewLegacySpec{}, validation("legacyReview", "unsupported legacy review specification")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ReviewLegacySpec{}, validation("legacyReview", "legacy specification has trailing data")
	}
	configuration, err := jsoncodec.Marshal(value.Configuration)
	if err != nil {
		return ReviewLegacySpec{}, err
	}
	value.Configuration, err = DecodeReviewLegacyConfiguration(configuration)
	return value, err
}
