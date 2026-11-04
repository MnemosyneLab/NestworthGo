// Package mobileapi is the closed mobile boundary of the existing
// desktop process. It never opens a database or registers Wails/MCP methods.
package mobileapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const prefix = "/mobile-api/v1"

// Config belongs to the desktop only. Token is never returned by a mobile route.
type Config struct {
	Enabled bool   `json:"enabled"`
	Listen  string `json:"listen"`
	Token   string `json:"token"`
}

const configVersion = 2

type storedConfig struct {
	Version      int    `json:"version"`
	SessionEpoch string `json:"sessionEpoch,omitempty"`
	Config
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	host, port, err := net.SplitHostPort(c.Listen)
	number, portErr := strconv.Atoi(port)
	ip, ipErr := netip.ParseAddr(host)
	if err != nil || portErr != nil || number < 1 || number > 65535 ||
		ipErr != nil || ip.Zone() != "" || ip.IsUnspecified() ||
		(!ip.IsPrivate() && !ip.IsLoopback()) ||
		c.Listen != net.JoinHostPort(ip.String(), strconv.Itoa(number)) {
		return invalid("listen")
	}
	if len(c.Token) < 8 || len(c.Token) > 64 {
		return invalid("token")
	}
	for _, ch := range c.Token {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') {
			return invalid("token")
		}
	}
	return nil
}

// The browser omits HTTP's default port in Host and Origin headers.
func (c Config) browserOrigin() string { return "http://" + strings.TrimSuffix(c.Listen, ":80") }

// GenerateToken returns a desktop draft, never enabling access or changing a
// saved credential. Ten random base32 characters provide 50 bits of entropy.
func (s *Server) GenerateToken() (string, error) {
	var raw [7]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])[:10], nil
}

