package domain

import (
	"context"
	"sync"
)

type boundedReadKey struct{}
type readBudget struct {
	mu          sync.Mutex
	rows, bytes int
	seen        map[string]bool
}

// WithBoundedRead requests a shared SQL preflight budget for the new opt-in
// protocol. Legacy reads retain established behavior and response shapes.
func WithBoundedRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, boundedReadKey{}, &readBudget{seen: map[string]bool{}})
}
func IsBoundedRead(ctx context.Context) bool {
	_, ok := ctx.Value(boundedReadKey{}).(*readBudget)
	return ok
}

// ReserveReadBudget is called before SQL row expansion, once per scoped query.
// The opaque internal key is memory-only and is never reported or logged.
func ReserveReadBudget(ctx context.Context, key string, rows, bytes int) error {
	budget, ok := ctx.Value(boundedReadKey{}).(*readBudget)
	if !ok {
		return nil
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.seen[key] {
		return nil
	}
	if rows < 0 || bytes < 0 || budget.rows+rows > 20000 || budget.bytes+bytes > 4<<20 {
		return &Error{Code: "too_large", Message: "capture exceeds SQL read budget; narrow targets or filters"}
	}
	budget.seen[key] = true
	budget.rows += rows
	budget.bytes += bytes
	return nil
}
