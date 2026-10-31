package mobileapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func decode(w http.ResponseWriter, r *http.Request, out any) error {
	d := jsoncodec.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return invalid("body")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return invalid("body")
	}
	return nil
}
func reply(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = jsoncodec.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	code, field, status := "unavailable", "", http.StatusServiceUnavailable
	var de *domain.Error
	if errors.As(err, &de) {
		code = string(de.Code)
		field = de.Field
		switch field {
		case "session":
			code = "session_expired"
			status = 401
		case "permission":
			code = "forbidden"
			status = 403
		case "rateLimit":
			code = "rate_limited"
			status = 429
		default:
			switch de.Code {
			case domain.ErrStalePreview, domain.ErrConflict:
				status = 409
			case domain.ErrNotFound:
				status = 404
			case domain.ErrBackupRestoreBusy, domain.ErrUnavailable:
				status = 503
			default:
				status = 422
			}
		}
	}
	state := "unknown"
	if code == "stale_preview" && field == "previewToken" {
		state = "rejected"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	reply(w, map[string]any{"error": map[string]any{"code": code, "field": field, "retryable": status == 503 || status == 429, "operationState": state, "requestId": requestID}})
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	name := "mobile.html"
	if r.URL.Path != "/" && r.URL.Path != "/mobile.html" {
		name = strings.TrimPrefix(r.URL.Path, "/")
		if !strings.HasPrefix(name, "assets/") || !fs.ValidPath(name) || (path.Ext(name) != ".js" && path.Ext(name) != ".css" && path.Ext(name) != ".woff2") {
			http.NotFound(w, r)
			return
		}
	}
	if s.assets == nil {
		http.NotFound(w, r)
		return
	}
	raw, err := fs.ReadFile(s.assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && (path.Ext(name) == ".js" || path.Ext(name) == ".css") {
		w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		writer := gzip.NewWriter(w)
		_, _ = writer.Write(raw)
		_ = writer.Close()
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(raw))
}

type pageState struct {
	owner, version, filter string
	index                  int
	after                  *domain.ActivityCursor
	expires                time.Time
}

func (s *Server) pagination(r *http.Request, d *session) (pageState, int, error) {
	limit := 30
	if text := r.URL.Query().Get("limit"); text != "" {
		n, err := strconv.Atoi(text)
		if err != nil || n < 1 || n > 100 {
			return pageState{}, 0, invalid("limit")
		}
		limit = n
	}
	q := r.URL.Query()
	q.Del("cursor")
	fingerprint := r.URL.Path + "?" + q.Encode()
	state := pageState{owner: d.ID, version: s.app.MobileDataVersion(), filter: fingerprint, expires: s.now().Add(10 * time.Minute)}
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		s.mu.RLock()
		saved, ok := s.pages[cursor]
		s.mu.RUnlock()
		if !ok || saved.owner != d.ID || saved.filter != fingerprint || saved.version != state.version || s.now().After(saved.expires) {
			return pageState{}, 0, &domain.Error{Code: domain.ErrConflict, Field: "cursor", Message: "cursor_stale"}
		}
		state = saved
	}
	return state, limit, nil
}
func (s *Server) nextPage(p pageState) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.pages {
		if s.now().After(v.expires) {
			delete(s.pages, k)
		}
	}
	if len(s.pages) >= 100 {
		for k := range s.pages {
			delete(s.pages, k)
			break
		}
	}
	key := uuid.NewString()
	p.version = s.app.MobileDataVersion()
	s.pages[key] = p
	return key
}
func (s *Server) envelope(ctx context.Context, data any) map[string]any {
	tz := "UTC"
	if origin, err := s.app.HistoryOrigin(ctx); err == nil && origin != nil {
		tz = origin.Timezone
	}
	return map[string]any{"asOf": s.now(), "dataVersion": s.app.MobileDataVersion(), "ledgerTimezone": tz, "data": data}
}