type Permissions struct {
	Read   bool `json:"read"`
	Record bool `json:"record"`
	Market bool `json:"market_refresh"`
	Manual bool `json:"manual_quote"`
}
type Device struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Permissions Permissions `json:"permissions"`
	CreatedAt   time.Time   `json:"createdAt"`
	LastUsed    time.Time   `json:"lastUsed"`
	ExpiresAt   time.Time   `json:"expiresAt"`
}
type session struct {
	Device
	digest        [32]byte
	csrf          string
	idle          time.Time
	reads, writes int
	window        time.Time
	active        int
	remember      bool
}
type loginBucket struct {
	at    time.Time
	count int
}
type plan struct {
	owner        string
	operation    string
	record       *application.MobileRecordPlan
	quote        *application.MobileQuotePlan
	market       *application.MobileMarketRequest
	marketResult map[string]any
	expires      time.Time
	mu           sync.Mutex
	state        string
}
type Status struct {
	Config  Config `json:"config"`
	URL     string `json:"url"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}
type Server struct {
	app           *application.Service
	assets        fs.FS
	path          string
	lifecycle     sync.Mutex
	mu            sync.RWMutex
	config        Config
	listener      net.Listener
	http          *http.Server
	lastError     string
	sessions      map[[32]byte]*session
	plans         map[string]*plan
	login         map[string]loginBucket
	requests      chan struct{}
	pages         map[string]pageState
	health        *application.MarketDataHealthReport
	healthVersion string
	healthScan    string
	healthAt      time.Time
	now           func() time.Time
	sessionEpoch  string
	registryReady bool
	registryError error
}

func New(app *application.Service, configPath string, assets fs.FS) *Server {
	s := &Server{app: app, path: configPath, assets: assets, sessions: map[[32]byte]*session{}, plans: map[string]*plan{}, login: map[string]loginBucket{}, requests: make(chan struct{}, 32), pages: map[string]pageState{}, now: time.Now}
	if raw, err := os.ReadFile(configPath); err == nil {
		var saved storedConfig
		if err = jsoncodec.Unmarshal(raw, &saved); err != nil {
			s.lastError = "invalid_configuration"
		} else if saved.Version != configVersion {
			// Never silently downgrade an existing TLS/proxy listener to HTTP
			// or widen old per-device grants into shared-token access.
			s.config.Listen = saved.Listen
			s.lastError = "configuration_reset"
		} else {
			s.config = saved.Config
			s.sessionEpoch = saved.SessionEpoch
		}
	}
	return s
}
func invalid(field string) error {
	return &domain.Error{Code: domain.ErrValidation, Field: field, Message: "invalid mobile access configuration"}
}
func secret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
func (s *Server) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := Status{URL: s.config.browserOrigin(), Config: s.config, Running: s.listener != nil, Error: s.lastError}
	return out
}

// Configure is desktop-only. The shared token lives in a private settings file;
// remembered sessions use a private hash registry. Every apply revokes sessions.
func (s *Server) Configure(c Config) (Status, error) {
	if err := c.Validate(); err != nil {
		return s.Status(), err
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if err := s.revoke(""); err != nil {
		return s.Status(), err
	}
	if err := s.stop(); err != nil {
		return s.Status(), err
	}
	epoch, err := secret()
	if err != nil {
		return s.Status(), err
	}
	s.mu.Lock()
	s.sessionEpoch = epoch
	s.mu.Unlock()
	if c.Enabled {
		if err := s.start(c); err != nil {
			s.mu.Lock()
			s.lastError = listenerErrorCode(err)
			s.mu.Unlock()
			return s.Status(), invalid("connection")
		}
	}
	raw, err := jsoncodec.Marshal(storedConfig{Version: configVersion, SessionEpoch: epoch, Config: c})
	if err == nil {
		err = os.MkdirAll(filepath.Dir(s.path), 0700)
	}
	if err == nil {
		err = saveConfig(s.path, raw)
	}
	if err != nil {
		_ = s.stop()
		return s.Status(), err
	}
	s.mu.Lock()
	s.config = c
	s.lastError = ""
	s.mu.Unlock()
	return s.Status(), nil
}

// Replace atomically with restrictive permissions, including existing legacy files.
func saveConfig(path string, raw []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".mobile-access-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func (s *Server) Resume() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.RLock()
	c := s.config
	s.mu.RUnlock()
	if !c.Enabled {
		return nil
	}
	if err := c.Validate(); err != nil {
		s.mu.Lock()
		s.lastError = "invalid_configuration"
		s.mu.Unlock()
		return err
	}
	err := s.start(c)
	if err != nil {
		s.mu.Lock()
		s.lastError = listenerErrorCode(err)
		s.mu.Unlock()
	}
	return err
}

func listenerErrorCode(err error) string {
	if errors.Is(err, syscall.EADDRINUSE) {
		return "port_in_use"
	}
	return "listener_unavailable"
}
func (s *Server) start(c Config) error {
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	s.mu.Lock()
	s.config = c
	if err := s.ensureSessionEpochLocked(); err != nil {
		s.mu.Unlock()
		_ = listener.Close()
		return err
	}
	if !s.registryReady {
		s.loadSessionsLocked()
	}
	if s.registryError != nil {
		err := s.registryError
		s.mu.Unlock()
		_ = listener.Close()
		return err
	}
	s.listener = listener
	s.http = server
	s.mu.Unlock()
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			if s.http == server {
				s.lastError = "listener_unavailable"
			}
			s.mu.Unlock()
		}
	}()
	return nil
}
func (s *Server) Revoke(id string) {
	_ = s.revoke(id)
}
func (s *Server) revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, d := range s.sessions {
		if id == "" || d.ID == id {
			delete(s.sessions, k)
		}
	}
	for k, p := range s.plans {
		if id == "" || p.owner == id {
			delete(s.plans, k)
		}
	}
	s.pages = map[string]pageState{}
	s.health = nil
	return s.persistSessionsLocked()
}
func (s *Server) stop() error {
	s.mu.Lock()
	server := s.http
	listener := s.listener
	s.http = nil
	s.listener = nil
	s.sessions = map[[32]byte]*session{}
	s.registryReady = false
	s.plans = map[string]*plan{}
	s.pages = map[string]pageState{}
	s.health = nil
	s.mu.Unlock()
	// Shutdown can race Serve's listener registration on an immediate reapply.
	// Close the owned listener even if Serve has not started yet.
	if listener != nil {
		_ = listener.Close()
	}
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := server.Shutdown(ctx)
	// The owned listener was explicitly closed above; Serve and Shutdown may
	// both observe that same close while draining. It is not a shutdown failure.
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// Close drains transport and drops process-local plans, preserving durable login.
func (s *Server) Close() error { s.lifecycle.Lock(); defer s.lifecycle.Unlock(); return s.stop() }

// RevokeForRestore must succeed before replacement; shutdown alone preserves login.
func (s *Server) RevokeForRestore() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if err := s.revoke(""); err != nil {
		return err
	}
	return s.stop()
}

func (s *Server) sessionFor(r *http.Request) (*session, error) {
	// HTTP uses explicit bearer credentials, never ambient cookies.
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, unauthorized()
	}
	value := strings.TrimPrefix(auth, "Bearer ")
	digest := sha256.Sum256([]byte(value))
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.sessions[digest]
	now := s.now()
	if s.registryError != nil {
		return nil, s.registryError
	}
	if s.listener == nil || d == nil || !now.Before(d.ExpiresAt) || !now.Before(d.idle) {
		delete(s.sessions, digest)
		return nil, unauthorized()
	}
	// Reject before updating idle/recent use or touching durable storage. Keep
	// admission, persistence and the active reservation under the same lock.
	if d.active >= 4 {
		return nil, limited()
	}
	if now.Sub(d.window) >= time.Minute {
		d.window = now
		d.reads = 0
		d.writes = 0
	}
	if r.Method == "GET" && d.reads >= 120 || r.Method != "GET" && d.writes >= 20 {
		return nil, limited()
	}
	previousUse, previousIdle := d.LastUsed, d.idle
	d.LastUsed = now
	d.idle = now.Add(24 * time.Hour)
	if d.remember {
		if err := s.persistSessionsLocked(); err != nil {
			d.LastUsed, d.idle = previousUse, previousIdle
			return nil, err
		}
	}
	if r.Method == "GET" {
		d.reads++
	} else {
		d.writes++
	}
	d.active++
	copy := *d
	return &copy, nil
}
func unauthorized() error { return &domain.Error{Code: domain.ErrUnavailable, Field: "session"} }
func limited() error      { return &domain.Error{Code: domain.ErrUnavailable, Field: "rateLimit"} }
func forbidden() error    { return &domain.Error{Code: domain.ErrUnavailable, Field: "permission"} }

// This lock is acquired by MobileAuthorizedWrite only after the application
// write gate. Revocation cannot overtake an admitted commit or deadlock restore.
func (s *Server) authorize(d *session, permission string) (func(), error) {
	s.mu.RLock()
	current := s.sessions[d.digest]
	now := s.now()
	if s.registryError != nil {
		err := s.registryError
		s.mu.RUnlock()
		return nil, err
	}
	if s.listener == nil || current == nil || !now.Before(current.ExpiresAt) || !now.Before(current.idle) {
		s.mu.RUnlock()
		return nil, unauthorized()
	}
	allowed := current.Permissions.Read
	switch permission {
	case "record":
		allowed = allowed && current.Permissions.Record
	case "manual_quote":
		allowed = allowed && current.Permissions.Manual
	case "market_refresh":
		allowed = allowed && current.Permissions.Market
	}
	if !allowed {
		s.mu.RUnlock()
		return nil, forbidden()
	}
	return s.mu.RUnlock, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Request-ID", uuid.NewString())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	s.mu.RLock()
	c := s.config
	s.mu.RUnlock()
	expectedHost := strings.TrimPrefix(c.browserOrigin(), "http://")
	if expectedHost == "" || r.Host != expectedHost {
		fail(w, forbidden())
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != c.browserOrigin() {
		fail(w, forbidden())
		return
	}
	// No separate frontend, proxy trust, CORS or TLS transport remains.
	if r.Method == "OPTIONS" {
		fail(w, forbidden())
		return
	}
	select {
	case s.requests <- struct{}{}:
		defer func() { <-s.requests }()
	default:
		fail(w, limited())
		return
	}
	if !strings.HasPrefix(r.URL.Path, prefix+"/") {
		s.static(w, r)
		return
	}
	if r.Method != "GET" {
		if origin == "" || r.Header.Get("X-Nestworth-Mobile") != "1" {
			fail(w, forbidden())
			return
		}
		mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if r.Method == "POST" && (mediaErr != nil || mediaType != "application/json") {
			fail(w, invalid("contentType"))
			return
		}
	}
	path := strings.TrimPrefix(r.URL.Path, prefix)
	if path == "/session" && r.Method == "POST" {
		s.loginSession(w, r)
		return
	}
	d, err := s.sessionFor(r)
	if err != nil {
		fail(w, err)
		return
	}
	defer func() {
		s.mu.Lock()
		if current := s.sessions[d.digest]; current != nil {
			current.active--
		}
		s.mu.Unlock()
	}()
	if r.Method != "GET" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(d.csrf)) != 1 {
		fail(w, forbidden())
		return
	}
	if r.Method == "GET" && path == "/session" {
		reply(w, map[string]any{"device": d.Device, "csrfToken": d.csrf, "instance": s.app.MobileWorkspace()})
		return
	}
	if r.Method == "DELETE" && path == "/session" {
		if err := s.revoke(d.ID); err != nil {
			fail(w, err)
			return
		}
		reply(w, map[string]bool{"ok": true})
		return
	}
	if !d.Permissions.Read {
		fail(w, forbidden())
		return
	}
	s.route(w, r, d, path)
}

func (s *Server) loginSession(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Remember bool   `json:"remember,omitempty"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, err)
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registryError != nil {
		fail(w, s.registryError)
		return
	}
	if s.listener == nil {
		fail(w, unauthorized())
		return
	}
	now := s.now()
	bucket := s.login[host]
	if now.Sub(bucket.at) >= time.Minute {
		bucket = loginBucket{at: now}
	}
	bucket.count++
	s.login[host] = bucket
	if len(s.login) > 128 {
		for key, v := range s.login {
			if now.Sub(v.at) >= time.Minute {
				delete(s.login, key)
			}
		}
		if len(s.login) > 128 {
			fail(w, limited())
			return
		}
	}
	if bucket.count > 5 {
		fail(w, limited())
		return
	}
	digest := sha256.Sum256([]byte(in.Token))
	expected := sha256.Sum256([]byte(s.config.Token))
	if s.config.Token == "" || subtle.ConstantTimeCompare(digest[:], expected[:]) != 1 {
		fail(w, unauthorized())
		return
	}
	for k, d := range s.sessions {
		if !now.Before(d.ExpiresAt) || !now.Before(d.idle) {
			delete(s.sessions, k)
		}
	}
	if len(s.sessions) >= 5 {
		// Admit a fresh login by replacing the least recently used inactive
		// session. Remembered logins retain their slot across reload/restart;
		// never evict an in-flight request.
		var oldest *session
		for _, current := range s.sessions {
			if current.active == 0 && (oldest == nil || current.LastUsed.Before(oldest.LastUsed)) {
				oldest = current
			}
		}
		if oldest == nil {
			fail(w, limited())
			return
		}
		delete(s.sessions, oldest.digest)
		for key, plan := range s.plans {
			if plan.owner == oldest.ID {
				delete(s.plans, key)
			}
		}
		for key, page := range s.pages {
			if page.owner == oldest.ID {
				delete(s.pages, key)
			}
		}
	}
	if in.Remember {
		if err := s.ensureSessionEpochLocked(); err != nil {
			fail(w, err)
			return
		}
		if _, err := s.registryScope(); err != nil {
			fail(w, errSessionRegistry)
			return
		}
	}
	value, err := secret()
	if err != nil {
		fail(w, err)
		return
	}
	csrf, err := secret()
	if err != nil {
		fail(w, err)
		return
	}
	device := Device{ID: uuid.NewString(), Name: "Mobile browser", Permissions: Permissions{Read: true, Record: true, Market: true, Manual: true}, CreatedAt: now, LastUsed: now, ExpiresAt: now.Add(7 * 24 * time.Hour)}
	sessionDigest := sha256.Sum256([]byte(value))
	s.sessions[sessionDigest] = &session{Device: device, digest: sessionDigest, csrf: csrf, idle: now.Add(24 * time.Hour), window: now, remember: in.Remember}
	if err := s.persistSessionsLocked(); err != nil {
		fail(w, err)
		return
	}
	reply(w, map[string]any{"device": device, "csrfToken": csrf, "instance": s.app.MobileWorkspace(), "accessToken": value})
}
