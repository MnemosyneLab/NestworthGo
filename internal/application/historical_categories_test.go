package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func categoryLeaf(key, role, class, currency, a, b string) HistoricalOverviewRow {
	cell := func(amount string) *HistoricalOverviewCell {
		return &HistoricalOverviewCell{Status: "active", Included: true, Complete: true, BaseAmount: historicalString(amount), Currency: currency, AssetClass: class}
	}
	return HistoricalOverviewRow{Key: key, ParentKey: "parent", Kind: "balance", Role: role, Left: cell(a), Right: cell(b)}
}

func categoryResult(rows ...HistoricalOverviewRow) HistoricalOverviewResult {
	return HistoricalOverviewResult{Left: HistoricalOverviewState{Currency: "CNY"}, Right: &HistoricalOverviewState{Currency: "CNY"}, Rows: rows}
}

func categoryAt(t *testing.T, result HistoricalOverviewResult, role, dimension, key string) HistoricalCategoryComparison {
	t.Helper()
	for _, c := range historicalCategoryComparisons(result) {
		if c.Role == role && c.Dimension == dimension && c.Key == key {
			return c
		}
	}
	t.Fatalf("missing category %s/%s/%s", role, dimension, key)
	return HistoricalCategoryComparison{}
}

func TestHistoricalCategoriesExactDistinctRolesAndNoParents(t *testing.T) {
	t.Parallel()
	cash := categoryLeaf("cash", "asset", "cash", "CNY", "1", "2")
	stock := categoryLeaf("stock", "asset", "stock", "USD", "3", "6.00012")
	debt := categoryLeaf("debt", "liability", "loan", "CNY", "2", "1")
	parent := categoryLeaf("parent", "asset", "cash", "CNY", "999", "999")
	parent.Kind = "account"
	parent.Key = "parent"
	parent.Left.BaseAmount = historicalString("999")
	result := categoryResult(cash, stock, debt, parent)
	c := categoryAt(t, result, "asset", "byClass", "cash")
	wantOverviewAmount(t, c.Left.Amount, "1")
	wantOverviewAmount(t, c.Right.Amount, "2")
	wantOverviewAmount(t, c.AmountChange, "1")
	wantOverviewAmount(t, c.Left.Share, "25")
	wantOverviewAmount(t, c.Right.Share, "25")
	wantOverviewAmount(t, c.ShareChangePp, "0")
	c = categoryAt(t, result, "asset", "byCurrency", "USD")
	wantOverviewAmount(t, c.AmountChange, "3.00012")
	c = categoryAt(t, result, "liability", "byClass", "loan")
	wantOverviewAmount(t, c.AmountChange, "-1")
	wantOverviewAmount(t, c.Left.Share, "100")
	reverse := result
	for i := range reverse.Rows {
		reverse.Rows[i].Left, reverse.Rows[i].Right = reverse.Rows[i].Right, reverse.Rows[i].Left
	}
	c = categoryAt(t, reverse, "liability", "byClass", "loan")
	wantOverviewAmount(t, c.AmountChange, "1")
}

func TestHistoricalCategoryPercentagePointsNotGrowth(t *testing.T) {
	t.Parallel()
	c := categoryAt(t, categoryResult(categoryLeaf("cash", "asset", "cash", "CNY", "1", "3"), categoryLeaf("stock", "asset", "stock", "CNY", "3", "1")), "asset", "byClass", "cash")
	wantOverviewAmount(t, c.AmountChange, "2")
	wantOverviewAmount(t, c.ShareChangePp, "50")
}

func TestHistoricalCategoryUnavailableBoundaries(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"partial", "absent", "excluded", "scope_changed", "existence_changed", "currency_mismatch", "zero", "negative", "tiny", "same_unknown"} {
		t.Run(name, func(t *testing.T) {
			leaf := categoryLeaf("a", "asset", "cash", "USD", "10", "12")
			result := categoryResult(leaf)
			switch name {
			case "partial", "same_unknown":
				leaf.Left.Complete = false
				leaf.Left.BaseAmount = nil
				result.Rows[0] = leaf
				if name == "same_unknown" {
					result.Rows[0].Right = leaf.Left
				}
			case "absent":
				result.Rows[0].Left = nil
			case "excluded":
				result.Rows[0].Left.Included = false
				result.Rows[0].Right.Included = false
			case "scope_changed":
				result.Rows[0].Right.Included = false
			case "existence_changed":
				extra := categoryLeaf("b", "asset", "cash", "USD", "1", "1")
				extra.Left = nil
				result.Rows = append(result.Rows, extra)
			case "currency_mismatch":
				result.Right.Currency = "USD"
			case "zero":
				result.Rows[0] = categoryLeaf("a", "asset", "cash", "USD", "0", "0")
			case "negative":
				result.Rows[0] = categoryLeaf("a", "asset", "cash", "USD", "-1", "1")
			case "tiny":
				result.Rows[0] = categoryLeaf("a", "asset", "cash", "USD", "0", "0.000000000000000000000001")
			}
			c := categoryAt(t, result, "asset", "byCurrency", "USD")
			if name == "zero" {
				wantOverviewAmount(t, c.AmountChange, "0")
				if c.ShareChangePp != nil || c.Left.Share != nil {
					t.Fatal("zero denominator invented share")
				}
				return
			}
			if name == "negative" {
				wantOverviewAmount(t, c.AmountChange, "2")
				if c.ShareChangePp != nil {
					t.Fatal("negative shares compared")
				}
				return
			}
			if name == "tiny" {
				wantOverviewAmount(t, c.AmountChange, "0.000000000000000000000001")
				return
			}
			if c.AmountChange != nil || c.ShareChangePp != nil {
				t.Fatalf("incomparable category has delta %+v", c)
			}
			if !strings.Contains(strings.Join(c.AmountReasons, ","), strings.ReplaceAll(name, "same_unknown", "partial")) {
				t.Fatalf("missing reason %s: %+v", name, c)
			}
		})
	}
}

