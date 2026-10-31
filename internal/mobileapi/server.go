// Package mobileapi is the closed mobile boundary of the existing
// desktop process. It never opens a database or registers Wails/MCP methods.
package mobileapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
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
const cookieName = "__Host-nw_mobile"

type Config struct {
	Enabled        bool   `json:"enabled"`
	Origin         string `json:"origin"`
	Listen         string `json:"listen"`
	Mode           string `json:"mode"` // tls or loopback_proxy
	Certificate    string `json:"certificate"`
	Key            string `json:"key"`
	FrontendOrigin string `json:"frontendOrigin"` // Optional, exact trusted HTTPS frontend origin.
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	u, err := url.Parse(c.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return invalid("origin")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !ip.IsPrivate() && !ip.IsLoopback() {
		return invalid("origin")
	}
	if c.FrontendOrigin != "" {
		frontend, err := url.Parse(c.FrontendOrigin)
		if err != nil || frontend.Scheme != "https" || frontend.Host == "" || frontend.User != nil || frontend.RawQuery != "" || frontend.Fragment != "" || frontend.Path != "" || frontend.Opaque != "" || c.FrontendOrigin != "https://"+frontend.Host || strings.ContainsAny(c.FrontendOrigin, "*\\\r\n") || frontend.Hostname() == "" {
			return invalid("frontendOrigin")
		}
	}
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil || port == "0" || port == "" {
		return invalid("listen")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.IsUnspecified() || (!ip.IsPrivate() && !ip.IsLoopback()) {
		return invalid("listen")
	}
	switch c.Mode {
	case "loopback_proxy":
		if !ip.IsLoopback() {
			return invalid("listen")
		}
	case "tls":
		if !filepath.IsAbs(c.Certificate) || !filepath.IsAbs(c.Key) {
			return invalid("certificate")
		}
	default:
		return invalid("mode")
	}
	return nil
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
	origin        string
	bearer        bool
	digest        [32]byte
	csrf          string
	idle          time.Time
	reads, writes int
	window        time.Time
	active        int
}
type grant struct {
	Device
	digest [32]byte
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
	Config  Config   `json:"config"`
	Running bool     `json:"running"`
	Error   string   `json:"error,omitempty"`
	Devices []Device `json:"devices"`
}
type Pairing struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	URL       string    `json:"url"`
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
	grants        map[[32]byte]grant
	plans         map[string]*plan
	login         map[string]loginBucket
	requests      chan struct{}
	pages         map[string]pageState
	health        *application.MarketDataHealthReport
	healthVersion string
	healthScan    string
	healthAt      time.Time
	now           func() time.Time
}

func New(app *application.Service, configPath string, assets fs.FS) *Server {
	s := &Server{app: app, path: configPath, assets: assets, sessions: map[[32]byte]*session{}, grants: map[[32]byte]grant{}, plans: map[string]*plan{}, login: map[string]loginBucket{}, requests: make(chan struct{}, 32), pages: map[string]pageState{}, now: time.Now}
	if raw, err := os.ReadFile(configPath); err == nil {
		if err = jsoncodec.Unmarshal(raw, &s.config); err != nil {
			s.config = Config{}
			s.lastError = "invalid_configuration"
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
	out := Status{Config: s.config, Running: s.listener != nil, Error: s.lastError, Devices: []Device{}}
	now := s.now()
	for _, d := range s.sessions {
		if now.Before(d.ExpiresAt) && now.Before(d.idle) {
			out.Devices = append(out.Devices, d.Device)
		}
	}
	return out
}

// Configure is a desktop-only action. Save only nonsecret connection settings;
// neither tokens nor sessions ever enter a file or a backup.
func (s *Server) Configure(c Config) (Status, error) {
	if err := c.Validate(); err != nil {
		return s.Status(), err
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if err := s.stop(); err != nil {
		return s.Status(), err
	}
	if c.Enabled {
		if err := s.start(c); err != nil {
			s.mu.Lock()
			s.lastError = listenerErrorCode(err)
			s.mu.Unlock()
			return s.Status(), invalid("connection")
		}
	}
	raw, err := jsoncodec.Marshal(c)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(s.path), 0700)
	}
	if err == nil {
		err = os.WriteFile(s.path, raw, 0600)
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
	var de *domain.Error
	if errors.As(err, &de) && de.Field == "certificate" {
		return "certificate_unavailable"
	}
	return "listener_unavailable"
}
func (s *Server) start(c Config) error {
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	if c.Mode == "tls" {
		cert, err := tls.LoadX509KeyPair(c.Certificate, c.Key)
		if err != nil {
			listener.Close()
			return invalid("certificate")
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		origin, _ := url.Parse(c.Origin)
		if err != nil || leaf.VerifyHostname(origin.Hostname()) != nil || s.now().Before(leaf.NotBefore) || s.now().After(leaf.NotAfter) {
			listener.Close()
			return invalid("certificate")
		}
		listener = tls.NewListener(listener, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})
	}
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	s.mu.Lock()
	s.config = c
	s.listener = listener
	s.http = server
	s.mu.Unlock()
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.lastError = "listener_unavailable"
			s.mu.Unlock()
		}
	}()
	return nil
}
func (s *Server) Revoke(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, d := range s.sessions {
		if id == "" || d.ID == id {
			delete(s.sessions, k)
		}
	}
	for k, g := range s.grants {
		if id == "" || g.ID == id {
			delete(s.grants, k)
		}
	}
	for k, p := range s.plans {
		if id == "" || p.owner == id {
			delete(s.plans, k)
		}
	}
	s.pages = map[string]pageState{}
	s.health = nil
}
func (s *Server) stop() error {
	s.Revoke("")
	s.mu.Lock()
	server := s.http
	s.http = nil
	s.listener = nil
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}
func (s *Server) Close() error { s.lifecycle.Lock(); defer s.lifecycle.Unlock(); return s.stop() }

func (s *Server) Pair(name string, p Permissions) (Pairing, error) {
	if len(strings.TrimSpace(name)) == 0 || len(name) > 100 || !p.Read {
		return Pairing{}, invalid("device")
	}
	value, err := secret()
	if err != nil {
		return Pairing{}, err
	}
	digest := sha256.Sum256([]byte(value))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return Pairing{}, invalid("connection")
	}
	now := s.now()
	for k, g := range s.grants {
		if !now.Before(g.ExpiresAt) {
			delete(s.grants, k)
		}
	}
	if len(s.grants) >= 5 {
		return Pairing{}, &domain.Error{Code: domain.ErrUnavailable}
	}
	expires := now.Add(10 * time.Minute)
	s.grants[digest] = grant{Device: Device{ID: uuid.NewString(), Name: strings.TrimSpace(name), Permissions: p, CreatedAt: now, ExpiresAt: expires}, digest: digest}
	return Pairing{Token: value, ExpiresAt: expires, URL: s.config.Origin}, nil
}

