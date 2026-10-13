package mcpserver

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type failingReviewStore struct {
	memoryInvestmentReviewStore
	fail  bool
	saves int
}

func (s *failingReviewStore) SaveConfiguration(key string, value []byte) error {
	s.saves++
	if s.fail {
		return errors.New("synthetic configuration failure")
	}
	return s.memoryInvestmentReviewStore.SaveConfiguration(key, value)
}
func TestScaleReviewSaveFailureAndNoop(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	store := &failingReviewStore{}
	fx.app.SetInvestmentReviewConfiguration(store)
	ids := []string{}
	for range 2 {
		i, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Review fund", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, i.ID.String())
		if _, err := fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: i.ID.String(), Quantity: "10", UnitCost: "1"}); err != nil {
			t.Fatal(err)
		}
	}
	in := application.ReviewEnrollmentBatchInput{ExpectedRevision: 0, Items: []application.ReviewEnrollmentItem{{InstrumentID: ids[0], Enabled: true}, {InstrumentID: ids[1], Enabled: true}}}
	store.fail = true
	if _, err := fx.app.SetInvestmentReviewEnrollmentBatch(t.Context(), in); err == nil {
		t.Fatal("save failure accepted")
	}
	r, err := fx.app.InvestmentReviews(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.Revision != 0 || r.Items[0].Monitored || r.Items[1].Monitored {
		t.Fatal("partial configuration")
	}
	store.fail = false
	receipt, err := fx.app.SetInvestmentReviewEnrollmentBatch(t.Context(), in)
	if err != nil || receipt.Revision != 1 || receipt.ChangedCount != 2 {
		t.Fatal(receipt, err)
	}
	saves := store.saves
	in.ExpectedRevision = 1
	receipt, err = fx.app.SetInvestmentReviewEnrollmentBatch(t.Context(), in)
	if err != nil || receipt.ChangedCount != 0 || store.saves != saves {
		t.Fatal("noop saved state", receipt, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	in.Items[0].Enabled = false
	if _, err := fx.app.SetInvestmentReviewEnrollmentBatch(ctx, in); err == nil {
		t.Fatal("cancel accepted")
	}
	if store.saves != saves {
		t.Fatal("cancel saved")
	}
}
func TestScaleMetadataUnknownReceiptAndProtectedTargets(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	read := func() map[string]any {
		cap := scaleCapture(t, c, "list_instruments", map[string]any{"responseMode": "paged", "filter": map[string]any{"ids": []string{fx.instrument}}})
		return scaleSection(t, c, cap, "instruments")[0].(map[string]any)
	}
	before, err := fx.app.ListInstruments(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	row := read()
	args := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{map[string]any{"id": fx.instrument, "expectedMetadataVersion": row["metadataVersion"], "changes": map[string]any{"shortName": "Alias"}}}}}
	call(t, c, "update_instruments_batch", args, false)
	op, err := fx.service.GetOperation(args["operationId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	op.Status = "pending"
	path, _ := fx.service.operationPath(op.ID)
	if err := fx.service.writePrivate(path, op); err != nil {
		t.Fatal(err)
	}
	row = read()
	call(t, c, "update_instruments_batch", args, true)
	if !reflect.DeepEqual(row, read()) {
		t.Fatal("unknown receipt replayed")
	}
	after, err := fx.app.ListInstruments(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	after[0].ShortName = before[0].ShortName
	after[0].UpdatedAt = before[0].UpdatedAt
	if !reflect.DeepEqual(before, after) {
		t.Fatal("nonmetadata instrument state changed")
	}
	if err := fx.app.ArchiveInstrument(t.Context(), domain.InstrumentID(fx.instrument), true); err != nil {
		t.Fatal(err)
	}
	cap := scaleCapture(t, c, "list_instruments", map[string]any{"responseMode": "paged", "includeArchived": true, "filter": map[string]any{"ids": []string{fx.instrument}}})
	archived := scaleSection(t, c, cap, "instruments")[0].(map[string]any)
	call(t, c, "update_instruments_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{map[string]any{"id": fx.instrument, "expectedMetadataVersion": archived["metadataVersion"], "changes": map[string]any{"name": "Rejected"}}}}}, true)
}

func TestScalePermissionMatrix(t *testing.T) {
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			fx := newLedgerFixture(t)
			if _, err := fx.service.Enable(mode); err != nil {
				t.Fatal(err)
			}
			c := connect(t, fx.service)
			tools := featureToolList(t, c)
			for _, name := range []string{"get_read_page", "get_read_item", "get_market_data_batch"} {
				if tools[name] == nil || !tools[name].Annotations.ReadOnlyHint {
					t.Fatalf("read tool unavailable %s", name)
				}
			}
			writes := map[string]bool{"update_instruments_batch": mode != ReadOnly, "set_investment_review_enrollment_batch": mode == LedgerWrite, "set_investment_review_snooze_batch": mode == LedgerWrite, "acknowledge_investment_reviews_batch": mode == LedgerWrite}
			for name, allowed := range writes {
				if (tools[name] != nil) != allowed {
					t.Fatalf("permission %s %s", mode, name)
				}
				if !allowed {
					result, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{}}}})
					if err == nil && (result == nil || !result.IsError) {
						t.Fatal("unauthorized call accepted")
					}
				}
			}
		})
	}
}
