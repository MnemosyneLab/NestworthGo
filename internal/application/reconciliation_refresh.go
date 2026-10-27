package application

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Account revisions still fence every stale preview. Their append-only changes
// locate the actual endpoints to recheck, including net-zero and backdated events.
// No schema change or rewrite of frozen attempts/completed sessions is required.
func (s *Service) reconciliationTouchedItems(ctx context.Context, household domain.HouseholdID, before, fresh domain.ReconciliationAccount, changes []domain.ReconciliationFinancialChange) (map[string]bool, error) {
	touched := map[string]bool{}
	holdingItems := map[string]map[string]bool{}
	for _, list := range [][]domain.ReconciliationItem{before.Items, fresh.Items} {
		for _, item := range list {
			if item.HoldingID != "" {
				if holdingItems[item.HoldingID] == nil {
					holdingItems[item.HoldingID] = map[string]bool{}
				}
				holdingItems[item.HoldingID][item.ID] = true
			}
		}
	}
	markHolding := func(id string) {
		if len(holdingItems[id]) == 0 {
			touched["scope"] = true
		}
		for item := range holdingItems[id] {
			touched[item] = true
		}
	}
	markAll := func() {
		for _, item := range before.Items {
			touched[item.ID] = true
		}
		for _, item := range fresh.Items {
			touched[item.ID] = true
		}
	}
	if before.FinancialRevision != fresh.FinancialRevision && len(changes) == 0 {
		markAll()
	}
	activities := map[domain.ActivityID]domain.Activity{}
	for _, change := range changes {
		var evidence struct {
			Currency   string `json:"currency"`
			HoldingID  string `json:"holdingId"`
			ActivityID string `json:"activityId"`
		}
		if err := jsoncodec.Unmarshal(change.Evidence, &evidence); err != nil {
			return nil, err
		}
		switch change.Family {
		case "accounts":
			// These triggers cover archive, tracking mode and currency boundaries only.
			markAll()
		case "account_values":
			touched["balance"] = true
		case "account_cash_values":
			if evidence.Currency == "" {
				markAll()
			} else {
				touched["cash:"+evidence.Currency] = true
			}
		case "holdings":
			if change.FactID == "" {
				markAll()
			} else {
				markHolding(change.FactID)
			}
		case "instrument_scope":
			touched["scope"] = true
			for _, item := range before.Items {
				if item.InstrumentID == change.FactID {
					touched[item.ID] = true
				}
			}
		case "product_contracts":
			if change.FactID == "" {
				markAll()
			} else {
				touched["product:"+change.FactID] = true
			}
		case "product_policy":
			if evidence.HoldingID == "" {
				markAll()
			} else {
				markHolding(evidence.HoldingID)
			}
		case "product_operation":
			id, err := domain.ParseProductOperationID(change.FactID)
			if err != nil {
				return nil, err
			}
			operation, err := s.repository.ProductOperationEvidence(ctx, household, id)
			if err != nil {
				return nil, err
			}
			for _, product := range operation.Products {
				touched["product:"+product.ProductID.String()] = true
			}
		case "activity_effects":
			id, err := domain.ParseActivityID(evidence.ActivityID)
			if err != nil {
				return nil, err
			}
			activity, ok := activities[id]
			if !ok {
				activity, err = s.repository.Activity(ctx, household, id)
				if err != nil {
					return nil, err
				}
				activities[id] = activity
			}
			found := false
			for _, effect := range activity.Effects {
				if effect.ID.String() != change.FactID {
					continue
				}
				found = true
				switch effect.Target {
				case domain.EffectTargetAccountValue:
					touched["balance"] = true
				case domain.EffectTargetAccountCash:
					if effect.Money == nil {
						markAll()
					} else {
						touched["cash:"+effect.Money.Currency().String()] = true
					}
				case domain.EffectTargetHoldingQuantity, domain.EffectTargetHoldingCost:
					if effect.HoldingID == nil {
						markAll()
					} else {
						markHolding(effect.HoldingID.String())
					}
				default:
					markAll()
				}
			}
			if !found {
				markAll()
			}
		default:
			// Legacy or future evidence without a proven endpoint remains conservative.
			markAll()
		}
	}
	return touched, nil
}

