package builtinai

import (
	"context"
	"sync"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
)

// Pool preserves frozen captures across live turns of one conversation, never
// across conversations, process restarts, configuration revisions or restore.
// Existing fixed capture TTL/budget and session permission checks still apply.
type Pool struct {
	mu         sync.Mutex
	drains     sync.WaitGroup
	app        *application.Service
	directory  string
	generation uint64
	closed     bool
	sessions   map[string]*mcpserver.InProcessSession
	retired    map[string][]*mcpserver.InProcessSession
}

func NewPool(app *application.Service, directory string) *Pool {
	return &Pool{app: app, directory: directory, sessions: map[string]*mcpserver.InProcessSession{}, retired: map[string][]*mcpserver.InProcessSession{}}
}
func (p *Pool) Get(ctx context.Context, id string) (*mcpserver.InProcessSession, error) {
	p.mu.Lock()
	if p.closed || ctx.Err() != nil {
		p.mu.Unlock()
		return nil, assistant.ErrUnavailable
	}
	if s := p.sessions[id]; s != nil {
		p.mu.Unlock()
		return s, nil
	}
	generation := p.generation
	old := p.retired[id]
	delete(p.retired, id)
	p.mu.Unlock()
	// Runtime invokes Get outside its lock and never inside restore's write gate.
	for _, s := range old {
		s.Close()
	}
	if ctx.Err() != nil {
		return nil, assistant.ErrUnavailable
	}
	session, err := mcpserver.NewInProcessSession(ctx, p.app, p.directory, id, mcpserver.ReadOnly, nil)
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	p.mu.Lock()
	if p.closed || p.generation != generation || ctx.Err() != nil || len(p.sessions) >= assistant.MaxConversations {
		p.mu.Unlock()
		session.Close()
		return nil, assistant.ErrStale
	}
	p.sessions[id] = session
	p.mu.Unlock()
	return session, nil
}

// Revoke only fences/cancels. Draining under runtime/restore locks would deadlock
// a worker that is waiting for application authority. Get/Close drain later.
func (p *Pool) Revoke() {
	p.mu.Lock()
	p.generation++
	old := p.sessions
	p.sessions = map[string]*mcpserver.InProcessSession{}
	for id, s := range old {
		p.retired[id] = append(p.retired[id], s)
	}
	p.mu.Unlock()
	for _, s := range old {
		_ = s.RevokeForRestore(context.Background())
	}
}
func (p *Pool) Drop(id string) {
	p.mu.Lock()
	items := p.retired[id]
	delete(p.retired, id)
	if session := p.sessions[id]; session != nil {
		items = append(items, session)
		delete(p.sessions, id)
	}
	if len(items) > 0 {
		p.drains.Add(1)
	}
	p.mu.Unlock()
	if len(items) > 0 {
		for _, session := range items {
			_ = session.RevokeForRestore(context.Background())
		}
		go func() {
			defer p.drains.Done()
			for _, session := range items {
				session.Close()
			}
		}()
	}
}
func (p *Pool) Close() {
	p.Revoke()
	p.mu.Lock()
	p.closed = true
	old := p.retired
	p.retired = map[string][]*mcpserver.InProcessSession{}
	p.mu.Unlock()
	for _, items := range old {
		for _, s := range items {
			s.Close()
		}
	}
	p.drains.Wait()
}
