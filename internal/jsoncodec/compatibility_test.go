//go:build jsoncompat

package jsoncodec_test

// Opt-in migration oracle only. Production and ordinary tests use Sonic;
// the prior stdlib codec is an explicit comparison oracle, never a fallback.
import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	stdjson "encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type spacedJSON struct{}

func (spacedJSON) MarshalJSON() ([]byte, error) {
	return []byte(` { "s": "<>&\u2028", "n": 1.2300 } `), nil
}

type invalidJSON struct{}

func (invalidJSON) MarshalJSON() ([]byte, error) { return []byte(`{"bad":}`), nil }

func compareEncoding(t *testing.T, value any) {
	t.Helper()
	old, oldErr := stdjson.Marshal(value)
	got, err := jsoncodec.Marshal(value)
	if (oldErr == nil) != (err == nil) || (err == nil && !bytes.Equal(old, got)) {
		t.Fatalf("%T encoding differs: old=%q (%v), sonic=%q (%v)", value, old, oldErr, got, err)
	}
	if err != nil {
		return
	}
	if sha256.Sum256(old) != sha256.Sum256(got) {
		t.Fatal("fingerprint differs")
	}
	a, b := hmac.New(sha256.New, []byte("synthetic-key")), hmac.New(sha256.New, []byte("synthetic-key"))
	a.Write(old)
	b.Write(got)
	if !hmac.Equal(a.Sum(nil), b.Sum(nil)) {
		t.Fatal("MAC differs")
	}
	for _, escape := range []bool{true, false} {
		var ob, nb bytes.Buffer
		oe, ne := stdjson.NewEncoder(&ob), jsoncodec.NewEncoder(&nb)
		oe.SetEscapeHTML(escape)
		ne.SetEscapeHTML(escape)
		oe.SetIndent("", "  ")
		ne.SetIndent("", "  ")
		if err := oe.Encode(value); err != nil {
			t.Fatal(err)
		}
		if err := ne.Encode(value); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ob.Bytes(), nb.Bytes()) {
			t.Fatalf("stream bytes differ for %T escape=%v:\nold=%q\nnew=%q", value, escape, ob.Bytes(), nb.Bytes())
		}
	}
	old, oldErr = stdjson.MarshalIndent(value, "prefix", "\t")
	got, err = jsoncodec.MarshalIndent(value, "prefix", "\t")
	if (oldErr == nil) != (err == nil) || !bytes.Equal(old, got) {
		t.Fatalf("indent bytes differ for %T", value)
	}
}

