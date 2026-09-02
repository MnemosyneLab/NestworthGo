package application

import (
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	CSVPreviewRowCap = 50
	CSVMaxFileBytes  = 8 << 20
)

// CSVTable is the application-owned parsed CSV shape. Infrastructure codecs
// fill it; use cases never import csvcodec types.
type CSVTable struct {
	Delimiter rune
	HasBOM    bool
	Headers   []string
	Rows      [][]string
}

// CSVCodecPort is the application port for parsing and encoding CSV bytes.
type CSVCodecPort interface {
	Parse(data []byte, delimiter rune) (CSVTable, error)
	Encode(headers []string, rows [][]string) ([]byte, error)
	DetectDelimiter(sample string) rune
}

func (s *Service) SetCSVCodec(codec CSVCodecPort) {
	s.stateMu.Lock()
	s.csvCodec = codec
	s.stateMu.Unlock()
}

func (s *Service) csv() CSVCodecPort {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.csvCodec
}

func (s *Service) requireCSV() (CSVCodecPort, error) {
	codec := s.csv()
	if codec == nil {
		return nil, &domain.Error{Code: domain.ErrUnavailable, Message: "CSV codec is not configured"}
	}
	return codec, nil
}

func csvHeaderIndex(headers []string, name string) int {
	want := strings.TrimSpace(strings.ToLower(name))
	for i, header := range headers {
		if strings.TrimSpace(strings.ToLower(header)) == want {
			return i
		}
	}
	return -1
}

func delimiterName(value rune) string {
	switch value {
	case ';':
		return "semicolon"
	case '\t':
		return "tab"
	default:
		return "comma"
	}
}

func parseDelimiter(value string) rune {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "semicolon", ";":
		return ';'
	case "tab", "\t":
		return '\t'
	default:
		return ','
	}
}
