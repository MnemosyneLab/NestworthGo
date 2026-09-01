package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) BeginExclusiveOperation() error {
	if !s.exclusive.CompareAndSwap(false, true) {
		return &domain.Error{Code: domain.ErrBackupRestoreBusy, Message: "a backup, restore, or import is already in progress"}
	}
	return nil
}

func (s *Service) EndExclusiveOperation() {
	s.exclusive.Store(false)
}

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
