package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	CSVImportSessionTTL   = 15 * time.Minute
	maxPendingCSVSessions = 2
	maxPendingCSVBytes    = CSVMaxFileBytes * 2
)

type csvImportSession struct {
	accountsTable   *CSVTable
	holdingsTable   *CSVTable
	accountsPlan    *CSVImportPlan
	holdingsPlan    *CSVImportPlan
	options         CSVParseOptions
	accountsMapping map[string]string
	holdingsMapping map[string]string
	previewOK       bool
	confirmed       bool
	createdAt       time.Time
	sizeBytes       int64
}

type CSVFileSelection struct {
	Token       string
	Profile     string
	FileName    string
	Delimiter   string
	HasBOM      bool
	Headers     []string
	RowCount    int
	ColumnCount int
}

type CSVPreviewRequest struct {
	Token           string
	Profile         string
	Delimiter       string
	DateFormat      string
	DecimalSep      string
	GroupingSep     string
	Mapping         map[string]string
	AccountsMapping map[string]string
	HoldingsMapping map[string]string
	Unresolved      []CSVUnresolvedAction
}

type CSVPreviewView struct {
	Profile     string
	Headers     []string
	PreviewRows [][]string
	Stats       CSVPreviewStats
	Errors      []CSVRowError
	Warnings    []CSVWarning
	Unresolved  []CSVUnresolvedName
	CanCommit   bool
}

func (s *Service) SelectCSV(profile, sessionToken, fileName string, data []byte) (CSVFileSelection, error) {
	codec, err := s.requireCSV()
	if err != nil {
		return CSVFileSelection{}, err
	}
	if int64(len(data)) > CSVMaxFileBytes {
		return CSVFileSelection{}, &domain.Error{Code: domain.ErrCSVLimitExceeded, Message: "CSV file exceeds the size limit"}
	}
	delimiter := codec.DetectDelimiter(string(data))
	table, err := codec.Parse(data, delimiter)
	if err != nil {
		return CSVFileSelection{}, err
	}
	s.csvMu.Lock()
	defer s.csvMu.Unlock()
	s.expireCSVLocked(s.clock())
	token := sessionToken
	if token == "" {
		if err := s.retainCSVLocked(int64(len(data))); err != nil {
			return CSVFileSelection{}, err
		}
		token = newCSVToken()
		s.csvSessions[token] = &csvImportSession{
			options:         CSVParseOptions{Delimiter: delimiter, DateFormat: CSVDateISO, DecimalSep: ".", GroupingSep: "none"},
			accountsMapping: map[string]string{},
			holdingsMapping: map[string]string{},
			createdAt:       s.clock(),
		}
	}
	session := s.csvSessions[token]
	if session == nil {
		return CSVFileSelection{}, &domain.Error{Code: domain.ErrNotFound, Message: "import session expired"}
	}
	session.previewOK = false
	session.confirmed = false
	copyTable := table
	switch strings.ToLower(profile) {
	case CSVProfileAccounts:
		session.accountsTable = &copyTable
	case CSVProfileHoldings:
		session.holdingsTable = &copyTable
	default:
		return CSVFileSelection{}, &domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "CSV profile is not supported"}
	}
	session.sizeBytes = csvSessionSize(session)
	if session.sizeBytes > maxPendingCSVBytes {
		delete(s.csvSessions, token)
		return CSVFileSelection{}, &domain.Error{Code: domain.ErrCSVLimitExceeded, Message: "CSV file exceeds the size limit"}
	}
	return CSVFileSelection{
		Token: token, Profile: strings.ToLower(profile), FileName: fileName,
		Delimiter: delimiterName(delimiter), HasBOM: table.HasBOM, Headers: table.Headers,
		RowCount: len(table.Rows), ColumnCount: len(table.Headers),
	}, nil
}

