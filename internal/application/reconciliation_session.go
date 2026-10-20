package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type ReconciliationAction struct {
	Action              string                      `json:"action"`
	SessionID           string                      `json:"sessionId,omitempty"`
	ExpectedRevision    int64                       `json:"expectedRevision,omitempty"`
	AccountIDs          []string                    `json:"accountIds,omitempty"`
	AccountID           string                      `json:"accountId,omitempty"`
	ItemID              string                      `json:"itemId,omitempty"`
	FinancialRevision   int64                       `json:"financialRevision,omitempty"`
	Outcome             string                      `json:"outcome,omitempty"`
	Source              domain.ReconciliationSource `json:"source,omitempty"`
	ReferenceComparable bool                        `json:"referenceComparable,omitempty"`
	ReferenceValue      string                      `json:"referenceValue,omitempty"`
	Note                string                      `json:"note,omitempty"`
	AllowPartial        bool                        `json:"allowPartial,omitempty"`
}
type ReconciliationSessionPlan struct {
	ID        string                       `json:"id"`
	Action    ReconciliationAction         `json:"action"`
	At        string                       `json:"at"`
	ExpiresAt time.Time                    `json:"expiresAt"`
	Result    domain.ReconciliationSession `json:"result"`
	Revisions map[string]int64             `json:"revisions"`
}

var reconciliationReferenceNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,17})(\.[0-9]{1,16})?$`)

func reconciliationError(field, message string) error {
	return &domain.Error{Code: domain.ErrValidation, Field: field, Message: message}
}
func reconciliationStale(field string) error {
	return &domain.Error{Code: domain.ErrConflict, Field: field, Message: "evidence changed; reread and review before confirming"}
}
func (s *Service) reconciliationRepo() (ReconciliationRepository, error) {
	r, ok := s.repository.(ReconciliationRepository)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "reconciliation storage unavailable"}
	}
	return r, nil
}

func (s *Service) ListReconciliationSessions(ctx context.Context) ([]domain.ReconciliationSession, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	r, err := s.reconciliationRepo()
	if err != nil {
		return nil, err
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.ReconciliationSessions(ctx, h.ID.String())
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Status != "completed" {
			current, err := s.getReconciliationSession(ctx, rows[i].ID)
			if err != nil {
				return nil, err
			}
			rows[i].Counts = current.Counts
		}
	}
	return rows, nil
}
func (s *Service) GetReconciliationSession(ctx context.Context, id string) (domain.ReconciliationSession, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return domain.ReconciliationSession{}, err
	}
	defer unlock()
	return s.getReconciliationSession(ctx, id)
}
func (s *Service) getReconciliationSession(ctx context.Context, id string) (domain.ReconciliationSession, error) {
	r, err := s.reconciliationRepo()
	if err != nil {
		return domain.ReconciliationSession{}, err
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.ReconciliationSession{}, err
	}
	session, err := r.ReconciliationSession(ctx, h.ID.String(), id)
	if err != nil {
		return session, err
	}
	revisions, err := r.ReconciliationRevisions(ctx)
	if err != nil {
		return session, err
	}
	coverage, err := s.reconciliationInventoryCoverage(ctx, session)
	if err != nil {
		return session, err
	}
	for i := range session.Accounts {
		a := &session.Accounts[i]
		a.UncoveredArchivedHoldings = coverage.uncovered[a.AccountID]
		a.ArchivedHoldingIDs = coverage.archived[a.AccountID]
		a.InventoryCoverageGap = len(a.UncoveredArchivedHoldings) > 0
		a.FinancialRevisionChanged = a.FinancialRevision != revisions[a.AccountID]
	}
	if session.Status == "completed" {
		for i := range session.Accounts {
			a := &session.Accounts[i]
			a.RecheckRequired = a.FinancialRevisionChanged
		}
		// Completed items, outcomes and counts are frozen. Current coverage and
		// bookkeeping context are separate account-level read projections.
		return session, nil
	}
	ids := []string{}
	for _, a := range session.Accounts {
		ids = append(ids, a.AccountID)
	}
	current, err := s.captureReconciliation(ctx, ids, revisions)
	if err != nil {
		return session, err
	}
	for i := range session.Accounts {
		a := &session.Accounts[i]
		if a.FinancialRevision == revisions[a.AccountID] {
			// Older versions omitted archived nonzero positions entirely.
			// Surface that known inventory gap even without a new financial
			// write. Preserve existing evidence; the added point stays pending.
			known := map[string]int{}
			for index, item := range a.Items {
				known[item.ID] = index
			}
			uncovered := map[string]bool{}
			for _, holding := range a.UncoveredArchivedHoldings {
				uncovered[holding.HoldingID] = true
			}
			if fresh := current[a.AccountID]; fresh != nil {
				for _, item := range fresh.Items {
					if !item.ArchivedNonzero && !uncovered[item.HoldingID] {
						continue
					}
					index, exists := known[item.ID]
					if exists && a.Items[index].Kind != "removed" {
						a.Items[index].ArchivedNonzero = item.ArchivedNonzero
						continue
					}
					item.RecheckRequired = true
					if exists {
						item.Attempts = a.Items[index].Attempts
						a.Items[index] = item
					} else {
						a.Items = append(a.Items, item)
					}
				}
			}
			sort.Slice(a.Items, func(i, j int) bool { return a.Items[i].ID < a.Items[j].ID })
			continue
		}
		fresh := current[a.AccountID]
		if fresh == nil {
			return session, reconciliationStale("accountId")
		}
		old := map[string]domain.ReconciliationItem{}
		for _, item := range a.Items {
			old[item.ID] = item
		}
		a.Items = nil
		for _, item := range fresh.Items {
			if prior, ok := old[item.ID]; ok {
				item.Attempts = prior.Attempts
				delete(old, item.ID)
			}
			item.RecheckRequired = true
			a.Items = append(a.Items, item)
		}
		for _, item := range old {
			item.RecheckRequired = true
			item.FinancialRevision = revisions[a.AccountID]
			item.Outcome = "pending"
			item.Kind = "removed"
			item.CheckedAt = ""
			a.Items = append(a.Items, item)
		}
		// Removed points originate in a map. Keep the entire refreshed evidence
		// in the same stable-ID order used by capture and persisted reads.
		sort.Slice(a.Items, func(i, j int) bool { return a.Items[i].ID < a.Items[j].ID })
		a.FinancialRevision = revisions[a.AccountID]
		a.RecheckRequired = true
		a.Outcome = "pending"
	}
	countReconciliation(&session)
	return session, nil
}

// Compare current inventory with recorded identities only. A gap does not
// establish when the position appeared and must not rewrite a completed check.
type reconciliationInventoryContext struct {
	uncovered map[string][]domain.ReconciliationCoverageHolding
	archived  map[string][]string
}

func (s *Service) reconciliationInventoryCoverage(ctx context.Context, session domain.ReconciliationSession) (reconciliationInventoryContext, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return reconciliationInventoryContext{}, err
	}
	selected := map[string]map[string]bool{}
	for _, account := range session.Accounts {
		known := map[string]bool{}
		for _, item := range account.Items {
			if item.HoldingID != "" && (item.Kind == "holding" || item.Kind == "product") {
				known[item.HoldingID] = true
			}
		}
		selected[account.AccountID] = known
	}
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	gaps := map[string][]domain.ReconciliationCoverageHolding{}
	archived := map[string][]string{}
	for _, holding := range snapshot.Holdings {
		known, selectedAccount := selected[holding.AccountID.String()]
		instrument := instruments[holding.InstrumentID]
		if !selectedAccount || (holding.ArchivedAt == nil && instrument.ArchivedAt == nil) {
			continue
		}
		archived[holding.AccountID.String()] = append(archived[holding.AccountID.String()], holding.ID.String())
		if known[holding.ID.String()] || holding.Quantity.IsZero() {
			continue
		}
		gaps[holding.AccountID.String()] = append(gaps[holding.AccountID.String()], domain.ReconciliationCoverageHolding{HoldingID: holding.ID.String(), InstrumentID: holding.InstrumentID.String(), Label: instrument.Name, Quantity: holding.Quantity.Canonical(), Currency: instrument.QuoteCurrency.String()})
	}
	for account := range gaps {
		sort.Slice(gaps[account], func(i, j int) bool { return gaps[account][i].HoldingID < gaps[account][j].HoldingID })
	}
	for account := range archived {
		sort.Strings(archived[account])
	}
	return reconciliationInventoryContext{uncovered: gaps, archived: archived}, nil
}

func countReconciliation(s *domain.ReconciliationSession) {
	s.Counts = domain.ReconciliationCounts{}
	for i := range s.Accounts {
		a := &s.Accounts[i]
		a.Counts = domain.ReconciliationCounts{}
		for _, item := range a.Items {
			switch item.Outcome {
			case "confirmed":
				a.Counts.Confirmed++
			case "skipped":
				a.Counts.Skipped++
			case "unresolved":
				a.Counts.Unresolved++
			default:
				a.Counts.Pending++
			}
		}
		if a.Counts.Pending+a.Counts.Skipped+a.Counts.Unresolved == 0 && len(a.Items) > 0 {
			a.Outcome = "confirmed"
		} else {
			a.Outcome = "partial"
		}
		if s.Status != "completed" {
			a.RecheckRequired = false
			for _, item := range a.Items {
				if item.RecheckRequired {
					a.RecheckRequired = true
				}
			}
		}
		s.Counts.Pending += a.Counts.Pending
		s.Counts.Confirmed += a.Counts.Confirmed
		s.Counts.Skipped += a.Counts.Skipped
		s.Counts.Unresolved += a.Counts.Unresolved
	}
}
func (s *Service) captureReconciliation(ctx context.Context, ids []string, revisions map[string]int64) (map[string]*domain.ReconciliationAccount, error) {
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	result := map[string]*domain.ReconciliationAccount{}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	for _, record := range snapshot.Accounts {
		a := record.Account
		id := a.ID.String()
		if !selected[id] {
			continue
		}
		account := &domain.ReconciliationAccount{AccountID: id, Name: a.Name, Currency: a.DefaultCurrency.String(), Mode: string(a.TrackingMode), FinancialRevision: revisions[id], Outcome: "pending"}
		account.Items = append(account.Items, domain.ReconciliationItem{ID: "scope", Kind: "scope", Label: "Account contents and completeness", Value: "Review all currencies, positions and products; include missing or unsettled records"})
		if a.TrackingMode != domain.TrackingHoldings {
			value := ""
			if record.LatestValue != nil {
				value = record.LatestValue.Amount.CanonicalAmount()
			}
			account.Items = append(account.Items, domain.ReconciliationItem{ID: "balance", Kind: "balance", Label: a.Name, Currency: a.DefaultCurrency.String(), Value: value})
		}
		result[id] = account
	}
	if len(result) != len(ids) {
		return nil, reconciliationError("accountIds", "selected accounts must belong to this household")
	}
	for _, v := range snapshot.CashValues {
		if a := result[v.AccountID.String()]; a != nil {
			a.Items = append(a.Items, domain.ReconciliationItem{ID: "cash:" + v.Amount.Currency().String(), Kind: "cash", Label: v.Amount.Currency().String(), Currency: v.Amount.Currency().String(), Value: v.Amount.CanonicalAmount()})
		}
	}
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for _, v := range snapshot.Instruments {
		instruments[v.ID] = v
	}
	products, err := s.repository.ListProducts(ctx, snapshot.Household.ID, nil, true)
	if err != nil {
		return nil, err
	}
	managed := map[domain.HoldingID]bool{}
	for _, p := range products {
		managed[p.HoldingID] = true
		if a := result[p.AccountID.String()]; a != nil {
			raw, err := jsoncodec.Marshal(p)
			if err != nil {
				return nil, err
			}
			a.Items = append(a.Items, domain.ReconciliationItem{ID: "product:" + p.ID.String(), Kind: "product", Label: p.Name, ProductID: p.ID.String(), HoldingID: p.HoldingID.String(), InstrumentID: p.InstrumentID.String(), Currency: p.Currency.String(), Value: p.Principal.CanonicalAmount(), Detail: raw})
		}
	}
	costs := newCostBasisReplayContext(s.repository, snapshot.Holdings)
	for _, h := range snapshot.Holdings {
		a := result[h.AccountID.String()]
		if a == nil || (h.ArchivedAt != nil && h.Quantity.IsZero()) || managed[h.ID] {
			continue
		}
		instrument := instruments[h.InstrumentID]
		if instrument.ArchivedAt != nil && h.Quantity.IsZero() {
			continue
		}
		item := domain.ReconciliationItem{ID: "holding:" + h.ID.String(), Kind: "holding", Label: instrument.Name, HoldingID: h.ID.String(), InstrumentID: h.InstrumentID.String(), Value: h.Quantity.Canonical(), Currency: instrument.QuoteCurrency.String()}
		item.ArchivedNonzero = !h.Quantity.IsZero() && (h.ArchivedAt != nil || instrument.ArchivedAt != nil)
		item.Detail, _ = jsoncodec.Marshal(map[string]string{"name": instrument.Name, "symbol": pointerText(instrument.Symbol), "isin": pointerText(instrument.ISIN), "marketCode": pointerText(instrument.MarketCode), "quantityUnit": instrument.QuantityUnit})
		lot, err := costs.replay(ctx, h.ID, nil)
		if err != nil {
			var domainErr *domain.Error
			if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrCostBasisRequired {
				return nil, err
			}
		} else if lot.Current.Quantity.Decimal().Equal(h.Quantity.Decimal()) {
			// A known zero cost is evidence; missing cost remains empty.
			item.Cost = lot.Current.AverageUnitCost.Canonical()
		}
		a.Items = append(a.Items, item)
	}
	for _, a := range result {
		sort.Slice(a.Items, func(i, j int) bool { return a.Items[i].ID < a.Items[j].ID })
		for i := range a.Items {
			v := &a.Items[i]
			v.AccountID = a.AccountID
			v.AccountName = a.Name
			v.FinancialRevision = a.FinancialRevision
			v.Outcome = "pending"
		}
	}
	return result, nil
}
func (s *Service) PreviewReconciliationSession(ctx context.Context, in ReconciliationAction) (ReconciliationSessionPlan, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return ReconciliationSessionPlan{}, err
	}
	defer unlock()
	plan := ReconciliationSessionPlan{ID: uuid.NewString(), Action: in, At: s.clock().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().Add(10 * time.Minute)}
	result, revisions, err := s.prepareReconciliationSession(ctx, plan)
	plan.Result = result
	plan.Revisions = revisions
	return plan, err
}
func (s *Service) prepareReconciliationSession(ctx context.Context, plan ReconciliationSessionPlan) (domain.ReconciliationSession, map[string]int64, error) {
	in := plan.Action
	var session domain.ReconciliationSession
	for _, field := range []struct {
		name, value string
		limit       int
	}{
		{"note", in.Note, 2000},
		{"referenceValue", in.ReferenceValue, 256},
		{"source.reference", in.Source.Reference, 512},
	} {
		if len(field.value) > field.limit {
			return session, nil, reconciliationError(field.name, fmt.Sprintf("reconciliation %s exceeds %d UTF-8 bytes", field.name, field.limit))
		}
	}
	r, err := s.reconciliationRepo()
	if err != nil {
		return session, nil, err
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return session, nil, err
	}
	revisions, err := r.ReconciliationRevisions(ctx)
	if err != nil {
		return session, nil, err
	}
	if in.Action == "start" {
		if in.ExpectedRevision != 0 || len(in.AccountIDs) > 100 {
			return session, nil, reconciliationError("accountIds", "select at most 100 accounts")
		}
		records, err := s.repository.ListAccountRecords(ctx, h.ID, domain.AccountFilter{})
		if err != nil {
			return session, nil, err
		}
		active := map[string]bool{}
		for _, v := range records {
			if v.Account.ArchivedAt == nil {
				active[v.Account.ID.String()] = true
			}
		}
		ids := append([]string{}, in.AccountIDs...)
		if len(ids) == 0 {
			for id := range active {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 || len(ids) > 100 {
			return session, nil, reconciliationError("accountIds", "select 1 to 100 active accounts")
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if !active[id] || seen[id] {
				return session, nil, reconciliationError("accountIds", "select distinct active accounts in this household")
			}
			seen[id] = true
		}
		sort.Strings(ids)
		captured, err := s.captureReconciliation(ctx, ids, revisions)
		if err != nil {
			return session, nil, err
		}
		session = domain.ReconciliationSession{ID: plan.ID, HouseholdID: h.ID.String(), Status: "in_progress", Revision: 1, StartedAt: plan.At, Note: in.Note}
		for _, id := range ids {
			a := captured[id]
			prior, err := s.reconciliationLastSuccess(ctx, h.ID.String(), id)
			if err != nil {
				return session, nil, err
			}
			if prior != nil {
				a.LastSuccessfulAt = prior.CompletedAt
				a.PriorSuccessfulAt = prior.CompletedAt
				for _, pa := range prior.Accounts {
					if pa.AccountID == id {
						a.PriorRevision = pa.FinancialRevision
					}
				}
			}
			session.Accounts = append(session.Accounts, *a)
		}
	} else {
		session, err = s.getReconciliationSession(ctx, in.SessionID)
		if err != nil {
			return session, nil, err
		}
		if session.Status == "completed" {
			return session, nil, &domain.Error{Code: domain.ErrConflict, Field: "expectedRevision", Message: "session is completed and read-only; start a new reconciliation session"}
		}
		if session.Revision != in.ExpectedRevision {
			return session, nil, reconciliationStale("expectedRevision")
		}
		switch in.Action {
		case "pause":
			session.Status = "paused"
		case "resume":
			session.Status = "in_progress"
		case "check":
			if session.Status != "in_progress" {
				return session, nil, reconciliationError("status", "resume the session before checking")
			}
			found := false
			for i := range session.Accounts {
				a := &session.Accounts[i]
				if a.AccountID != in.AccountID {
					continue
				}
				if a.FinancialRevision != in.FinancialRevision {
					return session, nil, reconciliationStale("financialRevision")
				}
				for j := range a.Items {
					item := &a.Items[j]
					if item.ID != in.ItemID {
						continue
					}
					found = true
					switch in.Outcome {
					case "confirmed":
						if (item.Kind == "cash" || item.Kind == "balance" || item.Kind == "holding") && item.Value == "" {
							return session, nil, reconciliationError("itemId", "current financial value is unavailable; leave this check pending or unresolved")
						}
						if item.Kind == "holding" && item.Cost == "" {
							return session, nil, reconciliationError("itemId", "current cost is unavailable; leave this check pending or unresolved")
						}
						if strings.TrimSpace(in.Source.Reference) == "" || in.Source.AsOfDate == "" {
							return session, nil, reconciliationError("source", "confirmation requires a user-provided source and as-of date")
						}
						if _, err := time.Parse("2006-01-02", in.Source.AsOfDate); err != nil {
							return session, nil, reconciliationError("asOfDate", "provide a valid civil date")
						}
						if in.ReferenceComparable && strings.TrimSpace(in.ReferenceValue) != "" && (item.Kind == "balance" || item.Kind == "cash" || item.Kind == "holding") {
							loc := time.UTC
							if origin, e := s.HistoryOrigin(ctx); e != nil {
								return session, nil, e
							} else if origin != nil {
								if l, e := time.LoadLocation(origin.Timezone); e == nil {
									loc = l
								}
							}
							at, e := time.Parse(time.RFC3339Nano, plan.At)
							if e != nil {
								return session, nil, e
							}
							if in.Source.AsOfDate != at.In(loc).Format(time.DateOnly) {
								return session, nil, reconciliationError("referenceComparable", "historical source values are not comparable to the current ledger checkpoint")
							}
							if !reconciliationReferenceNumber.MatchString(strings.TrimSpace(in.ReferenceValue)) {
								return session, nil, reconciliationError("referenceValue", "provide a bounded numeric value for comparison")
							}
							ref, e := decimal.NewFromString(strings.TrimSpace(in.ReferenceValue))
							if e != nil {
								return session, nil, reconciliationError("referenceValue", "provide a numeric value for this comparison")
							}
							current, e := decimal.NewFromString(item.Value)
							if e != nil {
								return session, nil, e
							}
							if !ref.Equal(current) && strings.TrimSpace(in.Note) == "" {
								return session, nil, reconciliationError("referenceValue", "source value differs; correct the ledger, leave unresolved, or explain the difference")
							}
						}
						if in.Source.AsOfTime != "" {
							if _, err := time.Parse("15:04", in.Source.AsOfTime); err != nil {
								return session, nil, reconciliationError("asOfTime", "provide HH:mm")
							}
							origin, err := s.HistoryOrigin(ctx)
							if err != nil {
								return session, nil, err
							}
							if origin == nil {
								return session, nil, reconciliationError("asOfTime", "history timezone is required")
							}
							if _, err := domain.ResolveLocalDateTime(in.Source.AsOfDate, in.Source.AsOfTime, origin.Timezone); err != nil {
								return session, nil, reconciliationError("asOfTime", "source time is not valid in the ledger timezone")
							}
							in.Source.Timezone = origin.Timezone
						} else {
							in.Source.Timezone = ""
						}
					case "skipped", "unresolved":
						if strings.TrimSpace(in.Note) == "" {
							return session, nil, reconciliationError("note", "explain skipped or unresolved checks")
						}
					case "pending":
					default:
						return session, nil, reconciliationError("outcome", "choose confirmed, skipped, unresolved or pending")
					}
					if len(item.Attempts) >= 100 {
						return session, nil, reconciliationError("itemId", "check attempt limit reached; finish partially and start a new session")
					}
					item.Outcome = in.Outcome
					item.Note = in.Note
					item.Source = in.Source
					item.ReferenceValue = in.ReferenceValue
					item.ReferenceComparable = in.ReferenceComparable
					item.CheckedAt = plan.At
					item.RecheckRequired = false
					item.Attempts = append(item.Attempts, domain.ReconciliationAttempt{Label: item.Label, Kind: item.Kind, Currency: item.Currency, Detail: item.Detail, Outcome: item.Outcome, Note: item.Note, Source: item.Source, CheckedAt: plan.At, FinancialRevision: item.FinancialRevision, Value: item.Value, Cost: item.Cost, ReferenceValue: item.ReferenceValue, ReferenceComparable: item.ReferenceComparable})
				}
			}
			if !found {
				return session, nil, reconciliationError("itemId", "item is outside the selected scope")
			}
		case "finish":
			countReconciliation(&session)
			partial := session.Counts.Pending+session.Counts.Skipped+session.Counts.Unresolved > 0
			if partial && (!in.AllowPartial || strings.TrimSpace(in.Note) == "") {
				return session, nil, reconciliationError("allowPartial", "partial finish requires explicit intent and a closing explanation")
			}
			session.Status = "completed"
			session.CompletedAt = plan.At
			session.Partial = partial
			session.ClosingNote = in.Note
			for i := range session.Accounts {
				if session.Accounts[i].Outcome == "confirmed" {
					session.Accounts[i].LastSuccessfulAt = plan.At
				}
			}
		default:
			return session, nil, reconciliationError("action", "unsupported reconciliation action")
		}
		session.Revision++
	}
	for i := range session.Accounts {
		session.Accounts[i].ArchivedHoldingIDs = nil
		session.Accounts[i].InventoryCoverageGap = false
		session.Accounts[i].FinancialRevisionChanged = false
		session.Accounts[i].UncoveredArchivedHoldings = nil
	}
	countReconciliation(&session)
	selected := map[string]int64{}
	for _, a := range session.Accounts {
		selected[a.AccountID] = revisions[a.AccountID]
	}
	return session, selected, nil
}
func (s *Service) CommitReconciliationSession(ctx context.Context, plan ReconciliationSessionPlan) (domain.ReconciliationSession, error) {
	ctx, unlock, err := s.beginWrite(ctx)
	if err != nil {
		return domain.ReconciliationSession{}, err
	}
	defer unlock()
	r, err := s.reconciliationRepo()
	if err != nil {
		return domain.ReconciliationSession{}, err
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.ReconciliationSession{}, err
	}
	raw, err := jsoncodec.Marshal(plan)
	if err != nil {
		return domain.ReconciliationSession{}, err
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if receipt, err := r.ReconciliationReceipt(ctx, h.ID.String(), plan.ID, hash); err != nil {
		return domain.ReconciliationSession{}, err
	} else if receipt != nil {
		return *receipt, nil
	}
	if time.Now().After(plan.ExpiresAt) {
		// Receipt lookup above proved this plan has no committed outcome. Keep
		// this distinct from an unavailable desktop plan with unknown outcome.
		return domain.ReconciliationSession{}, &domain.Error{Code: domain.ErrStalePreview, Field: "expiresAt", Message: "session preview expired; request a fresh review"}
	}
	result, revisions, err := s.prepareReconciliationSession(ctx, plan)
	if err != nil {
		return result, err
	}
	a, _ := jsoncodec.Marshal(result)
	b, _ := jsoncodec.Marshal(plan.Result)
	if string(a) != string(b) {
		return result, reconciliationStale("preview")
	}
	err = r.SaveReconciliation(ctx, result, plan.Action.ExpectedRevision, revisions, plan.ID, hash)
	return result, err
}
func (s *Service) reconciliationLastSuccess(ctx context.Context, household, account string) (*domain.ReconciliationSession, error) {
	r, err := s.reconciliationRepo()
	if err != nil {
		return nil, err
	}
	sessions, err := r.ReconciliationSessions(ctx, household)
	if err != nil {
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, sessions[i].CompletedAt)
		b, _ := time.Parse(time.RFC3339Nano, sessions[j].CompletedAt)
		if a.Equal(b) {
			return sessions[i].ID > sessions[j].ID
		}
		return a.After(b)
	})
	for _, header := range sessions {
		if header.Status != "completed" {
			continue
		}
		v, err := r.ReconciliationSession(ctx, household, header.ID)
		if err != nil {
			return nil, err
		}
		for _, a := range v.Accounts {
			if a.AccountID == account && a.Outcome == "confirmed" {
				return &v, nil
			}
		}
	}
	return nil, nil
}
func (s *Service) ReconciliationLastSuccess(ctx context.Context, account string) (*domain.ReconciliationSession, error) {
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.reconciliationLastSuccess(ctx, h.ID.String(), account)
	if err != nil || v == nil {
		return v, err
	}
	current, err := s.getReconciliationSession(ctx, v.ID)
	return &current, err
}
func (s *Service) ReconciliationLinkedChanges(ctx context.Context, id string) ([]domain.ReconciliationLinkedChange, error) {
	r, err := s.reconciliationRepo()
	if err != nil {
		return nil, err
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	return r.ReconciliationLinks(ctx, h.ID.String(), id)
}

// ValidateReconciliationLink fences previews as well as the repository's atomic
// commit check. An attestation is never implied by an attached correction.
func (s *Service) ValidateReconciliationLink(ctx context.Context, link *domain.ReconciliationLink, previews []domain.ChangePreview) error {
	if link == nil {
		return nil
	}
	session, err := s.getReconciliationSession(ctx, link.SessionID)
	if err != nil {
		return err
	}
	if session.Status != "in_progress" || session.Revision != link.ExpectedRevision {
		return reconciliationStale("reconciliationLink")
	}
	var selected *domain.ReconciliationItem
	for _, a := range session.Accounts {
		if a.AccountID != link.AccountID {
			continue
		}
		if a.FinancialRevision != link.FinancialRevision {
			return reconciliationStale("financialRevision")
		}
		for _, item := range a.Items {
			if item.ID == link.ItemID {
				v := item
				selected = &v
			}
		}
	}
	if selected == nil {
		return reconciliationError("reconciliationLink", "item is outside selected scope")
	}

	if selected.Kind == "removed" {
		return reconciliationError("reconciliationLink", "removed items retain evidence but cannot receive corrections")
	}
	matched := len(previews) == 0
	for _, p := range previews {
		for _, e := range p.Effects {
			account := ""
			if e.AccountID != nil {
				account = e.AccountID.String()
			}
			if e.HoldingID != nil {
				h, err := s.repository.Holding(ctx, *e.HoldingID)
				if err == nil {
					account = h.AccountID.String()
				}
			}
			matched = matched || domain.ReconciliationEffectMatches(*selected, account, e)
		}
	}
	if !matched {
		return reconciliationError("reconciliationLink", "the atomic correction must affect the selected holding, product or cash currency")
	}

	return nil
}

func (s *Service) validateProductReconciliationLink(ctx context.Context, link *domain.ReconciliationLink, id domain.ProductContractID) error {
	if link == nil {
		return nil
	}
	if err := s.ValidateReconciliationLink(ctx, link, nil); err != nil {
		return err
	}
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	p, err := s.repository.Product(ctx, h.ID, id)
	if err != nil {
		return err
	}
	if p.AccountID.String() != link.AccountID {
		return reconciliationError("reconciliationLink", "product belongs to another account")
	}
	session, err := s.getReconciliationSession(ctx, link.SessionID)
	if err != nil {
		return err
	}
	for _, a := range session.Accounts {
		if a.AccountID == link.AccountID {
			for _, item := range a.Items {
				if item.ID == link.ItemID && !(item.Kind == "scope" || item.Kind == "product" && item.ProductID == id.String()) {
					return reconciliationError("reconciliationLink", "product does not match the reviewed item")
				}
			}
		}
	}

	return nil
}

func (s *Service) ReconciliationChanges(ctx context.Context, id, account string) ([]domain.ReconciliationFinancialChange, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	v, err := s.getReconciliationSession(ctx, id)
	if err != nil {
		return nil, err
	}
	r, err := s.reconciliationRepo()
	if err != nil {
		return nil, err
	}
	result := []domain.ReconciliationFinancialChange{}
	for _, a := range v.Accounts {
		if account != "" && a.AccountID != account {
			continue
		}
		if a.PriorRevision == 0 && v.Status != "completed" {
			continue
		}
		rows, err := r.ReconciliationFinancialChanges(ctx, a.AccountID, a.PriorRevision)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			row.AfterCompletion = v.Status == "completed" && row.Revision > a.FinancialRevision
			if a.PriorRevision != 0 || row.AfterCompletion {
				result = append(result, row)
			}
		}
	}
	return result, nil
}

func (s *Service) ValidateReconciliationCommandLink(ctx context.Context, link *domain.ReconciliationLink, command any, preview domain.ChangePreview) error {
	if link == nil {
		return nil
	}
	if in, ok := command.(domain.PositionImportInput); ok {
		if err := s.ValidateReconciliationLink(ctx, link, nil); err != nil {
			return err
		}
		session, err := s.getReconciliationSession(ctx, link.SessionID)
		if err != nil {
			return err
		}
		validScope := false
		for _, a := range session.Accounts {
			for _, item := range a.Items {
				if a.AccountID == link.AccountID && item.ID == link.ItemID && item.Kind == "scope" {
					validScope = true
				}
			}
		}
		if in.AccountID.String() != link.AccountID || !validScope {
			return reconciliationError("reconciliationLink", "position import belongs to another account")
		}
		return nil
	}
	return s.ValidateReconciliationLink(ctx, link, []domain.ChangePreview{preview})
}

// Called AFTER receipt replay, under the owning financial write permit. Reads
// stay read-only; a new working point is persisted with its next linked write.
func (s *Service) reconciliationCommitContext(ctx context.Context) (context.Context, error) {
	link := domain.ReconciliationLinkFrom(ctx)
	if link == nil {
		return ctx, nil
	}
	if err := s.ValidateReconciliationLink(ctx, link, nil); err != nil {
		return ctx, err
	}
	session, err := s.getReconciliationSession(ctx, link.SessionID)
	if err != nil {
		return ctx, err
	}
	for _, a := range session.Accounts {
		if a.AccountID != link.AccountID {
			continue
		}
		for _, item := range a.Items {
			if item.ID == link.ItemID {
				return domain.WithReconciliationWorkingItem(ctx, item), nil
			}
		}
	}
	return ctx, reconciliationError("reconciliationLink", "reviewed point is outside the current scope")
}
