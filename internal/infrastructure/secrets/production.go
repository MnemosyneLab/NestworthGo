package secrets

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
)

// productionStore prefers the native OS secret store and keeps a credential
// only in memory when that store is unavailable or locked. This makes the
// configuration flow usable in restricted environments while preserving an
// explicit session_only status instead of claiming the key is durable.
type productionStore struct {
	native  application.SecretStore
	session *MemoryStore
}

func NewProductionStore() application.SecretStore {
	return &productionStore{native: newKeychainStore(), session: NewUnavailableStore()}
}

func (s *productionStore) Status(ctx context.Context, ref application.SecretRef) (application.SecretStatus, error) {
	status, err := s.native.Status(ctx, ref)
	if err == nil && status != application.SecretStatusUnavailable && status != application.SecretStatusLocked {
		return status, nil
	}
	return s.session.Status(ctx, ref)
}

func (s *productionStore) Get(ctx context.Context, ref application.SecretRef) ([]byte, application.SecretStatus, error) {
	status, statusErr := s.native.Status(ctx, ref)
	if statusErr == nil && status == application.SecretStatusAvailable {
		return s.native.Get(ctx, ref)
	}
	if statusErr == nil && status == application.SecretStatusMissing {
		if value, sessionStatus, err := s.session.Get(ctx, ref); err != nil || sessionStatus == application.SecretStatusSessionOnly {
			return value, sessionStatus, err
		}
		return nil, application.SecretStatusMissing, nil
	}
	return s.session.Get(ctx, ref)
}

func (s *productionStore) Put(ctx context.Context, ref application.SecretRef, value []byte) (application.SecretStatus, error) {
	status, err := s.native.Put(ctx, ref, value)
	if err == nil && status == application.SecretStatusAvailable {
		return status, nil
	}
	return s.session.Put(ctx, ref, value)
}

func (s *productionStore) Delete(ctx context.Context, ref application.SecretRef) (application.SecretStatus, error) {
	nativeStatus, nativeErr := s.native.Delete(ctx, ref)
	sessionStatus, sessionErr := s.session.Delete(ctx, ref)
	if sessionErr != nil {
		return "", sessionErr
	}
	if nativeErr != nil {
		return sessionStatus, nil
	}
	if nativeStatus == application.SecretStatusAvailable || nativeStatus == application.SecretStatusMissing {
		return application.SecretStatusMissing, nil
	}
	return sessionStatus, nil
}
