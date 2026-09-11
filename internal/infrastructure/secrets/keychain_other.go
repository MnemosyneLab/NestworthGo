//go:build !darwin

package secrets

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
)

// The first desktop target is macOS. Other platforms deliberately report an
// unavailable native store so the production wrapper can keep a session-only
// credential without pretending it is durable.
type unavailableKeychainStore struct{}

func newKeychainStore() application.SecretStore { return unavailableKeychainStore{} }

func (unavailableKeychainStore) Status(context.Context, application.SecretRef) (application.SecretStatus, error) {
	return application.SecretStatusUnavailable, nil
}

func (unavailableKeychainStore) Get(context.Context, application.SecretRef) ([]byte, application.SecretStatus, error) {
	return nil, application.SecretStatusUnavailable, nil
}

func (unavailableKeychainStore) Put(context.Context, application.SecretRef, []byte) (application.SecretStatus, error) {
	return application.SecretStatusUnavailable, nil
}

func (unavailableKeychainStore) Delete(context.Context, application.SecretRef) (application.SecretStatus, error) {
	return application.SecretStatusUnavailable, nil
}
