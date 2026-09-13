package marketdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type vnextFixtureMeta struct {
	FixtureID      string `json:"fixtureId"`
	Provider       string `json:"provider"`
	Capability     string `json:"capability"`
	ProviderSymbol string `json:"providerSymbol"`
	QuoteCurrency  string `json:"quoteCurrency"`
	BaseCurrency   string `json:"baseCurrency"`
	Market         string `json:"market"`
	RequestedRange struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"requestedRange"`
	SessionPolicy           string   `json:"sessionPolicy"`
	SessionKind             string   `json:"sessionKind"`
	SessionTimezone         string   `json:"sessionTimezone"`
	CloseClock              string   `json:"closeClock"`
	PriceBasis              string   `json:"priceBasis"`
	PriceBasisVerified      bool     `json:"priceBasisVerified"`
	SourcePolicy            string   `json:"sourcePolicy"`
	Clock                   string   `json:"clock"`
	LastFinalizedMarketDate string   `json:"lastFinalizedMarketDate"`
	PendingMarketDates      []string `json:"pendingMarketDates"`
	Truncated               bool     `json:"truncated"`
}

func vnextRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "testdata", "market-data", "vnext")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", os.ErrNotExist
}

func loadVNextFixture(relative string) (vnextFixtureMeta, []byte, error) {
	root, err := vnextRoot()
	if err != nil {
		return vnextFixtureMeta{}, nil, err
	}
	body, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		return vnextFixtureMeta{}, nil, err
	}
	metaPath := strings.TrimSuffix(relative, filepath.Ext(relative)) + ".meta.json"
	metaBytes, err := os.ReadFile(filepath.Join(root, metaPath))
	if err != nil {
		return vnextFixtureMeta{}, nil, err
	}
	var meta vnextFixtureMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return vnextFixtureMeta{}, nil, err
	}
	return meta, body, nil
}

func parseClock(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func sessionEvidence(meta vnextFixtureMeta) domain.SessionEvidence {
	return domain.SessionEvidence{
		Kind:       domain.SessionKind(meta.SessionKind),
		Timezone:   meta.SessionTimezone,
		CloseClock: meta.CloseClock,
		Policy:     meta.SessionPolicy,
	}
}

func completenessFor(meta vnextFixtureMeta, malformed bool) (domain.HistoryCompleteness, error) {
	return domain.ClassifyHistoryCompleteness(
		meta.RequestedRange.Start,
		meta.RequestedRange.End,
		meta.LastFinalizedMarketDate,
		meta.PendingMarketDates,
		meta.Truncated,
		malformed,
	)
}

func toAppRanges(ranges []domain.InclusiveDateRange) []application.DateRange {
	if len(ranges) == 0 {
		return nil
	}
	out := make([]application.DateRange, 0, len(ranges))
	for _, item := range ranges {
		out = append(out, application.DateRange{Start: application.MarketDate(item.Start), End: application.MarketDate(item.End)})
	}
	return out
}

func mappingStatusFor(completeness domain.HistoryCompleteness, status application.MappingStatus) application.MappingStatus {
	if status != "" && status != application.MappingMapped {
		return status
	}
	switch completeness.Status {
	case "invalid":
		return application.MappingInvalid
	case "pending":
		return application.MappingPending
	case "uncertain":
		return application.MappingUncertain
	default:
		return application.MappingMapped
	}
}
