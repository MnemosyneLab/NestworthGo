package application

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// ExclusiveKind names the operation that currently owns the exclusive write
// gate. It is recorded for tests and diagnostics; it is not a user-facing
// string.
type ExclusiveKind string

const (
	ExclusiveBackup  ExclusiveKind = "backup"
	ExclusiveRestore ExclusiveKind = "restore"
)

type writePermitKey struct{}
type exclusivePermitKey struct{}

type writePermit struct {
	ledger         bool
	writeRequested atomic.Bool
}

// WriteCoordinator is the application-owned write gate.
//
// Lock order, from outermost to innermost:
//  1. WriteCoordinator (ordinary write permit or exclusive permit)
//  2. Service.changeMu (ledger read-modify-write only)
//  3. SQLite transaction
//
// Ordinary mutations enter through WithWrite / beginWrite. Backup and restore
// enter through WithExclusive. Starting exclusive work fences
// in-flight refresh persistence by incrementing Service.refreshEpoch so a
// late provider result cannot write after exclusive work has begun.
type WriteCoordinator struct {
	mu           sync.Mutex
	cond         *sync.Cond
	exclusive    bool
	shuttingDown bool
	writers      int
	// revision changes after every outer write permit, including failed writes.
	// The nonce makes preview tokens invalid after a process restart.
	revision uint64
	nonce    string
}

func (c *WriteCoordinator) init() {
	c.cond = sync.NewCond(&c.mu)
	c.nonce = uuid.NewString()
}

func backupRestoreBusy() error {
	return &domain.Error{Code: domain.ErrBackupRestoreBusy, Message: "a backup or restore is already in progress"}
}

func permitFrom(ctx context.Context) *writePermit {
	if ctx == nil {
		return nil
	}
	permit, _ := ctx.Value(writePermitKey{}).(*writePermit)
	return permit
}

func exclusiveKindFrom(ctx context.Context) (ExclusiveKind, bool) {
	if ctx == nil {
		return "", false
	}
	kind, ok := ctx.Value(exclusivePermitKey{}).(ExclusiveKind)
	return kind, ok
}

func (c *WriteCoordinator) acquireWrite() error {
	c.mu.Lock()
	for {
		if c.exclusive || c.shuttingDown {
			c.mu.Unlock()
			return backupRestoreBusy()
		}
		if c.writers == 0 {
			c.writers = 1
			c.mu.Unlock()
			return nil
		}
		c.cond.Wait()
	}
}

func (c *WriteCoordinator) releaseWrite() {
	c.mu.Lock()
	c.revision++
	c.writers--
	c.cond.Broadcast()
	c.mu.Unlock()
}

// releasePreviewRead frees a write-coordinator slot without marking a
// mutation. Preview reads need the same exclusion as writes for a coherent
// snapshot, but must not invalidate their own tokens.
func (c *WriteCoordinator) releasePreviewRead() {
	c.mu.Lock()
	c.writers--
	c.cond.Broadcast()
	c.mu.Unlock()
}

func (c *WriteCoordinator) previewToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nonce + ":" + strconv.FormatUint(c.revision, 10)
}

func (c *WriteCoordinator) acquireExclusive(epoch *atomic.Uint64) error {
	c.mu.Lock()
	if c.exclusive || c.shuttingDown {
		c.mu.Unlock()
		return backupRestoreBusy()
	}
	c.exclusive = true
	if epoch != nil {
		epoch.Add(1)
	}
	c.cond.Broadcast()
	for c.writers > 0 {
		c.cond.Wait()
	}
	c.mu.Unlock()
	return nil
}

func (c *WriteCoordinator) releaseExclusive() {
	c.mu.Lock()
	c.revision++
	c.exclusive = false
	c.cond.Broadcast()
	c.mu.Unlock()
}

// beginPreviewRead serializes a read of all facts needed for a ledger preview
// with application writes. A caller already holding a permit stays reentrant.
func (s *Service) beginPreviewRead(ctx context.Context) (func(), error) {
	if permitFrom(ctx) != nil {
		return func() {}, nil
	}
	if _, ok := exclusiveKindFrom(ctx); ok {
		return func() {}, nil
	}
	if err := s.writes.acquireWrite(); err != nil {
		return nil, err
	}
	return s.writes.releasePreviewRead, nil
}

func (s *Service) beginWrite(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if permit := permitFrom(ctx); permit != nil {
		permit.writeRequested.Store(true)
		return ctx, func() {}, nil
	}
	if _, ok := exclusiveKindFrom(ctx); ok {
		ctx = context.WithValue(ctx, writePermitKey{}, &writePermit{})
		return ctx, func() {}, nil
	}
	if err := s.writes.acquireWrite(); err != nil {
		return ctx, nil, err
	}
	ctx = context.WithValue(ctx, writePermitKey{}, &writePermit{})
	return ctx, s.writes.releaseWrite, nil
}

