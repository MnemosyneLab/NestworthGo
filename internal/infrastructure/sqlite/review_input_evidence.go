package sqlite

import (
	"encoding/json"
	"fmt"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// FxRate's exact scalar is private and ordinary JSON encodes it as {}. The
// opt-in certified writer must preserve it explicitly before window reuse.
// This additive internal evidence field does not change source facts or DTOs.
type reviewInputEvidence struct {
	domain.ReviewDailyInput
	ExactSelectedFXRates map[domain.CurrencyCode]string `json:",omitempty"`
}

func encodeCompleteReviewInput(input domain.ReviewDailyInput) ([]byte, error) {
	if len(input.SelectedFX) == 0 {
		return json.Marshal(input)
	}
	rates := map[domain.CurrencyCode]string{}
	for currency, selected := range input.SelectedFX {
		rates[currency] = selected.Quote.Rate.String()
	}
	return json.Marshal(reviewInputEvidence{ReviewDailyInput: input, ExactSelectedFXRates: rates})
}

func decodeReviewInput(raw []byte, input *domain.ReviewDailyInput) error {
	var evidence reviewInputEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return err
	}
	if len(evidence.ExactSelectedFXRates) > 0 {
		if len(evidence.ExactSelectedFXRates) != len(evidence.SelectedFX) {
			return fmt.Errorf("incomplete selected FX evidence")
		}
		for currency, text := range evidence.ExactSelectedFXRates {
			selected, ok := evidence.SelectedFX[currency]
			if !ok {
				return fmt.Errorf("unexpected selected FX evidence")
			}
			rate, err := domain.ParseFxRate(text)
			if err != nil {
				return err
			}
			selected.Quote.Rate = rate
			evidence.SelectedFX[currency] = selected
		}
	}
	*input = evidence.ReviewDailyInput
	return nil
}
