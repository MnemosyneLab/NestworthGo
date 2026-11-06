package assistant

import (
	"context"
	"sync"
)

type runWriteFenceKey struct{}
type runWriteFence struct {
	mu        sync.Mutex
	uncertain bool
}

// Every write channel in a model run shares this gate. Acquire before Runtime.mu,
// and hold through outcome classification; inspection reads do not acquire it.
func acquireRunWrite(ctx context.Context) (func(bool), error) {
	fence, _ := ctx.Value(runWriteFenceKey{}).(*runWriteFence)
	if fence == nil {
		return func(bool) {}, nil
	}
	fence.mu.Lock()
	if fence.uncertain {
		fence.mu.Unlock()
		return nil, ErrRecoveryRequired
	}
	return func(uncertain bool) {
		fence.uncertain = fence.uncertain || uncertain
		fence.mu.Unlock()
	}, nil
}
