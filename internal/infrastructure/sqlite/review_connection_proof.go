package sqlite

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// C1 namespaces are necessary but not sufficient against someone manually
// rewinding journal/revision metadata after DML. Bind physical connection
// identity plus SQLite's non-writable commit/change counters as an additional
// fence. A different connection or any intervening write selects full recheck.
func (r *Repository) certifiedConnection(ctx context.Context) (*sql.Conn, uint64, error) {
	conn, err := r.database.SQL.Conn(ctx)
	if err != nil {
		return nil, 0, err
	}
	var id uint64
	err = conn.Raw(func(raw any) error {
		if !reflect.TypeOf(raw).Comparable() {
			return nil
		}
		key := &r.database.reviewCertificateKey
		key.connMu.Lock()
		defer key.connMu.Unlock()
		if raw != key.connection {
			if key.connectionID == ^uint64(0) {
				return nil
			}
			key.connection = raw // Strong reference prevents address-reuse ABA.
			key.connectionID++
		}
		id = key.connectionID
		return nil
	})
	if err != nil {
		conn.Close()
		return nil, 0, err
	}
	return conn, id, nil
}

func certifiedConnectionCheckpoint(ctx context.Context, q schemaQuery, id uint64) (domain.ReviewConnectionCheckpoint, error) {
	proof := domain.ReviewConnectionCheckpoint{Connection: id}
	if err := q.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&proof.DataVersion); err != nil {
		return proof, err
	}
	if err := q.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&proof.OwnChanges); err != nil {
		return proof, err
	}
	return proof, nil
}

func validCertifiedConnection(p domain.ReviewConnectionCheckpoint) bool {
	return p.Connection != 0 && p.DataVersion >= 0 && p.OwnChanges >= 0 && p.OwnChanges < int64(^uint64(0)>>1)
}

func (r *Repository) prepareCertifiedCounter(ctx context.Context, tx *sql.Tx, c *domain.ReviewTemporalCertificate, id uint64) error {
	proof, err := certifiedConnectionCheckpoint(ctx, tx, id)
	if err != nil {
		return err
	}
	if !validCertifiedConnection(proof) {
		return reviewRuleConflict("certified connection counter is unavailable")
	}
	// Mandatory checkpoint INSERT/UPDATE is the final row write and must affect
	// exactly one row. Unknown TEMP triggers cannot add hidden writes.
	proof.OwnChanges++
	c.CertifiedConnection = proof
	return nil
}

// Status readers must compare data_version only on the certified physical
// connection too. Passing a stored identity into a different pooled connection
// would give unrelated counters the appearance of provenance.
func (r *Repository) beginReviewStatusRead(ctx context.Context) (*sql.Tx, func(), uint64, error) {
	if !r.CertifiedReviewEnabled() {
		tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		return tx, func() {}, 0, err
	}
	conn, id, err := r.certifiedConnection(ctx)
	if err != nil {
		return nil, func() {}, 0, err
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		conn.Close()
		return nil, func() {}, 0, err
	}
	return tx, func() { _ = conn.Close() }, id, nil
}
