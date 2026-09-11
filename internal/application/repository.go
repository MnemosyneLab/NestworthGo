package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Repository is the persistence boundary. It is composed of bounded-context
// ports so new work has a place to go without widening one 90-method surface.
// sqlite.Repository still implements the composition as one type.
type Repository interface {
	DirectoryRepository
	AccountRepository
	PortfolioRepository
	HistoryRepository
	SnapshotRepository
	DatabaseAdminRepository
	CSVImportRepository
}

// DirectoryRepository owns Household, Members, Institutions, Groups, and
// the icon references those entities carry.
type DirectoryRepository interface {
	Household(context.Context) (*domain.Household, error)
	CreateOnboarding(context.Context, domain.Household, []domain.Member) error
	CreateOnboardingWithHistory(context.Context, domain.Household, []domain.Member, domain.HistoryOriginData) error
	ListMembers(context.Context, bool) ([]domain.Member, error)
	Member(context.Context, domain.HouseholdID, domain.MemberID) (domain.Member, error)
	CreateMember(context.Context, domain.Member) error
	UpdateMember(context.Context, domain.Member) error
	SetMemberArchive(context.Context, domain.HouseholdID, domain.MemberID, bool, time.Time) error
	CreateInstitution(context.Context, domain.Institution) error
	ListInstitutions(context.Context, bool) ([]domain.Institution, error)
	Institution(context.Context, domain.HouseholdID, domain.InstitutionID) (domain.Institution, error)
	UpdateInstitution(context.Context, domain.Institution) error
	SetInstitutionArchive(context.Context, domain.HouseholdID, domain.InstitutionID, bool, time.Time) error
	CreateGroup(context.Context, domain.Group) error
	ListGroups(context.Context, bool) ([]domain.Group, error)
	Group(context.Context, domain.HouseholdID, domain.GroupID) (domain.Group, error)
	UpdateGroup(context.Context, domain.Group) error
	SetGroupArchive(context.Context, domain.HouseholdID, domain.GroupID, bool, time.Time) error
	SetMemberIcon(context.Context, domain.HouseholdID, domain.MemberID, string, time.Time) error
	SetInstitutionIcon(context.Context, domain.HouseholdID, domain.InstitutionID, string, time.Time) error
	SetGroupIcon(context.Context, domain.HouseholdID, domain.GroupID, string, time.Time) error
	SetAccountIcon(context.Context, domain.HouseholdID, domain.AccountID, string, time.Time) error
}

// AccountRepository owns Account identity, ownership, archive, and current
// value observations.
type AccountRepository interface {
	CreateAccount(context.Context, domain.Account, domain.Ownership, *domain.AccountValue) error
	CreateAccountWithHistory(context.Context, domain.Account, domain.Ownership, *domain.AccountValue, domain.AccountStateObservation, *domain.ActivityCommit, time.Time) error
	UpdateAccount(context.Context, domain.Account, domain.Ownership) error
	UpdateAccountWithObservation(context.Context, domain.Account, domain.Ownership, domain.AccountStateObservation) error
	AppendAccountValue(context.Context, domain.AccountValue) error
	SetAccountArchive(context.Context, domain.HouseholdID, domain.AccountID, bool, time.Time) error
	SetAccountArchiveWithObservation(context.Context, domain.HouseholdID, domain.AccountID, bool, time.Time, domain.AccountStateObservation) error
	ListAccountRecords(context.Context, domain.HouseholdID, domain.AccountFilter) ([]domain.AccountRecord, error)
	AccountRecord(context.Context, domain.HouseholdID, domain.AccountID) (domain.AccountRecord, error)
}

