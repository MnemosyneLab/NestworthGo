package application

import (
	"context"
	"fmt"
	"testing"
	"time"

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

func TestOverviewAssetCompositionRetainsEveryExactPath(t *testing.T) {
	var first, second domain.InstitutionID = "institution-1", "institution-2"
	names := map[string]string{first.String(): "Same name", second.String(): "Same name"}
	c := overviewAssetComposition{paths: make(map[overviewAssetPathKey]domain.OverviewAssetPath)}
	add := func(id, class, amount string, institution *domain.InstitutionID) {
		c.add(decimal.RequireFromString(amount), class, domain.Account{ID: domain.AccountID(id), Name: "Repeated account", InstitutionID: institution}, names)
	}
	for i := range 8 {
		add(fmt.Sprintf("account-%02d", i), "cash", "1000", &first)
	}
	add("mixed", "cash", "900.123456", &first)
	add("mixed", "stock", "2000.25", &first)
	add("small", "cash", "0.002", &second)
	add("small-bond", "bond", "0.001", &second)
	add("unassigned", "", "0.003", nil)
	add("negative", "cash", "-0.0045", &first)
	add("zero", "cash", "0", nil)
	result := c.finish()
	if result.Positive.String() != "10900.379456" || result.Negative.String() != "-0.0045" ||
		result.Positive.Add(result.Negative).String() != "10900.374956" || result.ZeroCount != 1 || len(result.Paths) != 13 {
		t.Fatalf("allocation = %+v", result)
	}
	if result.Paths[1].AccountID != "account-00" || result.Paths[8].AccountID != "account-07" || result.Paths[9].AccountID != "mixed" || result.Paths[9].Amount.String() != "900.123456" {
		t.Fatalf("unexpected ranked paths: %+v", result.Paths)
	}
	assertOverviewCompositionGraph(t, result)
}

func TestOverviewAssetCompositionUsesCurrentInputsWithTheirOwnReferenceDates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, owner, now := overviewFixture(t)
	b, err := s.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	broker := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
	costs := map[domain.HoldingID]string{}
	holding := func(name, currency, source string) domain.Instrument {
		t.Helper()
		i, err := s.CreateInstrument(ctx, InstrumentInput{Name: name, Type: "stock", QuoteCurrency: currency, QuoteSource: source})
		if err != nil {
			t.Fatal(err)
		}
		h, err := s.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "1"})
		if err != nil {
			t.Fatal(err)
		}
		costs[h.ID] = "1"
		return i
	}
	manual := holding("Synthetic manual", "CNY", "manual")
	quoted := holding("Synthetic latest stock", "USD", "agent")
	missing := holding("Synthetic gap", "CNY", "manual")
	if _, err := s.AppendManualInstrumentQuote(ctx, manual.ID, "0.00006", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CaptureSyntheticOriginWithCosts(ctx, "Asia/Singapore", costs); err != nil {
		t.Fatal(err)
	}

	*now = time.Date(2026, 8, 4, 17, 0, 0, 0, time.UTC)
	for _, fixture := range []struct {
		day         int
		price, rate string
	}{
		{4, "5.1234", ""},
		{3, "3", "7.1"},
		{2, "", "7"},
	} {
		at := time.Date(2026, 8, fixture.day, 12, 0, 0, 0, time.UTC)
		date := at.Format("2006-01-02")
		if fixture.price != "" {
			q := domain.InstrumentQuote{
				ID: domain.NewInstrumentQuoteID(), InstrumentID: quoted.ID, UnitPrice: mustUnitPrice(t, fixture.price), Currency: "USD",
				SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", ObservationKind: string(InstrumentObservationClose),
				EffectiveDate: date, ValueEffectiveAt: at, QuotedAt: at, CreatedAt: *now, FetchedAt: *now,
				PriceBasis: "agent_unit_nav_v1", SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "date_label", Revision: 1,
			}
			if err := s.repository.AppendInstrumentQuote(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		if fixture.rate != "" {
			rate, err := domain.ParseFxRate(fixture.rate)
			if err != nil {
				t.Fatal(err)
			}
			q := domain.FXQuote{
				ID: domain.NewFXQuoteID(), HouseholdID: b.Household.ID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate,
				SourceKind: domain.QuoteSourceAgent, SourceKey: "agent", ObservationKind: string(FXObservationDailyReference),
				EffectiveDate: date, ValueEffectiveAt: at, QuotedAt: at, CreatedAt: *now, FetchedAt: *now,
				SourcePolicyVersion: "agent_supplied_v1", TimestampBasis: "date_label", Revision: 1,
			}
			if err := s.repository.AppendFXQuote(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
	}

	read := func(wantTotal string, wantMissing int) {
		t.Helper()
		got, err := s.Overview(ctx, domain.AccountFilter{})
		if err != nil {
			t.Fatal(err)
		}
		q := got.DisplayValuation.NetWorth.Quality
		if got.ValuationReadDate != "2026-08-05" || q.OldestMarketDate == nil || *q.OldestMarketDate != "2026-08-01" ||
			q.NewestMarketDate == nil || *q.NewestMarketDate != "2026-08-04" ||
			q.OldestCarriedMarketDate == nil || *q.OldestCarriedMarketDate != "2026-08-03" ||
			q.CarriedCount != 1 || q.StaleCount != 1 || q.UnpricedCount != wantMissing || got.Complete != (wantMissing == 0) {
			t.Fatalf("read date %s, complete %t, quality %+v", got.ValuationReadDate, got.Complete, q)
		}
		c := got.AssetComposition
		if got.Assets.String() != wantTotal || c.MissingCount != wantMissing || len(c.Paths) != 1 ||
			c.Paths[0].AccountID != broker.Account.ID.String() || c.Paths[0].Amount.String() != wantTotal {
			t.Fatalf("current assets = %s, allocation = %+v", got.Assets, c)
		}
		assertOverviewCompositionGraph(t, c)
	}
	// 5.1234 USD × 7.1 plus the older manual input, with the gap excluded.
	read("36.3762", 1)
	if _, err := s.AppendManualInstrumentQuote(ctx, missing.ID, "1.20", "2026-08-04", false); err != nil {
		t.Fatal(err)
	}
	read("37.5762", 0)

	// The Overview trend publishes a separate current point and a closed
	// snapshot for the prior day. The latter still has incomplete coverage.
	trend, err := s.NetWorthTrend(ctx, domain.TrendRange("2026-08-04:2026-08-05"))
	if err != nil {
		t.Fatal(err)
	}
	if len(trend.Points) != 2 {
		t.Fatalf("points = %+v", trend.Points)
	}
	closed, current := trend.Points[0], trend.Points[1]
	if closed.Current || closed.LocalDate != "2026-08-04" || closed.Complete ||
		!current.Current || current.LocalDate != "2026-08-05" || !current.Complete ||
		current.DisplayValuation.Assets.Total == nil || current.DisplayValuation.Assets.Total.Amount != "37.5762" {
		t.Fatalf("closed and current valuation points = %+v", trend.Points)
	}

	history, err := s.HistoricalOverviewSeries(ctx, "2026-08-03", "2026-08-04")
	if err != nil {
		t.Fatal(err)
	}
	if history.Overview.LastClosedDate != "2026-08-04" || history.Points[0].Assets != nil ||
		history.Points[0].DisplayValuation.Assets.Quality.UnpricedCount == 0 {
		t.Fatalf("the closed Aug 3 day must retain its missing coverage: %+v", history.Points[0])
	}
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
		switch node.Kind {
		case "total", "assetClass", "institution", "account":
		default:
			t.Fatalf("fabricated chart node: %+v", node)
		}
		if _, duplicate := byID[node.ID]; duplicate || !node.Amount.IsPositive() {
			t.Fatalf("duplicate or nonpositive chart node: %+v", node)
		}
		byID[node.ID] = node
		children[node.ParentID] = children[node.ParentID].Add(node.Amount)
	}
	accountNodes := 0
	for _, node := range c.Nodes {
		if node.Kind == "account" {
			accountNodes++
		}
	}
	if accountNodes != len(c.Paths) {
		t.Fatalf("got %d account nodes for %d paths", accountNodes, len(c.Paths))
	}
	for _, path := range c.Paths {
		classID := "assets/" + path.AssetClass
		institutionID := classID + "/institution:" + path.InstitutionID
		account := byID[institutionID+"/account:"+path.AccountID]
		institution, class := byID[institutionID], byID[classID]
		if account.Kind != "account" || account.ParentID != institution.ID || account.Key != path.AccountID || !account.Amount.Equal(path.Amount) ||
			institution.Kind != "institution" || institution.ParentID != class.ID || institution.Key != path.InstitutionID ||
			class.Kind != "assetClass" || class.ParentID != "assets" || class.Key != path.AssetClass {
			t.Fatalf("path %+v has wrong chart ancestry: %+v, %+v, %+v", path, class, institution, account)
		}
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
