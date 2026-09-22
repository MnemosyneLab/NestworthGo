package application

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestProductP2ReservationEditInvalidatesPreview(t *testing.T) {
	s, ctx, a, r := openRegressionProduct(t)
	p, e := s.Product(ctx, r.ProductIDs[0])
	if e != nil {
		t.Fatal(e)
	}
	src := domain.HoldingSourceRef(a, p.Contract.HoldingID)
	reserve, e := s.SaveLiquidityReservation(ctx, SaveReservationInput{Source: src, Label: "Reserve", Amount: "1000"})
	if e != nil {
		t.Fatal(e)
	}
	c := ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: p.Contract.ID.String(), ReturnedPrincipal: strPtr("100000"), ReleaseReservationIDs: []string{reserve.ID.String()}}}
	preview, e := s.PreviewProductOperation(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.SaveLiquidityReservation(ctx, SaveReservationInput{ID: &reserve.ID, ExpectedRevision: reserve.Revision, Source: src, Label: "Changed", Amount: "5000"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.RecordProductOperation(ctx, c, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if e == nil {
		t.Fatal("stale preview accepted and released edited reservation")
	}
}

func TestProductP2MulticurrencyPreviewUsesProductCurrency(t *testing.T) {
	s, ctx, a, _ := openRegressionProduct(t)
	if _, e := s.AppendAccountCashValue(ctx, a, "777", "EUR", ""); e != nil {
		t.Fatal(e)
	}
	c := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: a.String(), Currency: "USD", Principal: "1000", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Second", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	for i := 0; i < 100; i++ {
		p, e := s.PreviewProductOperation(ctx, c)
		if e != nil {
			t.Fatal(e)
		}
		if len(p.CashAfter) != 1 || p.CashAfter[0].Currency() != domain.CurrencyCode("USD") || p.CashAfter[0].CanonicalAmount() != "49000" {
			t.Fatalf("preview %d cashAfter=%v; expected affected USD 49000", i, p.CashAfter)
		}
	}
}

func TestProductP2BackupRejectsContractCurrencyMismatch(t *testing.T) {
	s, ctx, _, r := openRegressionProduct(t)
	path := filepath.Join(t.TempDir(), "corrupt.db")
	if e := s.SnapshotTo(ctx, path); e != nil {
		t.Fatal(e)
	}
	db, e := sqlite.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.SQL.ExecContext(ctx, "UPDATE product_contracts SET currency='EUR' WHERE id=?", r.ProductIDs[0].String())
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	checked, e := sqlite.OpenReadOnlyForVerify(path)
	if checked != nil {
		checked.Close()
	}
	if e == nil {
		t.Fatal("backup verification accepts EUR contract with USD private instrument and principal evidence")
	}
}

func TestProductP2SettledTermsAreReadOnly(t *testing.T) {
	s, ctx, _, r := openRegressionProduct(t)
	p, e := s.Product(ctx, r.ProductIDs[0])
	if e != nil {
		t.Fatal(e)
	}
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: p.Contract.ID.String(), ReturnedPrincipal: strPtr("100000")}})
	p, e = s.Product(ctx, p.Contract.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.UpdateProductTerms(ctx, UpdateProductTermsInput{ProductID: p.Contract.ID, ExpectedRevision: p.Contract.Revision, Terms: ProductTermsInput{Kind: "locked_product", Name: p.Contract.Name, StartOn: p.Contract.StartOn, MaturityOn: p.Contract.MaturityOn, InterestMode: "none"}, Policy: depositPolicy()})
	if e == nil {
		t.Fatal("settled deposit changed to locked_product through metadata endpoint")
	}
}

