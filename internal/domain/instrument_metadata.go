package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type InstrumentMetadataChanges struct {
	Name      json.RawMessage `json:"name,omitempty"`
	ShortName json.RawMessage `json:"shortName,omitempty"`
	Note      json.RawMessage `json:"note,omitempty"`
}
type InstrumentMetadataUpdate struct {
	ID                      string                    `json:"id"`
	ExpectedMetadataVersion string                    `json:"expectedMetadataVersion"`
	Changes                 InstrumentMetadataChanges `json:"changes"`
}
type InstrumentMetadataState struct {
	Instrument Instrument
	Managed    bool
}
type InstrumentMetadataReceiptItem struct {
	ID              string `json:"id"`
	Changed         bool   `json:"changed"`
	MetadataVersion string `json:"metadataVersion"`
}
type InstrumentMetadataReceipt struct {
	RequestedCount int                             `json:"requestedCount"`
	ChangedCount   int                             `json:"changedCount"`
	Items          []InstrumentMetadataReceiptItem `json:"items"`
}

func jsonNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func nonemptyMetadataString(field string, raw json.RawMessage) (string, error) {
	var value string
	if jsonNull(raw) || jsoncodec.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", validation(field, "must be a nonempty string; null clears")
	}
	return value, nil
}

func InstrumentMetadataVersion(i Instrument, managed bool) string {
	b, _ := jsoncodec.Marshal(struct {
		ID              InstrumentID
		Name, ShortName string
		Note            *string
		ArchivedAt      *time.Time
		Managed         bool
	}{i.ID, i.Name, i.ShortName, i.Note, i.ArchivedAt, managed})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func ApplyInstrumentMetadata(i Instrument, c InstrumentMetadataChanges, now time.Time) (Instrument, bool, error) {
	before := i
	if len(c.Name) == 0 && len(c.ShortName) == 0 && len(c.Note) == 0 {
		return i, false, validation("changes", "at least one metadata field is required")
	}
	if len(c.Name) > 0 {
		var v string
		if string(c.Name) == "null" || jsoncodec.Unmarshal(c.Name, &v) != nil {
			return i, false, validation("name", "must be a nonempty string")
		}
		name, err := validateName("name", v)
		if err != nil {
			return i, false, err
		}
		i.Name = name
	}
	if len(c.ShortName) > 0 {
		if jsonNull(c.ShortName) {
			i.ShortName = ""
		} else {
			name, err := nonemptyMetadataString("shortName", c.ShortName)
			if err != nil {
				return i, false, err
			}
			normalized, err := NormalizeInstrumentShortName(name)
			if err != nil {
				return i, false, err
			}
			i.ShortName = normalized
		}
	}
	if len(c.Note) > 0 {
		if jsonNull(c.Note) {
			i.Note = nil
		} else {
			text, err := nonemptyMetadataString("note", c.Note)
			if err != nil {
				return i, false, err
			}
			note, err := validateNote("note", &text)
			if err != nil {
				return i, false, err
			}
			i.Note = note
		}
	}
	changed := i.Name != before.Name || i.ShortName != before.ShortName || !reflect.DeepEqual(i.Note, before.Note)
	if changed {
		i.UpdatedAt = now
	}
	return i, changed, nil
}

type InstrumentMetadataReadQuery struct {
	IncludeArchived bool
	IDs             []string
	Types           []string
}
type InstrumentMetadataReadSnapshot struct {
	Items      []InstrumentMetadataState
	TotalCount int
}
