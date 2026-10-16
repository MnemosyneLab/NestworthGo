package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const ReviewLegacyEvaluator = "legacy_v1"

// ReviewLegacyConfiguration is the exact v1 preference set. Enrollment false
// remains distinct from an absent entry; neither becomes a v2 suppression.
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
	if err := json.Unmarshal(raw, &required); err != nil || required == nil {
		return empty, validation("legacyReview", "legacy configuration must be an object")
	}
	for _, key := range []string{"revision", "policy", "enrolled", "acknowledged"} {
		if len(required[key]) == 0 || bytes.Equal(required[key], []byte("null")) {
			return empty, validation("legacyReview", fmt.Sprintf("missing %s", key))
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
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

func DecodeReviewLegacySpec(raw []byte) (ReviewLegacySpec, error) {
	var value ReviewLegacySpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.SpecVersion != 1 || value.Evaluator != ReviewLegacyEvaluator {
		return ReviewLegacySpec{}, validation("legacyReview", "unsupported legacy review specification")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ReviewLegacySpec{}, validation("legacyReview", "legacy specification has trailing data")
	}
	configuration, err := json.Marshal(value.Configuration)
	if err != nil {
		return ReviewLegacySpec{}, err
	}
	value.Configuration, err = DecodeReviewLegacyConfiguration(configuration)
	return value, err
}