func TestProductP2CapRoundTripAndManagedRejection(t *testing.T) {
	s, ctx, a, r := openRegressionProduct(t)
	source := domain.AccountCashSourceRef(a, "USD")
	revision := 0
	for _, tc := range []struct {
		cap  *string
		want string
	}{{nil, "50000"}, {strPtr("0"), "0"}, {strPtr("5000"), "5000"}, {nil, "50000"}} {
		policy := depositPolicy()
		policy.AccessKind = "on_request"
		policy.UnlockOn = nil
		policy.AccessibleAmountCap = tc.cap
		saved, err := s.SaveLiquidityPolicy(ctx, SavePolicyInput{Source: source, ExpectedRevision: revision, Policy: policy})
		if err != nil {
			t.Fatal(err)
		}
		revision = saved.Revision
		overview, err := s.LiquidityOverview(ctx, LiquidityOverviewQuery{})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range overview.Sources {
			if row.Ref.Key() == source.Key() {
				found = true
				if row.NormalRoute.NetNative == nil || row.NormalRoute.NetNative.CanonicalAmount() != tc.want {
					t.Fatalf("cap %v: %+v", tc.cap, row.NormalRoute)
				}
			}
		}
		if !found {
			t.Fatal("cash missing")
		}
	}
	detail, err := s.Product(ctx, r.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	policy := depositPolicy()
	policy.AccessibleAmountCap = strPtr("0")
	_, err = s.UpdateProductTerms(ctx, UpdateProductTermsInput{ProductID: detail.Contract.ID, ExpectedRevision: detail.Contract.Revision, Terms: ProductTermsInput{Kind: "term_deposit", Name: "Review", StartOn: detail.Contract.StartOn, MaturityOn: detail.Contract.MaturityOn, InterestMode: "none"}, Policy: policy})
	if err == nil {
		t.Fatal("managed cap accepted")
	}
}

func TestProductP2PreviewValuesAndActualTime(t *testing.T) {
	s, ctx, a, r := openRegressionProduct(t)
	c := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: a.String(), Currency: "USD", Principal: "1000", OpeningFee: strPtr("10"), EffectiveLocalDate: "2026-09-20", EffectiveLocalTime: "04:30", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Second", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	p, err := s.PreviewProductOperation(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if p.ProductBefore == nil || p.ProductBefore.CanonicalAmount() != "0" || p.ProductAfter == nil || p.ProductAfter.CanonicalAmount() != "1000" || !p.NetWorthKnown || p.NetWorthDelta.CanonicalAmount() != "-10" || p.EffectiveAt.Format(time.RFC3339) != "2026-09-20T04:30:00Z" {
		t.Fatalf("preview=%+v", p)
	}
	receipt := recordRegressionProduct(t, s, ctx, c)
	if receipt.CashAfter[0].CanonicalAmount() != "48990" {
		t.Fatal(receipt)
	}
	renew := ProductCommand{Kind: domain.ProductOpRenew, Renew: &RenewProductCommand{Settle: SettleProductCommand{ProductID: r.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("1000"), EffectiveLocalDate: "2026-09-20", EffectiveLocalTime: "04:45"}, Principal: "100000", OpeningFee: strPtr("10"), Terms: c.Open.Terms, Policy: depositPolicy()}}
	p, err = s.PreviewProductOperation(ctx, renew)
	if err != nil {
		t.Fatal(err)
	}
	if p.ProductBefore.CanonicalAmount() != "100000" || p.ProductAfter.CanonicalAmount() != "100000" || p.NetWorthDelta.CanonicalAmount() != "990" {
		t.Fatalf("renew=%+v", p)
	}
	for _, a := range p.Activities {
		if !a.Activity.EffectiveAt.Equal(p.EffectiveAt) {
			t.Fatal("renewal times differ")
		}
	}
	recordRegressionProduct(t, s, ctx, renew)
}

