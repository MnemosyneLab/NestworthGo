package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// UpcomingActions is an ephemeral projection. It has no completion or write state.
type UpcomingActions struct {
	AsOf                                            time.Time
	LocalDate, Timezone, WindowStartOn, WindowEndOn string
	BaseCurrency                                    CurrencyCode
	Completeness                                    CompletenessStatus
	Counts                                          UpcomingCounts
	Items                                           []UpcomingAction
	DataChecks                                      []UpcomingDataCheck
	ScopeWarnings                                   []string
}
type UpcomingCounts struct{ Total, Attention, ReceiptDue, ReservationIssues, Scheduled, Eligible, DataChecks, ScopeNotes int }
type UpcomingTarget struct {
	Kind          string
	ProductID     *ProductContractID
	SourceRef     *LiquiditySourceRef
	ReservationID *LiquidityReservationID
}
type UpcomingAction struct {
	ID, OccurrenceKey, Kind, Phase, Group                   string
	Priority                                                int
	SortOn                                                  *string
	AccountID                                               AccountID
	AccountName                                             *string
	ProductID                                               *ProductContractID
	ProductKind                                             *ProductKind
	SourceKey                                               string
	SourceRef                                               *LiquiditySourceRef
	ReservationIDs, ReservationLabels                       []string
	DisplayName, AnchorKind                                 string
	AnchorOn, MaturityOn, EligibilityOn, EstimatedReceiptOn *string
	ReceiptBasis                                            string
	ReferenceOn                                             *string
	Amount, RequestedReserve                                *Money
	AmountRole, AmountStatus, CoverageStatus                string
	ReasonCodes, Assumptions                                []string
	PrimaryTarget                                           UpcomingTarget
	Targets                                                 []UpcomingTarget
	DaysFromToday                                           *int
	PolicyRevision                                          int
}
type UpcomingDataCheck struct {
	ID, SourceKey, Category string
	ProductID               *ProductContractID
	DisplayName             string
	ReasonCodes             []string
	RelatedActionID         *string
	AnchorOn                *string
	CoverageStatus          string
	Target                  UpcomingTarget
	Targets                 []UpcomingTarget
	ReferenceOn             *string
}

