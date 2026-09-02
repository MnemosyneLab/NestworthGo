package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/csvcodec"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/version"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
)

type Service struct {
	app      *application.Service
	store    *settings.Store
	dialogs  native.Dialogs
	refresh  native.RefreshGate
	database *sqlite.DB

	mu       sync.Mutex
	sessions map[string]*csvSession
}

type csvSession struct {
	accountsTable   *csvcodec.Table
	holdingsTable   *csvcodec.Table
	accountsPlan    *application.CSVImportPlan
	holdingsPlan    *application.CSVImportPlan
	options         application.CSVParseOptions
	accountsMapping map[string]string
	holdingsMapping map[string]string
	previewOK       bool
	confirmed       bool
}

func NewService(app *application.Service, store *settings.Store, database *sqlite.DB, dialogs native.Dialogs, refresh native.RefreshGate) *Service {
	if dialogs == nil {
		dialogs = native.NoopDialogs{}
	}
	if refresh == nil {
		refresh = native.NoopRefresh{}
	}
	return &Service{app: app, store: store, database: database, dialogs: dialogs, refresh: refresh, sessions: map[string]*csvSession{}}
}

type BackupStatusDTO struct {
	Available          bool   `json:"available"`
	BackupFileName     string `json:"backupFileName,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	SchemaVersion      int    `json:"schemaVersion,omitempty"`
	AppVersion         string `json:"appVersion,omitempty"`
	AppBuild           string `json:"appBuild,omitempty"`
	VerificationResult string `json:"verificationResult,omitempty"`
}

func (s *Service) LastBackupStatus() (BackupStatusDTO, error) {
	if s.database == nil {
		return BackupStatusDTO{}, nil
	}
	status, err := backup.ReadStatus(s.database.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return BackupStatusDTO{}, nil
		}
		return BackupStatusDTO{}, apierror.Wrap(err)
	}
	return BackupStatusDTO{
		Available: true, BackupFileName: status.BackupFileName, CreatedAt: status.CreatedAt,
		SchemaVersion: status.SchemaVersion, AppVersion: status.AppVersion, AppBuild: status.AppBuild,
		VerificationResult: status.VerificationResult,
	}, nil
}

type BackupResultDTO struct {
	Cancelled          bool   `json:"cancelled"`
	FileName           string `json:"fileName,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	SchemaVersion      int    `json:"schemaVersion,omitempty"`
	AppVersion         string `json:"appVersion,omitempty"`
	AppBuild           string `json:"appBuild,omitempty"`
	Accounts           int    `json:"accounts"`
	Holdings           int    `json:"holdings"`
	Activities         int    `json:"activities"`
	VerificationResult string `json:"verificationResult,omitempty"`
}

