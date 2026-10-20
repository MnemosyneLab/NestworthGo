package domain

import (
	"sort"
	"time"
)

const ReviewFXDescriptionVersion = "direct-agent-daily-fx-v1"

// The complete two-location inventory is MAC-bound with the non-FX invariant.
// Fingerprints include every captured field and exact private financial scalar.
// No rate or selected-only input grants reuse authority.
type ReviewFXDescription struct {
	Version                                        string
	NonFXFingerprint                               string
	Base, Quote                                    CurrencyCode
	SourceKey, SourcePolicyVersion, TimestampBasis string
	Eligible                                       bool
	Buckets                                        []ReviewFXDaySeal
}
type ReviewFXCandidateSeal struct {
	ID                   FXQuoteID
	Fingerprint          string
	SelectionFingerprint string
}
type ReviewFXDaySeal struct {
	Location    string
	Base, Quote CurrencyCode
	On          string
	Candidates  []ReviewFXCandidateSeal
}
type ReviewFXDependency struct {
	ID    FXQuoteID    `json:"id"`
	Base  CurrencyCode `json:"base_currency"`
	Quote CurrencyCode `json:"quote_currency"`
	On    string       `json:"effective_date"`
}

func DescribeReviewFX(batch ReviewHistoricalBatch) (*ReviewFXDescription, error) {
	invariant := batch
	invariant.History.FXQuoteFacts = nil
	invariant.History.Portfolio.FXQuotes = nil
	invariant.History.InputGeneration = 0
	fingerprint, err := ReviewHistoricalFingerprint(invariant)
	if err != nil {
		return nil, err
	}
	out := &ReviewFXDescription{Version: ReviewFXDescriptionVersion, NonFXFingerprint: fingerprint, Eligible: true, Buckets: []ReviewFXDaySeal{}}
	type key struct {
		location    string
		base, quote CurrencyCode
		on          string
	}
	groups := map[key][]ReviewFXCandidateSeal{}
	facts := map[FXQuoteID]string{}
	for _, input := range []struct {
		location string
		quotes   []FXQuote
	}{{"history", batch.History.FXQuoteFacts}, {"portfolio", batch.History.Portfolio.FXQuotes}} {
		seen := map[FXQuoteID]bool{}
		for _, q := range input.quotes {
			fp, e := ReviewHistoricalFingerprint(ReviewHistoricalBatch{History: HistoricalSnapshotBatch{FXQuoteFacts: []FXQuote{q}}})
			if e != nil {
				return nil, e
			}
			if seen[q.ID] || q.ID == "" {
				out.Eligible = false
			}
			seen[q.ID] = true
			if input.location == "history" {
				facts[q.ID] = fp
			} else if facts[q.ID] != fp {
				out.Eligible = false
			}
			if out.Base == "" {
				out.Base, out.Quote = q.BaseCurrency, q.QuoteCurrency
				out.SourceKey, out.SourcePolicyVersion, out.TimestampBasis = q.SourceKey, q.SourcePolicyVersion, q.TimestampBasis
			}
			// Agent reference selection is gated by the civil label, never by the
			// timestamp branch. Keep manual/provider quality/routing outside v1.
			_, dateErr := time.Parse(time.DateOnly, q.EffectiveDate)
			_, baseErr := ParseSupportedCurrency(string(q.BaseCurrency))
			_, quoteErr := ParseSupportedCurrency(string(q.QuoteCurrency))
			if dateErr != nil || baseErr != nil || quoteErr != nil || q.BaseCurrency == q.QuoteCurrency || q.BaseCurrency != out.Base || q.QuoteCurrency != out.Quote || q.SourceKey != out.SourceKey || q.SourcePolicyVersion != out.SourcePolicyVersion || q.TimestampBasis != out.TimestampBasis || q.SourceKind != QuoteSourceAgent || q.ObservationKind != "daily_reference" || q.ValueEffectiveAt.IsZero() || q.SourcePolicyVersion == "" || q.TimestampBasis == "" {
				out.Eligible = false
			}
			k := key{input.location, q.BaseCurrency, q.QuoteCurrency, q.EffectiveDate}
			selection := q
			selection.Rate = FxRate{}
			selection.Revision = 0
			selectionFP, e := ReviewHistoricalFingerprint(ReviewHistoricalBatch{History: HistoricalSnapshotBatch{FXQuoteFacts: []FXQuote{selection}}})
			if e != nil {
				return nil, e
			}
			groups[k] = append(groups[k], ReviewFXCandidateSeal{q.ID, fp, selectionFP})
		}
	}
	if len(facts) == 0 {
		out.Eligible = false
	}
	for k, candidates := range groups {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
		out.Buckets = append(out.Buckets, ReviewFXDaySeal{k.location, k.base, k.quote, k.on, candidates})
	}
	sort.Slice(out.Buckets, func(i, j int) bool {
		a, b := out.Buckets[i], out.Buckets[j]
		if a.Location != b.Location {
			return a.Location < b.Location
		}
		if a.Base != b.Base {
			return a.Base < b.Base
		}
		if a.Quote != b.Quote {
			return a.Quote < b.Quote
		}
		return a.On < b.On
	})
	return out, nil
}