func (s *Server) route(w http.ResponseWriter, r *http.Request, d *session, p string) {
	var result any
	var err error
	if r.Method == "GET" {
		version, e := s.app.MobileReadSnapshot(r.Context(), func(ctx context.Context) error { var e error; result, e = s.read(ctx, r, d, p); return e })
		err = e
		if envelope, ok := result.(map[string]any); ok {
			envelope["dataVersion"] = version
			if data, ok := envelope["data"].(map[string]any); ok {
				if next, ok := data["next"].(string); ok && next != "" {
					s.mu.Lock()
					page := s.pages[next]
					page.version = version
					s.pages[next] = page
					s.mu.Unlock()
				}
			}
		}
	} else if r.Method == "POST" {
		if strings.HasPrefix(p, "/market-jobs/") && strings.HasSuffix(p, "/cancel") {
			var in struct{}
			if err = decode(w, r, &in); err == nil {
				var release func()
				release, err = s.authorize(d, "market_refresh")
				if err == nil {
					id := strings.TrimSuffix(strings.TrimPrefix(p, "/market-jobs/"), "/cancel")
					result = map[string]bool{"requested": s.app.RequestCancelMobileMarket(id)}
					release()
				}
			}
			if err != nil {
				fail(w, err)
			} else {
				reply(w, result)
			}
			return
		}
		switch p {
		case "/record-previews":
			if !d.Permissions.Record {
				err = forbidden()
				break
			}
			var in application.MobileRecordInput
			if err = decode(w, r, &in); err != nil {
				break
			}
			var reviewed *application.MobileRecordPlan
			reviewed, err = s.app.PreviewMobileRecord(r.Context(), in)
			if err == nil {
				id, e := s.savePlan(&plan{owner: d.ID, operation: in.OperationID, record: reviewed, expires: reviewed.ExpiresAt, state: "previewed"})
				err = e
				result = map[string]any{"planId": id, "operationId": in.OperationID, "expiresAt": reviewed.ExpiresAt, "timezone": reviewed.Timezone, "kind": reviewed.Kind, "preview": wire.FromChangePreview(reviewed.Preview), "before": wire.FromEndpointViews(reviewed.Before)}
			}
		case "/quote-previews":
			if !d.Permissions.Manual {
				err = forbidden()
				break
			}
			var in application.MobileQuoteInput
			if err = decode(w, r, &in); err != nil {
				break
			}
			var reviewed *application.MobileQuotePlan
			reviewed, err = s.app.PreviewMobileQuote(r.Context(), in, d.ID)
			if err == nil {
				id, e := s.savePlan(&plan{owner: d.ID, operation: in.OperationID, quote: reviewed, expires: reviewed.ExpiresAt, state: "previewed"})
				err = e
				result = map[string]any{"planId": id, "operationId": in.OperationID, "expiresAt": reviewed.ExpiresAt, "receipt": reviewed.Receipt}
			}
		case "/market-previews":
			if !d.Permissions.Market {
				err = forbidden()
				break
			}
			var in application.MobileMarketRequest
			if err = decode(w, r, &in); err != nil {
				break
			}
			var preview *application.SyncPlanPreview
			preview, err = s.app.PreviewMobileMarket(r.Context(), in)
			target := ""
			if err == nil && in.Scope == "instrument" {
				instruments, e := s.app.ListInstruments(r.Context(), false)
				err = e
				for _, i := range instruments {
					if i.ID.String() == in.InstrumentID {
						target = i.DisplayLabel()
						break
					}
				}
				if err == nil && target == "" {
					err = &domain.Error{Code: domain.ErrNotFound}
				}
			}
			if in.Scope == "fx" {
				target = in.CurrencyA + " / " + in.CurrencyB
			}
			if err == nil {
				id, e := s.savePlan(&plan{owner: d.ID, market: &in, expires: s.now().Add(10 * time.Minute), state: "previewed"})
				err = e
				result = map[string]any{"planId": id, "request": in, "historicalPlan": preview, "targetLabel": target}
			}
		case "/record-confirmations", "/quote-confirmations", "/market-jobs":
			result, err = s.confirm(w, r, d, p)
		default:
			http.NotFound(w, r)
			return
		}
	} else {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, result)
}

