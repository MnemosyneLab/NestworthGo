package ui

import (
	"context"
)

// asyncTask tracks one cancellable background job keyed by a monotonically
// increasing generation. A late completion from an abandoned generation is
// rejected instead of mutating the page state.
//
// All methods must be called from the UI thread (finish from inside fyne.Do);
// only the returned ctx travels to the worker goroutine.
type asyncTask[T any] struct {
	cancel     context.CancelFunc
	generation uint64
	pending    bool
	progress   string
	payload    *T
	err        error
}

// begin invalidates any previous job, starts the next generation, and returns
// its token plus the cancellable context for the new job. Previous results are
// cleared so stale payloads never outlive the job that produced them.
func (t *asyncTask[T]) begin(progress string) (uint64, context.Context) {
	t.cancelPrevious()
	t.generation++
	t.pending = true
	t.progress = progress
	t.payload = nil
	t.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	return t.generation, ctx
}

// finish records completion of the job identified by generation. It reports
// whether the result was accepted; callers use this to skip UI work for stale
// completions.
func (t *asyncTask[T]) finish(generation uint64, payload T, err error) bool {
	if generation != t.generation {
		return false
	}
	t.cancelPrevious()
	t.cancel = nil
	t.pending = false
	t.progress = ""
	t.err = err
	if err != nil {
		t.payload = nil
	} else {
		t.payload = &payload
	}
	return true
}

// stop abandons the current job: the generation advances before the context is
// cancelled so a completion racing a stop is rejected by finish.
func (t *asyncTask[T]) stop() {
	t.generation++
	t.cancelPrevious()
	t.cancel = nil
	t.pending = false
	t.progress = ""
}

func (t *asyncTask[T]) cancelPrevious() {
	if t.cancel != nil {
		t.cancel()
	}
}
