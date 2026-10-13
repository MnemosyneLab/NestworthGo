package liquidity

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type reviewWireRepository struct {
	application.Repository
	err error
}

func (r reviewWireRepository) ReadInvestmentReviewSnapshot(context.Context) (domain.InvestmentReviewSnapshot, error) {
	return domain.InvestmentReviewSnapshot{}, r.err
}

type reviewWireStore struct{}

func (reviewWireStore) LoadConfiguration(string) ([]byte, bool, error) { return nil, false, nil }
func (reviewWireStore) SaveConfiguration(string, []byte) error         { return nil }
func TestInvestmentReviewWireEmptyArrayAndErrors(t *testing.T) {
	app := application.NewService(reviewWireRepository{})
	app.SetInvestmentReviewConfiguration(reviewWireStore{})
	s := NewService(app)
	report, err := s.InvestmentReviews(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["schemaVersion"] != "investment-review/1" || wire["items"] == nil || report.Policy.WindowDays != 30 {
		t.Fatalf("unstable wire %s", raw)
	}
	p := report.Policy
	p.WindowDays = 1
	if _, err = s.SaveInvestmentReviewPolicy(t.Context(), SaveInvestmentReviewPolicyRequest{Policy: p}); err == nil {
		t.Fatal("invalid policy crossed IPC")
	}
	failed := application.NewService(reviewWireRepository{err: errors.New("private storage detail")})
	failed.SetInvestmentReviewConfiguration(reviewWireStore{})
	_, err = NewService(failed).InvestmentReviews(t.Context())
	var w *apierror.WireError
	if !errors.As(err, &w) || w.Code != "internal" || w.Message == "private storage detail" {
		t.Fatalf("unwrapped error %v", err)
	}
}
