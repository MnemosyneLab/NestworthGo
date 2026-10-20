package sqlite

import (
	"context"
	"database/sql"
	"reflect"
	"sync/atomic"
	"time"

	modernsqlite "modernc.org/sqlite"

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
		hooker, ok := raw.(interface {
			RegisterCommitHook(modernsqlite.CommitHookFn)
		})
		if !ok {
			return nil
		} // Unknown driver capability cannot grant continuity.
		key := &r.database.reviewCertificateKey
		key.connMu.Lock()
		defer key.connMu.Unlock()
		if raw != key.connection {
			if key.connectionID == ^uint64(0) {
				return nil
			}
			key.connection = raw // Strong reference prevents address-reuse ABA.
			key.connectionID++
			counter := &atomic.Uint64{}
			key.commits = counter
			// This candidate owns the driver's single commit hook. No SQL and
			// no connection reentry are permitted inside this callback. A hook
			// invocation before a failed COMMIT conservatively invalidates reuse.
			hooker.RegisterCommitHook(func() int32 {
				for {
					old := counter.Load()
					if old == ^uint64(0) || counter.CompareAndSwap(old, old+1) {
						return 0
					}
				}
			})
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

func (r *Repository) certifiedConnectionCheckpoint(ctx context.Context, q schemaQuery, id uint64) (domain.ReviewConnectionCheckpoint, error) {
	proof := domain.ReviewConnectionCheckpoint{Connection: id}
	if err := q.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&proof.DataVersion); err != nil {
		return proof, err
	}
	if err := q.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&proof.OwnChanges); err != nil {
		return proof, err
	}
	key := &r.database.reviewCertificateKey
	key.connMu.Lock()
	if id == 0 || id != key.connectionID || key.commits == nil {
		proof.Connection = 0
	} else {
		proof.OwnCommits = key.commits.Load()
	}
	key.connMu.Unlock()
	return proof, nil
}

func validCertifiedConnection(p domain.ReviewConnectionCheckpoint) bool {
	return p.Connection != 0 && p.OwnCommits < ^uint64(0) && p.DataVersion >= 0 && p.OwnChanges >= 0 && p.OwnChanges < int64(^uint64(0)>>1)
}

// The driver retains registered Go callbacks. Unregister the current hook on a
// live connection before normal DB shutdown; the callback itself captures only
// its small atomic counter, never a DB handle or ledger data.
func (db *DB) releaseReviewCommitHook() {
	key := &db.reviewCertificateKey
	key.connMu.Lock()
	registered := key.commits != nil
	key.connMu.Unlock()
	if !registered || db.SQL.Stats().Idle == 0 {
		return
	}
	// Cleanup must not make Close wait forever for an idle sql.Conn leased
	// by another caller. Normal shutdown is quiescent; a racing lease can
	// only skip callback cleanup, never extend integrity authority.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	conn, err := db.SQL.Conn(ctx)
	if err != nil {
		return
	} // Already closed/replaced pools cannot be reentered.
	defer conn.Close()
	_ = conn.Raw(func(raw any) error {
		key.connMu.Lock()
		defer key.connMu.Unlock()
		if reflect.TypeOf(raw).Comparable() && raw == key.connection {
			if hooker, ok := raw.(interface {
				RegisterCommitHook(modernsqlite.CommitHookFn)
			}); ok {
				hooker.RegisterCommitHook(nil)
				key.commits = nil
			}
		}
		return nil
	})
}

func (r *Repository) prepareCertifiedCounter(ctx context.Context, tx *sql.Tx, c *domain.ReviewTemporalCertificate, id uint64) error {
	proof, err := r.certifiedConnectionCheckpoint(ctx, tx, id)
	if err != nil {
		return err
	}
	if !validCertifiedConnection(proof) {
		return reviewRuleConflict("certified connection counter is unavailable")
	}
	// Mandatory checkpoint INSERT/UPDATE is the final row write and must affect
	// exactly one row. Unknown TEMP triggers cannot add hidden writes.
	proof.OwnChanges++
	proof.OwnCommits++ // This publication always writes, then commits exactly once.
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