func (s *Server) savePlan(p *plan) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, old := range s.plans {
		if s.now().After(old.expires) {
			delete(s.plans, id)
		}
	}
	if len(s.plans) >= 100 {
		return "", limited()
	}
	id := uuid.NewString()
	s.plans[id] = p
	return id, nil
}

func (s *Server) confirm(w http.ResponseWriter, r *http.Request, d *session, route string) (any, error) {
	var in struct {
		PlanID      string `json:"planId"`
		OperationID string `json:"operationId,omitempty"`
	}
	if err := decode(w, r, &in); err != nil {
		return nil, err
	}
	s.mu.RLock()
	p := s.plans[in.PlanID]
	s.mu.RUnlock()
	if p == nil || p.owner != d.ID {
		return nil, &domain.Error{Code: domain.ErrStalePreview, Field: "planId"}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	permission := "record"
	if route == "/quote-confirmations" {
		permission = "manual_quote"
	}
	if route == "/market-jobs" {
		permission = "market_refresh"
	}
	if route == "/record-confirmations" && p.record == nil || route == "/quote-confirmations" && p.quote == nil || route == "/market-jobs" && p.market == nil {
		return nil, invalid("planId")
	}
	if p.operation != in.OperationID {
		return nil, &domain.Error{Code: domain.ErrConflict, Field: "operationId"}
	}
	// Request cancellation does not interrupt an accepted financial commit.
	// HTTP status queries read persistent receipts after a lost response.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer cancel()
	var result any
	err := s.app.MobileAuthorizedWrite(ctx, func() (func(), error) { return s.authorize(d, permission) }, func(ctx context.Context) error {
		p.state = "submitting"
		if p.record != nil {
			ctx = domain.WithMobileAudit(ctx, domain.MobileAudit{DeviceID: d.ID, OperationID: p.operation, RequestID: w.Header().Get("X-Request-ID"), Kind: p.record.Kind})
			receipt, err := s.app.ConfirmMobileRecord(ctx, p.record)
			if err != nil {
				return err
			}
			p.state = "committed"
			result = map[string]any{"operationState": "committed", "operationId": p.operation, "receipt": wire.FromChangePreview(receipt)}
			return nil
		}
		if p.quote != nil {
			receipt, err := s.app.ConfirmMobileQuote(ctx, p.quote)
			if err != nil {
				return err
			}
			p.state = "committed"
			result = map[string]any{"operationState": "committed", "operationId": p.operation, "receipt": receipt}
			return nil
		}
		if p.marketResult != nil {
			result = p.marketResult
			p.state = "accepted"
			return nil
		}
		if s.now().After(p.expires) {
			return &domain.Error{Code: domain.ErrStalePreview, Field: "previewToken"}
		}
		job, attached, conflict, err := s.app.StartMobileMarket(ctx, *p.market)
		if err != nil {
			return err
		}
		result = map[string]any{"job": mobileJobDTO(job, 0, 30), "attached": attached, "conflict": conflict}
		p.state = "accepted"
		if conflict {
			p.state = "rejected"
		} else {
			p.marketResult = result.(map[string]any)
		}
		return nil
	})
	if err != nil {
		p.state = "unknown"
		var e *domain.Error
		if errors.As(err, &e) && e.Code == domain.ErrStalePreview && e.Field == "previewToken" {
			p.state = "rejected"
		}
	}
	slog.Info("mobile operation", "device", d.ID, "operation", p.operation, "kind", permission, "state", p.state, "failed", err != nil)
	return result, err
}

type accountRow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Institution string          `json:"institution"`
	Currency    string          `json:"currency"`
	Type        string          `json:"type"`
	Role        string          `json:"role"`
	Mode        string          `json:"mode"`
	Balance     *wire.MoneyView `json:"balance"`
	Value       *wire.MoneyView `json:"value"`
	Complete    bool            `json:"complete"`
}
type holdingRow struct {
	ID            string                `json:"id"`
	AccountID     string                `json:"accountId"`
	InstrumentID  string                `json:"instrumentId"`
	Name          string                `json:"name"`
	Symbol        string                `json:"symbol"`
	Quantity      string                `json:"quantity"`
	Value         *wire.MoneyView       `json:"value"`
	Cost          *wire.MoneyView       `json:"cost"`
	Gain          *wire.SignedMoneyView `json:"gain"`
	MissingReason string                `json:"missingReason"`
}

