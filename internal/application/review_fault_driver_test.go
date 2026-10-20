package application

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	modernsqlite "modernc.org/sqlite"
)

// Per-fixture driver faults preserve the real SQLite connection and commit
// hook from database creation. They never add triggers, change schema trust,
// replace a certified connection, or expose a production diagnostic API.
type reviewDriverFaults struct {
	mu         sync.Mutex
	afterQuery func(string)
	beforeExec func(string, []driver.NamedValue) (driver.Result, error, bool)
	afterExec  func(string)
}

func (f *reviewDriverFaults) queryClosed(query string) {
	f.mu.Lock()
	callback := f.afterQuery
	f.mu.Unlock()
	if callback != nil {
		callback(query)
	}
}
func (f *reviewDriverFaults) execute(query string, args []driver.NamedValue, run func() (driver.Result, error)) (driver.Result, error) {
	f.mu.Lock()
	before, after := f.beforeExec, f.afterExec
	f.mu.Unlock()
	if before != nil {
		if result, err, handled := before(query, args); handled {
			return result, err
		}
	}
	result, err := run()
	if err == nil && after != nil {
		after(query)
	}
	return result, err
}

var reviewFaultDriverSequence atomic.Uint64

func openReviewFaultDatabase(t *testing.T) (*sqlite.DB, *reviewDriverFaults) {
	t.Helper()
	return openReviewFaultDatabaseAt(t, filepath.Join(t.TempDir(), "synthetic-review.db"))
}

func openReviewFaultDatabaseAt(t *testing.T, path string) (*sqlite.DB, *reviewDriverFaults) {
	t.Helper()
	faults := &reviewDriverFaults{}
	name := fmt.Sprintf("review-fault-%d", reviewFaultDriverSequence.Add(1))
	sql.Register(name, &reviewFaultDriver{faults: faults})
	db, err := sqlite.OpenReviewPerformanceDatabase(path, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, faults
}

type reviewFaultDriver struct{ faults *reviewDriverFaults }

func (d *reviewFaultDriver) Open(name string) (driver.Conn, error) {
	c, err := (&modernsqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &reviewFaultConn{Conn: c, faults: d.faults}, nil
}

type reviewFaultConn struct {
	driver.Conn
	faults *reviewDriverFaults
}

func (c *reviewFaultConn) RegisterCommitHook(fn modernsqlite.CommitHookFn) {
	c.Conn.(interface {
		RegisterCommitHook(modernsqlite.CommitHookFn)
	}).RegisterCommitHook(fn)
}
func (c *reviewFaultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
func (c *reviewFaultConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}
func (c *reviewFaultConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	s, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &reviewFaultStmt{Stmt: s, query: query, faults: c.faults}, nil
}
func (c *reviewFaultConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	return c.faults.execute(q, args, func() (driver.Result, error) { return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args) })
}
func (c *reviewFaultConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
	if err != nil {
		return nil, err
	}
	return &reviewFaultRows{Rows: rows, query: q, faults: c.faults}, nil
}

type reviewFaultStmt struct {
	driver.Stmt
	query  string
	faults *reviewDriverFaults
}

func (s *reviewFaultStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.faults.execute(s.query, args, func() (driver.Result, error) { return s.Stmt.(driver.StmtExecContext).ExecContext(ctx, args) })
}
func (s *reviewFaultStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := s.Stmt.(driver.StmtQueryContext).QueryContext(ctx, args)
	if err != nil {
		return nil, err
	}
	return &reviewFaultRows{Rows: rows, query: s.query, faults: s.faults}, nil
}

type reviewFaultRows struct {
	driver.Rows
	query  string
	faults *reviewDriverFaults
	once   sync.Once
}

func (r *reviewFaultRows) Close() error {
	err := r.Rows.Close()
	r.once.Do(func() { r.faults.queryClosed(r.query) })
	return err
}
