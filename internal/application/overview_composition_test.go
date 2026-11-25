package application

import (
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestOverviewAssetCompositionUsesJointAllocations(t *testing.T) {
	t.Parallel()
	s, ctx, bootstrap, _ := newOnboardedService(t, "overview-composition", nil)
	broker, err := s.CreateInstitution(ctx, "Broker A", domain.InstitutionBrokerage)
	if err != nil {
		t.Fatal(err)
	}
	bank, err := s.CreateInstitution(ctx, "Bank B", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	account := func(name, accountType, role, tracking, currency, institution, amount string, included bool) domain.AccountID {
		t.Helper()
		record, err := s.CreateAccount(ctx, AccountInput{
			Name: name, AccountType: accountType, BalanceSheetRole: role, TrackingMode: tracking,
			DefaultCurrency: currency, InstitutionID: institution, InitialAmount: amount,
			OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}, IncludeInNetWorth: included,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return record.Account.ID
	}
	mixed := account("Savings", "brokerage", "asset", "holdings", "CNY", broker.ID.String(), "", true)
	cash := account("Savings", "bank_account", "asset", "balance", "CNY", bank.ID.String(), "20", true)
	zero := account("Zero", "bank_account", "asset", "balance", "CNY", "", "0", true)
	unvalued := account("Unvalued cash", "bank_account", "asset", "balance", "USD", bank.ID.String(), "20", true)
	account("Outside net worth", "bank_account", "asset", "balance", "CNY", "", "800", false)
	account("Credit card", "credit_card", "liability", "balance", "CNY", "", "5", true)
	account("Foreign credit card", "credit_card", "liability", "balance", "GBP", "", "5", true)
	archived := account("Archived", "bank_account", "asset", "balance", "CNY", "", "900", true)
	if err := s.ArchiveAccount(ctx, archived, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAccountCashValue(ctx, mixed, "40", "CNY", "2026-08-01"); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ name, price string }{{"Stock one", "20"}, {"Stock two", "40"}, {"Missing quote", ""}} {
		instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: fixture.name, Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateHolding(ctx, HoldingInput{AccountID: mixed.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); err != nil {
			t.Fatal(err)
		}
		if fixture.price != "" {
			if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, fixture.price, "2026-08-01", false); err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	c := result.AssetComposition
	if result.Complete || result.Currency != "CNY" || result.Assets.String() != "120" || result.Liabilities.String() != "5" ||
		c.MissingCount != 2 || c.ZeroCount != 1 || c.Positive.String() != "120" || !c.Negative.IsZero() {
		t.Fatalf("Overview assets = %s, liabilities = %s, allocation = %+v", result.Assets, result.Liabilities, c)
	}
	want := []struct{ class, institutionID, institution, account, amount string }{
		{"stock", broker.ID.String(), "Broker A", mixed.String(), "60"},
		{"cash", broker.ID.String(), "Broker A", mixed.String(), "40"},
		{"cash", bank.ID.String(), "Bank B", cash.String(), "20"},
	}
	if len(c.Paths) != len(want) {
		t.Fatalf("paths = %+v", c.Paths)
	}
	for i, path := range c.Paths {
		got := struct{ class, institutionID, institution, account, amount string }{
			path.AssetClass, path.InstitutionID, path.InstitutionName, path.AccountID, path.Amount.String(),
		}
		if got != want[i] || path.AccountName != "Savings" {
			t.Fatalf("path %d = %+v, want %+v", i, path, want[i])
		}
	}
	assertOverviewCompositionGraph(t, c)
	for _, tt := range []struct {
		ids                   []domain.AccountID
		assets                string
		missing, zeros, paths int
	}{
		{[]domain.AccountID{mixed}, "100", 1, 0, 2},
		{[]domain.AccountID{unvalued}, "0", 1, 0, 0},
		{[]domain.AccountID{zero}, "0", 0, 1, 0},
		{[]domain.AccountID{archived}, "0", 0, 0, 0},
	} {
		filtered, err := s.Overview(ctx, domain.AccountFilter{AccountIDs: tt.ids})
		if err != nil {
			t.Fatal(err)
		}
		c := filtered.AssetComposition
		if filtered.Assets.String() != tt.assets || c.MissingCount != tt.missing || c.ZeroCount != tt.zeros || len(c.Paths) != tt.paths {
			t.Fatalf("filtered overview assets = %s, allocation = %+v for %+v", filtered.Assets, c, tt)
		}
		assertOverviewCompositionGraph(t, c)
	}
}

func TestOverviewAssetCompositionGroupsWithoutLosingExactValues(t *testing.T) {
	var first, second domain.InstitutionID = "institution-1", "institution-2"
	names := map[string]string{first.String(): "Same name", second.String(): "Same name"}
	c := overviewAssetComposition{paths: make(map[overviewAssetPathKey]domain.OverviewAssetPath)}
	add := func(id, class, amount string, institution *domain.InstitutionID) {
		c.add(decimal.RequireFromString(amount), class, domain.Account{ID: domain.AccountID(id), Name: "Repeated account", InstitutionID: institution}, names)
	}
	for i := range overviewNamedAssetPaths {
		add(fmt.Sprintf("account-%02d", i), "cash", "1", &first)
	}
	add("within-institution", "cash", "0.001", &first)
	add("unnamed-institution", "cash", "0.002", &second)
	add("unclassified", "", "0.003", nil)
	add("negative", "cash", "-0.0045", &first)
	add("zero", "cash", "0", nil)
	result := c.finish()
	if result.Positive.String() != "8.006" || result.Negative.String() != "-0.0045" ||
		result.Positive.Add(result.Negative).String() != "8.0015" || result.ZeroCount != 1 || len(result.Paths) != 11 {
		t.Fatalf("allocation = %+v", result)
	}
	if result.Paths[0].AccountID != "account-00" || result.Paths[7].AccountID != "account-07" {
		t.Fatalf("equal amounts weren't stably ranked: %+v", result.Paths[:8])
	}
	want := map[string]string{
		"assets/cash/institution:institution-1/other-accounts": "1.001",
		"assets/cash/other-institutions":                       "0.002",
		"assets//institution:/account:unclassified":            "0.003",
	}
	for _, node := range result.Nodes {
		if amount, ok := want[node.ID]; ok && node.Amount.String() == amount {
			delete(want, node.ID)
		}
	}
	if len(want) > 0 {
		t.Fatalf("missing chart groups: %v", want)
	}
	assertOverviewCompositionGraph(t, result)
}

func assertOverviewCompositionGraph(t *testing.T, c domain.OverviewAssetComposition) {
	t.Helper()
	if len(c.Paths) == 0 {
		if len(c.Nodes) != 0 || !c.Positive.IsZero() {
			t.Fatalf("no paths with chart data: %+v", c)
		}
		return
	}
	children := map[string]decimal.Decimal{}
	byID := map[string]domain.OverviewAssetNode{}
	for _, node := range c.Nodes {
		if _, duplicate := byID[node.ID]; duplicate || !node.Amount.IsPositive() {
			t.Fatalf("duplicate or nonpositive chart node: %+v", node)
		}
		byID[node.ID] = node
		children[node.ParentID] = children[node.ParentID].Add(node.Amount)
	}
	if !children[""].Equal(c.Positive) {
		t.Fatalf("root amount = %s, want %s", children[""], c.Positive)
	}
	for parent, total := range children {
		if parent != "" && !byID[parent].Amount.Equal(total) {
			t.Fatalf("node %s amount = %s, children total = %s", parent, byID[parent].Amount, total)
		}
	}
}
