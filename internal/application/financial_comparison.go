package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type FinancialComparisonRequest struct {
	LeftAsOf   string                       `json:"leftAsOf"`
	RightAsOf  string                       `json:"rightAsOf"`
	Scope      FinancialContextScopeRequest `json:"scope,omitempty"`
	Disclosure string                       `json:"disclosure,omitempty"`
}
type FinancialComparisonSide struct {
	AsOf        FinancialContextAsOf     `json:"asOf"`
	Scope       FinancialContextScope    `json:"scope"`
	Basis       FinancialContextBasis    `json:"basis"`
	Summary     FinancialContextSummary  `json:"summary"`
	Cash        FinancialContextAmount   `json:"cash"`
	Investments FinancialContextAmount   `json:"investments"`
	OtherAssets FinancialContextAmount   `json:"otherAssets"`
	Coverage    FinancialContextCoverage `json:"coverage"`
	DataAsOf    FinancialContextDataAsOf `json:"dataAsOf"`
}
type FinancialComparisonChange struct {
	Assets      FinancialContextAmount `json:"assets"`
	Cash        FinancialContextAmount `json:"cash"`
	Investments FinancialContextAmount `json:"investments"`
	OtherAssets FinancialContextAmount `json:"otherAssets"`
	Liabilities FinancialContextAmount `json:"liabilities"`
	NetWorth    FinancialContextAmount `json:"netWorth"`
}
type FinancialComparisonPosition struct {
	Ref            string                    `json:"ref"`
	ParentRef      string                    `json:"parentRef,omitempty"`
	Kind           string                    `json:"kind"`
	Left           *FinancialContextPosition `json:"left"`
	Right          *FinancialContextPosition `json:"right"`
	BaseChange     *string                   `json:"baseChange"`
	NativeChange   *string                   `json:"nativeChange"`
	QuantityChange *string                   `json:"quantityChange"`
	Changed        bool                      `json:"changed"`
}
type FinancialComparisonGap struct {
	Side string `json:"side"`
	FinancialContextGap
}
type FinancialComparisonEvidence struct {
	Side string `json:"side"`
	FinancialContextEvidence
}
type FinancialComparisonContent struct {
	SchemaVersion      string                        `json:"schemaVersion"`
	CalculationVersion string                        `json:"calculationVersion"`
	ResolverPolicy     string                        `json:"resolverPolicy"`
	Disclosure         string                        `json:"disclosure"`
	ChangeBasis        string                        `json:"changeBasis"`
	InvestmentBasis    string                        `json:"investmentBasis"`
	AbsenceBasis       string                        `json:"absenceBasis"`
	Left               FinancialComparisonSide       `json:"left"`
	Right              FinancialComparisonSide       `json:"right"`
	Change             FinancialComparisonChange     `json:"change"`
	Positions          []FinancialComparisonPosition `json:"positions"`
	Gaps               []FinancialComparisonGap      `json:"gaps"`
	Evidence           []FinancialComparisonEvidence `json:"evidence"`
}
type FinancialComparisonResult struct {
	CapturedAt  time.Time
	ContentHash string
	Content     FinancialComparisonContent
}

