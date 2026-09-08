package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type Result struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Notes  string `json:"notes"`
}

type Report struct {
	Commit  string            `json:"commit"`
	DBPath  string            `json:"db_path"`
	IDs     map[string]string `json:"ids"`
	Results []Result          `json:"results"`
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func money(amount, currency string) domain.Money {
	return must(domain.ParseMoney(amount, domain.CurrencyCode(currency)))
}

func qty(value string) domain.Quantity {
	return must(domain.ParseQuantity(value))
}

func add(r *Report, name, status, notes string) {
	r.Results = append(r.Results, Result{Name: name, Status: status, Notes: notes})
	fmt.Printf("[%s] %s — %s\n", status, name, notes)
}

func countQuery(db *sql.DB, query string) int {
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		panic(err)
	}
	return n
}

func main() {
	base := "/workspace/nestworth-analytics-qa"
	dbPath := os.Getenv("NESTWORTH_DATABASE_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(base, "data", "nestworth.db")
	}
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	database := must(sqlite.Open(dbPath))
	defer database.Close()
	repo := sqlite.NewRepository(database)
	svc := application.NewService(repo)
	ctx := context.Background()

	report := &Report{
		Commit: "4cee77220fde42a60131282ef2f637c5750cb96b",
		DBPath: dbPath,
		IDs:    map[string]string{},
	}

	must0 := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	// 1) Onboard WITHOUT timezone so HistoryOrigin is not created empty.
	must0(svc.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "Analytics QA",
		BaseCurrency:  "AUD",
		MemberNames:   []string{"Weichen"},
		Timezone:      "",
	}))
	bootstrap := must(svc.Bootstrap(ctx))
	hh := bootstrap.Household.ID
	m1 := bootstrap.Members[0].ID
	report.IDs["household"] = string(hh)
	report.IDs["member"] = string(m1)
	add(report, "onboarding", "PASS", fmt.Sprintf("household=%s member=%s base=AUD tz=(empty, no history yet)", hh, m1))

	// 2) Create accounts before StartHistory so origin captures cash components.
	cash := must(svc.CreateAccount(ctx, application.AccountInput{
		Name: "AUD Cash", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "AUD", InitialAmount: "10000",
		IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{m1},
	}))
	broker := must(svc.CreateAccount(ctx, application.AccountInput{
		Name: "US Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "USD",
		IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{m1},
	}))
	report.IDs["acct_cash"] = string(cash.Account.ID)
	report.IDs["acct_broker"] = string(broker.Account.ID)
	add(report, "accounts", "PASS", fmt.Sprintf("cash=%s broker=%s", cash.Account.ID, broker.Account.ID))

	// 3) Create AAPL instrument (manual quote source).
	aapl := must(svc.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Apple Inc", Type: "stock", QuoteCurrency: "USD", Symbol: "AAPL",
		MarketCode: "XNAS", CountryCode: "US", QuoteSource: "manual",
	}))
	report.IDs["inst_aapl"] = string(aapl.ID)
	if aapl.QuoteSource != domain.QuoteSourceManual {
		add(report, "instrument_aapl", "FAIL", fmt.Sprintf("quoteSource=%s want=manual", aapl.QuoteSource))
	} else {
		add(report, "instrument_aapl", "PASS", fmt.Sprintf("id=%s quoteSource=manual", aapl.ID))
	}

	// 4) One current FX + AAPL quote before StartHistory (FX useful even without holdings).
	nowISO := time.Now().UTC().Format(time.RFC3339)
	_ = must(svc.AppendManualFXQuote(ctx, "USD", "AUD", "1.5000", nowISO))
	_ = must(svc.AppendManualInstrumentQuote(ctx, aapl.ID, "180.00", nowISO, false))
	add(report, "current_quotes", "PASS", fmt.Sprintf("fx USD/AUD=1.5000 aapl=180.00 at %s", nowISO))

	// 4b) Prefer manual FX for USD/AUD BEFORE StartHistory so origin captures it.
	// Without this, historical valuation implicitly prefers QuoteSourceProvider and
	// ignores seeded manual FX quotes → USD cash items stay incomplete forever.
	pref := must(svc.SetFXPreference(ctx, "USD", "AUD", "manual"))
	add(report, "fx_preference_manual", "PASS", fmt.Sprintf("USD/AUD source=%s", pref.SourceKind))

	// 5) StartHistory AFTER accounts exist → non-empty history_origin_components.
	origin := must(svc.StartHistory(ctx, "Asia/Singapore"))
	compCount := countQuery(database.SQL, `SELECT COUNT(*) FROM history_origin_components`)
	report.IDs["history_origin_id"] = string(origin.ID)
	if compCount <= 0 {
		add(report, "start_history", "FAIL", fmt.Sprintf("history_origin_components=%d want >0", compCount))
	} else {
		add(report, "start_history", "PASS", fmt.Sprintf("tz=Asia/Singapore components=%d origin=%s", compCount, origin.ID))
	}
	report.IDs["history_origin_components"] = fmt.Sprintf("%d", compCount)

	// 6) SQL backdate history_origins.started_at / created_at ~45 days ago UTC.
	originAt := time.Now().UTC().Add(-45 * 24 * time.Hour).Truncate(time.Second)
	originISO := originAt.Format(time.RFC3339Nano)
	if _, err := database.SQL.Exec(`UPDATE history_origins SET started_at = ?, created_at = ?`, originISO, originISO); err != nil {
		panic(err)
	}
	report.IDs["history_origin"] = originAt.Format(time.RFC3339)
	add(report, "backdate_origin", "PASS", fmt.Sprintf("started_at=%s (~45d ago)", originISO))

	day := func(offset int) time.Time {
		return originAt.Add(time.Duration(offset) * 24 * time.Hour).UTC()
	}
	iso := func(t time.Time) string { return t.Format(time.RFC3339) }

	// 7) Seed daily FX + AAPL quotes for d=0..44 with QuotedAt = originAt+d days.
	quoteCount := 0
	for d := 0; d <= 44; d++ {
		t := day(d)
		ts := iso(t)
		_ = must(svc.AppendManualFXQuote(ctx, "USD", "AUD", fmt.Sprintf("%.4f", 1.50+float64(d)*0.0002), ts))
		_ = must(svc.AppendManualInstrumentQuote(ctx, aapl.ID, fmt.Sprintf("%.2f", 180.0+float64(d)*0.5), ts, false))
		quoteCount++
	}
	add(report, "daily_quotes", "PASS", fmt.Sprintf("days=0..44 count=%d fx~1.50..%.4f aapl~180..%.2f", quoteCount, 1.50+44*0.0002, 180.0+44*0.5))

	// 8) Activities with EffectiveAt = originAt + offsets (after backdated origin).
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: broker.Account.ID,
			Amount: money("20000", "USD"), Reason: domain.ReasonContribution, EffectiveAt: day(1),
		})
		return err
	}())
	add(report, "act_day1_contrib_usd", "PASS", "brokerage +20000 USD contribution")

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: cash.Account.ID,
			Amount: money("5000", "AUD"), Reason: domain.ReasonContribution, EffectiveAt: day(2),
		})
		return err
	}())
	add(report, "act_day2_contrib_aud", "PASS", "cash +5000 AUD contribution")

	must0(func() error {
		fee := money("5", "USD")
		_, err := svc.RecordChange(ctx, domain.TradeInput{
			HouseholdID: hh, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID,
			InstrumentID: aapl.ID, Quantity: qty("10"), Gross: money("1800", "USD"), Fee: &fee, EffectiveAt: day(5),
		})
		return err
	}())
	add(report, "act_day5_buy_aapl", "PASS", "buy 10 AAPL ~1800 USD fee 5")

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: cash.Account.ID,
			Amount: money("3000", "AUD"), Reason: domain.ReasonIncome, EffectiveAt: day(10),
		})
		return err
	}())
	add(report, "act_day10_income", "PASS", "cash +3000 AUD income/salary")

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyRemovedInput{
			HouseholdID: hh, AccountID: cash.Account.ID,
			Amount: money("200", "AUD"), Reason: domain.ReasonExpense, EffectiveAt: day(15),
		})
		return err
	}())
	add(report, "act_day15_spending", "PASS", "cash -200 AUD expense via MoneyRemovedInput")

	// day20: AAPL ~180+20*0.5=190 → 5*190=950
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.TradeInput{
			HouseholdID: hh, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID,
			InstrumentID: aapl.ID, Quantity: qty("5"), Gross: money("950", "USD"), EffectiveAt: day(20),
		})
		return err
	}())
	add(report, "act_day20_buy_aapl", "PASS", "buy 5 AAPL ~950 USD")

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: broker.Account.ID,
			Amount: money("1000", "USD"), Reason: domain.ReasonContribution, EffectiveAt: day(30),
		})
		return err
	}())
	add(report, "act_day30_contrib_usd", "PASS", "brokerage +1000 USD contribution")

	// 8b) Backdate instrument/holding created_at so historical replay includes them.
	// Trades stamp CreatedAt=now; replay skips entities created after the day cutoff
	// unless they were present at origin (AAPL holding was not).
	instrumentAt := originAt.Format(time.RFC3339Nano)
	holdingAt := day(5).Format(time.RFC3339Nano)
	if _, err := database.SQL.Exec(`UPDATE instruments SET created_at = ? WHERE id = ?`, instrumentAt, string(aapl.ID)); err != nil {
		panic(err)
	}
	if _, err := database.SQL.Exec(`UPDATE holdings SET created_at = ?, updated_at = ?`, holdingAt, holdingAt); err != nil {
		panic(err)
	}
	add(report, "backdate_entities", "PASS", fmt.Sprintf("instrument.created_at=%s holding.created_at=%s", instrumentAt, holdingAt))

	// 9) Rebuild snapshots in chunks ≤31 days from origin date through yesterday.
	startDate := originAt.Format("2006-01-02")
	endDate := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	cursor, _ := time.Parse("2006-01-02", startDate)
	endT, _ := time.Parse("2006-01-02", endDate)
	totalSnap := 0
	var rebuildErr error
	for !cursor.After(endT) {
		// Inclusive span of 31 calendar days: cursor .. cursor+30d
		chunkEnd := cursor.Add(30 * 24 * time.Hour)
		if chunkEnd.After(endT) {
			chunkEnd = endT
		}
		n, err := svc.RebuildHistoricalSnapshots(ctx, cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02"))
		if err != nil {
			rebuildErr = err
			break
		}
		totalSnap += n
		cursor = chunkEnd.Add(24 * time.Hour)
	}
	report.IDs["snapshot_rebuild_count"] = fmt.Sprintf("%d", totalSnap)
	if rebuildErr != nil {
		add(report, "rebuild_snapshots", "FAIL", rebuildErr.Error())
	} else {
		add(report, "rebuild_snapshots", "PASS", fmt.Sprintf("rebuilt=%d range=%s..%s chunks<=31d", totalSnap, startDate, endDate))
		fmt.Printf("rebuilt snapshot count: %d\n", totalSnap)
	}

	// CompleteDailySnapshotRange for last closed day when API is available.
	if rebuildErr == nil {
		if err := svc.CompleteDailySnapshotRange(ctx, hh, endDate); err != nil {
			add(report, "complete_daily_range", "FAIL", err.Error())
		} else {
			add(report, "complete_daily_range", "PASS", fmt.Sprintf("targetDate=%s", endDate))
		}
	}

	// 10) Verify non-empty origin components + snapshot items with assets.
	compCount = countQuery(database.SQL, `SELECT COUNT(*) FROM history_origin_components`)
	var usableSnaps int
	_ = database.SQL.QueryRow(`
		SELECT COUNT(*) FROM daily_valuation_snapshots
		WHERE component_count > 0 AND assets_amount IS NOT NULL AND assets_amount != '' AND assets_amount != '0'
	`).Scan(&usableSnaps)
	itemCount := countQuery(database.SQL, `SELECT COUNT(*) FROM daily_valuation_snapshot_items`)
	var maxComp int
	_ = database.SQL.QueryRow(`SELECT COALESCE(MAX(component_count), 0) FROM daily_valuation_snapshots`).Scan(&maxComp)
	var sampleAssets string
	_ = database.SQL.QueryRow(`
		SELECT COALESCE(assets_amount, '') FROM daily_valuation_snapshots
		WHERE component_count > 0 AND assets_amount IS NOT NULL AND assets_amount != '' AND assets_amount != '0'
		ORDER BY local_date DESC LIMIT 1
	`).Scan(&sampleAssets)

	report.IDs["verify_origin_components"] = fmt.Sprintf("%d", compCount)
	report.IDs["verify_usable_snapshots"] = fmt.Sprintf("%d", usableSnaps)
	report.IDs["verify_snapshot_items"] = fmt.Sprintf("%d", itemCount)
	report.IDs["verify_max_component_count"] = fmt.Sprintf("%d", maxComp)
	report.IDs["verify_sample_assets"] = sampleAssets

	verifyOK := compCount > 0 && usableSnaps > 0 && itemCount > 0
	notes := fmt.Sprintf("origin_components=%d usable_snapshots=%d items=%d max_component_count=%d sample_assets=%s",
		compCount, usableSnaps, itemCount, maxComp, sampleAssets)
	if verifyOK {
		add(report, "verify_snapshots", "PASS", notes)
	} else {
		add(report, "verify_snapshots", "FAIL", notes)
	}

	// 11) Align settings currency AUD so UI Base matches household.
	settingsPath := filepath.Join(base, "data", "settings.json")
	settings := map[string]any{
		"schema_version":     1,
		"appearance":         "system",
		"accent":             "nestworth",
		"language":           "en",
		"timezone":           "system",
		"week_start":         "monday",
		"date_format":        "iso",
		"time_format":        "24h",
		"currency":           "AUD",
		"decimal_separator":  ".",
		"grouping_separator": ",",
		"decimal_places":     2,
		"window_width":       1280,
		"window_height":      720,
		"fx_provider":        "frankfurter",
		"quote_cache_ttl":    "12h",
	}
	sb, _ := json.MarshalIndent(settings, "", "  ")
	must0(os.WriteFile(settingsPath, append(sb, '\n'), 0o644))
	add(report, "settings_aud", "PASS", fmt.Sprintf("wrote %s currency=AUD", settingsPath))

	outDir := filepath.Join(base, "seed")
	_ = os.MkdirAll(outDir, 0o755)
	out := filepath.Join(outDir, "seed-results.json")
	b, _ := json.MarshalIndent(report, "", "  ")
	must0(os.WriteFile(out, b, 0o644))
	fmt.Printf("wrote %s\n", out)

	failed := false
	for _, r := range report.Results {
		if r.Status == "FAIL" {
			failed = true
			fmt.Printf("FAIL: %s — %s\n", r.Name, r.Notes)
		}
	}
	if failed {
		fmt.Println("OVERALL: FAIL")
		os.Exit(2)
	}
	fmt.Println("OVERALL: PASS")
}
