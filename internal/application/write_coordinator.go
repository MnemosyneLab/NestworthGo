package application

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ExclusiveKind names the operation that currently owns the exclusive write
// gate. It is recorded for tests and diagnostics; it is not a user-facing
// string.
type ExclusiveKind string

const (
	ExclusiveBackup  ExclusiveKind = "backup"
	ExclusiveRestore ExclusiveKind = "restore"
	ExclusiveCSV     ExclusiveKind = "csv"
)

type writePermitKey struct{}
type exclusivePermitKey struct{}

type writePermit struct {
	ledger bool
}

// WriteCoordinator is the application-owned write gate.
//
// Lock order, from outermost to innermost:
//  1. WriteCoordinator (ordinary write permit or exclusive permit)
//  2. Service.changeMu (ledger read-modify-write only)
//  3. SQLite transaction
//
// Ordinary mutations enter through WithWrite / beginWrite. Backup, Restore,
// and CSV commit enter through WithExclusive. Starting exclusive work fences
// in-flight refresh persistence by incrementing Service.refreshEpoch so a
// late provider result cannot write after exclusive work has begun.
type WriteCoordinator struct {
	mu        sync.Mutex
	cond      *sync.Cond
	exclusive bool
	writers   int
}

func (c *WriteCoordinator) init() {
	c.cond = sync.NewCond(&c.mu)
}

func backupRestoreBusy() error {
	return &domain.Error{Code: domain.ErrBackupRestoreBusy, Message: "a backup, restore, or import is already in progress"}
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
		if c.exclusive {
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
	c.writers--
	c.cond.Broadcast()
	c.mu.Unlock()
}

func (c *WriteCoordinator) acquireExclusive(epoch *atomic.Uint64) error {
	c.mu.Lock()
	if c.exclusive {
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
	c.exclusive = false
	c.cond.Broadcast()
	c.mu.Unlock()
}

func (s *Service) beginWrite(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if permitFrom(ctx) != nil {
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

// WithExclusive runs fn with the exclusive permit used by backup, Restore,
// and CSV commit. A second exclusive caller receives backup_restore_busy.
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
