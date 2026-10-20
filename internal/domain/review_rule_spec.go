package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const (
	ReviewSpecVersion         = 1
	ReviewMetricPolicyVersion = "daily_metrics_v1"
	ReviewEvaluator           = "daily_predicate_v1"
	ReviewCutoffVersion       = "civil_day_end_v1"
)

// ReviewRuleSpec is intentionally a closed wire format. DecodeReviewRuleSpec
// rejects fields from legacy duration policies or future versions instead of
// silently evaluating only the recognized subset.
type ReviewRuleSpec struct {
	SpecVersion         int                  `json:"specVersion"`
	MetricPolicyVersion string               `json:"metricPolicyVersion"`
	Evaluator           string               `json:"evaluator"`
	Target              ReviewRuleTarget     `json:"target"`
	Predicate           ReviewPredicateNode  `json:"predicate"`
	Schedule            ReviewRuleSchedule   `json:"schedule"`
	TriggerPolicy       ReviewTriggerPolicy  `json:"triggerPolicy"`
	DataPolicy          ReviewRuleDataPolicy `json:"dataPolicy"`
}

type ReviewRuleTarget struct {
	Kind          string         `json:"kind"` // all_current or instruments
	InstrumentIDs []InstrumentID `json:"instrumentIds,omitempty"`
}

type ReviewRuleSchedule struct {
	Period        string `json:"period"`
	Timezone      string `json:"timezone"`
	CutoffVersion string `json:"cutoffVersion"`
}

type ReviewRuleDataPolicy struct {
	Missing                string `json:"missing"`
	Calendar               string `json:"calendar"`
	QuoteCarry             string `json:"quoteCarry"`
	BaselineMaxGapDays     int    `json:"baselineMaxGapDays"`
	FXReferenceMaxAgeDays  int    `json:"fxReferenceMaxAgeDays"`
	FXTimestampMaxAgeHours int    `json:"fxTimestampMaxAgeHours"`
}

func DefaultReviewRuleDataPolicy() ReviewRuleDataPolicy {
	return ReviewRuleDataPolicy{Missing: "unknown", Calendar: "all_civil_days", QuoteCarry: "forbid_for_daily_signal", BaselineMaxGapDays: 7, FXReferenceMaxAgeDays: 7, FXTimestampMaxAgeHours: 12}
}

// A node has exactly one of All, Any, or Leaf. Groups may nest once beneath
// the root; the hard positive-quantity gate is always outside this tree.
type ReviewPredicateNode struct {
	All  []ReviewPredicateNode `json:"all,omitempty"`
	Any  []ReviewPredicateNode `json:"any,omitempty"`
	Leaf *ReviewRuleLeaf       `json:"leaf,omitempty"`
}

type ReviewRuleLeaf struct {
	Metric        string           `json:"metric"`
	Operator      string           `json:"operator,omitempty"`
	Value         string           `json:"value,omitempty"`
	Currency      CurrencyCode     `json:"currency,omitempty"`
	FXMode        string           `json:"fxMode,omitempty"` // native or convert_on_day
	LookbackDays  int              `json:"lookbackDays,omitempty"`
	InstrumentIDs []InstrumentID   `json:"instrumentIds,omitempty"`
	Types         []InstrumentType `json:"types,omitempty"`
}

