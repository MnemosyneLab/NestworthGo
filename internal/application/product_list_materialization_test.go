package application

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// productListReadCounter observes production SQLite calls without replacing data.
type productListReadCounter struct {
	Repository
	liquidity, portfolio, policies, reservations, products int
	afterSnapshot                                          func()
	afterProducts                                          func()
}

func (r *productListReadCounter) ReadLiquiditySnapshot(ctx context.Context) (domain.LiquiditySnapshot, error) {
	r.liquidity++
	snapshot, err := r.Repository.ReadLiquiditySnapshot(ctx)
	if err == nil && r.afterSnapshot != nil {
		r.afterSnapshot()
	}
	return snapshot, err
}
func (r *productListReadCounter) ReadPortfolioSnapshot(ctx context.Context, filter domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	r.portfolio++
	return r.Repository.ReadPortfolioSnapshot(ctx, filter)
}
func (r *productListReadCounter) ListLiquidityPolicies(ctx context.Context, id domain.HouseholdID) ([]domain.LiquidityPolicy, error) {
	r.policies++
	return r.Repository.ListLiquidityPolicies(ctx, id)
}
func (r *productListReadCounter) ListLiquidityReservations(ctx context.Context, id domain.HouseholdID, released bool) ([]domain.LiquidityReservation, error) {
	r.reservations++
	return r.Repository.ListLiquidityReservations(ctx, id, released)
}
func (r *productListReadCounter) ListProducts(ctx context.Context, id domain.HouseholdID, account *domain.AccountID, closed bool) ([]domain.ProductContract, error) {
	r.products++
	contracts, err := r.Repository.ListProducts(ctx, id, account, closed)
	if err == nil && r.afterProducts != nil {
		r.afterProducts()
	}
	return contracts, err
}
func assertProductListReads(t *testing.T, r *productListReadCounter) {
	t.Helper()
	t.Logf("reads: liquidity=%d portfolio=%d policies=%d reservations=%d products=%d", r.liquidity, r.portfolio, r.policies, r.reservations, r.products)
	if r.liquidity != 1 || r.portfolio != 0 || r.policies != 0 || r.reservations != 0 || r.products != 0 {
		t.Errorf("want one coherent liquidity snapshot and no independent list/portfolio reads")
	}
}
func openProductListFixture(t *testing.T, s *Service, ctx context.Context, account domain.AccountID, name string, startDates ...string) ProductOperationReceipt {
	t.Helper()
	start := "2026-09-20"
	if len(startDates) > 0 {
		start = startDates[0]
	}
	return recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100", Terms: ProductTermsInput{Kind: "term_deposit", Name: name, StartOn: start, MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy(),
	}})
}
func TestProductListOneSnapshotIndependentOfCount(t *testing.T) {
	// 0, 1, and 10 cover an empty list, a single contract, and the settle/undo
	// branches inside a multi-contract snapshot. 50 and 100 repeated that same
	// read-budget assertion without another branch.
	for _, count := range []int{0, 1, 10} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
			account := seedHoldingsCash(t, s, ctx, "1000000")
			for i := 0; i < count; i++ {
				receipt := openProductListFixture(t, s, ctx, account, fmt.Sprintf("Synthetic %03d", i))
				if count >= 10 && i == 0 {
					recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: receipt.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100")}})
				}
				if count >= 10 && i == 1 {
					recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: receipt.OperationID.String()}})
				}
			}
			r := &productListReadCounter{Repository: s.repository}
			s.repository = r
			details, err := s.ListProducts(ctx, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			if len(details) != count {
				t.Fatalf("details=%d, want %d", len(details), count)
			}
			assertProductListReads(t, r)
		})
	}
}

