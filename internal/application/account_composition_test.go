package application

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func compositionFixture() domain.AccountValuation {
	return domain.AccountValuation{Account: domain.Account{TrackingMode: domain.TrackingHoldings, BalanceSheetRole: domain.RoleAsset}, Complete: true}
}

func compositionComponent(key, amount string) domain.ValuationComponent {
	holding := domain.HoldingID(key)
	instrument := domain.InstrumentID(key)
	component := domain.ValuationComponent{HoldingID: &holding, InstrumentID: &instrument, NativeAmount: amount, NativeCurrency: "USD", Available: amount != ""}
	if amount != "" {
		component.BaseAmountExact = amount
		component.BaseAmount = &domain.MoneyView{Amount: "0", Currency: "USD"}
	}
	return component
}

func assertCompositionSums(t *testing.T, result domain.AccountComposition) {
	t.Helper()
	detailShares := map[string]int{}
	for _, detail := range result.Details {
		detailShares[detail.Key] = detail.ShareBPS
	}
	for _, rows := range [][]domain.AccountCompositionItem{result.Types, result.Details, result.DetailSlices} {
		total, shares := decimal.Zero, 0
		refs := map[string]bool{}
		for _, row := range rows {
			if row.BaseAmountExact != "" {
				total = total.Add(decimal.RequireFromString(row.BaseAmountExact))
			}
			shares += row.ShareBPS
			originalShare := 0
			for _, ref := range row.DetailRefs {
				originalShare += detailShares[ref]
			}
			if row.ShareBPS != originalShare {
				t.Fatalf("row %s share=%d, original share sum=%d", row.Key, row.ShareBPS, originalShare)
			}
			for _, ref := range row.DetailRefs {
				if refs[ref] {
					t.Fatalf("duplicate detail ref %s", ref)
				}
				refs[ref] = true
			}
		}
		if total.String() != result.TotalExact || shares != 10000 || len(refs) != len(result.Details) {
			t.Fatalf("inconsistent projection: %s/%d/%d: %+v", total, shares, len(refs), result)
		}
	}
}

func TestAccountCompositionCanonicalTypesAndUnknown(t *testing.T) {
	account := compositionFixture()
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for index, kind := range append(domain.AllInstrumentTypes(), domain.InstrumentType(""), domain.InstrumentType("future_type")) {
		key := fmt.Sprintf("item-%02d", index)
		item := compositionComponent(key, "10")
		account.Components = append(account.Components, item)
		archived := time.Now()
		instruments[*item.InstrumentID] = domain.Instrument{ID: *item.InstrumentID, Type: kind, Name: "Cash stock ETF", ArchivedAt: &archived}
	}
	account.Components = append(account.Components, domain.ValuationComponent{NativeCurrency: "SGD", NativeAmount: "99", BaseAmountExact: "20", Available: true}, domain.ValuationComponent{NativeCurrency: "USD", NativeAmount: "10", BaseAmountExact: "10", Available: true})
	result, err := projectAccountComposition(account, instruments, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Types) != 10 || len(result.Details) != 12 || result.Types[0].TypeKey != "cash" || result.Types[0].BaseAmountExact != "30" {
		t.Fatalf("canonical groups = %+v", result)
	}
	found := map[string]int{}
	for _, group := range result.Types {
		found[group.TypeKey] = len(group.DetailRefs)
	}
	if found["unknown"] != 2 || found["other"] != 1 || found["etf"] != 1 || found["mutual_fund"] != 1 {
		t.Fatal(found)
	}
	if result.DetailSlices[10].Key == "account-composition:type:other" {
		t.Fatal("remainder collides")
	}
	assertCompositionSums(t, result)
	// Input ordering cannot change amount ordering or group/detail identities.
	for i, j := 0, len(account.Components)-1; i < j; i, j = i+1, j-1 {
		account.Components[i], account.Components[j] = account.Components[j], account.Components[i]
	}
	again, err := projectAccountComposition(account, instruments, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, again) {
		t.Fatal("unstable projection")
	}
}

