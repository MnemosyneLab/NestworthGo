package continuousbackup

import (
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
 CREATE TABLE IF NOT EXISTS backup_retention (target TEXT PRIMARY KEY, record TEXT NOT NULL)`)
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
	rows, err := s.db.Query(`SELECT stream,record FROM backup_streams WHERE target=?`, targetKey(c))
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	result := map[string]streamRecord{}
	for rows.Next() {
		var stream, data string
		var r streamRecord
		if rows.Scan(&stream, &data) != nil || json.Unmarshal([]byte(data), &r) != nil || r.Identity.StreamID != stream {
			return nil, ErrUnavailable
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
