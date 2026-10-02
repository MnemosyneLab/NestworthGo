package continuousbackup

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// This local ledger is the only ownership authority. Remote identity alone,
// a previous process's open row, and historical global status never confer it.
type streamRecord struct {
	Identity       streamIdentity
	State          string
	SealedAt       string
	FinalTXID      string
	Objects        []storedObject
	DeletedObjects int
}
type storedObject struct {
	Key  string
	Size int64
	ETag string
}

func targetKey(c config) string {
	return strings.ToLower(c.AccountID) + "/" + c.Bucket + "/" + c.BackupID
}
func (s *configStore) initLedger() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS backup_streams (target TEXT NOT NULL, stream TEXT NOT NULL, record TEXT NOT NULL, PRIMARY KEY(target,stream));
 CREATE TABLE IF NOT EXISTS backup_retention (target TEXT PRIMARY KEY, record TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS backup_stream_progress (target TEXT NOT NULL, stream TEXT NOT NULL, deleted_objects INTEGER NOT NULL CHECK(deleted_objects >= 0), PRIMARY KEY(target,stream))`)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *configStore) createStream(c config, identity streamIdentity) error {
	data, _ := json.Marshal(streamRecord{Identity: identity, State: "open"})
	_, err := s.db.Exec(`INSERT INTO backup_streams VALUES(?,?,?)`, targetKey(c), identity.StreamID, string(data))
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *configStore) streams(c config) (map[string]streamRecord, error) {
	rows, err := s.db.Query(`SELECT s.stream,s.record,p.deleted_objects FROM backup_streams s LEFT JOIN backup_stream_progress p ON p.target=s.target AND p.stream=s.stream WHERE s.target=?`, targetKey(c))
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	result := map[string]streamRecord{}
	for rows.Next() {
		var stream, data string
		var r streamRecord
		var completed sql.NullInt64
		if rows.Scan(&stream, &data, &completed) != nil || json.Unmarshal([]byte(data), &r) != nil || r.Identity.StreamID != stream {
			return nil, ErrUnavailable
		}
		// Older deleting rows keep their manifest/counter in record. New
		// checkpoints are tiny independent rows; never rewrite that manifest.
		if completed.Valid && completed.Int64 > int64(r.DeletedObjects) {
			r.DeletedObjects = int(completed.Int64)
		}
		result[stream] = r
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}
func (s *configStore) writeStream(c config, r streamRecord) error {
	data, _ := json.Marshal(r)
	result, err := s.db.Exec(`UPDATE backup_streams SET record=? WHERE target=? AND stream=?`, string(data), targetKey(c), r.Identity.StreamID)
	if err != nil {
		return ErrUnavailable
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return ErrUnavailable
	}
	return nil
}
func (s *configStore) seal(c config, identity streamIdentity, txid string, confirmed time.Time) error {
	rows, err := s.streams(c)
	if err != nil {
		return err
	}
	r, ok := rows[identity.StreamID]
	if !ok || r.Identity != identity || r.State != "open" || txid == "" || confirmed.IsZero() {
		return ErrUnavailable
	}
	started, err := time.Parse(time.RFC3339Nano, identity.StartedAt)
	if err != nil || confirmed.Before(started) {
		return ErrUnavailable
	}
	r.State = "sealed"
	r.SealedAt = confirmed.UTC().Format(time.RFC3339Nano)
	r.FinalTXID = txid
	return s.writeStream(c, r)
}

// checkpointStream stores only monotonic advisory progress. Resume safety still
// comes from matching the complete immutable manifest against remote inventory,
// including objects deleted before a lost response or an uncommitted checkpoint.
func (s *configStore) checkpointStream(c config, stream string, completed int) error {
	if completed < 0 {
		return ErrUnavailable
	}
	_, err := s.db.Exec(`INSERT INTO backup_stream_progress(target,stream,deleted_objects) VALUES(?,?,?)
 ON CONFLICT(target,stream) DO UPDATE SET deleted_objects=excluded.deleted_objects
 WHERE excluded.deleted_objects>backup_stream_progress.deleted_objects`, targetKey(c), stream, completed)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