// ProjectUpcomingActions classifies an already evaluated normal-route snapshot.
// It never reads a clock, repository, provider, or writes/mutates its inputs.
func ProjectUpcomingActions(overview LiquidityOverview, sources []LiquiditySource, snapshot LiquiditySnapshot) (UpcomingActions, error) {
	end, err := AddCalendarDays(overview.LocalDate, 30)
	if err != nil {
		return UpcomingActions{}, err
	}
	out := UpcomingActions{AsOf: overview.AsOf, LocalDate: overview.LocalDate, Timezone: overview.Timezone,
		WindowStartOn: overview.LocalDate, WindowEndOn: end, BaseCurrency: overview.BaseCurrency, Completeness: StatusComplete,
		Items: []UpcomingAction{}, DataChecks: []UpcomingDataCheck{}, ScopeWarnings: []string{"normal_routes_only"}}
	raw := make(map[string]LiquiditySource, len(sources))
	for _, s := range sources {
		raw[s.Ref.Key()] = s
	}
	accounts := make(map[AccountID]Account, len(snapshot.Portfolio.Accounts))
	for _, r := range snapshot.Portfolio.Accounts {
		accounts[r.Account.ID] = r.Account
	}
	holdings := make(map[HoldingID]Holding, len(snapshot.Portfolio.Holdings))
	for _, h := range snapshot.Portfolio.Holdings {
		holdings[h.ID] = h
	}
	unresolved := make(map[LiquidityReservationID]bool, len(overview.UnresolvedReservations))
	reservations := make(map[string][]LiquidityReservation)
	for _, u := range overview.UnresolvedReservations {
		unresolved[u.Reservation.ID] = true
	}
	for _, r := range snapshot.Reservations {
		if r.Active() && !unresolved[r.ID] {
			reservations[r.Source.Key()] = append(reservations[r.Source.Key()], r)
		}
	}
	for k, rs := range reservations {
		sort.Slice(rs, func(i, j int) bool { return rs[i].ID.String() < rs[j].ID.String() })
		reservations[k] = rs
	}
	checks := make(map[string]UpcomingDataCheck)
	actionBySource := make(map[string]int)
	addCheck := func(s LiquiditySourceResult, category, coverage string, reasons []string, anchor *string, target UpcomingTarget) {
		if len(reasons) == 0 {
			return
		}
		id := "ua1/check/" + s.SourceKey + "/" + category
		c := checks[id]
		c.ID = id
		c.SourceKey = s.SourceKey
		c.Category = category
		c.ProductID = s.ProductID
		c.DisplayName = s.DisplayName
		if c.Target.Kind == "" {
			c.Target = target
		}
		c.Targets = appendUpcomingTarget(c.Targets, target)
		c.ReasonCodes = upcomingStrings(append(c.ReasonCodes, reasons...))
		c.CoverageStatus = coverage
		if coverage != "" {
			c.ReferenceOn = &end
		}
		if anchor != nil {
			c.AnchorOn = anchor
		}
		checks[id] = c
		if category == "data_issue" {
			out.Completeness = StatusPartial
		}
	}
	for _, u := range overview.UnresolvedReservations {
		r := u.Reservation
		ref := r.Source
		id := r.ID
		amount := r.Amount
		reason := "source_missing"
		if a, ok := accounts[ref.AccountID]; ok {
			switch {
			case a.ArchivedAt != nil:
				reason = "source_archived"
			case a.IsLiability():
				reason = "source_liability"
			default:
				if s, ok := raw[ref.Key()]; ok {
					switch {
					case s.QuantityZero:
						reason = "source_zero_quantity"
					case !liquiditySourcePositiveOrUnknown(s):
						reason = "source_non_positive"
					default:
						reason = "source_unavailable"
					}
				} else if ref.HoldingID != nil {
					if h, ok := holdings[*ref.HoldingID]; ok {
						if h.ArchivedAt != nil {
							reason = "source_archived"
						} else if h.Quantity.IsZero() {
							reason = "source_zero_quantity"
						}
					}
				}
			}
		}
		item := UpcomingAction{ID: "ua1/reservation/" + id.String() + "/unresolved", Kind: "reservation_unresolved", Phase: "current_issue", Group: "attention", Priority: 1,
			AccountID: ref.AccountID, SourceKey: ref.Key(), SourceRef: &ref, DisplayName: r.Label, AnchorKind: "none", ReceiptBasis: "unknown",
			ReservationIDs: []string{id.String()}, ReservationLabels: []string{r.Label}, Amount: &amount, AmountRole: "reservation_requested", AmountStatus: "known", CoverageStatus: "source_unresolved",
			ReasonCodes: []string{reason}, Assumptions: []string{}, PrimaryTarget: UpcomingTarget{Kind: "reservations", ReservationID: &id}}
		if a, ok := accounts[ref.AccountID]; ok {
			name := a.Name
			item.AccountName = &name
		}
		item.OccurrenceKey = upcomingHash([]string{item.ID})
		out.Items = append(out.Items, item)
	}
	for _, s := range overview.Sources {
		r, exists := raw[s.SourceKey]
		if !exists {
			continue
		}
		ref := s.Ref
		target := UpcomingTarget{Kind: "source_policy", SourceRef: &ref}
		if r.Contract != nil {
			id := r.Contract.ID
			target = UpcomingTarget{Kind: "product", ProductID: &id}
		}
		// Archive/liability/quantity exclusions cannot be bypassed by contractual dates.
		if r.Archived || r.Liability || r.QuantityZero {
			if r.QuantityZero && r.Contract != nil && r.Contract.State == ProductStateOpen {
				addCheck(s, "data_issue", "", []string{"invalid_product_quantity"}, nil, UpcomingTarget{Kind: "data_health"})
			}
			continue
		}
		managed := r.Managed && r.Contract != nil
		if managed && (r.Contract.State != ProductStateOpen || (r.Contract.Kind != ProductTermDeposit && r.Contract.Kind != ProductLockedProduct)) {
			continue
		}
		policy := s.Policy
		if policy != nil && policy.AccessKind == AccessExcluded {
			out.ScopeWarnings = append(out.ScopeWarnings, "excluded_sources_omitted")
			var anchor *string
			if r.ExplicitPolicy != nil {
				anchor = policy.UnlockOn
			}
			if managed {
				if r.Contract.MaturityOn != nil {
					anchor = r.Contract.MaturityOn
				} else if anchor == nil {
					anchor = policy.ReceiptOnOverride
				}
			}
			if anchor != nil {
				addCheck(s, "scope_note", "", []string{"schedule_excluded"}, anchor, target)
			}
		} else {
			item := upcomingSchedule(s, r, overview.LocalDate, end)
			reasons := upcomingMissing(s, r, overview.BaseCurrency)
			if item != nil {
				if a, ok := accounts[item.AccountID]; ok {
					name := a.Name
					item.AccountName = &name
				}
				item.ReasonCodes = upcomingStrings(reasons)
				if item.Amount != nil && len(reasons) > 0 {
					item.AmountStatus = "partial"
				}
				if len(reasons) > 0 {
					out.Completeness = StatusPartial
				}
				actionBySource[s.SourceKey] = len(out.Items)
				out.Items = append(out.Items, *item)
			} else if managed || len(reservations[s.SourceKey]) > 0 || (r.ExplicitPolicy != nil && r.ExplicitPolicy.UnlockOn != nil) {
				addCheck(s, "data_issue", "", reasons, nil, target)
			}
		}
		rs := reservations[s.SourceKey]
		if len(rs) == 0 {
			continue
		}
		coverage := "amount_unknown"
		category := "data_issue"
		reason := "reservation_coverage_unknown"
		mismatch := false
		for _, res := range rs {
			if res.Currency != s.NativeCurrency || res.Amount.Currency() != s.NativeCurrency {
				mismatch = true
			}
		}
		var bucket *LiquidityBucketResult
		for i := range s.BucketResults {
			if s.BucketResults[i].HorizonOn == end {
				bucket = &s.BucketResults[i]
				break
			}
		}
		switch {
		case mismatch:
			coverage = "currency_mismatch"
			reason = "currency_mismatch"
		case policy != nil && policy.AccessKind == AccessExcluded:
			coverage = "excluded_source"
			category = "scope_note"
			reason = "reservation_excluded"
		case s.DueUnconfirmed:
			coverage = "due_route_suppressed"
			category = "information"
			reason = "due_route_suppressed"
		case s.NormalRoute != nil && ((s.NormalRoute.EligibleOn != nil && *s.NormalRoute.EligibleOn > end) || (s.NormalRoute.ReceiptOn != nil && *s.NormalRoute.ReceiptOn > end)):
			coverage = "not_available_by_E"
			category = "information"
			reason = "not_available_by_E"
		case bucket != nil && bucket.SelectedRoute != nil && bucket.SelectedRoute.Kind == RouteNormal && nativeResultComplete(*bucket) && bucket.ReserveShortfallNative != nil:
			if bucket.ReserveShortfallNative.Amount().IsPositive() {
				coverage = "short"
			} else {
				coverage = "covered"
			}
		}
		if coverage == "covered" {
			continue
		}
		reserveTarget := UpcomingTarget{Kind: "reservations", SourceRef: &ref}
		if coverage != "short" {
			addCheck(s, category, coverage, []string{reason}, nil, reserveTarget)
			continue
		}
		requested := s.ReservationRequested
		item := UpcomingAction{ID: "ua1/reserve/" + s.SourceKey + "/" + s.NativeCurrency.String(), Kind: "reservation_shortfall", Phase: "current_issue", Group: "attention", Priority: 1,
			AccountID: s.AccountID, ProductID: s.ProductID, SourceKey: s.SourceKey, SourceRef: &ref, DisplayName: s.DisplayName, AnchorKind: "none", ReceiptBasis: "unknown", ReferenceOn: &end,
			Amount: bucket.ReserveShortfallNative, RequestedReserve: &requested, AmountRole: "reserve_shortfall", AmountStatus: "known", CoverageStatus: coverage,
			ReservationIDs: []string{}, ReservationLabels: []string{}, ReasonCodes: []string{}, Assumptions: []string{}, PrimaryTarget: reserveTarget}
		for _, res := range rs {
			item.ReservationIDs = append(item.ReservationIDs, res.ID.String())
			item.ReservationLabels = append(item.ReservationLabels, res.Label)
		}
		if s.NativeCurrency != overview.BaseCurrency && bucket.NetBase == nil {
			item.ReasonCodes = []string{"missing_fx"}
			item.AmountStatus = "partial"
			out.Completeness = StatusPartial
		}
		if a, ok := accounts[item.AccountID]; ok {
			name := a.Name
			item.AccountName = &name
		}
		item.OccurrenceKey = upcomingHash([]string{item.ID})
		if _, ok := actionBySource[s.SourceKey]; !ok {
			actionBySource[s.SourceKey] = len(out.Items)
		}
		out.Items = append(out.Items, item)
	}
	// Merge checks into related actions to avoid counting the same data gap twice.
	for _, c := range checks {
		if c.Category == "data_issue" {
			if i, ok := actionBySource[c.SourceKey]; ok {
				out.Items[i].ReasonCodes = upcomingStrings(append(out.Items[i].ReasonCodes, c.ReasonCodes...))
				for _, target := range c.Targets {
					out.Items[i].Targets = appendUpcomingTarget(out.Items[i].Targets, target)
				}
				if c.CoverageStatus != "" {
					out.Items[i].CoverageStatus = c.CoverageStatus
					out.Items[i].ReferenceOn = c.ReferenceOn
				}
				continue
			}
		}
		out.DataChecks = append(out.DataChecks, c)
	}
	for i := range out.Items {
		item := &out.Items[i]
		item.Targets = appendUpcomingTarget(item.Targets, item.PrimaryTarget)
		sortUpcomingTargets(item.Targets)
		item.ReasonCodes = upcomingStrings(item.ReasonCodes)
		if item.Amount != nil && len(item.ReasonCodes) > 0 {
			item.AmountStatus = "partial"
		}
		item.Assumptions = upcomingStrings(item.Assumptions)
		if item.Group == "eligible" {
			out.Counts.Eligible++
			continue
		}
		out.Counts.Total++
		if item.Group == "attention" {
			out.Counts.Attention++
		} else {
			out.Counts.Scheduled++
		}
		if item.Phase == "receipt_due" {
			out.Counts.ReceiptDue++
		}
		if item.Kind == "reservation_shortfall" || item.Kind == "reservation_unresolved" {
			out.Counts.ReservationIssues++
		}
		for _, a := range item.Assumptions {
			if a == string(AssumptionWeekdaysNoHolidays) {
				out.ScopeWarnings = append(out.ScopeWarnings, "weekdays_no_holidays")
			}
		}
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		sub := func(a UpcomingAction) int {
			if a.Kind == "reservation_shortfall" {
				return 1
			}
			return 0
		}
		if sub(a) != sub(b) {
			return sub(a) < sub(b)
		}
		if upcomingDate(a.SortOn) != upcomingDate(b.SortOn) {
			return upcomingDate(a.SortOn) < upcomingDate(b.SortOn)
		}
		return a.ID < b.ID
	})
	sort.Slice(out.DataChecks, func(i, j int) bool {
		a, b := out.DataChecks[i], out.DataChecks[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.SourceKey != b.SourceKey {
			return a.SourceKey < b.SourceKey
		}
		return a.ID < b.ID
	})
	for i := range out.DataChecks {
		sortUpcomingTargets(out.DataChecks[i].Targets)
	}
	for _, c := range out.DataChecks {
		if c.Category == "data_issue" {
			out.Counts.DataChecks++
		} else {
			out.Counts.ScopeNotes++
		}
	}
	out.ScopeWarnings = upcomingStrings(out.ScopeWarnings)
	return out, nil
}

