package assistant

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// UsageCall contains accounting metadata only, never prompts, audio or responses.
// Each nullable counter preserves provider omission. Details are subsets, not
// additional tokens. No current price is applied to historical requests.
type UsageCall struct {
	CacheWrite30m  *int64     `json:"cacheWrite30m"`
	CacheWrite12h  *int64     `json:"cacheWrite12h"`
	ServiceTier    string     `json:"serviceTier"`
	PricingScope   string     `json:"pricingScope"`
	UsageInvalid   bool       `json:"usageInvalid"`
	Cost           *UsageCost `json:"cost,omitempty"`
	ID             string     `json:"id"`
	ConversationID string     `json:"conversationId"`
	RunID          string     `json:"runId"`
	Kind           string     `json:"kind"`
	RequestedModel string     `json:"requestedModel"`
	Model          string     `json:"model"`
	Started        time.Time  `json:"started"`
	Finished       *time.Time `json:"finished"`
	Status         string     `json:"status"`
	Input          *int64     `json:"input"`
	CachedInput    *int64     `json:"cachedInput"`
	CacheWrite     *int64     `json:"cacheWrite"`
	Output         *int64     `json:"output"`
	Reasoning      *int64     `json:"reasoning"`
	AudioSeconds   *string    `json:"audioSeconds"`
}
type UsageStore interface {
	Record(UsageCall) error
	Calls() ([]UsageCall, error)
	Close() error
}
type usageKey struct{}
type usageSink func(UsageCall)

func WithUsage(ctx context.Context, sink func(UsageCall)) context.Context {
	return context.WithValue(ctx, usageKey{}, usageSink(sink))
}
func emitUsage(ctx context.Context, call UsageCall) {
	defer func() { _ = recover() }() // Accounting must never interrupt conversation work.
	if sink, ok := ctx.Value(usageKey{}).(usageSink); ok {
		sink(call)
	}
}
func BeginUsage(ctx context.Context, model string) UsageCall {
	c := UsageCall{ID: uuid.NewString(), RequestedModel: model, Started: time.Now().UTC(), Status: "pending"}
	emitUsage(ctx, c)
	return c
}
func FinishUsage(ctx context.Context, c UsageCall, succeeded bool) {
	now := time.Now().UTC()
	c.Finished = &now
	c.Status = "failed"
	if succeeded {
		c.Status = "succeeded"
	}
	emitUsage(ctx, c)
}

