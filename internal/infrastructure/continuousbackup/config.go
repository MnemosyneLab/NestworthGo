// Package continuousbackup owns app-running replication. Its separate local
// configuration database is never replicated, exported, or exposed through MCP.
package continuousbackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var ErrConfiguration = errors.New("backup configuration is invalid; check the account, bucket and credential pair")
var ErrUnavailable = errors.New("backup storage is unavailable; check the connection and credentials")
var ErrDisabled = errors.New("continuous backup is disabled")
var accountPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

type config struct {
	Enabled         bool
	AccountID       string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	BackupID        string
}

// Update never returns credentials. Blank fields keep both existing secrets.
// Removal is explicit and requires backup to be disabled.
type Update struct {
	Enabled           bool   `json:"enabled"`
	AccountID         string `json:"accountID"`
	Bucket            string `json:"bucket"`
	AccessKeyID       string `json:"accessKeyID"`
	SecretAccessKey   string `json:"secretAccessKey"`
	RemoveCredentials bool   `json:"removeCredentials"`
}
type View struct {
	Enabled               bool   `json:"enabled"`
	AccountID             string `json:"accountID"`
	Bucket                string `json:"bucket"`
	CredentialsConfigured bool   `json:"credentialsConfigured"`
	BackupID              string `json:"backupID"`
	State                 string `json:"state"`
	StreamID              string `json:"streamID"`
	LastAttempt           string `json:"lastAttempt"`
	LastSuccessfulBackup  string `json:"lastSuccessfulBackup"`
	ErrorSummary          string `json:"errorSummary"`
	RestoreState          string `json:"restoreState"`
}
type status struct {
	State                string
	StreamID             string
	LastAttempt          string
	LastSuccessfulBackup string
	ErrorSummary         string
	RestoreState         string
}
type configStore struct {
	db *sql.DB
	mu sync.Mutex
}

func openConfig(path string) (*configStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, ErrUnavailable
	}
	// Precreate privately before SQLite can write headers or credentials.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, ErrUnavailable
	}
	f.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return nil, ErrUnavailable
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, ErrUnavailable
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS backup_local (id INTEGER PRIMARY KEY CHECK(id=1), config TEXT NOT NULL, status TEXT NOT NULL)`); err != nil {
		db.Close()
		return nil, ErrUnavailable
	}
	s := &configStore{db: db}
	c := config{BackupID: uuid.NewString()}
	cb, _ := json.Marshal(c)
	sb, _ := json.Marshal(status{State: "disabled"})
	if _, err = db.Exec(`INSERT OR IGNORE INTO backup_local VALUES(1,?,?)`, string(cb), string(sb)); err != nil {
		db.Close()
		return nil, ErrUnavailable
	}
	return s, nil
}
func (s *configStore) load() (config, status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cb, sb string
	var c config
	var st status
	if err := s.db.QueryRow(`SELECT config,status FROM backup_local WHERE id=1`).Scan(&cb, &sb); err != nil {
		return c, st, ErrUnavailable
	}
	if json.Unmarshal([]byte(cb), &c) != nil || json.Unmarshal([]byte(sb), &st) != nil {
		return c, st, ErrUnavailable
	}
	return c, st, nil
}
func (s *configStore) save(c config, st status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cb, _ := json.Marshal(c)
	sb, _ := json.Marshal(st)
	_, err := s.db.Exec(`UPDATE backup_local SET config=?,status=? WHERE id=1`, string(cb), string(sb))
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *configStore) writeStatus(st status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sb, _ := json.Marshal(st)
	if _, err := s.db.Exec(`UPDATE backup_local SET status=? WHERE id=1`, string(sb)); err != nil {
		return ErrUnavailable
	}
	return nil
}
func validateUpdate(old config, u Update) (config, error) {
	c := old
	c.Enabled = u.Enabled
	c.AccountID = strings.TrimSpace(u.AccountID)
	c.Bucket = strings.TrimSpace(u.Bucket)
	a, b := strings.TrimSpace(u.AccessKeyID), strings.TrimSpace(u.SecretAccessKey)
	if (a == "") != (b == "") || strings.Contains(a, "••••") || strings.Contains(b, "••••") {
		return old, ErrConfiguration
	}
	if u.RemoveCredentials {
		if u.Enabled || a != "" || b != "" {
			return old, ErrConfiguration
		}
		c.AccessKeyID = ""
		c.SecretAccessKey = ""
	} else if a != "" {
		c.AccessKeyID = a
		c.SecretAccessKey = b
	}
	if c.AccountID != "" && !accountPattern.MatchString(c.AccountID) || c.Bucket != "" && !bucketPattern.MatchString(c.Bucket) {
		return old, ErrConfiguration
	}
	if c.Enabled && (c.AccountID == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "") {
		return old, ErrConfiguration
	}
	if _, err := uuid.Parse(c.BackupID); err != nil {
		return old, ErrConfiguration
	}
	return c, nil
}
func endpoint(c config) string {
	return "https://" + strings.ToLower(c.AccountID) + ".r2.cloudflarestorage.com"
}
func contextTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, operationTimeout)
}
