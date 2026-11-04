package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const MarketComparisonMaxItems = 500
const marketComparisonInputBytes = 128 << 10

type MarketComparisonItem struct {
	ClientItemID  string `json:"clientItemId"`
	InstrumentID  string `json:"instrumentId,omitempty"`
	BaseCurrency  string `json:"baseCurrency,omitempty"`
	QuoteCurrency string `json:"quoteCurrency,omitempty"`
	Kind          string `json:"kind"`
	Currency      string `json:"currency,omitempty"`
	Value         string `json:"value"`
	Date          string `json:"date,omitempty"`
	QuotedAt      string `json:"quotedAt,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	SourceTitle   string `json:"sourceTitle,omitempty"`
	SourceURL     string `json:"sourceUrl,omitempty"`
}
type MarketComparisonRequest struct {
	Items        []MarketComparisonItem `json:"items"`
	DisplayScale *int                   `json:"displayScale,omitempty"`
}
type MarketComparisonNormalized struct {
	Target   domain.MarketReadTarget `json:"target"`
	Kind     string                  `json:"kind"`
	Basis    string                  `json:"basis"`
	Currency string                  `json:"currency,omitempty"`
	Date     string                  `json:"date,omitempty"`
	QuotedAt string                  `json:"quotedAt"`
	Value    string                  `json:"value"`
}
type MarketComparisonRow struct {
	Ordinal        int                         `json:"ordinal"`
	ClientItemID   string                      `json:"clientItemId"`
	Input          MarketComparisonItem        `json:"input"`
	Normalized     *MarketComparisonNormalized `json:"normalized,omitempty"`
	Status         string                      `json:"status"`
	Reason         string                      `json:"reason,omitempty"`
	MatchID        string                      `json:"matchId,omitempty"`
	DuplicateGroup string                      `json:"duplicateGroup,omitempty"`
	BatchConflict  bool                        `json:"batchConflict"`
}
type MarketComparisonEvidence struct {
	ID                string                  `json:"id"`
	Target            domain.MarketReadTarget `json:"target"`
	Currency          string                  `json:"currency,omitempty"`
	Value             string                  `json:"value"`
	Kind              string                  `json:"kind"`
	Basis             string                  `json:"basis"`
	PriceBasis        string                  `json:"priceBasis,omitempty"`
	Date              string                  `json:"date,omitempty"`
	QuotedAt          time.Time               `json:"quotedAt"`
	CreatedAt         time.Time               `json:"createdAt"`
	TimestampBasis    string                  `json:"timestampBasis,omitempty"`
	SourceKind        domain.QuoteSourceKind  `json:"sourceKind"`
	SourceKey         string                  `json:"sourceKey"`
	State             string                  `json:"state"`
	Revision          int                     `json:"revision"`
	SupersedesQuoteID *string                 `json:"supersedesQuoteId,omitempty"`
	AuditIDs          []string                `json:"auditIds"`
	ConversionJSON    string                  `json:"conversionJson,omitempty"`
	ChartSelected     bool                    `json:"chartSelected"`
}
type MarketComparisonRelation struct {
	EvidenceID     string `json:"evidenceId"`
	Relation       string `json:"relation"`
	Delta          string `json:"delta,omitempty"`
	DifferenceKind string `json:"differenceKind,omitempty"`
	SourceMatch    bool   `json:"sourceMatch"`
}
type MarketComparisonMatch struct {
	ID                   string                     `json:"id"`
	Status               string                     `json:"status"`
	Reason               string                     `json:"reason,omitempty"`
	ActiveEqual          int                        `json:"activeEqual"`
	ActiveDifferent      int                        `json:"activeDifferent"`
	HistoricalComparable int                        `json:"historicalComparable"`
	SourceEvidence       string                     `json:"sourceEvidence"`
	DisplayScale         int                        `json:"displayScale"`
	Relations            []MarketComparisonRelation `json:"relations"`
	TargetKey            string                     `json:"targetKey"`
	Target               MarketComparisonTarget     `json:"-"`
	SelectionImpact      string                     `json:"selectionImpact"`
	SelectionReason      string                     `json:"selectionReason,omitempty"`
}
type MarketComparisonTarget struct {
	DisplayName     string                   `json:"displayName"`
	Key             string                   `json:"key"`
	Policy          string                   `json:"policy"`
	CurrentEvidence *MarketComparisonCurrent `json:"currentEvidence,omitempty"`
	CurrentQuoteID  string                   `json:"currentQuoteId,omitempty"`
	CurrentValue    string                   `json:"currentValue,omitempty"`
}
type MarketComparisonCurrent struct {
	Target     domain.MarketReadTarget `json:"target"`
	Currency   string                  `json:"currency,omitempty"`
	Value      string                  `json:"value"`
	SourceKind domain.QuoteSourceKind  `json:"sourceKind"`
	SourceKey  string                  `json:"sourceKey"`
	Date       string                  `json:"date,omitempty"`
	QuotedAt   time.Time               `json:"quotedAt"`
	PriceBasis string                  `json:"priceBasis,omitempty"`
}
type MarketComparisonResult struct {
	Targets    []MarketComparisonTarget
	CapturedAt time.Time
	InputHash  string
	Results    []MarketComparisonRow
	Matches    []MarketComparisonMatch
	Evidence   []MarketComparisonEvidence
	Audits     []domain.MarketComparisonAudit
}
type marketComparisonRepository interface {
	ReadMarketComparisonSnapshot(context.Context, []domain.MarketComparisonScope, time.Time) (domain.MarketComparisonSnapshot, error)
}

func comparisonItemAgent(i MarketComparisonItem) AgentMarketDataItem {
	return AgentMarketDataItem{InstrumentID: i.InstrumentID, BaseCurrency: i.BaseCurrency, QuoteCurrency: i.QuoteCurrency, Kind: i.Kind, Currency: i.Currency, Value: i.Value, Date: i.Date, QuotedAt: i.QuotedAt, Timezone: i.Timezone}
}
func comparisonTarget(i MarketComparisonItem) (domain.MarketReadTarget, error) {
	t := domain.MarketReadTarget{InstrumentID: i.InstrumentID, BaseCurrency: i.BaseCurrency, QuoteCurrency: i.QuoteCurrency}
	if t.InstrumentID != "" {
		id, err := domain.ParseInstrumentID(t.InstrumentID)
		if err != nil {
			return t, err
		}
		if t.BaseCurrency != "" || t.QuoteCurrency != "" {
			return t, agentDataValidation("target", "choose instrument or FX pair")
		}
		t.InstrumentID = id.String()
		return t, nil
	}
	a, b, err := parseOrientedPair(domain.CurrencyCode(t.BaseCurrency), domain.CurrencyCode(t.QuoteCurrency))
	t.BaseCurrency, t.QuoteCurrency = a.String(), b.String()
	return t, err
}
func comparisonKey(n MarketComparisonNormalized) string {
	when := n.Date
	if n.Kind == "latest" {
		when = n.QuotedAt
	}
	return n.Target.Key() + "|" + n.Kind + "|" + n.Basis + "|" + n.Currency + "|" + when
}
func comparisonBasis(kind, basis string) string {
	if kind == "nav" || basis == "agent_unit_nav_v1" {
		return "unit_nav"
	}
	if basis == "agent_raw_close_v1" || basis == domain.YahooRawClosePriceBasis {
		return "raw_close"
	}
	if kind == "latest" || kind == "realtime" || kind == "manual" {
		return "latest"
	}
	if kind == "daily_reference" {
		return "daily_reference"
	}
	return "unknown"
}

// CompareMarketData is exclusively a local snapshot read. It creates no import
// plan, write receipt, snapshot refresh, provider request or write authorization.
func (s *Service) CompareMarketData(ctx context.Context, in MarketComparisonRequest) (MarketComparisonResult, error) {
	out := MarketComparisonResult{CapturedAt: s.clock(), Results: []MarketComparisonRow{}, Matches: []MarketComparisonMatch{}, Evidence: []MarketComparisonEvidence{}, Audits: []domain.MarketComparisonAudit{}}
	if len(in.Items) < 1 || len(in.Items) > MarketComparisonMaxItems {
		return out, agentDataValidation("items", "requires 1 to 500 rows")
	}
	if in.DisplayScale != nil && (*in.DisplayScale < 0 || *in.DisplayScale > 12) {
		return out, agentDataValidation("displayScale", "must be 0 to 12; display only, never comparison tolerance")
	}
	encoded, err := jsoncodec.Marshal(in)
	if err != nil {
		return out, err
	}
	if len(encoded) > marketComparisonInputBytes {
		return out, &domain.Error{Code: "too_large", Message: "comparison input exceeds 128 KiB"}
	}
	h := sha256.Sum256(encoded)
	out.InputHash = hex.EncodeToString(h[:])
	seen := map[string]bool{}
	scopes := map[string]*domain.MarketComparisonScope{}
	targets := map[int]domain.MarketReadTarget{}
	for j, i := range in.Items {
		if i.ClientItemID == "" || len(i.ClientItemID) > 128 || seen[i.ClientItemID] {
			return out, agentDataValidation("clientItemId", "must be unique, nonempty, at most 128 UTF-8 bytes")
		}
		seen[i.ClientItemID] = true
		row := MarketComparisonRow{Ordinal: j, ClientItemID: i.ClientItemID, Input: i, Status: "invalid"}
		t, e := comparisonTarget(i)
		if e != nil {
			row.Reason = "invalid_target"
			out.Results = append(out.Results, row)
			continue
		}
		if e = comparisonSyntax(i, out.CapturedAt); e != nil {
			row.Reason = e.Error()
			out.Results = append(out.Results, row)
			continue
		}
		if i.Kind == "latest" {
			at, _ := time.Parse(time.RFC3339Nano, i.QuotedAt)
			if at.Nanosecond()%int(time.Millisecond) != 0 {
				row.Status = "unavailable"
				row.Reason = "submillisecond_timestamp_not_preserved_by_storage"
				out.Results = append(out.Results, row)
				continue
			}
		}
		targets[j] = t
		row.Status = "pending"
		out.Results = append(out.Results, row)
		p := scopes[t.Key()]
		if p == nil {
			p = &domain.MarketComparisonScope{Target: t}
			scopes[t.Key()] = p
		}
		// Malformed dates are classified as invalid after pure validation, but are
		// never allowed to broaden a SQL date predicate.
		if d, e := time.Parse(time.DateOnly, i.Date); e == nil && d.Format(time.DateOnly) == i.Date {
			p.Dates = append(p.Dates, i.Date)
		}
		if at, e := time.Parse(time.RFC3339Nano, i.QuotedAt); e == nil {
			p.Instants = append(p.Instants, at.UTC())
		}
	}
	if len(scopes) > 100 {
		return out, agentDataValidation("items", "at most 100 distinct targets per capture")
	}
	query := []domain.MarketComparisonScope{}
	for _, p := range scopes {
		sort.Strings(p.Dates)
		p.Dates = compactComparisonStrings(p.Dates)
		query = append(query, *p)
	}
	sort.Slice(query, func(i, j int) bool { return query[i].Target.Key() < query[j].Target.Key() })
	repo, ok := s.repository.(marketComparisonRepository)
	if !ok {
		return out, &domain.Error{Code: domain.ErrUnavailable, Message: "comparison storage unavailable"}
	}
	if !domain.IsBoundedRead(ctx) {
		ctx = domain.WithBoundedRead(ctx)
	}
	providerKey := s.FXProviderKey()
	snap, err := repo.ReadMarketComparisonSnapshot(ctx, query, out.CapturedAt)
	if err != nil {
		return out, err
	}
	location := time.UTC
	if snap.Origin != nil {
		location, err = time.LoadLocation(snap.Origin.Timezone)
		if err != nil {
			return out, err
		}
	}
	instruments := map[string]*domain.Instrument{}
	for j := range snap.Instruments {
		i := &snap.Instruments[j]
		instruments[i.ID.String()] = i
	}
	byQuote := map[string][]string{}
	inactive := map[string]string{}
	provenance := map[string]domain.MarketComparisonAudit{}
	for _, a := range snap.Audits {
		if a.QuoteID != "" {
			byQuote[a.QuoteID] = append(byQuote[a.QuoteID], a.RecordID)
			provenance[a.QuoteID] = a
		}
		if a.TargetQuoteID != "" {
			byQuote[a.TargetQuoteID] = append(byQuote[a.TargetQuoteID], a.RecordID)
			state := "corrected"
			if a.Operation == "retract" {
				state = "withdrawn"
			}
			inactive[a.TargetQuoteID] = state
		}
	}
	out.Audits = append(out.Audits, snap.Audits...)
	for _, q := range snap.InstrumentQuotes {
		basis := comparisonBasis(q.ObservationKind, q.PriceBasis)
		kind := q.ObservationKind
		if basis == "unit_nav" {
			kind = "nav"
		}
		if kind == "realtime" || kind == "manual" {
			kind = "latest"
		}
		out.Evidence = append(out.Evidence, MarketComparisonEvidence{ID: q.ID.String(), Target: domain.MarketReadTarget{InstrumentID: q.InstrumentID.String()}, Currency: q.Currency.String(), Value: q.UnitPrice.Canonical(), Kind: kind, Basis: basis, PriceBasis: q.PriceBasis, Date: q.EffectiveDate, QuotedAt: q.QuotedAt, CreatedAt: q.CreatedAt, TimestampBasis: q.TimestampBasis, SourceKind: q.SourceKind, SourceKey: q.SourceKey, Revision: q.Revision, SupersedesQuoteID: q.SupersedesQuoteID, ConversionJSON: q.ConversionJSON})
	}
	for _, q := range snap.FXQuotes {
		kind := q.ObservationKind
		if kind == "manual" || kind == "realtime" {
			kind = "latest"
		}
		out.Evidence = append(out.Evidence, MarketComparisonEvidence{ID: q.ID.String(), Target: domain.MarketReadTarget{BaseCurrency: q.BaseCurrency.String(), QuoteCurrency: q.QuoteCurrency.String()}, Value: q.Rate.Canonical(), Kind: kind, Basis: comparisonBasis(q.ObservationKind, ""), Date: q.EffectiveDate, QuotedAt: q.QuotedAt, CreatedAt: q.CreatedAt, TimestampBasis: q.TimestampBasis, SourceKind: q.SourceKind, SourceKey: q.SourceKey, Revision: q.Revision, SupersedesQuoteID: q.SupersedesQuoteID})
	}
	chart := map[string][]domain.QuoteSeriesPoint{}
	for j := range out.Evidence {
		e := &out.Evidence[j]
		e.State = "active"
		if v := inactive[e.ID]; v != "" {
			e.State = v
		}
		e.AuditIDs = append([]string{}, byQuote[e.ID]...)
		if e.State == "active" {
			chartKey := e.Target.Key()
			if e.Target.InstrumentID == "" {
				a, b, _ := domain.NormalizeFXPair(domain.CurrencyCode(e.Target.BaseCurrency), domain.CurrencyCode(e.Target.QuoteCurrency))
				chartKey = "fx/" + a.String() + "/" + b.String()
			}
			chart[chartKey] = append(chart[chartKey], domain.QuoteSeriesPoint{ID: e.ID, Value: e.Value, ObservationKind: e.Kind, EffectiveDate: e.Date, QuotedAt: e.QuotedAt, CreatedAt: e.CreatedAt, SourceKind: e.SourceKind})
		}
	}
	selected := map[string]bool{}
	for _, p := range chart {
		for _, v := range chartQuotePoints(p, domain.TrendAllTime, location) {
			selected[v.ID] = true
		}
	}
	for j := range out.Evidence {
		out.Evidence[j].ChartSelected = selected[out.Evidence[j].ID]
	}
	sort.Slice(out.Evidence, func(i, j int) bool { return out.Evidence[i].ID < out.Evidence[j].ID })
	// Bound serialized expansion incrementally, before constructing cross products
	// of candidates and evidence. The final capture also enforces its 4 MiB cap.
	used := 0
	reserve := func(v any) error {
		b, e := jsoncodec.Marshal(v)
		if e != nil {
			return e
		}
		used += len(b)
		if used > 4<<20 {
			return &domain.Error{Code: "too_large", Message: "comparison evidence exceeds 4 MiB"}
		}
		return nil
	}
	if err = reserve(out.Evidence); err != nil {
		return out, err
	}
	if err = reserve(out.Audits); err != nil {
		return out, err
	}
	groups := map[string][]int{}
	matches := map[string]int{}
	for j, i := range in.Items {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		row := &out.Results[j]
		if row.Status != "pending" {
			continue
		}
		t := targets[j]
		var instrument *domain.Instrument
		if t.InstrumentID != "" {
			instrument = instruments[t.InstrumentID]
			if instrument == nil {
				row.Status = "unknown_target"
				row.Reason = "instrument_not_found_in_household"
				value, _ := domain.ParseUnitPrice(i.Value)
				n := MarketComparisonNormalized{Target: t, Kind: i.Kind, Basis: comparisonBasis(i.Kind, ""), Currency: i.Currency, Date: i.Date, Value: value.Canonical()}
				if i.Kind == "close" {
					n.Basis = "raw_close"
				}
				if at, e := time.Parse(time.RFC3339Nano, i.QuotedAt); e == nil {
					n.QuotedAt = at.UTC().Format(time.RFC3339Nano)
				}
				row.Normalized = &n
				key := comparisonKey(n)
				groups[key] = append(groups[key], j)
				continue
			}
		}
		item := comparisonItemAgent(i)
		item.InstrumentID = t.InstrumentID
		item.BaseCurrency = t.BaseCurrency
		item.QuoteCurrency = t.QuoteCurrency
		price, rate, e := parseAgentQuoteValue(item, instrument)
		if e != nil {
			row.Status = "invalid"
			row.Reason = e.Error()
			continue
		}
		when, _, e := parseAgentMarketDataTime(item, instrument, location, out.CapturedAt)
		if e != nil {
			row.Status = "invalid"
			row.Reason = e.Error()
			continue
		}
		if _, _, e := parseAgentQuoteProvenance(i.SourceTitle, i.SourceURL, false); e != nil {
			row.Status = "invalid"
			row.Reason = e.Error()
			continue
		}
		n := MarketComparisonNormalized{Target: t, Kind: i.Kind, Date: i.Date, QuotedAt: when.Format(time.RFC3339Nano), Currency: i.Currency, Value: rate.Canonical()}
		n.Basis = comparisonBasis(i.Kind, "")
		if i.Kind == "close" {
			n.Basis = "raw_close"
		}
		if instrument != nil {
			n.Value = price.Canonical()
		}
		row.Normalized = &n
		key := comparisonKey(n)
		groups[key] = append(groups[key], j)
		matchKeyBytes, _ := jsoncodec.Marshal([]string{key, n.Value, i.Value, i.SourceTitle, i.SourceURL})
		mk := string(matchKeyBytes)
		if k, exists := matches[mk]; exists {
			m := out.Matches[k]
			row.MatchID = m.ID
			row.Status = m.Status
			continue
		}
		m := MarketComparisonMatch{ID: fmt.Sprintf("match/%d", len(out.Matches)), Status: "missing", SourceEvidence: "not_requested", DisplayScale: 4, Relations: []MarketComparisonRelation{}, SelectionImpact: "unknown", SelectionReason: "selection_simulation_unavailable"}
		if t.InstrumentID == "" {
			m.DisplayScale = 6
		}
		if in.DisplayScale != nil {
			m.DisplayScale = *in.DisplayScale
		}
		if i.SourceTitle != "" || i.SourceURL != "" {
			m.SourceEvidence = "no_matching_active_agent_provenance"
		}
		relatedUnknown, reverse := false, false
		for _, ev := range out.Evidence {
			direct := ev.Target == t
			rev := t.InstrumentID == "" && ev.Target.InstrumentID == "" && ev.Target.BaseCurrency == t.QuoteCurrency && ev.Target.QuoteCurrency == t.BaseCurrency
			if !direct && !rev {
				continue
			}
			date := ev.Date
			if date == "" {
				date = ev.QuotedAt.In(location).Format(time.DateOnly)
			}
			if i.Kind == "latest" {
				if !ev.QuotedAt.Equal(when) {
					continue
				}
			} else if ev.Date == "" {
				// No market date means the quote could belong to any timezone's
				// adjacent civil date. Retain conservative incomplete evidence.
				requested, _ := time.Parse(time.DateOnly, i.Date)
				utcDate := ev.QuotedAt.UTC().Format(time.DateOnly)
				if utcDate < requested.AddDate(0, 0, -1).Format(time.DateOnly) || utcDate > requested.AddDate(0, 0, 1).Format(time.DateOnly) {
					continue
				}
			} else if date != i.Date {
				continue
			}
			rel := MarketComparisonRelation{EvidenceID: ev.ID, Relation: "different_semantics"}
			if rev {
				rel.Relation = "reverse_direction"
				reverse = true
				m.Relations = append(m.Relations, rel)
				continue
			}
			if ev.Basis == "unknown" || ev.ConversionJSON != "" || ev.Date == "" && i.Kind != "latest" {
				rel.Relation = "incomplete_basis_or_date"
				relatedUnknown = true
				m.Relations = append(m.Relations, rel)
				continue
			}
			if ev.Kind != n.Kind || ev.Basis != n.Basis || ev.Currency != n.Currency {
				m.Relations = append(m.Relations, rel)
				continue
			}
			a, _ := decimal.NewFromString(n.Value)
			b, _ := decimal.NewFromString(ev.Value)
			rel.Delta = a.Sub(b).String()
			rel.Relation = "different"
			rel.DifferenceKind = "value_difference"
			if a.Equal(b) {
				rel.Relation = "equal"
				rel.DifferenceKind = "exact"
				if i.Value != ev.Value {
					rel.DifferenceKind = "format_only"
				}
			} else if a.Round(int32(m.DisplayScale)).Equal(b.Round(int32(m.DisplayScale))) {
				rel.DifferenceKind = "display_precision_tail"
			}
			p := provenance[ev.ID]
			rel.SourceMatch = ev.SourceKind == domain.QuoteSourceAgent && (i.SourceTitle != "" || i.SourceURL != "") && strings.TrimSpace(i.SourceTitle) == p.SourceTitle && strings.TrimSpace(i.SourceURL) == p.SourceURL
			if ev.State != "active" {
				m.HistoricalComparable++
			} else if a.Equal(b) {
				m.ActiveEqual++
				if rel.SourceMatch {
					m.SourceEvidence = "matching_active_agent_provenance"
				}
			} else {
				m.ActiveDifferent++
			}
			m.Relations = append(m.Relations, rel)
		}
		switch {
		case relatedUnknown:
			m.Status = "unavailable"
			m.Reason = "incomplete_basis_or_date"
		case m.ActiveDifferent > 0:
			m.Status = "conflict"
		case m.ActiveEqual > 0:
			m.Status = "same"
		case m.HistoricalComparable > 0:
			m.Status = "historical_only"
		case reverse:
			m.Status = "unavailable"
			m.Reason = "reverse_direction_only"
		default:
			m.Reason = "no_comparable_local_observation"
		}
		m.TargetKey = t.Key()
		m.Target.Key = t.Key()
		m.Target.DisplayName = t.Key()
		if instrument != nil {
			m.Target.DisplayName = instrument.DisplayLabel()
		}
		comparisonSelection(&m, n, instrument, snap, when, out.CapturedAt, providerKey)
		if when.Nanosecond()%int(time.Millisecond) != 0 {
			m.SelectionImpact = "unknown"
			m.SelectionReason = "submillisecond_timestamp_not_preserved_by_storage"
		}
		if instrument != nil && (instrument.ArchivedAt != nil || snap.Managed[t.InstrumentID]) {
			m.SelectionImpact = "ineligible"
			m.SelectionReason = "archived_or_product_managed_instrument"
		}
		if err = reserve(m); err != nil {
			return out, err
		}
		matches[mk] = len(out.Matches)
		out.Matches = append(out.Matches, m)
		row.MatchID = m.ID
		row.Status = m.Status
	}
	for _, indices := range groups {
		if len(indices) < 2 {
			continue
		}
		values := map[string]bool{}
		for _, j := range indices {
			values[out.Results[j].Normalized.Value] = true
		}
		id := fmt.Sprintf("duplicate/%d", indices[0])
		for _, j := range indices {
			out.Results[j].DuplicateGroup = id
			out.Results[j].BatchConflict = len(values) > 1
		}
	}
	targetMap := map[string]MarketComparisonTarget{}
	for _, m := range out.Matches {
		targetMap[m.TargetKey] = m.Target
	}
	for _, t := range targetMap {
		out.Targets = append(out.Targets, t)
	}
	sort.Slice(out.Targets, func(i, j int) bool { return out.Targets[i].Key < out.Targets[j].Key })
	if err = reserve(out.Targets); err != nil {
		return out, err
	}
	if err = reserve(out.Results); err != nil {
		return out, err
	}
	return out, nil
}

// Reject malformed candidates before any scoped evidence lookup. Metadata and
// timezone-dependent date membership are validated again against the snapshot.
func comparisonSyntax(i MarketComparisonItem, now time.Time) error {
	item := comparisonItemAgent(i)
	var instrument *domain.Instrument
	if i.InstrumentID != "" {
		currency, err := domain.ParseSupportedCurrency(i.Currency)
		if err != nil {
			return err
		}
		instrument = &domain.Instrument{QuoteCurrency: currency}
	}
	if _, _, err := parseAgentQuoteValue(item, instrument); err != nil {
		return err
	}
	if i.Kind == "latest" {
		if i.Date != "" {
			return agentDataValidation("date", "latest requires quotedAt, not a daily date")
		}
		at, err := time.Parse(time.RFC3339Nano, i.QuotedAt)
		if err != nil || at.After(now) {
			return agentDataValidation("quotedAt", "latest requires an actual nonfuture RFC3339 quote time")
		}
	} else {
		d, err := time.Parse(time.DateOnly, i.Date)
		if err != nil || d.Format(time.DateOnly) != i.Date {
			return agentDataValidation("date", "must be a valid YYYY-MM-DD observation date")
		}
		if i.QuotedAt != "" {
			if _, err := time.Parse(time.RFC3339Nano, i.QuotedAt); err != nil {
				return agentDataValidation("quotedAt", "must be an actual RFC3339 quote time")
			}
		}
	}
	if i.Timezone != "" {
		if _, err := time.LoadLocation(i.Timezone); err != nil {
			return agentDataValidation("timezone", "must be an IANA timezone")
		}
	}
	return nil
}

func compactComparisonStrings(s []string) []string {
	out := s[:0]
	for _, v := range s {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}

func comparisonSelection(m *MarketComparisonMatch, n MarketComparisonNormalized, i *domain.Instrument, snap domain.MarketComparisonSnapshot, at, now time.Time, provider string) {
	// Simulate a single Agent append using the same current selectors. Ties that
	// require a future generated ID and converted-metal dependencies stay unknown.
	if i != nil {
		m.Target.Policy = string(i.QuoteSource)
		current := selectInstrumentQuote(*i, snap.CurrentInstrumentQuotes)
		if current != nil {
			m.Target.CurrentQuoteID = current.ID.String()
			m.Target.CurrentValue = current.UnitPrice.Canonical()
			m.Target.CurrentEvidence = &MarketComparisonCurrent{Target: domain.MarketReadTarget{InstrumentID: current.InstrumentID.String()}, Currency: current.Currency.String(), Value: m.Target.CurrentValue, SourceKind: current.SourceKind, SourceKey: current.SourceKey, Date: current.EffectiveDate, QuotedAt: current.QuotedAt, PriceBasis: current.PriceBasis}
		}
		if i.MetalTemplate != "" {
			m.SelectionReason = "converted_metal_dependencies_not_simulated"
			return
		}
		p, _ := domain.ParseUnitPrice(n.Value)
		q := domain.InstrumentQuote{ID: "comparison_candidate", InstrumentID: i.ID, UnitPrice: p, Currency: i.QuoteCurrency, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: at, CreatedAt: now, ObservationKind: "close", EffectiveDate: n.Date, PriceBasis: "agent_raw_close_v1", ValueEffectiveAt: at, SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "date_label", BindingRevision: i.ProviderBindingRevision}
		if n.Kind == "latest" {
			q.ObservationKind = "realtime"
			q.PriceBasis = "agent_latest_v1"
		}
		if n.Kind == "nav" {
			q.PriceBasis = "agent_unit_nav_v1"
		}
		for _, v := range snap.CurrentInstrumentQuotes {
			if v.QuotedAt.Equal(at) && v.CreatedAt.Equal(now) && v.SourceKind == domain.QuoteSourceAgent {
				m.SelectionReason = "future_quote_id_tie"
				return
			}
		}
		chosen := selectInstrumentQuote(*i, append(append([]domain.InstrumentQuote{}, snap.CurrentInstrumentQuotes...), q))
		m.SelectionImpact = "current_unchanged"
		if chosen != nil && chosen.ID == q.ID {
			m.SelectionImpact = "would_select_candidate"
		}
		m.SelectionReason = "single_append_current_selector_only_history_not_simulated"
		return
	}
	a, b, _ := domain.NormalizeFXPair(domain.CurrencyCode(n.Target.BaseCurrency), domain.CurrencyCode(n.Target.QuoteCurrency))
	for _, p := range snap.FXPreferences {
		if p.CurrencyA != a || p.CurrencyB != b {
			continue
		}
		m.Target.Policy = string(p.SourceKind)
		current := selectFXQuote(p, snap.CurrentFXQuotes, a, b, provider, nil)
		if current != nil {
			m.Target.CurrentQuoteID = current.ID.String()
			m.Target.CurrentValue = current.Rate.Canonical()
			m.Target.CurrentEvidence = &MarketComparisonCurrent{Target: domain.MarketReadTarget{BaseCurrency: current.BaseCurrency.String(), QuoteCurrency: current.QuoteCurrency.String()}, Value: m.Target.CurrentValue, SourceKind: current.SourceKind, SourceKey: current.SourceKey, Date: current.EffectiveDate, QuotedAt: current.QuotedAt}
		}
		rate, _ := domain.ParseFxRate(n.Value)
		q := domain.FXQuote{ID: "comparison_candidate", HouseholdID: p.HouseholdID, BaseCurrency: domain.CurrencyCode(n.Target.BaseCurrency), QuoteCurrency: domain.CurrencyCode(n.Target.QuoteCurrency), Rate: rate, SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", QuotedAt: at, CreatedAt: now, ObservationKind: n.Kind, EffectiveDate: n.Date, ValueEffectiveAt: at, SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "date_label"}
		for _, v := range snap.CurrentFXQuotes {
			if v.QuotedAt.Equal(at) && v.CreatedAt.Equal(now) && v.SourceKind == domain.QuoteSourceAgent {
				m.SelectionReason = "future_quote_id_tie"
				return
			}
		}
		chosen := selectFXQuote(p, append(append([]domain.FXQuote{}, snap.CurrentFXQuotes...), q), a, b, provider, nil)
		m.SelectionImpact = "current_unchanged"
		if chosen != nil && chosen.ID == q.ID {
			m.SelectionImpact = "would_select_candidate"
		}
		m.SelectionReason = "single_append_current_selector_only_history_not_simulated"
		return
	}
	m.SelectionReason = "no_captured_fx_preference"
}
