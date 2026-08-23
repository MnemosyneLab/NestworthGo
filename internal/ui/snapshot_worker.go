package ui

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (c *Controller) startSnapshotWorker() {
	if c.service == nil || c.snapshotPending {
		return
	}
	origin, err := c.service.HistoryOrigin(context.Background())
	if err != nil || origin == nil {
		return
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return
	}
	now := time.Now().In(location)
	year, month, day := now.Date()
	end := time.Date(year, month, day, 0, 0, 0, 0, location).AddDate(0, 0, -1)
	start := origin.StartedAt.In(location)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, location)
	state, err := c.service.DailySnapshotState(context.Background(), origin.HouseholdID)
	if err != nil {
		return
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
		return
	}
	c.snapshotGeneration++
	generation := c.snapshotGeneration
	ctx, cancel := context.WithCancel(context.Background())
	c.snapshotCancel = cancel
	c.snapshotPending = true
	c.snapshotProgress = c.translator.T("snapshot.updating")
	c.snapshotError = nil
	c.snapshotCompleted = 0
	c.RefreshContent()
	go func() {
		completed := 0
		for chunkStart := start; !chunkStart.After(end); {
			chunkEnd := chunkStart.AddDate(0, 0, 30)
			if chunkEnd.After(end) {
				chunkEnd = end
			}
			count, rebuildErr := c.service.RebuildHistoricalSnapshots(ctx, chunkStart.Format("2006-01-02"), chunkEnd.Format("2006-01-02"))
			completed += count
			if rebuildErr != nil {
				fyne.Do(func() { c.finishSnapshotWorker(generation, completed, rebuildErr) })
				return
			}
			chunkStart = chunkEnd.AddDate(0, 0, 1)
		}
		fyne.Do(func() { c.finishSnapshotWorker(generation, completed, nil) })
	}()
}

func (c *Controller) finishSnapshotWorker(generation uint64, completed int, err error) bool {
	if generation != c.snapshotGeneration {
		return false
	}
	if c.snapshotCancel != nil {
		c.snapshotCancel()
	}
	c.snapshotCancel = nil
	c.snapshotPending = false
	c.snapshotProgress = ""
	c.snapshotCompleted = completed
	c.snapshotError = err
	c.RefreshContent()
	return true
}

func (c *Controller) cancelSnapshotWorker() {
	c.snapshotGeneration++
	if c.snapshotCancel != nil {
		c.snapshotCancel()
	}
	c.snapshotCancel = nil
	c.snapshotPending = false
	c.snapshotProgress = ""
	c.snapshotError = nil
}

func snapshotFeedback(c *Controller) fyne.CanvasObject {
	if c.snapshotPending {
		return mutedLabel(c.snapshotProgress)
	}
	if c.snapshotError != nil {
		retry := widget.NewButton(c.translator.T("common.retry"), func() {
			c.snapshotError = nil
			c.startSnapshotWorker()
		})
		retry.Importance = widget.LowImportance
		return container.NewVBox(errorPanel(c, c.translator.T("snapshot.historyRebuild"), c.snapshotError), container.NewCenter(retry))
	}
	if c.snapshotCompleted > 0 {
		return mutedLabel(fmt.Sprintf(c.translator.T("snapshot.revisionsUpdated"), c.snapshotCompleted))
	}
	return nil
}
