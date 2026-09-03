package application

import (
	"context"
)

// LockWrites takes changeMu. Call it only while holding the exclusive
// permit, around the SQLite snapshot or Restore close path. Do not hold it
// across a save-file dialog or backup packaging I/O.
func (s *Service) LockWrites() {
	s.changeMu.Lock()
}

func (s *Service) UnlockWrites() {
	s.changeMu.Unlock()
}

func (s *Service) CheckpointWAL(ctx context.Context) error {
	return s.repository.CheckpointWAL(ctx)
}

func (s *Service) CloseDatabase() error {
	return s.repository.Close()
}

func (s *Service) SnapshotTo(ctx context.Context, dest string) error {
	return s.repository.SnapshotTo(ctx, dest)
}