func (s *Server) sessionFor(r *http.Request) (*session, error) {
	origin := r.Header.Get("Origin")
	s.mu.RLock()
	c := s.config
	s.mu.RUnlock()
	cross := origin != "" && origin != c.Origin
	value := ""
	if cross {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			return nil, unauthorized()
		}
		value = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	} else {
		cookie, err := r.Cookie(cookieName)
		if err != nil || r.Header.Get("Authorization") != "" {
			return nil, unauthorized()
		}
		value = cookie.Value
		origin = c.Origin
	}
	digest := sha256.Sum256([]byte(value))
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.sessions[digest]
	now := s.now()
	if d == nil || !now.Before(d.ExpiresAt) || !now.Before(d.idle) {
		delete(s.sessions, digest)
		return nil, unauthorized()
	}
	if d.bearer != cross || d.origin != origin {
		return nil, unauthorized()
	}
	d.LastUsed = now
	d.idle = now.Add(24 * time.Hour)
	if now.Sub(d.window) >= time.Minute {
		d.window = now
		d.reads = 0
		d.writes = 0
	}
	if d.active >= 4 {
		return nil, limited()
	}
	if r.Method == "GET" {
		d.reads++
		if d.reads > 120 {
			return nil, limited()
		}
	} else {
		d.writes++
		if d.writes > 20 {
			return nil, limited()
		}
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
	if current == nil || !now.Before(current.ExpiresAt) || !now.Before(current.idle) {
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
	u, err := url.Parse(c.Origin)
	if err != nil || u.Host == "" || r.Host != u.Host {
		fail(w, forbidden())
		return
	}
	if c.Mode == "tls" && r.TLS == nil {
		fail(w, forbidden())
		return
	}
	if c.Mode == "loopback_proxy" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() || r.Header.Get("X-Forwarded-Proto") != "https" {
			fail(w, forbidden())
			return
		}
	}
	origin := r.Header.Get("Origin")
	cross := origin != "" && origin != c.Origin
	if cross && (c.FrontendOrigin == "" || origin != c.FrontendOrigin || !strings.HasPrefix(r.URL.Path, prefix+"/")) {
		fail(w, forbidden())
		return
	}
	if cross {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		// Bearer mode deliberately does not enable credentialed CORS.
		if r.Method == "OPTIONS" {
			s.preflight(w, r)
			return
		}
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
	if path == "/connection" && r.Method == "GET" {
		auth := "cookie"
		if cross {
			auth = "bearer"
		}
		reply(w, map[string]any{"service": "nestworth-mobile", "version": 1, "authentication": auth})
		return
	}
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
		s.Revoke(d.ID)
		if !d.bearer {
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
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
		Token string `json:"token"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, err)
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	s.mu.Lock()
	defer s.mu.Unlock()
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
	var found *grant
	for _, g := range s.grants {
		if subtle.ConstantTimeCompare(digest[:], g.digest[:]) == 1 {
			copy := g
			found = &copy
		}
	}
	if found == nil || !now.Before(found.ExpiresAt) {
		fail(w, unauthorized())
		return
	}
	for k, d := range s.sessions {
		if !now.Before(d.ExpiresAt) || !now.Before(d.idle) {
			delete(s.sessions, k)
		}
	}
	if len(s.sessions) >= 5 {
		fail(w, limited())
		return
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
	delete(s.grants, digest)
	device := found.Device
	device.ExpiresAt = now.Add(7 * 24 * time.Hour)
	device.LastUsed = now
	sessionDigest := sha256.Sum256([]byte(value))
	origin := r.Header.Get("Origin")
	cross := origin != s.config.Origin
	s.sessions[sessionDigest] = &session{Device: device, origin: origin, bearer: cross, digest: sessionDigest, csrf: csrf, idle: now.Add(24 * time.Hour), window: now}
	result := map[string]any{"device": device, "csrfToken": csrf, "instance": s.app.MobileWorkspace()}
	if cross {
		result["accessToken"] = value
	} else {
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 7 * 24 * 3600})
	}
	reply(w, result)
}