// Frozen projection from main 788faf1 is an independent parity oracle for persisted fixtures.
func baselineProductDetail(s *Service, ctx context.Context, householdID domain.HouseholdID, contract domain.ProductContract) (ProductDetail, error) {
	policies, err := s.repository.ListLiquidityPolicies(ctx, householdID)
	if err != nil {
		return ProductDetail{}, err
	}
	var policy domain.LiquidityPolicy
	key := domain.HoldingSourceRef(contract.AccountID, contract.HoldingID).Key()
	for _, candidate := range policies {
		if candidate.Source.Key() == key {
			policy = candidate
			break
		}
	}
	reservations, err := s.repository.ListLiquidityReservations(ctx, householdID, false)
	if err != nil {
		return ProductDetail{}, err
	}
	active := []domain.LiquidityReservation{}
	for _, reservation := range reservations {
		if reservation.Source.Key() == key && reservation.Active() {
			active = append(active, reservation)
		}
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return ProductDetail{}, err
	}
	valued, _, err := s.valuation.ValueAccounts(snapshot)
	if err != nil {
		return ProductDetail{}, err
	}
	var current *domain.Money
	for _, account := range valued {
		for _, component := range account.Components {
			if component.HoldingID != nil && *component.HoldingID == contract.HoldingID && component.NativeAmount != "" {
				money, err := displayNativeValuation(component.NativeAmount, component.NativeCurrency)
				if err != nil {
					return ProductDetail{}, err
				}
				current = &money
			}
		}
	}
	localDate, _, err := domain.LocalCivilDate(s.clock(), timezoneOrUTC(snapshot.Origin))
	if err != nil {
		return ProductDetail{}, err
	}
	display, err := domain.ProductAvailabilityState(contract, policy, localDate)
	if err != nil {
		return ProductDetail{}, err
	}
	disabled := map[string]string{}
	actions := []string{}
	if contract.State == domain.ProductStateOpen {
		actions = append(actions, "settle", "receive_interest", "renew")
		if contract.Kind == domain.ProductLockedProduct {
			actions = append(actions, "value_observation")
		}
	} else {
		disabled["settle"] = "the contract is not open"
	}
	symbol := ""
	for _, instrument := range snapshot.Instruments {
		if instrument.ID == contract.InstrumentID {
			symbol = pointerText(instrument.Symbol)
			break
		}
	}
	return ProductDetail{
		Symbol: symbol, Contract: contract, Policy: policy, CurrentValue: current, DisplayState: display,
		Reservations: active, PredecessorID: contract.RenewedFromID, PermittedActions: actions, DisabledReasons: disabled,
	}, nil
}