func (s *Server) read(ctx context.Context, r *http.Request, d *session, p string) (any, error) {
	q := r.URL.Query()
	switch {
	case p == "/bootstrap":
		b, err := s.app.Bootstrap(ctx)
		if err != nil {
			return nil, err
		}
		if b.Household == nil {
			return nil, &domain.Error{Code: domain.ErrUnavailable}
		}
		origin, err := s.app.HistoryOrigin(ctx)
		if err != nil {
			return nil, err
		}
		tz := "UTC"
		date, timeText := "", ""
		started := origin != nil
		if started {
			tz = origin.Timezone
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil, err
		}
		now := s.now().In(loc)
		date = now.Format("2006-01-02")
		timeText = now.Format("15:04:05")
		return s.envelope(ctx, map[string]any{"householdId": b.Household.ID, "name": b.Household.Name, "currency": b.Household.BaseCurrency.String(), "historyStarted": started, "today": date, "time": timeText, "capabilities": d.Permissions, "instance": s.app.MobileWorkspace()}), nil
	case p == "/overview":
		o, err := s.app.Overview(ctx, domain.AccountFilter{})
		if err != nil {
			return nil, err
		}
		rangeText := q.Get("range")
		if rangeText == "" {
			rangeText = "30d"
		}
		trendRange, err := domain.ParseTrendRange(rangeText)
		if err != nil {
			return nil, err
		}
		trend, err := s.app.PortfolioTrend(ctx, trendRange)
		if err != nil && o.HistoryStarted {
			return nil, err
		}
		points := []map[string]any{}
		for _, point := range trend.Points {
			points = append(points, map[string]any{"date": point.LocalDate, "valuation": wire.FromValuationAmount(point.DisplayValuation), "complete": point.Complete})
		}
		if len(points) > 90 {
			stride := (len(points) + 88) / 89
			bounded := []map[string]any{}
			for i := 0; i < len(points)-1; i += stride {
				bounded = append(bounded, points[i])
			}
			points = append(bounded, points[len(points)-1])
		}
		recent := o.RecentActivities
		if len(recent) > 3 {
			recent = recent[:3]
		}
		return s.envelope(ctx, map[string]any{"currency": o.Currency.String(), "valuation": wire.FromHouseholdDisplay(o.DisplayValuation), "complete": o.Complete, "distribution": wire.FromBreakdowns(o.AssetsByType), "trend": points, "recent": s.activityDTOs(ctx, recent)}), nil
	case p == "/accounts":
		page, limit, err := s.pagination(r, d)
		if err != nil {
			return nil, err
		}
		values, err := s.app.AccountValuations(ctx, domain.AccountFilter{})
		if err != nil {
			return nil, err
		}
		rows := []accountRow{}
		needle := strings.ToLower(q.Get("q"))
		for _, v := range values {
			a := v.Account
			if a.ArchivedAt != nil || needle != "" && !strings.Contains(strings.ToLower(a.Name+" "+v.InstitutionName), needle) || q.Get("role") != "" && q.Get("role") != string(a.BalanceSheetRole) || q.Get("type") != "" && q.Get("type") != string(a.AccountType) {
				continue
			}
			rows = append(rows, accountRow{ID: a.ID.String(), Name: a.Name, Institution: v.InstitutionName, Currency: a.DefaultCurrency.String(), Type: string(a.AccountType), Role: string(a.BalanceSheetRole), Mode: string(a.TrackingMode), Value: wire.FromMoneyView(v.BaseValue), Complete: v.Complete})
		}
		start := page.index
		end := min(start+limit, len(rows))
		if start > end {
			return nil, invalid("cursor")
		}
		next := ""
		if end < len(rows) {
			page.index = end
			next = s.nextPage(page)
		}
		return s.envelope(ctx, map[string]any{"items": rows[start:end], "next": next}), nil
	case strings.HasPrefix(p, "/accounts/"):
		id, err := domain.ParseAccountID(strings.TrimPrefix(p, "/accounts/"))
		if err != nil {
			return nil, err
		}
		v, err := s.app.AccountValuation(ctx, id)
		if err != nil {
			return nil, err
		}
		return s.envelope(ctx, wire.FromAccountValuation(v)), nil
	case p == "/holdings":
		page, limit, err := s.pagination(r, d)
		if err != nil {
			return nil, err
		}
		accounts, err := s.app.ListAccounts(ctx, domain.AccountFilter{})
		if err != nil {
			return nil, err
		}
		ids := []domain.AccountID{}
		for _, a := range accounts {
			if q.Get("accountId") == "" || q.Get("accountId") == a.Account.ID.String() {
				ids = append(ids, a.Account.ID)
			}
		}
		if q.Get("accountId") != "" && len(ids) == 0 {
			return nil, &domain.Error{Code: domain.ErrNotFound}
		}
		gains, err := s.app.AccountGains(ctx, ids)
		if err != nil {
			return nil, err
		}
		rows := []holdingRow{}
		for _, a := range gains {
			for _, g := range a.Holdings {
				if needle := strings.ToLower(q.Get("q")); needle != "" && !strings.Contains(strings.ToLower(g.InstrumentName+" "+g.InstrumentSymbol), needle) {
					continue
				}
				rows = append(rows, holdingRow{ID: g.HoldingID.String(), AccountID: g.AccountID.String(), InstrumentID: g.InstrumentID.String(), Name: g.InstrumentName, Symbol: g.InstrumentSymbol, Quantity: g.Quantity, Value: wire.FromMoneyView(g.CurrentValue), Cost: wire.FromMoneyView(g.TotalCost), Gain: wire.FromSignedMoneyView(g.UnrealizedGain), MissingReason: g.MissingReason})
			}
		}
		start := page.index
		end := min(start+limit, len(rows))
		if start > end {
			return nil, invalid("cursor")
		}
		next := ""
		if end < len(rows) {
			page.index = end
			next = s.nextPage(page)
		}
		return s.envelope(ctx, map[string]any{"items": rows[start:end], "next": next}), nil
	case strings.HasPrefix(p, "/holdings/"):
		id, err := domain.ParseHoldingID(strings.TrimPrefix(p, "/holdings/"))
		if err != nil {
			return nil, err
		}
		g, err := s.app.HoldingGain(ctx, id)
		if err != nil {
			return nil, err
		}
		v, err := s.app.AccountValuation(ctx, g.AccountID)
		if err != nil {
			return nil, err
		}
		quote, err := s.app.CurrentInstrumentQuote(ctx, g.InstrumentID)
		if err != nil {
			return nil, err
		}
		return s.envelope(ctx, map[string]any{"holding": g, "valuation": wire.FromAccountValuation(v), "quote": wire.FromInstrumentQuotePtr(quote)}), nil
	case p == "/activities":
		page, limit, err := s.pagination(r, d)
		if err != nil {
			return nil, err
		}
		query := domain.ActivityQuery{Limit: limit, After: page.after, FromLocalDate: q.Get("from"), ToLocalDate: q.Get("to")}
		if q.Get("accountId") != "" {
			id, err := domain.ParseAccountID(q.Get("accountId"))
			if err != nil {
				return nil, err
			}
			query.AccountID = &id
		}
		if q.Get("kind") != "" {
			query.Kinds = []domain.ActivityKind{domain.ActivityKind(q.Get("kind"))}
		}
		result, err := s.app.ListActivityPage(ctx, query)
		if err != nil {
			return nil, err
		}
		next := ""
		if result.HasMore {
			page.after = result.Next
			next = s.nextPage(page)
		}
		return s.envelope(ctx, map[string]any{"items": s.activityDTOs(ctx, result.Activities), "next": next}), nil
	case strings.HasPrefix(p, "/activities/"):
		id, err := domain.ParseActivityID(strings.TrimPrefix(p, "/activities/"))
		if err != nil {
			return nil, err
		}
		a, err := s.app.Activity(ctx, id)
		if err != nil {
			return nil, err
		}
		return s.envelope(ctx, s.activityDTOs(ctx, []domain.Activity{a})[0]), nil
	case p == "/health":
		page, limit, err := s.pagination(r, d)
		if err != nil {
			return nil, err
		}
		s.mu.RLock()
		report := s.health
		version := s.healthVersion
		scan := s.healthScan
		at := s.healthAt
		s.mu.RUnlock()
		if report == nil || version != s.app.MobileDataVersion() || page.index == 0 && page.after == nil && (q.Get("scan") != scan || s.now().Sub(at) > 30*time.Second) {
			fresh, err := s.app.ScanMarketDataHealth(ctx)
			if err != nil {
				return nil, err
			}
			report = &fresh
			s.mu.Lock()
			s.health = report
			s.healthVersion = s.app.MobileDataVersion()
			s.healthScan = q.Get("scan")
			s.healthAt = s.now()
			s.mu.Unlock()
		}
		start := page.index
		end := min(start+limit, len(report.Issues))
		if start > end {
			return nil, invalid("cursor")
		}
		next := ""
		if end < len(report.Issues) {
			page.index = end
			next = s.nextPage(page)
		}
		items := append([]application.HealthIssue{}, report.Issues[start:end]...)
		return s.envelope(ctx, map[string]any{"healthy": report.Healthy, "count": report.IssueCount, "items": items, "next": next}), nil
	case strings.HasPrefix(p, "/operations/"):
		id := strings.TrimPrefix(p, "/operations/")
		if d.Permissions.Record {
			receipt, err := s.app.LookupMobileRecord(ctx, id)
			if err != nil {
				return nil, err
			}
			if receipt != nil {
				return map[string]any{"operationId": id, "operationState": "committed", "receipt": wire.FromChangePreview(*receipt)}, nil
			}
		}
		if d.Permissions.Manual {
			receipt, err := s.app.LookupMobileQuote(ctx, id)
			if err != nil {
				return nil, err
			}
			if receipt != nil {
				return map[string]any{"operationId": id, "operationState": "committed", "receipt": receipt}, nil
			}
		}
		if !d.Permissions.Record && !d.Permissions.Manual {
			return nil, forbidden()
		}
		return map[string]any{"operationId": id, "operationState": "unknown"}, nil
	case strings.HasPrefix(p, "/market-jobs/"):
		if !d.Permissions.Market {
			return nil, forbidden()
		}
		job, ok := s.app.MobileMarketJob(strings.TrimPrefix(p, "/market-jobs/"))
		if !ok {
			return map[string]any{"job": nil, "operationState": "unknown", "instance": s.app.MobileWorkspace()}, nil
		}
		// A cursor issued by /current must bind the resolved task, so the next
		// page can use its stable ID even when another task becomes current.
		pageRequest := r.Clone(r.Context())
		pageRequest.URL.Path = prefix + "/market-jobs/" + job.ID
		page, limit, err := s.pagination(pageRequest, d)
		if err != nil {
			return nil, err
		}
		next := ""
		if page.index+limit < max(len(job.Items), len(job.Result.Items), len(job.Blockers)) {
			advance := page
			advance.index += limit
			next = s.nextPage(advance)
		}
		return map[string]any{"job": mobileJobDTO(job, page.index, limit), "next": next}, nil
	default:
		return nil, &domain.Error{Code: domain.ErrNotFound}
	}
}