func TestProductP2ClosedMetadataOnly(t *testing.T) {
	s, ctx, _, r := openRegressionProduct(t)
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: r.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000")}})
	detail, err := s.Product(ctx, r.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	input := UpdateProductTermsInput{ProductID: detail.Contract.ID, ExpectedRevision: detail.Contract.Revision, Terms: ProductTermsInput{Kind: "term_deposit", Name: "Renamed", Note: strPtr("Metadata"), StartOn: detail.Contract.StartOn, MaturityOn: detail.Contract.MaturityOn, InterestMode: "none"}, Policy: depositPolicy()}
	changed, err := s.UpdateProductTerms(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Contract.Name != "Renamed" || changed.Policy.Revision != detail.Policy.Revision {
		t.Fatal("metadata changed policy")
	}
	input.ExpectedRevision = changed.Contract.Revision
	input.Policy.NormalExitFee = strPtr("5")
	if _, err = s.UpdateProductTerms(ctx, input); err == nil {
		t.Fatal("closed policy edit allowed")
	}
	input.Policy = depositPolicy()
	input.Terms.MaturityOn = strPtr("2027-01-01")
	if _, err = s.UpdateProductTerms(ctx, input); err == nil {
		t.Fatal("closed maturity edit allowed")
	}
}

func TestProductP2BackupLifecycleAndCorruption(t *testing.T) {
	s, ctx, a, r := openRegressionProduct(t)
	product, err := s.Product(ctx, r.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := s.SaveLiquidityReservation(ctx, SaveReservationInput{Source: domain.HoldingSourceRef(a, product.Contract.HoldingID), Label: "Reserved", Amount: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	renew := ProductCommand{Kind: domain.ProductOpRenew, Renew: &RenewProductCommand{Settle: SettleProductCommand{ProductID: r.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("1000"), ReleaseReservationIDs: []string{reserve.ID.String()}}, Principal: "100000", Terms: ProductTermsInput{Kind: "term_deposit", Name: "New term", StartOn: "2026-09-20", MaturityOn: strPtr("2027-09-20"), InterestMode: "none"}, Policy: depositPolicyForMaturity("2027-09-20")}}
	receipt := recordRegressionProduct(t, s, ctx, renew)
	for _, stage := range []string{"renewed", "undone"} {
		if stage == "undone" {
			recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: receipt.OperationID.String()}})
		}
		path := filepath.Join(t.TempDir(), stage+".db")
		if err := s.SnapshotTo(ctx, path); err != nil {
			t.Fatal(err)
		}
		verified, err := sqlite.OpenReadOnlyForVerify(path)
		if err != nil {
			t.Fatal(err)
		}
		verified.Close()
		db, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := sqlite.NewRepository(db).ReadLiquiditySnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want, err := s.repository.ReadLiquiditySnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Contracts) != len(want.Contracts) || len(got.Reservations) != len(want.Reservations) {
			t.Fatal("snapshot facts differ")
		}
		for i := range want.Contracts {
			if got.Contracts[i].ID != want.Contracts[i].ID || got.Contracts[i].State != want.Contracts[i].State || got.Contracts[i].Revision != want.Contracts[i].Revision {
				t.Fatal("contracts differ")
			}
		}
		for i := range want.Reservations {
			if got.Reservations[i].Revision != want.Reservations[i].Revision || !sameOptionalTime(got.Reservations[i].ReleasedAt, want.Reservations[i].ReleasedAt) {
				t.Fatal("reservations differ")
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("restored liquidity snapshot differs, including cash and valuation facts")
		}
		restored := NewService(sqlite.NewRepository(db))
		restored.setClock(s.clock)
		gotOverview, err := restored.LiquidityOverview(ctx, LiquidityOverviewQuery{IncludeEarlyWithdrawal: true})
		if err != nil {
			t.Fatal(err)
		}
		wantOverview, err := s.LiquidityOverview(ctx, LiquidityOverviewQuery{IncludeEarlyWithdrawal: true})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotOverview, wantOverview) {
			t.Fatal("restored availability differs")
		}
		db.Close()
	}
	for _, tc := range []struct{ name, sql string }{{"quantity", `UPDATE holdings SET quantity='2' WHERE id=(SELECT holding_id FROM product_contracts WHERE state='open' LIMIT 1)`}, {"type", `UPDATE instruments SET instrument_type='etf' WHERE id=(SELECT instrument_id FROM product_contracts LIMIT 1)`}, {"policy_cap", `UPDATE liquidity_policies SET accessible_amount_cap='100' WHERE holding_id IS NOT NULL`}, {"reservation_currency", `UPDATE liquidity_reservations SET currency='EUR'`}, {"contract_dates", `UPDATE product_contracts SET maturity_on='2020-01-01'`}} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "corrupt.db")
			if err := s.SnapshotTo(ctx, path); err != nil {
				t.Fatal(err)
			}
			db, err := sqlite.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.SQL.ExecContext(ctx, tc.sql); err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()
			check, err := sqlite.OpenReadOnlyForVerify(path)
			if check != nil {
				check.Close()
			}
			if err == nil {
				t.Fatal("corrupt backup accepted")
			}
			check, err = sqlite.Open(path)
			if check != nil {
				check.Close()
			}
			if err == nil {
				t.Fatal("corrupt live database accepted")
			}
		})
	}
}

