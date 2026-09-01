package csvcodec

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseAndEncodeRoundTrip(t *testing.T) {
	data, err := Encode([]string{"account_name", "currency"}, [][]string{{"Cash", "USD"}, {"quoted,name", "SGD"}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("expected UTF-8 BOM")
	}
	if !bytes.Contains(data, []byte("\r\n")) {
		t.Fatal("expected CRLF")
	}
	table, err := Parse(data, DelimiterComma)
	if err != nil {
		t.Fatal(err)
	}
	if !table.HasBOM || len(table.Rows) != 2 || table.Rows[1][0] != "quoted,name" {
		t.Fatalf("%+v", table)
	}
}

func TestDetectDelimiterPrefersSemicolon(t *testing.T) {
	if DetectDelimiter("a;b;c\n1;2;3") != DelimiterSemicolon {
		t.Fatal(DetectDelimiter("a;b;c"))
	}
}

func TestParseRejectsTooManyRows(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("a\n")
	for i := 0; i < MaxDataRows+1; i++ {
		builder.WriteString("x\n")
	}
	if _, err := Parse([]byte(builder.String()), DelimiterComma); err == nil {
		t.Fatal("expected limit error")
	}
}

func TestParseRejectsTooManyColumnsInDataRow(t *testing.T) {
	data := "header\n" + strings.Repeat("x,", MaxSourceCols) + "x\n"
	if _, err := Parse([]byte(data), DelimiterComma); err == nil {
		t.Fatal("expected data row column limit error")
	}
}
