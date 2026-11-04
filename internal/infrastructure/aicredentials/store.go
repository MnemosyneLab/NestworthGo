// Package aicredentials stores AI credentials in the live household database.
// Full database backups include these plaintext values, like other provider keys.
package aicredentials

import (
	"context"
	"errors"

	"github.com/waltwang/nestworth-go/internal/assistant"
)

const configurationKey = "assistant.credentials.v1"

var errUnavailable = errors.New("assistant_credentials_unavailable")

// Repository is the existing app_configuration port. It follows the live
// database handle across backup/restore; no paths or household IDs come from IPC.
type Repository interface {
	LoadConfiguration(string) ([]byte, bool, error)
	SaveConfiguration(string, []byte) error
}
type store struct{ repository Repository }

func New(repository Repository) assistant.Credentials { return &store{repository: repository} }
func (s *store) Available() bool {
	if s.repository == nil {
		return false
	}
	value, _, err := s.repository.LoadConfiguration(configurationKey)
	clear(value)
	return err == nil
}
func (s *store) Put(ctx context.Context, key []byte) error {
	if ctx.Err() != nil || s.repository == nil || len(key) < 8 || len(key) > 4096 {
		return errUnavailable
	}
	if s.repository.SaveConfiguration(configurationKey, key) != nil {
		return errUnavailable
	}
	return nil
}
func (s *store) With(ctx context.Context, use func([]byte) error) error {
	if ctx.Err() != nil || s.repository == nil {
		return errUnavailable
	}
	key, found, err := s.repository.LoadConfiguration(configurationKey)
	defer clear(key)
	if err != nil || !found || len(key) == 0 {
		return errUnavailable
	}
	return use(key)
}
func (s *store) Delete(ctx context.Context) error {
	if ctx.Err() != nil || s.repository == nil {
		return errUnavailable
	}
	// Empty tombstone uses the established configuration upsert without a schema
	// change. It does not erase prior backups or promise forensic disk erasure.
	if s.repository.SaveConfiguration(configurationKey, nil) != nil {
		return errUnavailable
	}
	return nil
}
