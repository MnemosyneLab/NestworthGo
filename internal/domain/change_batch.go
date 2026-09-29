package domain

import "time"

// ChangeBatchMutationRecord is the durable receipt for one atomic ledger batch.
// ActivityIDs retain input order, which can differ from historical timeline order.
type ChangeBatchMutationRecord struct {
	HouseholdID   HouseholdID
	ID            MutationID
	PayloadSHA256 string
	ActivityIDs   []ActivityID
	CreatedAt     time.Time
}