// PlanReviewFXInvalidation compares complete authenticated inventories before
// consulting journal claims. Every changed historical candidate needs exact
// OLD/NEW pair/date/ID coverage. Date/pair moves and mixed families fail closed.
// Portfolio is an independently sealed subset of history; replacing its latest
// view cannot introduce a new historical candidate. Daily review explicitly
// replaces portfolio.FXQuotes with the captured complete historical facts.
func PlanReviewFXInvalidation(changes []ReviewSourceChange, before, after *ReviewFXDescription, capture ReviewProjectionCapture) ReviewInvalidationPlan {
	fail := ReviewInvalidationPlan{Full: true, Reason: "unsupported_fx_dependency"}
	if before == nil || after == nil || before.Version != ReviewFXDescriptionVersion || after.Version != ReviewFXDescriptionVersion || !before.Eligible || !after.Eligible || before.NonFXFingerprint == "" || before.NonFXFingerprint != after.NonFXFingerprint || before.Base != after.Base || before.Quote != after.Quote || before.SourceKey != after.SourceKey || before.SourcePolicyVersion != after.SourcePolicyVersion || before.TimestampBasis != after.TimestampBasis {
		return fail
	}
	// All non-identity conversion targets use the one direct pair. This rejects
	// inverse/cross routes, including historical instruments outside current scope.
	for _, instrument := range capture.Historical.History.Portfolio.Instruments {
		if instrument.QuoteCurrency != after.Base && instrument.QuoteCurrency != after.Quote {
			return fail
		}
	}
	var directTargets func(ReviewPredicateNode) bool
	directTargets = func(n ReviewPredicateNode) bool {
		if n.Leaf != nil && n.Leaf.Currency != "" && n.Leaf.Currency != after.Quote {
			return false
		}
		for _, child := range append(append([]ReviewPredicateNode{}, n.All...), n.Any...) {
			if !directTargets(child) {
				return false
			}
		}
		return true
	}
	for _, rule := range capture.Rules {
		if !directTargets(rule.Spec.Predicate) {
			return fail
		}
	}
	type entry struct {
		scope       ReviewFXDependency
		fingerprint string
		selection   string
	}
	inventory := func(d *ReviewFXDescription) map[FXQuoteID]entry {
		m := map[FXQuoteID]entry{}
		for _, b := range d.Buckets {
			if b.Location != "history" {
				continue
			}
			for _, c := range b.Candidates {
				m[c.ID] = entry{ReviewFXDependency{c.ID, b.Base, b.Quote, b.On}, c.Fingerprint, c.SelectionFingerprint}
			}
		}
		return m
	}
	old, current := inventory(before), inventory(after)
	// Compare both complete bucket sets: every portfolio key and exact payload
	// must resolve to the corresponding complete history candidate in that
	// snapshot. A latest-view membership change needs no historical dirty date;
	// changed/inserted/deleted underlying candidates do, checked below.
	portfolioCovered := func(d *ReviewFXDescription, history map[FXQuoteID]entry) bool {
		for _, b := range d.Buckets {
			if b.Location == "history" {
				continue
			}
			if b.Location != "portfolio" {
				return false
			}
			for _, candidate := range b.Candidates {
				e, ok := history[candidate.ID]
				if !ok || e.scope.Base != b.Base || e.scope.Quote != b.Quote || e.scope.On != b.On || e.fingerprint != candidate.Fingerprint || e.selection != candidate.SelectionFingerprint {
					return false
				}
			}
		}
		return true
	}
	if !portfolioCovered(before, old) || !portfolioCovered(after, current) {
		return fail
	}
	covered := map[FXQuoteID]bool{}
	from := ""
	for _, c := range changes {
		if c.Table == "history_snapshot_state" && c.Operation == "update" {
			continue
		}
		if c.Table != "fx_quotes" {
			return fail
		}
		switch c.Operation {
		case "insert":
			if c.OldFX != nil || c.NewFX == nil {
				return fail
			}
		case "delete":
			if c.NewFX != nil || c.OldFX == nil {
				return fail
			}
		case "update":
			if c.OldFX == nil || c.NewFX == nil || *c.OldFX != *c.NewFX {
				return fail
			}
		default:
			return fail
		}
		var id FXQuoteID
		for _, side := range []struct {
			scope   *ReviewFXDependency
			entries map[FXQuoteID]entry
		}{{c.OldFX, old}, {c.NewFX, current}} {
			if side.scope == nil {
				continue
			}
			e, ok := side.entries[side.scope.ID]
			if !ok || e.scope != *side.scope {
				return fail
			}
			if id != "" && id != side.scope.ID {
				return fail
			}
			id = side.scope.ID
			if from == "" || side.scope.On < from {
				from = side.scope.On
			}
		}
		// v1 accepts one mutation per ID; intermediate states need a separate proof.
		if covered[id] {
			return fail
		}
		_, had := old[id]
		_, has := current[id]
		if (c.Operation == "insert" && had) || (c.Operation == "delete" && has) {
			return fail
		}
		if c.Operation == "update" && old[id].selection != current[id].selection {
			return fail
		}
		covered[id] = true
	}
	changed := false
	for id, e := range old {
		if n, ok := current[id]; !ok || e != n {
			if !covered[id] {
				return fail
			}
			changed = true
		}
	}
	for id, e := range current {
		if o, ok := old[id]; !ok || e != o {
			if !covered[id] {
				return fail
			}
			changed = true
		}
	}
	if !changed || from == "" {
		return fail
	}
	plan := ReviewInvalidationPlan{Reason: "direct_fx_suffix", Dirty: []ReviewDirtyPair{}}
	// Invalidate the complete known instrument universe for every rule. The reader
	// adds explicit/evaluated/open-episode targets too; no dependency may be skipped.
	ids := map[InstrumentID]bool{}
	for _, i := range capture.Historical.History.Portfolio.Instruments {
		ids[i.ID] = true
	}
	for _, r := range capture.Rules {
		for _, id := range r.Spec.Target.InstrumentIDs {
			ids[id] = true
		}
		for _, id := range capture.EvaluatedInstruments[r.ID] {
			ids[id] = true
		}
	}
	for _, e := range capture.OpenEpisodes {
		ids[e.InstrumentID] = true
	}
	for _, r := range capture.Rules {
		for id := range ids {
			plan.Dirty = append(plan.Dirty, ReviewDirtyPair{r.ID, id, from})
		}
	}
	sort.Slice(plan.Dirty, func(i, j int) bool {
		a, b := plan.Dirty[i], plan.Dirty[j]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.InstrumentID < b.InstrumentID
	})
	return plan
}