func (s *Service) CreateBackup() (BackupResultDTO, error) {
	if s.app == nil || s.database == nil {
		return BackupResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	filename := backup.DefaultBackupFileName(time.Now())
	path, err := s.dialogs.SaveFile("Back up data", filename, "Nestworth Backup", "*.nestworth-backup")
	if err != nil {
		return BackupResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the backup location could not be selected"})
	}
	if path == "" {
		return BackupResultDTO{Cancelled: true}, nil
	}
	if !strings.HasSuffix(strings.ToLower(path), backup.FileExt) {
		path += backup.FileExt
	}
	if _, statErr := os.Stat(path); statErr == nil {
		replace, confirmErr := s.dialogs.ConfirmReplace(filepath.Base(path))
		if confirmErr != nil {
			return BackupResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the existing backup could not be confirmed"})
		}
		if !replace {
			return BackupResultDTO{Cancelled: true}, nil
		}
	}
	var result BackupResultDTO
	if err := s.app.WithExclusive(context.Background(), application.ExclusiveBackup, func(ctx context.Context) error {
		s.refresh.CancelAllAndWait()
		s.app.LockWrites()
		snapPath := filepath.Join(filepath.Dir(path), ".nestworth-snapshot-tmp.sqlite")
		_ = os.Remove(snapPath)
		snapErr := s.app.SnapshotTo(ctx, snapPath)
		s.app.UnlockWrites()
		if snapErr != nil {
			_ = os.Remove(snapPath)
			return snapErr
		}
		defer os.Remove(snapPath)
		verified, err := sqlite.OpenReadOnlyForVerify(snapPath)
		if err != nil {
			return err
		}
		counts, err := verified.EntityCounts(ctx)
		_ = verified.Close()
		if err != nil {
			return err
		}
		databaseBytes, err := os.ReadFile(snapPath)
		if err != nil {
			return &domain.Error{Code: domain.ErrUnavailable, Message: "the database snapshot could not be read"}
		}
		settingsJSON := []byte("{}\n")
		if s.store != nil {
			if current, loadErr := s.store.Load(); loadErr == nil {
				if encoded, encodeErr := json.Marshal(current); encodeErr == nil {
					settingsJSON = append(encoded, '\n')
				}
			}
		}
		pkg := backup.Package{
			Manifest: backup.NewManifest(time.Now().UTC(), databaseBytes, settingsJSON, counts),
			Database: databaseBytes,
			Settings: settingsJSON,
		}
		if err := backup.WritePackage(path, pkg); err != nil {
			return err
		}
		status := backup.Status{
			BackupFileName: filepath.Base(path), CreatedAt: pkg.Manifest.CreatedAt, SchemaVersion: pkg.Manifest.SchemaVersion,
			AppVersion: pkg.Manifest.AppVersion, AppBuild: pkg.Manifest.AppBuild, VerificationResult: "ok",
		}
		if err := backup.WriteStatus(s.database.Path, status); err != nil {
			return err
		}
		result = BackupResultDTO{
			FileName: status.BackupFileName, CreatedAt: status.CreatedAt, SchemaVersion: status.SchemaVersion,
			AppVersion: version.Version, AppBuild: version.Build, Accounts: counts.Accounts, Holdings: counts.Holdings,
			Activities: counts.Activities, VerificationResult: "ok",
		}
		return nil
	}); err != nil {
		return BackupResultDTO{}, apierror.Wrap(err)
	}
	return result, nil
}

type CSVExportResultDTO struct {
	Cancelled bool   `json:"cancelled"`
	FileName  string `json:"fileName,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Rows      int    `json:"rows"`
}

func (s *Service) ExportCSV(profile string, includeArchived bool) (CSVExportResultDTO, error) {
	if s.app == nil {
		return CSVExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	filename := "Nestworth-" + strings.ToLower(profile) + ".csv"
	path, err := s.dialogs.SaveFile("Export CSV", filename, "CSV", "*.csv")
	if err != nil {
		return CSVExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the export location could not be selected"})
	}
	if path == "" {
		return CSVExportResultDTO{Cancelled: true}, nil
	}
	if !strings.HasSuffix(strings.ToLower(path), ".csv") {
		path += ".csv"
	}
	if _, statErr := os.Stat(path); statErr == nil {
		replace, confirmErr := s.dialogs.ConfirmReplace(filepath.Base(path))
		if confirmErr != nil || !replace {
			return CSVExportResultDTO{Cancelled: true}, nil
		}
	}
	ctx := context.Background()
	var data []byte
	switch strings.ToLower(profile) {
	case application.CSVProfileAccounts:
		data, err = s.app.ExportAccountsCSV(ctx, includeArchived)
	case application.CSVProfileHoldings:
		data, err = s.app.ExportHoldingsCSV(ctx, includeArchived)
	default:
		return CSVExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "CSV profile is not supported"})
	}
	if err != nil {
		return CSVExportResultDTO{}, apierror.Wrap(err)
	}
	if err := backup.WriteAtomicFile(path, data, 0o600); err != nil {
		return CSVExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the CSV file could not be written"})
	}
	written, readErr := os.ReadFile(path)
	if readErr != nil {
		return CSVExportResultDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the saved CSV file could not be read back"})
	}
	table, parseErr := csvcodec.Parse(written, csvcodec.DelimiterComma)
	if parseErr != nil {
		return CSVExportResultDTO{}, apierror.Wrap(parseErr)
	}
	return CSVExportResultDTO{FileName: filepath.Base(path), Profile: strings.ToLower(profile), Rows: len(table.Rows)}, nil
}

type CSVFileDTO struct {
	Cancelled   bool     `json:"cancelled"`
	Token       string   `json:"token,omitempty"`
	Profile     string   `json:"profile,omitempty"`
	FileName    string   `json:"fileName,omitempty"`
	Delimiter   string   `json:"delimiter,omitempty"`
	HasBOM      bool     `json:"hasBom"`
	Headers     []string `json:"headers,omitempty"`
	RowCount    int      `json:"rowCount"`
	ColumnCount int      `json:"columnCount"`
}

func (s *Service) SelectCSV(profile, sessionToken string) (CSVFileDTO, error) {
	path, err := s.dialogs.OpenFile("Import CSV", "CSV", "*.csv")
	if err != nil {
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the CSV file could not be selected"})
	}
	if path == "" {
		return CSVFileDTO{Cancelled: true}, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "the CSV file could not be read"})
	}
	if info.Size() > csvcodec.MaxFileBytes {
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVLimitExceeded, Message: "CSV file exceeds the size limit"})
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVInvalidEncoding, Message: "the CSV file could not be read"})
	}
	delimiter := csvcodec.DetectDelimiter(string(data))
	table, err := csvcodec.Parse(data, delimiter)
	if err != nil {
		return CSVFileDTO{}, apierror.Wrap(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	token := sessionToken
	if token == "" {
		token = newCSVToken()
		s.sessions[token] = &csvSession{options: application.CSVParseOptions{Delimiter: rune(delimiter), DateFormat: application.CSVDateISO, DecimalSep: ".", GroupingSep: "none"}, accountsMapping: map[string]string{}, holdingsMapping: map[string]string{}}
	}
	session := s.sessions[token]
	if session == nil {
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "import session expired"})
	}
	session.previewOK = false
	session.confirmed = false
	copyTable := table
	switch strings.ToLower(profile) {
	case application.CSVProfileAccounts:
		session.accountsTable = &copyTable
	case application.CSVProfileHoldings:
		session.holdingsTable = &copyTable
	default:
		return CSVFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "CSV profile is not supported"})
	}
	return CSVFileDTO{
		Token: token, Profile: strings.ToLower(profile), FileName: filepath.Base(path),
		Delimiter: delimiterName(delimiter), HasBOM: table.HasBOM, Headers: table.Headers,
		RowCount: len(table.Rows), ColumnCount: len(table.Headers),
	}, nil
}

type CSVOptionsRequest struct {
	Token           string                            `json:"token"`
	Profile         string                            `json:"profile"`
	Delimiter       string                            `json:"delimiter"`
	DateFormat      string                            `json:"dateFormat"`
	DecimalSep      string                            `json:"decimalSep"`
	GroupingSep     string                            `json:"groupingSep"`
	Mapping         map[string]string                 `json:"mapping"`
	AccountsMapping map[string]string                 `json:"accountsMapping"`
	HoldingsMapping map[string]string                 `json:"holdingsMapping"`
	Unresolved      []application.CSVUnresolvedAction `json:"unresolved"`
}

type CSVPreviewDTO struct {
	Profile     string                          `json:"profile"`
	Headers     []string                        `json:"headers"`
	PreviewRows [][]string                      `json:"previewRows"`
	Stats       application.CSVPreviewStats     `json:"stats"`
	Errors      []application.CSVRowError       `json:"errors"`
	Warnings    []application.CSVWarning        `json:"warnings"`
	Unresolved  []application.CSVUnresolvedName `json:"unresolved"`
	CanCommit   bool                            `json:"canCommit"`
}

type CSVConfirmDTO struct {
	Confirmed bool `json:"confirmed"`
}

func (s *Service) PreviewCSV(request CSVOptionsRequest) (CSVPreviewDTO, error) {
	if s.app == nil {
		return CSVPreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	s.mu.Lock()
	session := s.sessions[request.Token]
	if session == nil {
		s.mu.Unlock()
		return CSVPreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "import session expired"})
	}
	session.options = application.CSVParseOptions{
		Delimiter: parseDelimiter(request.Delimiter), DateFormat: request.DateFormat,
		DecimalSep: request.DecimalSep, GroupingSep: request.GroupingSep, Unresolved: request.Unresolved,
	}
	profile := strings.ToLower(strings.TrimSpace(request.Profile))
	if profile == "" {
		profile = application.CSVProfileAccounts
	}
	if request.AccountsMapping != nil {
		session.accountsMapping = request.AccountsMapping
	}
	if request.HoldingsMapping != nil {
		session.holdingsMapping = request.HoldingsMapping
	}
	if request.Mapping != nil {
		switch profile {
		case application.CSVProfileHoldings:
			if request.HoldingsMapping == nil {
				session.holdingsMapping = request.Mapping
			}
		default:
			if request.AccountsMapping == nil {
				session.accountsMapping = request.Mapping
			}
		}
	}
	session.previewOK = false
	session.confirmed = false
	session.accountsPlan = nil
	session.holdingsPlan = nil
	input := snapshotCSVSession(session)
	s.mu.Unlock()
	ctx := context.Background()
	plans, err := s.buildCSVPlans(ctx, input)
	if err != nil {
		return CSVPreviewDTO{}, apierror.Wrap(err)
	}
	s.mu.Lock()
	session = s.sessions[request.Token]
	if session == nil {
		s.mu.Unlock()
		return CSVPreviewDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "import session expired"})
	}
	session.previewOK = true
	session.confirmed = false
	if plans.accounts != nil {
		accountsCopy := *plans.accounts
		session.accountsPlan = &accountsCopy
	}
	if plans.holdings != nil {
		holdingsCopy := *plans.holdings
		session.holdingsPlan = &holdingsCopy
	}
	display := plans.combined
	if profile == application.CSVProfileAccounts && plans.accounts != nil {
		display = *plans.accounts
	} else if profile == application.CSVProfileHoldings && plans.holdings != nil {
		display = *plans.holdings
	}
	s.mu.Unlock()
	return CSVPreviewDTO{
		Profile: profile, Headers: display.Headers, PreviewRows: display.PreviewRows, Stats: plans.combined.Stats,
		Errors: plans.combined.Errors, Warnings: plans.combined.Warnings, Unresolved: plans.combined.Unresolved, CanCommit: len(plans.combined.Errors) == 0,
	}, nil
}

func (s *Service) ConfirmCSV(token string) (CSVConfirmDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[token]
	if session == nil {
		return CSVConfirmDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "import session expired"})
	}
	if !session.previewOK {
		return CSVConfirmDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVRowInvalid, Message: "CSV import must be previewed before confirmation"})
	}
	plan := session.holdingsPlan
	if plan == nil {
		plan = session.accountsPlan
	}
	if plan == nil || len(plan.Errors) > 0 {
		return CSVConfirmDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVRowInvalid, Message: "CSV import has blocking errors"})
	}
	session.confirmed = true
	return CSVConfirmDTO{Confirmed: true}, nil
}

func (s *Service) CommitCSV(token string) (application.CSVPreviewStats, error) {
	if s.app == nil {
		return application.CSVPreviewStats{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "database is not available"})
	}
	s.mu.Lock()
	session := s.sessions[token]
	if session == nil {
		s.mu.Unlock()
		return application.CSVPreviewStats{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "import session expired"})
	}
	if !session.confirmed {
		s.mu.Unlock()
		return application.CSVPreviewStats{}, apierror.Wrap(&domain.Error{Code: domain.ErrCSVRowInvalid, Message: "CSV import must be previewed and confirmed before commit"})
	}
	input := snapshotCSVSession(session)
	s.mu.Unlock()
	s.refresh.CancelAllAndWait()
	stats, err := s.app.CommitCSVImportBuilt(context.Background(), func() (application.CSVImportPlan, error) {
		return s.buildCombinedPlan(context.Background(), input)
	})
	if err != nil {
		return application.CSVPreviewStats{}, apierror.Wrap(err)
	}
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
	return stats, nil
}

type csvPlanInput struct {
	accountsTable   *csvcodec.Table
	holdingsTable   *csvcodec.Table
	accountsMapping map[string]string
	holdingsMapping map[string]string
	options         application.CSVParseOptions
}

func snapshotCSVSession(session *csvSession) csvPlanInput {
	input := csvPlanInput{
		accountsMapping: copyStringMap(session.accountsMapping),
		holdingsMapping: copyStringMap(session.holdingsMapping),
		options:         session.options,
	}
	input.options.Unresolved = append([]application.CSVUnresolvedAction{}, session.options.Unresolved...)
	if session.accountsTable != nil {
		table := *session.accountsTable
		input.accountsTable = &table
	}
	if session.holdingsTable != nil {
		table := *session.holdingsTable
		input.holdingsTable = &table
	}
	return input
}

type csvPlanSet struct {
	combined application.CSVImportPlan
	accounts *application.CSVImportPlan
	holdings *application.CSVImportPlan
}

func (s *Service) buildCSVPlans(ctx context.Context, input csvPlanInput) (csvPlanSet, error) {
	var result csvPlanSet
	if input.accountsTable != nil {
		plan, err := s.app.BuildCSVImportPlan(ctx, application.CSVProfileAccounts, *input.accountsTable, input.accountsMapping, input.options, nil)
		if err != nil {
			return csvPlanSet{}, err
		}
		result.accounts = &plan
		result.combined = plan
	}
	if input.holdingsTable != nil {
		plan, err := s.app.BuildCSVImportPlan(ctx, application.CSVProfileHoldings, *input.holdingsTable, input.holdingsMapping, input.options, &result.combined)
		if err != nil {
			return csvPlanSet{}, err
		}
		result.holdings = &plan
		result.combined = plan
	}
	if result.accounts == nil && result.holdings == nil {
		return csvPlanSet{}, &domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "no CSV file is selected"}
	}
	return result, nil
}

func (s *Service) buildCombinedPlan(ctx context.Context, input csvPlanInput) (application.CSVImportPlan, error) {
	plans, err := s.buildCSVPlans(ctx, input)
	if err != nil {
		return application.CSVImportPlan{}, err
	}
	return plans.combined, nil
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

type CSVErrorFileDTO struct {
	Cancelled bool `json:"cancelled"`
}

func (s *Service) DownloadCSVErrors(token string) (CSVErrorFileDTO, error) {
	s.mu.Lock()
	session := s.sessions[token]
	s.mu.Unlock()
	if session == nil {
		return CSVErrorFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrNotFound, Message: "import session expired"})
	}
	plan := session.holdingsPlan
	if plan == nil {
		plan = session.accountsPlan
	}
	if plan == nil {
		return CSVErrorFileDTO{Cancelled: true}, nil
	}
	path, err := s.dialogs.SaveFile("Download CSV errors", "Nestworth CSV Errors.csv", "CSV", "*.csv")
	if err != nil || path == "" {
		return CSVErrorFileDTO{Cancelled: true}, nil
	}
	rows := make([][]string, 0, len(plan.Errors))
	for _, item := range plan.Errors {
		rows = append(rows, []string{strconv.Itoa(item.Row), item.Field, item.Source, item.Code, item.Message, item.Suggestion})
	}
	data, err := csvcodec.Encode([]string{"row", "field", "source", "code", "message", "suggestion"}, rows)
	if err != nil {
		return CSVErrorFileDTO{}, apierror.Wrap(err)
	}
	if err := backup.WriteAtomicFile(path, data, 0o600); err != nil {
		return CSVErrorFileDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrUnavailable, Message: "the error CSV could not be written"})
	}
	return CSVErrorFileDTO{}, nil
}

func (s *Service) CancelCSV(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func delimiterName(value csvcodec.Delimiter) string {
	switch value {
	case csvcodec.DelimiterSemicolon:
		return "semicolon"
	case csvcodec.DelimiterTab:
		return "tab"
	default:
		return "comma"
	}
}

func parseDelimiter(value string) rune {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "semicolon", ";":
		return ';'
	case "tab", "\t":
		return '\t'
	default:
		return ','
	}
}

func newCSVToken() string {
	var entropy [8]byte
	_, _ = rand.Read(entropy[:])
	return hex.EncodeToString(entropy[:])
}
