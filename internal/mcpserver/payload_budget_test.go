package mcpserver

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

// Deliberately synthetic assumptions, not sampled from a real household. The
// large tier retains two years of facts but probes bounded windows; setup and
// derived analysis/review preparation are shared by all probes in that tier.
type payloadHousehold struct {
	ledgerFixture
	accounts          []string
	instruments       []string
	origin, timepoint string
}

func newPayloadHousehold(t *testing.T, large bool) payloadHousehold {
	t.Helper()
	total, brokers, perBroker, instrumentCount := 20, 6, 4, 8
	origin := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if large {
		total, brokers, perBroker, instrumentCount = 200, 20, 5, 25
		origin = time.Date(2024, 10, 1, 12, 0, 0, 0, time.UTC)
	}
	now := origin
	app := wailstest.NewService(t)
	app.SetClock(func() time.Time { return now })
	if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "SYNTHETIC payload regression", BaseCurrency: "USD", MemberNames: []string{"Synthetic owner", "Synthetic co-owner"}, Timezone: "UTC", HistoryStartDate: origin.Format(time.DateOnly)}); err != nil {
		t.Fatal(err)
	}
	members, err := app.ListMembers(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	p := payloadHousehold{origin: origin.Format(time.DateOnly), timepoint: "2026-10-12"}
	p.app = app
	currencies := []string{"USD", "EUR", "CNY", "JPY"}
	note := strings.Repeat("Synthetic note with quoted text \"only\" and Unicode 数据. ", 4)
	for i := 0; i < total; i++ {
		mode, kind, role := "balance", "bank_account", "asset"
		currency := currencies[i%len(currencies)]
		if i < brokers {
			mode, kind, currency = "holdings", "brokerage", "USD"
		}
		if i == brokers {
			currency = "USD"
		}
		if i == total-1 {
			kind, role = "credit_card", "liability"
		}
		initial := "0"
		if mode == "holdings" {
			initial = ""
		}
		a, e := app.CreateAccount(t.Context(), application.AccountInput{Name: fmt.Sprintf("Synthetic account %03d / long institution and purpose label", i), AccountType: kind, BalanceSheetRole: role, TrackingMode: mode, DefaultCurrency: currency, InitialAmount: initial, IncludeInNetWorth: true, IncludeInPortfolio: i < brokers, OwnerIDs: []domain.MemberID{members[0].ID}, Note: &note})
		if e != nil {
			t.Fatal(e)
		}
		p.accounts = append(p.accounts, a.Account.ID.String())
	}
	p.brokerage, p.checking, p.savings, p.debt = p.accounts[0], p.accounts[brokers], p.accounts[brokers+1], p.accounts[total-1]
	for i, currency := range currencies[1:] {
		if _, e := app.AppendManualFXQuote(t.Context(), "USD", currency, []string{"0.92", "7.1", "149.2"}[i], origin.Format(time.RFC3339)); e != nil {
			t.Fatal(e)
		}
	}
	quotes := []application.AgentMarketDataItem{}
	for i := 0; i < instrumentCount; i++ {
		currency := currencies[i%len(currencies)]
		instrument, e := app.CreateInstrument(t.Context(), application.InstrumentInput{Name: fmt.Sprintf("Synthetic investment %03d / long descriptive holding name", i), Type: "crypto", QuoteCurrency: currency, QuoteSource: "manual", Note: &note})
		if e != nil {
			t.Fatal(e)
		}
		p.instruments = append(p.instruments, instrument.ID.String())
		quotes = append(quotes, application.AgentMarketDataItem{InstrumentID: instrument.ID.String(), Currency: currency, Kind: "close", Date: p.origin, Timezone: "UTC", Value: "100.12345678", SourceTitle: strings.Repeat("Synthetic provenance ", 5), SourceURL: "https://synthetic.invalid/quote?example=only"})
	}
	p.instrument = p.instruments[0]
	for i := 0; i < brokers; i++ {
		for j := 0; j < perBroker; j++ {
			if _, e := app.CreateHolding(t.Context(), application.HoldingInput{AccountID: p.accounts[i], InstrumentID: p.instruments[(i*perBroker+j)%instrumentCount], Quantity: "2.12345678", UnitCost: "90.12345678"}); e != nil {
				t.Fatal(e)
			}
		}
	}
	if _, e := app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: quotes}); e != nil {
		t.Fatal(e)
	}
	now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		if _, e := history.NewService(app).RecordChange(t.Context(), history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: p.checking, Amount: "123.4567", Currency: "USD", Reason: "income", Note: &note, EffectiveAt: at.Format(time.RFC3339)}); e != nil {
			t.Fatal(e)
		}
	}
	p.service = New(app, t.TempDir(), nil)
	t.Cleanup(p.service.Close)
	return p
}