func reconciliationSamePoint(before, after domain.ReconciliationItem) bool {
	if before.Kind != after.Kind || before.Currency != after.Currency || before.HoldingID != after.HoldingID || before.InstrumentID != after.InstrumentID || before.ProductID != after.ProductID || before.Value != after.Value || before.Cost != after.Cost || before.ArchivedNonzero != after.ArchivedNonzero {
		return false
	}
	if before.Kind != "product" {
		return true
	}
	// Product terms/policy facts require review; descriptive metadata and revision
	// counters do not change an attested principal or quantity on a later refresh.
	normalize := func(raw json.RawMessage) (map[string]json.RawMessage, bool) {
		var value map[string]json.RawMessage
		if err := jsoncodec.Unmarshal(raw, &value); err != nil {
			return nil, false
		}
		for _, key := range []string{"name", "shortName", "note", "revision", "createdAt", "updatedAt"} {
			delete(value, key)
		}
		return value, true
	}
	a, aOK := normalize(before.Detail)
	b, bOK := normalize(after.Detail)
	return aOK && bOK && reflect.DeepEqual(a, b)
}

func reconciliationSameInventory(before, after []domain.ReconciliationItem) bool {
	identities := func(items []domain.ReconciliationItem) map[string]string {
		result := map[string]string{}
		for _, item := range items {
			if item.Kind != "scope" && item.Kind != "removed" {
				result[item.ID] = item.Kind + "|" + item.Currency + "|" + item.HoldingID + "|" + item.InstrumentID + "|" + item.ProductID
			}
		}
		return result
	}
	return reflect.DeepEqual(identities(before), identities(after))
}

func refreshReconciliationItems(account *domain.ReconciliationAccount, fresh domain.ReconciliationAccount, touched map[string]bool) {
	if !reconciliationSameInventory(account.Items, fresh.Items) || account.Mode != fresh.Mode || account.Currency != fresh.Currency {
		touched["scope"] = true
	}
	old := map[string]domain.ReconciliationItem{}
	for _, item := range account.Items {
		old[item.ID] = item
	}
	account.Items = nil
	for _, item := range fresh.Items {
		if prior, ok := old[item.ID]; ok {
			item.Attempts = prior.Attempts
			if !touched[item.ID] && reconciliationSamePoint(prior, item) {
				item = prior
			}
			if touched[item.ID] || !reconciliationSamePoint(prior, item) {
				item.RecheckRequired = true
			}
			delete(old, item.ID)
		} else {
			item.RecheckRequired = true
		}
		// This is a refreshed working point at the account CAS revision. Attempts
		// retain their original checked revision, source, value and timestamp.
		item.FinancialRevision = fresh.FinancialRevision
		account.Items = append(account.Items, item)
	}
	for _, item := range old {
		if item.Kind != "removed" {
			item.Kind = "removed"
			item.Outcome = "pending"
			item.CheckedAt = ""
			item.RecheckRequired = true
		}
		item.FinancialRevision = fresh.FinancialRevision
		account.Items = append(account.Items, item)
	}
	sort.Slice(account.Items, func(i, j int) bool { return account.Items[i].ID < account.Items[j].ID })
	account.FinancialRevision = fresh.FinancialRevision
}

// Legacy rows may have a current account watermark but omit archived nonzero
// positions. Add only that known coverage gap; do not rewrite retained zero
// evidence solely because today's capture omits archived zero positions.
func refreshLegacyReconciliationCoverage(account *domain.ReconciliationAccount, fresh domain.ReconciliationAccount) {
	known := map[string]int{}
	for i, item := range account.Items {
		known[item.ID] = i
	}
	uncovered := map[string]bool{}
	for _, holding := range account.UncoveredArchivedHoldings {
		uncovered[holding.HoldingID] = true
	}
	added := false
	for _, item := range fresh.Items {
		if !item.ArchivedNonzero && !uncovered[item.HoldingID] {
			continue
		}
		index, exists := known[item.ID]
		if exists && account.Items[index].Kind != "removed" {
			account.Items[index].ArchivedNonzero = item.ArchivedNonzero
			continue
		}
		added = true
		item.RecheckRequired = true
		if exists {
			item.Attempts = account.Items[index].Attempts
			account.Items[index] = item
		} else {
			account.Items = append(account.Items, item)
		}
	}
	if added {
		for i, item := range account.Items {
			if item.Kind == "scope" {
				item.Outcome = "pending"
				item.CheckedAt = ""
				item.Source = domain.ReconciliationSource{}
				item.Note = ""
				item.ReferenceValue = ""
				item.ReferenceComparable = false
				item.RecheckRequired = true
				account.Items[i] = item
			}
		}
	}
	sort.Slice(account.Items, func(i, j int) bool { return account.Items[i].ID < account.Items[j].ID })
}
