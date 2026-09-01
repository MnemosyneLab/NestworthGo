package sqlite

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) SnapshotTo(ctx context.Context, dest string) error {
	if r == nil || r.database == nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "database is not open"}
	}
	return r.database.SnapshotTo(ctx, dest)
}

func (r *Repository) CheckpointWAL(ctx context.Context) error {
	if r == nil || r.database == nil {
		return &domain.Error{Code: domain.ErrUnavailable, Message: "database is not open"}
	}
	return r.database.CheckpointWAL(ctx)
}

func (r *Repository) Close() error {
	if r == nil || r.database == nil {
		return nil
	}
	return r.database.Close()
}

func (r *Repository) Path() string {
	if r == nil || r.database == nil {
		return ""
	}
	return r.database.Path
}

func (r *Repository) PreviewCounts(ctx context.Context) (accounts, holdings, activities int, err error) {
	if r == nil || r.database == nil {
		return 0, 0, 0, &domain.Error{Code: domain.ErrUnavailable, Message: "database is not open"}
	}
	counts, err := r.database.EntityCounts(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	return counts.Accounts, counts.Holdings, counts.Activities, nil
}
