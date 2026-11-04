package application

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// HistoricalReconciliation is an endpoint bridge, not a return calculation.
// Unknown totals stay nil; reasons survive even when the residual nets to zero.
type HistoricalReconciliation struct {
	Status        string                           `json:"status"`
	Currency      string                           `json:"currency"`
	Basis         string                           `json:"basis"`
	Total         *string                          `json:"total"`
	Supported     *string                          `json:"supported"`
	Residual      *string                          `json:"residual"`
	Contributions []HistoricalContribution         `json:"contributions"`
	Reasons       []string                         `json:"reasons"`
	Details       []HistoricalReconciliationDetail `json:"details"`
}
type HistoricalContribution struct {
	Key    string `json:"key"`
	Amount string `json:"amount"`
}

const historicalReconciliationMaxWork = 100000
const historicalReconciliationBasis = "endpoint_bridge/2; later_endpoint_fx; opening_exposure_fx; paired_principal_valuation; retained_corrected_facts"

func historicalReconciliation(ctx context.Context, b *domain.HistoricalSnapshotBatch, overview HistoricalOverviewResult) (*HistoricalReconciliation, error) {
	workCtx, cancel := context.WithTimeout(ctx, historicalSeriesTimeout)
	defer cancel()
	result, err := computeHistoricalReconciliation(workCtx, b, overview)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		out := &HistoricalReconciliation{Status: "unavailable", Currency: overview.Left.Currency, Basis: historicalReconciliationBasis, Contributions: []HistoricalContribution{}, Details: []HistoricalReconciliationDetail{}, Reasons: []string{"budget_exceeded"}}
		if overview.Change != nil {
			out.Total = overview.Change.NetWorth
		}
		return out, nil
	}
	return result, err
}

