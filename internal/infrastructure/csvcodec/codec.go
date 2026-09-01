package csvcodec

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	MaxFileBytes  = 8 << 20
	MaxDataRows   = 5000
	MaxCellBytes  = 4 << 10
	MaxSourceCols = 128
	UTF8BOM       = "\uFEFF"
	PreviewRowCap = 50
)

type Delimiter rune

const (
	DelimiterComma     Delimiter = ','
	DelimiterSemicolon Delimiter = ';'
	DelimiterTab       Delimiter = '\t'
)

type Table struct {
	Delimiter Delimiter
	HasBOM    bool
	Headers   []string
	Rows      [][]string
}

func DetectDelimiter(sample string) Delimiter {
	line := sample
	if idx := strings.IndexAny(sample, "\r\n"); idx >= 0 {
		line = sample[:idx]
	}
	counts := map[Delimiter]int{
		DelimiterComma:     strings.Count(line, ","),
		DelimiterSemicolon: strings.Count(line, ";"),
		DelimiterTab:       strings.Count(line, "\t"),
	}
	best := DelimiterComma
	bestCount := -1
	for delimiter, count := range counts {
		if count > bestCount {
			best = delimiter
			bestCount = count
		}
	}
	return best
}

func Parse(data []byte, delimiter Delimiter) (Table, error) {
	if int64(len(data)) > MaxFileBytes {
		return Table{}, &domain.Error{Code: domain.ErrCSVLimitExceeded, Message: "CSV file exceeds the size limit"}
	}
	if !utf8.Valid(data) {
		return Table{}, &domain.Error{Code: domain.ErrCSVInvalidEncoding, Message: "CSV must be UTF-8"}
	}
	table := Table{Delimiter: delimiter}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		table.HasBOM = true
		data = data[3:]
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = rune(delimiter)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = false
	records, err := reader.ReadAll()
	if err != nil {
		return Table{}, &domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "CSV could not be parsed"}
	}
	if len(records) == 0 {
		return Table{}, &domain.Error{Code: domain.ErrCSVInvalidFormat, Message: "CSV has no header row"}
	}
	if len(records)-1 > MaxDataRows {
		return Table{}, &domain.Error{Code: domain.ErrCSVLimitExceeded, Message: "CSV has too many data rows"}
	}
	for _, record := range records {
		if len(record) > MaxSourceCols {
			return Table{}, &domain.Error{Code: domain.ErrCSVLimitExceeded, Message: "CSV has too many columns"}
		}
		for _, cell := range record {
			if len(cell) > MaxCellBytes {
				return Table{}, &domain.Error{Code: domain.ErrCSVLimitExceeded, Message: "a CSV cell exceeds the size limit"}
			}
		}
	}
	table.Headers = records[0]
	if len(records) > 1 {
		table.Rows = records[1:]
	}
	return table, nil
}

func Encode(headers []string, rows [][]string) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString(UTF8BOM)
	writer := csv.NewWriter(&buffer)
	writer.UseCRLF = true
	writer.Comma = ','
	if err := writer.Write(headers); err != nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "CSV could not be written"}
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "CSV could not be written"}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "CSV could not be written"}
	}
	_ = io.NopCloser(&buffer)
	return buffer.Bytes(), nil
}

func HeaderIndex(headers []string, name string) int {
	want := strings.TrimSpace(strings.ToLower(name))
	for i, header := range headers {
		if strings.TrimSpace(strings.ToLower(header)) == want {
			return i
		}
	}
	return -1
}