var reviewPercentSyntax = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,4})(\.[0-9]{1,8})?$`)

func DecodeReviewRuleSpec(raw []byte) (ReviewRuleSpec, error) {
	if len(raw) == 0 || len(raw) > 64*1024 {
		return ReviewRuleSpec{}, validation("spec", "rule JSON must be 1–65536 bytes")
	}
	decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var spec ReviewRuleSpec
	if err := decoder.Decode(&spec); err != nil {
		return ReviewRuleSpec{}, validation("spec", fmt.Sprintf("invalid rule JSON: %v", err))
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ReviewRuleSpec{}, validation("spec", "must contain exactly one JSON object")
	}
	return spec.Normalize()
}

// Normalize makes semantically equivalent decimals, sets, and one-day
// policies share a hash. Validation precedes persistence and evaluation.
func (s ReviewRuleSpec) Normalize() (ReviewRuleSpec, error) {
	if s.SpecVersion != ReviewSpecVersion || s.MetricPolicyVersion != ReviewMetricPolicyVersion || s.Evaluator != ReviewEvaluator {
		return ReviewRuleSpec{}, validation("specVersion", "unsupported daily review rule version")
	}
	if s.Schedule.Period != "calendar_day" || s.Schedule.CutoffVersion != ReviewCutoffVersion || strings.TrimSpace(s.Schedule.Timezone) != s.Schedule.Timezone || s.Schedule.Timezone == "" || s.Schedule.Timezone == "Local" {
		return ReviewRuleSpec{}, validation("schedule", "a fixed IANA timezone and civil-day cutoff are required")
	}
	if _, err := LoadFinancialLocation(s.Schedule.Timezone); err != nil {
		return ReviewRuleSpec{}, validation("timezone", "must be an available IANA timezone")
	}
	if s.DataPolicy != DefaultReviewRuleDataPolicy() {
		return ReviewRuleSpec{}, validation("dataPolicy", "unsupported daily evidence admission policy")
	}
	policy, err := s.TriggerPolicy.Normalize()
	if err != nil {
		return ReviewRuleSpec{}, err
	}
	s.TriggerPolicy = policy
	switch s.Target.Kind {
	case "all_current":
		if len(s.Target.InstrumentIDs) != 0 {
			return ReviewRuleSpec{}, validation("target", "all-current target cannot include instrument IDs")
		}
	case "instruments":
		if len(s.Target.InstrumentIDs) == 0 || len(s.Target.InstrumentIDs) > 200 {
			return ReviewRuleSpec{}, validation("target", "select 1–200 instrument IDs")
		}
		ids, err := normalizeReviewIDs(s.Target.InstrumentIDs)
		if err != nil {
			return ReviewRuleSpec{}, err
		}
		s.Target.InstrumentIDs = ids
	default:
		return ReviewRuleSpec{}, validation("target", "unsupported target kind")
	}
	leaves := 0
	predicate, err := normalizeReviewPredicate(s.Predicate, 0, &leaves)
	if err != nil {
		return ReviewRuleSpec{}, err
	}
	s.Predicate = predicate
	if !reviewAnnualizedScoped(predicate, false) {
		return ReviewRuleSpec{}, validation("predicate", "annualized conditions require an explicit bank-product type in the same all group")
	}
	return s, nil
}

// Every path to an annualized leaf needs a bank-only type gate in an AND
// ancestor. An OR sibling cannot restrict an annualized branch.
func reviewAnnualizedScoped(node ReviewPredicateNode, bankOnly bool) bool {
	if node.Leaf != nil {
		return node.Leaf.Metric != "annualized" || bankOnly
	}
	if node.All != nil {
		for _, child := range node.All {
			if child.Leaf != nil && child.Leaf.Metric == "type" && len(child.Leaf.Types) == 1 && child.Leaf.Types[0] == InstrumentBankInvestmentProduct {
				bankOnly = true
			}
		}
	}
	children := node.All
	if node.Any != nil {
		children = node.Any
	}
	for _, child := range children {
		if !reviewAnnualizedScoped(child, bankOnly) {
			return false
		}
	}
	return true
}

func normalizeReviewIDs(ids []InstrumentID) ([]InstrumentID, error) {
	result := make([]InstrumentID, 0, len(ids))
	for _, id := range ids {
		parsed, err := ParseInstrumentID(string(id))
		if err != nil {
			return nil, err
		}
		result = append(result, parsed)
	}
	slices.Sort(result)
	return slices.Compact(result), nil
}

func normalizeReviewPredicate(node ReviewPredicateNode, depth int, leaves *int) (ReviewPredicateNode, error) {
	shapes := 0
	if node.All != nil {
		shapes++
	}
	if node.Any != nil {
		shapes++
	}
	if node.Leaf != nil {
		shapes++
	}
	if shapes != 1 {
		return ReviewPredicateNode{}, validation("predicate", "each node needs exactly one of all, any, or leaf")
	}
	if node.Leaf != nil {
		if depth == 0 {
			return ReviewPredicateNode{}, validation("predicate", "root must be an all or any group")
		}
		*leaves++
		if *leaves > 20 {
			return ReviewPredicateNode{}, validation("predicate", "at most 20 conditions are supported")
		}
		leaf, err := normalizeReviewLeaf(*node.Leaf)
		if err != nil {
			return ReviewPredicateNode{}, err
		}
		return ReviewPredicateNode{Leaf: &leaf}, nil
	}
	if depth >= 2 {
		return ReviewPredicateNode{}, validation("predicate", "at most two group levels are supported")
	}
	children := node.All
	if node.Any != nil {
		children = node.Any
	}
	if len(children) == 0 || len(children) > 20 {
		return ReviewPredicateNode{}, validation("predicate", "a group needs 1–20 children")
	}
	normalized := make([]ReviewPredicateNode, 0, len(children))
	for _, child := range children {
		item, err := normalizeReviewPredicate(child, depth+1, leaves)
		if err != nil {
			return ReviewPredicateNode{}, err
		}
		normalized = append(normalized, item)
	}
	// AND and OR are commutative. Keep a single copy of each identical leaf or
	// subgroup and sort them for a stable semantic hash.
	sort.Slice(normalized, func(i, j int) bool {
		a, _ := jsoncodec.Marshal(normalized[i])
		b, _ := jsoncodec.Marshal(normalized[j])
		return bytes.Compare(a, b) < 0
	})
	unique := normalized[:0]
	for _, item := range normalized {
		if len(unique) == 0 || !reflectReviewNodesEqual(unique[len(unique)-1], item) {
			unique = append(unique, item)
		}
	}
	if node.All != nil {
		if err := validateReviewNumericIntervals(unique); err != nil {
			return ReviewPredicateNode{}, err
		}
	}
	if node.All != nil {
		return ReviewPredicateNode{All: unique}, nil
	}
	return ReviewPredicateNode{Any: unique}, nil
}

func reflectReviewNodesEqual(a, b ReviewPredicateNode) bool {
	left, _ := jsoncodec.Marshal(a)
	right, _ := jsoncodec.Marshal(b)
	return bytes.Equal(left, right)
}

func validateReviewNumericIntervals(children []ReviewPredicateNode) error {
	type bound struct {
		value  decimal.Decimal
		strict bool
		set    bool
	}
	type interval struct{ lower, upper bound }
	intervals := map[string]interval{}
	for _, child := range children {
		if child.Leaf == nil {
			continue
		}
		leaf := child.Leaf
		if leaf.Metric != "amount" && leaf.Metric != "cost" && leaf.Metric != "price_change" && leaf.Metric != "annualized" {
			continue
		}
		value, _ := decimal.NewFromString(leaf.Value)
		key := fmt.Sprintf("%s/%d/%s/%s", leaf.Metric, leaf.LookbackDays, leaf.Currency, leaf.FXMode)
		current := intervals[key]
		newBound := bound{value: value, strict: leaf.Operator == ">" || leaf.Operator == "<", set: true}
		if leaf.Operator == ">" || leaf.Operator == ">=" {
			if !current.lower.set || value.GreaterThan(current.lower.value) || (value.Equal(current.lower.value) && newBound.strict) {
				current.lower = newBound
			}
		} else if !current.upper.set || value.LessThan(current.upper.value) || (value.Equal(current.upper.value) && newBound.strict) {
			current.upper = newBound
		}
		intervals[key] = current
	}
	for _, interval := range intervals {
		if interval.lower.set && interval.upper.set && (interval.lower.value.GreaterThan(interval.upper.value) || (interval.lower.value.Equal(interval.upper.value) && (interval.lower.strict || interval.upper.strict))) {
			return validation("predicate", "numeric conditions have an impossible interval")
		}
	}
	return nil
}

func normalizeReviewLeaf(leaf ReviewRuleLeaf) (ReviewRuleLeaf, error) {
	switch leaf.Metric {
	case "instrument":
		if leaf.Operator != "in" && leaf.Operator != "not_in" {
			return leaf, validation("operator", "instrument condition requires in or not_in")
		}
		if leaf.Value != "" || leaf.Currency != "" || leaf.FXMode != "" || leaf.LookbackDays != 0 || len(leaf.Types) != 0 || len(leaf.InstrumentIDs) == 0 || len(leaf.InstrumentIDs) > 200 {
			return leaf, validation("condition", "invalid instrument condition fields")
		}
		ids, err := normalizeReviewIDs(leaf.InstrumentIDs)
		if err != nil {
			return leaf, err
		}
		leaf.InstrumentIDs = ids
	case "type":
		if leaf.Operator != "in" || leaf.Value != "" || leaf.Currency != "" || leaf.FXMode != "" || leaf.LookbackDays != 0 || len(leaf.InstrumentIDs) != 0 || len(leaf.Types) == 0 || len(leaf.Types) > len(AllInstrumentTypes()) {
			return leaf, validation("condition", "type condition requires a type set")
		}
		for _, kind := range leaf.Types {
			if _, err := ParseInstrumentType(string(kind)); err != nil {
				return leaf, err
			}
		}
		slices.Sort(leaf.Types)
		leaf.Types = slices.Compact(leaf.Types)
	case "amount", "cost", "price_change", "annualized":
		if leaf.Operator != ">" && leaf.Operator != ">=" && leaf.Operator != "<" && leaf.Operator != "<=" {
			return leaf, validation("operator", "use >, >=, <, or <=")
		}
		if len(leaf.InstrumentIDs) != 0 || len(leaf.Types) != 0 {
			return leaf, validation("condition", "numeric condition cannot contain a set")
		}
		if leaf.Metric == "amount" || leaf.Metric == "cost" {
			if leaf.LookbackDays != 0 || (leaf.FXMode != "native" && leaf.FXMode != "convert_on_day") {
				return leaf, validation("condition", "amount and cost require a currency comparison mode")
			}
			currency, err := ParseSupportedCurrency(string(leaf.Currency))
			if err != nil {
				return leaf, err
			}
			leaf.Currency = currency
			value, err := ParseMoney(leaf.Value, currency)
			if err != nil {
				return leaf, err
			}
			if value.Amount().Exponent() < -int32(CurrencyFractionDigits(currency)) {
				return leaf, validation("value", "threshold exceeds the currency's minor-unit precision")
			}
			leaf.Value = value.CanonicalAmount()
		} else {
			if leaf.Currency != "" || leaf.FXMode != "" || !ValidQuotePeriodWindow(leaf.LookbackDays) || leaf.LookbackDays == 0 || !reviewPercentSyntax.MatchString(leaf.Value) {
				return leaf, validation("condition", "invalid performance threshold or lookback")
			}
			value, err := decimal.NewFromString(leaf.Value)
			if err != nil {
				return leaf, validation("value", "invalid percentage")
			}
			leaf.Value = canonicalDecimal(value)
			if leaf.Metric == "price_change" && value.LessThan(decimal.NewFromInt(-100)) {
				return leaf, validation("value", "price change cannot be below -100%")
			}
			if value.GreaterThan(decimal.NewFromInt(10000)) || value.LessThan(decimal.NewFromInt(-10000)) {
				return leaf, validation("value", "percentage is outside the supported range")
			}
		}
	default:
		return leaf, validation("metric", "unsupported daily condition")
	}
	return leaf, nil
}

func (s ReviewRuleSpec) SemanticHash() (string, error) {
	normalized, err := s.Normalize()
	if err != nil {
		return "", err
	}
	data, err := jsoncodec.Marshal(normalized)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