// Safe, bounded task details. Provider messages, workspace paths and raw logs
// never cross the mobile boundary. Persisted acceptance is not completion.
func mobileJobDTO(job application.MobileMarketJob, start, limit int) map[string]any {
	items := []map[string]string{}
	begin := min(start, len(job.Items))
	end := min(start+limit, len(job.Items))
	for _, item := range job.Items[begin:end] {
		items = append(items, map[string]string{"label": item.Label, "status": item.Status, "kind": item.Kind, "startDate": item.StartDate, "endDate": item.EndDate, "errorCode": safeJobCode(item.Detail)})
	}
	result := job.Result
	begin = min(start, len(result.Items))
	result.Items = append([]application.RefreshTargetResult{}, result.Items[begin:min(begin+limit, len(result.Items))]...)
	current := ""
	if job.Current != nil {
		current = job.Current.Label
	}
	blockers := []string{}
	begin = min(start, len(job.Blockers))
	for _, blocker := range job.Blockers[begin:min(begin+limit, len(job.Blockers))] {
		code := safeJobCode(blocker.Code)
		if code == "" {
			code = "unavailable"
		}
		blockers = append(blockers, code)
	}
	return map[string]any{"jobId": job.ID, "instance": job.Instance, "request": job.Request, "outcome": job.Outcome, "phase": job.Phase, "startedAt": job.StartedAt, "finishedAt": job.FinishedAt, "completed": job.Completed, "total": job.Total, "result": result, "items": items, "current": current, "nextEligibilityAt": job.NextEligibilityAt, "errorCode": safeJobCode(job.ErrorCode), "blockers": blockers, "source": job.Source, "retryable": job.Retryable, "cancellable": job.Cancellable && job.Outcome == application.SyncOutcomeRunning}
}

