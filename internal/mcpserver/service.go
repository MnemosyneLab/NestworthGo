// Package mcpserver hosts the local agent boundary. It shares the desktop's
// application service; it never opens the household database independently.
package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/settings"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
)

const ChangedEvent = "agent.data.changed"
const ReadOnly = "read_only"
const DirectoryWrite = "directory_write"
const LedgerWrite = "ledger_write"

type Config struct {
	Enabled    bool   `json:"enabled"`
	Mode       string `json:"mode"`
	Port       int    `json:"port"`
	Token      string `json:"token"`
	InstanceID string `json:"instanceId"`
}
type Status struct {
	Running    bool   `json:"running"`
	Mode       string `json:"mode"`
	Endpoint   string `json:"endpoint"`
	InstanceID string `json:"instanceId"`
	Error      string `json:"error,omitempty"`
}
type Connection struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
	JSON     string `json:"json"`
}

type Service struct {
	mu          sync.Mutex
	operationMu sync.Mutex
	app         *application.Service
	dir         string
	repository  settings.ConfigurationRepository
	config      Config
	server      *http.Server
	listener    net.Listener
	endpoint    string
	lastError   string
	changed     func()
}

// Directory isolates credentials and receipts for each settings/database pair.
func Directory(settingsPath, databasePath string) string {
	settingsPath, _ = filepath.Abs(settingsPath)
	databasePath, _ = filepath.Abs(databasePath)
	hash := sha256.Sum256([]byte(settingsPath + "\x00" + databasePath))
	return filepath.Join(filepath.Dir(settingsPath), "agent", hex.EncodeToString(hash[:8]))
}
func New(app *application.Service, directory string, changed func(), repositories ...settings.ConfigurationRepository) *Service {
	s := &Service{app: app, dir: directory, changed: changed, config: Config{Mode: ReadOnly}}
	if len(repositories) > 0 {
		s.repository = repositories[0]
	}
	return s
}

// Resume is called only after the desktop event manager is ready.
func (s *Service) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.migrateConfiguration(); err != nil {
		s.lastError = "configuration_unavailable"
		return err
	}
	data, err := s.readPrivate(filepath.Join(s.dir, "connection.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		s.lastError = "configuration_unavailable"
		return err
	}
	var config Config
	if err = json.Unmarshal(data, &config); err != nil {
		s.lastError = "invalid_configuration"
		return err
	}
	if config.Mode != ReadOnly && config.Mode != DirectoryWrite && config.Mode != LedgerWrite {
		s.lastError = "invalid_configuration"
		return fmt.Errorf("invalid agent mode")
	}
	if config.Port < 0 || config.Port > 65535 || (config.Enabled && (len(config.Token) != 64 || uuid.Validate(config.InstanceID) != nil)) {
		s.lastError = "invalid_configuration"
		return fmt.Errorf("invalid agent configuration")
	}
	s.config = config
	if !config.Enabled {
		return nil
	}
	return s.startLocked()
}

func (s *Service) Enable(mode string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mode != ReadOnly && mode != DirectoryWrite && mode != LedgerWrite {
		return s.statusLocked(), fmt.Errorf("invalid agent permission")
	}
	if s.server != nil && s.config.Mode == mode {
		return s.statusLocked(), nil
	}
	s.closeLocked()
	// Permission changes revoke the previous credential.
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return s.statusLocked(), err
	}
	s.config.Token = hex.EncodeToString(token)
	if s.config.InstanceID == "" {
		s.config.InstanceID = uuid.NewString()
	}
	s.config.Mode = mode
	s.config.Enabled = true
	err := s.startLocked()
	return s.statusLocked(), err
}
func (s *Service) startLocked() error {
	if s.app == nil {
		s.lastError = "database_unavailable"
		return fmt.Errorf("database unavailable")
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", s.config.Port))
	if err != nil {
		s.lastError = "port_unavailable"
		return err
	}
	s.config.Port = listener.Addr().(*net.TCPAddr).Port
	if err = s.writePrivate(filepath.Join(s.dir, "connection.json"), s.config); err != nil {
		listener.Close()
		s.lastError = "configuration_unavailable"
		return err
	}
	endpoint := "http://" + listener.Addr().String() + "/mcp"
	server := s.tools(s.config.Mode)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := &http.Server{Handler: protect(endpoint, s.config.Token, handler), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	s.server = httpServer
	s.listener = listener
	s.endpoint = endpoint
	s.lastError = ""
	go func() {
		err := httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			if s.server == httpServer {
				s.server = nil
				s.endpoint = ""
				s.lastError = "server_stopped"
			}
			s.mu.Unlock()
		}
	}()
	return nil
}
func (s *Service) Disable() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
	s.config.Enabled = false
	s.config.Token = ""
	err := s.writePrivate(filepath.Join(s.dir, "connection.json"), s.config)
	if err != nil {
		s.lastError = "configuration_unavailable"
	} else {
		s.lastError = ""
	}
	return s.statusLocked(), err
}

// Close stops the listener without changing the user's persisted preference.
func (s *Service) Close() { s.mu.Lock(); defer s.mu.Unlock(); s.closeLocked() }
func (s *Service) closeLocked() {
	// Close can race the Serve goroutine before net/http has registered its
	// listener. Own and close it explicitly to make immediate restarts reliable.
	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}
	if s.server != nil {
		_ = s.server.Close()
		s.server = nil
	}
	// Drain any write that started before revocation before returning.
	s.operationMu.Lock()
	s.operationMu.Unlock()
	s.endpoint = ""
}
func (s *Service) Status() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.statusLocked() }
func (s *Service) statusLocked() Status {
	return Status{Running: s.server != nil, Mode: s.config.Mode, Endpoint: s.endpoint, InstanceID: s.config.InstanceID, Error: s.lastError}
}
func (s *Service) Connection() (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return Connection{}, fmt.Errorf("agent server is disabled")
	}
	value := map[string]any{"mcpServers": map[string]any{"nestworth": map[string]any{"url": s.endpoint, "headers": map[string]string{"Authorization": "Bearer " + s.config.Token}}}}
	data, err := json.MarshalIndent(value, "", "  ")
	return Connection{Endpoint: s.endpoint, Token: s.config.Token, JSON: string(data)}, err
}

func protect(endpoint, token string, next http.Handler) http.Handler {
	expected, _ := url.Parse(endpoint)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		if r.Host != expected.Host {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != expected.Scheme+"://"+expected.Host {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func writePrivateJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".agent-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
