package csvcodec

import "github.com/waltwang/nestworth-go/internal/application"

// Adapter implements application.CSVCodecPort using this package's codec.
type Adapter struct{}

func (Adapter) Parse(data []byte, delimiter rune) (application.CSVTable, error) {
	table, err := Parse(data, Delimiter(delimiter))
	if err != nil {
		return application.CSVTable{}, err
	}
	return application.CSVTable{
		Delimiter: rune(table.Delimiter),
		HasBOM:    table.HasBOM,
		Headers:   table.Headers,
		Rows:      table.Rows,
	}, nil
}

func (Adapter) Encode(headers []string, rows [][]string) ([]byte, error) {
	return Encode(headers, rows)
}

func (Adapter) DetectDelimiter(sample string) rune {
	return rune(DetectDelimiter(sample))
}
