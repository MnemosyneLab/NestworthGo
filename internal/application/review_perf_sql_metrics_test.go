package application

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	modernsqlite "modernc.org/sqlite"
)

const reviewPerfDriverName = "nestworth-review-perf-metrics"

var reviewPerfStats = &reviewSQLStats{byStatement: make(map[string]*reviewSQLStatement)}

func init() {
	sql.Register(reviewPerfDriverName, &reviewSQLDriver{base: &modernsqlite.Driver{}, stats: reviewPerfStats})
}

// enableReviewSQLMetrics swaps only the synthetic benchmark's already-created
// database connection pool for an equivalent pool with a driver decorator.
// The wrapper is test-only and does not affect application or production SQL.
func enableReviewSQLMetrics(t testingT, fixture reviewFullPipelineFixture) *reviewSQLStats {
	t.Helper()
	if err := fixture.db.SQL.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open(reviewPerfDriverName, fixture.dbPath+"?_txlock=immediate&_pragma=busy_timeout%3d5000&_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(0)
	fixture.db.SQL = database
	reviewPerfStats.reset()
	return reviewPerfStats
}

// testingT is intentionally tiny so metric setup is easy to use from opt-in
// benchmark evidence without coupling the driver wrapper to *testing.T.
type testingT interface {
	Helper()
	Fatal(...any)
}

type reviewSQLDriver struct {
	base  driver.Driver
	stats *reviewSQLStats
}

func (d *reviewSQLDriver) Open(name string) (driver.Conn, error) {
	connection, err := d.base.Open(name)
	if err != nil {
		return nil, err
	}
	return &reviewSQLConn{Conn: connection, stats: d.stats}, nil
}

type reviewSQLConn struct {
	driver.Conn
	stats *reviewSQLStats
}

func (c *reviewSQLConn) Prepare(query string) (driver.Stmt, error) {
	started := time.Now()
	statement, err := c.Conn.Prepare(query)
	c.stats.recordPrepare(query, time.Since(started))
	if err != nil {
		return nil, err
	}
	return &reviewSQLStmt{Stmt: statement, query: query, stats: c.stats}, nil
}

func (c *reviewSQLConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	started := time.Now()
	var statement driver.Stmt
	var err error
	if preparer, ok := c.Conn.(driver.ConnPrepareContext); ok {
		statement, err = preparer.PrepareContext(ctx, query)
	} else {
		statement, err = c.Conn.Prepare(query)
	}
	c.stats.recordPrepare(query, time.Since(started))
	if err != nil {
		return nil, err
	}
	return &reviewSQLStmt{Stmt: statement, query: query, stats: c.stats}, nil
}

func (c *reviewSQLConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if beginner, ok := c.Conn.(driver.ConnBeginTx); ok {
		return beginner.BeginTx(ctx, opts)
	}
	if opts.ReadOnly || opts.Isolation != driver.IsolationLevel(0) {
		return nil, driver.ErrSkip
	}
	return c.Conn.Begin()
}

func (c *reviewSQLConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	started := time.Now()
	var result driver.Result
	var err error
	if execer, ok := c.Conn.(driver.ExecerContext); ok {
		result, err = execer.ExecContext(ctx, query, args)
	} else {
		statement, prepareErr := c.PrepareContext(ctx, query)
		if prepareErr != nil {
			return nil, prepareErr
		}
		defer statement.Close()
		result, err = statement.(driver.StmtExecContext).ExecContext(ctx, args)
	}
	c.stats.recordExec(query, result, time.Since(started))
	return result, err
}

func (c *reviewSQLConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	started := time.Now()
	var rows driver.Rows
	var err error
	if querier, ok := c.Conn.(driver.QueryerContext); ok {
		rows, err = querier.QueryContext(ctx, query, args)
	} else {
		statement, prepareErr := c.PrepareContext(ctx, query)
		if prepareErr != nil {
			return nil, prepareErr
		}
		defer statement.Close()
		rows, err = statement.(driver.StmtQueryContext).QueryContext(ctx, args)
	}
	if err != nil {
		c.stats.recordQuery(query, 0, time.Since(started))
		return nil, err
	}
	return &reviewSQLRows{Rows: rows, query: query, stats: c.stats, started: started}, nil
}

type reviewSQLStmt struct {
	driver.Stmt
	query string
	stats *reviewSQLStats
}

func (s *reviewSQLStmt) Exec(args []driver.Value) (driver.Result, error) {
	started := time.Now()
	result, err := s.Stmt.Exec(args)
	s.stats.recordExec(s.query, result, time.Since(started))
	return result, err
}

func (s *reviewSQLStmt) Query(args []driver.Value) (driver.Rows, error) {
	started := time.Now()
	rows, err := s.Stmt.Query(args)
	if err != nil {
		s.stats.recordQuery(s.query, 0, time.Since(started))
		return nil, err
	}
	return &reviewSQLRows{Rows: rows, query: s.query, stats: s.stats, started: started}, nil
}

func (s *reviewSQLStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	started := time.Now()
	var result driver.Result
	var err error
	if execer, ok := s.Stmt.(driver.StmtExecContext); ok {
		result, err = execer.ExecContext(ctx, args)
	} else {
		values, convertErr := reviewDriverValues(args)
		if convertErr != nil {
			return nil, convertErr
		}
		result, err = s.Stmt.Exec(values)
	}
	s.stats.recordExec(s.query, result, time.Since(started))
	return result, err
}

func (s *reviewSQLStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	started := time.Now()
	var rows driver.Rows
	var err error
	if querier, ok := s.Stmt.(driver.StmtQueryContext); ok {
		rows, err = querier.QueryContext(ctx, args)
	} else {
		values, convertErr := reviewDriverValues(args)
		if convertErr != nil {
			return nil, convertErr
		}
		rows, err = s.Stmt.Query(values)
	}
	if err != nil {
		s.stats.recordQuery(s.query, 0, time.Since(started))
		return nil, err
	}
	return &reviewSQLRows{Rows: rows, query: s.query, stats: s.stats, started: started}, nil
}

func reviewDriverValues(args []driver.NamedValue) ([]driver.Value, error) {
	values := make([]driver.Value, len(args))
	for index, arg := range args {
		value, ok := arg.Value.(driver.Value)
		if !ok {
			return nil, fmt.Errorf("unsupported SQL driver argument type %T", arg.Value)
		}
		values[index] = value
	}
	return values, nil
}

type reviewSQLRows struct {
	driver.Rows
	query   string
	stats   *reviewSQLStats
	started time.Time
	count   int64
	once    sync.Once
}

func (r *reviewSQLRows) Next(dest []driver.Value) error {
	err := r.Rows.Next(dest)
	if err == nil {
		r.count++
	}
	return err
}

func (r *reviewSQLRows) Close() error {
	err := r.Rows.Close()
	r.once.Do(func() { r.stats.recordQuery(r.query, r.count, time.Since(r.started)) })
	return err
}

type reviewSQLStatement struct {
	query                   string
	queries, rows           int64
	queryTime               time.Duration
	execs, writes, affected int64
	execTime                time.Duration
	prepares                int64
	prepareTime             time.Duration
}

type reviewSQLSnapshot struct {
	queries, rows           int64
	queryTime               time.Duration
	execs, writes, affected int64
	execTime                time.Duration
	prepares                int64
	prepareTime             time.Duration
	statements              []reviewSQLStatement
}

type reviewSQLStats struct {
	mu          sync.Mutex
	byStatement map[string]*reviewSQLStatement
	queries     int64
	rows        int64
	queryTime   time.Duration
	execs       int64
	writes      int64
	affected    int64
	execTime    time.Duration
	prepares    int64
	prepareTime time.Duration
}

func (s *reviewSQLStats) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byStatement = make(map[string]*reviewSQLStatement)
	s.queries, s.rows, s.queryTime = 0, 0, 0
	s.execs, s.writes, s.affected, s.execTime = 0, 0, 0, 0
	s.prepares, s.prepareTime = 0, 0
}

