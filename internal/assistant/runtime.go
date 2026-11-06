package assistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const (
	StateVersion     = 1
	MaxStateBytes    = 16 << 20
	MaxHistoryBytes  = 1 << 20
	MaxConversations = 20
	MaxRuns          = 64 // per conversation; deleting a conversation never reverses a ledger operation
	MaxPromptBytes   = 16 << 10
	MaxTextBytes     = 64 << 10
	MaxEvents        = 256
)

// Errors are fixed codes. Never return implementation/provider/credential errors
// to IPC, logs, events or persistence.
var (
	ErrUnavailable = errors.New("assistant_unavailable")
	ErrStale       = errors.New("assistant_stale_binding")
	ErrConflict    = errors.New("assistant_request_conflict")
	ErrBudget      = errors.New("assistant_budget_exceeded")
	ErrStorage     = errors.New("assistant_storage_unavailable")
)

// Credentials is a host-owned credential port, backed by the live database.
// With supplies a short-lived lease; production callers never return its value.
type Credentials interface {
	Available() bool
	Put(context.Context, []byte) error
	With(context.Context, func([]byte) error) error
	Delete(context.Context) error
}

type Persistence interface {
	Load() (State, error)
	Save(State) error // atomic and durable before returning success
	Close() error
}

// Engine preserves complete provider-native history, separate from display text.
// A failed round must not return a usable history. It exposes no approval API.
type Engine interface {
	Complete(context.Context, []byte, string) (Completion, error)
	Close()
}
type Completion struct {
	Navigation *ModelNavigationTarget
	Evidence   []ReadEvidence
	History    []byte
	Text       string
}
type EngineFactory func(context.Context, ModelConfig, []byte, string) (Engine, error)

type ModelConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Enabled  bool   `json:"enabled"`
}

func (c ModelConfig) Valid() bool {
	return c.Provider == "openai-responses" && modelName.MatchString(c.Model) && !strings.HasPrefix(c.Model, "sk-") && (!c.Enabled || c.Model != "unconfigured")
}

var modelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,95}$`)

// State is private persistence, never an IPC result, backup or operation receipt.
// It contains neither credentials nor host approval authority.
type State struct {
	Version       int                 `json:"version"`
	Ledger        string              `json:"ledger"`
	Config        ModelConfig         `json:"config"`
	Revision      uint64              `json:"revision"`
	Conversations []ConversationState `json:"conversations"`
}
type ConversationState struct {
	Reviews        []ReviewRef `json:"reviews,omitempty"`
	Title          string      `json:"title,omitempty"`
	TitleSource    string      `json:"titleSource,omitempty"`
	TitleAttempted bool        `json:"titleAttempted,omitempty"`
	ID             string      `json:"id"`
	Updated        time.Time   `json:"updated"`
	History        []byte      `json:"history,omitempty"`
	HistoryDigest  string      `json:"historyDigest,omitempty"`
	Runs           []RunState  `json:"runs"`
	Events         []Event     `json:"events"`
	Sequence       uint64      `json:"sequence"`
}
type RunState struct {
	Run    Run    `json:"run"`
	Digest string `json:"digest"`
}
type Run struct {
	Tools          []ToolProgress `json:"tools,omitempty"`
	PageLeaseID    string         `json:"pageLeaseId,omitempty"`
	Evidence       []ReadEvidence `json:"evidence,omitempty"`
	Page           PageSelection  `json:"page"`
	ID             string         `json:"id"`
	RequestID      string         `json:"requestId"`
	ConversationID string         `json:"conversationId"`
	State          string         `json:"state"`
	Code           string         `json:"code,omitempty"`
	Prompt         string         `json:"prompt"`
	Text           string         `json:"text,omitempty"`
	Created        time.Time      `json:"created"`
	Finished       *time.Time     `json:"finished,omitempty"`
	Provider       string         `json:"provider"`
	Model          string         `json:"model"`
	ConfigRevision uint64         `json:"configRevision"`
}
type Event struct {
	Sequence uint64 `json:"sequence"`
	RunID    string `json:"runId"`
	State    string `json:"state"`
	Code     string `json:"code,omitempty"`
}
type Snapshot struct {
	Version     int         `json:"version"`
	Epoch       string      `json:"epoch"`
	Revision    uint64      `json:"revision"`
	Config      ModelConfig `json:"config"`
	Available   bool        `json:"available"`
	Credentials string      `json:"credentials"`
	FullAccess  bool        `json:"fullAccess"`
	Reason      string      `json:"reason"`
}
type Conversation struct {
	Title       string                 `json:"title,omitempty"`
	TitleSource string                 `json:"titleSource,omitempty"`
	Progress    *Progress              `json:"progress,omitempty"`
	Navigation  *ModelNavigationIntent `json:"navigation,omitempty"`
	ID          string                 `json:"id"`
	Updated     time.Time              `json:"updated"`
	Runs        []Run                  `json:"runs"`
	RunCount    int                    `json:"runCount"`
	Offset      int                    `json:"offset"`
}
type EventPage struct {
	Events []Event `json:"events"`
	Latest uint64  `json:"latest"`
	Reset  bool    `json:"reset"` // fetch Conversation if journal rolled over
}
type StartRequest struct {
	PageLeaseID    string        `json:"pageLeaseId,omitempty"`
	Page           PageSelection `json:"page"`
	Epoch          string        `json:"epoch"`
	Revision       uint64        `json:"revision"`
	ConversationID string        `json:"conversationId"`
	RequestID      string        `json:"requestId"`
	Prompt         string        `json:"prompt"`
}

type CapabilityLifecycle interface {
	Revoke()
	Drop(string)
	Close()
}

type Options struct {
	Transcriber         Transcriber
	Reviews             ReviewProvider
	TitleGenerator      TitleGenerator
	TitleTimeout        time.Duration
	NavigationValidator func(context.Context, ModelNavigationTarget) bool
	NavigationClock     func() time.Time
	Capabilities        CapabilityLifecycle
	Ledger              string // host-derived database path + household binding; never UI-supplied
	Store               Persistence
	Credentials         Credentials
	Factory             EngineFactory
	Validate            func(context.Context) bool // recheck live host/database authority
	Timeout             time.Duration
}
type Runtime struct {
	fullEnabled          bool
	reviewBusy           map[string]bool
	titles               map[string]context.CancelFunc
	progress             map[string]*Progress
	pageReads            map[string]pendingPageRead
	pageOperations       map[string]pendingPageOperation
	navigationGeneration uint64
	pageLease            *DesktopPageLease
	navigationIntents    map[string]ModelNavigationIntent
	mu                   sync.Mutex
	state                State
	epoch                string
	options              Options
	ctx                  context.Context
	cancel               context.CancelFunc
	active               map[string]context.CancelFunc
	wg                   sync.WaitGroup
	closed               bool
	closeOnce            sync.Once
	closeErr             error
}

func NewRuntime(o Options) (*Runtime, error) {
	if o.Store == nil || o.Credentials == nil || o.Factory == nil || o.Validate == nil || o.Ledger == "" {
		return nil, ErrUnavailable
	}
	if o.TitleTimeout == 0 {
		o.TitleTimeout = 10 * time.Second
	}
	if o.TitleTimeout < 0 || o.TitleTimeout > 10*time.Second {
		return nil, ErrBudget
	}
	if o.Timeout == 0 {
		o.Timeout = 120 * time.Second
	}
	if o.Timeout <= 0 || o.Timeout > 120*time.Second {
		return nil, ErrBudget
	}
	state, err := o.Store.Load()
	if err != nil {
		return nil, ErrStorage
	}
	if state.Version != StateVersion || state.Ledger != o.Ledger {
		return nil, ErrStale
	}
	if ValidateState(state) != nil {
		return nil, ErrStorage
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{reviewBusy: map[string]bool{}, state: state, options: o, epoch: uuid.NewString(), ctx: ctx, cancel: cancel, active: map[string]context.CancelFunc{}}
	now := time.Now().UTC()
	kept := r.state.Conversations[:0]
	for _, c := range r.state.Conversations {
		if c.Updated.After(now.Add(-30*24*time.Hour)) || r.pendingReview(&c) {
			kept = append(kept, c)
		}
	}
	r.state.Conversations = kept
	for i := range r.state.Conversations {
		c := &r.state.Conversations[i]
		// Existing untitled history receives a local title without a new provider request.
		if c.Title == "" && len(c.Runs) > 0 {
			c.Title = localTitle(c.Runs[0].Run.Prompt)
			c.TitleSource = "local"
			c.TitleAttempted = true
		}
		if c.TitleSource == "pending" {
			c.TitleSource = "local"
		}
		for j := range c.Runs {
			run := &c.Runs[j].Run
			if !terminal(run.State) {
				run.State = "interrupted"
				run.Code = "restart_requires_new_request"
				run.Finished = &now
				r.event(c, *run)
			}
		}
	}
	// Do not resume provider requests or tool calls at startup.
	if err := r.persist(); err != nil {
		cancel()
		return nil, err
	}
	return r, nil
}
func InitialState(ledger string) State {
	return State{Version: StateVersion, Ledger: ledger, Revision: 1, Config: ModelConfig{Provider: "openai-responses", Model: "unconfigured", Enabled: false}, Conversations: []ConversationState{}}
}
func terminal(s string) bool {
	return s == "succeeded" || s == "failed" || s == "cancelled" || s == "interrupted"
}
func canonicalID(s string) bool {
	id, e := uuid.Parse(s)
	return e == nil && id != uuid.Nil && id.String() == s
}
func (r *Runtime) valid(ctx context.Context, epoch string) error {
	if r.closed || r.ctx.Err() != nil || epoch != r.epoch || !r.options.Validate(ctx) {
		return ErrStale
	}
	return nil
}
func (r *Runtime) persist() error {
	raw, err := jsoncodec.Marshal(r.state)
	if err != nil || len(raw) > MaxStateBytes {
		r.closed = true
		r.cancel()
		for _, cancel := range r.active {
			cancel()
		}
		return ErrBudget
	}
	if r.options.Store.Save(r.state) != nil {
		r.closed = true
		r.cancel()
		for _, cancel := range r.active {
			cancel()
		}
		return ErrStorage
	}
	return nil
}
func (r *Runtime) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	live := !r.closed && r.ctx.Err() == nil && r.options.Validate(context.Background())
	credentialStatus, reason := "unavailable", "secure_runtime_unavailable"
	if live && r.options.Credentials.Available() {
		credentialStatus = "missing"
		reason = "credentials_unavailable"
		if r.options.Credentials.With(context.Background(), func(key []byte) error {
			if !validSecret(key) {
				return ErrUnavailable
			}
			return nil
		}) == nil {
			credentialStatus = "present"
			reason = "disabled"
			if r.state.Config.Enabled {
				reason = "ready"
			}
		}
	}
	return Snapshot{Version: 1, Epoch: r.epoch, Revision: r.state.Revision, Config: r.state.Config, FullAccess: live && r.fullEnabled && r.state.Config.Enabled, Available: reason == "ready", Credentials: credentialStatus, Reason: reason}
}
func (r *Runtime) Configure(ctx context.Context, epoch string, revision uint64, cfg ModelConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return e
	}
	if revision != r.state.Revision {
		return ErrStale
	}
	if !cfg.Valid() {
		return ErrUnavailable
	}
	if len(r.active) != 0 {
		if cfg.Enabled {
			return ErrConflict
		}
		for _, cancel := range r.active {
			cancel()
		}
	}
	if r.options.Credentials.Available() {
		tainted := false
		_ = r.options.Credentials.With(ctx, func(key []byte) error {
			if validSecret(key) && strings.Contains(cfg.Model, string(key)) {
				tainted = true
			}
			return nil
		})
		if tainted {
			return ErrUnavailable
		}
	}
	if cfg.Enabled {
		if !r.options.Credentials.Available() {
			return ErrUnavailable
		}
		if r.options.Credentials.With(ctx, func(key []byte) error {
			if !validSecret(key) {
				return ErrUnavailable
			}
			return nil
		}) != nil {
			return ErrUnavailable
		}
	}
	// Native provider history is bound to its provider/model. Keep display history
	// but start a fresh model context after a model change.
	if cfg.Provider != r.state.Config.Provider || cfg.Model != r.state.Config.Model {
		for i := range r.state.Conversations {
			r.state.Conversations[i].History = nil
			r.state.Conversations[i].HistoryDigest = ""
		}
	}
	if r.options.Capabilities != nil {
		r.options.Capabilities.Revoke()
	}
	r.fullEnabled = false
	r.state.Config = cfg
	r.clearNavigation()
	r.cancelTitles()
	r.state.Revision++
	return r.persist()
}
func validSecret(key []byte) bool {
	if len(key) < 8 || len(key) > 4096 {
		return false
	}
	for _, b := range key {
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.') {
			return false
		}
	}
	return true
}

// SetCredential accepts explicit desktop settings input. Mobile has no binding.
func (r *Runtime) SetCredential(ctx context.Context, epoch string, key []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return e
	}
	if len(r.active) != 0 {
		return ErrConflict
	}
	if !validSecret(key) || !r.options.Credentials.Available() {
		return ErrUnavailable
	}
	raw, encodeErr := jsoncodec.Marshal(r.state)
	if encodeErr != nil || strings.Contains(string(raw), string(key)) {
		return ErrUnavailable
	}
	for _, c := range r.state.Conversations {
		if strings.Contains(string(c.History), string(key)) {
			return ErrUnavailable
		}
	}
	if r.options.Credentials.Put(ctx, key) != nil {
		return ErrUnavailable
	}
	r.fullEnabled = false
	if r.options.Capabilities != nil {
		r.options.Capabilities.Revoke()
	}
	r.clearNavigation()
	r.cancelTitles()
	r.state.Revision++
	return r.persist()
}
func (r *Runtime) DeleteCredential(ctx context.Context, epoch string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return e
	}
	for _, cancel := range r.active {
		cancel()
	}
	if r.options.Capabilities != nil {
		r.options.Capabilities.Revoke()
	}
	r.fullEnabled = false
	r.state.Config.Enabled = false
	r.clearNavigation()
	r.cancelTitles()
	r.state.Revision++
	if err := r.persist(); err != nil {
		return err
	}
	if r.options.Credentials.Delete(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
func (r *Runtime) Create(ctx context.Context, epoch string) (Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return Conversation{}, e
	}
	r.clearNavigation()
	// Retention never prunes active conversations or creates new dedup authority.
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	kept := r.state.Conversations[:0]
	for _, c := range r.state.Conversations {
		if c.Updated.After(cutoff) || r.active[c.ID] != nil || r.reviewBusy[c.ID] || r.pendingReview(&c) {
			kept = append(kept, c)
		} else if r.options.Capabilities != nil {
			r.options.Capabilities.Drop(c.ID)
		}
	}
	r.state.Conversations = kept
	if len(kept) >= MaxConversations {
		return Conversation{}, ErrBudget
	}
	c := ConversationState{ID: uuid.NewString(), Updated: time.Now().UTC(), Runs: []RunState{}, Events: []Event{}}
	r.state.Conversations = append(r.state.Conversations, c)
	if e := r.persist(); e != nil {
		return Conversation{}, e
	}
	return view(c), nil
}
func (r *Runtime) find(id string) *ConversationState {
	for i := range r.state.Conversations {
		if r.state.Conversations[i].ID == id {
			return &r.state.Conversations[i]
		}
	}
	return nil
}

// Public history is paginated one attempted turn at a time (<=80 KiB before
// JSON escaping). The latest turn is returned by Inspect after reconnect.
func view(c ConversationState) Conversation { return page(c, len(c.Runs)-1) }
func page(c ConversationState, offset int) Conversation {
	v := Conversation{Title: c.Title, TitleSource: c.TitleSource, ID: c.ID, Updated: c.Updated, Runs: []Run{}, RunCount: len(c.Runs), Offset: offset}
	if offset >= 0 && offset < len(c.Runs) {
		run := c.Runs[offset].Run
		run.Evidence = append([]ReadEvidence(nil), run.Evidence...)
		run.Tools = append([]ToolProgress(nil), run.Tools...)
		if run.Finished != nil {
			finished := *run.Finished
			run.Finished = &finished
		}
		v.Runs = append(v.Runs, run)
	}
	return v
}
func (r *Runtime) Messages(ctx context.Context, epoch, id string, offset int) (Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(ctx, epoch); err != nil {
		return Conversation{}, err
	}
	c := r.find(id)
	if c == nil || offset < 0 || offset >= len(c.Runs) {
		return Conversation{}, ErrStale
	}
	return page(*c, offset), nil
}

func (r *Runtime) List(ctx context.Context, epoch string) ([]Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return nil, e
	}
	out := make([]Conversation, 0, len(r.state.Conversations))
	for _, c := range r.state.Conversations {
		out = append(out, Conversation{Title: c.Title, TitleSource: c.TitleSource, ID: c.ID, Updated: c.Updated, Runs: []Run{}, RunCount: len(c.Runs)})
	}
	return out, nil
}
func (r *Runtime) Inspect(ctx context.Context, epoch, id string) (Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return Conversation{}, e
	}
	c := r.find(id)
	if c == nil {
		return Conversation{}, ErrStale
	}
	v := view(*c)
	r.navigationView(&v)
	r.progressView(&v)
	return v, nil
}
func (r *Runtime) Events(ctx context.Context, epoch, id string, after uint64) (EventPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return EventPage{}, e
	}
	c := r.find(id)
	if c == nil {
		return EventPage{}, ErrStale
	}
	out := EventPage{Latest: c.Sequence, Events: []Event{}}
	if after > c.Sequence || (len(c.Events) > 0 && after < c.Events[0].Sequence-1) {
		out.Reset = true
		return out, nil
	}
	for _, e := range c.Events {
		if e.Sequence > after {
			out.Events = append(out.Events, e)
			if len(out.Events) == 64 {
				break
			}
		}
	}
	return out, nil
}
func (r *Runtime) Delete(ctx context.Context, epoch, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return e
	}
	if r.active[id] != nil || r.reviewBusy[id] || (r.find(id) != nil && r.pendingReview(r.find(id))) {
		return ErrConflict
	}
	for i, c := range r.state.Conversations {
		if c.ID == id {
			r.clearNavigation()
			if r.options.Capabilities != nil {
				r.options.Capabilities.Drop(id)
			}
			r.cancelTitle(id)
			delete(r.progress, id)
			r.state.Conversations = append(r.state.Conversations[:i], r.state.Conversations[i+1:]...)
			return r.persist()
		}
	}
	return ErrStale
}
func (r *Runtime) Start(ctx context.Context, q StartRequest) (Run, error) {
	if (q.PageLeaseID != "" && !canonicalID(q.PageLeaseID)) || !q.Page.Valid() || !canonicalID(q.RequestID) || !canonicalID(q.ConversationID) || len(q.Prompt) == 0 || len(q.Prompt) > MaxPromptBytes || strings.TrimSpace(q.Prompt) == "" {
		return Run{}, ErrBudget
	}
	digest := promptDigest(q.Prompt, q.Page, q.PageLeaseID)
	hash := hex.EncodeToString(digest[:])
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, q.Epoch); e != nil {
		return Run{}, e
	}
	c := r.find(q.ConversationID)
	if c == nil {
		return Run{}, ErrStale
	}
	for _, saved := range c.Runs {
		if saved.Run.RequestID == q.RequestID {
			if saved.Digest != hash {
				return Run{}, ErrConflict
			}
			out := saved.Run
			out.Evidence = append([]ReadEvidence(nil), out.Evidence...)
			out.Tools = append([]ToolProgress(nil), out.Tools...)
			if out.Finished != nil {
				finished := *out.Finished
				out.Finished = &finished
			}
			return out, nil
		}
	}
	if q.Revision != r.state.Revision {
		return Run{}, ErrStale
	}
	if r.active[c.ID] != nil || r.reviewBusy[c.ID] {
		return Run{}, ErrConflict
	}
	if len(r.active) >= 2 || len(c.Runs) >= MaxRuns {
		return Run{}, ErrBudget
	}
	if !r.state.Config.Enabled || !r.options.Credentials.Available() {
		return Run{}, ErrUnavailable
	}
	// Validate secret taint before persisting user text. Credential-store failures
	// cannot leak their implementation errors or cause a provider request.
	if e := r.options.Credentials.With(ctx, func(key []byte) error {
		if !validSecret(key) || strings.Contains(q.Prompt, string(key)) || strings.Contains(q.Page.AccountID, string(key)) || strings.Contains(q.PageLeaseID, string(key)) {
			return ErrUnavailable
		}
		return nil
	}); e != nil {
		return Run{}, ErrUnavailable
	}
	grant, grantErr := r.grantForStart(q)
	if grantErr != nil {
		return Run{}, grantErr
	}
	r.navigationGeneration++
	r.navigationIntents = nil
	if grant.requestID == "" {
		r.clearNavigation()
	} else {
		grant.generation = r.navigationGeneration
	}
	run := Run{PageLeaseID: q.PageLeaseID, Page: q.Page, ID: uuid.NewString(), RequestID: q.RequestID, ConversationID: c.ID, State: "accepted", Prompt: q.Prompt, Created: time.Now().UTC(), Provider: r.state.Config.Provider, Model: r.state.Config.Model, ConfigRevision: r.state.Revision}
	c.Runs = append(c.Runs, RunState{Run: run, Digest: hash})
	c.Updated = run.Created
	if c.Title == "" {
		c.Title = localTitle(q.Prompt)
		c.TitleSource = "local"
	}
	r.event(c, run)
	if e := r.persist(); e != nil {
		return Run{}, e
	}
	runCtx, cancel := context.WithTimeout(context.WithValue(r.ctx, pageSelectionKey{}, q.Page), r.options.Timeout)
	runCtx = context.WithValue(runCtx, runWriteFenceKey{}, &runWriteFence{})
	if r.fullEnabled && r.options.Reviews != nil {
		runCtx = context.WithValue(runCtx, fullKey{}, ToolHandler(func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
			return r.executeFull(context.WithValue(ctx, runWriteFenceKey{}, runCtx.Value(runWriteFenceKey{})), q.Epoch, c.ID, run.ID, q.Revision, raw)
		}))
	}
	runCtx = context.WithValue(runCtx, pageContentKey{}, r.pageReader(runCtx, q.Revision))
	runCtx = context.WithValue(runCtx, pageOperatorKey{}, r.pageOperator(runCtx, q.Epoch, q.Revision))
	if grant.requestID != "" {
		runCtx = context.WithValue(runCtx, modelNavigationGrantKey{}, grant)
	}
	r.active[c.ID] = cancel
	history := append([]byte(nil), c.History...)
	cfg := r.state.Config
	r.wg.Add(1)
	go r.execute(runCtx, cancel, c.ID, run.ID, cfg, history, q.Prompt)
	return run, nil
}
func (r *Runtime) event(c *ConversationState, run Run) {
	c.Sequence++
	c.Events = append(c.Events, Event{Sequence: c.Sequence, RunID: run.ID, State: run.State, Code: run.Code})
	if len(c.Events) > MaxEvents {
		c.Events = append([]Event(nil), c.Events[len(c.Events)-MaxEvents:]...)
	}
}
func (r *Runtime) execute(ctx context.Context, cancel context.CancelFunc, id, runID string, cfg ModelConfig, history []byte, prompt string) {
	defer r.wg.Done()
	defer cancel()
	r.mu.Lock()
	c := r.find(id)
	if c == nil {
		delete(r.active, id)
		r.mu.Unlock()
		return
	}
	run := &c.Runs[len(c.Runs)-1].Run
	if ctx.Err() != nil || r.valid(ctx, r.epoch) != nil {
		r.finish(id, runID, Completion{}, "cancelled", "authority_revoked")
		delete(r.active, id)
		r.mu.Unlock()
		return
	}
	run.State = "running"
	r.event(c, *run)
	err := r.persist()
	r.mu.Unlock()
	result := Completion{}
	if err == nil {
		result, err = r.invoke(ctx, cfg, id, runID, history, prompt)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	defer delete(r.active, id)
	state, code := "succeeded", ""
	if err != nil {
		state = "failed"
		code = FailureCode(err)
	}
	if ctx.Err() != nil {
		state = "cancelled"
		code = "cancelled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			state = "failed"
			code = "timeout"
		}
	}
	if r.valid(context.Background(), r.epoch) != nil || !r.state.Config.Enabled || r.state.Config.Provider != cfg.Provider || r.state.Config.Model != cfg.Model {
		state = "interrupted"
		code = "authority_revoked"
	}
	r.finish(id, runID, result, state, code)
	if state == "succeeded" {
		r.offerNavigation(ctx, id, runID, result)
		r.startTitle(id, cfg, prompt)
	}
}

// Caller owns mu. Only successful complete runs advance recoverable history.
func (r *Runtime) finish(id, runID string, result Completion, state, code string) {
	c := r.find(id)
	if c == nil {
		return
	}
	for i := range c.Runs {
		run := &c.Runs[i].Run
		if run.ID == runID {
			now := time.Now().UTC()
			if p := r.progress[id]; p != nil && p.RunID == runID {
				p.Text = ""
				for j := range p.Tools {
					if p.Tools[j].State == "running" {
						p.Tools[j].State = "interrupted"
					}
				}
				p.Revision++
				run.Tools = append([]ToolProgress(nil), p.Tools...)
				delete(r.progress, id)
			}
			run.State = state
			run.Code = code
			run.Finished = &now
			c.Updated = now
			if state == "succeeded" {
				c.History = append([]byte(nil), result.History...)
				c.HistoryDigest = historyDigest(r.state.Ledger, c.ID, c.History)
				run.Text = result.Text
				run.Evidence = append([]ReadEvidence(nil), result.Evidence...)
			}
			r.event(c, *run)
			_ = r.persist()
			return
		}
	}
}
func (r *Runtime) Cancel(ctx context.Context, epoch, id, runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.valid(ctx, epoch); e != nil {
		return e
	}
	c := r.find(id)
	if c == nil {
		return ErrStale
	}
	for _, saved := range c.Runs {
		if saved.Run.ID == runID {
			r.clearNavigation()
			if !terminal(saved.Run.State) {
				if cancel := r.active[id]; cancel != nil {
					cancel()
				}
			}
			return nil
		}
	}
	return ErrStale
}

// Revoke is nonblocking: restore may own the application's exclusive permit.
// The store is independent of the ledger; Close drains only after that permit.
func (r *Runtime) Revoke() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.cancel()
	r.clearNavigation()
	if r.options.Capabilities != nil {
		r.options.Capabilities.Revoke()
	}
	for _, cancel := range r.active {
		cancel()
	}
}
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		r.Revoke()
		r.wg.Wait()
		if r.options.Capabilities != nil {
			r.options.Capabilities.Close()
		}
		r.closeErr = r.options.Store.Close()
	})
	return r.closeErr
}

// FenceForRestore purges this ledger's conversation namespace before replacement.
// It never waits for a model/tool worker while restore owns the exclusive permit.
// A failure blocks restore; normal shutdown uses Revoke and preserves history.
func (r *Runtime) FenceForRestore() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.cancel()
	r.clearNavigation()
	if r.options.Capabilities != nil {
		r.options.Capabilities.Revoke()
	}
	for _, cancel := range r.active {
		cancel()
	}
	for i := range r.state.Conversations {
		clear(r.state.Conversations[i].History)
	}
	r.state.Conversations = []ConversationState{}
	r.fullEnabled = false
	r.state.Config.Enabled = false
	r.clearNavigation()
	r.cancelTitles()
	r.state.Revision++
	return r.persist()
}

// Treat provider/adapter panics like other untrusted failures. The panic value
// can contain a response body or key, so it is neither formatted nor logged.
func (r *Runtime) invoke(ctx context.Context, cfg ModelConfig, id, runID string, history []byte, prompt string) (result Completion, err error) {
	defer func() {
		if recover() != nil {
			result = Completion{}
			err = ErrUnavailable
		}
	}()
	// The database handle may close after the restore fence. Acquire the key
	// under the runtime lock, but never keep that lock during provider work.
	var key []byte
	defer func() { clear(key) }()
	err = func() error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.valid(ctx, r.epoch) != nil {
			return ErrStale
		}
		return r.options.Credentials.With(ctx, func(value []byte) error {
			if !validSecret(value) {
				return ErrUnavailable
			}
			key = append([]byte(nil), value...)
			return nil
		})
	}()
	if err != nil {
		return Completion{}, ErrUnavailable
	}
	engine, e := r.options.Factory(ctx, cfg, key, id)
	if e != nil || engine == nil {
		return Completion{}, ErrUnavailable
	}
	defer engine.Close()
	if ctx.Err() != nil {
		return Completion{}, ctx.Err()
	}
	ctx = WithProgress(ctx, func(event ProgressEvent) { r.observe(ctx, id, runID, key, event) })
	result, err = engine.Complete(ctx, history, prompt)
	if err != nil {
		return Completion{}, safeFailure(err)
	}
	if result.Navigation != nil && !result.Navigation.Valid() {
		return Completion{}, ErrUnavailable
	}
	evidence, encodeErr := jsoncodec.Marshal([]any{result.Evidence, result.Navigation})
	if encodeErr != nil || !validEvidence(result.Evidence) || strings.Contains(string(evidence), string(key)) {
		return Completion{}, ErrUnavailable
	}
	if len(result.History) > MaxHistoryBytes {
		return Completion{}, BudgetFailure("history_bytes")
	}
	if len(result.Text) > MaxTextBytes {
		return Completion{}, BudgetFailure("text_bytes")
	}
	if len(result.History) == 0 || strings.Contains(result.Text, string(key)) || strings.Contains(string(result.History), string(key)) {
		return Completion{}, ErrUnavailable
	}
	return result, err
}

func promptDigest(prompt string, page PageSelection, leases ...string) [32]byte {
	lease := ""
	if len(leases) > 0 {
		lease = leases[0]
	}
	if page == (PageSelection{}) && lease == "" {
		return sha256.Sum256([]byte(prompt))
	}
	var raw []byte
	if lease == "" {
		raw, _ = jsoncodec.Marshal(page)
	} else {
		raw, _ = jsoncodec.Marshal([]any{page, lease})
	}
	return sha256.Sum256(append(append([]byte(prompt), 0), raw...))
}
