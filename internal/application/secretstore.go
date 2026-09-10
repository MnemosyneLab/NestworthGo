package application

import "context"

const TiingoSecretName = "tiingo_api_key"

type SecretRef struct {
	Name string
}

type SecretStatus string

const (
	SecretStatusMissing     SecretStatus = "missing"
	SecretStatusAvailable   SecretStatus = "available"
	SecretStatusUnavailable SecretStatus = "unavailable"
	SecretStatusLocked      SecretStatus = "locked"
	SecretStatusSessionOnly SecretStatus = "session_only"
)

// SecretStore holds provider credentials outside SQLite and backups.
// OS-backed stores are the production target; memory and session-only
// implementations exist for tests and unavailable/locked OS stores.
type SecretStore interface {
	Status(context.Context, SecretRef) (SecretStatus, error)
	Get(context.Context, SecretRef) ([]byte, SecretStatus, error)
	Put(context.Context, SecretRef, []byte) (SecretStatus, error)
	Delete(context.Context, SecretRef) (SecretStatus, error)
}

func TiingoKeyConfigured(status SecretStatus) bool {
	return status == SecretStatusAvailable || status == SecretStatusSessionOnly
}

func TiingoSecretRef() SecretRef {
	return SecretRef{Name: TiingoSecretName}
}