func appendUpcomingTarget(targets []UpcomingTarget, target UpcomingTarget) []UpcomingTarget {
	if target.Kind == "" {
		return targets
	}
	key, _ := jsoncodec.Marshal(target)
	for _, existing := range targets {
		v, _ := jsoncodec.Marshal(existing)
		if string(v) == string(key) {
			return targets
		}
	}
	return append(targets, target)
}

func sortUpcomingTargets(targets []UpcomingTarget) {
	sort.Slice(targets, func(i, j int) bool {
		a, _ := jsoncodec.Marshal(targets[i])
		b, _ := jsoncodec.Marshal(targets[j])
		return string(a) < string(b)
	})
}

func upcomingSchedule(s LiquiditySourceResult, raw LiquiditySource, today, end string) *UpcomingAction {
	managed := raw.Managed && raw.Contract != nil
	p := s.Policy
	var maturity, unlock, override *string
	if managed {
		maturity = raw.Contract.MaturityOn
	}
	if p != nil {
		unlock = p.UnlockOn
		override = p.ReceiptOnOverride
	}
	knownAccess := p != nil && p.AccessKind != AccessUnknown && p.AccessKind != AccessExcluded
	anchor := maturity
	anchorKind := "maturity"
	if anchor == nil && knownAccess && raw.ExplicitPolicy != nil {
		anchor = unlock
		anchorKind = "eligibility"
	}
	if anchor == nil && managed && knownAccess {
		anchor = override
		anchorKind = "scheduled_receipt"
	}
	if anchor == nil {
		return nil
	}
	normal := s.NormalRoute
	timingKnown := knownAccess && normal != nil && normal.EligibleOn != nil && normal.ReceiptOn != nil
	item := UpcomingAction{ID: "ua1/source/" + s.SourceKey + "/normal", Kind: "source_access", Group: "scheduled", Priority: 4, AccountID: s.AccountID,
		ProductID: s.ProductID, SourceKey: s.SourceKey, DisplayName: s.DisplayName, AnchorKind: anchorKind, AnchorOn: anchor, MaturityOn: maturity, EligibilityOn: unlock,
		ReceiptBasis: "unknown", AmountRole: "unknown", AmountStatus: "unknown", ReservationIDs: []string{}, ReservationLabels: []string{}, ReasonCodes: []string{}, Assumptions: []string{}}
	ref := s.Ref
	item.SourceRef = &ref
	item.PrimaryTarget = UpcomingTarget{Kind: "source_policy", SourceRef: &ref}
	identity := s.SourceKey
	if managed {
		id := raw.Contract.ID
		kind := raw.Contract.Kind
		item.ProductID = &id
		item.ProductKind = &kind
		item.ID = "ua1/product/" + id.String() + "/normal"
		item.Kind = "product_schedule"
		identity = id.String()
		item.PrimaryTarget = UpcomingTarget{Kind: "product", ProductID: &id}
	}
	item.OccurrenceKey = upcomingHash([]string{identity, anchorKind, upcomingDate(maturity), upcomingDate(unlock), upcomingDate(override)})
	if p != nil {
		item.PolicyRevision = p.Revision
	}
	if normal != nil {
		// Ordinary overrides remain receipt details in Available Funds, not request-date estimates here.
		if managed || override == nil {
			item.EstimatedReceiptOn = normal.ReceiptOn
		}
		item.Amount = normal.NetNative
		item.AmountRole = "estimated_net_proceeds"
		if normal.NetNative != nil {
			item.AmountStatus = "known"
		}
		for _, a := range normal.Assumptions {
			item.Assumptions = append(item.Assumptions, string(a))
		}
	}
	if timingKnown && (managed || override == nil) {
		switch {
		case maturity != nil || (managed && override != nil):
			item.ReceiptBasis = "fixed_schedule"
		case unlock != nil && *unlock >= today:
			item.ReceiptBasis = "request_on_eligible_date"
		default:
			item.ReceiptBasis = "request_today"
		}
	}
	switch {
	case s.DueUnconfirmed && managed && normal != nil && normal.ReceiptOn != nil:
		item.Phase = "receipt_due"
		item.Group = "attention"
		item.Priority = 0
		item.SortOn = normal.ReceiptOn
	case managed && maturity != nil && !timingKnown:
		if *maturity > end {
			return nil
		}
		item.Phase = "blocked"
		item.SortOn = maturity
		if *maturity <= today {
			item.Group = "attention"
			item.Priority = 2
		}
	case !knownAccess:
		return nil
	case *anchor == today:
		item.Phase = "today"
		item.Priority = 3
		item.SortOn = anchor
	case maturity != nil && *maturity < today && normal != nil && normal.ReceiptOn != nil && *normal.ReceiptOn > today:
		item.Phase = "awaiting_receipt"
		item.SortOn = normal.ReceiptOn
	case managed && maturity == nil && override != nil && timingKnown && *override > today && (unlock != nil && *unlock < today || unlock == nil && *override <= end):
		item.Phase = "scheduled_receipt"
		item.SortOn = normal.ReceiptOn
	case *anchor > today && *anchor <= end:
		item.Phase = "upcoming"
		item.SortOn = anchor
	case maturity == nil && anchorKind == "eligibility" && *anchor < today && (!managed || override == nil):
		item.Phase = "eligible"
		item.Group = "eligible"
		item.Priority = 5
		item.SortOn = anchor
	default:
		return nil
	}
	if item.SortOn != nil {
		var days int
		if *item.SortOn >= today {
			days, _ = CivilDaysBetween(today, *item.SortOn)
		} else {
			days, _ = CivilDaysBetween(*item.SortOn, today)
			days = -days
		}
		item.DaysFromToday = &days
	}
	return &item
}
func upcomingMissing(s LiquiditySourceResult, raw LiquiditySource, base CurrencyCode) []string {
	reasons := []string{}
	p := s.Policy
	n := s.NormalRoute
	if p == nil {
		reasons = append(reasons, "missing_policy")
	} else if p.AccessKind == AccessUnknown {
		reasons = append(reasons, "unknown_access")
	}
	if n == nil || n.ReceiptOn == nil {
		reasons = append(reasons, "unknown_receipt_timing")
	}
	if n == nil || n.NetNative == nil {
		reasons = append(reasons, "unknown_amount")
	}
	if p != nil && p.NormalExitFee == nil {
		reasons = append(reasons, "unknown_fee")
	}

	if s.NativeCurrency != base && s.FXEvidence == nil && raw.CurrentNativeValue != nil && !raw.CurrentNativeValue.IsZero() {
		reasons = append(reasons, "missing_fx")
	}
	return upcomingStrings(reasons)
}
func upcomingDate(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func upcomingHash(v []string) string {
	b, _ := jsoncodec.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func upcomingStrings(v []string) []string {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
