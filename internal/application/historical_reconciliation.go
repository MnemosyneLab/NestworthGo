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
	Status        string                   `json:"status"`
	Currency      string                   `json:"currency"`
	Basis         string                   `json:"basis"`
	Total         *string                  `json:"total"`
	Supported     *string                  `json:"supported"`
	Residual      *string                  `json:"residual"`
	Contributions []HistoricalContribution `json:"contributions"`
	Reasons       []string                 `json:"reasons"`
}
type HistoricalContribution struct {
	Key    string `json:"key"`
	Amount string `json:"amount"`
}

const historicalReconciliationMaxWork = 100000
const historicalReconciliationBasis = "endpoint_bridge/1; later_endpoint_fx; opening_exposure_fx; unchanged_quantity_price; retained_corrected_facts"

func historicalReconciliation(ctx context.Context, b *domain.HistoricalSnapshotBatch, overview HistoricalOverviewResult) (*HistoricalReconciliation, error) {
	workCtx, cancel := context.WithTimeout(ctx, historicalSeriesTimeout)
	defer cancel()
	result, err := computeHistoricalReconciliation(workCtx, b, overview)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		out := &HistoricalReconciliation{Status: "unavailable", Currency: overview.Left.Currency, Basis: historicalReconciliationBasis, Contributions: []HistoricalContribution{}, Reasons: []string{"budget_exceeded"}}
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
	out := &HistoricalReconciliation{Status: "unavailable", Currency: overview.Left.Currency, Basis: historicalReconciliationBasis, Contributions: []HistoricalContribution{}, Reasons: []string{}}
	reasons := map[string]bool{}
	reason := func(key string) { reasons[key] = true }
	finish := func() (*HistoricalReconciliation, error) {
		for key := range reasons {
			out.Reasons = append(out.Reasons, key)
		}
		sort.Strings(out.Reasons)
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
		return finish()
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
	sums := map[string]decimal.Decimal{}
	if start.Equal(end) {
		out.Status, out.Supported, out.Residual = "reconciled", historicalString("0"), historicalString("0")
		return finish()
	}
	work := len(b.Portfolio.Accounts) + len(overview.Rows) + len(b.AccountStateObservations) + len(b.HoldingStateObservations) + len(b.InstrumentStateObservations) + len(b.InstrumentPreferenceFacts) + len(b.FXPreferenceFacts) + len(b.InstrumentProviderBindingFacts) + len(b.Activities)
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
		if inPeriod(o.EffectiveAt) {
			changedInstruments[o.InstrumentID.String()] = true
		}
	}
	for _, o := range b.InstrumentPreferenceFacts {
		if inPeriod(o.EffectiveAt) {
			changedInstruments[o.InstrumentID.String()] = true
		}
	}
	for _, o := range b.InstrumentProviderBindingFacts {
		if inPeriod(o.EffectiveFrom) {
			changedInstruments[o.InstrumentID.String()] = true
		}
	}
	fxChanged := false
	for _, o := range b.FXPreferenceFacts {
		if inPeriod(o.EffectiveAt) {
			fxChanged = true
		}
	}
	accounts := map[domain.AccountID]domain.Account{}
	for _, record := range b.Portfolio.Accounts {
		accounts[record.Account.ID] = record.Account
	}
	rows, holdings := map[string]HistoricalOverviewRow{}, map[string]string{}
	stable := map[string]bool{}
	for _, row := range overview.Rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if row.Kind == "account" {
			continue
		}
		rows[row.Key] = row
		if row.HoldingID != "" {
			holdings[row.HoldingID] = row.Key
		}
		a, z := row.Left, row.Right
		if a != nil && z != nil && !a.Included && !z.Included && !changedAccounts[row.AccountID] {
			continue
		}
		if a == nil || z == nil || a.Included != z.Included || changedAccounts[row.AccountID] || changedHoldings[row.HoldingID] {
			reason("scope_changed")
			continue
		}
		if !a.Included || !z.Included {
			reason("scope_changed")
			continue
		}
		if !a.Complete || !z.Complete || a.NativeAmount == nil || z.NativeAmount == nil || a.BaseAmount == nil || z.BaseAmount == nil {
			reason("missing_evidence")
			continue
		}
		if a.Currency != z.Currency || changedInstruments[row.InstrumentID] || (fxChanged && a.Currency != out.Currency) {
			reason("basis_changed")
			continue
		}
		stable[row.Key] = true
	}
	effectKey := func(e domain.ActivityEffect) string {
		if e.HoldingID != nil {
			return holdings[e.HoldingID.String()]
		}
		if e.AccountID == nil || e.Money == nil {
			return ""
		}
		id := e.AccountID.String()
		if a, ok := accounts[*e.AccountID]; ok && a.TrackingMode == domain.TrackingHoldings {
			return id + ":cash:" + e.Money.Currency().String()
		}
		return id + ":balance"
	}
	touched, exposureChanged := map[string]bool{}, map[string]bool{}
	// Recover semantic kind/reason through reversal chains; replay already owns
	// corrected magnitudes and directions. Never mutate the batch or invert twice.
	semantics := map[domain.ActivityID]domain.Activity{}
	for _, raw := range b.Activities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		activity := raw
		if raw.ReversesActivityID != nil {
			if original, ok := semantics[*raw.ReversesActivityID]; ok {
				activity.Kind, activity.Reason = original.Kind, original.Reason
			} else if inPeriod(raw.EffectiveAt) {
				reason("unsupported_activity")
			}
		}
		semantics[raw.ID] = activity
		if !inPeriod(raw.EffectiveAt) {
			continue
		}
		endpoints, inside := 0, 0
		hasQuantity := false
		for _, e := range activity.Effects {
			if e.Target == domain.EffectTargetHoldingCost {
				continue
			}
			endpoints++
			key := effectKey(e)
			if stable[key] {
				inside++
			}
			if e.Quantity != nil {
				hasQuantity = true
				touched[key] = true
				if stable[key] {
					reason("quantity_activity")
				}
			}
		}
		neutral := decimal.Zero
		for _, e := range activity.Effects {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if e.Target == domain.EffectTargetHoldingCost {
				continue
			}
			key := effectKey(e)
			row, exists := rows[key]
			if !exists {
				reason("unsupported_activity")
				continue
			}
			if !stable[key] {
				continue
			}
			if e.Money == nil {
				continue
			}
			later := row.Right
			if reverse {
				later = row.Left
			}
			if later.Currency != e.Money.Currency().String() {
				reason("basis_changed")
				continue
			}
			component := domain.ComponentID{AccountID: *e.AccountID, Cash: true}
			bucket := assetBucketFor(activity, e, component, inside, endpoints, accounts)
			// Quantity principal cannot be measured by this endpoint-only bridge.
			if hasQuantity || e.Classification == domain.ClassificationInternalTransfer || e.Classification == domain.ClassificationDebtPrincipal {
				exposureChanged[key] = true
			}
			if bucket == nil {
				internal := e.Classification == domain.ClassificationInternalTransfer || e.Classification == domain.ClassificationTradePrincipal || e.Classification == domain.ClassificationDebtPrincipal
				if !internal {
					reason("unsupported_activity")
				}
				if internal && !hasQuantity && inside == endpoints {
					if value, ok := reconciliationConvert(e.Money.Amount(), later, out.Currency); ok {
						value = signedDirection(e.Direction, value)
						if row.Role == "liability" {
							value = value.Neg()
						}
						neutral = neutral.Add(value)
					} else {
						reason("missing_evidence")
					}
				}
				if inside != endpoints {
					reason("unsupported_activity")
				}
				continue
			}
			// Never claim an external flow merely because the other side is unstable.
			if (e.Classification == domain.ClassificationInternalTransfer || e.Classification == domain.ClassificationTradePrincipal || e.Classification == domain.ClassificationDebtPrincipal) && inside != endpoints {
				reason("scope_changed")
				continue
			}
			value, ok := reconciliationConvert(e.Money.Amount(), later, out.Currency)
			if !ok {
				reason("missing_evidence")
				continue
			}
			value = signedDirection(e.Direction, value)
			if row.Role == "liability" {
				value = value.Neg()
			}
			sums[string(*bucket)] = sums[string(*bucket)].Add(value)
		}
		if !neutral.IsZero() {
			reason("internal_imbalance")
		}
	}
	for key, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !stable[key] {
			continue
		}
		a, z := row.Left, row.Right
		if reverse {
			a, z = z, a
		}
		sign := decimal.NewFromInt(1)
		if row.Role == "liability" {
			sign = sign.Neg()
		}
		nativeA, err := decimal.NewFromString(*a.NativeAmount)
		if err != nil {
			return nil, err
		}
		if row.Kind == "holding" {
			if touched[key] || a.Quantity == nil || z.Quantity == nil || *a.Quantity != *z.Quantity {
				reason("quantity_activity")
				continue
			}
			if nativeA.IsZero() && *z.NativeAmount == "0" {
				continue
			}
			if a.Price == nil || z.Price == nil || a.Price.Value == nil || z.Price.Value == nil {
				reason("missing_evidence")
				continue
			}
			if a.Price.Currency != a.Currency || z.Price.Currency != z.Currency || !reconciliationPriceCompatible(a.Price, z.Price) {
				reason("basis_changed")
				continue
			}
			nativeZ, err := decimal.NewFromString(*z.NativeAmount)
			if err != nil {
				return nil, err
			}
			price, ok := reconciliationConvert(nativeZ.Sub(nativeA), z, out.Currency)
			if !ok {
				reason("missing_evidence")
				continue
			}
			sums["price_change"] = sums["price_change"].Add(price.Mul(sign))
		}
		if a.Currency == out.Currency || (nativeA.IsZero() && *z.NativeAmount == "0" && !exposureChanged[key]) {
			continue
		}
		if a.FX == nil || z.FX == nil || a.FX.Value == nil || z.FX.Value == nil {
			reason("missing_evidence")
			continue
		}
		if !reconciliationFXCompatible(a.FX, z.FX) {
			reason("basis_changed")
			continue
		}
		if exposureChanged[key] {
			reason("foreign_internal_exposure")
			continue
		}
		atA, okA := reconciliationConvert(nativeA, a, out.Currency)
		atZ, okZ := reconciliationConvert(nativeA, z, out.Currency)
		if !okA || !okZ {
			reason("missing_evidence")
			continue
		}
		sums["fx_impact"] = sums["fx_impact"].Add(atZ.Sub(atA).Mul(sign))
	}
	supported := decimal.Zero
	keys := make([]string, 0, len(sums))
	for key := range sums {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := sums[key]
		if reverse {
			value = value.Neg()
		}
		supported = supported.Add(value)
		out.Contributions = append(out.Contributions, HistoricalContribution{key, value.String()})
	}
	total, err := decimal.NewFromString(*out.Total)
	if err != nil {
		return nil, err
	}
	residual := total.Sub(supported)
	out.Supported, out.Residual = historicalString(supported.String()), historicalString(residual.String())
	out.Status = "reconciled"
	if !residual.IsZero() {
		reason("unexplained_movement")
	}
	if len(reasons) > 0 {
		out.Status = "partial"
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
	return a != nil && b != nil && a.Value != nil && b.Value != nil && a.Currency == b.Currency && a.Source == b.Source && a.PriceBasis == b.PriceBasis && a.ObservationKind == b.ObservationKind && a.SourcePolicyVersion == b.SourcePolicyVersion
}

func reconciliationFXCompatible(a, b *HistoricalOverviewEvidence) bool {
	return a != nil && b != nil && a.Source == b.Source && a.ObservationKind == b.ObservationKind && a.TimestampBasis == b.TimestampBasis && a.SourcePolicyVersion == b.SourcePolicyVersion
}