func TestProductP2LocalTimeUsesOriginAndRejectsInvalidTime(t *testing.T) {
	origin := &domain.HistoryOrigin{Timezone: "America/New_York", StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	now := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	got, err := resolveProductEffectiveTime("", "2026-09-20", "12:30", origin, now)
	if err != nil || got != "2026-09-20T16:30:00Z" {
		t.Fatalf("%s %v", got, err)
	}
	for _, tc := range [][2]string{{"2026-03-08", "02:30"}, {"2026-11-01", "01:30"}, {"2025-12-31", "12:00"}, {"2027-01-01", "12:00"}, {"2026-09-20", ""}} {
		if _, err := resolveProductEffectiveTime("", tc[0], tc[1], origin, now); err == nil {
			t.Fatalf("accepted %v", tc)
		}
	}
}

func TestProductP2UnknownValuationIsNotZero(t *testing.T) {
	s, ctx, _, r := openRegressionProduct(t)
	detail, err := s.Product(ctx, r.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.InstrumentQuotes = nil
	before, after, err := s.productPreviewValues(snapshot, productPlan{beforeContracts: []domain.ProductContract{detail.Contract}, contracts: []domain.ProductContract{detail.Contract}})
	if err != nil {
		t.Fatal(err)
	}
	if before != nil || after != nil {
		t.Fatal("missing quote converted to known product value")
	}
	delta, err := productNetWorthDelta(before, after, nil, nil)
	if err != nil || delta != nil {
		t.Fatal("unknown impact became zero")
	}
}

func TestProductP2ClosedMetadataAcceptsEquivalentDecimalTerms(t *testing.T) {
	s, ctx, _, r := openRegressionProduct(t)
	p, err := s.Product(ctx, r.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	input := UpdateProductTermsInput{ProductID: p.Contract.ID, ExpectedRevision: p.Contract.Revision, Terms: ProductTermsInput{Kind: "term_deposit", Name: "Interest", StartOn: p.Contract.StartOn, MaturityOn: p.Contract.MaturityOn, InterestMode: "simple_act_365", AnnualRatePercent: strPtr("2.5"), InterestPaidThroughOn: strPtr(p.Contract.StartOn)}, Policy: depositPolicy()}
	if _, err = s.UpdateProductTerms(ctx, input); err != nil {
		t.Fatal(err)
	}
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: p.Contract.ID.String(), ReturnedPrincipal: strPtr("100000")}})
	p, err = s.Product(ctx, p.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedRevision = p.Contract.Revision
	input.Terms.Name = "Renamed"
	input.Terms.AnnualRatePercent = strPtr("2.500")
	input.Policy.NormalExitFee = strPtr("0.00")
	got, err := s.UpdateProductTerms(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.Name != "Renamed" || got.Contract.AnnualRate.Canonical() != p.Contract.AnnualRate.Canonical() || got.Policy.Revision != p.Policy.Revision {
		t.Fatal("metadata edit altered financial terms")
	}
}
