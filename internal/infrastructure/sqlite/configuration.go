package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

// ConfigurationRepository shares the live DB connection, including backup and
// restore lifecycle. Values contain credentials and must never be logged/exported.
type ConfigurationRepository struct{ database *DB }

func NewConfigurationRepository(db *DB) *ConfigurationRepository { return &ConfigurationRepository{db} }
func (r *ConfigurationRepository) LoadConfiguration(key string) ([]byte, bool, error) {
	if r.database == nil || r.database.SQL == nil {
		return nil, false, sql.ErrConnDone
	}
	var data []byte
	err := r.database.SQL.QueryRow("SELECT value FROM app_configuration WHERE key = ?", key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return data, err == nil, err
}
func (r *ConfigurationRepository) SaveConfiguration(key string, value []byte) error {
	if r.database == nil || r.database.SQL == nil {
		return sql.ErrConnDone
	}
	_, err := r.database.SQL.Exec("INSERT INTO app_configuration(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, string(value))
	return err
}
func (r *ConfigurationRepository) ListConfiguration(prefix string) (map[string][]byte, error) {
	if r.database == nil || r.database.SQL == nil {
		return nil, sql.ErrConnDone
	}
	rows, err := r.database.SQL.Query("SELECT key,value FROM app_configuration WHERE substr(key,1,?)=?", len(prefix), prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]byte{}
	for rows.Next() {
		var key string
		var value []byte
		if err = rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, rows.Err()
}
func (r *ConfigurationRepository) CoreSettings() (string, string, error) {
	if r.database == nil || r.database.SQL == nil {
		return "", "", sql.ErrConnDone
	}
	var currency, timezone string
	err := r.database.SQL.QueryRow("SELECT base_currency FROM households LIMIT 1").Scan(&currency)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	err = r.database.SQL.QueryRow("SELECT timezone FROM history_origins LIMIT 1").Scan(&timezone)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	return currency, timezone, nil
}
func migrateV12ToV13(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE app_configuration (key TEXT PRIMARY KEY NOT NULL, value TEXT NOT NULL); PRAGMA user_version = 13;`); err != nil {
		return err
	}
	if err = verifySchema(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