// CaptureUsage bypasses lossy SDK usage structs. Invalid/absent numbers remain
// unknown. Only explicitly allowlisted metadata leaves the response boundary.
func CaptureUsage(c *UsageCall, raw []byte) {
	var response struct {
		Model       string                     `json:"model"`
		ServiceTier string                     `json:"service_tier"`
		Usage       map[string]json.RawMessage `json:"usage"`
	}
	if jsoncodec.Unmarshal(raw, &response) != nil {
		return
	}
	if (ModelConfig{Provider: "openai-responses", Model: response.Model}).Valid() {
		c.Model = response.Model
	}
	number := func(raw json.RawMessage) *int64 {
		var n int64
		if len(raw) == 0 || string(raw) == "null" || jsoncodec.Unmarshal(raw, &n) != nil || n < 0 || n > 1_000_000_000 {
			return nil
		}
		return &n
	}
	switch response.ServiceTier {
	case "default", "flex", "priority", "scale", "auto":
		c.ServiceTier = response.ServiceTier
	}
	u := response.Usage
	c.Input = number(u["input_tokens"])
	c.Output = number(u["output_tokens"])
	var in, out map[string]json.RawMessage
	_ = jsoncodec.Unmarshal(u["input_tokens_details"], &in)
	_ = jsoncodec.Unmarshal(u["output_tokens_details"], &out)
	c.CachedInput = number(in["cached_tokens"])
	c.CacheWrite = number(in["cache_write_tokens"])
	var lifetimes map[string]json.RawMessage
	_ = jsoncodec.Unmarshal(in["cache_write_tokens_details"], &lifetimes)
	c.CacheWrite30m = number(lifetimes["ttl_30m_tokens"])
	c.CacheWrite12h = number(lifetimes["ttl_12h_tokens"])
	c.Reasoning = number(out["reasoning_tokens"])
	if c.Input != nil {
		if c.CachedInput != nil && *c.CachedInput > *c.Input {
			c.CachedInput = nil
			c.UsageInvalid = true
		}
		if c.CacheWrite != nil && *c.CacheWrite > *c.Input {
			c.CacheWrite = nil
			c.UsageInvalid = true
		}
	}
	if c.Input != nil && c.CachedInput != nil && c.CacheWrite != nil && *c.CachedInput+*c.CacheWrite > *c.Input {
		c.CachedInput = nil
		c.CacheWrite = nil
		c.UsageInvalid = true
	}
	if c.CacheWrite != nil && c.CacheWrite30m != nil && c.CacheWrite12h != nil && *c.CacheWrite30m+*c.CacheWrite12h != *c.CacheWrite {
		c.CacheWrite30m = nil
		c.CacheWrite12h = nil
		c.UsageInvalid = true
	}
	if c.Output != nil && c.Reasoning != nil && *c.Reasoning > *c.Output {
		c.Reasoning = nil
	}
	// Transcription duration-based usage has a different unit; never invent tokens.
	if string(u["type"]) == `"duration"` {
		raw := string(u["seconds"])
		if len(raw) > 0 && len(raw) < 32 && raw[0] != '"' {
			if seconds, err := decimal.NewFromString(raw); err == nil && seconds.Exponent() >= -9 && seconds.Exponent() <= 6 && !seconds.IsNegative() && seconds.LessThanOrEqual(decimal.NewFromInt(1_000_000)) {
				value := seconds.String()
				c.AudioSeconds = &value
			}
		}
	}
}
func (r *Runtime) usageContext(ctx context.Context, conversation, run, kind string) context.Context {
	return WithUsage(ctx, func(c UsageCall) {
		defer func() {
			if recover() != nil {
				r.usageFailed.Store(true)
			}
		}()
		c.ConversationID = conversation
		c.RunID = run
		c.Kind = kind
		if c.Status != "pending" {
			c.Cost = estimateUsage(c, usagePrices)
		}
		if r.options.Usage == nil || r.options.Usage.Record(c) != nil {
			r.usageFailed.Store(true)
		}
	})
}

type UsageQuery struct {
	Timezone       string `json:"timezone"`
	From           string `json:"from"`
	To             string `json:"to"`
	ConversationID string `json:"conversationId"`
	RunID          string `json:"runId"`
	Model          string `json:"model"`
	GroupBy        string `json:"groupBy"`
}
type UsageTotal struct {
	EstimatedUSD     string `json:"estimatedUsd"`
	CachedUSD        string `json:"cachedUsd"`
	CacheWriteUSD    string `json:"cacheWriteUsd"`
	PricedCalls      int    `json:"pricedCalls"`
	UnpricedCalls    int    `json:"unpricedCalls"`
	AudioSeconds     string `json:"audioSeconds"`
	AudioCalls       int    `json:"audioCalls"`
	MissingAudio     int    `json:"missingAudio"`
	Tokens           int64  `json:"tokens"`
	MissingTokens    int    `json:"missingTokens"`
	Calls            int    `json:"calls"`
	Failed           int    `json:"failed"`
	Pending          int    `json:"pending"`
	Input            int64  `json:"input"`
	Output           int64  `json:"output"`
	CachedInput      int64  `json:"cachedInput"`
	CacheWrite       int64  `json:"cacheWrite"`
	Reasoning        int64  `json:"reasoning"`
	MissingInput     int    `json:"missingInput"`
	MissingOutput    int    `json:"missingOutput"`
	MissingCached    int    `json:"missingCached"`
	MissingWrite     int    `json:"missingWrite"`
	MissingReasoning int    `json:"missingReasoning"`
}
type UsageGroup struct {
	Key     string     `json:"key"`
	Label   string     `json:"label"`
	Deleted bool       `json:"deleted"`
	Total   UsageTotal `json:"total"`
}
type UsageReport struct {
	Prices        []PriceSnapshot `json:"prices"`
	Conversations []UsageGroup    `json:"conversations"`
	Total         UsageTotal      `json:"total"`
	Groups        []UsageGroup    `json:"groups"`
	Models        []string        `json:"models"`
	Available     bool            `json:"available"`
	Incomplete    bool            `json:"incomplete"`
	CostStatus    string          `json:"costStatus"`
}

