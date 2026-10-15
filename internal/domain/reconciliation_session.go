package domain

import (
	"context"
	"encoding/json"
)

// Reconciliation evidence is decimal text and frozen business identity. Source
// dates deliberately remain civil dates when no source time was supplied.
type ReconciliationSource struct {
	Reference string `json:"reference"`
	AsOfDate  string `json:"asOfDate"`
	AsOfTime  string `json:"asOfTime,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
}
type ReconciliationSession struct {
	ID          string                  `json:"id"`
	HouseholdID string                  `json:"householdId"`
	Status      string                  `json:"status"`
	Revision    int64                   `json:"revision"`
	StartedAt   string                  `json:"startedAt"`
	CompletedAt string                  `json:"completedAt,omitempty"`
	Note        string                  `json:"note"`
	Partial     bool                    `json:"partial"`
	Accounts    []ReconciliationAccount `json:"accounts,omitempty"`
	Counts      ReconciliationCounts    `json:"counts"`
}
type ReconciliationCounts struct {
	Pending    int `json:"pending"`
	Confirmed  int `json:"confirmed"`
	Skipped    int `json:"skipped"`
	Unresolved int `json:"unresolved"`
}
type ReconciliationAccount struct {
	AccountID         string               `json:"accountId"`
	Name              string               `json:"name"`
	Currency          string               `json:"currency"`
	Mode              string               `json:"mode"`
	FinancialRevision int64                `json:"financialRevision"`
	PriorRevision     int64                `json:"priorRevision"`
	LastSuccessfulAt  string               `json:"lastSuccessfulAt,omitempty"`
	Outcome           string               `json:"outcome"`
	RecheckRequired   bool                 `json:"recheckRequired"`
	Counts            ReconciliationCounts `json:"counts"`
	Items             []ReconciliationItem `json:"items,omitempty"`
}
type ReconciliationItem struct {
	ID                string                  `json:"id"`
	AccountID         string                  `json:"accountId"`
	AccountName       string                  `json:"accountName"`
	Kind              string                  `json:"kind"`
	Label             string                  `json:"label"`
	Currency          string                  `json:"currency,omitempty"`
	HoldingID         string                  `json:"holdingId,omitempty"`
	InstrumentID      string                  `json:"instrumentId,omitempty"`
	ProductID         string                  `json:"productId,omitempty"`
	Value             string                  `json:"value,omitempty"`
	Cost              string                  `json:"cost,omitempty"`
	Detail            json.RawMessage         `json:"detail,omitempty"`
	FinancialRevision int64                   `json:"financialRevision"`
	Outcome           string                  `json:"outcome"`
	Note              string                  `json:"note"`
	Source            ReconciliationSource    `json:"source"`
	CheckedAt         string                  `json:"checkedAt,omitempty"`
	ReferenceValue    string                  `json:"referenceValue,omitempty"`
	RecheckRequired   bool                    `json:"recheckRequired"`
	Attempts          []ReconciliationAttempt `json:"attempts,omitempty"`
}
type ReconciliationAttempt struct {
	Label             string               `json:"label"`
	Kind              string               `json:"kind"`
	Currency          string               `json:"currency,omitempty"`
	Detail            json.RawMessage      `json:"detail,omitempty"`
	Outcome           string               `json:"outcome"`
	Note              string               `json:"note"`
	Source            ReconciliationSource `json:"source"`
	CheckedAt         string               `json:"checkedAt"`
	FinancialRevision int64                `json:"financialRevision"`
	Value             string               `json:"value"`
	Cost              string               `json:"cost,omitempty"`
	ReferenceValue    string               `json:"referenceValue,omitempty"`
}
type ReconciliationLink struct {
	SessionID         string `json:"sessionId"`
	AccountID         string `json:"accountId"`
	ItemID            string `json:"itemId"`
	ExpectedRevision  int64  `json:"expectedRevision"`
	FinancialRevision int64  `json:"financialRevision"`
}
type ReconciliationLinkedChange struct {
	SessionID   string          `json:"sessionId"`
	AccountID   string          `json:"accountId"`
	ItemID      string          `json:"itemId"`
	OperationID string          `json:"operationId"`
	Family      string          `json:"family"`
	Evidence    json.RawMessage `json:"evidence"`
}

// Link context is set only from the immutable reviewed command envelope. The
// owning repository validates it and saves evidence in the financial transaction.
type reconciliationLinkKey struct{}

func WithReconciliationLink(ctx context.Context, link *ReconciliationLink) context.Context {
	if link == nil {
		return ctx
	}
	return context.WithValue(ctx, reconciliationLinkKey{}, *link)
}
func ReconciliationLinkFrom(ctx context.Context) *ReconciliationLink {
	link, ok := ctx.Value(reconciliationLinkKey{}).(ReconciliationLink)
	if !ok {
		return nil
	}
	return &link
}

type ReconciliationFinancialChange struct {
	AccountID   string          `json:"accountId"`
	Revision    int64           `json:"revision"`
	Family      string          `json:"family"`
	FactID      string          `json:"factId"`
	RecordedAt  string          `json:"recordedAt"`
	EffectiveAt string          `json:"effectiveAt"`
	Evidence    json.RawMessage `json:"evidence"`
}

// The application supplies the authoritative refreshed working point at commit.
// It is not a client command field and cannot fabricate an attestation.
type reconciliationItemKey struct{}

func WithReconciliationWorkingItem(ctx context.Context, item ReconciliationItem) context.Context {
	return context.WithValue(ctx, reconciliationItemKey{}, item)
}
func ReconciliationWorkingItemFrom(ctx context.Context) *ReconciliationItem {
	v, ok := ctx.Value(reconciliationItemKey{}).(ReconciliationItem)
	if !ok {
		return nil
	}
	return &v
}
