package mobileapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Outside SQLite and Nestworth backups: no bearer, shared token, CSRF,
// previews or financial bodies are stored.
type rememberedSession struct {
	Digest    string    `json:"digest"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type sessionRegistry struct {
	Version  int                 `json:"version"`
	Scope    string              `json:"scope"`
	Sessions []rememberedSession `json:"sessions"`
}

var errSessionRegistry = errors.New("mobile session storage unavailable")

func (s *Server) registryPath() string { return s.path + ".sessions" }
func (s *Server) registryScope() (string, error) {
	if s.app == nil || s.sessionEpoch == "" {
		return "", errSessionRegistry
	}
	identity, err := databaseFileIdentity(s.app.MobileSessionDatabasePath())
	if err != nil {
		return "", err
	}
	// Reject DB replacement even at the same path, or copied settings.
	// The separate random epoch prevents a leaked registry from becoming an
	// offline guessing oracle for a user-chosen shared token.
	mac := hmac.New(sha256.New, []byte(s.sessionEpoch))
	_, _ = mac.Write([]byte(identity + "\x00" + s.config.Listen + "\x00" + s.config.Token))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Called under mu before publishing a resumed listener. Corruption and old
// scopes grant no access; replace durably before allowing fresh token login.
func (s *Server) loadSessionsLocked() {
	s.registryReady = true
	s.registryError = nil
	scope, err := s.registryScope()
	if err != nil {
		_ = s.persistSessionsLocked()
		return
	}
	info, err := os.Lstat(s.registryPath())
	if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0077 == 0 && info.Size() <= 16384 {
		raw, readErr := os.ReadFile(s.registryPath())
		var saved sessionRegistry
		if readErr == nil && jsoncodec.Unmarshal(raw, &saved) == nil && saved.Version == 1 && saved.Scope == scope && len(saved.Sessions) <= 5 {
			now := s.now()
			for _, item := range saved.Sessions {
				digest, decodeErr := hex.DecodeString(item.Digest)
				if decodeErr != nil || len(digest) != 32 || item.ID == "" || item.CreatedAt.IsZero() || item.LastUsed.Before(item.CreatedAt) || item.LastUsed.After(now) || !item.ExpiresAt.Equal(item.CreatedAt.Add(7*24*time.Hour)) || !now.Before(item.ExpiresAt) || !now.Before(item.LastUsed.Add(24*time.Hour)) {
					continue
				}
				csrf, secretErr := secret()
				if secretErr != nil {
					s.registryError = errSessionRegistry
					return
				}
				key := [32]byte(digest)
				s.sessions[key] = &session{Device: Device{ID: item.ID, Name: "Mobile browser", Permissions: Permissions{Read: true, Record: true, Market: true, Manual: true}, CreatedAt: item.CreatedAt, LastUsed: item.LastUsed, ExpiresAt: item.ExpiresAt}, digest: key, csrf: csrf, idle: item.LastUsed.Add(24 * time.Hour), window: now, remember: true}
			}
		}
	}
	_ = s.persistSessionsLocked()
}

// Serialize with authorization/revocation. Persist last-use before accepting a
// remembered request. Crash may shorten validity, never extend it or undo an
// acknowledged revocation. Errors fence writes and never acknowledge success.
func (s *Server) persistSessionsLocked() error {
	scope, err := s.registryScope()
	if err != nil {
		hasRemembered := false
		for _, d := range s.sessions {
			hasRemembered = hasRemembered || d.remember
		}
		if !hasRemembered {
			scope, err = "", nil
		}
	}
	if err == nil {
		saved := sessionRegistry{Version: 1, Scope: scope, Sessions: []rememberedSession{}}
		now := s.now()
		for key, d := range s.sessions {
			if d.remember && now.Before(d.ExpiresAt) && now.Before(d.idle) {
				saved.Sessions = append(saved.Sessions, rememberedSession{Digest: hex.EncodeToString(key[:]), ID: d.ID, CreatedAt: d.CreatedAt, LastUsed: d.LastUsed, ExpiresAt: d.ExpiresAt})
			}
		}
		var raw []byte
		raw, err = jsoncodec.Marshal(saved)
		if err == nil {
			err = os.MkdirAll(filepath.Dir(s.path), 0700)
		}
		if err == nil {
			err = saveConfig(s.registryPath(), raw)
		}
	}
	if err != nil {
		s.registryError = errSessionRegistry
		// Best effort: remove older grants. Failure remains uncertain; the caller
		// must not claim sign-out/restore success.
		_ = os.Remove(s.registryPath())
		_ = syncDirectory(filepath.Dir(s.path))
		return errSessionRegistry
	}
	s.registryError = nil
	return nil
}

// Upgrade an existing version-2 configuration lazily without exposing this
// random scope key through desktop/mobile DTOs. Never store it in the registry.
func (s *Server) ensureSessionEpochLocked() error {
	decoded, err := base64.RawURLEncoding.DecodeString(s.sessionEpoch)
	if err == nil && len(decoded) == 32 {
		return nil
	}
	epoch, err := secret()
	if err != nil {
		return errSessionRegistry
	}
	raw, err := jsoncodec.Marshal(storedConfig{Version: configVersion, SessionEpoch: epoch, Config: s.config})
	if err == nil {
		err = os.MkdirAll(filepath.Dir(s.path), 0700)
	}
	if err == nil {
		err = saveConfig(s.path, raw)
	}
	if err != nil {
		return errSessionRegistry
	}
	s.sessionEpoch = epoch
	return nil
}
