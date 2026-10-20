package sqlite

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// This is a separate, default-off C3 candidate. Legacy counting-only
// experiments grant it no authority. It has no persistent cache or trigger.
// A failed no-change confirmation uses one full preparation. The separate
// lineage shadow must be explicitly selected to audit bounded publication.
func (r *Repository) WithCertifiedReviewCandidate() *Repository {
	copy := *r
	copy.reviewCertifiedCandidate = true
	copy.reviewNoChangeCandidate = true
	return &copy
}
func (r *Repository) CertifiedReviewEnabled() bool {
	return r.reviewCertifiedCandidate && !r.reviewFullRecompute
}

// The private key never leaves this DB handle or enters backup/export. A
// restart/another handle cannot bless arbitrary preexisting projection data.
type reviewCertificateKey struct {
	once         sync.Once
	key          [32]byte
	err          error
	issued       atomic.Bool
	connMu       sync.Mutex
	connection   any
	connectionID uint64
	commits      *atomic.Uint64
}

func (r *Repository) reviewMAC(value any) (string, error) {
	k := &r.database.reviewCertificateKey
	k.once.Do(func() { _, k.err = rand.Read(k.key[:]) })
	if k.err != nil {
		return "", k.err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, k.key[:])
	mac.Write([]byte("certified-review-session-v1\x00"))
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
func (r *Repository) certifiedMAC(c domain.ReviewTemporalCertificate) (string, error) {
	c.CertifiedMAC = ""
	return r.reviewMAC(c)
}
func (r *Repository) validReviewMAC(c *domain.ReviewTemporalCertificate) bool {
	if c == nil || c.NoChangeCompletenessVersion != domain.ReviewCertifiedCompletenessVersion || c.CertifiedMAC == "" {
		return false
	}
	mac, err := r.certifiedMAC(*c)
	return err == nil && hmac.Equal([]byte(mac), []byte(c.CertifiedMAC))
}
func (r *Repository) authenticReview(c *domain.ReviewTemporalCertificate, seal string) bool {
	return c != nil && c.CertifiedProjectionSeal == seal && r.validReviewMAC(c)
}

func (r *Repository) sourceCaptureMAC(c domain.ReviewProjectionCapture) (string, error) {
	suffixDigest, err := domain.ReviewSuffixFingerprint(c.Suffix)
	if err != nil {
		return "", err
	}
	proof := *c.Certified
	proof.SourceMAC = ""
	proof.Provider = domain.ReviewProviderSelection{}
	return r.reviewMAC(struct {
		Household   domain.HouseholdID
		Fingerprint string
		Cutoff      any
		Source      *domain.ReviewSourceCheckpoint
		Control     domain.ReviewProjectionControl
		Temporal    string
		Proof       domain.ReviewCertifiedCapture
		Suffix      string
	}{c.HouseholdID, c.HistoricalFingerprint, c.SourceCutoff, c.SourceRevision, c.Control, reviewTemporalDigest(c.Temporal), proof, suffixDigest})
}

// Complete row bytes, including all daily JSON, historical conditions, frozen
// lifecycle JSON and acknowledgement identities. This is deliberately O(rows),
// not a narrow fingerprint or an assumption that conditional writes repair it.
func certifiedProjectionSeal(ctx context.Context, tx *sql.Tx, id domain.HouseholdID) (string, error) {
	digest := sha256.New()
	digest.Write([]byte("certified-row-bytes-v2\x00"))
	var rowbuf []byte
	queries := []string{
		`SELECT * FROM investment_review_control WHERE household_id=? ORDER BY household_id`,
		`SELECT * FROM investment_review_rules WHERE household_id=? ORDER BY id`,
		`SELECT v.* FROM investment_review_rule_versions v JOIN investment_review_rules r ON r.id=v.rule_id WHERE r.household_id=? ORDER BY v.rule_id,v.condition_version`,
		`SELECT t.* FROM investment_review_rule_targets t JOIN investment_review_rules r ON r.id=t.rule_id WHERE r.household_id=? ORDER BY t.rule_id,t.instrument_id`,
		`SELECT * FROM investment_review_suppressions WHERE household_id=? ORDER BY id`,
		`SELECT * FROM investment_review_daily_inputs WHERE household_id=? ORDER BY instrument_id,timezone_version,on_date,input_policy`,
		`SELECT d.* FROM investment_review_daily_results d JOIN investment_review_rules r ON r.id=d.rule_id WHERE r.household_id=? ORDER BY d.rule_id,d.condition_version,d.instrument_id,d.timezone_version,d.on_date`,
		`SELECT * FROM investment_review_position_epochs WHERE household_id=? ORDER BY id`,
		`SELECT s.* FROM investment_review_evaluation_state s JOIN investment_review_rules r ON r.id=s.rule_id WHERE r.household_id=? ORDER BY s.rule_id,s.instrument_id,s.position_epoch_id`,
		`SELECT e.* FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? ORDER BY e.id`,
		`SELECT v.* FROM investment_review_episode_events v JOIN investment_review_episodes e ON e.id=v.episode_id JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? ORDER BY v.id`,
	}
	for i, q := range queries {
		rowbuf = append(rowbuf[:0], 0) // table boundary; row records start with 1
		rowbuf = binary.LittleEndian.AppendUint64(rowbuf, uint64(i))
		digest.Write(rowbuf)
		rows, err := tx.QueryContext(ctx, q, id.String())
		if err != nil {
			return "", err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		values, ptrs := make([]any, len(cols)), make([]any, len(cols))
		for j := range values {
			ptrs[j] = &values[j]
		}
		for rows.Next() {
			if err = rows.Scan(ptrs...); err != nil {
				break
			}
			rowbuf, err = appendCertifiedRow(rowbuf[:0], values)
			if err != nil {
				break
			}
			digest.Write(rowbuf)
		}
		if err = errors.Join(err, rows.Err(), rows.Close()); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (r *Repository) verifiedCertifiedTracking(ctx context.Context, tx schemaQuery) (int, bool, error) {
	var cookie, fk, version int
	for _, p := range []struct {
		sql  string
		into *int
	}{{`PRAGMA schema_version`, &cookie}, {`PRAGMA foreign_keys`, &fk}, {`PRAGMA user_version`, &version}} {
		if err := tx.QueryRowContext(ctx, p.sql).Scan(p.into); err != nil {
			return 0, false, err
		}
	}
	if cookie == 0 || cookie != r.database.reviewVerifiedSchemaVersion || fk != 1 || version != CurrentSchemaVersion {
		return cookie, false, nil
	}
	var temporaryTriggers int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_temp_master WHERE name NOT LIKE 'sqlite_%'`).Scan(&temporaryTriggers); err != nil {
		return cookie, false, err
	}
	if temporaryTriggers != 0 {
		return cookie, false, nil
	}
	trusted, err := verifyCertifiedSchema(ctx, tx)
	if err != nil || !trusted {
		return cookie, false, err
	}
	trusted, err = verifyCertifiedTriggers(ctx, tx)
	if err != nil || !trusted {
		return cookie, false, err
	}
	if err := verifyFinancialSourceTrackingState(ctx, tx, true); err != nil {
		return cookie, false, ctx.Err()
	}
	return cookie, true, nil
}

func (r *Repository) captureCertifiedReview(ctx context.Context, tx *sql.Tx, c *domain.ReviewProjectionCapture, connection domain.ReviewConnectionCheckpoint) error {
	proof := &domain.ReviewCertifiedCapture{Connection: connection}
	var err error
	c.Certified = proof
	// Equality is meaningful only on this physical connection. A valid
	// signed post-publication counter pair proves every stored row unchanged;
	// TEMP/permanent writer assumptions are still checked below in this tx.
	seal := ""
	previous := c.PublishedTemporal
	if previous != nil && previous.CertifiedProjectionSeal != "" && r.validReviewMAC(previous) && validCertifiedConnection(connection) && connection == previous.CertifiedConnection {
		seal = previous.CertifiedProjectionSeal
	} else {
		seal, err = certifiedProjectionSeal(ctx, tx, c.HouseholdID)
		if err != nil {
			return err
		}
	}
	proof.ProjectionSeal = seal
	err = tx.QueryRowContext(ctx, `SELECT certificate_json FROM review_temporal_checkpoints WHERE household_id=?`, c.HouseholdID.String()).Scan(&proof.TemporalJSON)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	proof.ReviewFingerprint, err = noChangeReviewFingerprint(ctx, tx, c.HouseholdID)
	if err != nil {
		return err
	}
	counts, err := noChangeProjectionCounts(ctx, tx, c.HouseholdID)
	if err != nil {
		return err
	}
	empty := true
	for _, n := range counts {
		empty = empty && n == 0
	}
	empty = empty && c.PublishedTemporal == nil && c.Control.PublishedFingerprint == "" && !r.database.reviewCertificateKey.issued.Load()
	if empty && r.reviewInitialConnectionMatches(connection) {
		var prior int
		if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM investment_review_mutation_receipts WHERE household_id=? AND json_type(result_json,'$.episodeId') IS NOT NULL)+(SELECT count(*) FROM investment_review_legacy_state WHERE household_id=?)+(SELECT count(*) FROM investment_review_legacy_quarantine)`, c.HouseholdID.String(), c.HouseholdID.String()).Scan(&prior); err != nil {
			return err
		}
		if prior == 0 {
			proof.InitialPublication = true
		} else {
			r.database.reviewInitialPublicationAllowed.Store(false)
		}
	}
	proof.MayCertify = validCertifiedConnection(connection) && (proof.InitialPublication || r.authenticReview(c.PublishedTemporal, seal))
	var verified bool
	proof.SchemaCookie, verified, err = r.verifiedCertifiedTracking(ctx, tx)
	if err != nil {
		return err
	}
	if !verified || c.SourceRevision == nil || !c.SourceRevision.Valid() {
		proof.MayCertify = false
		return nil
	}
	if c.PublishedTemporal != nil {
		proof.PreviousProvider = c.PublishedTemporal.CertifiedProvider
	}
	if !proof.MayCertify && !r.database.reviewCertificateKey.issued.Load() && validCertifiedConnection(connection) {
		proof.MayBootstrap, err = certifiedBootstrapSupported(ctx, tx, *c, counts)
		if err != nil {
			return err
		}
	}
	proof.SourceMAC, err = r.sourceCaptureMAC(*c)
	return err
}

// The caller holds the application provider lease through COMMIT. Direct
// legacy publication never enters this shortcut. Any current projection or
// review mutation conflicts, even when row counts/revisions were not advanced.
func (r *Repository) verifyCertifiedPublication(ctx context.Context, tx *sql.Tx, c domain.ReviewProjectionCapture, leased bool, connectionID uint64) (*domain.ReviewSourceCheckpoint, bool, bool, error) {
	if !r.CertifiedReviewEnabled() || !leased || c.Certified == nil || !c.Certified.Provider.Pinned {
		return nil, false, false, nil
	}
	mac, err := r.sourceCaptureMAC(c)
	if err != nil {
		return nil, false, false, err
	}
	if c.Certified.SourceMAC == "" || !hmac.Equal([]byte(mac), []byte(c.Certified.SourceMAC)) {
		return nil, false, false, nil
	}
	cookie, ok, err := r.verifiedCertifiedTracking(ctx, tx)
	if err != nil {
		return nil, false, false, err
	}
	if !ok || cookie != c.Certified.SchemaCookie {
		return nil, false, false, nil
	}
	var resolver string
	if err := tx.QueryRowContext(ctx, `SELECT resolver_policy_version FROM history_snapshot_state WHERE household_id=?`, c.HouseholdID.String()).Scan(&resolver); err != nil {
		return nil, false, false, err
	}
	current, err := readReviewSourceCheckpoint(ctx, tx, c.HouseholdID, resolver)
	if err != nil {
		return nil, false, false, err
	}
	if current == nil || c.SourceRevision == nil || *current != *c.SourceRevision {
		return nil, false, false, reviewRuleConflict("certified financial namespace changed")
	}
	observed, err := r.certifiedConnectionCheckpoint(ctx, tx, connectionID)
	if err != nil {
		return nil, false, false, err
	}
	if validCertifiedConnection(observed) && validCertifiedConnection(c.Certified.Connection) && observed == c.Certified.Connection {
		// No intervening DML/commit since before capture's first financial
		// read: its complete protected snapshot and metadata are unchanged.
		return current, true, true, nil
	}
	// Different/unknown counters never assume the projection unchanged.
	// Recheck its complete byte seal, then retain the original source reload.
	seal, err := certifiedProjectionSeal(ctx, tx, c.HouseholdID)
	if err != nil {
		return nil, false, false, err
	}
	meta, err := noChangeReviewFingerprint(ctx, tx, c.HouseholdID)
	if err != nil {
		return nil, false, false, err
	}
	if seal != c.Certified.ProjectionSeal || meta != c.Certified.ReviewFingerprint {
		return nil, false, false, reviewRuleConflict("certified projection or review changed")
	}
	return current, false, validCertifiedConnection(observed), nil
}

// Initial empty namespaces and authenticated same-session chains may certify.
// Legacy/restart bootstrap is disabled: complete replacement cannot prove
// missing handling history. A private initial claim must also be consumed by
// the publisher; a checksum or empty namespace alone never grants trust.
func (r *Repository) certifyPublishedReview(ctx context.Context, tx *sql.Tx, c domain.ReviewProjectionCapture, out *domain.ReviewTemporalCertificate, leased bool, connectionID uint64, bootstrap bool) error {
	bootstrap = bootstrap && reviewBootstrapProvenanceEnabled
	if r.CertifiedReviewEnabled() {
		out.NoChangeCompletenessVersion = ""
	}
	out.CertifiedNeedsReconcile = false
	out.CertifiedMAC = ""
	out.CertifiedProjectionSeal = ""
	out.CertifiedProvider = domain.ReviewProviderSelection{}
	out.CertifiedConnection = domain.ReviewConnectionCheckpoint{}
	if !r.CertifiedReviewEnabled() || !leased || c.Certified == nil || (!c.Certified.MayCertify && !(bootstrap && c.Certified.MayBootstrap)) || !c.Certified.Provider.Pinned {
		return nil
	}
	mac, err := r.sourceCaptureMAC(c)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(mac), []byte(c.Certified.SourceMAC)) {
		return nil
	}
	out.CertifiedProjectionSeal, err = certifiedProjectionSeal(ctx, tx, c.HouseholdID)
	if err != nil {
		return err
	}
	if r.reviewDependencySuffix {
		out.SuffixQuoteSeals, err = domain.ReviewQuoteDaySeals(c.Historical)
		if err != nil {
			return err
		}
		out.SuffixNonQuoteFingerprint, err = domain.ReviewNonQuoteFingerprint(c.Historical)
		if err != nil {
			return err
		}
	}
	out.CertifiedProvider = c.Certified.Provider
	out.NoChangeCompletenessVersion = domain.ReviewCertifiedCompletenessVersion
	if err := r.prepareCertifiedCounter(ctx, tx, out, connectionID); err != nil {
		return err
	}
	out.CertifiedMAC, err = r.certifiedMAC(*out)
	return err
}

// Revisioned review commands are the non-publication writers of sealed
// control/configuration/handling rows. Preserve a valid chain inside its existing transaction, never heal an
// invalid one. Temporal state/rule revisions stay old, forcing reconciliation.
func (r *Repository) beforeCertifiedHandling(ctx context.Context, tx *sql.Tx, id domain.HouseholdID) (*domain.ReviewTemporalCertificate, error) {
	if !r.CertifiedReviewEnabled() {
		return nil, nil
	}
	_, trusted, err := r.verifiedCertifiedTracking(ctx, tx)
	if err != nil || !trusted {
		return nil, err
	}
	c, err := readReviewTemporal(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	seal, err := certifiedProjectionSeal(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if !r.authenticReview(c, seal) {
		return nil, nil
	}
	return c, nil
}
func (r *Repository) afterCertifiedHandling(ctx context.Context, tx *sql.Tx, id domain.HouseholdID, c *domain.ReviewTemporalCertificate) error {
	if c == nil {
		return nil
	}
	_, trusted, err := r.verifiedCertifiedTracking(ctx, tx)
	if err != nil || !trusted {
		return err
	}
	old, err := json.Marshal(c)
	if err != nil {
		return err
	}
	c.CertifiedNeedsReconcile = true
	c.CertifiedProjectionSeal, err = certifiedProjectionSeal(ctx, tx, id)
	if err != nil {
		return err
	}
	// Handling may use a different pooled physical connection. The full
	// authenticated row seal remains valid, but no connection continuity is
	// asserted until the mandatory full reconciliation publishes afresh.
	c.CertifiedConnection = domain.ReviewConnectionCheckpoint{}
	c.CertifiedMAC, err = r.certifiedMAC(*c)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE review_temporal_checkpoints SET certificate_json=? WHERE household_id=? AND certificate_json=?`, string(raw), id.String(), string(old))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return reviewRuleConflict("certified handling CAS failed")
	}
	return nil
}

func (r *Repository) persistCertifiedTemporal(ctx context.Context, tx *sql.Tx, c domain.ReviewProjectionCapture, out domain.ReviewTemporalCertificate, now time.Time) error {
	expected := ""
	if c.Certified != nil {
		expected = c.Certified.TemporalJSON
	} else {
		err := tx.QueryRowContext(ctx, `SELECT certificate_json FROM review_temporal_checkpoints WHERE household_id=?`, c.HouseholdID.String()).Scan(&expected)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	var result sql.Result
	if expected == "" {
		result, err = tx.ExecContext(ctx, `INSERT INTO review_temporal_checkpoints(household_id,evaluated_at,next_transition_at,certificate_json) VALUES(?,?,?,?)`, c.HouseholdID.String(), now.UTC().Format(time.RFC3339Nano), out.NextTransitionAt.UTC().Format(time.RFC3339Nano), string(raw))
	} else {
		result, err = tx.ExecContext(ctx, `UPDATE review_temporal_checkpoints SET evaluated_at=?,next_transition_at=?,certificate_json=? WHERE household_id=? AND certificate_json=?`, now.UTC().Format(time.RFC3339Nano), out.NextTransitionAt.UTC().Format(time.RFC3339Nano), string(raw), c.HouseholdID.String(), expected)
	}
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return reviewRuleConflict("certified temporal CAS failed")
	}
	return nil
}
