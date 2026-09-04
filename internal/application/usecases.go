package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// DirectoryService is the household and directory catalog use case.
type DirectoryService interface {
	Bootstrap(ctx context.Context) (Bootstrap, error)
	CompleteOnboarding(ctx context.Context, input OnboardingInput) error
	Household(ctx context.Context) (domain.Household, error)
	ListMembers(ctx context.Context, includeArchived bool) ([]domain.Member, error)
	CreateMember(ctx context.Context, name string, iconKeys ...string) (domain.Member, error)
	UpdateMember(ctx context.Context, id domain.MemberID, name string) (domain.Member, error)
	ArchiveMember(ctx context.Context, id domain.MemberID, archived bool) error
	ListInstitutions(ctx context.Context, includeArchived bool) ([]domain.Institution, error)
	CreateInstitution(ctx context.Context, name string, institutionType domain.InstitutionType, iconKeys ...string) (domain.Institution, error)
	UpdateInstitution(ctx context.Context, id domain.InstitutionID, name string) (domain.Institution, error)
	ArchiveInstitution(ctx context.Context, id domain.InstitutionID, archived bool) error
	ListGroups(ctx context.Context, includeArchived bool) ([]domain.Group, error)
	CreateGroup(ctx context.Context, name string, iconKeys ...string) (domain.Group, error)
	UpdateGroup(ctx context.Context, id domain.GroupID, name string) (domain.Group, error)
	ArchiveGroup(ctx context.Context, id domain.GroupID, archived bool) error
	CreateAccount(ctx context.Context, input AccountInput) (domain.AccountRecord, error)
	UpdateAccount(ctx context.Context, id domain.AccountID, input AccountInput) (domain.AccountRecord, error)
	ListAccounts(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountRecord, error)
	ArchiveAccount(ctx context.Context, id domain.AccountID, archived bool) error
}

// LedgerService is the holdings, instrument, and quote use case.
type LedgerService interface {
	CreateInstrument(ctx context.Context, input InstrumentInput) (domain.Instrument, error)
	UpdateInstrument(ctx context.Context, id domain.InstrumentID, input InstrumentInput) (domain.Instrument, error)
	ListInstruments(ctx context.Context, includeArchived bool) ([]domain.Instrument, error)
	ArchiveInstrument(ctx context.Context, id domain.InstrumentID, archived bool) error
	CreateHolding(ctx context.Context, input HoldingInput) (domain.Holding, error)
	ListHoldings(ctx context.Context, accountID domain.AccountID, includeArchived bool) ([]domain.Holding, error)
	AppendAccountValue(ctx context.Context, accountID domain.AccountID, amount, effectiveAt string) (domain.AccountValue, error)
	AppendManualInstrumentQuote(ctx context.Context, instrumentID domain.InstrumentID, unitPrice, quotedAt string, delayed bool) (domain.InstrumentQuote, error)
	AppendManualFXQuote(ctx context.Context, baseCurrency, quoteCurrency, rate, quotedAt string) (domain.FXQuote, error)
}

// ValuationUseCase is the read-side valuation and gain surface. The existing
// ValuationService type remains the calculation engine owned by Service.
type ValuationUseCase interface {
	Overview(ctx context.Context, filter domain.AccountFilter) (domain.OverviewResult, error)
	Portfolio(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioValuation, error)
	AccountValuation(ctx context.Context, id domain.AccountID) (domain.AccountValuation, error)
	AccountValuations(ctx context.Context, filter domain.AccountFilter) ([]domain.AccountValuation, error)
	HoldingGain(ctx context.Context, id domain.HoldingID) (domain.HoldingGainView, error)
	AccountGain(ctx context.Context, id domain.AccountID) (domain.AccountGainView, error)
	AccountGains(ctx context.Context, ids []domain.AccountID) ([]domain.AccountGainView, error)
	RealizedGain(ctx context.Context, scope domain.GainScope, trendRange domain.TrendRange) (domain.RealizedGainView, error)
	DividendIncome(ctx context.Context, scope domain.GainScope, trendRange domain.TrendRange) (domain.DividendIncomeView, error)
	PortfolioTrend(ctx context.Context, trendRange domain.TrendRange) (domain.PortfolioTrend, error)
}

// AnalysisUseCase is the Phase 2a Asset Changes projection surface. All
// methods consume one memoized PeriodAnalysisResult per query; they do not
// expose a second calculation path.
type AnalysisUseCase interface {
	AssetChange(context.Context, domain.AnalysisQuery) (AssetChangeResult, error)
	AssetDriverDetail(context.Context, domain.AnalysisQuery, string) (AssetDriverDetailResult, error)
	AssetTrend(context.Context, domain.AnalysisQuery, AssetTrendGranularity, AssetTrendMetric) (AssetTrendResult, error)
	Categories(context.Context, domain.AnalysisQuery, AnalysisCategoryType) (CategoriesResult, error)
	CategoryDetail(context.Context, domain.AnalysisQuery, AnalysisCategoryType, string) (CategoryDetailResult, error)
}

// HistoryService is the activity ledger and change-command use case.
type HistoryService interface {
	HistoryOrigin(ctx context.Context) (*domain.HistoryOrigin, error)
	StartHistory(ctx context.Context, timezone string) (domain.HistoryOrigin, error)
	ListActivities(ctx context.Context, limit int) ([]domain.Activity, error)
	Activity(ctx context.Context, activityID domain.ActivityID) (domain.Activity, error)
	ListActivityPage(ctx context.Context, query domain.ActivityQuery) (domain.ActivityPage, error)
	PreviewChange(ctx context.Context, command any) (domain.ChangePreview, error)
	RecordChange(ctx context.Context, command any) (domain.ChangePreview, error)
	UndoChange(ctx context.Context, activityID domain.ActivityID) (domain.ChangePreview, error)
	FixChange(ctx context.Context, activityID domain.ActivityID, replacementCommand any) (domain.ChangePreview, error)
}

// ImportExportService is the backup and CSV orchestration use case.
type ImportExportService interface {
	CreateBackup(ctx context.Context, destPath string, settingsJSON []byte) (BackupCreateResult, error)
	DefaultBackupFileName() string
	LastBackupStatus() (BackupStatus, bool, error)
	ExportCSVBytes(ctx context.Context, profile string, includeArchived bool) ([]byte, int, error)
	WriteExportFile(path string, data []byte) error
	SelectCSV(profile, sessionToken, fileName string, data []byte) (CSVFileSelection, error)
	PreviewCSV(ctx context.Context, request CSVPreviewRequest) (CSVPreviewView, error)
	ConfirmCSV(token string) error
	CommitCSV(ctx context.Context, token string) (CSVPreviewStats, error)
	CSVErrorReport(token string) ([]byte, error)
	CancelCSV(token string)
	ShutdownCSV()
}

// RecoveryService is the Restore inspect/confirm use case. It is implemented
// by Recovery so blocked startup can restore without a live Service.
type RecoveryService interface {
	InspectBackup(ctx context.Context, sourcePath string) (RestorePreview, error)
	ConfirmRestore(ctx context.Context, input RestoreConfirmInput) (RestoreResult, error)
	Shutdown()
}

var (
	_ DirectoryService    = (*Service)(nil)
	_ LedgerService       = (*Service)(nil)
	_ ValuationUseCase    = (*Service)(nil)
	_ AnalysisUseCase     = (*Service)(nil)
	_ HistoryService      = (*Service)(nil)
	_ ImportExportService = (*Service)(nil)
	_ RecoveryService     = (*Recovery)(nil)
)