func (s *Service) PreviewCSV(ctx context.Context, request CSVPreviewRequest) (CSVPreviewView, error) {
	s.csvMu.Lock()
	s.expireCSVLocked(s.clock())
	session := s.csvSessions[request.Token]
	if session == nil {
		s.csvMu.Unlock()
		return CSVPreviewView{}, &domain.Error{Code: domain.ErrNotFound, Message: "import session expired"}
	}
	session.options = CSVParseOptions{
		Delimiter: parseDelimiter(request.Delimiter), DateFormat: request.DateFormat,
		DecimalSep: request.DecimalSep, GroupingSep: request.GroupingSep, Unresolved: request.Unresolved,
	}
	profile := strings.ToLower(strings.TrimSpace(request.Profile))
	if profile == "" {
		profile = CSVProfileAccounts
	}
	if request.AccountsMapping != nil {
		session.accountsMapping = request.AccountsMapping
	}
	if request.HoldingsMapping != nil {
		session.holdingsMapping = request.HoldingsMapping
	}
	if request.Mapping != nil {
		switch profile {
		case CSVProfileHoldings:
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
	s.csvMu.Unlock()
	plans, err := s.buildCSVPlans(ctx, input)
	if err != nil {
		return CSVPreviewView{}, err
	}
	s.csvMu.Lock()
	session = s.csvSessions[request.Token]
	if session == nil {
		s.csvMu.Unlock()
		return CSVPreviewView{}, &domain.Error{Code: domain.ErrNotFound, Message: "import session expired"}
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
	if profile == CSVProfileAccounts && plans.accounts != nil {
		display = *plans.accounts
	} else if profile == CSVProfileHoldings && plans.holdings != nil {
		display = *plans.holdings
	}
	s.csvMu.Unlock()
	return CSVPreviewView{
		Profile: profile, Headers: display.Headers, PreviewRows: display.PreviewRows, Stats: plans.combined.Stats,
		Errors: plans.combined.Errors, Warnings: plans.combined.Warnings, Unresolved: plans.combined.Unresolved, CanCommit: len(plans.combined.Errors) == 0,
	}, nil
}

func (s *Service) ConfirmCSV(token string) error {
	s.csvMu.Lock()
	defer s.csvMu.Unlock()
	s.expireCSVLocked(s.clock())
	session := s.csvSessions[token]
	if session == nil {
		return &domain.Error{Code: domain.ErrNotFound, Message: "import session expired"}
	}
	if !session.previewOK {
		return &domain.Error{Code: domain.ErrCSVRowInvalid, Message: "CSV import must be previewed before confirmation"}
	}
	plan := session.holdingsPlan
	if plan == nil {
		plan = session.accountsPlan
	}
	if plan == nil || len(plan.Errors) > 0 {
		return &domain.Error{Code: domain.ErrCSVRowInvalid, Message: "CSV import has blocking errors"}
	}
	session.confirmed = true
	return nil
}

func (s *Service) CommitCSV(ctx context.Context, token string) (CSVPreviewStats, error) {
	s.csvMu.Lock()
	s.expireCSVLocked(s.clock())
	session := s.csvSessions[token]
	if session == nil {
		s.csvMu.Unlock()
		return CSVPreviewStats{}, &domain.Error{Code: domain.ErrNotFound, Message: "import session expired"}
	}
	if !session.confirmed {
		s.csvMu.Unlock()
		return CSVPreviewStats{}, &domain.Error{Code: domain.ErrCSVRowInvalid, Message: "CSV import must be previewed and confirmed before commit"}
	}
	input := snapshotCSVSession(session)
	s.csvMu.Unlock()
	stats, err := s.CommitCSVImportBuilt(ctx, func() (CSVImportPlan, error) {
		return s.buildCombinedPlan(ctx, input)
	})
	if err != nil {
		return CSVPreviewStats{}, err
	}
	s.csvMu.Lock()
	delete(s.csvSessions, token)
	s.csvMu.Unlock()
	return stats, nil
}

func (s *Service) CSVErrorReport(token string) ([]byte, error) {
	codec, err := s.requireCSV()
	if err != nil {
		return nil, err
	}
	s.csvMu.Lock()
	s.expireCSVLocked(s.clock())
	session := s.csvSessions[token]
	s.csvMu.Unlock()
	if session == nil {
		return nil, &domain.Error{Code: domain.ErrNotFound, Message: "import session expired"}
	}
	plan := session.holdingsPlan
	if plan == nil {
		plan = session.accountsPlan
	}
	if plan == nil {
		return nil, nil
	}
	rows := make([][]string, 0, len(plan.Errors))
	for _, item := range plan.Errors {
		rows = append(rows, []string{strconv.Itoa(item.Row), item.Field, item.Source, item.Code, item.Message, item.Suggestion})
	}
	return codec.Encode([]string{"row", "field", "source", "code", "message", "suggestion"}, rows)
}

func (s *Service) CancelCSV(token string) {
	s.csvMu.Lock()
	delete(s.csvSessions, token)
	s.csvMu.Unlock()
}

func (s *Service) ShutdownCSV() {
	if s == nil {
		return
	}
	s.csvMu.Lock()
	s.csvSessions = map[string]*csvImportSession{}
	s.csvMu.Unlock()
}

type csvPlanInput struct {
	accountsTable   *CSVTable
	holdingsTable   *CSVTable
	accountsMapping map[string]string
	holdingsMapping map[string]string
	options         CSVParseOptions
}

func snapshotCSVSession(session *csvImportSession) csvPlanInput {
	input := csvPlanInput{
		accountsMapping: copyStringMap(session.accountsMapping),
		holdingsMapping: copyStringMap(session.holdingsMapping),
		options:         session.options,
	}
	input.options.Unresolved = append([]CSVUnresolvedAction{}, session.options.Unresolved...)
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
	combined CSVImportPlan
	accounts *CSVImportPlan
	holdings *CSVImportPlan
}

func (s *Service) buildCSVPlans(ctx context.Context, input csvPlanInput) (csvPlanSet, error) {
	var result csvPlanSet
	if input.accountsTable != nil {
		plan, err := s.BuildCSVImportPlan(ctx, CSVProfileAccounts, *input.accountsTable, input.accountsMapping, input.options, nil)
		if err != nil {
			return csvPlanSet{}, err
		}
		result.accounts = &plan
		result.combined = plan
	}
	if input.holdingsTable != nil {
		plan, err := s.BuildCSVImportPlan(ctx, CSVProfileHoldings, *input.holdingsTable, input.holdingsMapping, input.options, &result.combined)
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

func (s *Service) buildCombinedPlan(ctx context.Context, input csvPlanInput) (CSVImportPlan, error) {
	plans, err := s.buildCSVPlans(ctx, input)
	if err != nil {
		return CSVImportPlan{}, err
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

func (s *Service) expireCSVLocked(now time.Time) {
	for token, session := range s.csvSessions {
		if session == nil || now.Sub(session.createdAt) >= CSVImportSessionTTL {
			delete(s.csvSessions, token)
		}
	}
}

func (s *Service) retainCSVLocked(extraBytes int64) error {
	for {
		if len(s.csvSessions) < maxPendingCSVSessions && s.csvBytesLocked()+extraBytes <= maxPendingCSVBytes {
			return nil
		}
		if len(s.csvSessions) == 0 {
			return &domain.Error{Code: domain.ErrCSVLimitExceeded, Message: "CSV file exceeds the size limit"}
		}
		delete(s.csvSessions, oldestCSVToken(s.csvSessions))
	}
}

func (s *Service) csvBytesLocked() int64 {
	var total int64
	for _, session := range s.csvSessions {
		if session != nil {
			total += session.sizeBytes
		}
	}
	return total
}

func csvSessionSize(session *csvImportSession) int64 {
	if session == nil {
		return 0
	}
	var total int64
	if session.accountsTable != nil {
		total += int64(len(session.accountsTable.Headers))
		for _, row := range session.accountsTable.Rows {
			for _, cell := range row {
				total += int64(len(cell))
			}
		}
	}
	if session.holdingsTable != nil {
		total += int64(len(session.holdingsTable.Headers))
		for _, row := range session.holdingsTable.Rows {
			for _, cell := range row {
				total += int64(len(cell))
			}
		}
	}
	return total
}

func oldestCSVToken(sessions map[string]*csvImportSession) string {
	var token string
	var created time.Time
	for key, session := range sessions {
		if session == nil {
			continue
		}
		if token == "" || session.createdAt.Before(created) {
			token = key
			created = session.createdAt
		}
	}
	return token
}

func newCSVToken() string {
	var entropy [8]byte
	_, _ = rand.Read(entropy[:])
	return hex.EncodeToString(entropy[:])
}