func TestProductListExactBaselineParity(t *testing.T) {
	s, ctx := newProductTestServiceTZ(t, time.Date(2026, 9, 20, 23, 30, 0, 0, time.UTC), "Asia/Tokyo")
	accounts := []domain.AccountID{seedHoldingsCash(t, s, ctx, "1000000"), seedHoldingsCash(t, s, ctx, "1000000")}
	var receipts []ProductOperationReceipt
	for i := 0; i < 5; i++ {
		s.setClock(func() time.Time { return time.Date(2026, 9, 20+i/2, 23, 30, 0, 0, time.UTC) })
		start := "2026-09-20"
		if i == 4 {
			start = "2026-09-19"
		}
		receipts = append(receipts, openProductListFixture(t, s, ctx, accounts[i%2], fmt.Sprintf("Parity %d", i), start))
	}
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: receipts[1].ProductIDs[0].String(), ReturnedPrincipal: strPtr("100")}})
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: receipts[2].OperationID.String()}})
	product, err := s.Product(ctx, receipts[0].ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateProductTerms(ctx, UpdateProductTermsInput{ProductID: product.Contract.ID, ExpectedRevision: product.Contract.Revision,
		Terms: ProductTermsInput{Kind: "term_deposit", Name: product.Contract.Name, ShortName: strPtr("Short fixture"), StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy(),
	}); err != nil {
		t.Fatal(err)
	}
	source := domain.HoldingSourceRef(product.Contract.AccountID, product.Contract.HoldingID)
	for i := 0; i < 3; i++ {
		reservation, err := s.SaveLiquidityReservation(ctx, SaveReservationInput{Source: source, Label: fmt.Sprintf("Reserve %d", i), Amount: "1"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if _, err := s.ReleaseLiquidityReservation(ctx, reservation.ID, reservation.Revision); err != nil {
				t.Fatal(err)
			}
		}
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []*domain.AccountID{nil, &accounts[0], &accounts[1], new(domain.AccountID)} {
		for _, closed := range []bool{false, true} {
			contracts, err := s.repository.ListProducts(ctx, household.ID, account, closed)
			if err != nil {
				t.Fatal(err)
			}
			want := make([]ProductDetail, 0, len(contracts))
			for _, contract := range contracts {
				detail, err := baselineProductDetail(s, ctx, household.ID, contract)
				if err != nil {
					t.Fatal(err)
				}
				want = append(want, detail)
			}
			got, err := s.ListProducts(ctx, account, closed)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("account=%v closed=%t\ngot=%+v\nwant=%+v", account, closed, got, want)
			}
		}
	}
}

func TestProductListWriterInterleavingAndMutationInvalidation(t *testing.T) {
	s, ctx, account, receipt := openRegressionProduct(t)
	before, err := s.ListProducts(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	source := domain.HoldingSourceRef(account, before[0].Contract.HoldingID)
	reserve, err := s.SaveLiquidityReservation(ctx, SaveReservationInput{Source: source, Label: "Active before writer", Amount: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	before, err = s.ListProducts(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	// A second service writes real synthetic SQLite facts after the read boundary.
	// Synchronization waits for commit; no timing or scheduler assumption is needed.
	writer := NewService(s.repository)
	writer.setClock(s.clock)
	r := &productListReadCounter{Repository: s.repository}
	mutate := func() {
		done := make(chan error, 1)
		go func() {
			_, err := writer.UpdateProductTerms(ctx, UpdateProductTermsInput{
				ProductID: receipt.ProductIDs[0], ExpectedRevision: before[0].Contract.Revision,
				Terms: ProductTermsInput{Kind: "term_deposit", Name: "Changed by interleaving writer", StartOn: "2026-09-20", MaturityOn: strPtr("2027-01-20"), InterestMode: "none"}, Policy: depositPolicyForMaturity("2027-01-20"),
			})
			if err == nil {
				_, err = writer.ReleaseLiquidityReservation(ctx, reserve.ID, reserve.Revision)
			}
			done <- err
		}()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	r.afterSnapshot, r.afterProducts = mutate, mutate
	s.repository = r
	got, err := s.ListProducts(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, before) {
		t.Fatalf("response mixed pre/post-write facts:\ngot=%+v\nwant=%+v", got, before)
	}
	assertProductListReads(t, r)
	r.afterSnapshot, r.afterProducts = nil, nil
	after, err := s.ListProducts(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Contract.Name != "Changed by interleaving writer" || after[0].Contract.Revision != before[0].Contract.Revision+1 || *after[0].Policy.UnlockOn != "2027-01-20" || len(after[0].Reservations) != 0 {
		t.Fatalf("next request did not observe committed mutations: %+v", after[0])
	}
	recordRegressionProduct(t, writer, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: receipt.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000")}})
	after, err = s.ListProducts(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Contract.State != domain.ProductStateSettled || after[0].CurrentValue == nil || !after[0].CurrentValue.Amount().IsZero() {
		t.Fatalf("stale settled state/value: %+v", after[0])
	}
	open, err := s.ListProducts(ctx, nil, false)
	if err != nil || len(open) != 0 {
		t.Fatalf("open=%+v err=%v", open, err)
	}
}

func TestProductListZeroLargeValuesAndFixedReadSettings(t *testing.T) {
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, s, ctx, "1000000")
	var ids []domain.ProductContractID
	for i := 0; i < 2; i++ {
		receipt := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
			AccountID: account.String(), Currency: "USD", Principal: "100", Terms: ProductTermsInput{Kind: "locked_product", Name: fmt.Sprintf("Exact %d", i), StartOn: "2026-09-20", InterestMode: "none"},
			Policy: ProductPolicyInput{AccessKind: "on_date", UnlockOn: strPtr("2026-10-05"), EarlyKind: "not_allowed", SettlementDays: intPtr(0), DayBasis: strPtr("calendar")},
		}})
		ids = append(ids, receipt.ProductIDs[0])
	}
	s.setClock(func() time.Time { return time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC) })
	for i, amount := range []string{"0", "999999000000.9999"} {
		if i == 0 {
			recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: ids[i].String(), GrossProceeds: strPtr("100")}})
			continue
		}
		if _, err := s.AppendProductValuation(ctx, AppendProductValuationInput{ProductID: ids[i], Amount: amount, ObservedAt: "2026-09-20T05:00:00Z", MutationID: domain.NewProductOperationID().String()}); err != nil {
			t.Fatal(err)
		}
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := s.repository.ListProducts(ctx, household.ID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]ProductDetail, 0, len(contracts))
	for _, contract := range contracts {
		detail, err := baselineProductDetail(s, ctx, household.ID, contract)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, detail)
	}
	clockCalls, providerCalls, ttlCalls := 0, 0, 0
	s.setClock(func() time.Time { clockCalls++; return time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC) })
	s.valuation.fxProviderKey = func() string { providerCalls++; return "manual" }
	s.valuation.quoteCacheTTL = func() time.Duration { ttlCalls++; return time.Hour }
	got, err := s.ListProducts(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exact locked-product parity failed: got=%+v want=%+v", got, want)
	}
	if clockCalls != 1 || providerCalls != 1 || ttlCalls != 1 {
		t.Fatalf("settings evaluated clock=%d provider=%d ttl=%d, want 1 each", clockCalls, providerCalls, ttlCalls)
	}
	values := map[domain.ProductContractID]string{}
	for _, detail := range got {
		if detail.CurrentValue == nil {
			t.Fatal("missing exact value")
		}
		values[detail.Contract.ID] = detail.CurrentValue.CanonicalAmount()
	}
	if values[ids[0]] != "0" || values[ids[1]] != "999999000000.9999" {
		t.Fatalf("values=%v", values)
	}
	if _, err := s.AppendProductValuation(ctx, AppendProductValuationInput{ProductID: ids[1], Amount: "42.1234", ObservedAt: "2026-09-20T06:00:00Z", MutationID: domain.NewProductOperationID().String()}); err != nil {
		t.Fatal(err)
	}
	after, err := s.ListProducts(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, detail := range after {
		if detail.Contract.ID == ids[1] && (detail.CurrentValue == nil || detail.CurrentValue.CanonicalAmount() != "42.1234") {
			t.Fatalf("stale value after observation: %+v", detail)
		}
	}
}
