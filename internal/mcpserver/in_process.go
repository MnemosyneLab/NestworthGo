package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
)

const inProcessInputLimit = 64 << 10
const inProcessOutputLimit = 64 << 10

type inProcessOwner struct {
	app         *application.Service
	householdID string
	sessionID   string
}

var inProcessOwners = struct {
	sync.Mutex
	active map[inProcessOwner]bool
}{active: map[inProcessOwner]bool{}}

// InProcessSession is a prerequisite for the built-in adapter, not an installed
// assistant. It never listens, creates an MCP client or reads an external token.
// It is household-wide: never expose it to scoped/mobile callers. Public Call
// deliberately refuses writers. The desktop runtime alone adapts the separate
// Full and host review APIs after checking explicit session authority, never mobile.
type InProcessSession struct {
	mu           sync.Mutex
	reviewMu     sync.Mutex
	closed       bool
	inflight     sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	service      *Service
	registry     *agentcapability.Registry
	fullRegistry *agentcapability.Registry
	fullOnce     sync.Once
	householdID  string
	owner        inProcessOwner
	release      sync.Once
}

// NewInProcessSession uses a host-owned stable session UUID for receipt storage
// and a fresh process identity for captures. Mode is independent of external MCP
// settings. The same application.Service and WriteCoordinator remain authoritative.
func NewInProcessSession(ctx context.Context, app *application.Service, directory, sessionID, mode string, changed func(), repositories ...settings.ConfigurationRepository) (*InProcessSession, error) {
	if app == nil {
		return nil, fail("database_unavailable", "database unavailable")
	}
	id, err := uuid.Parse(sessionID)
	if err != nil || id == uuid.Nil {
		return nil, fail("validation", "session identity must be a nonzero UUID")
	}
	if mode != ReadOnly && mode != DirectoryWrite && mode != LedgerWrite {
		return nil, fail("validation", "invalid in-process permission")
	}
	bootstrap, err := app.Bootstrap(ctx)
	if err != nil {
		return nil, safeError(err)
	}
	if bootstrap.Household == nil {
		return nil, fail("household_unavailable", "household unavailable")
	}
	householdID := bootstrap.Household.ID.String()
	owner := inProcessOwner{app, householdID, id.String()}
	inProcessOwners.Lock()
	if inProcessOwners.active[owner] {
		inProcessOwners.Unlock()
		return nil, fail("conflict", "in-process session already has a live owner")
	}
	inProcessOwners.active[owner] = true
	inProcessOwners.Unlock()
	privateDirectory := filepath.Join(directory, "in-process", householdID, id.String())
	var scoped []settings.ConfigurationRepository
	if len(repositories) > 0 && repositories[0] != nil {
		scoped = append(scoped, scopedConfiguration{ConfigurationRepository: repositories[0], prefix: "builtin_ai.capability.v1." + householdID + "." + id.String() + "."})
	}
	service := New(app, privateDirectory, changed, scoped...)
	service.builtinSessionID = id.String()
	service.config = Config{Mode: mode, InstanceID: uuid.NewString()}
	service.contextGeneration = service.contexts.activate()
	service.reads.activate(service.contextGeneration)
	// Lifetime is host-owned, not inherited from a single model request.
	lifetime, cancel := context.WithCancel(context.Background())
	return &InProcessSession{ctx: lifetime, cancel: cancel, service: service, registry: service.toolRegistry(mode), householdID: householdID, owner: owner}, nil
}

func modelCallable(d agentcapability.Definition) bool {
	return d.Effect != agentcapability.Write && d.Annotations != nil && d.Annotations.OpenWorldHint != nil && !*d.Annotations.OpenWorldHint
}

func (s *InProcessSession) Definitions() ([]agentcapability.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.service.contexts.active(s.service.contextGeneration) {
		return nil, fail("session_revoked", "in-process session revoked")
	}
	allowed := []agentcapability.Definition{}
	for _, d := range s.registry.Definitions() {
		if modelCallable(d) {
			allowed = append(allowed, d)
		}
	}
	return allowed, nil
}

// Call is the model-facing surface: read/analysis/private-preview only, no
// provider network calls and no approval tool. A preview is never a write grant.
func (s *InProcessSession) Call(ctx context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
	return s.call(ctx, name, arguments, false)
}