func TestHistoricalCategoryCoverageIndependentByRoleAndCategory(t *testing.T) {
	t.Parallel()
	cash := categoryLeaf("cash", "asset", "cash", "CNY", "10", "12")
	debt := categoryLeaf("loan", "liability", "loan", "USD", "", "20")
	debt.Left.Complete = false
	c := categoryAt(t, categoryResult(cash, debt), "asset", "byClass", "cash")
	wantOverviewAmount(t, c.AmountChange, "2")
	wantOverviewAmount(t, c.ShareChangePp, "0")
	missing := categoryLeaf("missing", "asset", "stock", "USD", "", "3")
	missing.Left.Complete = false
	c = categoryAt(t, categoryResult(cash, missing), "asset", "byClass", "cash")
	wantOverviewAmount(t, c.AmountChange, "2")
	if c.ShareChangePp != nil || c.Left.ShareBasis != "known_subtotal" {
		t.Fatalf("partial denominator %+v", c)
	}
}

func TestHistoricalOverviewP1ReadOnlyWireAndFrozenEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	foreign := overviewAccount(t, s, owner, "Foreign", "bank_account", "asset", "balance", "USD", "10")
	loan := overviewAccount(t, s, owner, "Debt", "loan", "liability", "balance", "CNY", "20")
	if _, err := s.AppendManualFXQuote(ctx, "CNY", "USD", "0.125", "2026-08-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if _, err := s.AppendManualFXQuote(ctx, "CNY", "USD", "0.1", "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	r, err := s.HistoricalOverview(ctx, "2026-08-01", "current")
	if err != nil {
		t.Fatal(err)
	}
	row := overviewRow(t, r, foreign.Account.ID.String()+":balance")
	if row.AccountID != foreign.Account.ID.String() || row.Left.FX == nil || row.Left.FX.Operation != "divide" || row.Left.FX.BaseCurrency != "CNY" || row.Left.FX.QuoteCurrency != "USD" {
		t.Fatalf("oriented evidence %+v", row)
	}
	wantOverviewAmount(t, row.Left.FX.Value, "0.125")
	wantOverviewAmount(t, row.Right.FX.Value, "0.1")
	c := categoryAt(t, r, "asset", "byCurrency", "USD")
	wantOverviewAmount(t, c.AmountChange, "20")
	c = categoryAt(t, r, "liability", "byCurrency", "CNY")
	wantOverviewAmount(t, c.AmountChange, "0")
	if overviewRow(t, r, loan.Account.ID.String()).AccountID != loan.Account.ID.String() {
		t.Fatal("account identity missing")
	}
	wire, err := jsoncodec.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"categories"`, `"shareChangePp"`, `"missingDetails"`, `"operation":"divide"`, `"accountId"`} {
		if !strings.Contains(string(wire), field) {
			t.Fatalf("wire lacks %s", field)
		}
	}
	// The returned value never references new observations or refreshed current data.
	*now = now.Add(time.Hour)
	wantOverviewAmount(t, row.Right.FX.Value, "0.1")
}

func TestHistoricalEvidenceOnlySelectedObservation(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	id := domain.NewInstrumentQuoteID()
	price, _ := domain.ParseUnitPrice("0.00000012")
	q := domain.InstrumentQuote{ID: id, UnitPrice: price, Currency: "USD", EffectiveDate: "2026-08-01", ValueEffectiveAt: at, ObservationKind: "daily_close", PriceBasis: "raw", TimestampBasis: "session_close", SourcePolicyVersion: "policy"}
	evidence := &domain.QuoteEvidenceView{ObservationID: id.String(), Source: domain.QuoteSourceProvider, SourceKey: "provider", QuotedAt: at}
	e := historicalPriceEvidence(evidence, []domain.InstrumentQuote{q}, at, true, time.Hour)
	wantOverviewAmount(t, e.Value, "0.00000012")
	if e.SourceKey != "provider" || e.PriceBasis != "raw" || e.TimestampBasis != "session_close" || e.MarketDate != "2026-08-01" {
		t.Fatalf("lost source metadata %+v", e)
	}
	e = historicalPriceEvidence(evidence, nil, at, true, time.Hour)
	if e.Value != nil || e.PriceBasis != "" {
		t.Fatal("invented unretained evidence")
	}
}

func TestHistoricalCategoryPartialBothSidesAndIncludedZero(t *testing.T) {
	t.Parallel()
	a := categoryLeaf("a", "asset", "cash", "CNY", "10", "")
	a.Right.Complete = false
	b := categoryLeaf("b", "asset", "cash", "CNY", "", "12")
	b.Left.Complete = false
	c := categoryAt(t, categoryResult(a, b), "asset", "byClass", "cash")
	if c.AmountChange != nil || c.Left.Amount != nil || c.Right.Amount != nil || c.ShareChangePp != nil {
		t.Fatalf("different missing sets were compared: %+v", c)
	}
	wantOverviewAmount(t, c.Left.KnownAmount, "10")
	wantOverviewAmount(t, c.Right.KnownAmount, "12")
	zero := categoryLeaf("zero", "asset", "stock", "CNY", "0", "0")
	c = categoryAt(t, categoryResult(zero, categoryLeaf("cash", "asset", "cash", "CNY", "10", "20")), "asset", "byClass", "stock")
	wantOverviewAmount(t, c.Left.Amount, "0")
	wantOverviewAmount(t, c.AmountChange, "0")
	wantOverviewAmount(t, c.ShareChangePp, "0")
}

func TestHistoricalOverviewP1MissingDetailsAndExplicitHoldingIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, db, owner, now := overviewFixture(t)
	broker := overviewAccount(t, s, owner, "Broker", "brokerage", "asset", "holdings", "CNY", "")
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Unpriced", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, "1", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holding.ID: "1"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if _, err := db.SQL.Exec(`DELETE FROM instrument_quotes WHERE instrument_id = ?`, instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	r, err := s.HistoricalOverview(ctx, "2026-08-01", "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	row := overviewRow(t, r, broker.Account.ID.String()+":holding:"+holding.ID.String())
	if row.HoldingID != holding.ID.String() || row.InstrumentID != instrument.ID.String() || row.AccountID != broker.Account.ID.String() {
		t.Fatalf("lost navigation identity: %+v", row)
	}
	if row.Left.Price != nil || len(row.Left.MissingDetails) != 1 || row.Left.MissingDetails[0].Kind != "instrument_price" || row.Left.MissingDetails[0].QuoteCurrency != "USD" || row.Left.MissingDetails[0].InstrumentID != instrument.ID.String() {
		t.Fatalf("missing detail: %+v", row.Left)
	}
	c := categoryAt(t, r, "asset", "byClass", "stock")
	if c.Left.Amount != nil || c.AmountChange != nil {
		t.Fatalf("unknown same date became zero: %+v", c)
	}
}

func TestHistoricalCategoryCurrencyMembershipChangeNotHiddenInAggregate(t *testing.T) {
	t.Parallel()
	moved := categoryLeaf("moved", "asset", "cash", "USD", "10", "12")
	moved.Right.Currency = "CNY"
	r := categoryResult(moved, categoryLeaf("usd", "asset", "cash", "USD", "1", "1"), categoryLeaf("cny", "asset", "cash", "CNY", "2", "2"))
	for _, key := range []string{"USD", "CNY"} {
		c := categoryAt(t, r, "asset", "byCurrency", key)
		if c.Left.Amount == nil || c.Right.Amount == nil || c.AmountChange != nil || c.ShareChangePp != nil || !strings.Contains(strings.Join(c.AmountReasons, ","), "membership_changed") {
			t.Fatalf("category membership hidden: %+v", c)
		}
	}
	c := categoryAt(t, r, "asset", "byClass", "cash")
	wantOverviewAmount(t, c.AmountChange, "2")
}

func TestHistoricalCategoryAllMissingIsNotObservedZero(t *testing.T) {
	t.Parallel()
	missing := categoryLeaf("missing", "asset", "stock", "USD", "", "")
	missing.Left.Complete = false
	missing.Right.Complete = false
	observed := categoryLeaf("observed", "asset", "cash", "CNY", "10", "10")
	c := categoryAt(t, categoryResult(missing, observed), "asset", "byClass", "stock")
	for _, side := range []HistoricalCategoryCell{c.Left, c.Right} {
		if side.Status != "partial" || side.KnownAmount != nil || side.Amount != nil || side.Share != nil || side.ShareBasis != "unavailable" {
			t.Fatalf("all-missing category became observed zero: %+v", side)
		}
	}
	zero := categoryLeaf("zero", "asset", "stock", "USD", "0", "0")
	c = categoryAt(t, categoryResult(zero, observed), "asset", "byClass", "stock")
	wantOverviewAmount(t, c.Left.KnownAmount, "0")
	wantOverviewAmount(t, c.Left.Amount, "0")
	wantOverviewAmount(t, c.Left.Share, "0")
	c = categoryAt(t, categoryResult(zero, missing, observed), "asset", "byClass", "stock")
	wantOverviewAmount(t, c.Left.KnownAmount, "0")
	wantOverviewAmount(t, c.Left.Share, "0")
	if c.Left.Amount != nil || c.Left.ShareBasis != "known_subtotal" {
		t.Fatalf("observed zero within partial category lost: %+v", c.Left)
	}
}