func TestAccountCompositionDetailLimitsAndExactRemainder(t *testing.T) {
	for _, count := range []int{0, 1, 10, 11, 80} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			account := compositionFixture()
			for i := 0; i < count; i++ {
				account.Components = append(account.Components, compositionComponent(fmt.Sprintf("item-%03d", i), "0.00005"))
			}
			result, err := projectAccountComposition(account, nil, "USD")
			if err != nil {
				t.Fatal(err)
			}
			expectedSlices := count
			if count > 10 {
				expectedSlices = 11
			}
			if len(result.Details) != count || len(result.DetailSlices) != expectedSlices {
				t.Fatal(result)
			}
			if count == 0 {
				if result.Status != "empty" {
					t.Fatal(result)
				}
				return
			}
			assertCompositionSums(t, result)
			if result.Types[0].BaseAmountExact != decimal.RequireFromString("0.00005").Mul(decimal.NewFromInt(int64(count))).String() {
				t.Fatal(result)
			}
			if count > 10 && (result.DetailSlices[10].Key != accountCompositionRemainderKey || len(result.DetailSlices[10].DetailRefs) != count-10) {
				t.Fatal(result)
			}
		})
	}
}

func TestAccountCompositionAvailabilityZeroAndInvalid(t *testing.T) {
	for _, test := range []struct {
		name, status    string
		amounts         []string
		valued, missing int
	}{
		{"partial", "partial", []string{"10", ""}, 1, 1},
		{"all missing", "unavailable", []string{"", ""}, 0, 2},
		{"zero", "empty", []string{"0", "0"}, 2, 0},
		{"zero and missing", "empty", []string{"0", ""}, 1, 1},
		{"negative positive total", "invalid", []string{"100", "-1"}, 2, 0},
		{"negative zero total", "invalid", []string{"1", "-1"}, 2, 0},
		{"negative total", "invalid", []string{"-1"}, 1, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := compositionFixture()
			account.Complete = test.missing == 0
			for i, amount := range test.amounts {
				account.Components = append(account.Components, compositionComponent(fmt.Sprint(i), amount))
			}
			result, err := projectAccountComposition(account, nil, "USD")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != test.status || result.ValuedCount != test.valued || result.MissingCount != test.missing || result.Complete != account.Complete {
				t.Fatal(result)
			}
			if test.status == "partial" {
				assertCompositionSums(t, result)
			}
			for _, row := range result.Details {
				if row.ValuedCount == 0 && (row.BaseAmount != nil || row.BaseAmountExact != "") {
					t.Fatal("missing became zero")
				}
				if test.status == "invalid" && row.ShareBPS != 0 {
					t.Fatal("negative was reweighted")
				}
			}
		})
	}
	account := compositionFixture()
	account.Account.BalanceSheetRole = domain.RoleLiability
	result, err := projectAccountComposition(account, nil, "USD")
	if err != nil || result.Status != "unsupported" {
		t.Fatal(result, err)
	}
}

func TestAccountCompositionMaxTinyAndCorruptAmount(t *testing.T) {
	account := compositionFixture()
	account.Components = []domain.ValuationComponent{compositionComponent("max", "999999999999.9998"), compositionComponent("tiny", "0.000000000001")}
	result, err := projectAccountComposition(account, nil, "USD")
	if err != nil {
		t.Fatal(err)
	}
	assertCompositionSums(t, result)
	if result.Details[1].BaseAmountExact != "0.000000000001" || result.Details[1].BaseAmount.Amount != "0" {
		t.Fatal(result)
	}
	account.Components[0].BaseAmountExact = "NaN"
	if _, err = projectAccountComposition(account, nil, "USD"); err == nil {
		t.Fatal("corrupt exact amount accepted")
	}
}

