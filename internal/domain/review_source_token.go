package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ReviewHistoricalFingerprint covers the complete consistent financial read:
// origin, current directory/holding facts, effective activities, corrections,
// quotes, FX, metadata and cost events. Unlike MAX(updated_at), a deletion or
// withdrawal changes this token. The publisher recaptures it in its own SQL
// transaction before writing a completed review projection.
func ReviewHistoricalFingerprint(batch ReviewHistoricalBatch) (string, error) {
	raw, err := json.Marshal(batch)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