// Never send provider message text, paths or arbitrary Detail strings.
func safeJobCode(code string) string {
	switch domain.ErrorCode(code) {
	case domain.ErrProviderUnavailable, domain.ErrProviderAuthentication, domain.ErrProviderRateLimit, domain.ErrUnavailable, domain.ErrConflict, domain.ErrValidation, domain.ErrNotFound, domain.ErrStalePreview, domain.ErrUnsupportedProviderSymbol, domain.ErrMalformedProviderResponse, domain.ErrMarketDataResponseTooLarge, domain.ErrHistoryNotStarted, domain.ErrHistoryUpdateFailed, domain.ErrBackupRestoreBusy:
		return code
	}
	return ""
}

// Activities contain authoritative effects; readable labels come from current
// directory metadata. Missing metadata remains unnamed rather than leaking IDs.
func (s *Server) activityDTOs(ctx context.Context, values []domain.Activity) []wire.ActivityDTO {
	accounts, _ := s.app.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	instruments, _ := s.app.ListInstruments(ctx, true)
	names := map[string]string{}
	instrumentNames := map[string]string{}
	for _, a := range accounts {
		names[a.Account.ID.String()] = a.Account.Name
	}
	for _, i := range instruments {
		instrumentNames[i.ID.String()] = i.Name
	}
	result := wire.FromActivities(values)
	for ai := range result {
		for ri := range result[ai].Resulting {
			r := &result[ai].Resulting[ri]
			if r.AccountID != nil {
				if name := names[*r.AccountID]; name != "" {
					r.Name = name
				}
			}
			if r.HoldingID != nil {
				for _, e := range result[ai].Effects {
					if e.HoldingID != nil && *e.HoldingID == *r.HoldingID && e.InstrumentID != nil {
						r.Name = instrumentNames[*e.InstrumentID]
					}
				}
			}
		}
	}
	return result
}