// call's trusted branch is used by adapter equivalence tests and the host review
// coordinator. It stays unexported and is never registered as a model tool.
func (s *InProcessSession) call(ctx context.Context, name string, arguments json.RawMessage, trusted bool) (json.RawMessage, error) {
	return s.callObserved(ctx, name, arguments, trusted, nil, s.registry)
}
func (s *InProcessSession) callObserved(ctx context.Context, name string, arguments json.RawMessage, trusted bool, dispatched *bool, registry *agentcapability.Registry) (json.RawMessage, error) {
	s.mu.Lock()
	if s.closed || !s.service.contexts.active(s.service.contextGeneration) {
		s.mu.Unlock()
		return nil, fail("session_revoked", "in-process session revoked")
	}
	s.inflight.Add(1)
	callContext, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	s.mu.Unlock()
	defer s.inflight.Done()
	defer cancel()
	defer stop()
	if err := callContext.Err(); err != nil {
		return nil, fail("cancelled", "request cancelled before capability dispatch")
	}
	d, err := registry.Definition(name)
	if err != nil {
		return nil, fail("not_found", "capability unavailable for this permission")
	}
	if !trusted && !modelCallable(d) {
		return nil, fail("approval_required", "capability requires a host review that is not available")
	}
	if len(arguments) > inProcessInputLimit {
		return nil, fail("too_large", "capability arguments exceed the in-process budget")
	}
	bootstrap, err := s.service.app.Bootstrap(callContext)
	if err != nil {
		if callContext.Err() != nil {
			return nil, fail("cancelled", "request cancelled before capability dispatch")
		}
		return nil, safeError(err)
	}
	if bootstrap.Household == nil || bootstrap.Household.ID.String() != s.householdID {
		s.RevokeForRestore(callContext)
		return nil, fail("session_revoked", "household changed; start a new session")
	}
	if err := callContext.Err(); err != nil {
		return nil, fail("cancelled", "request cancelled before capability dispatch")
	}
	if dispatched != nil {
		*dispatched = true
	}
	result, err := registry.Call(callContext, name, arguments)
	if err != nil {
		var unsubmitted unsubmittedCapability
		if errors.As(err, &unsubmitted) {
			return nil, unsubmitted
		}
		if callContext.Err() != nil {
			if d.Effect == agentcapability.Write {
				return nil, fail("operation_outcome_unknown", "request cancelled during a write; inspect the original operation receipt")
			}
			return nil, fail("cancelled", "request cancelled")
		}
		if err == agentcapability.ErrInvalidArguments {
			return nil, fail("validation", "invalid capability arguments")
		}
		return nil, safeError(err)
	}
	raw, err := jsoncodec.Marshal(result)
	if err != nil {
		return nil, fail("internal", "capability result unavailable")
	}
	if len(raw) > inProcessOutputLimit {
		if d.Effect == agentcapability.Write {
			return nil, fail("operation_outcome_unknown", "write result exceeds the display budget; inspect the original operation receipt")
		}
		return nil, fail("too_large", "capability result exceeds the in-process budget; narrow or page the read")
	}
	// A known write receipt remains a success after request cancellation. The
	// owner still must reconcile its display/run state; cancellation is no rollback.
	// Revocation, unlike request cancellation, forbids delivery to that principal.
	if s.ctx.Err() != nil {
		if d.Effect == agentcapability.Write {
			return nil, fail("operation_outcome_unknown", "session revoked during a write; inspect the original operation receipt")
		}
		return nil, fail("session_revoked", "in-process session revoked")
	}
	if callContext.Err() != nil && d.Effect != agentcapability.Write {
		return nil, fail("cancelled", "request cancelled")
	}
	return raw, nil
}

// RevokeForRestore fences/cancels without draining: restore already owns the
// application's write permit. Close may drain only outside that permit.
func (s *InProcessSession) RevokeForRestore(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	return s.service.RevokeForRestore(ctx)
}

func (s *InProcessSession) Close() {
	_ = s.RevokeForRestore(context.Background())
	s.inflight.Wait()
	s.service.Close()
	s.release.Do(func() {
		inProcessOwners.Lock()
		delete(inProcessOwners.active, s.owner)
		inProcessOwners.Unlock()
	})
}

// The existing storage adapter continues using its exact legacy MCP keys. A
// scoped repository maps them into a disjoint built-in namespace, including
// plans and receipts. External token/permission state cannot be read or changed.
type scopedConfiguration struct {
	settings.ConfigurationRepository
	prefix string
}

func (r scopedConfiguration) scoped(key string) string {
	return r.prefix + strings.TrimPrefix(key, "mcp.")
}

func (r scopedConfiguration) LoadConfiguration(key string) ([]byte, bool, error) {
	return r.ConfigurationRepository.LoadConfiguration(r.scoped(key))
}

func (r scopedConfiguration) SaveConfiguration(key string, value []byte) error {
	return r.ConfigurationRepository.SaveConfiguration(r.scoped(key), value)
}

func (r scopedConfiguration) ListConfiguration(prefix string) (map[string][]byte, error) {
	values, err := r.ConfigurationRepository.ListConfiguration(r.scoped(prefix))
	if err != nil {
		return nil, err
	}
	results := make(map[string][]byte, len(values))
	for key, value := range values {
		if strings.HasPrefix(key, r.scoped(prefix)) {
			results["mcp."+strings.TrimPrefix(key, r.prefix)] = value
		}
	}
	return results, nil
}