func (s *reviewSQLStats) statement(query string) *reviewSQLStatement {
	if item := s.byStatement[query]; item != nil {
		return item
	}
	item := &reviewSQLStatement{query: query}
	s.byStatement[query] = item
	return item
}

func (s *reviewSQLStats) recordPrepare(query string, elapsed time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.statement(query)
	item.prepares++
	item.prepareTime += elapsed
	s.prepares++
	s.prepareTime += elapsed
}

func (s *reviewSQLStats) recordQuery(query string, rows int64, elapsed time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.statement(query)
	item.queries++
	item.rows += rows
	item.queryTime += elapsed
	s.queries++
	s.rows += rows
	s.queryTime += elapsed
}

func (s *reviewSQLStats) recordExec(query string, result driver.Result, elapsed time.Duration) {
	var affected int64
	if result != nil {
		affected, _ = result.RowsAffected()
	}
	isWrite := reviewIsWriteQuery(query)
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.statement(query)
	item.execs++
	item.execTime += elapsed
	if isWrite {
		item.writes++
		item.affected += affected
		s.writes++
		s.affected += affected
	}
	s.execs++
	s.execTime += elapsed
}

func reviewIsWriteQuery(query string) bool {
	query = strings.TrimSpace(strings.ToUpper(query))
	for _, keyword := range []string{"INSERT ", "UPDATE ", "DELETE ", "REPLACE ", "CREATE ", "DROP ", "ALTER ", "PRAGMA "} {
		if strings.HasPrefix(query, keyword) {
			return true
		}
	}
	return false
}