func computeHistoricalReconciliation(ctx context.Context, b *domain.HistoricalSnapshotBatch, overview HistoricalOverviewResult) (*HistoricalReconciliation, error) {
	if overview.Right == nil {
		return nil, ctx.Err()
	}
	out := &HistoricalReconciliation{Status: "unavailable", Currency: overview.Left.Currency, Basis: historicalReconciliationBasis, Contributions: []HistoricalContribution{}, Details: []HistoricalReconciliationDetail{}, Reasons: []string{}}
	reasons := map[string]bool{}
	reason := func(key string) { reasons[key] = true }
	finish := func() (*HistoricalReconciliation, error) {
		for key := range reasons {
			out.Reasons = append(out.Reasons, key)
		}
		sort.Strings(out.Reasons)
		if out.Supported != nil && len(out.Reasons) > 0 {
			out.Status = "partial"
		}
		return out, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if overview.Left.Currency != overview.Right.Currency {
		reason("currency_mismatch")
		return finish()
	}
	if overview.Change != nil {
		out.Total = overview.Change.NetWorth
	}
	if overview.Right.Current {
		reason("current_endpoint")
		return finish()
	}
	if out.Total == nil {
		reason("incomplete_endpoints")
	}
	start, err := time.Parse(time.RFC3339Nano, overview.Left.CutoffAt)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse(time.RFC3339Nano, overview.Right.CutoffAt)
	if err != nil {
		return nil, err
	}
	reverse := start.After(end)
	if reverse {
		start, end = end, start
	}
	if start.Equal(end) {
		if out.Total == nil {
			return finish()
		}
		out.Status, out.Supported, out.Residual = "reconciled", historicalString("0"), historicalString("0")
		return finish()
	}
	work := len(b.Portfolio.Accounts) + len(b.Portfolio.Holdings) + len(b.Portfolio.Instruments) + len(b.OriginData.Components) + len(overview.Rows) + len(b.AccountStateObservations) + len(b.HoldingStateObservations) + len(b.InstrumentStateObservations) + len(b.InstrumentPreferenceFacts) + len(b.FXPreferenceFacts) + len(b.InstrumentProviderBindingFacts) + len(b.Activities)
	for _, a := range b.Activities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		work += len(a.Effects)
		if work > historicalReconciliationMaxWork {
			reason("budget_exceeded")
			return finish()
		}
	}
	if work > historicalReconciliationMaxWork {
		reason("budget_exceeded")
		return finish()
	}
	inPeriod := func(at time.Time) bool { return at.After(start) && !at.After(end) }
	instrumentCreated := map[string]time.Time{}
	for _, i := range b.Portfolio.Instruments {
		instrumentCreated[i.ID.String()] = i.CreatedAt
	}
	changedAccounts, changedHoldings, changedInstruments := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, o := range b.AccountStateObservations {
		if inPeriod(o.EffectiveAt) {
			changedAccounts[o.AccountID.String()] = true
		}
	}
	for _, o := range b.HoldingStateObservations {
		if inPeriod(o.EffectiveAt) {
			changedHoldings[o.HoldingID.String()] = true
		}
	}
	for _, o := range b.InstrumentStateObservations {
		if inPeriod(o.EffectiveAt) && !(o.ArchivedAt == nil && o.EffectiveAt.Equal(instrumentCreated[o.InstrumentID.String()])) {
			changedInstruments[o.InstrumentID.String()] = true
		}
	}
	initialPreferences := map[string]bool{}
	for _, o := range b.InstrumentPreferenceFacts {
		if !inPeriod(o.EffectiveAt) {
			continue
		}
		id := o.InstrumentID.String()
		if o.EffectiveAt.Equal(instrumentCreated[id]) && !initialPreferences[id] {
			initialPreferences[id] = true
			continue
		}
		changedInstruments[id] = true
	}
	initialBindings := map[string]bool{}
	for _, o := range b.InstrumentProviderBindingFacts {
		if !inPeriod(o.EffectiveFrom) {
			continue
		}
		id := o.InstrumentID.String()
		if o.EffectiveFrom.Equal(instrumentCreated[id]) && !initialBindings[id] {
			initialBindings[id] = true
			continue
		}
		changedInstruments[id] = true
	}
	fxChanged := false
	for _, o := range b.FXPreferenceFacts {
		if inPeriod(o.EffectiveAt) {
			fxChanged = true
		}
	}
	bridge := newHistoricalBridge(ctx, b, overview, out, start, end, reverse, reason)
	bridge.changedAccounts, bridge.changedHoldings, bridge.changedInstruments, bridge.fxChanged = changedAccounts, changedHoldings, changedInstruments, fxChanged
	if err := bridge.compute(); err != nil {
		return nil, err
	}
	return finish()
}

func reconciliationConvert(value decimal.Decimal, cell *HistoricalOverviewCell, base string) (decimal.Decimal, bool) {
	if value.IsZero() || cell.Currency == base {
		return value, true
	}
	if cell.FX == nil || cell.FX.Value == nil {
		return decimal.Zero, false
	}
	rate, err := decimal.NewFromString(*cell.FX.Value)
	if err != nil || !rate.IsPositive() {
		return decimal.Zero, false
	}
	// Match valuation's decimal multiplication/division; any finite-division
	// remainder remains visible instead of being assigned to investment profit.
	if cell.FX.Operation == "multiply" {
		return value.Mul(rate), true
	}
	if cell.FX.Operation == "divide" {
		return value.Div(rate), true
	}
	return decimal.Zero, false
}
func reconciliationPriceCompatible(a, b *HistoricalOverviewEvidence) bool {
	return a != nil && b != nil && a.Value != nil && b.Value != nil && a.Currency == b.Currency && a.Source == b.Source && a.SourceKey == b.SourceKey && a.PriceBasis == b.PriceBasis && a.ObservationKind == b.ObservationKind && a.TimestampBasis == b.TimestampBasis && a.SourcePolicyVersion == b.SourcePolicyVersion
}

func reconciliationFXCompatible(a, b *HistoricalOverviewEvidence) bool {
	return a != nil && b != nil && a.Source == b.Source && a.SourceKey == b.SourceKey && a.BaseCurrency == b.BaseCurrency && a.QuoteCurrency == b.QuoteCurrency && a.Operation == b.Operation && a.ObservationKind == b.ObservationKind && a.TimestampBasis == b.TimestampBasis && a.SourcePolicyVersion == b.SourcePolicyVersion
}
