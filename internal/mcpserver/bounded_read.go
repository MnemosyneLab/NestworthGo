package mcpserver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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
	CaptureID string `json:"captureId,omitempty"`
	Section   string `json:"section,omitempty"`
	Cursor    string `json:"cursor"`
	Limit     int    `json:"limit,omitempty"`
}
type ReadItemInput struct {
	CaptureID string   `json:"captureId"`
	Section   string   `json:"section"`
	ID        string   `json:"id"`
	Sections  []string `json:"sections,omitempty" jsonschema:"Child section names to read; omitted selects business details. Use availableSections to request other evidence."`
	Limit     int      `json:"limit,omitempty" jsonschema:"Maximum initial rows per selected child section for every capture kind; default 25, maximum 100, subject to the response byte budget. Continue remaining rows with get_read_page."`
}
type ReadPageInfo struct {
	Total       int     `json:"total"`
	Returned    int     `json:"returned"`
	HasMore     bool    `json:"hasMore"`
	NextCursor  *string `json:"nextCursor"`
	Consistency string  `json:"consistency"`
}
type ReadSection struct {
	Items []json.RawMessage `json:"items"`
	Page  ReadPageInfo      `json:"page"`
}

// ReadDelivery reports whether every row of the selected sections is already in
// this response. It describes transport completeness only: data completeness,
// quality and missing inputs stay in the summary and are never implied by it.
type ReadDelivery struct {
	SelectionComplete bool     `json:"selectionComplete"`
	PendingSections   []string `json:"pendingSections"`
}
type ReadEnvelope struct {
	SchemaVersion     string                 `json:"schemaVersion"`
	CaptureID         string                 `json:"captureId"`
	CaptureKind       string                 `json:"captureKind"`
	CapturedAt        string                 `json:"capturedAt"`
	Summary           any                    `json:"summary,omitempty"`
	Sections          map[string]ReadSection `json:"sections,omitempty"`
	Delivery          *ReadDelivery          `json:"delivery,omitempty"`
	Section           string                 `json:"section,omitempty"`
	Items             []json.RawMessage      `json:"items,omitempty"`
	Page              *ReadPageInfo          `json:"page,omitempty"`
	AvailableSections []string               `json:"availableSections,omitempty"`
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
	AvailableSections []string                     `json:"availableSections,omitempty"`
	// drilldown is frozen with the capture and omitted from the marshal that
	// store charges against readCaptureLimit. Its own retained charge is added
	// to size for admission and eviction. Reattach it after the JSON
	// round-trip below.
	drilldown   *periodDrilldown
	itemSection string
	expires     time.Time
	size        int
}
type readCursor struct {
	Version    int    `json:"v"`
	CaptureID  string `json:"capture"`
	Section    string `json:"section"`
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

// Reads whose first response used to carry five rows per section choose
// their initial rows from a byte budget when the caller gave no limit. An
// explicit limit keeps its exact meaning. The sentinel is internal and never
// reaches the wire.
const (
	autoInlineLimit      = math.MinInt
	autoInlineFloorRows  = 5
	autoInlineMaxRows    = 25
	autoInlineWireBudget = 24 << 10
)

func autoInlineWhenUnset(n int) int {
	switch n {
	case 0:
		return autoInlineLimit
	case autoInlineLimit:
		return -1 // an explicit sentinel value is just another invalid limit
	}
	return n
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
func (c *boundedReadCache) cursor(id, section string, offset int) string {
	// The immutable capture UUID binds the frozen content in this session.
	// Binary framing avoids repeating JSON keys, UUID text and a 64-byte hash.
	capture := uuid.MustParse(id)
	b := make([]byte, 29, 29+len(section)+sha256.Size)
	b[0] = 2
	copy(b[1:17], capture[:])
	binary.BigEndian.PutUint64(b[17:25], c.generation)
	binary.BigEndian.PutUint32(b[25:29], uint32(offset))
	b = append(b, section...)
	mac := hmac.New(sha256.New, c.key[:])
	mac.Write(b)
	return "r2." + base64.RawURLEncoding.EncodeToString(append(b, mac.Sum(nil)...))
}
func (c *boundedReadCache) decode(v string) (readCursor, error) {
	var out readCursor
	if len(v) > 2048 || !strings.HasPrefix(v, "r2.") {
		return out, fail("validation", "invalid read cursor")
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(v, "r2."))
	if err != nil || len(b) < 30+sha256.Size || b[0] != 2 {
		return out, fail("validation", "invalid read cursor")
	}
	data, sig := b[:len(b)-sha256.Size], b[len(b)-sha256.Size:]
	mac := hmac.New(sha256.New, c.key[:])
	mac.Write(data)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return out, fail("validation", "invalid read cursor")
	}
	capture, _ := uuid.FromBytes(data[1:17])
	return readCursor{Version: 2, CaptureID: capture.String(), Generation: binary.BigEndian.Uint64(data[17:25]), Offset: int(binary.BigEndian.Uint32(data[25:29])), Section: string(data[29:])}, nil
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
	return ReadEnvelope{SchemaVersion: "mcp-read/2", CaptureID: id, CaptureKind: e.Kind, CapturedAt: e.CapturedAt.UTC().Format(time.RFC3339Nano)}
}
func (c *boundedReadCache) pageInfo(id, section string, e readCapture, start, n int) ReadPageInfo {
	rows, _, _ := readSectionRows(e, section)
	return c.pageInfoSize(id, section, e, len(rows), start, n)
}
func (c *boundedReadCache) pageInfoSize(id, section string, e readCapture, total, start, n int) ReadPageInfo {
	p := ReadPageInfo{Total: total, Returned: n, HasMore: start+n < total, Consistency: "frozen"}
	if p.HasMore {
		v := c.cursor(id, section, start+n)
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
	r.Section, r.AvailableSections = e.itemSection, e.AvailableSections
	r.Summary = e.Summary
	r.Sections = map[string]ReadSection{}
	names := make([]string, 0, len(e.Sections))
	for k := range e.Sections {
		if len(e.InitialSections) == 0 || hasString(e.InitialSections, k) {
			names = append(names, k)
		}
	}
	if comparisonSummary(e) {
		names = append(names, "result_details")
	}
	sort.Slice(names, func(i, j int) bool {
		// Small shared target metadata can fit in the first comparison response.
		// Reserving it first avoids a separate metadata-only continuation call.
		if e.Kind == "market_import_comparison" && ((names[i] == "targets") != (names[j] == "targets")) {
			return names[i] == "targets"
		}
		return names[i] < names[j]
	})
	for _, k := range names {
		r.Sections[k] = ReadSection{Items: []json.RawMessage{}, Page: c.pageInfo(id, k, e, 0, 0)}
	}
	selected := make([]string, 0, len(names))
	for _, k := range names {
		if !comparisonSummary(e) || k == "results" {
			selected = append(selected, k)
		}
	}
	// Reserve the worst-case delivery object before any wire-budget check.
	r.Delivery = &ReadDelivery{SelectionComplete: false, PendingSections: selected}
	if ok, err := readFits(r); err != nil {
		return r, err
	} else if !ok {
		return r, fail("too_large", "summary exceeds read wire budget; narrow query")
	}
	auto := limit == autoInlineLimit
	if auto {
		limit = autoInlineFloorRows
	}
	allRows := map[string][]json.RawMessage{}
	for _, k := range selected {
		rows, _, err := readSectionRows(e, k)
		if err != nil {
			return r, err
		}
		allRows[k] = rows
		for n := 0; n < min(limit, len(rows)); n++ {
			old := r.Sections[k]
			items, err := c.outputRows(id, e, k, rows, 0, n+1)
			if err != nil {
				return r, err
			}
			r.Sections[k] = ReadSection{Items: items, Page: c.pageInfoSize(id, k, e, len(rows), 0, n+1)}
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
	if auto {
		// Extend one row per section per pass so a large early section cannot
		// starve later ones. A section that fails to grow stays stalled; the
		// floor above is never reduced.
		for pass := autoInlineFloorRows; pass < autoInlineMaxRows; pass++ {
			progressed := false
			for _, k := range selected {
				rows := allRows[k]
				if len(r.Sections[k].Items) != pass || pass >= len(rows) {
					continue
				}
				old := r.Sections[k]
				items, err := c.outputRows(id, e, k, rows, 0, pass+1)
				if err != nil {
					return r, err
				}
				r.Sections[k] = ReadSection{Items: items, Page: c.pageInfoSize(id, k, e, len(rows), 0, pass+1)}
				size, err := financialContextWireSize(r)
				if err != nil {
					return r, err
				}
				if size > autoInlineWireBudget {
					r.Sections[k] = old
					continue
				}
				progressed = true
			}
			if !progressed {
				break
			}
		}
	}
	pending := []string{}
	for _, k := range selected {
		if r.Sections[k].Page.HasMore {
			pending = append(pending, k)
		}
	}
	r.Delivery = &ReadDelivery{SelectionComplete: len(pending) == 0, PendingSections: pending}
	return r, nil
}
func (c *boundedReadCache) store(g uint64, e readCapture, limit int) (ReadEnvelope, error) {
	if e.CapturedAt.IsZero() {
		e.CapturedAt = c.now()
	}
	drilldown := e.drilldown
	e.Query = normalizeFrozenQuery(e.Query)
	b, err := jsoncodec.Marshal(e)
	if err != nil {
		return ReadEnvelope{}, err
	}
	if len(b) > readCaptureLimit {
		return ReadEnvelope{}, fail("too_large", "capture exceeds read budget; narrow query")
	}
	retainedSize := len(b)
	if drilldown != nil {
		retainedSize += drilldown.size
	}
	if retainedSize > readRetainedCaptureLimit {
		return ReadEnvelope{}, fail("too_large", "capture and period details exceed retained budget; narrow query")
	}
	// Decode into a fresh capture: Sonic may reuse map values and their slice
	// storage when decoding into e, which would retain caller-owned aliases.
	var frozen readCapture
	if err := decodeReadValue(b, &frozen); err != nil {
		return ReadEnvelope{}, err
	}
	e = frozen
	e.drilldown = drilldown
	e.size = retainedSize
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
	if err != nil || cur.CaptureID != in.CaptureID || cur.Section != in.Section || cur.Generation != g || cur.Offset < 0 {
		return ReadEnvelope{}, fail("validation", "cursor must match capture, section and query")
	}
	rows, ok, sectionErr := readSectionRows(e, in.Section)
	if sectionErr != nil {
		return ReadEnvelope{}, sectionErr
	}
	if !ok || cur.Offset >= len(rows) {
		return ReadEnvelope{}, fail("validation", "cursor must match capture, section and query")
	}
	r := c.base(in.CaptureID, e)
	r.Section = in.Section
	r.Items = []json.RawMessage{}
	p := c.pageInfoSize(in.CaptureID, in.Section, e, len(rows), cur.Offset, 0)
	r.Page = &p
	for n := 0; n < min(limit, len(rows)-cur.Offset); n++ {
		r.Items, err = c.outputRows(in.CaptureID, e, in.Section, rows, cur.Offset, n+1)
		if err != nil {
			return r, err
		}
		p = c.pageInfoSize(in.CaptureID, in.Section, e, len(rows), cur.Offset, n+1)
		ok, err := readFits(r)
		if err != nil {
			return r, err
		}
		if !ok {
			if n == 0 {
				return ReadEnvelope{}, fail("too_large", "single_item exceeds read wire budget in "+in.Section+"; use a narrower detail workflow")
			}
			r.Items, err = c.outputRows(in.CaptureID, e, in.Section, rows, cur.Offset, n)
			if err != nil {
				return r, err
			}
			p = c.pageInfoSize(in.CaptureID, in.Section, e, len(rows), cur.Offset, n)
			break
		}
	}
	return r, nil
}
func (s *Service) buildRead(ctx context.Context, g uint64, limit int, fn func(context.Context) (readCapture, error)) (ReadEnvelope, error) {
	if limit != autoInlineLimit {
		var err error
		if limit, err = readLimit(limit); err != nil {
			return ReadEnvelope{}, err
		}
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
func (s *Service) boundedReadTools(server *toolSet, g uint64) {
	readTool(server, "get_read_page", "Continue any nextCursor with only cursor and an optional limit (default 25, max 100); the signed cursor carries its scope. A page with hasMore, or a deferred zero-row page, is incomplete. Restart the original read after read_expired, read_revoked or a stale cursor; an oversized item fails explicitly.", func(ctx context.Context, in ReadPageInput) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return s.continuePage(ctx, server, g, in)
	})
	readTool(server, "get_read_item", "Open a frozen item by captureId, section and id without recapturing or renewing TTL. Every kind, including market comparison and period analysis, accepts limit (default 25/max 100) for initial rows per child within the byte budget. Optional sections selects short child names; availableSections lists unselected names. Omitted selects business details (accounts: components and missing inputs). Continue child cursors with get_read_page. Comparison: section=results, id=clientItemId; contributions: section=rows, id=row key; period: section=returnDays, id=YYYY-MM-DD, or section=assetDrivers, id=assetChange bucket key. Period children are frozen with analyze_period but omitted from its stored body.", func(ctx context.Context, in ReadItemInput) (any, error) {
		return s.readItem(g, in, "")
	})
}
func (s *Service) readItem(g uint64, in ReadItemInput, requiredKind string) (any, error) {
	s.reads.mu.Lock()
	defer s.reads.mu.Unlock()
	e, err := s.reads.entry(g, in.CaptureID)
	if err != nil {
		return nil, err
	}
	if requiredKind != "" && e.Kind != requiredKind {
		return nil, fail("validation", "capture kind does not match item tool")
	}
	if e.Kind == "market_import_comparison" {
		return s.reads.comparisonItem(in.CaptureID, e, in)
	}
	if e.Kind == "period_analysis" {
		return s.reads.periodItem(in.CaptureID, e, in)
	}
	if e.Kind == "contributions" || e.Kind == "historical_overview" || e.Kind == "historical_overview_series" || e.Kind == "account_valuations" || e.Kind == "directory" || e.Kind == "instruments" {
		if e.Kind == "contributions" && strings.TrimSpace(in.ID) == "" {
			return nil, fail("validation", "groupKey must be a non-empty list_contributions row key")
		}
		key := in.Section + "/" + in.ID
		v, ok := e.Details[key]
		if !ok {
			return nil, fail("not_found", "item not found in capture section")
		}
		r := s.reads.base(in.CaptureID, e)
		r.Section, r.Summary = in.Section, v
		names := e.DetailSections[key]
		selected := append([]string(nil), names...)
		if len(in.Sections) > 0 {
			selected = nil
			for _, name := range in.Sections {
				section := name + "/" + in.ID
				if !slices.Contains(names, section) {
					return nil, fail("validation", "unknown child section")
				}
				if !slices.Contains(selected, section) {
					selected = append(selected, section)
				}
			}
		} else if e.Kind == "account_valuations" {
			selected = []string{"components/" + in.ID, "missingInputs/" + in.ID}
		}
		// Reuse the byte-bounded initial-page planner on this same frozen capture.
		focused := e
		focused.Summary, focused.InitialSections = v, selected
		focused.itemSection = in.Section
		for _, section := range names {
			if !slices.Contains(selected, section) {
				focused.AvailableSections = append(focused.AvailableSections, strings.TrimSuffix(section, "/"+in.ID))
			}
		}
		limit, err := readLimit(in.Limit)
		if err != nil {
			return nil, err
		}
		if len(selected) == 0 {
			r.Sections = map[string]ReadSection{}
		} else {
			r, err = s.reads.initial(in.CaptureID, focused, limit)
			if err != nil {
				return nil, err
			}
			r.Section = in.Section
		}
		if ok, err := readFits(r); err != nil {
			return nil, err
		} else if !ok {
			return nil, fail("too_large", "single_item detail exceeds read wire budget")
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
	for _, section := range in.Sections {
		if section != "reservations" {
			return nil, fail("validation", "unknown child section")
		}
	}
	limit, err := readLimit(in.Limit)
	if err != nil {
		return nil, err
	}
	focused := e
	focused.Summary = v
	focused.InitialSections = []string{"reservations/" + in.ID}
	focused.itemSection = in.Section
	return s.reads.initial(in.CaptureID, focused, limit)
}

// focusedChildren projects lazy item sections through the same byte-bounded
// planner as ordinary items. It never stores a new capture or changes its TTL.
func (c *boundedReadCache) focusedChildren(id string, e readCapture, in ReadItemInput, summary any, names []string) (ReadEnvelope, error) {
	limit, err := readLimit(in.Limit)
	if err != nil {
		return ReadEnvelope{}, err
	}
	aliases := make(map[string]string, len(names))
	for _, name := range names {
		aliases[name[strings.LastIndex(name, "/")+1:]] = name
	}
	for _, name := range in.Sections {
		if _, ok := aliases[name]; !ok {
			return ReadEnvelope{}, fail("validation", "unknown child section")
		}
	}
	focused := e
	focused.Summary, focused.itemSection = summary, in.Section
	focused.Sections = make(map[string][]json.RawMessage, len(names))
	focused.InitialSections, focused.AvailableSections = nil, nil
	for _, name := range names {
		alias := name[strings.LastIndex(name, "/")+1:]
		if len(in.Sections) > 0 && !slices.Contains(in.Sections, alias) {
			focused.AvailableSections = append(focused.AvailableSections, alias)
			continue
		}
		rows, ok, err := readSectionRows(e, name)
		if err != nil {
			return ReadEnvelope{}, err
		}
		if !ok {
			return ReadEnvelope{}, fail("not_found", "item evidence section unavailable")
		}
		focused.Sections[name] = rows
	}
	return c.initial(id, focused, limit)
}

func isReadScaleTool(name string) bool {
	switch name {
	case "get_read_page", "get_read_item", "get_market_data", "compare_market_data_import", "update_instruments_batch":
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
func (c *boundedReadCache) outputRows(id string, e readCapture, section string, source []json.RawMessage, start, count int) ([]json.RawMessage, error) {
	rows := source[start : start+count]
	if comparisonSummary(e) && section == "results" {
		return comparisonCompactRows(rows)
	}
	if e.Kind != "market_data" || section != "targets" {
		return rows, nil
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
	return out, nil
}

// Keep revision/CAS integers exact when freezing heterogeneous DTO envelopes.
// Financial amounts remain their existing decimal-string projections.
func decodeReadValue(raw []byte, value any) error {
	decoder := jsoncodec.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(value)
}
