package jsoncodec

import (
	"bytes"
	"errors"
	"io"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/decoder"
)

// Sonic 1.15.4's stream decoder can discard trailing malformed data as EOF
// and consume both values in "01". Frame with Sonic's strict Skip API, then
// decode each frame through ConfigStd. This keeps all parsing/decoding in
// Sonic and preserves sequential values without reading an entire stream.
// Skip's incomplete-input code is derived through its public API.
var incompleteInput, _ = decoder.Skip(nil)

type streamDecoder struct {
	reader    io.Reader
	buffer    []byte
	readErr   error
	syntaxErr error
	options   sonic.Config
	api       sonic.API
}

func newStreamDecoder(r io.Reader, exactCase bool) *streamDecoder {
	options := standardOptions
	options.CaseSensitive = exactCase
	return &streamDecoder{reader: r, options: options, api: options.Froze()}
}

func (d *streamDecoder) UseNumber() { d.options.UseNumber = true; d.api = d.options.Froze() }
func (d *streamDecoder) DisallowUnknownFields() {
	d.options.DisallowUnknownFields = true
	d.api = d.options.Froze()
}
func (d *streamDecoder) Buffered() io.Reader { return bytes.NewReader(d.buffer) }
func jsonSpace(c byte) bool                  { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
func numberByte(c byte) bool {
	return c >= '0' && c <= '9' || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E'
}

func (d *streamDecoder) readMore() {
	// Grow reads geometrically so framing a large object does not repeatedly
	// rescan it in fixed-size increments. Read-ahead stays in Buffered().
	size := len(d.buffer)
	if size < 512 {
		size = 512
	}
	chunk := make([]byte, size)
	for tries := 0; tries < 100; tries++ {
		n, err := d.reader.Read(chunk)
		d.buffer = append(d.buffer, chunk[:n]...)
		d.readErr = err
		if n > 0 || err != nil {
			return
		}
	}
	d.readErr = io.ErrNoProgress
}

func (d *streamDecoder) skipSpace() bool {
	for {
		n := 0
		for n < len(d.buffer) && jsonSpace(d.buffer[n]) {
			n++
		}
		d.buffer = d.buffer[n:]
		if len(d.buffer) > 0 {
			return true
		}
		if d.readErr != nil {
			return false
		}
		d.readMore()
	}
}

func (d *streamDecoder) More() bool {
	return d.syntaxErr == nil && d.skipSpace() && d.buffer[0] != ']' && d.buffer[0] != '}'
}

func (d *streamDecoder) Decode(value any) error {
	if d.syntaxErr != nil {
		return d.syntaxErr
	}
	if !d.skipSpace() {
		return d.readErr
	}
	for {
		start, end := decoder.Skip(d.buffer)
		number := d.buffer[0] == '-' || (d.buffer[0] >= '0' && d.buffer[0] <= '9')
		literalPrefix := (len(d.buffer) < 4 && bytes.HasPrefix([]byte("true"), d.buffer)) ||
			(len(d.buffer) < 5 && bytes.HasPrefix([]byte("false"), d.buffer)) ||
			(len(d.buffer) < 4 && bytes.HasPrefix([]byte("null"), d.buffer))
		numberPrefix := number
		if numberPrefix {
			for _, c := range d.buffer {
				if !numberByte(c) {
					numberPrefix = false
					break
				}
			}
		}
		// A number at the current buffer boundary may continue in the next read.
		if d.readErr == nil && (start == incompleteInput || (start < 0 && (numberPrefix || literalPrefix || end >= len(d.buffer))) || (start >= 0 && number && end == len(d.buffer))) {
			d.readMore()
			continue
		}
		if start < 0 {
			if start == incompleteInput {
				if d.readErr != nil && d.readErr != io.EOF {
					d.syntaxErr = d.readErr
				} else {
					d.syntaxErr = io.ErrUnexpectedEOF
				}
			} else {
				var discarded any
				d.syntaxErr = d.api.Unmarshal(d.buffer, &discarded)
				if d.syntaxErr == nil {
					d.syntaxErr = errors.New("invalid JSON stream framing")
				}
			}
			return d.syntaxErr
		}
		frame := d.buffer[start:end]
		d.buffer = d.buffer[end:]
		err := d.api.Unmarshal(frame, value)
		var syntax decoder.SyntaxError
		if errors.As(err, &syntax) {
			d.syntaxErr = err
		}
		return err
	}
}
