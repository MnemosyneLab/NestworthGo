package application

import (
	"context"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// A leaf's total = contributions + paired principal + residual. Paired
// principal cancels across the household, and is never income or return.
type HistoricalReconciliationDetail struct {
	Key              string                           `json:"key"`
	AccountID        string                           `json:"accountId"`
	HoldingID        string                           `json:"holdingId"`
	InstrumentID     string                           `json:"instrumentId"`
	Kind             string                           `json:"kind"`
	Total            *string                          `json:"total"`
	Supported        string                           `json:"supported"`
	Principal        string                           `json:"principal"`
	Residual         *string                          `json:"residual"`
	OpeningQuantity  *string                          `json:"openingQuantity"`
	ActivityQuantity *string                          `json:"activityQuantity"`
	ClosingQuantity  *string                          `json:"closingQuantity"`
	QuantityResidual *string                          `json:"quantityResidual"`
	Contributions    []HistoricalContribution         `json:"contributions"`
	Reasons          []string                         `json:"reasons"`
	Records          []HistoricalReconciliationRecord `json:"records"`
}

type HistoricalReconciliationRecord struct {
	ID          string                           `json:"id"`
	Kind        string                           `json:"kind"`
	Reason      string                           `json:"reason"`
	EffectiveAt string                           `json:"effectiveAt"`
	Note        *string                          `json:"note"`
	Effects     []HistoricalReconciliationEffect `json:"effects"`
}

type HistoricalReconciliationEffect struct {
	Classification string  `json:"classification"`
	Amount         *string `json:"amount"`
	Currency       string  `json:"currency"`
	Quantity       *string `json:"quantity"`
}

type historicalBridgeLeaf struct {
	row                                              HistoricalOverviewRow
	a, z                                             *HistoricalOverviewCell
	detail                                           HistoricalReconciliationDetail
	stable                                           bool
	quantity, principal, nativePrincipal, adjustment decimal.Decimal
	sums                                             map[string]decimal.Decimal
	reasons                                          map[string]bool
	quantityUnsupported, foreignExposure             bool
}

type historicalBridge struct {
	ctx                                                  context.Context
	batch                                                *domain.HistoricalSnapshotBatch
	overview                                             HistoricalOverviewResult
	out                                                  *HistoricalReconciliation
	start, end                                           time.Time
	reverse                                              bool
	reason                                               func(string)
	accounts                                             map[domain.AccountID]domain.Account
	leaves                                               map[string]*historicalBridgeLeaf
	holdings                                             map[string]string
	changedAccounts, changedHoldings, changedInstruments map[string]bool
	fxChanged                                            bool
}

func newHistoricalBridge(ctx context.Context, b *domain.HistoricalSnapshotBatch, o HistoricalOverviewResult, out *HistoricalReconciliation, start, end time.Time, reverse bool, reason func(string)) *historicalBridge {
	return &historicalBridge{ctx: ctx, batch: b, overview: o, out: out, start: start, end: end, reverse: reverse, reason: reason, accounts: map[domain.AccountID]domain.Account{}, leaves: map[string]*historicalBridgeLeaf{}, holdings: map[string]string{}}
}

func (b *historicalBridge) flag(s *historicalBridgeLeaf, reason string) {
	b.reason(reason)
	if s != nil {
		s.reasons[reason] = true
	}
}
func (b *historicalBridge) inPeriod(at time.Time) bool { return at.After(b.start) && !at.After(b.end) }
func (s *historicalBridgeLeaf) sign(v decimal.Decimal) decimal.Decimal {
	if s.row.Role == "liability" {
		return v.Neg()
	}
	return v
}
func (s *historicalBridgeLeaf) add(key string, v decimal.Decimal) { s.sums[key] = s.sums[key].Add(v) }
func (b *historicalBridge) effectLeaf(e domain.ActivityEffect) *historicalBridgeLeaf {
	if e.HoldingID != nil {
		return b.leaves[b.holdings[e.HoldingID.String()]]
	}
	if e.AccountID == nil || e.Money == nil {
		return nil
	}
	key := e.AccountID.String() + ":balance"
	if b.accounts[*e.AccountID].TrackingMode == domain.TrackingHoldings {
		key = e.AccountID.String() + ":cash:" + e.Money.Currency().String()
	}
	return b.leaves[key]
}

func (b *historicalBridge) internalPrincipal(a domain.Activity, e domain.ActivityEffect) bool {
	internal := e.Classification == domain.ClassificationInternalTransfer || e.Classification == domain.ClassificationDebtPrincipal || e.Classification == domain.ClassificationTradePrincipal
	if !internal || e.Money == nil || e.AccountID == nil {
		return internal
	}
	// Preserve semantic precedence from the shared classifier (for example
	// product interest), rather than netting income into neutral principal.
	bucket := assetBucketFor(a, e, domain.ComponentID{AccountID: *e.AccountID, Cash: true}, 2, 2, b.accounts)
	return bucket == nil || *bucket == domain.BucketExternalFlow
}

func (b *historicalBridge) compute() error {
	for _, r := range b.batch.Portfolio.Accounts {
		b.accounts[r.Account.ID] = r.Account
	}
	parents := map[string]HistoricalOverviewRow{}
	for _, r := range b.overview.Rows {
		if r.Kind == "account" {
			parents[r.Key] = r
		}
	}
	newHoldings := map[string]bool{}
	originHoldings := map[domain.HoldingID]bool{}
	for _, c := range b.batch.OriginData.Components {
		if c.HoldingID != nil {
			originHoldings[*c.HoldingID] = true
		}
	}
	for _, h := range b.batch.Portfolio.Holdings {
		if b.inPeriod(h.CreatedAt) && !originHoldings[h.ID] {
			newHoldings[h.ID.String()] = true
		}
	}
	for _, row := range b.overview.Rows {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if row.Kind == "account" {
			continue
		}
		a, z := row.Left, row.Right
		if b.reverse {
			a, z = z, a
		}
		s := &historicalBridgeLeaf{row: row, a: a, z: z, sums: map[string]decimal.Decimal{}, reasons: map[string]bool{}}
		s.detail = HistoricalReconciliationDetail{Key: row.Key, AccountID: row.AccountID, HoldingID: row.HoldingID, InstrumentID: row.InstrumentID, Kind: row.Kind, Contributions: []HistoricalContribution{}, Reasons: []string{}, Records: []HistoricalReconciliationRecord{}}
		b.leaves[row.Key] = s
		if row.HoldingID != "" {
			b.holdings[row.HoldingID] = row.Key
		}
		// Absence is zero only for a proven newly-created position in an
		// existing included account, never for an unknown endpoint value.
		parent := parents[row.AccountID]
		newPosition := a == nil && z != nil && newHoldings[row.HoldingID] && parent.Left != nil && parent.Right != nil && parent.Left.Included && parent.Right.Included && !b.changedAccounts[row.AccountID]
		if newPosition {
			a = &HistoricalOverviewCell{Included: true, Complete: true, Status: "zero", Currency: z.Currency, NativeAmount: historicalString("0"), BaseAmount: historicalString("0"), Quantity: historicalString("0")}
			s.a = a
		}
		va, oka := reconciliationLeafValue(a)
		vz, okz := reconciliationLeafValue(z)
		if oka && okz {
			s.detail.Total = historicalString(s.sign(vz.Sub(va)).String())
		}
		if a != nil && z != nil && !a.Included && !z.Included && !b.changedAccounts[row.AccountID] {
			continue
		}
		if a == nil || z == nil || a.Included != z.Included || b.changedAccounts[row.AccountID] || b.changedHoldings[row.HoldingID] {
			b.flag(s, "scope_changed")
			continue
		}
		if !a.Included || !z.Included {
			b.flag(s, "scope_changed")
			continue
		}
		if !a.Complete || !z.Complete || a.NativeAmount == nil || z.NativeAmount == nil || a.BaseAmount == nil || z.BaseAmount == nil {
			b.flag(s, "missing_evidence")
			continue
		}
		if a.Currency != z.Currency || b.changedInstruments[row.InstrumentID] || (b.fxChanged && a.Currency != b.out.Currency) {
			b.flag(s, "basis_changed")
			continue
		}
		s.stable = true
	}
	// Recover semantic metadata only. The capture already owns corrected
	// magnitudes and reversal directions; do not invert or filter them again.
	semantics := map[domain.ActivityID]domain.Activity{}
	for _, raw := range b.batch.Activities {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		a := raw
		if raw.ReversesActivityID != nil {
			if original, ok := semantics[*raw.ReversesActivityID]; ok {
				a.Kind, a.Reason, a.TradeDetail = original.Kind, original.Reason, original.TradeDetail
			} else if b.inPeriod(raw.EffectiveAt) {
				b.reason("unsupported_activity")
			}
		}
		semantics[raw.ID] = a
		if !b.inPeriod(raw.EffectiveAt) {
			continue
		}
		if err := b.activity(a, raw); err != nil {
			return err
		}
	}
	for _, row := range b.overview.Rows {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		s := b.leaves[row.Key]
		if s == nil {
			continue
		}
		if s.stable {
			b.market(s)
		}
	}
	b.finish()
	return b.ctx.Err()
}

func reconciliationLeafValue(c *HistoricalOverviewCell) (decimal.Decimal, bool) {
	if c == nil || !c.Included {
		return decimal.Zero, true
	}
	if !c.Complete || c.BaseAmount == nil {
		return decimal.Zero, false
	}
	v, err := decimal.NewFromString(*c.BaseAmount)
	return v, err == nil
}

func (b *historicalBridge) activity(a, raw domain.Activity) error {
	inside, endpoints := 0, 0
	quantityEffects, internalEffects := []domain.ActivityEffect{}, []domain.ActivityEffect{}
	records := map[*historicalBridgeLeaf][]HistoricalReconciliationEffect{}
	for _, e := range a.Effects {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if e.Target == domain.EffectTargetHoldingCost {
			continue
		}
		endpoints++
		s := b.effectLeaf(e)
		if s == nil {
			b.reason("unsupported_activity")
			continue
		}
		if s.stable {
			inside++
		}
		ref := HistoricalReconciliationEffect{Classification: string(e.Classification)}
		if e.Money != nil {
			ref.Amount = historicalString(signedDirection(e.Direction, e.Money.Amount()).String())
			ref.Currency = e.Money.Currency().String()
		}
		if e.Quantity != nil {
			ref.Quantity = historicalString(signedDirection(e.Direction, e.Quantity.Decimal()).String())
			s.quantity = s.quantity.Add(signedDirection(e.Direction, e.Quantity.Decimal()))
			quantityEffects = append(quantityEffects, e)
		}
		records[s] = append(records[s], ref)
		if b.internalPrincipal(a, e) {
			internalEffects = append(internalEffects, e)
		}
	}
	for s, effects := range records {
		s.detail.Records = append(s.detail.Records, HistoricalReconciliationRecord{ID: raw.ID.String(), Kind: string(raw.Kind), Reason: string(a.Reason), EffectiveAt: a.EffectiveAt.UTC().Format(time.RFC3339Nano), Note: raw.Note, Effects: effects})
	}
	paired := map[int]bool{}
	if len(quantityEffects) > 0 {
		if (a.Kind == domain.ActivityBuy || a.Kind == domain.ActivitySell) && b.pairTrade(a, quantityEffects, internalEffects) {
			for i, e := range a.Effects {
				if e.Classification == domain.ClassificationTradePrincipal && b.internalPrincipal(a, e) {
					paired[i] = true
				}
			}
		} else if a.Kind == domain.ActivityPositionTransfer && b.pairPositionTransfer(quantityEffects, internalEffects) {
			for i, e := range a.Effects {
				if e.Classification == domain.ClassificationInternalTransfer && b.internalPrincipal(a, e) {
					paired[i] = true
				}
			}
		} else {
			for _, e := range internalEffects {
				b.flag(b.effectLeaf(e), "principal_pairing")
			}
			for _, e := range quantityEffects {
				s := b.effectLeaf(e)
				if s == nil {
					continue
				}
				if e.Classification == domain.ClassificationRemeasurement && a.Kind == domain.ActivityPositionTransfer && a.Reason == domain.ReasonReconciliation && len(quantityEffects) == 1 && len(internalEffects) == 0 && b.remeasurement(s, e) {
					continue
				}
				s.quantityUnsupported = true
				b.flag(s, "quantity_activity")
			}
		}
	} else {
		// Monetary internal legs are assigned only as a complete zero-sum
		// set. Cross-currency imbalances and exposure paths stay unexplained.
		if b.pairMoney(internalEffects) {
			for i, e := range a.Effects {
				if (e.Classification == domain.ClassificationInternalTransfer || e.Classification == domain.ClassificationDebtPrincipal) && b.internalPrincipal(a, e) {
					paired[i] = true
				}
			}
		}
	}
	for i, e := range a.Effects {
		if e.Target == domain.EffectTargetHoldingCost || paired[i] || e.Money == nil {
			continue
		}
		s := b.effectLeaf(e)
		if s == nil || !s.stable {
			continue
		}
		if e.AccountID == nil || s.z.Currency != e.Money.Currency().String() {
			b.flag(s, "basis_changed")
			continue
		}
		bucket := assetBucketFor(a, e, domain.ComponentID{AccountID: *e.AccountID, Cash: true}, inside, endpoints, b.accounts)
		if b.internalPrincipal(a, e) {
			if inside != endpoints {
				b.flag(s, "scope_changed")
			}
			continue
		}
		if bucket == nil {
			b.flag(s, "unsupported_activity")
			continue
		}
		v, ok := reconciliationConvert(e.Money.Amount(), s.z, b.out.Currency)
		if !ok {
			b.flag(s, "missing_evidence")
			continue
		}
		s.add(string(*bucket), s.sign(signedDirection(e.Direction, v)))
	}
	return nil
}

func (b *historicalBridge) pairTrade(a domain.Activity, quantities, internal []domain.ActivityEffect) bool {
	// No proportional allocation: one trade detail must match exactly one
	// quantity leg and one opposite cash principal leg. Fees are excluded.
	if a.TradeDetail == nil || len(quantities) != 1 || len(internal) != 2 {
		return false
	}
	q := quantities[0]
	d := a.TradeDetail
	if q.HoldingID == nil || *q.HoldingID != d.HoldingID || q.InstrumentID == nil || *q.InstrumentID != d.InstrumentID || q.Classification != domain.ClassificationTradePrincipal || !q.Quantity.Decimal().Equal(d.Quantity.Decimal()) {
		return false
	}
	s := b.effectLeaf(q)
	if s == nil || !s.stable || s.row.Role == "liability" {
		return false
	}
	var cash *domain.ActivityEffect
	for i := range internal {
		if internal[i].Money != nil {
			cash = &internal[i]
		}
	}
	if cash == nil || cash.Quantity != nil || cash.Direction == q.Direction || cash.Classification != domain.ClassificationTradePrincipal || cash.Money.Currency() != d.Gross.Currency() || !cash.Money.Amount().Equal(d.Gross.Amount()) || cash.Money.Currency().String() != s.z.Currency {
		b.flag(s, "principal_pairing")
		return false
	}
	c := b.effectLeaf(*cash)
	if c == nil || !c.stable || c.row.Role == "liability" || c.z.Currency != s.z.Currency {
		b.flag(s, "principal_pairing")
		return false
	}
	// A cleared position has known zero value and may need no quote/FX in
	// valuation. Its uniquely paired settlement cash supplies the later FX
	// convention; this does not fill a missing rate on positive exposure.
	if s.z.Currency != b.out.Currency && s.z.FX == nil && s.z.NativeAmount != nil && *s.z.NativeAmount == "0" && s.z.Quantity != nil && *s.z.Quantity == "0" {
		cell := *s.z
		cell.FX = c.z.FX
		s.z = &cell
	}
	value, ok := reconciliationConvert(cash.Money.Amount(), c.z, b.out.Currency)
	if !ok {
		b.flag(s, "missing_evidence")
		return false
	}
	// Use both components' adopted rates; different quote authorities cannot
	// be silently paired even if a rounded numeric amount happens to match.
	if s.z.Currency != b.out.Currency && !reconciliationFXCompatible(c.z.FX, s.z.FX) {
		b.flag(s, "basis_changed")
		return false
	}
	other, ok := reconciliationConvert(cash.Money.Amount(), s.z, b.out.Currency)
	if !ok || !other.Equal(value) {
		b.flag(s, "basis_changed")
		return false
	}
	cashValue := signedDirection(cash.Direction, value)
	c.principal = c.principal.Add(cashValue)
	s.principal = s.principal.Sub(cashValue)
	s.nativePrincipal = s.nativePrincipal.Add(signedDirection(q.Direction, d.Gross.Amount()))
	return true
}

func (b *historicalBridge) pairPositionTransfer(q, internal []domain.ActivityEffect) bool {
	if len(q) != 2 || len(internal) != 2 || q[0].HoldingID == nil || q[1].HoldingID == nil || *q[0].HoldingID == *q[1].HoldingID || q[0].InstrumentID == nil || q[1].InstrumentID == nil || *q[0].InstrumentID != *q[1].InstrumentID || q[0].Direction == q[1].Direction || !q[0].Quantity.Decimal().Equal(q[1].Quantity.Decimal()) || q[0].Classification != domain.ClassificationInternalTransfer || q[1].Classification != domain.ClassificationInternalTransfer {
		return false
	}
	a, z := b.effectLeaf(q[0]), b.effectLeaf(q[1])
	if a == nil || z == nil || !a.stable || !z.stable || a.row.Role != z.row.Role || a.z.Currency != z.z.Currency {
		return false
	}
	// A full transfer can leave the source with zero exposure and therefore
	// no adopted quote/FX. The other leg has the same recorded instrument;
	// use its later evidence only for a proven zero, and still compare the
	// source's positive opening quote to that authority.
	for _, pair := range [][2]*historicalBridgeLeaf{{a, z}, {z, a}} {
		s, other := pair[0], pair[1]
		if s.z.Quantity != nil && *s.z.Quantity == "0" && s.z.NativeAmount != nil && *s.z.NativeAmount == "0" {
			cell := *s.z
			if cell.Price == nil {
				cell.Price = other.z.Price
			}
			if cell.FX == nil {
				cell.FX = other.z.FX
			}
			s.z = &cell
			opening, _ := reconciliationDecimal(s.a.Quantity)
			if !opening.IsZero() && !reconciliationPriceCompatible(s.a.Price, s.z.Price) {
				b.flag(s, "basis_changed")
				return false
			}
		}
	}
	if !reconciliationPriceCompatible(a.z.Price, z.z.Price) || *a.z.Price.Value != *z.z.Price.Value {
		return false
	}
	price, err := decimal.NewFromString(*a.z.Price.Value)
	if err != nil {
		return false
	}
	native := q[0].Quantity.Decimal().Mul(price)
	va, oka := reconciliationConvert(native, a.z, b.out.Currency)
	vz, okz := reconciliationConvert(native, z.z, b.out.Currency)
	if !oka || !okz || !va.Equal(vz) || (a.z.Currency != b.out.Currency && !reconciliationFXCompatible(a.z.FX, z.z.FX)) {
		return false
	}
	for i, s := range []*historicalBridgeLeaf{a, z} {
		s.nativePrincipal = s.nativePrincipal.Add(signedDirection(q[i].Direction, native))
		s.principal = s.principal.Add(s.sign(signedDirection(q[i].Direction, va)))
	}
	return true
}

func (b *historicalBridge) pairMoney(effects []domain.ActivityEffect) bool {
	if len(effects) < 2 {
		for _, e := range effects {
			b.flag(b.effectLeaf(e), "principal_pairing")
		}
		return false
	}
	total := decimal.Zero
	values := make([]decimal.Decimal, len(effects))
	ok := true
	for i, e := range effects {
		s := b.effectLeaf(e)
		if s == nil || !s.stable || e.Money == nil || s.z.Currency != e.Money.Currency().String() {
			ok = false
			continue
		}
		v, known := reconciliationConvert(e.Money.Amount(), s.z, b.out.Currency)
		if !known {
			b.flag(s, "missing_evidence")
			ok = false
			continue
		}
		values[i] = s.sign(signedDirection(e.Direction, v))
		total = total.Add(values[i])
	}
	if !total.IsZero() {
		ok = false
		b.reason("internal_imbalance")
	}
	if !ok {
		for _, e := range effects {
			if s := b.effectLeaf(e); s != nil {
				b.flag(s, "internal_imbalance")
				if s.z != nil && s.z.Currency != b.out.Currency {
					s.foreignExposure = true
					b.flag(s, "foreign_internal_exposure")
				}
			}
		}
		return false
	}
	for i, e := range effects {
		s := b.effectLeaf(e)
		s.principal = s.principal.Add(values[i])
	}
	return true
}

func (b *historicalBridge) remeasurement(s *historicalBridgeLeaf, e domain.ActivityEffect) bool {
	if !s.stable || s.a.Quantity == nil || s.z.Quantity == nil || s.z.Price == nil || s.z.Price.Value == nil {
		return false
	}
	if !b.comparablePrices(s) {
		return false
	}
	// Without a stated cost, changing unit prices can be a share restatement
	// (for example a split). Do not invent either income or an adjustment.
	if e.CostUnitPrice == nil && s.a.Price != nil && s.a.Price.Value != nil && *s.a.Price.Value != *s.z.Price.Value {
		b.flag(s, "remeasurement_basis")
		return false
	}
	price, err := decimal.NewFromString(*s.z.Price.Value)
	if err != nil {
		return false
	}
	s.adjustment = s.adjustment.Add(signedDirection(e.Direction, e.Quantity.Decimal().Mul(price)))
	return true
}

func (b *historicalBridge) comparablePrices(s *historicalBridgeLeaf) bool {
	for _, c := range []*HistoricalOverviewCell{s.a, s.z} {
		q, ok := reconciliationDecimal(c.Quantity)
		if !ok {
			b.flag(s, "missing_evidence")
			return false
		}
		if q.IsZero() {
			if c.NativeAmount == nil || *c.NativeAmount != "0" {
				b.flag(s, "valuation_mismatch")
				return false
			}
			continue
		}
		if c.Price == nil || c.Price.Value == nil {
			b.flag(s, "missing_evidence")
			return false
		}
		p, ok := reconciliationDecimal(c.Price.Value)
		native, known := reconciliationDecimal(c.NativeAmount)
		if !ok || !known || !q.Mul(p).Equal(native) {
			b.flag(s, "valuation_mismatch")
			return false
		}
		if c.Price.Currency != c.Currency {
			b.flag(s, "basis_changed")
			return false
		}
	}
	// A genuine zero exposure needs no opening/closing quote. Positive
	// exposures require the same adopted source and valuation authority.
	qa, _ := reconciliationDecimal(s.a.Quantity)
	qz, _ := reconciliationDecimal(s.z.Quantity)
	if !qa.IsZero() && !qz.IsZero() && !reconciliationPriceCompatible(s.a.Price, s.z.Price) {
		b.flag(s, "basis_changed")
		return false
	}
	return true
}

func (b *historicalBridge) market(s *historicalBridgeLeaf) {
	nativeA, _ := reconciliationDecimal(s.a.NativeAmount)
	if s.row.Kind == "holding" {
		qa, oka := reconciliationDecimal(s.a.Quantity)
		qz, okz := reconciliationDecimal(s.z.Quantity)
		if !oka || !okz || !qz.Sub(qa).Equal(s.quantity) {
			b.flag(s, "quantity_mismatch")
			return
		}
		if s.quantityUnsupported {
			return
		}
		if !b.comparablePrices(s) {
			return
		}
		nativeZ, _ := reconciliationDecimal(s.z.NativeAmount)
		// These are endpoint valuation changes after recorded principal,
		// not pure price return: execution-versus-valuation spread is included.
		price, ok := reconciliationConvert(nativeZ.Sub(nativeA).Sub(s.nativePrincipal).Sub(s.adjustment), s.z, b.out.Currency)
		adjustment, known := reconciliationConvert(s.adjustment, s.z, b.out.Currency)
		if !ok || !known {
			b.flag(s, "missing_evidence")
			return
		}
		s.add("price_change", s.sign(price))
		if !s.adjustment.IsZero() {
			s.add("adjustment", s.sign(adjustment))
		}
	}
	if s.a.Currency == b.out.Currency || nativeA.IsZero() {
		return
	}
	if s.foreignExposure {
		b.flag(s, "foreign_internal_exposure")
		return
	}
	if s.a.FX == nil || s.z.FX == nil || s.a.FX.Value == nil || s.z.FX.Value == nil {
		b.flag(s, "missing_evidence")
		return
	}
	if !reconciliationFXCompatible(s.a.FX, s.z.FX) {
		b.flag(s, "basis_changed")
		return
	}
	va, oka := reconciliationConvert(nativeA, s.a, b.out.Currency)
	vz, okz := reconciliationConvert(nativeA, s.z, b.out.Currency)
	if !oka || !okz {
		b.flag(s, "missing_evidence")
		return
	}
	s.add("fx_impact", s.sign(vz.Sub(va)))
}

func reconciliationDecimal(v *string) (decimal.Decimal, bool) {
	if v == nil {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(*v)
	return d, err == nil
}

func (b *historicalBridge) orient(v decimal.Decimal) decimal.Decimal {
	if b.reverse {
		return v.Neg()
	}
	return v
}
func historicalContributions(sums map[string]decimal.Decimal, reverse bool) ([]HistoricalContribution, decimal.Decimal) {
	keys := make([]string, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]HistoricalContribution, 0, len(keys))
	total := decimal.Zero
	for _, k := range keys {
		v := sums[k]
		if reverse {
			v = v.Neg()
		}
		rows = append(rows, HistoricalContribution{Key: k, Amount: v.String()})
		total = total.Add(v)
	}
	return rows, total
}
func (b *historicalBridge) finish() {
	sums := map[string]decimal.Decimal{}
	for _, row := range b.overview.Rows {
		s := b.leaves[row.Key]
		if s == nil {
			continue
		}
		if s.row.Kind == "holding" {
			if s.a != nil {
				s.detail.OpeningQuantity = s.a.Quantity
			}
			if s.z != nil {
				s.detail.ClosingQuantity = s.z.Quantity
			}
			s.detail.ActivityQuantity = historicalString(s.quantity.String())
			qa, oka := reconciliationDecimal(s.detail.OpeningQuantity)
			qz, okz := reconciliationDecimal(s.detail.ClosingQuantity)
			if oka && okz {
				s.detail.QuantityResidual = historicalString(qz.Sub(qa).Sub(s.quantity).String())
			}
		}
		for k, v := range s.sums {
			sums[k] = sums[k].Add(v)
		}
		var supported decimal.Decimal
		s.detail.Contributions, supported = historicalContributions(s.sums, b.reverse)
		s.detail.Supported = supported.String()
		s.detail.Principal = b.orient(s.principal).String()
		if total, ok := reconciliationDecimal(s.detail.Total); ok {
			total = b.orient(total)
			s.detail.Total = historicalString(total.String())
			residual := total.Sub(supported).Sub(b.orient(s.principal))
			s.detail.Residual = historicalString(residual.String())
			if !residual.IsZero() {
				b.flag(s, "unexplained_movement")
			}
		}
		for reason := range s.reasons {
			s.detail.Reasons = append(s.detail.Reasons, reason)
		}
		sort.Strings(s.detail.Reasons)
		b.out.Details = append(b.out.Details, s.detail)
	}
	var supported decimal.Decimal
	b.out.Contributions, supported = historicalContributions(sums, b.reverse)
	if b.out.Total == nil {
		// Known leaf detail remains useful, but an incomplete household has
		// neither a full contribution subtotal nor a fabricated residual.
		b.out.Contributions = []HistoricalContribution{}
		return
	}
	total, _ := reconciliationDecimal(b.out.Total)
	residual := total.Sub(supported)
	b.out.Supported = historicalString(supported.String())
	b.out.Residual = historicalString(residual.String())
	b.out.Status = "reconciled"
	if !residual.IsZero() {
		b.reason("unexplained_movement")
	}
	for _, s := range b.leaves {
		if len(s.reasons) > 0 {
			b.out.Status = "partial"
			break
		}
	}
	if !residual.IsZero() {
		b.out.Status = "partial"
	}
}