func (s *Service) beginLedgerWrite(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if permit := permitFrom(ctx); permit != nil {
		permit.writeRequested.Store(true)
		if permit.ledger {
			return ctx, func() {}, nil
		}
		s.changeMu.Lock()
		permit.ledger = true
		return ctx, func() {
			permit.ledger = false
			s.changeMu.Unlock()
		}, nil
	}
	if _, ok := exclusiveKindFrom(ctx); ok {
		s.changeMu.Lock()
		ctx = context.WithValue(ctx, writePermitKey{}, &writePermit{ledger: true})
		return ctx, s.changeMu.Unlock, nil
	}
	if err := s.writes.acquireWrite(); err != nil {
		return ctx, nil, err
	}
	s.changeMu.Lock()
	ctx = context.WithValue(ctx, writePermitKey{}, &writePermit{ledger: true})
	return ctx, func() {
		s.changeMu.Unlock()
		s.writes.releaseWrite()
	}, nil
}

// withSnapshotMaintenance holds the ordinary serial gate through coverage
// reads and possible reconstruction. Pure reads release it without changing
// preview tokens; nested write entry points mark attempts, including partial
// or failed rebuilds. Existing outer write/exclusive permits keep their rules.
func (s *Service) withSnapshotMaintenance(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if permitFrom(ctx) != nil {
		return fn(ctx)
	}
	if _, ok := exclusiveKindFrom(ctx); ok {
		return fn(ctx)
	}
	if err := s.writes.acquireWrite(); err != nil {
		return err
	}
	permit := &writePermit{}
	defer func() {
		if permit.writeRequested.Load() {
			s.writes.releaseWrite()
		} else {
			s.writes.releasePreviewRead()
		}
	}()
	return fn(context.WithValue(ctx, writePermitKey{}, permit))
}

// WithWrite runs fn with an ordinary write permit. Nested calls on the same
// context are reentrant. If exclusive work is active or waiting, WithWrite
// returns backup_restore_busy immediately instead of waiting on SQLite.
func (s *Service) WithWrite(ctx context.Context, fn func(context.Context) error) error {
	ctx, unlock, err := s.beginWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return fn(ctx)
}

// WithExclusive runs fn with the exclusive permit used by backup and restore
// operations. A second exclusive caller receives backup_restore_busy.
// In-flight ordinary writers are allowed to finish; new ordinary writers
// receive backup_restore_busy without waiting on SQLite's busy timeout.
func (s *Service) WithExclusive(ctx context.Context, kind ExclusiveKind, fn func(context.Context) error) error {
	return s.withExclusive(ctx, kind, func(ctx context.Context) (bool, error) {
		return false, fn(ctx)
	})
}

// WithExclusiveKeep is Restore's variant: returning keepHeld=true leaves the
// exclusive permit held because the live session is closed and the process
// is about to quit.
func (s *Service) WithExclusiveKeep(ctx context.Context, kind ExclusiveKind, fn func(context.Context) (keepHeld bool, err error)) error {
	return s.withExclusive(ctx, kind, fn)
}

func (s *Service) withExclusive(ctx context.Context, kind ExclusiveKind, fn func(context.Context) (bool, error)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := exclusiveKindFrom(ctx); ok {
		keepHeld, err := fn(ctx)
		_ = keepHeld
		return err
	}
	if err := s.writes.acquireExclusive(&s.refreshEpoch); err != nil {
		return err
	}
	held := true
	defer func() {
		if held {
			s.writes.releaseExclusive()
		}
	}()
	ctx = context.WithValue(ctx, exclusivePermitKey{}, kind)
	keepHeld, err := fn(ctx)
	if keepHeld {
		held = false
	}
	return err
}

func (s *Service) persistRefreshWrite(ctx context.Context, epoch uint64, persist func(context.Context) error) error {
	ctx, unlock, err := s.beginWrite(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if s.refreshEpoch.Load() != epoch {
		return backupRestoreBusy()
	}
	return persist(ctx)
}

// QuiesceWritesForShutdown permanently rejects new write permits and drains
// existing writers. It also fences late provider results before their workers
// are canceled/joined. It does not take changeMu or borrow a SQLite connection,
// so it remains safe after a restore retained its exclusive permit and mutex.
// Call only after the host has committed to exiting; there is no resume path.
func (s *Service) QuiesceWritesForShutdown() {
	c := &s.writes
	c.mu.Lock()
	c.shuttingDown = true
	s.refreshEpoch.Add(1)
	c.cond.Broadcast()
	for c.writers > 0 {
		c.cond.Wait()
	}
	c.mu.Unlock()
}
