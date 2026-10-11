// Package preview retains immutable desktop confirmation plans. Nothing is
// persisted by preview; a restart requires a new preview. Successful ledger
// confirmations also have durable application mutation receipts.
package preview

import (
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"sync"
	"time"
)

type entry[T any] struct {
	value   T
	created time.Time
}
type Store[T any] struct {
	mu    sync.Mutex
	plans map[string]entry[T]
}

func (s *Store[T]) Put(value T) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plans == nil {
		s.plans = make(map[string]entry[T])
	}
	// Bound memory without discarding recently confirmed retry plans.
	for id, item := range s.plans {
		if time.Since(item.created) > 24*time.Hour {
			delete(s.plans, id)
		}
	}
	if len(s.plans) >= 1024 {
		var oldest string
		var at time.Time
		for id, item := range s.plans {
			if oldest == "" || item.created.Before(at) {
				oldest, at = id, item.created
			}
		}
		delete(s.plans, oldest)
	}
	id := uuid.NewString()
	s.plans[id] = entry[T]{value, time.Now()}
	return id
}
func (s *Store[T]) Get(id string) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, ok := s.plans[id]; ok {
		return item.value, nil
	}
	var zero T
	return zero, &domain.Error{Code: domain.ErrStalePreview, Message: "request a fresh preview"}
}