func (s *reviewSQLStats) snapshot(limit int) reviewSQLSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := reviewSQLSnapshot{queries: s.queries, rows: s.rows, queryTime: s.queryTime, execs: s.execs, writes: s.writes, affected: s.affected, execTime: s.execTime, prepares: s.prepares, prepareTime: s.prepareTime}
	for _, item := range s.byStatement {
		result.statements = append(result.statements, *item)
	}
	sort.Slice(result.statements, func(i, j int) bool { return result.statements[i].queryTime > result.statements[j].queryTime })
	if limit > 0 && len(result.statements) > limit {
		result.statements = result.statements[:limit]
	}
	return result
}

func (s reviewSQLSnapshot) summary() string {
	return fmt.Sprintf("sql queries=%d rowsRead=%d driverQueryTime=%s execs=%d writeStatements=%d rowsAffected=%d driverExecTime=%s prepares=%d prepareTime=%s", s.queries, s.rows, s.queryTime, s.execs, s.writes, s.affected, s.execTime, s.prepares, s.prepareTime)
}

func (s reviewSQLSnapshot) slowest() string {
	parts := make([]string, 0, len(s.statements))
	for _, item := range s.statements {
		query := strings.Join(strings.Fields(item.query), " ")
		if len(query) > 150 {
			query = query[:147] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s calls=%d rows=%d driver=%s sql=%q", item.queryTime, item.queries, item.rows, item.queryTime, query))
	}
	return strings.Join(parts, " | ")
}

var _ driver.Driver = (*reviewSQLDriver)(nil)
var _ driver.Conn = (*reviewSQLConn)(nil)
var _ driver.ConnPrepareContext = (*reviewSQLConn)(nil)
var _ driver.ConnBeginTx = (*reviewSQLConn)(nil)
var _ driver.ExecerContext = (*reviewSQLConn)(nil)
var _ driver.QueryerContext = (*reviewSQLConn)(nil)
var _ driver.StmtExecContext = (*reviewSQLStmt)(nil)
var _ driver.StmtQueryContext = (*reviewSQLStmt)(nil)
var _ driver.Rows = (*reviewSQLRows)(nil)
