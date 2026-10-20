package sqlite

import (
	"context"
	"reflect"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Called only after this Open initialized a new schema, never for a reopened,
// upgraded or restored file. Root trust comes from that initialization and the
// controlled writer lifecycle, not an empty projection or a stored checksum.
func (db *DB) establishReviewGenesis() {
	ctx := context.Background()
	conn, err := db.SQL.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()
	var rawIdentity any
	if err := conn.Raw(func(raw any) error {
		if reflect.TypeOf(raw).Comparable() {
			rawIdentity = raw
		}
		return nil
	}); err != nil || rawIdentity == nil {
		return
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var households, prior int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM households`).Scan(&households); err != nil || households != 0 {
		return
	}
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM investment_review_episodes)+(SELECT count(*) FROM investment_review_episode_events)+(SELECT count(*) FROM investment_review_mutation_receipts)+(SELECT count(*) FROM investment_review_legacy_state)+(SELECT count(*) FROM investment_review_legacy_quarantine)+(SELECT count(*) FROM review_temporal_checkpoints)`).Scan(&prior); err != nil || prior != 0 {
		return
	}
	var version int64
	if err := tx.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&version); err != nil {
		return
	}
	if err := tx.Commit(); err != nil {
		return
	}
	db.reviewGenesisConnection, db.reviewGenesisDataVersion = rawIdentity, version
	db.reviewInitialPublicationAllowed.Store(true)
}

func (r *Repository) reviewInitialConnectionMatches(p domain.ReviewConnectionCheckpoint) bool {
	db := r.database
	if !db.reviewInitialPublicationAllowed.Load() || !validCertifiedConnection(p) || p.DataVersion != db.reviewGenesisDataVersion {
		return false
	}
	key := &db.reviewCertificateKey
	key.connMu.Lock()
	defer key.connMu.Unlock()
	return key.connectionID == p.Connection && key.connection == db.reviewGenesisConnection
}
