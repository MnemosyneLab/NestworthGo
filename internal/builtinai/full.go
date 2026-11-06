package builtinai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

// This bounded stage intentionally excludes directory composites and batches.
// Both dispatch and discovery use the same closed command fields.
type fullCommand struct {
	Kind             history.ChangeCommandKind `json:"kind"`
	AccountID        string                    `json:"accountId,omitempty"`
	Amount           string                    `json:"amount,omitempty"`
	Currency         string                    `json:"currency,omitempty"`
	Reason           string                    `json:"reason,omitempty"`
	FromAccountID    string                    `json:"fromAccountId,omitempty"`
	ToAccountID      string                    `json:"toAccountId,omitempty"`
	Sent             string                    `json:"sent,omitempty"`
	SentCurrency     string                    `json:"sentCurrency,omitempty"`
	Received         string                    `json:"received,omitempty"`
	ReceivedCurrency string                    `json:"receivedCurrency,omitempty"`
	Fee              string                    `json:"fee,omitempty"`
	FeeCurrency      string                    `json:"feeCurrency,omitempty"`
	EffectiveAt      string                    `json:"effectiveAt"`
	Note             string                    `json:"note,omitempty"`
}

func (s reviewSession) PreviewFull(ctx context.Context, raw json.RawMessage) (assistant.ChangeReview, error) {
	var in fullCommand
	d := jsoncodec.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 4096 || d.Decode(&in) != nil || in.EffectiveAt == "" {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	if err := validateFullCommand(in); err != nil {
		return assistant.ChangeReview{}, err
	}
	canonical, err := jsoncodec.Marshal(in)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	var command history.ChangeCommandRequest
	if jsoncodec.Unmarshal(canonical, &command) != nil {
		return assistant.ChangeReview{}, assistant.ErrUnavailable
	}
	r, err := s.session.PreviewFullChange(ctx, command)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(r)
}
func (s reviewSession) ExecuteFull(ctx context.Context, id string) (assistant.ChangeReview, error) {
	r, err := s.session.ExecuteFullChange(ctx, id)
	if err != nil {
		return assistant.ChangeReview{}, err
	}
	return reviewView(r)
}

// Full does not infer business defaults from absent model arguments.
func validateFullCommand(in fullCommand) error {
	if _, err := time.Parse(time.RFC3339Nano, in.EffectiveAt); err != nil {
		return assistant.ErrUnavailable
	}
	var required []string
	switch in.Kind {
	case history.ChangeMoneyAdded, history.ChangeMoneyRemoved:
		required = []string{in.AccountID, in.Amount, in.Currency, in.Reason}
	case history.ChangeCashTransfer:
		required = []string{in.FromAccountID, in.ToAccountID, in.Sent, in.SentCurrency, in.Received, in.ReceivedCurrency}
	default:
		return assistant.ErrUnavailable
	}
	for _, value := range required {
		if strings.TrimSpace(value) == "" {
			return assistant.ErrUnavailable
		}
	}
	return nil
}
