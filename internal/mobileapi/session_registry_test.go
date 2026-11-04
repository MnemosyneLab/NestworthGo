package mobileapi

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func rememberLogin(t *testing.T, f *httpFixture) {
	t.Helper()
	response, raw := f.request(t, "POST", "/session", map[string]any{"token": f.s.config.Token, "remember": true}, nil)
	if response.StatusCode != 200 {
		t.Fatalf("remember login: %d %s", response.StatusCode, raw)
	}
	f.acceptLogin(t, response, raw)
}
func restartRemembered(t *testing.T, f *httpFixture) {
	t.Helper()
	raw, err := jsoncodec.Marshal(storedConfig{Version: configVersion, SessionEpoch: f.s.sessionEpoch, Config: f.s.config})
	if err != nil {
		t.Fatal(err)
	}
	if err = saveConfig(f.s.path, raw); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.http.Client().CloseIdleConnections()
	fresh := New(f.s.app, f.s.path, nil)
	if err = fresh.Resume(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	f.s = fresh
}
func TestRememberedSessionRestartOnlyStoresHashAndRotatesCSRF(t *testing.T) {
	f := fixture(t, Permissions{Read: true, Record: true, Manual: true, Market: true})
	rememberLogin(t, f)
	oldCSRF := f.csrf
	raw, err := os.ReadFile(f.s.registryPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{f.token, f.csrf, f.s.config.Token, f.s.sessionEpoch} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("plaintext credential in registry")
		}
	}
	info, _ := os.Stat(f.s.registryPath())
	if info.Mode().Perm() != 0600 {
		t.Fatal("registry permissions")
	}
	// A new owner can recover immediately without a shutdown flush (crash case).
	probe := New(f.s.app, f.s.path, nil)
	probe.config = f.s.config
	probe.mu.Lock()
	probe.loadSessionsLocked()
	probe.mu.Unlock()
	if len(probe.sessions) != 1 {
		t.Fatal("unflushed login lost")
	}
	restartRemembered(t, f)
	response, raw := f.request(t, "GET", "/session", nil, nil)
	if response.StatusCode != 200 {
		t.Fatalf("restart: %d %s", response.StatusCode, raw)
	}
	var current struct {
		CSRF string `json:"csrfToken"`
	}
	if err = jsoncodec.Unmarshal(raw, &current); err != nil {
		t.Fatal(err)
	}
	if current.CSRF == oldCSRF || current.CSRF == "" || len(f.s.plans) != 0 || len(f.s.pages) != 0 {
		t.Fatal("process secrets/plans restored")
	}
	response, _ = f.request(t, "POST", "/record-previews", map[string]string{}, nil)
	if response.StatusCode != 403 {
		t.Fatal("old csrf survived")
	}
	f.csrf = current.CSRF
	response, _ = f.request(t, "DELETE", "/session", nil, nil)
	if response.StatusCode != 200 {
		t.Fatal("logout failed")
	}
	restartRemembered(t, f)
	response, _ = f.request(t, "GET", "/session", nil, nil)
	if response.StatusCode != 401 {
		t.Fatal("logout resurrected")
	}
}
func TestRememberedSessionExpiryLastUseAndRevocations(t *testing.T) {
	for _, kind := range []string{"idle", "absolute", "recent", "all", "restore", "apply", "token", "disabled", "other_database"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(t, Permissions{Read: true, Record: true})
			rememberLogin(t, f)
			start := time.Now().UTC()
			f.s.mu.Lock()
			f.s.now = func() time.Time { return start }
			for _, d := range f.s.sessions {
				d.CreatedAt = start
				d.LastUsed = start
				d.ExpiresAt = start.Add(7 * 24 * time.Hour)
				d.idle = start.Add(24 * time.Hour)
			}
			if err := f.s.persistSessionsLocked(); err != nil {
				t.Fatal(err)
			}
			f.s.mu.Unlock()
			switch kind {
			case "idle":
				f.s.now = func() time.Time { return start.Add(24 * time.Hour) }
			case "absolute":
				f.s.now = func() time.Time { return start.Add(7 * 24 * time.Hour) }
				for _, d := range f.s.sessions {
					d.idle = start.Add(8 * 24 * time.Hour)
				}
			case "recent":
				f.s.now = func() time.Time { return start.Add(23 * time.Hour) }
				response, _ := f.request(t, "GET", "/session", nil, nil)
				if response.StatusCode != 200 {
					t.Fatal("recent request failed")
				}
				f.s.mu.Lock()
				probe := New(f.s.app, f.s.path, nil)
				probe.config = f.s.config
				probe.now = func() time.Time { return start.Add(25 * time.Hour) }
				probe.loadSessionsLocked()
				f.s.mu.Unlock()
				if len(probe.sessions) != 1 {
					t.Fatal("last-use was not durable")
				}
				return
			case "all":
				f.s.Revoke("")
			case "restore":
				if err := f.s.RevokeForRestore(); err != nil {
					t.Fatal(err)
				}
			case "apply", "token", "disabled":
				c := f.s.config
				oldEpoch := f.s.sessionEpoch
				if kind == "token" {
					c.Token = "CHANGED12"
				}
				if kind == "disabled" {
					c.Enabled = false
				}
				if _, err := f.s.Configure(c); err != nil {
					t.Fatal(err)
				}
				if f.s.sessionEpoch == oldEpoch {
					t.Fatal("apply did not rotate session scope key")
				}
			case "other_database":
				other := fixture(t, Permissions{Read: true})
				probe := New(other.s.app, f.s.path, nil)
				probe.config = f.s.config
				probe.loadSessionsLocked()
				if len(probe.sessions) != 0 {
					t.Fatal("database switch retained grants")
				}
				return
			}
			if kind == "restore" || kind == "disabled" {
				probe := New(f.s.app, f.s.path, nil)
				probe.config = f.s.config
				probe.loadSessionsLocked()
				if len(probe.sessions) != 0 {
					t.Fatal("closed access resurrected a grant")
				}
				return
			}
			response, _ := f.request(t, "GET", "/session", nil, nil)
			if response.StatusCode != 401 {
				t.Fatalf("%s accepted old credential: %d", kind, response.StatusCode)
			}
			if kind != "idle" && kind != "absolute" {
				probe := New(f.s.app, f.s.path, nil)
				probe.config = f.s.config
				probe.loadSessionsLocked()
				if len(probe.sessions) != 0 {
					t.Fatal("revocation not durable")
				}
			}
		})
	}
}
func TestRegistryCorruptionAndFailedRevocationAreFailClosed(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	rememberLogin(t, f)
	if err := os.WriteFile(f.s.registryPath(), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	probe := New(f.s.app, f.s.path, nil)
	probe.config = f.s.config
	probe.loadSessionsLocked()
	if len(probe.sessions) != 0 || probe.registryError != nil {
		t.Fatal("corruption did not safely reset")
	}
	if err := os.Remove(f.s.registryPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(f.s.registryPath(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.s.registryPath()+"/block", []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RevokeForRestore(); err == nil {
		t.Fatal("acknowledged failed durable revocation")
	}
	response, _ := f.request(t, "GET", "/session", nil, nil)
	if response.StatusCode != 503 {
		t.Fatalf("storage failure should preserve browser credential: %d", response.StatusCode)
	}
}
func TestRememberedConcurrentRequestsAndRevocation(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	rememberLogin(t, f)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { f.request(t, http.MethodGet, "/session", nil, nil) })
	}
	wg.Go(func() { f.s.Revoke("") })
	wg.Wait()
	response, _ := f.request(t, "GET", "/session", nil, nil)
	if response.StatusCode != 401 {
		t.Fatal("concurrent revoke lost")
	}
	probe := New(f.s.app, f.s.path, nil)
	probe.config = f.s.config
	probe.loadSessionsLocked()
	if len(probe.sessions) != 0 {
		t.Fatal("concurrent request restored revoked grant")
	}
}

func TestRememberedLimitSurvivesReloadAndEvictsLeastRecentlyUsedInactive(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	f.s.Revoke("")
	now := time.Now()
	var tokens []string
	for i := range 5 {
		f.s.now = func() time.Time { return now.Add(time.Duration(i+1) * time.Minute) }
		rememberLogin(t, f)
		tokens = append(tokens, f.token)
	}
	f.s.mu.Lock()
	for _, d := range f.s.sessions {
		d.active = 1
	}
	f.s.mu.Unlock()
	response, _ := f.request(t, "POST", "/session", map[string]any{"token": f.s.config.Token, "remember": true}, nil)
	if response.StatusCode != 429 {
		t.Fatal("evicted an in-flight session")
	}
	f.s.mu.Lock()
	for _, d := range f.s.sessions {
		d.active = 0
	}
	f.s.mu.Unlock()
	// Touch the oldest, making the second session the eviction candidate.
	f.token = tokens[0]
	response, _ = f.request(t, "GET", "/session", nil, nil)
	if response.StatusCode != 200 {
		t.Fatal("oldest session unavailable")
	}
	f.s.now = func() time.Time { return now.Add(6 * time.Minute) }
	rememberLogin(t, f)
	if len(f.s.sessions) != 5 {
		t.Fatal("session cap changed")
	}
	response, _ = f.request(t, "GET", "/session", nil, map[string]string{"Authorization": "Bearer " + tokens[1]})
	if response.StatusCode != 401 {
		t.Fatal("did not evict least recently used inactive session")
	}
	probe := New(f.s.app, f.s.path, nil)
	probe.config = f.s.config
	probe.now = f.s.now
	probe.loadSessionsLocked()
	if len(probe.sessions) != 5 {
		t.Fatal("durable cap lost")
	}
}
func TestDatabaseFileReplacementChangesSessionScope(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	if err := f.s.ensureSessionEpochLocked(); err != nil {
		t.Fatal(err)
	}
	before, err := f.s.registryScope()
	if err != nil {
		t.Fatal(err)
	}
	path := f.s.app.MobileSessionDatabasePath()
	// Only the disposable synthetic test DB is moved; no business writes follow.
	if err = os.Rename(path, path+".synthetic-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("synthetic replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := f.s.registryScope()
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("same-path replacement retained grants")
	}
}

// Kill a writer after file+directory fsync, without graceful server cleanup.
func TestSessionRegistrySurvivesWriterProcessCrash(t *testing.T) {
	f := fixture(t, Permissions{Read: true})
	rememberLogin(t, f)
	raw, err := os.ReadFile(f.s.registryPath())
	if err != nil {
		t.Fatal(err)
	}
	var saved sessionRegistry
	if err = jsoncodec.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	for _, revoked := range []bool{false, true} {
		if revoked {
			saved.Sessions = nil
			raw, err = jsoncodec.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSessionRegistryCrashWriter$")
		cmd.Env = append(os.Environ(), "NESTWORTH_TEST_MOBILE_REGISTRY="+f.s.registryPath())
		cmd.Stdin = bytes.NewReader(raw)
		output, pipeErr := cmd.StdoutPipe()
		if pipeErr != nil {
			cancel()
			t.Fatal(pipeErr)
		}
		if err = cmd.Start(); err != nil {
			cancel()
			t.Fatal(err)
		}
		ready, readErr := bufio.NewReader(output).ReadString('\n')
		if readErr != nil || ready != "durable\n" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			cancel()
			t.Fatalf("writer did not persist: %v", readErr)
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		cancel()
		probe := New(f.s.app, f.s.path, nil)
		probe.config = f.s.config
		probe.loadSessionsLocked()
		expected := 1
		if revoked {
			expected = 0
		}
		if len(probe.sessions) != expected {
			t.Fatal("crash lost durable login or resurrected revocation")
		}
	}
}
func TestSessionRegistryCrashWriter(t *testing.T) {
	path := os.Getenv("NESTWORTH_TEST_MOBILE_REGISTRY")
	if path == "" {
		t.Skip("subprocess helper")
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 16385))
	if err != nil || len(raw) > 16384 {
		t.Fatal("invalid synthetic input")
	}
	if err = saveConfig(path, raw); err != nil {
		t.Fatal(err)
	}
	fmt.Println("durable")
	for {
		time.Sleep(time.Hour)
	}
}
