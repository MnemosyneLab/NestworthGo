package mcpserver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const readTTL = 5 * time.Minute
const readCaptureLimit = 4 << 20
const readCacheLimit = 16 << 20
const readCacheCount = 8
const readBuildTimeout = 10 * time.Second
const readBuildConcurrency = 2

type ReadPageInput struct {
	CaptureID string `json:"captureId"`
	Section   string `json:"section"`
	Cursor    string `json:"cursor"`
	Limit     int    `json:"limit,omitempty"`
}
type ReadItemInput struct {
	CaptureID string `json:"captureId"`
	Section   string `json:"section"`
	ID        string `json:"id"`
}
type ReadPageInfo struct {
	Total      int     `json:"total"`
	Returned   int     `json:"returned"`
	HasMore    bool    `json:"hasMore"`
	NextCursor *string `json:"nextCursor"`
}
type ReadSection struct {
	Items []json.RawMessage `json:"items"`
	Page  ReadPageInfo      `json:"page"`
}
type ReadEnvelope struct {
	SchemaVersion string                 `json:"schemaVersion"`
	CaptureID     string                 `json:"captureId"`
	CaptureKind   string                 `json:"captureKind"`
	CapturedAt    string                 `json:"capturedAt"`
	ExpiresAt     string                 `json:"expiresAt"`
	ContentHash   string                 `json:"contentHash"`
	SummaryHash   string                 `json:"summaryHash"`
	Query         any                    `json:"query,omitempty"`
	Summary       any                    `json:"summary,omitempty"`
	Sections      map[string]ReadSection `json:"sections,omitempty"`
	Section       string                 `json:"section,omitempty"`
	Items         []json.RawMessage      `json:"items,omitempty"`
	Page          *ReadPageInfo          `json:"page,omitempty"`
}
type readCapture struct {
	DetailSections    map[string][]string          `json:"detailSections,omitempty"`
	InitialSections   []string                     `json:"initialSections,omitempty"`
	Kind              string                       `json:"kind"`
	Query             any                          `json:"query"`
	Summary           any                          `json:"summary"`
	Sections          map[string][]json.RawMessage `json:"sections"`
	Details           map[string]any               `json:"details,omitempty"`
	CapturedAt        time.Time                    `json:"capturedAt"`
	hash, summaryHash string
	expires           time.Time
	size              int
}
type readCursor struct {
	Version    int    `json:"v"`
	CaptureID  string `json:"capture"`
	Section    string `json:"section"`
	Hash       string `json:"hash"`
	Generation uint64 `json:"generation"`
	Offset     int    `json:"offset"`
}
type boundedReadCache struct {
	mu         sync.Mutex
	generation uint64
	revoked    bool
	key        [32]byte
	entries    map[string]readCapture
	bytes      int
	now        func() time.Time
	builds     chan struct{}
}