type payloadMetric struct{ Calls, Bytes, Max int }
type payloadRecorder struct {
	t       *testing.T
	token   string
	mu      sync.Mutex
	metrics map[string]payloadMetric
}

// Observe actual HTTP request and final complete JSON-RPC response bytes. This
// includes SDK text/structuredContent, escaping and metadata; no token estimates.
func (p *payloadRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	name := ""
	if req.Body != nil {
		raw, e := io.ReadAll(req.Body)
		if e != nil {
			return nil, e
		}
		req.Body = io.NopCloser(bytes.NewReader(raw))
		var call financialContextHTTPCall
		if jsoncodec.Unmarshal(raw, &call) == nil {
			name = call.Params.Name
		}
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+p.token)
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if name != "" {
		p.mu.Lock()
		metric := p.metrics[name]
		metric.Calls++
		metric.Bytes += len(raw)
		metric.Max = max(metric.Max, len(raw))
		p.metrics[name] = metric
		p.mu.Unlock()
		budget := contextWireLimit
		// Lower budgets protect normal summary/discovery paths from gradual bloat;
		// continuation and explicit evidence pages retain the complete wire ceiling.
		switch name {
		case "get_catalog":
			budget = 24 << 10
		case "get_context":
			budget = 16 << 10
		case "get_financial_context":
			budget = 28 << 10
		case "get_historical_overview", "get_historical_overview_series":
			budget = 56 << 10
		}
		if len(raw) > budget {
			p.t.Errorf("%s final JSON-RPC response %d bytes exceeds %d-byte profile budget", name, len(raw), budget)
		}
	}
	return res, nil
}
func connectPayload(t *testing.T, s *Service) (*mcp.ClientSession, *payloadRecorder) {
	t.Helper()
	if _, e := s.Enable(LedgerWrite); e != nil {
		t.Fatal(e)
	}
	cfg, e := s.Connection()
	if e != nil {
		t.Fatal(e)
	}
	r := &payloadRecorder{t: t, token: cfg.Token, metrics: map[string]payloadMetric{}}
	c, e := mcp.NewClient(&mcp.Implementation{Name: "synthetic-payload-regression", Version: "1"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: cfg.Endpoint, HTTPClient: &http.Client{Transport: r}, DisableStandaloneSSE: true}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	return c, r
}
func (p *payloadRecorder) total() payloadMetric {
	p.mu.Lock()
	defer p.mu.Unlock()
	total := payloadMetric{}
	for _, m := range p.metrics {
		total.Calls += m.Calls
		total.Bytes += m.Bytes
		total.Max = max(total.Max, m.Max)
	}
	return total
}
func (p *payloadRecorder) journey(name string, start payloadMetric, maxCalls, maxBytes int) {
	p.t.Helper()
	end := p.total()
	calls, bytes := end.Calls-start.Calls, end.Bytes-start.Bytes
	p.t.Logf("journey %s calls=%d final-JSON-RPC-bytes=%d", name, calls, bytes)
	if calls > maxCalls || bytes > maxBytes {
		p.t.Fatalf("journey %s inflated: calls=%d/%d bytes=%d/%d", name, calls, maxCalls, bytes, maxBytes)
	}
}

// Full issue audits request the maximum count; byte-aware sizing still applies.
// This prevents the journey budget from accepting hundreds of tiny pages.
func payloadSection(t *testing.T, c *mcp.ClientSession, cap map[string]any, section string) []any {
	t.Helper()
	part := cap["sections"].(map[string]any)[section].(map[string]any)
	rows := append([]any{}, part["items"].([]any)...)
	page := part["page"].(map[string]any)
	for page["hasMore"] == true {
		next := scaleCapture(t, c, "get_read_page", ReadPageInput{CaptureID: cap["captureId"].(string), Section: section, Cursor: page["nextCursor"].(string), Limit: 100})
		if next["contentHash"] != cap["contentHash"] {
			t.Fatal("mixed issue capture")
		}
		items := next["items"].([]any)
		if len(items) == 0 {
			t.Fatal("non-advancing issue page")
		}
		rows = append(rows, items...)
		page = next["page"].(map[string]any)
	}
	if len(rows) != int(page["total"].(float64)) {
		t.Fatal("omitted issue rows")
	}
	return rows
}
func TestMCPPayloadBudgetsRepresentative(t *testing.T) { runPayloadBudgets(t, false) }
func runPayloadBudgets(t *testing.T, large bool) {
	f := newPayloadHousehold(t, large)
	c, rec := connectPayload(t, f.service)
	tools, err := c.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	discovery, _ := jsoncodec.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": tools})
	if len(discovery) > 160<<10 {
		t.Fatalf("tool discovery grew to %d bytes", len(discovery))
	}
	t.Logf("discovery registered=%d complete-JSON-RPC-bytes=%d", len(tools.Tools), len(discovery))
	// Management and catalog (including note-preserving full item retrieval).
	for name, args := range map[string]any{"get_context": Empty{}, "get_catalog": Empty{}, "list_members": map[string]any{"responseMode": "paged"}, "list_institutions": map[string]any{"responseMode": "paged"}, "list_groups": map[string]any{"responseMode": "paged"}, "list_instruments": map[string]any{"responseMode": "paged"}, "list_holdings": map[string]any{"accountId": f.brokerage, "responseMode": "paged"}, "get_account_snapshot": IDInput{f.brokerage}, "get_market_data": map[string]any{"instrumentId": f.instrument}, "scan_data_health": map[string]any{"responseMode": "paged"}, "preview_data_repair": map[string]any{"responseMode": "paged"}} {
		call(t, c, name, args, false)
	}
	start := rec.total()
	for _, name := range []string{"scan_data_health", "preview_data_repair"} {
		cap := scaleCapture(t, c, name, map[string]any{"responseMode": "paged"})
		for section := range cap["sections"].(map[string]any) {
			payloadSection(t, c, cap, section)
		}
	}
	rec.journey("health and repair preview all sections", start, 25, 1<<20)
	start = rec.total()
	directory := scaleCapture(t, c, "list_accounts", map[string]any{"responseMode": "paged"})
	rows := scaleSection(t, c, directory, "accounts")
	if len(rows) != len(f.accounts) {
		t.Fatal("directory omitted accounts")
	}
	seen := map[string]bool{}
	for _, raw := range rows {
		id := raw.(map[string]any)["id"].(string)
		if seen[id] {
			t.Fatal("duplicate account")
		}
		seen[id] = true
	}
	detail := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: directory["captureId"].(string), Section: "accounts", ID: f.checking})
	if detail["summary"].(map[string]any)["account"].(map[string]any)["note"] == nil {
		t.Fatal("directory detail lost note")
	}
	rec.journey("directory all pages + one item", start, 20, 1<<20)
	// Current account DTO layouts are all reachable through separate sections.
	accountCap := scaleCapture(t, c, "get_account_valuations", map[string]any{"responseMode": "paged", "accountIds": []string{f.brokerage}})
	accountItem := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: accountCap["captureId"].(string), Section: "accounts", ID: f.brokerage})
	for section := range accountItem["sections"].(map[string]any) {
		scaleSection(t, c, accountItem, section)
	}
	// Same-capture historical summary -> all points/leaves -> full item evidence.
	start = rec.total()
	capture := scaleCapture(t, c, "get_historical_overview_series", map[string]any{"from": "2026-09-12", "to": "2026-10-12"})
	leaves := assertHistoricalMatches(t, c, capture)
	if len(scaleSection(t, c, capture, "points")) != 31 {
		t.Fatal("lost closed dates")
	}
	if len(leaves) == 0 {
		t.Fatal("no historical leaves")
	}
	scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: capture["captureId"].(string), Section: "rows", ID: leaves[0].(map[string]any)["id"].(string)})
	point := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: capture["captureId"].(string), Section: "points", ID: f.timepoint})
	scaleSection(t, c, point, "missing/"+f.timepoint)
	rec.journey("31-day history all leaves/points + item", start, 40, 2<<20)
	start = rec.total()
	filtered := scaleCapture(t, c, "get_historical_overview", map[string]any{"date": f.timepoint, "filter": map[string]any{"accountIds": []string{f.brokerage}, "kinds": []string{"holding"}}})
	selected := assertHistoricalMatches(t, c, filtered)
	if len(selected) >= len(leaves) {
		t.Fatal("targeted filter did not reduce rows")
	}
	rec.journey("targeted historical leaves", start, 4, 64<<10)
	// A default summary is useful; its sections and item evidence preserve counts.
	start = rec.total()
	context := decodeContext(t, call(t, c, "get_financial_context", Empty{}, false))
	positions := append([]application.FinancialContextPosition{}, context.Content.Positions...)
	for _, section := range []string{"positions", "gaps", "evidence"} {
		page := context.PositionsPage
		if section == "gaps" {
			page = context.GapsPage
		}
		if section == "evidence" {
			page = context.EvidencePage
		}
		count := page.Returned
		for page.HasMore {
			next := decodeContext(t, call(t, c, "get_financial_context_page", FinancialContextPageInput{ContextID: context.ContextID, Section: section, Cursor: page.NextCursor}, false))
			if next.ContentHash != context.ContentHash {
				t.Fatal("mixed financial context")
			}
			switch section {
			case "positions":
				positions = append(positions, next.Content.Positions...)
				page = next.PositionsPage
			case "gaps":
				page = next.GapsPage
			case "evidence":
				page = next.EvidencePage
			}
			if page.Returned == 0 {
				t.Fatal("context did not advance")
			}
			count += page.Returned
		}
		if count != page.Total {
			t.Fatal("omitted context section")
		}
	}
	seenPositions := map[string]bool{}
	if context.Content.Scope.AccountCount != len(f.accounts) {
		t.Fatal("context omitted account scope")
	}
	pending := 0
	for _, p := range positions {
		if seenPositions[p.Ref] {
			t.Fatal("duplicate context position")
		}
		seenPositions[p.Ref] = true
		pending += p.PendingContribution
	}
	if pending != context.Content.Summary.DisplayValuation.NetWorth.Quality.CarriedCount {
		t.Fatal("context pending total mismatched")
	}
	call(t, c, "get_financial_context_item", FinancialContextItemInput{ContextID: context.ContextID, Ref: positions[0].Ref}, false)
	rec.journey("financial context all sections + item", start, 40, 2<<20)
	// Local market histories and Agent provenance use their bounded entrances.
	market := scaleCapture(t, c, "get_market_data_batch", map[string]any{"targets": []any{map[string]any{"instrumentId": f.instrument}, map[string]any{"baseCurrency": "USD", "quoteCurrency": "EUR"}}, "view": "history", "range": "all"})
	targets := scaleSection(t, c, market, "targets")
	if len(targets) != 2 {
		t.Fatal("market targets lost")
	}
	for _, raw := range targets {
		v := raw.(map[string]any)
		if sections, ok := v["sections"].(map[string]any); ok {
			target := map[string]any{"captureId": market["captureId"], "contentHash": market["contentHash"], "sections": sections}
			for section := range sections {
				scaleSection(t, c, target, section)
			}
		}
	}
	agents := scaleCapture(t, c, "list_agent_market_data", map[string]any{"responseMode": "paged"})
	scaleSection(t, c, agents, "records")
	// Activity lists use their native three-part keyset and complete item reads.
	activities := call(t, c, "list_activities", map[string]any{"accountId": f.checking, "limit": 5}, false)["data"].(map[string]any)
	activityRows := activities["activities"].([]any)
	if len(activityRows) == 0 {
		t.Fatal("activity fixture is empty")
	}
	call(t, c, "get_activity", IDInput{activityRows[0].(map[string]any)["id"].(string)}, false)
	if cursor, ok := activities["next"].(map[string]any); ok {
		call(t, c, "list_activities", map[string]any{"accountId": f.checking, "limit": 5, "afterEffectiveAt": cursor["effectiveAt"], "afterCreatedAt": cursor["createdAt"], "afterId": cursor["id"]}, false)
	}
	if !large {
		call(t, c, "get_overview", Empty{}, false)
	}
	// Ledger previews/commits and managed contracts run only in this synthetic DB.
	plan := ledgerPreview(t, c, map[string]any{"kind": "money_added", "accountId": f.checking, "amount": "1.2345", "currency": "USD", "reason": "income", "effectiveAt": "2026-10-12T12:00:00Z"})
	op := uuid.NewString()
	ledgerCommit(t, c, op, plan["planId"].(string))
	call(t, c, "get_operation", IDInput{op}, false)
	product, _ := productDataSeed(t, f.ledgerFixture, c, "locked_product")
	products := scaleCapture(t, c, "list_products", map[string]any{"responseMode": "paged"})
	scaleSection(t, c, products, "products")
	call(t, c, "get_product", map[string]any{"id": product, "responseMode": "paged"}, false)
	call(t, c, "list_product_operations", map[string]any{"productId": product}, false)
	liquidity := scaleCapture(t, c, "get_liquidity_overview", map[string]any{"responseMode": "paged"})
	scaleSection(t, c, liquidity, "sources")
	upcoming := scaleCapture(t, c, "get_upcoming_actions", map[string]any{"responseMode": "paged"})
	scaleSection(t, c, upcoming, "items")
	scaleSection(t, c, upcoming, "dataChecks")
	session := sessionAction(t, c, map[string]any{"action": "start", "accountIds": []string{f.checking}})
	call(t, c, "list_reconciliation_sessions", Empty{}, false)
	call(t, c, "get_reconciliation_session", IDInput{session["id"].(string)}, false)
	checked := call(t, c, "get_reconciliation_session_page", map[string]any{"sessionId": session["id"], "section": "items"}, false)["data"].(map[string]any)
	checkItem := checked["items"].([]any)[0].(map[string]any)["item"].(map[string]any)
	call(t, c, "get_reconciliation_item", map[string]any{"sessionId": session["id"], "accountId": f.checking, "itemId": checkItem["id"]}, false)
	// Analysis work is narrow and done once; other probes reuse those snapshots.
	query := map[string]any{"from": "2026-10-01", "to": "2026-10-02", "scopeKind": "account", "scopeId": f.checking}
	call(t, c, "analyze_period", map[string]any{"query": query}, false)
	contributionQuery := map[string]any{"from": "2026-10-01", "to": "2026-10-02", "scopeKind": "account", "scopeId": f.brokerage}
	contributions := call(t, c, "list_contributions", map[string]any{"query": contributionQuery, "returnType": "total_return", "groupBy": "instrument", "limit": 5}, false)["data"].(map[string]any)
	if rows := contributions["rows"].([]any); len(rows) > 0 {
		call(t, c, "get_contribution_item", map[string]any{"query": contributionQuery, "returnType": "total_return", "groupBy": "instrument", "groupKey": rows[0].(map[string]any)["key"]}, false)
	}
	call(t, c, "get_return_day", map[string]any{"query": query, "date": "2026-10-02"}, false)
	call(t, c, "get_asset_driver_detail", map[string]any{"query": query, "driverKey": "external_flow"}, false)
	comparisonArgs := map[string]any{"leftAsOf": "2026-10-01", "rightAsOf": "2026-10-02", "scope": map[string]any{"kind": "accounts", "accountIds": []string{f.checking}}}
	comparison := call(t, c, "compare_financial_context", comparisonArgs, false)["data"].(map[string]any)
	page := comparison["positionsPage"].(map[string]any)
	if cursor, ok := page["nextCursor"].(string); ok && cursor != "" {
		call(t, c, "get_financial_comparison_page", map[string]any{"comparisonId": comparison["comparisonId"], "section": "positions", "cursor": cursor}, false)
	}
	call(t, c, "compare_financial_attribution", comparisonArgs, false)
	// Two rules, one explicitly selected investment, real persisted review results.
	spec := testReviewV2Spec()
	spec.Target = domain.ReviewRuleTarget{Kind: "instruments", InstrumentIDs: []domain.InstrumentID{domain.InstrumentID(f.instrument)}}
	var object map[string]any
	raw, _ := jsoncodec.Marshal(spec)
	if jsoncodec.Unmarshal(raw, &object) != nil {
		t.Fatal("spec")
	}
	var ruleID string
	for i := 0; i < 2; i++ {
		ruleID = uuid.NewString()
		call(t, c, "create_investment_review_rule", map[string]any{"operationId": uuid.NewString(), "input": ReviewV2RuleInput{RuleID: ruleID, MutationID: uuid.NewString(), Name: fmt.Sprintf("Synthetic named review rule %d", i), Enabled: true, Severity: "notice", Spec: object}}, false)
	}
	call(t, c, "preview_investment_review_rule", ReviewV2PreviewInput{Spec: object, InstrumentID: f.instrument, LatestClosedOn: f.timepoint}, false)
	call(t, c, "refresh_investment_reviews", map[string]any{"operationId": uuid.NewString(), "input": Empty{}}, false)
	call(t, c, "list_investment_review_rules", ReviewV2PageInput{PageSize: 10}, false)
	alerts := call(t, c, "list_investment_review_alerts", ReviewV2AlertPageInput{Filter: domain.ReviewAlertFilter{Status: "all"}, PageSize: 5}, false)["data"].(map[string]any)
	coverage := call(t, c, "get_investment_review_coverage", ReviewV2CoverageInput{Filter: domain.ReviewCoverageFilter{Status: "all"}, PageSize: 5}, false)["data"].(map[string]any)
	coverageRevision := coverage["reportRevision"]
	seenCoverage := map[string]bool{}
	for {
		for _, raw := range coverage["items"].([]any) {
			id := raw.(map[string]any)["instrumentId"].(string)
			if seenCoverage[id] {
				t.Fatal("duplicate review group")
			}
			seenCoverage[id] = true
		}
		cursor, _ := coverage["nextCursor"].(string)
		if cursor == "" {
			break
		}
		coverage = call(t, c, "get_investment_review_coverage", ReviewV2CoverageInput{Filter: domain.ReviewCoverageFilter{Status: "all"}, PageSize: 5, Cursor: cursor}, false)["data"].(map[string]any)
		if coverage["reportRevision"] != coverageRevision {
			t.Fatal("mixed review publication")
		}
	}
	if len(seenCoverage) < len(f.instruments) {
		t.Fatal("review coverage omitted investments")
	}
	call(t, c, "list_investment_review_suppressions", ReviewV2PageInput{PageSize: 10}, false)
	call(t, c, "list_investment_review_episode_history", ReviewV2HistoryInput{RuleID: ruleID, PageSize: 5}, false)
	if items := alerts["items"].([]any); len(items) > 0 {
		episode := items[0].(map[string]any)["rules"].([]any)[0].(map[string]any)["episodeId"].(string)
		call(t, c, "get_investment_review_alert", ReviewV2AlertInput{EpisodeID: episode}, false)
		call(t, c, "list_investment_review_daily_evidence", ReviewV2EvidenceInput{EpisodeID: episode, PageSize: 5}, false)
		call(t, c, "list_investment_review_episode_events", ReviewV2EventInput{EpisodeID: episode, PageSize: 5}, false)
	} else {
		t.Fatal("review fixture must include a real published alert")
	}
	rec.mu.Lock()
	names := []string{}
	for name := range rec.metrics {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		m := rec.metrics[name]
		t.Logf("payload %s calls=%d bytes=%d max=%d", name, m.Calls, m.Bytes, m.Max)
	}
	rec.mu.Unlock()
	t.Logf("synthetic large=%v accounts=%d instruments=%d covered-tools=%d total=%+v", large, len(f.accounts), len(f.instruments), len(names), rec.total())
}
