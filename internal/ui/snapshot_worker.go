package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type snapshotWorkerStatus string

const (
	snapshotWorkerCompleted snapshotWorkerStatus = "completed"
	snapshotWorkerNoHistory snapshotWorkerStatus = "no_history"
	snapshotWorkerNoWork    snapshotWorkerStatus = "no_work"
)

type snapshotWorkerResult struct {
	status    snapshotWorkerStatus
	completed int
}

type snapshotPreparation struct {
	result snapshotWorkerResult
	start  time.Time
	end    time.Time
}

func prepareSnapshot(ctx context.Context, service snapshotService) (snapshotPreparation, error) {
	if err := ctx.Err(); err != nil {
		return snapshotPreparation{}, err
	}
	origin, err := service.HistoryOrigin(ctx)
	if err != nil {
		return snapshotPreparation{}, err
	}
	if err := ctx.Err(); err != nil {
		return snapshotPreparation{}, err
	}
	if origin == nil {
		return snapshotPreparation{result: snapshotWorkerResult{status: snapshotWorkerNoHistory}}, nil
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return snapshotPreparation{}, &domain.Error{Code: domain.ErrValidation, Field: "timezone", Message: "saved history timezone is invalid"}
	}
	now := time.Now().In(location)
	year, month, day := now.Date()
	end := time.Date(year, month, day, 0, 0, 0, 0, location).AddDate(0, 0, -1)
	start := origin.StartedAt.In(location)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, location)
	state, err := service.DailySnapshotState(ctx, origin.HouseholdID)
	if err != nil {
		return snapshotPreparation{}, err
	}
	if err := ctx.Err(); err != nil {
		return snapshotPreparation{}, err
	}
	if state.LastCompletedClosedOn != nil {
		if completed, parseErr := time.ParseInLocation("2006-01-02", *state.LastCompletedClosedOn, location); parseErr == nil {
			candidate := completed.AddDate(0, 0, 1)
			if candidate.After(start) {
				start = candidate
			}
		}
	}
	if state.DirtyFrom != nil {
		if dirty, parseErr := time.ParseInLocation("2006-01-02", *state.DirtyFrom, location); parseErr == nil && dirty.Before(start) {
			start = dirty
		}
	}
	if end.Before(start) {
		return snapshotPreparation{result: snapshotWorkerResult{status: snapshotWorkerNoWork}}, nil
	}
	return snapshotPreparation{result: snapshotWorkerResult{status: snapshotWorkerCompleted}, start: start, end: end}, nil
}

type snapshotService interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	DailySnapshotState(context.Context, domain.HouseholdID) (domain.DailySnapshotState, error)
	RebuildHistoricalSnapshots(context.Context, string, string) (int, error)
}

func snapshotError(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *domain.Error
	if errors.As(err, &domainErr) && domainErr != nil {
		return err
	}
	return &domain.Error{Code: domain.ErrUnavailable, Message: "history snapshots could not be prepared"}
}

func (c *Controller) startSnapshotWorker() {
	service := c.snapshotRunner
	if service == nil && c.service != nil {
		service = snapshotService(c.service)
	}
	if service == nil || c.snapshotTask.pending {
		return
	}
	generation, ctx := c.snapshotTask.begin(c.translator.T("snapshot.updating"))
	c.RefreshContent()
	go func() {
		preparation, preparationErr := prepareSnapshot(ctx, service)
		if preparationErr != nil {
			presentationErr := snapshotError(preparationErr)
			fyne.Do(func() { c.finishSnapshotWorker(generation, snapshotWorkerResult{}, presentationErr) })
			return
		}
		if preparation.result.status != snapshotWorkerCompleted {
			fyne.Do(func() { c.finishSnapshotWorker(generation, preparation.result, nil) })
			return
		}
		completed := 0
		for chunkStart := preparation.start; !chunkStart.After(preparation.end); {
			if err := ctx.Err(); err != nil {
				fyne.Do(func() { c.finishSnapshotWorker(generation, snapshotWorkerResult{}, err) })
				return
			}
			chunkEnd := chunkStart.AddDate(0, 0, 30)
			if chunkEnd.After(preparation.end) {
				chunkEnd = preparation.end
			}
			count, rebuildErr := service.RebuildHistoricalSnapshots(ctx, chunkStart.Format("2006-01-02"), chunkEnd.Format("2006-01-02"))
			completed += count
			if rebuildErr != nil {
				presentationErr := snapshotError(rebuildErr)
				fyne.Do(func() {
					c.finishSnapshotWorker(generation, snapshotWorkerResult{status: snapshotWorkerCompleted, completed: completed}, presentationErr)
				})
				return
			}
			chunkStart = chunkEnd.AddDate(0, 0, 1)
		}
		fyne.Do(func() {
			c.finishSnapshotWorker(generation, snapshotWorkerResult{status: snapshotWorkerCompleted, completed: completed}, nil)
		})
	}()
}

func (c *Controller) finishSnapshotWorker(generation uint64, result snapshotWorkerResult, err error) bool {
	if !c.snapshotTask.finish(generation, result, err) {
		return false
	}
	c.RefreshContent()
	if c.snapshotObserver != nil {
		c.snapshotObserver()
	}
	return true
}

func (c *Controller) cancelSnapshotWorker() {
	c.snapshotTask.stop()
	c.snapshotTask.err = nil
}

func snapshotFeedback(c *Controller) fyne.CanvasObject {
	if c.snapshotTask.pending {
		return mutedLabel(c.snapshotTask.progress)
	}
	if c.snapshotTask.err != nil {
		retry := widget.NewButton(c.translator.T("common.retry"), func() {
			c.snapshotTask.err = nil
			c.startSnapshotWorker()
		})
		retry.Importance = widget.LowImportance
		return container.NewVBox(errorPanel(c, c.translator.T("snapshot.historyRebuild"), c.snapshotTask.err), container.NewCenter(retry))
	}
	if result := c.snapshotTask.payload; result != nil {
		switch result.status {
		case snapshotWorkerNoHistory:
			return mutedLabel(c.translator.T("snapshot.noHistory"))
		case snapshotWorkerCompleted:
			if result.completed > 0 {
				return mutedLabel(fmt.Sprintf(c.translator.T("snapshot.revisionsUpdated"), result.completed))
			}
		}
	}
	return nil
}