func newBoundedReadCache() *boundedReadCache {
	c := &boundedReadCache{revoked: true, entries: map[string]readCapture{}, now: time.Now, builds: make(chan struct{}, readBuildConcurrency)}
	if _, err := rand.Read(c.key[:]); err != nil {
		panic("read cursor entropy unavailable")
	}
	return c
}
func (c *boundedReadCache) activate(g uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation = g
	c.revoked = false
	c.entries = map[string]readCapture{}
	c.bytes = 0
}
func (c *boundedReadCache) revoke() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revoked = true
	c.entries = map[string]readCapture{}
	c.bytes = 0
}
func readLimit(n int) (int, error) {
	if n == 0 {
		return 25, nil
	}
	if n < 1 || n > 100 {
		return 0, fail("validation", "limit must be 1 to 100")
	}
	return n, nil
}
func readRows[T any](values []T) ([]json.RawMessage, error) {
	rows := make([]json.RawMessage, 0, len(values))
	size := 0
	for _, v := range values {
		b, e := jsoncodec.Marshal(v)
		if e != nil {
			return nil, e
		}
		size += len(b)
		if size > readCaptureLimit {
			return nil, fail("too_large", "capture exceeds read budget; narrow filters")
		}
		rows = append(rows, b)
	}
	return rows, nil
}
func (c *boundedReadCache) cursor(id, section string, e readCapture, offset int) string {
	b, _ := jsoncodec.Marshal(readCursor{1, id, section, e.hash, c.generation, offset})
	mac := hmac.New(sha256.New, c.key[:])
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(append(b, mac.Sum(nil)...))
}
func (c *boundedReadCache) decode(v string) (readCursor, error) {
	var out readCursor
	if len(v) > 2048 {
		return out, fail("validation", "invalid read cursor")
	}
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil || len(b) < 32 {
		return out, fail("validation", "invalid read cursor")
	}
	data, sig := b[:len(b)-32], b[len(b)-32:]
	mac := hmac.New(sha256.New, c.key[:])
	mac.Write(data)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return out, fail("validation", "invalid read cursor")
	}
	if jsoncodec.Unmarshal(data, &out) != nil || out.Version != 1 {
		return out, fail("validation", "invalid read cursor")
	}
	return out, nil
}
func (c *boundedReadCache) prune() {
	now := c.now()
	for id, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, id)
			c.bytes -= e.size
		}
	}
}
func (c *boundedReadCache) entry(g uint64, id string) (readCapture, error) {
	if c.revoked || c.generation != g {
		return readCapture{}, fail("read_revoked", "read capture revoked; restart every section")
	}
	c.prune()
	e, ok := c.entries[id]
	if !ok {
		return e, fail("read_expired", "read capture expired or evicted; restart every section")
	}
	return e, nil
}
func (c *boundedReadCache) base(id string, e readCapture) ReadEnvelope {
	return ReadEnvelope{SchemaVersion: "mcp-read/1", CaptureID: id, CaptureKind: e.Kind, CapturedAt: e.CapturedAt.UTC().Format(time.RFC3339Nano), ExpiresAt: e.expires.UTC().Format(time.RFC3339Nano), ContentHash: e.hash, SummaryHash: e.summaryHash}
}
func (c *boundedReadCache) pageInfo(id, section string, e readCapture, start, n int) ReadPageInfo {
	total := len(e.Sections[section])
	p := ReadPageInfo{Total: total, Returned: n, HasMore: start+n < total}
	if p.HasMore {
		v := c.cursor(id, section, e, start+n)
		p.NextCursor = &v
	}
	return p
}
func readFits(v any) (bool, error) {
	size, err := financialContextWireSize(v)
	return size <= contextWireLimit, err
}
func (c *boundedReadCache) initial(id string, e readCapture, limit int) (ReadEnvelope, error) {
	r := c.base(id, e)
	r.Query = e.Query
	r.Summary = e.Summary
	r.Sections = map[string]ReadSection{}
	names := make([]string, 0, len(e.Sections))
	for k := range e.Sections {
		if len(e.InitialSections) == 0 || hasString(e.InitialSections, k) {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	for _, k := range names {
		r.Sections[k] = ReadSection{Items: []json.RawMessage{}, Page: c.pageInfo(id, k, e, 0, 0)}
	}
	if ok, err := readFits(r); err != nil {
		return r, err
	} else if !ok {
		return r, fail("too_large", "summary exceeds read wire budget; narrow query")
	}
	for _, k := range names {
		for n := 0; n < min(limit, len(e.Sections[k])); n++ {
			old := r.Sections[k]
			r.Sections[k] = ReadSection{Items: c.outputRows(id, e, k, 0, n+1), Page: c.pageInfo(id, k, e, 0, n+1)}
			ok, err := readFits(r)
			if err != nil {
				return r, err
			}
			if !ok {
				r.Sections[k] = old
				break
			}
		}
	}
	return r, nil
}
func (c *boundedReadCache) store(g uint64, e readCapture, limit int) (ReadEnvelope, error) {
	if e.CapturedAt.IsZero() {
		e.CapturedAt = c.now()
	}
	e.Query = normalizeFrozenQuery(e.Query)
	b, err := jsoncodec.Marshal(e)
	if err != nil {
		return ReadEnvelope{}, err
	}
	if len(b) > readCaptureLimit {
		return ReadEnvelope{}, fail("too_large", "capture exceeds read budget; narrow query")
	}
	// Decode into a fresh capture: Sonic may reuse map values and their slice
	// storage when decoding into e, which would retain caller-owned aliases.
	var frozen readCapture
	if err := decodeReadValue(b, &frozen); err != nil {
		return ReadEnvelope{}, err
	}
	e = frozen
	h := sha256.Sum256(b)
	e.hash = hex.EncodeToString(h[:])
	summary, _ := jsoncodec.Marshal(e.Summary)
	h = sha256.Sum256(summary)
	e.summaryHash = hex.EncodeToString(h[:])
	e.size = len(b)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revoked || g != c.generation {
		return ReadEnvelope{}, fail("read_revoked", "read capture revoked")
	}
	now := c.now()
	if e.CapturedAt.IsZero() {
		e.CapturedAt = now
	}
	e.expires = now.Add(readTTL)
	id := uuid.NewString()
	r, err := c.initial(id, e, limit)
	if err != nil {
		return r, err
	}
	c.prune()
	for len(c.entries) >= readCacheCount || c.bytes+e.size > readCacheLimit {
		old := ""
		for k, v := range c.entries {
			if old == "" || v.expires.Before(c.entries[old].expires) || (v.expires.Equal(c.entries[old].expires) && k < old) {
				old = k
			}
		}
		c.bytes -= c.entries[old].size
		delete(c.entries, old)
	}
	c.entries[id] = e
	c.bytes += e.size
	return r, nil
}
func (c *boundedReadCache) page(g uint64, in ReadPageInput) (ReadEnvelope, error) {
	limit, err := readLimit(in.Limit)
	if err != nil {
		return ReadEnvelope{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, err := c.entry(g, in.CaptureID)
	if err != nil {
		return ReadEnvelope{}, err
	}
	cur, err := c.decode(in.Cursor)
	rows, ok := e.Sections[in.Section]
	if err != nil || !ok || cur.CaptureID != in.CaptureID || cur.Section != in.Section || cur.Hash != e.hash || cur.Generation != g || cur.Offset < 0 || cur.Offset >= len(rows) {
		return ReadEnvelope{}, fail("validation", "cursor must match capture, section and query")
	}
	r := c.base(in.CaptureID, e)
	r.Section = in.Section
	r.Items = []json.RawMessage{}
	p := c.pageInfo(in.CaptureID, in.Section, e, cur.Offset, 0)
	r.Page = &p
	for n := 0; n < min(limit, len(rows)-cur.Offset); n++ {
		r.Items = c.outputRows(in.CaptureID, e, in.Section, cur.Offset, n+1)
		p = c.pageInfo(in.CaptureID, in.Section, e, cur.Offset, n+1)
		ok, err := readFits(r)
		if err != nil {
			return r, err
		}
		if !ok {
			if n == 0 {
				return ReadEnvelope{}, fail("too_large", "single_item exceeds read wire budget in "+in.Section+"; use a narrower detail workflow")
			}
			r.Items = c.outputRows(in.CaptureID, e, in.Section, cur.Offset, n)
			p = c.pageInfo(in.CaptureID, in.Section, e, cur.Offset, n)
			break
		}
	}
	return r, nil
}
func (s *Service) buildRead(ctx context.Context, g uint64, limit int, fn func(context.Context) (readCapture, error)) (ReadEnvelope, error) {
	limit, err := readLimit(limit)
	if err != nil {
		return ReadEnvelope{}, err
	}
	ctx, cancel := context.WithTimeout(domain.WithBoundedRead(ctx), readBuildTimeout)
	defer cancel()
	select {
	case s.reads.builds <- struct{}{}:
		defer func() { <-s.reads.builds }()
	case <-ctx.Done():
		return ReadEnvelope{}, fail("read_busy", "read build budget elapsed")
	}
	s.reads.mu.Lock()
	active := !s.reads.revoked && s.reads.generation == g
	s.reads.mu.Unlock()
	if !active {
		return ReadEnvelope{}, fail("read_revoked", "read capture revoked")
	}
	e, err := fn(ctx)
	if err != nil {
		return ReadEnvelope{}, err
	}
	if ctx.Err() != nil {
		return ReadEnvelope{}, fail("read_busy", "read build budget elapsed")
	}
	return s.reads.store(g, e, limit)
}
func (s *Service) boundedReadTools(server *mcp.Server, g uint64) {
	readTool(server, "get_read_page", "Continue a frozen mcp-read/1 section with its exact captureId and signed cursor. Default limit 25, maximum 100. Follow every cursor before claiming completeness; restart all sections after read_expired/read_revoked. No DB reads or TTL renewal.", func(ctx context.Context, in ReadPageInput) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return s.reads.page(g, in)
	})
	readTool(server, "get_read_item", "Read full item evidence from a frozen products, historical, account-valuations or directory capture. Supply the row id and section. Returned child sections have independent signed cursors; follow them before claiming completeness. Does not recapture or renew TTL.", func(ctx context.Context, in ReadItemInput) (any, error) {
		s.reads.mu.Lock()
		defer s.reads.mu.Unlock()
		e, err := s.reads.entry(g, in.CaptureID)
		if err != nil {
			return nil, err
		}
		if e.Kind == "historical_overview" || e.Kind == "historical_overview_series" || e.Kind == "account_valuations" || e.Kind == "directory" {
			key := in.Section + "/" + in.ID
			v, ok := e.Details[key]
			if !ok {
				return nil, fail("not_found", "item not found in capture section")
			}
			r := s.reads.base(in.CaptureID, e)
			r.Section, r.Summary = in.Section, v
			r.Sections = map[string]ReadSection{}
			for _, section := range e.DetailSections[key] {
				r.Sections[section] = ReadSection{Items: []json.RawMessage{}, Page: s.reads.pageInfo(in.CaptureID, section, e, 0, 0)}
			}
			if ok, err := readFits(r); err != nil {
				return nil, err
			} else if !ok {
				return nil, fail("too_large", "single_item detail exceeds read wire budget; narrow the source query")
			}
			return r, nil
		}
		if in.Section != "products" || e.Kind != "products" {
			return nil, fail("validation", "item section must be products")
		}
		v, ok := e.Details[in.ID]
		if !ok {
			return nil, fail("not_found", "item not found in capture")
		}
		r := s.reads.base(in.CaptureID, e)
		r.Summary = v
		r.Sections = map[string]ReadSection{}
		k := "reservations/" + in.ID
		r.Sections[k] = ReadSection{Items: []json.RawMessage{}, Page: s.reads.pageInfo(in.CaptureID, k, e, 0, 0)}
		if ok, err := readFits(r); err != nil {
			return nil, err
		} else if !ok {
			return nil, fail("too_large", "single_item product detail exceeds read wire budget")
		}
		return r, nil
	})
}
func isReadScaleTool(name string) bool {
	switch name {
	case "get_read_page", "get_read_item", "get_market_data_batch", "update_instruments_batch":
		return true
	}
	return false
}

func normalizeFrozenQuery(v any) any {
	b, err := jsoncodec.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if decodeReadValue(b, &out) != nil {
		return v
	}
	return normalizeFrozenValue(out, "")
}
func normalizeFrozenValue(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			x[k] = normalizeFrozenValue(value, k)
		}
		return x
	case []any:
		byJSON := map[string]any{}
		keys := []string{}
		for _, value := range x {
			value = normalizeFrozenValue(value, key)
			b, _ := jsoncodec.Marshal(value)
			k := string(b)
			if _, ok := byJSON[k]; !ok {
				keys = append(keys, k)
				byJSON[k] = value
			}
		}
		sort.Strings(keys)
		out := make([]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, byJSON[k])
		}
		return out
	case string:
		if key == "query" {
			return normalizedReadName(x)
		}
	}
	return v
}

// Market target pages carry lazy history entrances. Advertising all 200 history
// cursors in a 100-target initial header would itself exceed the wire budget.
func (c *boundedReadCache) outputRows(id string, e readCapture, section string, start, count int) []json.RawMessage {
	rows := e.Sections[section][start : start+count]
	if e.Kind != "market_data" || section != "targets" {
		return rows
	}
	out := make([]json.RawMessage, 0, len(rows))
	for _, raw := range rows {
		var v map[string]any
		if decodeReadValue(raw, &v) != nil {
			out = append(out, raw)
			continue
		}
		sections := map[string]ReadSection{}
		for _, field := range []string{"pointsSection", "observationsSection"} {
			if key, ok := v[field].(string); ok {
				sections[key] = ReadSection{Items: []json.RawMessage{}, Page: c.pageInfo(id, key, e, 0, 0)}
			}
		}
		if len(sections) > 0 {
			v["sections"] = sections
		}
		b, _ := jsoncodec.Marshal(v)
		out = append(out, b)
	}
	return out
}

// Keep revision/CAS integers exact when freezing heterogeneous DTO envelopes.
// Financial amounts remain their existing decimal-string projections.
func decodeReadValue(raw []byte, value any) error {
	decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(value)
}
