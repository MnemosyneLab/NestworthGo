package domain

import "time"

// CSVImportedAccount is one create-only account row prepared for a single
// import transaction. It is not a user-facing CSV schema.
type CSVImportedAccount struct {
	Account     Account
	Ownership   Ownership
	Value       *AccountValue
	Observation *AccountStateObservation
	Activity    *ActivityCommit
}

// CSVImportedInstrument is one create-only instrument row prepared for a
// single import transaction.
type CSVImportedInstrument struct {
	Instrument  Instrument
	Observation *InstrumentPreferenceObservation
}

// CSVImportBatch is the all-or-nothing write set for Accounts and Holdings
// CSV import. SQLite commits it in one transaction.
type CSVImportBatch struct {
	Members      []Member
	Institutions []Institution
	Groups       []Group
	Accounts     []CSVImportedAccount
	Instruments  []CSVImportedInstrument
	Holdings     []Holding
	Quotes       []InstrumentQuote
	AsOf         time.Time
}