// PortfolioRepository owns instruments, holdings, quotes, cash, and FX.
type PortfolioRepository interface {
	ReadSnapshot(context.Context, domain.AccountFilter) (domain.ReadSnapshot, error)
	ReadPortfolioSnapshot(context.Context, domain.AccountFilter) (domain.PortfolioSnapshot, error)
	CreateInstrument(context.Context, domain.Instrument) error
	CreateInstrumentWithObservation(context.Context, domain.Instrument, domain.InstrumentPreferenceObservation) error
	UpdateInstrument(context.Context, domain.Instrument) error
	UpdateInstrumentWithObservation(context.Context, domain.Instrument, domain.InstrumentPreferenceObservation) error
	Instrument(context.Context, domain.HouseholdID, domain.InstrumentID) (domain.Instrument, error)
	ListInstruments(context.Context, domain.HouseholdID, bool) ([]domain.Instrument, error)
	SetInstrumentArchive(context.Context, domain.HouseholdID, domain.InstrumentID, bool, time.Time) error
	SetInstrumentIcon(context.Context, domain.HouseholdID, domain.InstrumentID, string, time.Time) error
	SetInstrumentQuoteSource(context.Context, domain.HouseholdID, domain.InstrumentID, domain.QuoteSourceKind, time.Time) error
	SetInstrumentQuoteSourceWithObservation(context.Context, domain.HouseholdID, domain.InstrumentID, domain.QuoteSourceKind, domain.InstrumentPreferenceObservation) error
	CreateHolding(context.Context, domain.Holding) error
	CreateHoldingWithActivity(context.Context, domain.Holding, domain.ActivityCommit, time.Time) error
	UpdateHolding(context.Context, domain.Holding) error
	Holding(context.Context, domain.HoldingID) (domain.Holding, error)
	ListHoldings(context.Context, domain.AccountID, bool) ([]domain.Holding, error)
	ListHoldingsByAccounts(context.Context, []domain.AccountID) ([]domain.Holding, error)
	SetHoldingArchive(context.Context, domain.HouseholdID, domain.HoldingID, bool, time.Time) error
	AppendAccountCashValue(context.Context, domain.AccountCashValue) error
	ListAccountCashValues(context.Context, domain.AccountID) ([]domain.AccountCashValue, error)
	AppendInstrumentQuote(context.Context, domain.InstrumentQuote) error
	AppendProviderInstrumentQuoteIfChanged(context.Context, domain.InstrumentQuote) (bool, error)
	AppendInstrumentQuoteAndSelectManual(context.Context, domain.InstrumentQuote) error
	ListInstrumentQuotes(context.Context, domain.InstrumentID) ([]domain.InstrumentQuote, error)
	AppendFXQuote(context.Context, domain.FXQuote) error
	AppendProviderFXQuoteIfChanged(context.Context, domain.FXQuote) (bool, error)
	AppendFXQuoteAndSelectManual(context.Context, domain.FXQuote) error
	ListFXQuotes(context.Context, domain.HouseholdID) ([]domain.FXQuote, error)
	SetFXPreference(context.Context, domain.FXPreference) error
	SetFXPreferenceWithObservation(context.Context, domain.FXPreference, domain.FXPreferenceObservation) error
	FXPreference(context.Context, domain.HouseholdID, domain.CurrencyCode, domain.CurrencyCode) (domain.FXPreference, error)
	ListFXPreferences(context.Context, domain.HouseholdID) ([]domain.FXPreference, error)
}