func TestDifferentialEncoding(t *testing.T) {
	stamp := time.Date(2026, 10, 20, 12, 34, 56, 123456789, time.UTC)
	values := []any{nil, []string(nil), []string{}, map[string]any(nil), map[string]any{}, true, "<>&\u2028\u2029\n\t\x00\"\\中文", string([]byte{0xff, 0xfe, 'x'}), math.Copysign(0, -1), math.SmallestNonzeroFloat64, math.MaxFloat64, 1e-7, 1e-6, 1e20, 1e21, stdjson.Number("9007199254740993"), stdjson.RawMessage(` {"z":1, "a":"<>&"} `), spacedJSON{}, invalidJSON{}, decimal.RequireFromString("999999999999.99990000"), stamp, map[int]string{10: "x", 2: "y", -1: "z"}, map[string]any{"z": nil, "a": []byte{0, 1, 255}, "n": stdjson.Number("1.2300e+20")}, domain.ReviewTemporalCertificate{EvaluatedAt: stamp, NextTransitionAt: stamp.Add(time.Hour), PolicyVersion: domain.ReviewTemporalPolicyVersion, ClockEpoch: "<epoch>", CertifiedMAC: "synthetic", ConfigRevision: 9007199254740993}, domain.ReviewHistoricalBatch{}, domain.ReviewSuffixCapture{}}
	for i, v := range values {
		t.Run(fmt.Sprint(i), func(t *testing.T) { compareEncoding(t, v) })
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 10000; i++ {
		f := math.Float64frombits(rng.Uint64())
		if !math.IsInf(f, 0) && !math.IsNaN(f) {
			a, _ := stdjson.Marshal(f)
			b, _ := jsoncodec.Marshal(f)
			if !bytes.Equal(a, b) {
				t.Fatalf("float %g: %s != %s", f, a, b)
			}
		}
	}
}

func TestDifferentialFixtures(t *testing.T) {
	count := 0
	err := filepath.WalkDir("../../testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var old, got any
		od, nd := stdjson.NewDecoder(bytes.NewReader(raw)), jsoncodec.NewDecoder(bytes.NewReader(raw))
		od.UseNumber()
		nd.UseNumber()
		oe, ne := od.Decode(&old), nd.Decode(&got)
		if (oe == nil) != (ne == nil) || !reflect.DeepEqual(old, got) {
			t.Errorf("fixture decode differs %s: %v / %v", path, oe, ne)
			return nil
		}
		if oe == nil {
			compareEncoding(t, old)
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("compared %d JSON fixtures", count)
}

func TestDifferentialDecode(t *testing.T) {
	inputs := []string{`null`, `{"n":9007199254740993,"d":1.23000000000000000001}`, `{"a":1,"a":2}`, `{"s":"\ud800"}`, `"` + string([]byte{0xff}) + `"`, `{"x":}`, `[1,]`, `01`, `1e400`, `{"x":"a` + "\x00" + `b"}`, `{} {}`, `{}garbage`, ``, `{"x": [1,2}`}
	for i, raw := range inputs {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if stdjson.Valid([]byte(raw)) != jsoncodec.Valid([]byte(raw)) {
				t.Errorf("Valid differs for %q", raw)
			}
			for _, number := range []bool{false, true} {
				var old, got any
				od, nd := stdjson.NewDecoder(strings.NewReader(raw)), jsoncodec.NewDecoder(strings.NewReader(raw))
				if number {
					od.UseNumber()
					nd.UseNumber()
				}
				oe, ne := od.Decode(&old), nd.Decode(&got)
				if (oe == nil) != (ne == nil) || (oe == nil && !reflect.DeepEqual(old, got)) {
					t.Errorf("stream number=%v differs for %q: %#v/%v vs %#v/%v", number, raw, old, oe, got, ne)
				}
				if oe == nil && ne == nil {
					oe, ne = od.Decode(&old), nd.Decode(&got)
					if (oe == nil) != (ne == nil) || (oe == io.EOF) != (ne == io.EOF) {
						t.Errorf("trailing decode differs: %v / %v", oe, ne)
					}
				}
			}
			var a, b any
			oe, ne := stdjson.Unmarshal([]byte(raw), &a), jsoncodec.Unmarshal([]byte(raw), &b)
			if (oe == nil) != (ne == nil) || (oe == nil && !reflect.DeepEqual(a, b)) {
				t.Errorf("Unmarshal differs: %q: %#v/%v vs %#v/%v", raw, a, oe, b, ne)
			}
		})
	}
}

func TestDifferentialStrictFields(t *testing.T) {
	type envelope struct {
		Name   string             `json:"name"`
		Params stdjson.RawMessage `json:"params"`
	}
	for _, raw := range []string{`{"name":"protected","Name":"other"}`, `{"name":"protected","name":null}`, `{"params":{"a":1},"params":{"b":2}}`, `{"name":"first","name":"last"}`, `{"Name":"upper"}`, `{"name":"ok"} {}`, `{"name":"ok","extra":1}`, `{"name":12}`} {
		var a, b envelope
		sd := stdjson.NewDecoder(strings.NewReader(raw))
		sd.DisallowUnknownFields()
		nd := jsoncodec.NewDecoder(strings.NewReader(raw))
		nd.DisallowUnknownFields()
		oe, ne := sd.Decode(&a), nd.Decode(&b)
		if (oe == nil) != (ne == nil) || (oe == nil && !reflect.DeepEqual(a, b)) {
			t.Errorf("strict differs %q: %#v/%v vs %#v/%v", raw, a, oe, b, ne)
		}
	}
}
