package pilot

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

// ErrStartupRecovery marks failures in the shared startup restore sequence.
var ErrStartupRecovery = errors.New("the local restore could not be reconciled")

// Session owns a writable database and its shared Wails instance lock.
type Session struct {
	DB           *sqlite.DB
	CashReceipts *CashReceiptStore
	close        func() error
}

// Close checkpoints and closes the database before releasing its lock.
// Callers must first stop work that uses the database.
func (s *Session) Close() error { return s.close() }

func OpenSession(ctx context.Context, paths Paths) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	unlock, err := backup.AcquireInstanceLock(paths.database)
	if err != nil {
		return nil, err
	}
	if err := backup.ReconcileOnStartup(paths.database, settings.NewStore(paths.settings)); err != nil {
		return nil, errors.Join(ErrStartupRecovery, err, unlock())
	}
	db, err := sqlite.Open(paths.database)
	if err != nil {
		return nil, errors.Join(err, unlock())
	}
	info, _ := os.Stat(paths.database)
	receipts := &CashReceiptStore{paths: paths, verified: info, db: db}
	return &Session{DB: db, CashReceipts: receipts, close: sync.OnceValue(func() error {
		receipts.mu.Lock()
		receipts.closed = true
		receipts.mu.Unlock()
		return errors.Join(db.CheckpointWAL(context.Background()), db.Close(), unlock())
	})}, nil
}