// Both sides share the same bounded transaction and configuration capture.
// Identities are aligned before disclosure, never by independently assigned aliases.
func (s *Service) BuildFinancialComparison(ctx context.Context, in FinancialComparisonRequest) (FinancialComparisonResult, error) {
	var empty FinancialComparisonResult
	if in.LeftAsOf == "" || in.LeftAsOf == "current" || in.RightAsOf == "" {
		return empty, contextValidation("leftAsOf requires a closed date; rightAsOf requires a closed date or current")
	}
	leftRequest, ids, err := normalizeFinancialContextRequest(FinancialContextRequest{AsOf: in.LeftAsOf, Scope: in.Scope, Disclosure: in.Disclosure})
	if err != nil {
		return empty, err
	}
	rightRequest, _, err := normalizeFinancialContextRequest(FinancialContextRequest{AsOf: in.RightAsOf, Scope: in.Scope, Disclosure: in.Disclosure})
	if err != nil {
		return empty, err
	}
	inputs, now, provider, ttl, err := s.captureFinancialContext(ctx, true, ids)
	if err != nil {
		return empty, err
	}
	left, err := s.prepareFinancialContextSide(ctx, leftRequest, ids, inputs, now, provider, ttl)
	if err != nil {
		return empty, err
	}
	right, err := s.prepareFinancialContextSide(ctx, rightRequest, ids, inputs, now, provider, ttl)
	if err != nil {
		return empty, err
	}
	aligned := alignHistoricalOverview(left.rows, right.rows, true)
	sort.Slice(aligned, func(i, j int) bool { return aligned[i].Key < aligned[j].Key })
	refs := financialContextRefs(aligned, leftRequest.Disclosure)
	l, err := projectFinancialContext(ctx, leftRequest, left, ttl, refs)
	if err != nil {
		return empty, err
	}
	r, err := projectFinancialContext(ctx, rightRequest, right, ttl, refs)
	if err != nil {
		return empty, err
	}
	// Evidence is side-specific: the same observation can have different freshness
	// or replay values. Prefix references instead of conflating these assertions.
	prefixComparisonEvidence(&l, "left:")
	prefixComparisonEvidence(&r, "right:")
	content := FinancialComparisonContent{SchemaVersion: "financial-comparison/1", CalculationVersion: "valuation-context/1", ResolverPolicy: domain.MarketDataResolverPolicy, Disclosure: leftRequest.Disclosure,
		ChangeBasis: "right_minus_left; not_return_or_attribution", InvestmentBasis: "included asset holdings and unclassified_investment balances; cash excluded; remaining assets are otherAssets", AbsenceBasis: "null side means absent from replay; requested accounts not yet created use not_created; absent values are not zero",
		Left: comparisonSide(l), Right: comparisonSide(r), Positions: []FinancialComparisonPosition{}, Gaps: []FinancialComparisonGap{}, Evidence: []FinancialComparisonEvidence{}}
	currency := l.Basis.BaseCurrency
	delta := func(a, b FinancialContextAmount) FinancialContextAmount {
		return contextAmount(historicalDifference(a.Value, b.Value), currency)
	}
	content.Change = FinancialComparisonChange{Assets: delta(l.Summary.Assets, r.Summary.Assets), Cash: delta(content.Left.Cash, content.Right.Cash), Investments: delta(content.Left.Investments, content.Right.Investments), OtherAssets: delta(content.Left.OtherAssets, content.Right.OtherAssets), Liabilities: delta(l.Summary.Liabilities, r.Summary.Liabilities), NetWorth: delta(l.Summary.NetWorth, r.Summary.NetWorth)}
	lm, rm := map[string]*FinancialContextPosition{}, map[string]*FinancialContextPosition{}
	for i := range l.Positions {
		lm[l.Positions[i].Ref] = &l.Positions[i]
	}
	for i := range r.Positions {
		rm[r.Positions[i].Ref] = &r.Positions[i]
	}
	for _, row := range aligned {
		content.Positions = append(content.Positions, FinancialComparisonPosition{Ref: refs[row.Key], ParentRef: refs[row.ParentKey], Kind: row.Kind, Left: lm[refs[row.Key]], Right: rm[refs[row.Key]], BaseChange: row.BaseChange, NativeChange: row.NativeChange, QuantityChange: row.QuantityChange, Changed: row.Changed})
	}
	for _, pair := range []struct {
		name    string
		content FinancialContextContent
	}{{"left", l}, {"right", r}} {
		for _, gap := range pair.content.Gaps {
			content.Gaps = append(content.Gaps, FinancialComparisonGap{pair.name, gap})
		}
		for _, evidence := range pair.content.Evidence {
			content.Evidence = append(content.Evidence, FinancialComparisonEvidence{pair.name, evidence})
		}
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return empty, err
	}
	digest := sha256.Sum256(encoded)
	return FinancialComparisonResult{CapturedAt: now, ContentHash: "sha256:" + hex.EncodeToString(digest[:]), Content: content}, nil
}
func prefixComparisonEvidence(c *FinancialContextContent, prefix string) {
	refs := map[string]string{}
	for i := range c.Evidence {
		e := &c.Evidence[i]
		refs[e.Ref] = prefix + e.Ref
		e.Ref = prefix + e.Ref
	}
	for i := range c.Positions {
		for j, ref := range c.Positions[i].EvidenceRefs {
			c.Positions[i].EvidenceRefs[j] = refs[ref]
		}
	}
	for i := range c.Gaps {
		if ref, ok := refs[c.Gaps[i].DependencyRef]; ok {
			c.Gaps[i].DependencyRef = ref
		}
	}
}

// This is a projection of valued engine components, not another valuation engine.
func comparisonSide(c FinancialContextContent) FinancialComparisonSide {
	sums := [3]decimal.Decimal{decimal.Zero, decimal.Zero, decimal.Zero}
	complete := [3]bool{true, true, true}
	for _, p := range c.Positions {
		if p.Kind == "account" || !p.Included || p.Role == "liability" {
			continue
		}
		bucket := 2
		if p.AssetClass == domain.BucketCash {
			bucket = 0
		} else if p.Kind == "holding" || p.AssetClass == domain.BucketUnclassifiedInvestment {
			bucket = 1
		}
		if !p.Complete || p.BaseAmount == nil {
			complete[bucket] = false
			continue
		}
		amount, err := decimal.NewFromString(*p.BaseAmount)
		if err != nil {
			complete[bucket] = false
			continue
		}
		sums[bucket] = sums[bucket].Add(amount)
	}
	amounts := [3]FinancialContextAmount{}
	for i := range sums {
		var value *string
		if complete[i] {
			value = historicalString(sums[i].String())
		}
		amounts[i] = contextAmount(value, c.Basis.BaseCurrency)
	}
	return FinancialComparisonSide{AsOf: c.AsOf, Scope: c.Scope, Basis: c.Basis, Summary: c.Summary, Cash: amounts[0], Investments: amounts[1], OtherAssets: amounts[2], Coverage: c.Coverage, DataAsOf: c.DataAsOf}
}
