package secrets

import (
	"context"
	"strings"
	"sync"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// MemoryStore is the test and unavailable-OS implementation of SecretStore.
// It never writes SQLite, backups, or plaintext files.
type MemoryStore struct {
	mu          sync.Mutex
	values      map[string][]byte
	backend     application.SecretStatus
	sessionOnly bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{values: map[string][]byte{}, backend: application.SecretStatusAvailable}
}

func NewUnavailableStore() *MemoryStore {
	return &MemoryStore{values: map[string][]byte{}, backend: application.SecretStatusUnavailable, sessionOnly: true}
}

func NewLockedStore() *MemoryStore {
	return &MemoryStore{values: map[string][]byte{}, backend: application.SecretStatusLocked}
}

func (s *MemoryStore) Status(_ context.Context, ref application.SecretRef) (application.SecretStatus, error) {
	if err := validateSecretRef(ref); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked(ref), nil
}

func (s *MemoryStore) Get(_ context.Context, ref application.SecretRef) ([]byte, application.SecretStatus, error) {
	if err := validateSecretRef(ref); err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == application.SecretStatusLocked {
		return nil, application.SecretStatusLocked, nil
	}
	if s.backend == application.SecretStatusUnavailable && !s.sessionOnly {
		return nil, application.SecretStatusUnavailable, nil
	}
	value, ok := s.values[ref.Name]
	if !ok {
		status := application.SecretStatusMissing
		if s.backend == application.SecretStatusUnavailable {
			status = application.SecretStatusUnavailable
		}
		return nil, status, nil
	}
	copied := append([]byte(nil), value...)
	return copied, s.statusLocked(ref), nil
}

func (s *MemoryStore) Put(_ context.Context, ref application.SecretRef, value []byte) (application.SecretStatus, error) {
	if err := validateSecretRef(ref); err != nil {
		return "", err
	}
	if len(bytesTrim(value)) == 0 {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "secret", Message: "secret value is required"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == application.SecretStatusLocked {
		return application.SecretStatusLocked, nil
	}
	if s.backend == application.SecretStatusUnavailable && !s.sessionOnly {
		return application.SecretStatusUnavailable, nil
	}
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	s.values[ref.Name] = append([]byte(nil), value...)
	if s.backend == application.SecretStatusUnavailable {
		return application.SecretStatusSessionOnly, nil
	}
	return application.SecretStatusAvailable, nil
}

func (s *MemoryStore) Delete(_ context.Context, ref application.SecretRef) (application.SecretStatus, error) {
	if err := validateSecretRef(ref); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == application.SecretStatusLocked {
		return application.SecretStatusLocked, nil
	}
	delete(s.values, ref.Name)
	if s.backend == application.SecretStatusUnavailable {
		return application.SecretStatusUnavailable, nil
	}
	return application.SecretStatusMissing, nil
}

func (s *MemoryStore) statusLocked(ref application.SecretRef) application.SecretStatus {
	if s.backend == application.SecretStatusLocked {
		return application.SecretStatusLocked
	}
	_, ok := s.values[ref.Name]
	if s.backend == application.SecretStatusUnavailable {
		if ok {
			return application.SecretStatusSessionOnly
		}
		return application.SecretStatusUnavailable
	}
	if ok {
		return application.SecretStatusAvailable
	}
	return application.SecretStatusMissing
}

func validateSecretRef(ref application.SecretRef) error {
	if strings.TrimSpace(ref.Name) == "" {
		return &domain.Error{Code: domain.ErrValidation, Field: "secret", Message: "secret name is required"}
	}
	return nil
}

func bytesTrim(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}