func TestAccountCompositionManagedProductsShareCanonicalType(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "20000")
	for _, kind := range []string{"term_deposit", "locked_product"} {
		terms := ProductTermsInput{Kind: kind, Name: kind, StartOn: "2026-09-20", InterestMode: "none"}
		policy := depositPolicy()
		if kind == "term_deposit" {
			terms.MaturityOn = strPtr("2026-12-20")
		} else {
			policy = ProductPolicyInput{AccessKind: "on_date", UnlockOn: strPtr("2026-10-05"), EarlyKind: "not_allowed", SettlementDays: intPtr(0), DayBasis: strPtr("calendar")}
		}
		command := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: account.String(), Currency: "USD", Principal: "1000", EffectiveAt: "2026-09-20T04:00:00Z", Terms: terms, Policy: policy}}
		preview, err := service.PreviewProductOperation(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.RecordProductOperation(ctx, command, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err != nil {
			t.Fatal(err)
		}
	}
	valuation, err := service.AccountValuation(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	result := valuation.Composition
	if len(result.Types) != 2 || result.TotalExact != "20000" || result.Types[1].TypeKey != "bank_investment_product" || result.Types[1].BaseAmountExact != "2000" || len(result.Types[1].DetailRefs) != 2 {
		t.Fatal(result)
	}
	assertCompositionSums(t, *result)
}

func TestAccountCompositionSnapshotFXAndPartial(t *testing.T) {
	fixture := newGoldenValuationFixture(t, true)
	valuation, err := fixture.service.AccountValuation(t.Context(), fixture.account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	result := valuation.Composition
	if result.TotalExact != "62190" || len(result.Types) != 3 || result.Types[0].TypeKey != "cash" || result.Types[0].BaseAmountExact != "26500" {
		t.Fatal(result)
	}
	assertCompositionSums(t, *result)
	native := map[string]string{}
	for _, detail := range result.Details {
		native[detail.TypeKey] = detail.NativeAmount + " " + detail.NativeCurrency.String()
	}
	if native["cash"] != "5000 SGD" || native["etf"] != "2100 USD" || native["stock"] != "4000 SGD" {
		t.Fatal(native)
	}
	partial := newGoldenValuationFixture(t, false)
	valuation, err = partial.service.AccountValuation(t.Context(), partial.account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if valuation.Composition.Status != "partial" || valuation.Composition.TotalExact != "40990" || valuation.Composition.MissingCount != 1 {
		t.Fatal(valuation.Composition)
	}
	assertCompositionSums(t, *valuation.Composition)
}

// Independent rounding of slices used to change an unchanged top-ten item's
// displayed percentage on expansion. Detail shares must be the sole allocation
// and every grouped view must be an additive partition of that allocation.
func TestAccountCompositionShareAllocationStableAcrossViewsAndTies(t *testing.T) {
	account := compositionFixture()
	instruments := map[domain.InstrumentID]domain.Instrument{}
	amounts := []int{181, 178, 178, 171, 142, 137, 122, 97, 57, 41, 28, 12}
	expected := []int{1347, 1325, 1324, 1272, 1057, 1019, 908, 722, 424, 305, 208, 89}
	for i, amount := range amounts {
		component := compositionComponent(fmt.Sprintf("item-%02d", i), fmt.Sprint(amount))
		account.Components = append(account.Components, component)
		kind := domain.InstrumentStock
		if i%2 == 0 {
			kind = domain.InstrumentETF
		}
		instruments[*component.InstrumentID] = domain.Instrument{ID: *component.InstrumentID, Type: kind}
	}
	result, err := projectAccountComposition(account, instruments, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalExact != "1344" {
		t.Fatal(result)
	}
	// The UI regression consumes this exact wire payload, so its displayed
	// percentages are tied to the backend projection rather than a mock drift.
	fixtureJSON, err := os.ReadFile("../../testdata/current/account-composition-shares.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture wire.AccountCompositionDTO
	if err := json.Unmarshal(fixtureJSON, &fixture); err != nil {
		t.Fatal(err)
	}
	if actual := wire.FromAccountComposition(&result); !reflect.DeepEqual(actual, &fixture) {
		t.Fatalf("backend wire projection differs from shared UI fixture: %+v", actual)
	}
	assertCompositionSums(t, result)
	for i, detail := range result.Details {
		if detail.ShareBPS != expected[i] {
			t.Fatalf("detail %s share=%d, want %d", detail.Key, detail.ShareBPS, expected[i])
		}
		if i < 10 && !reflect.DeepEqual(result.DetailSlices[i], detail) {
			t.Fatalf("collapsed item %s differs from original: %+v vs %+v", detail.Key, result.DetailSlices[i], detail)
		}
	}
	remainder := result.DetailSlices[10]
	if remainder.BaseAmountExact != "40" || remainder.ShareBPS != 297 {
		t.Fatalf("remainder=%+v", remainder)
	}
	detailsByRef := map[string]domain.AccountCompositionItem{}
	for _, detail := range result.Details {
		detailsByRef[detail.Key] = detail
	}
	for _, group := range result.Types {
		share := 0
		for _, ref := range group.DetailRefs {
			share += detailsByRef[ref].ShareBPS
		}
		if group.ShareBPS != share {
			t.Fatalf("type %s share=%d, original sum=%d", group.TypeKey, group.ShareBPS, share)
		}
	}
	// Equal amounts and fractional remainders are resolved by stable detail key,
	// independent of input order and whether one is visible or in the remainder.
	for i, j := 0, len(account.Components)-1; i < j; i, j = i+1, j-1 {
		account.Components[i], account.Components[j] = account.Components[j], account.Components[i]
	}
	reversed, err := projectAccountComposition(account, instruments, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, reversed) {
		t.Fatal("tie allocation depends on input order")
	}
}
