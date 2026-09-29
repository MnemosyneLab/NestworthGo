package mcpserver

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func configurationKey(path string) string {
	if filepath.Base(path) == "connection.json" {
		return "mcp.connection"
	}
	return "mcp.operation." + strings.TrimSuffix(filepath.Base(path), ".json")
}
func (s *Service) readPrivate(path string) ([]byte, error) {
	if s.repository == nil {
		return os.ReadFile(path)
	}
	value, found, err := s.repository.LoadConfiguration(configurationKey(path))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, os.ErrNotExist
	}
	return value, nil
}
func (s *Service) writePrivate(path string, value any) error {
	if s.repository == nil {
		return writePrivateJSON(path, value)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.repository.SaveConfiguration(configurationKey(path), data)
}

// Import each receipt before deleting its legacy copy. Existing SQLite values
// win, so a crash cannot resurrect a revoked token or replay an old receipt.
func (s *Service) migrateConfiguration() error {
	if s.repository == nil {
		return nil
	}
	files := []string{filepath.Join(s.dir, "connection.json")}
	entries, err := os.ReadDir(filepath.Join(s.dir, "operations"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			files = append(files, filepath.Join(s.dir, "operations", entry.Name()))
		}
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, found, err := s.repository.LoadConfiguration(configurationKey(path))
		if err != nil {
			return err
		}
		if !found {
			if !json.Valid(raw) {
				return errors.New("invalid legacy agent configuration")
			}
			if err = s.repository.SaveConfiguration(configurationKey(path), raw); err != nil {
				return err
			}
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

type configurationEntry string

func (e configurationEntry) Name() string      { return string(e) }
func (e configurationEntry) IsDir() bool       { return false }
func (e configurationEntry) Type() os.FileMode { return 0 }
func (e configurationEntry) Info() (os.FileInfo, error) {
	return nil, errors.New("configuration has no file metadata")
}
func (s *Service) operationFiles() ([]os.DirEntry, error) {
	if s.repository == nil {
		return os.ReadDir(filepath.Join(s.dir, "operations"))
	}
	values, err := s.repository.ListConfiguration("mcp.operation.")
	if err != nil {
		return nil, err
	}
	entries := make([]os.DirEntry, 0, len(values))
	for key := range values {
		entries = append(entries, configurationEntry(strings.TrimPrefix(key, "mcp.operation.")+".json"))
	}
	return entries, nil
}
