// Package aiusage persists metadata separately from conversations and the ledger.
package aiusage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	_ "modernc.org/sqlite"
)

var errStore = errors.New("assistant_usage_unavailable")

type Store struct {
	db             *sql.DB
	incompletePath string
	incomplete     atomic.Bool
}

// The owning conversation store holds the directory lock. A separate database
// prevents an accounting error or growth from blocking conversation persistence.
func Open(directory string) (*Store, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, errStore
	}
	path := filepath.Join(directory, "usage.db")
	for _, p := range []string{path, path + "-journal", filepath.Join(directory, "usage-incomplete")} {
		if i, e := os.Lstat(p); e == nil {
			if !i.Mode().IsRegular() || i.Mode().Perm() != 0600 {
				return nil, errStore
			}
		} else if !os.IsNotExist(e) {
			return nil, errStore
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errStore
	}
	_ = f.Close()
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, errStore
	}
	db.SetMaxOpenConns(1)
	fail := func() (*Store, error) { _ = db.Close(); return nil, errStore }
	for _, q := range []string{"PRAGMA journal_mode=DELETE", "PRAGMA synchronous=FULL", "PRAGMA busy_timeout=50"} {
		if _, err = db.Exec(q); err != nil {
			return fail()
		}
	}
	var version int
	if db.QueryRow("PRAGMA user_version").Scan(&version) != nil {
		return fail()
	}
	if version == 0 {
		var count int
		if db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&count) != nil || count != 0 {
			return fail()
		}
		tx, e := db.Begin()
		if e != nil {
			return fail()
		}
		if _, e = tx.Exec(`CREATE TABLE usage_calls(id TEXT PRIMARY KEY, payload BLOB NOT NULL CHECK(length(payload)<4096)); PRAGMA user_version=1;`); e != nil {
			_ = tx.Rollback()
			return fail()
		}
		if tx.Commit() != nil {
			return fail()
		}
	} else if version != 1 {
		return fail()
	}
	s := &Store{db: db, incompletePath: filepath.Join(directory, "usage-incomplete")}
	if _, err := os.Stat(s.incompletePath); err == nil {
		s.incomplete.Store(true)
	}
	return s, nil
}
func (s *Store) Record(c assistant.UsageCall) (err error) {
	defer func() {
		if err != nil {
			s.incomplete.Store(true)
			if f, e := os.OpenFile(s.incompletePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600); e == nil {
				_ = f.Sync()
				_ = f.Close()
			}
		}
	}()
	if !assistant.ValidateUsageCall(c) {
		return errStore
	}
	raw, e := jsoncodec.Marshal(c)
	if e != nil {
		return errStore
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, e = s.db.ExecContext(ctx, `INSERT INTO usage_calls(id,payload) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`, c.ID, raw)
	if e != nil {
		return errStore
	}
	return nil
}
func (s *Store) Calls() ([]assistant.UsageCall, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM usage_calls")
	if err != nil {
		return nil, errStore
	}
	defer rows.Close()
	result := []assistant.UsageCall{}
	for rows.Next() {
		var raw []byte
		var c assistant.UsageCall
		if rows.Scan(&raw) != nil || jsoncodec.Unmarshal(raw, &c) != nil || !assistant.ValidateUsageCall(c) {
			return nil, errStore
		}
		result = append(result, c)
	}
	if rows.Err() != nil {
		return nil, errStore
	}
	return result, nil
}
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Incomplete() bool { return s.incomplete.Load() }