// HistoryRepository owns the Starting point, Activities, cost basis, and
// the immutable observation facts used to replay a date.
type HistoryRepository interface {
	HistoryOrigin(context.Context, domain.HouseholdID) (*domain.HistoryOrigin, error)
	ListHistoryOriginComponents(context.Context, domain.HistoryOriginID) ([]domain.HistoryOriginComponent, error)
	HistoryOriginData(context.Context, domain.HistoryOriginID) (domain.HistoryOriginData, error)
	ListCostBasisEvents(context.Context, domain.HoldingID, domain.CostBasisReadFilter) ([]domain.CostBasisEvent, error)
	StartingPointCost(context.Context, domain.HoldingID, domain.CostBasisReadFilter) (*domain.UnitPrice, error)
	ListAccountStateObservations(context.Context, domain.HouseholdID) ([]domain.AccountStateObservation, error)
	ListInstrumentStateObservations(context.Context, domain.HouseholdID) ([]domain.InstrumentStateObservation, error)
	ListHoldingStateObservations(context.Context, domain.HouseholdID) ([]domain.HoldingStateObservation, error)
	ListInstrumentPreferenceObservations(context.Context, domain.HouseholdID) ([]domain.InstrumentPreferenceObservation, error)
	ListFXPreferenceObservations(context.Context, domain.HouseholdID) ([]domain.FXPreferenceObservation, error)
	LoadHistoricalSnapshotBatch(context.Context, domain.HouseholdID, time.Time) (domain.HistoricalSnapshotBatch, error)
	StartHistory(context.Context, domain.HistoryOriginData) (domain.HistoryOrigin, error)
	CommitActivity(context.Context, domain.Activity, []domain.ActivityEffect, []domain.EndpointView, time.Time) error
	CommitActivityBatch(context.Context, []domain.ActivityCommit, time.Time) error
	LookupActivityMutation(context.Context, domain.HouseholdID, domain.MutationID) (*domain.ActivityMutationRecord, error)
	Activity(context.Context, domain.HouseholdID, domain.ActivityID) (domain.Activity, error)
	ActivityEffects(context.Context, domain.ActivityID) ([]domain.ActivityEffect, error)
	ActivityHasReversal(context.Context, domain.HouseholdID, domain.ActivityID) (bool, error)
	ListActivities(context.Context, domain.HouseholdID, int) ([]domain.Activity, error)
	ListActivityPage(context.Context, domain.HouseholdID, domain.ActivityQuery) (domain.ActivityPage, error)
	ListActivitiesUntil(context.Context, domain.HouseholdID, time.Time) ([]domain.Activity, error)
	AppendAccountStateObservation(context.Context, domain.AccountStateObservation) error
	AppendInstrumentPreferenceObservation(context.Context, domain.InstrumentPreferenceObservation) error
	AppendFXPreferenceObservation(context.Context, domain.FXPreferenceObservation) error
}

// SnapshotRepository owns daily valuation snapshots and their completion
// watermark.
type SnapshotRepository interface {
	MarkDailySnapshotCompleted(context.Context, domain.HouseholdID, string, time.Time) error
	SaveDailyValuationSnapshotAndMarkCompleted(context.Context, domain.DailyValuationSnapshot, time.Time) (bool, error)
	CompleteDailySnapshotRange(context.Context, domain.HouseholdID, string, time.Time) error
	DailySnapshotState(context.Context, domain.HouseholdID) (domain.DailySnapshotState, error)
	ListDailyValuationSnapshots(context.Context, domain.HouseholdID, time.Time, time.Time) ([]domain.DailyValuationSnapshot, error)
	ListInstrumentHistoryCoverage(context.Context, domain.HouseholdID) ([]domain.InstrumentHistoryCoverage, error)
	ListFXHistoryCoverage(context.Context, domain.HouseholdID) ([]domain.FXHistoryCoverage, error)
}

// GenerationAwareSnapshotRepository makes snapshot publication conditional
// on the generation captured by the same read used for reconstruction. The
// optional interface keeps small in-memory test repositories source-compatible
// while SQLite uses the atomic implementation.
type GenerationAwareSnapshotRepository interface {
	SaveDailyValuationSnapshotAndMarkCompletedAtGeneration(context.Context, domain.DailyValuationSnapshot, time.Time, int) (bool, error)
	CompleteDailySnapshotRangeAtGeneration(context.Context, domain.HouseholdID, string, time.Time, int) error
}

// DatabaseAdminRepository is the live-file snapshot and close path used by
// backup and restore. It never copies a live main SQLite file.
type DatabaseAdminRepository interface {
	SnapshotTo(context.Context, string) error
	CheckpointWAL(context.Context) error
	Close() error
	Path() string
	PreviewCounts(context.Context) (accounts, holdings, activities int, err error)
}

// CSVImportRepository writes a create-only import plan in one transaction.
type CSVImportRepository interface {
	CommitCSVImport(context.Context, domain.CSVImportBatch) error
}
