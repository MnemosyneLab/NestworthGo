package sqlite

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Length-delimited, type-tagged SQLite values preserve every stored byte
// without JSON-escaping the already serialized evidence a second time. The
// caller reuses one row buffer; no row values or financial inputs are cached.
func appendCertifiedRow(buf []byte, values []any) ([]byte, error) {
	buf = append(buf, 1)
	buf = binary.LittleEndian.AppendUint64(buf, uint64(len(values)))
	for _, value := range values {
		switch v := value.(type) {
		case nil:
			buf = append(buf, 0)
		case int64:
			buf = append(buf, 1)
			buf = binary.LittleEndian.AppendUint64(buf, uint64(v))
		case float64:
			buf = append(buf, 2)
			buf = binary.LittleEndian.AppendUint64(buf, math.Float64bits(v))
		case string:
			buf = append(buf, 3)
			buf = binary.LittleEndian.AppendUint64(buf, uint64(len(v)))
			buf = append(buf, v...)
		case []byte:
			buf = append(buf, 4)
			buf = binary.LittleEndian.AppendUint64(buf, uint64(len(v)))
			buf = append(buf, v...)
		case bool:
			buf = append(buf, 5)
			if v {
				buf = append(buf, 1)
			} else {
				buf = append(buf, 0)
			}
		default:
			return nil, fmt.Errorf("unsupported certified SQLite value %T", value)
		}
	}
	return buf, nil
}