func (s *UsageTotal) add(c UsageCall) {
	s.Calls++
	if c.Cost != nil && c.Cost.Status == "estimated" {
		s.PricedCalls++
		for _, pair := range []struct {
			dst   *string
			value string
		}{{&s.EstimatedUSD, c.Cost.USD}, {&s.CachedUSD, c.Cost.CachedUSD}, {&s.CacheWriteUSD, c.Cost.CacheWriteUSD}} {
			old, _ := decimal.NewFromString(*pair.dst)
			v, _ := decimal.NewFromString(pair.value)
			*pair.dst = old.Add(v).String()
		}
	} else {
		s.UnpricedCalls++
	}
	if c.Kind == "transcription" {
		s.AudioCalls++
		if c.AudioSeconds == nil {
			s.MissingAudio++
		} else {
			current, _ := decimal.NewFromString(s.AudioSeconds)
			seconds, _ := decimal.NewFromString(*c.AudioSeconds)
			s.AudioSeconds = current.Add(seconds).String()
		}
	}
	if c.Input == nil || c.Output == nil {
		s.MissingTokens++
	} else {
		s.Tokens += *c.Input + *c.Output
	}
	if c.Status == "failed" {
		s.Failed++
	}
	if c.Status == "pending" {
		s.Pending++
	}
	add := func(v *int64, sum *int64, missing *int) {
		if v == nil {
			*missing++
		} else {
			*sum += *v
		}
	}
	add(c.Input, &s.Input, &s.MissingInput)
	add(c.Output, &s.Output, &s.MissingOutput)
	add(c.CachedInput, &s.CachedInput, &s.MissingCached)
	add(c.CacheWrite, &s.CacheWrite, &s.MissingWrite)
	add(c.Reasoning, &s.Reasoning, &s.MissingReasoning)
}
func (r *Runtime) Usage(ctx context.Context, epoch string, q UsageQuery) (UsageReport, error) {
	report := UsageReport{Prices: []PriceSnapshot{}, Conversations: []UsageGroup{}, Groups: []UsageGroup{}, Models: []string{}, CostStatus: "unavailable"}
	r.mu.Lock()
	if err := r.valid(ctx, epoch); err != nil {
		r.mu.Unlock()
		return report, err
	}
	titles := map[string]string{}
	for i, c := range r.state.Conversations {
		titles[c.ID] = c.Title
		if c.Title == "" {
			titles[c.ID] = strconv.Itoa(i + 1)
		}
	}
	r.mu.Unlock()
	loc, err := time.LoadLocation(q.Timezone)
	if err != nil {
		return report, ErrUnavailable
	}
	for _, d := range []string{q.From, q.To} {
		if d != "" {
			if _, err := time.Parse("2006-01-02", d); err != nil {
				return report, ErrUnavailable
			}
		}
	}
	if q.From != "" && q.To != "" && q.From > q.To {
		return report, ErrUnavailable
	}
	if q.GroupBy != "day" && q.GroupBy != "conversation" && q.GroupBy != "model" && q.GroupBy != "kind" {
		return report, ErrUnavailable
	}
	if r.options.Usage == nil {
		return report, nil
	}
	calls, err := r.options.Usage.Calls()
	if err != nil {
		return report, ErrStorage
	}
	r.mu.Lock()
	validErr := r.valid(ctx, epoch)
	r.mu.Unlock()
	if validErr != nil {
		return UsageReport{}, validErr
	}
	report.Available = true
	report.Incomplete = r.usageFailed.Load()
	if coverage, ok := r.options.Usage.(interface{ Incomplete() bool }); ok {
		report.Incomplete = report.Incomplete || coverage.Incomplete()
	}
	groups := map[string]*UsageGroup{}
	models := map[string]bool{}
	conversations := map[string]bool{}
	for _, c := range calls {
		if c.ConversationID != "" {
			conversations[c.ConversationID] = true
		}
		model := c.Model
		if model == "" {
			model = c.RequestedModel + " (?)"
		}
		models[model] = true
		day := c.Started.In(loc).Format("2006-01-02")
		if (q.From != "" && day < q.From) || (q.To != "" && day > q.To) || (q.ConversationID != "" && c.ConversationID != q.ConversationID) || (q.RunID != "" && c.RunID != q.RunID) || (q.Model != "" && model != q.Model) {
			continue
		}
		key, label, deleted := day, day, false
		switch q.GroupBy {
		case "model":
			key = model
			label = model
		case "kind":
			key = c.Kind
			label = c.Kind
		case "conversation":
			key = c.ConversationID
			label = titles[key]
			deleted = key != "" && label == ""
		}
		if groups[key] == nil {
			groups[key] = &UsageGroup{Key: key, Label: label, Deleted: deleted}
		}
		groups[key].Total.add(c)
		report.Total.add(c)
		if c.Cost != nil && c.Cost.Price != nil {
			p := *c.Cost.Price
			found := false
			for _, prior := range report.Prices {
				if prior == p {
					found = true
					break
				}
			}
			if !found {
				report.Prices = append(report.Prices, p)
			}
		}
	}
	for _, g := range groups {
		report.Groups = append(report.Groups, *g)
	}
	sort.Slice(report.Groups, func(i, j int) bool { return report.Groups[i].Key > report.Groups[j].Key })
	for id := range conversations {
		report.Conversations = append(report.Conversations, UsageGroup{Key: id, Label: titles[id], Deleted: titles[id] == ""})
	}
	sort.Slice(report.Conversations, func(i, j int) bool { return report.Conversations[i].Key < report.Conversations[j].Key })
	for m := range models {
		report.Models = append(report.Models, m)
	}
	sort.Strings(report.Models)
	sort.Slice(report.Prices, func(i, j int) bool {
		a, b := report.Prices[i], report.Prices[j]
		if a.Model != b.Model {
			return a.Model < b.Model
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		return !a.LongContext && b.LongContext
	})
	if report.Total.PricedCalls > 0 {
		report.CostStatus = "estimated"
		if report.Total.UnpricedCalls > 0 {
			report.CostStatus = "partial"
		}
	}
	return report, nil
}

// ValidateUsageCall rejects corrupt accounting metadata independently of chat state.
func ValidateUsageCall(c UsageCall) bool {
	if !canonicalID(c.ID) || (c.ConversationID != "" && !canonicalID(c.ConversationID)) || (c.RunID != "" && !canonicalID(c.RunID)) || c.Started.IsZero() {
		return false
	}
	if c.Kind != "conversation" && c.Kind != "title" && c.Kind != "transcription" {
		return false
	}
	if !(ModelConfig{Provider: "openai-responses", Model: c.RequestedModel}).Valid() || (c.Model != "" && !(ModelConfig{Provider: "openai-responses", Model: c.Model}).Valid()) {
		return false
	}
	if c.Status != "pending" && c.Status != "succeeded" && c.Status != "failed" {
		return false
	}
	if (c.Status == "pending") != (c.Finished == nil) {
		return false
	}
	for _, n := range []*int64{c.Input, c.Output, c.CachedInput, c.CacheWrite, c.CacheWrite30m, c.CacheWrite12h, c.Reasoning} {
		if n != nil && (*n < 0 || *n > 1_000_000_000) {
			return false
		}
	}
	if c.AudioSeconds != nil {
		if len(*c.AudioSeconds) > 32 {
			return false
		}
		d, err := decimal.NewFromString(*c.AudioSeconds)
		if err != nil || d.Exponent() < -9 || d.Exponent() > 6 || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(1_000_000)) {
			return false
		}
	}
	return validUsageCost(c.Cost)
}
