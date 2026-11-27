package sqlite

import "github.com/waltwang/nestworth-go/internal/testfixture/storage"

func fixtureWriter(repo *Repository) *storage.Writer {
	return storage.NewWriter(storage.Handle{SQL: repo.database.SQL, WithTx: repo.database.WithTx})
}
