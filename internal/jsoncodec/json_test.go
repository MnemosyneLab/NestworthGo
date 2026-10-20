package jsoncodec_test

import (
	"bytes"
	"encoding/json" // Number type only; no stdlib codec in the default suite.
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestCanonicalBytes(t *testing.T) {
	value := map[string]any{"z": json.Number("9007199254740993"), "a": "<>&\u2028\u2029", "nil": []string(nil), "empty": []string{}}
	const want = `{"a":"\u003c\u003e\u0026\u2028\u2029","empty":[],"nil":null,"z":9007199254740993}`
	got, err := jsoncodec.Marshal(value)
	if err != nil || string(got) != want {
		t.Fatalf("got %s, %v", got, err)
	}
	var stream bytes.Buffer
	if err := jsoncodec.NewEncoder(&stream).Encode(value); err != nil {
		t.Fatal(err)
	}
	if stream.String() != want+"\n" {
		t.Fatalf("stream = %q", stream.String())
	}
}

func TestStreamFramingAndPrecision(t *testing.T) {
	for _, wrap := range []struct {
		name string
		fn   func(io.Reader) io.Reader
	}{
		{"buffered", func(r io.Reader) io.Reader { return r }},
		{"one-byte", iotest.OneByteReader},
		{"data-and-EOF", iotest.DataErrReader},
	} {
		t.Run(wrap.name, func(t *testing.T) {
			raw := `  01 1e+12 -0.25 {"n":9007199254740993} ["x",null] true false null "end"`
			dec := jsoncodec.NewDecoder(wrap.fn(strings.NewReader(raw)))
			dec.UseNumber()
			for i, want := range []string{"0", "1", "1e+12", "-0.25"} {
				var v any
				if err := dec.Decode(&v); err != nil || v != json.Number(want) {
					t.Fatalf("number %d: %#v / %v", i, v, err)
				}
			}
			var obj map[string]any
			if err := dec.Decode(&obj); err != nil || obj["n"] != json.Number("9007199254740993") {
				t.Fatalf("precision: %#v %v", obj, err)
			}
			for i := 0; i < 5; i++ {
				var v any
				if err := dec.Decode(&v); err != nil {
					t.Fatalf("value %d: %v", i, err)
				}
			}
			var v any
			if err := dec.Decode(&v); err != io.EOF {
				t.Fatalf("end: %v", err)
			}
		})
	}
}

func TestStrictTrailingAndMalformed(t *testing.T) {
	for _, raw := range []string{`{}garbage`, `{} {`, `{} 1e+`, `{} "unfinished`} {
		d := jsoncodec.NewDecoder(strings.NewReader(raw))
		var v any
		if err := d.Decode(&v); err != nil {
			t.Fatal(err)
		}
		if err := d.Decode(&v); err == nil || err == io.EOF {
			t.Fatalf("accepted trailing %q: %v", raw, err)
		}
	}
	for _, raw := range []string{`{"x":"a` + "\x00" + `b"}`, `[1,]`, `01`, `1e+`, `{"x":}`} {
		if jsoncodec.Valid([]byte(raw)) {
			t.Errorf("Valid accepted %q", raw)
		}
	}
	if !jsoncodec.Valid([]byte(`1e400`)) {
		t.Error("valid out-of-float-range number rejected")
	}
}

func TestStreamStrictFieldsAndCase(t *testing.T) {
	type record struct {
		Name string `json:"name"`
	}
	d := jsoncodec.NewDecoder(strings.NewReader(`{"name":"x","extra":1}`))
	d.DisallowUnknownFields()
	var v record
	if err := d.Decode(&v); err == nil {
		t.Fatal("unknown field accepted")
	}
	d = jsoncodec.NewCaseSensitiveDecoder(strings.NewReader(`{"name":"protected","Name":"other","name":null}`))
	if err := d.Decode(&v); err != nil || v.Name != "protected" {
		t.Fatalf("case/null: %#v %v", v, err)
	}
	d = jsoncodec.NewDecoder(strings.NewReader(`{} {}`))
	var a any
	if err := d.Decode(&a); err != nil {
		t.Fatal(err)
	}
	remaining, _ := io.ReadAll(d.Buffered())
	if strings.TrimSpace(string(remaining)) != "{}" {
		t.Fatalf("buffered=%q", remaining)
	}
	if !d.More() {
		t.Fatal("More missed second value")
	}
	if err := d.Decode(&a); err != nil {
		t.Fatal(err)
	}
	if d.More() {
		t.Fatal("More beyond EOF")
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
func TestStreamReaderError(t *testing.T) {
	sentinel := errors.New("synthetic read failure")
	d := jsoncodec.NewDecoder(io.MultiReader(strings.NewReader(`{"x":`), errorReader{sentinel}))
	var v any
	if err := d.Decode(&v); !errors.Is(err, sentinel) {
		t.Fatalf("lost reader error: %v", err)
	}
}
