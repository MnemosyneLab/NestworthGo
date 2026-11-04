// Package aistate stores private AI conversations outside the financial database,
// its configuration table, operation receipts and backup/export selection.
package aistate

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	_ "modernc.org/sqlite"
)

type Store struct {
	mu     sync.Mutex
	db     *sql.DB
	lock   *flock.Flock
	closed bool
}

var errStore = errors.New("private assistant storage unavailable")

// Open requires a dedicated host-derived directory. Symlinks and permissive
// directories/files are refused. SQLite DELETE journaling avoids persistent WAL
// copies; secure_delete also applies when conversations are explicitly removed.
func Open(directory, ledger string) (*Store, error) {
	if ledger == "" || !filepath.IsAbs(directory) {
		return nil, errStore
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, errStore
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, errStore
	}
	for _, name := range []string{"state.db", "state.db-journal", "state.lock"} {
		info, err := os.Lstat(filepath.Join(directory, name))
		if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm() != 0600) {
			return nil, errStore
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, errStore
		}
	}
	lock := flock.New(filepath.Join(directory, "state.lock"))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		_ = lock.Close()
		return nil, errStore
	}
	fail := func() (*Store, error) { _ = lock.Unlock(); _ = lock.Close(); return nil, errStore }
	path := filepath.Join(directory, "state.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fail()
	}
	// Reject unknown/unversioned/future files before a writable SQLite open.
	// Version-1 hot journals may then use SQLite's normal recovery path.
	header := make([]byte, 100)
	n, readErr := f.ReadAt(header, 0)
	_ = f.Close()
	if n != 0 && (readErr != nil || !bytes.Equal(header[:16], []byte("SQLite format 3\x00")) || binary.BigEndian.Uint32(header[60:64]) != assistant.StateVersion) {
		return fail()
	}
	if n == 0 && readErr != io.EOF {
		return fail()
	}
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return fail()
	}
	db.SetMaxOpenConns(1)
	reject := func() (*Store, error) { _ = db.Close(); return fail() }
	for _, q := range []string{"PRAGMA journal_mode=DELETE", "PRAGMA synchronous=FULL", "PRAGMA secure_delete=ON", "PRAGMA busy_timeout=1000"} {
		if _, e := db.Exec(q); e != nil {
			return reject()
		}
	}
	var version int
	if db.QueryRow("PRAGMA user_version").Scan(&version) != nil {
		return reject()
	}
	if version == 0 {
		var count int
		if db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&count) != nil || count != 0 {
			return reject()
		}
		tx, e := db.Begin()
		if e != nil {
			return reject()
		}
		_, e = tx.Exec(`CREATE TABLE assistant_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL CHECK(length(payload)<=16777216));`)
		if e == nil {
			_, e = tx.Exec("PRAGMA user_version=1")
		}
		raw, _ := jsoncodec.Marshal(assistant.InitialState(ledger))
		if e == nil {
			_, e = tx.Exec("INSERT INTO assistant_state(id,payload) VALUES(1,?)", raw)
		}
		if e != nil {
			_ = tx.Rollback()
			return reject()
		}
		if tx.Commit() != nil {
			return reject()
		}
	} else if version != assistant.StateVersion {
		return reject()
	}
	var schema string
	var tables int
	if db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&tables) != nil || tables != 1 || db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='assistant_state'").Scan(&schema) != nil || schema != "CREATE TABLE assistant_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL CHECK(length(payload)<=16777216))" {
		return reject()
	}
	var integrity string
	if db.QueryRow("PRAGMA integrity_check").Scan(&integrity) != nil || integrity != "ok" {
		return reject()
	}
	store := &Store{db: db, lock: lock}
	state, e := store.Load()
	if e != nil || state.Ledger != ledger || state.Version != assistant.StateVersion {
		_ = store.Close()
		return nil, errStore
	}
	return store, nil
}
func (s *Store) Load() (assistant.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var state assistant.State
	if s.closed {
		return state, errStore
	}
	var raw []byte
	if s.db.QueryRow("SELECT payload FROM assistant_state WHERE id=1").Scan(&raw) != nil || len(raw) > assistant.MaxStateBytes {
		return state, errStore
	}
	decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil || assistant.ValidateState(state) != nil {
		return assistant.State{}, errStore
	}

	return state, nil
}
func (s *Store) Save(state assistant.State) error {
	raw, e := jsoncodec.Marshal(state)
	if e != nil || len(raw) > assistant.MaxStateBytes || state.Version != assistant.StateVersion {
		return errStore
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errStore
	}
	result, e := s.db.Exec("UPDATE assistant_state SET payload=? WHERE id=1", raw)
	if e != nil {
		return errStore
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		return errStore
	}
	return nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	e := s.db.Close()
	_ = s.lock.Unlock()
	_ = s.lock.Close()
	if e != nil {
		return errStore
	}
	return nil
}
