// Package repos registers the already-open synthetic database's real
// transaction boundary with the isolated fixture writer.
package repos

import (
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/testfixture/storage"
)

func New(db *sqlite.DB) *sqlite.Repository {
	r := sqlite.NewRepository(db)
	storage.Register(db.Path, storage.Handle{SQL: db.SQL, WithTx: db.WithTx})
	return r
}
