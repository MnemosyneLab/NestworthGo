package domain

import (
	"sort"
	"time"
)

const ReviewFXDescriptionVersion = "direct-agent-import-fx-v2"

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
	ProvenanceVersion                              string
	Provenance                                     []ReviewFXProvenanceSeal
}
type ReviewFXCandidateSeal struct {
	ID          FXQuoteID
	Fingerprint string
	Supersedes  FXQuoteID
}
type ReviewFXDaySeal struct {
	Location    string
	Base, Quote CurrencyCode
	On          string
	Candidates  []ReviewFXCandidateSeal
}

const ReviewFXProvenanceVersion = "complete-agent-records-v1"

type ReviewFXProvenanceSeal struct {
	ID                     string
	TargetType, Operation  string
	QuoteID, TargetQuoteID FXQuoteID
	Fingerprint            string
}
type ReviewFXDependency struct {
	ID    FXQuoteID    `json:"id"`
	Base  CurrencyCode `json:"base_currency"`
	Quote CurrencyCode `json:"quote_currency"`
	On    string       `json:"effective_date"`
}
type ReviewFXProvenanceDependency struct {
	ID            string    `json:"id"`
	TargetType    string    `json:"target_type"`
	QuoteID       FXQuoteID `json:"quote_id"`
	TargetQuoteID FXQuoteID `json:"target_quote_id"`
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
			// timestamp branch. Keep manual/provider quality/routing outside this version.
			_, dateErr := time.Parse(time.DateOnly, q.EffectiveDate)
			_, baseErr := ParseSupportedCurrency(string(q.BaseCurrency))
			_, quoteErr := ParseSupportedCurrency(string(q.QuoteCurrency))
			if dateErr != nil || baseErr != nil || quoteErr != nil || q.BaseCurrency == q.QuoteCurrency || q.BaseCurrency != out.Base || q.QuoteCurrency != out.Quote || q.SourceKey != out.SourceKey || q.SourcePolicyVersion != out.SourcePolicyVersion || q.TimestampBasis != out.TimestampBasis || q.SourceKind != QuoteSourceAgent || q.ObservationKind != "daily_reference" || q.ValueEffectiveAt.IsZero() || q.SourcePolicyVersion == "" || q.TimestampBasis == "" {
				out.Eligible = false
			}
			k := key{input.location, q.BaseCurrency, q.QuoteCurrency, q.EffectiveDate}
			supersedes := FXQuoteID("")
			if q.SupersedesQuoteID != nil {
				supersedes = FXQuoteID(*q.SupersedesQuoteID)
			}
			groups[k] = append(groups[k], ReviewFXCandidateSeal{q.ID, fp, supersedes})
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
// consulting journal claims. Every active-set change needs exact captured
// append/correct/retract provenance plus matching journal coverage. Date/pair
// moves and unrelated source families fail closed.
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
		supersedes  FXQuoteID
	}
	inventory := func(d *ReviewFXDescription) map[FXQuoteID]entry {
		m := map[FXQuoteID]entry{}
		for _, b := range d.Buckets {
			if b.Location != "history" {
				continue
			}
			for _, c := range b.Candidates {
				m[c.ID] = entry{ReviewFXDependency{c.ID, b.Base, b.Quote, b.On}, c.Fingerprint, c.Supersedes}
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
				if !ok || e.scope.Base != b.Base || e.scope.Quote != b.Quote || e.scope.On != b.On || e.fingerprint != candidate.Fingerprint || e.supersedes != candidate.Supersedes {
					return false
				}
			}
		}
		return true
	}
	if !portfolioCovered(before, old) || !portfolioCovered(after, current) {
		return fail
	}

	// The production importer appends immutable replacement/withdrawal provenance;
	// it never updates/deletes quote rows. Cover active-set changes through exactly
	// those independently captured records, retaining all prior provenance bytes.
	if before.ProvenanceVersion != ReviewFXProvenanceVersion || after.ProvenanceVersion != ReviewFXProvenanceVersion {
		return fail
	}
	priorRecords := map[string]ReviewFXProvenanceSeal{}
	currentRecords := map[string]ReviewFXProvenanceSeal{}
	for _, r := range before.Provenance {
		if r.ID == "" || r.Fingerprint == "" {
			return fail
		}
		if _, ok := priorRecords[r.ID]; ok {
			return fail
		}
		priorRecords[r.ID] = r
	}
	for _, r := range after.Provenance {
		if r.ID == "" || r.Fingerprint == "" {
			return fail
		}
		if _, ok := currentRecords[r.ID]; ok {
			return fail
		}
		currentRecords[r.ID] = r
	}
	for id, r := range priorRecords {
		if currentRecords[id] != r {
			return fail
		}
	}
	// Active candidates need a corresponding creation record and no withdrawal.
	activeProven := func(entries map[FXQuoteID]entry, records map[string]ReviewFXProvenanceSeal) bool {
		origins, withdrawals := map[FXQuoteID]int{}, map[FXQuoteID]int{}
		for _, r := range records {
			if r.TargetType != "fx" {
				continue
			}
			if r.QuoteID != "" {
				origins[r.QuoteID]++
			}
			if r.TargetQuoteID != "" {
				withdrawals[r.TargetQuoteID]++
			}
		}
		for id := range entries {
			if origins[id] != 1 || withdrawals[id] != 0 {
				return false
			}
		}
		return true
	}
	if !activeProven(old, priorRecords) || !activeProven(current, currentRecords) {
		return fail
	}
	recordClaims := map[string]bool{}
	quoteClaims := map[FXQuoteID]bool{}
	for _, c := range changes {
		if c.Table == "history_snapshot_state" && c.Operation == "update" {
			continue
		}
		switch c.Table {
		case "fx_quotes":
			if c.Operation != "insert" || c.OldFX != nil || c.NewFX == nil {
				return fail
			}
			e, ok := current[c.NewFX.ID]
			if !ok || e.scope != *c.NewFX {
				return fail
			}
			if _, had := old[c.NewFX.ID]; had || quoteClaims[c.NewFX.ID] {
				return fail
			}
			quoteClaims[c.NewFX.ID] = true
		case "agent_quote_records":
			if c.Operation != "insert" || c.OldAgent != nil || c.NewAgent == nil {
				return fail
			}
			claim := c.NewAgent
			r, ok := currentRecords[claim.ID]
			if !ok || r.TargetType != "fx" || r.TargetType != claim.TargetType || r.QuoteID != claim.QuoteID || r.TargetQuoteID != claim.TargetQuoteID || recordClaims[r.ID] {
				return fail
			}
			if _, had := priorRecords[r.ID]; had {
				return fail
			}
			recordClaims[r.ID] = true
		default:
			return fail
		}
	}
	added, removed := map[FXQuoteID]bool{}, map[FXQuoteID]bool{}
	from := ""
	touch := func(on string) {
		if from == "" || on < from {
			from = on
		}
	}
	for id, r := range currentRecords {
		if _, had := priorRecords[id]; had {
			continue
		}
		if !recordClaims[id] || r.TargetType != "fx" {
			return fail
		}
		switch r.Operation {
		case "append":
			if r.QuoteID == "" || r.TargetQuoteID != "" {
				return fail
			}
		case "correct":
			if r.QuoteID == "" || r.TargetQuoteID == "" {
				return fail
			}
		case "retract":
			if r.QuoteID != "" || r.TargetQuoteID == "" {
				return fail
			}
		default:
			return fail
		}
		if r.QuoteID != "" {
			e, ok := current[r.QuoteID]
			if !ok || !quoteClaims[r.QuoteID] || added[r.QuoteID] {
				return fail
			}
			if _, had := old[r.QuoteID]; had {
				return fail
			}
			if e.supersedes != r.TargetQuoteID {
				return fail
			}
			added[r.QuoteID] = true
			touch(e.scope.On)
		}
		if r.TargetQuoteID != "" {
			e, ok := old[r.TargetQuoteID]
			if !ok || removed[r.TargetQuoteID] {
				return fail
			}
			if _, has := current[r.TargetQuoteID]; has {
				return fail
			}
			// Correction cannot change pair/date. All other policy fields are already
			// constrained by the common description; quote IDs/timestamps legitimately
			// differ for an append-only replacement.
			if r.Operation == "correct" && current[r.QuoteID].scope.On != e.scope.On {
				return fail
			}
			removed[r.TargetQuoteID] = true
			touch(e.scope.On)
		}
	}
	for id, e := range old {
		if n, ok := current[id]; ok {
			if e != n {
				return fail
			}
		} else if !removed[id] {
			return fail
		}
	}
	for id := range current {
		if _, had := old[id]; !had && !added[id] {
			return fail
		}
	}
	for id := range quoteClaims {
		if !added[id] {
			return fail
		}
	}
	if from == "" {
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
