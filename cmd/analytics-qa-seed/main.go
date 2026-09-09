// Command analytics-qa-seed builds the Insights Round-3 “mini family” fixture
// (analytics-linux-qa-v3) for desktop/probe acceptance of the analytics test
// plan §6 / FC-01–06.
//
// Env (unchanged from v2):
//
//	NESTWORTH_QA_SCENARIO     complete | missing-price | missing-fx | missing-both
//	NESTWORTH_QA_ANCHOR       RFC3339 UTC (default 2026-07-26T00:00:00Z)
//	NESTWORTH_QA_RESET        set to 1 to replace an existing QA database
//	NESTWORTH_QA_OUTPUT_DIR   artifact root (default /workspace/nestworth-analytics-qa)
//	NESTWORTH_DATABASE_PATH   sqlite path (default $OUTPUT_DIR/data/nestworth.db)
//
// FC-07 loan is intentionally omitted; see README.md.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	FixtureVersion string            `json:"fixture_version"`
	Scenario       string            `json:"scenario"`
	Anchor         string            `json:"anchor"`
	Commit         string            `json:"commit"`
	Dirty          bool              `json:"dirty"`
	DBPath         string            `json:"db_path"`
	OutputDir      string            `json:"output_dir"`
	IDs            map[string]string `json:"ids"`
	Results        []Result          `json:"results"`
}

const (
	fixtureVersion  = "analytics-linux-qa-v3"
	defaultQAAnchor = "2026-07-26T00:00:00Z"

	missingQuoteDay     = 22
	flatZeroDay         = 36
	negStart, negEnd    = 25, 28
	sellDay             = 32
	dividendDay         = 33
	feeDay              = 34
	transferOutDay      = 3
	sgdFundDay          = 4
	interestDay         = 7
	qqqSameDay          = 8
	qqqHoldDay          = 12
	es3BuyDay           = 18
	transferBackDay     = 21
	quoteHorizon        = 44
	expectedQuoteDays   = quoteHorizon + 1
	expectedGapQuotes   = expectedQuoteDays - (missingQuoteDay + 1)
	expectedAccountN    = 3
	expectedInstrumentN = 3
	expectedHoldingN    = 3
)

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func gitIdentity() (string, bool) {
	commit := strings.TrimSpace(os.Getenv("NESTWORTH_QA_COMMIT"))
	if commit == "" {
		if output, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
			commit = strings.TrimSpace(string(output))
		}
	}
	if commit == "" {
		commit = "unknown"
	}
	dirtyOutput, err := exec.Command("git", "status", "--porcelain").Output()
	return commit, err == nil && len(strings.TrimSpace(string(dirtyOutput))) > 0
}

func parseAnchor(value string) time.Time {
	anchor, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(fmt.Errorf("NESTWORTH_QA_ANCHOR must be RFC3339: %w", err))
	}
	return anchor.UTC().Truncate(time.Second)
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

func moneyPtr(amount, currency string) *domain.Money {
	value := money(amount, currency)
	return &value
}

func qty(value string) domain.Quantity {
	return must(domain.ParseQuantity(value))
}

func add(r *Report, name, status, notes string) {
	r.Results = append(r.Results, Result{Name: name, Status: status, Notes: notes})
	fmt.Printf("[%s] %s — %s\n", status, name, notes)
}

func countQuery(db *sql.DB, query string, args ...any) int {
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		panic(err)
	}
	return n
}

func pathWithin(base, target string) bool {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(baseAbs), filepath.Clean(targetAbs))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func usdAudRate(d int) float64 {
	if d == 35 || d == 36 {
		return 1.5070
	}
	return 1.50 + float64(d)*0.0002
}

func sgdAudRate(d int) float64 {
	if d == 35 || d == 36 {
		return 1.1570
	}
	return 1.1500 + float64(d)*0.0002
}

func aaplPrice(d int) float64 {
	switch {
	case d >= 25 && d <= 28:
		base24 := 180.0 + 24*0.5            // 192
		factor := 1.0 - 0.025*float64(d-24) // 0.975, 0.95, 0.925, 0.90
		return base24 * factor
	case d == 29:
		return 185.00
	case d == 30:
		return 195.00
	case d == 31:
		return 200.00
	case d == 32:
		return 205.00 // sell day — clear gain vs ~183.7 cost
	case d == 33:
		return 206.00
	case d == 34:
		return 207.00
	case d == 35, d == 36:
		return 207.00 // flat day 36 vs 35 for true-zero-ish (no activity on 36)
	default:
		return 180.0 + float64(d)*0.5
	}
}

func qqqPrice(d int) float64 {
	if d == 35 || d == 36 {
		return 420.00
	}
	return 400.0 + float64(d)*0.25
}

func es3Price(d int) float64 {
	if d == 35 || d == 36 {
		return 3.40
	}
	return 3.20 + float64(d)*0.005
}

func holdingIDFrom(preview domain.ChangePreview) domain.HoldingID {
	if preview.Activity.TradeDetail != nil {
		return preview.Activity.TradeDetail.HoldingID
	}
	return ""
}

func main() {
	base := envOr("NESTWORTH_QA_OUTPUT_DIR", "/workspace/nestworth-analytics-qa")
	dbPath := strings.TrimSpace(os.Getenv("NESTWORTH_DATABASE_PATH"))
	if dbPath == "" {
		dbPath = filepath.Join(base, "data", "nestworth.db")
	}
	scenario := envOr("NESTWORTH_QA_SCENARIO", "complete")
	if scenario != "complete" && scenario != "missing-price" && scenario != "missing-fx" && scenario != "missing-both" {
		panic(fmt.Errorf("unsupported NESTWORTH_QA_SCENARIO %q", scenario))
	}
	anchor := parseAnchor(envOr("NESTWORTH_QA_ANCHOR", defaultQAAnchor))
	reset := os.Getenv("NESTWORTH_QA_RESET") == "1"
	if _, err := os.Stat(dbPath); err == nil && !reset {
		panic(fmt.Errorf("refusing to overwrite existing QA database %s; set NESTWORTH_QA_RESET=1 for an explicit QA reset", dbPath))
	} else if err != nil && !os.IsNotExist(err) {
		panic(fmt.Errorf("inspect QA database %s: %w", dbPath, err))
	}
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	if reset {
		if !pathWithin(base, dbPath) {
			panic(fmt.Errorf("refusing to reset database outside NESTWORTH_QA_OUTPUT_DIR: %s", dbPath))
		}
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	}

	database := must(sqlite.Open(dbPath))
	defer database.Close()
	repo := sqlite.NewRepository(database)
	svc := application.NewService(repo)
	ctx := context.Background()

	commit, dirty := gitIdentity()
	report := &Report{FixtureVersion: fixtureVersion, Scenario: scenario, Anchor: anchor.Format(time.RFC3339), Commit: commit, Dirty: dirty, DBPath: dbPath, OutputDir: base, IDs: map[string]string{}}

	must0 := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	sgt := must(time.LoadLocation("Asia/Singapore"))
	localDate := func(t time.Time) string {
		return t.In(sgt).Format("2006-01-02")
	}
	originAt := anchor
	day := func(offset int) time.Time {
		return originAt.Add(time.Duration(offset) * 24 * time.Hour).UTC()
	}
	iso := func(t time.Time) string { return t.Format(time.RFC3339) }

	missingPrice := scenario == "missing-price" || scenario == "missing-both"
	missingFX := scenario == "missing-fx" || scenario == "missing-both"

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
	report.IDs["fixture_version"] = fixtureVersion
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
	sgBroker := must(svc.CreateAccount(ctx, application.AccountInput{
		Name: "SG Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "SGD",
		IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{m1},
	}))
	report.IDs["acct_cash"] = string(cash.Account.ID)
	report.IDs["acct_broker"] = string(broker.Account.ID)
	report.IDs["acct_sg"] = string(sgBroker.Account.ID)
	add(report, "accounts", "PASS", fmt.Sprintf("cash=%s us_broker=%s sg_broker=%s", cash.Account.ID, broker.Account.ID, sgBroker.Account.ID))

	// 3) Instruments (manual quote source): AAPL stock, QQQ US ETF, ES3 SGD ETF.
	aapl := must(svc.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Apple Inc", Type: "stock", QuoteCurrency: "USD", Symbol: "AAPL",
		MarketCode: "XNAS", CountryCode: "US", QuoteSource: "manual",
	}))
	qqq := must(svc.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Invesco QQQ Trust", Type: "etf", QuoteCurrency: "USD", Symbol: "QQQ",
		MarketCode: "XNAS", CountryCode: "US", QuoteSource: "manual",
	}))
	es3 := must(svc.CreateInstrument(ctx, application.InstrumentInput{
		Name: "SPDR STI ETF", Type: "etf", QuoteCurrency: "SGD", Symbol: "ES3",
		MarketCode: "XSES", CountryCode: "SG", QuoteSource: "manual",
	}))
	report.IDs["inst_aapl"] = string(aapl.ID)
	report.IDs["inst_qqq"] = string(qqq.ID)
	report.IDs["inst_es3"] = string(es3.ID)
	if aapl.QuoteSource != domain.QuoteSourceManual || qqq.QuoteSource != domain.QuoteSourceManual || es3.QuoteSource != domain.QuoteSourceManual {
		add(report, "instruments", "FAIL", fmt.Sprintf("quoteSource aapl=%s qqq=%s es3=%s want=manual", aapl.QuoteSource, qqq.QuoteSource, es3.QuoteSource))
	} else {
		add(report, "instruments", "PASS", fmt.Sprintf("aapl=%s qqq=%s es3=%s quoteSource=manual", aapl.ID, qqq.ID, es3.ID))
	}

	// 4) One current FX + instrument quote before StartHistory.
	nowISO := iso(originAt)
	if !missingFX {
		_ = must(svc.AppendManualFXQuote(ctx, "USD", "AUD", "1.5000", nowISO))
		_ = must(svc.AppendManualFXQuote(ctx, "SGD", "AUD", "1.1500", nowISO))
	}
	if !missingPrice {
		_ = must(svc.AppendManualInstrumentQuote(ctx, aapl.ID, "180.00", nowISO, false))
		_ = must(svc.AppendManualInstrumentQuote(ctx, qqq.ID, "400.00", nowISO, false))
		_ = must(svc.AppendManualInstrumentQuote(ctx, es3.ID, "3.20", nowISO, false))
	}
	add(report, "current_quotes", "PASS", fmt.Sprintf("scenario=%s fx=%s prices=%s at %s", scenario, map[bool]string{true: "omitted", false: "USD/AUD=1.5000 SGD/AUD=1.1500"}[missingFX], map[bool]string{true: "omitted", false: "AAPL=180 QQQ=400 ES3=3.20"}[missingPrice], nowISO))

	// 4b) Prefer manual FX for USD/AUD and SGD/AUD BEFORE StartHistory.
	usdPref := must(svc.SetFXPreference(ctx, "USD", "AUD", "manual"))
	sgdPref := must(svc.SetFXPreference(ctx, "SGD", "AUD", "manual"))
	manualPreferenceCount := countQuery(database.SQL, `
		SELECT COUNT(*) FROM fx_preferences
		WHERE source_kind = 'manual'
		  AND ((currency_a = 'AUD' AND currency_b = 'USD') OR (currency_a = 'AUD' AND currency_b = 'SGD'))
	`)
	if manualPreferenceCount != 2 || usdPref.SourceKind != domain.QuoteSourceManual || sgdPref.SourceKind != domain.QuoteSourceManual {
		add(report, "fx_preference_manual", "FAIL", fmt.Sprintf("USD/AUD source=%s SGD/AUD source=%s rows=%d want=2", usdPref.SourceKind, sgdPref.SourceKind, manualPreferenceCount))
	} else {
		add(report, "fx_preference_manual", "PASS", fmt.Sprintf("USD/AUD+SGD/AUD source=manual rows=%d", manualPreferenceCount))
	}

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

	// 6) SQL backdate history_origins.started_at / created_at to the fixed QA anchor.
	originISO := originAt.Format(time.RFC3339Nano)
	if _, err := database.SQL.Exec(`UPDATE history_origins SET started_at = ?, created_at = ?`, originISO, originISO); err != nil {
		panic(err)
	}
	report.IDs["history_origin"] = originAt.Format(time.RFC3339)
	add(report, "backdate_origin", "PASS", fmt.Sprintf("started_at=%s anchor=%s", originISO, report.Anchor))

	// 7) Daily FX + instrument quotes for d=0..44. Completeness matrix omits
	// price and/or FX through missingQuoteDay (inclusive) per scenario.
	usdFXCount, sgdFXCount := 0, 0
	aaplQuoteCount, qqqQuoteCount, es3QuoteCount := 0, 0, 0
	for d := 0; d <= quoteHorizon; d++ {
		ts := iso(day(d))
		if !(missingFX && d <= missingQuoteDay) {
			_ = must(svc.AppendManualFXQuote(ctx, "USD", "AUD", fmt.Sprintf("%.4f", usdAudRate(d)), ts))
			_ = must(svc.AppendManualFXQuote(ctx, "SGD", "AUD", fmt.Sprintf("%.4f", sgdAudRate(d)), ts))
			usdFXCount++
			sgdFXCount++
		}
		if !(missingPrice && d <= missingQuoteDay) {
			_ = must(svc.AppendManualInstrumentQuote(ctx, aapl.ID, fmt.Sprintf("%.2f", aaplPrice(d)), ts, false))
			_ = must(svc.AppendManualInstrumentQuote(ctx, qqq.ID, fmt.Sprintf("%.2f", qqqPrice(d)), ts, false))
			_ = must(svc.AppendManualInstrumentQuote(ctx, es3.ID, fmt.Sprintf("%.2f", es3Price(d)), ts, false))
			aaplQuoteCount++
			qqqQuoteCount++
			es3QuoteCount++
		}
	}
	missingLocal := localDate(day(missingQuoteDay))
	report.IDs["missing_quote_day_offset"] = fmt.Sprintf("%d", missingQuoteDay)
	report.IDs["missing_quote_local_date"] = missingLocal
	report.IDs["negative_price_window"] = fmt.Sprintf("day_%d..%d local=%s..%s", negStart, negEnd, localDate(day(negStart)), localDate(day(negEnd)))
	report.IDs["flat_zero_day_offset"] = fmt.Sprintf("%d", flatZeroDay)
	report.IDs["flat_zero_local_date"] = localDate(day(flatZeroDay))
	expectedPairQuotes, expectedInstrumentQuotes := expectedQuoteDays, expectedQuoteDays
	if missingFX {
		expectedPairQuotes = expectedGapQuotes
	}
	if missingPrice {
		expectedInstrumentQuotes = expectedGapQuotes
	}
	quoteStatus := "PASS"
	if usdFXCount != expectedPairQuotes || sgdFXCount != expectedPairQuotes ||
		aaplQuoteCount != expectedInstrumentQuotes || qqqQuoteCount != expectedInstrumentQuotes || es3QuoteCount != expectedInstrumentQuotes {
		quoteStatus = "FAIL"
	}
	add(report, "daily_quotes", quoteStatus, fmt.Sprintf(
		"scenario=%s usd_fx=%d/%d sgd_fx=%d/%d aapl=%d/%d qqq=%d/%d es3=%d/%d missing_through_day=%d local=%s; neg_drop day_%d..%d; flat day_%d=%s",
		scenario, usdFXCount, expectedPairQuotes, sgdFXCount, expectedPairQuotes,
		aaplQuoteCount, expectedInstrumentQuotes, qqqQuoteCount, expectedInstrumentQuotes, es3QuoteCount, expectedInstrumentQuotes,
		missingQuoteDay, missingLocal, negStart, negEnd, flatZeroDay, localDate(day(flatZeroDay)),
	))

	// 8) Activities with EffectiveAt = originAt + offsets (after backdated origin).
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: broker.Account.ID,
			Amount: money("20000", "USD"), Reason: domain.ReasonContribution, EffectiveAt: day(1),
		})
		return err
	}())
	add(report, "act_day1_contrib_usd", "PASS", "US Brokerage +20000 USD contribution")

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: cash.Account.ID,
			Amount: money("5000", "AUD"), Reason: domain.ReasonContribution, EffectiveAt: day(2),
		})
		return err
	}())
	add(report, "act_day2_contrib_aud", "PASS", "AUD Cash +5000 AUD contribution")

	// FC-01: same-currency internal transfer into the US Brokerage AUD sleeve.
	xferOutLocal := localDate(day(transferOutDay))
	report.IDs["transfer_out_day_offset"] = fmt.Sprintf("%d", transferOutDay)
	report.IDs["transfer_out_local_date"] = xferOutLocal
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.CashTransferInput{
			HouseholdID: hh, FromAccountID: cash.Account.ID, ToAccountID: broker.Account.ID,
			Sent: money("1000", "AUD"), Received: money("1000", "AUD"), EffectiveAt: day(transferOutDay),
		})
		return err
	}())
	add(report, "act_day3_transfer_aud_to_us", "PASS", fmt.Sprintf("FC-01 AUD Cash → US Brokerage AUD sleeve 1000 AUD local=%s", xferOutLocal))

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: sgBroker.Account.ID,
			Amount: money("8000", "SGD"), Reason: domain.ReasonContribution, EffectiveAt: day(sgdFundDay),
		})
		return err
	}())
	add(report, "act_day4_contrib_sgd", "PASS", "SG Brokerage +8000 SGD contribution")

	buy5 := must(svc.RecordChange(ctx, domain.TradeInput{
		HouseholdID: hh, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID,
		InstrumentID: aapl.ID, Quantity: qty("10"), Gross: money("1800", "USD"),
		Fee: moneyPtr("5", "USD"), EffectiveAt: day(5),
	}))
	aaplHoldingID := holdingIDFrom(buy5)
	report.IDs["holding_aapl"] = string(aaplHoldingID)
	add(report, "act_day5_buy_aapl", "PASS", fmt.Sprintf("buy 10 AAPL ~1800 USD fee 5 holding=%s", aaplHoldingID))

	// FC-05: interest (Dividend & Interest, not Dietz capital) vs day-10 salary.
	interestLocal := localDate(day(interestDay))
	report.IDs["interest_day_offset"] = fmt.Sprintf("%d", interestDay)
	report.IDs["interest_local_date"] = interestLocal
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: cash.Account.ID,
			Amount: money("500", "AUD"), Reason: domain.ReasonInterest, EffectiveAt: day(interestDay),
		})
		return err
	}())
	add(report, "act_day7_interest", "PASS", fmt.Sprintf("FC-05 AUD Cash +500 AUD interest local=%s", interestLocal))

	// FC-03: same-day QQQ buy 1 @ 400 / sell 1 @ 410 (ending qty 0, realized +10 USD).
	qqqSameLocal := localDate(day(qqqSameDay))
	report.IDs["qqq_sameday_offset"] = fmt.Sprintf("%d", qqqSameDay)
	report.IDs["qqq_sameday_local_date"] = qqqSameLocal
	qqqBuy := must(svc.RecordChange(ctx, domain.TradeInput{
		HouseholdID: hh, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID,
		InstrumentID: qqq.ID, Quantity: qty("1"), Gross: money("400", "USD"),
		EffectiveAt: day(qqqSameDay),
	}))
	qqqHoldingID := holdingIDFrom(qqqBuy)
	report.IDs["holding_qqq"] = string(qqqHoldingID)
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.TradeInput{
			HouseholdID: hh, Side: domain.TradeSell, SettlementAccountID: broker.Account.ID,
			HoldingID: qqqHoldingID, InstrumentID: qqq.ID, Quantity: qty("1"),
			Gross: money("410", "USD"), EffectiveAt: day(qqqSameDay).Add(2 * time.Hour),
		})
		return err
	}())
	add(report, "act_day8_qqq_sameday", "PASS", fmt.Sprintf("FC-03 buy 1 QQQ @400 then sell 1 @410 same local=%s holding=%s", qqqSameLocal, qqqHoldingID))

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: cash.Account.ID,
			Amount: money("3000", "AUD"), Reason: domain.ReasonIncome, EffectiveAt: day(10),
		})
		return err
	}())
	add(report, "act_day10_income", "PASS", "FC-05 AUD Cash +3000 AUD income/salary")

	// FC-02: buy-and-hold QQQ from US Brokerage USD cash (instrument scope external).
	qqqHoldLocal := localDate(day(qqqHoldDay))
	report.IDs["qqq_hold_day_offset"] = fmt.Sprintf("%d", qqqHoldDay)
	report.IDs["qqq_hold_local_date"] = qqqHoldLocal
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.TradeInput{
			HouseholdID: hh, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID,
			HoldingID: qqqHoldingID, InstrumentID: qqq.ID, Quantity: qty("2"),
			Gross: money("810", "USD"), EffectiveAt: day(qqqHoldDay),
		})
		return err
	}())
	add(report, "act_day12_buy_qqq", "PASS", fmt.Sprintf("FC-02 buy 2 QQQ ~810 USD local=%s holding=%s", qqqHoldLocal, qqqHoldingID))

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyRemovedInput{
			HouseholdID: hh, AccountID: cash.Account.ID,
			Amount: money("200", "AUD"), Reason: domain.ReasonExpense, EffectiveAt: day(15),
		})
		return err
	}())
	add(report, "act_day15_spending", "PASS", "AUD Cash -200 AUD expense via MoneyRemovedInput")

	es3Local := localDate(day(es3BuyDay))
	report.IDs["es3_buy_day_offset"] = fmt.Sprintf("%d", es3BuyDay)
	report.IDs["es3_buy_local_date"] = es3Local
	es3Buy := must(svc.RecordChange(ctx, domain.TradeInput{
		HouseholdID: hh, Side: domain.TradeBuy, SettlementAccountID: sgBroker.Account.ID,
		InstrumentID: es3.ID, Quantity: qty("10"), Gross: money("35.00", "SGD"),
		EffectiveAt: day(es3BuyDay),
	}))
	es3HoldingID := holdingIDFrom(es3Buy)
	report.IDs["holding_es3"] = string(es3HoldingID)
	add(report, "act_day18_buy_es3", "PASS", fmt.Sprintf("FC-04 buy 10 ES3 ~35 SGD local=%s holding=%s", es3Local, es3HoldingID))

	// day20: AAPL ~180+20*0.5=190 → 5*190=950
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.TradeInput{
			HouseholdID: hh, Side: domain.TradeBuy, SettlementAccountID: broker.Account.ID,
			HoldingID: aaplHoldingID, InstrumentID: aapl.ID, Quantity: qty("5"), Gross: money("950", "USD"), EffectiveAt: day(20),
		})
		return err
	}())
	add(report, "act_day20_buy_aapl", "PASS", "buy 5 AAPL ~950 USD")

	xferBackLocal := localDate(day(transferBackDay))
	report.IDs["transfer_back_day_offset"] = fmt.Sprintf("%d", transferBackDay)
	report.IDs["transfer_back_local_date"] = xferBackLocal
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.CashTransferInput{
			HouseholdID: hh, FromAccountID: broker.Account.ID, ToAccountID: cash.Account.ID,
			Sent: money("200", "AUD"), Received: money("200", "AUD"), EffectiveAt: day(transferBackDay),
		})
		return err
	}())
	add(report, "act_day21_transfer_us_to_aud", "PASS", fmt.Sprintf("FC-01 reverse US Brokerage AUD sleeve → AUD Cash 200 AUD local=%s", xferBackLocal))

	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyAddedInput{
			HouseholdID: hh, AccountID: broker.Account.ID,
			Amount: money("1000", "USD"), Reason: domain.ReasonContribution, EffectiveAt: day(30),
		})
		return err
	}())
	add(report, "act_day30_contrib_usd", "PASS", "US Brokerage +1000 USD contribution")

	// Partial sell for Realized Gain (CO-02 / FC-03 companion): day 32 sell 3 AAPL.
	// Cost ≈ (1800+5+950)/15 ≈ 183.67; sell @ 205 → gross 615.
	sellLocal := localDate(day(sellDay))
	report.IDs["sell_day_offset"] = fmt.Sprintf("%d", sellDay)
	report.IDs["sell_local_date"] = sellLocal
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.TradeInput{
			HouseholdID: hh, Side: domain.TradeSell, SettlementAccountID: broker.Account.ID,
			HoldingID: aaplHoldingID, InstrumentID: aapl.ID, Quantity: qty("3"),
			Gross: money("615", "USD"), EffectiveAt: day(sellDay),
		})
		return err
	}())
	add(report, "act_day32_sell_aapl", "PASS", fmt.Sprintf("sell 3 AAPL gross=615 USD (~205/sh vs ~183.7 cost) local=%s holding=%s", sellLocal, aaplHoldingID))

	// FC-06 / CO-03 / CAT-05 dividend via CashDividendInput.
	divLocal := localDate(day(dividendDay))
	report.IDs["dividend_day_offset"] = fmt.Sprintf("%d", dividendDay)
	report.IDs["dividend_local_date"] = divLocal
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.CashDividendInput{
			HouseholdID: hh, HoldingID: aaplHoldingID,
			Amount: money("12.50", "USD"), EffectiveAt: day(dividendDay),
		})
		return err
	}())
	add(report, "act_day33_dividend", "PASS", fmt.Sprintf("FC-06 CashDividendInput +12.50 USD on AAPL holding local=%s", divLocal))

	feeLocal := localDate(day(feeDay))
	report.IDs["fee_day_offset"] = fmt.Sprintf("%d", feeDay)
	report.IDs["fee_local_date"] = feeLocal
	must0(func() error {
		_, err := svc.RecordChange(ctx, domain.MoneyRemovedInput{
			HouseholdID: hh, AccountID: cash.Account.ID,
			Amount: money("15", "AUD"), Reason: domain.ReasonFee, EffectiveAt: day(feeDay),
		})
		return err
	}())
	add(report, "act_day34_bank_fee", "PASS", fmt.Sprintf("AUD Cash -15 AUD fee local=%s", feeLocal))

	add(report, "true_zero_day_fixture", "PASS", fmt.Sprintf(
		"day_%d local=%s flat AAPL=207 QQQ=420 ES3=3.40 USD/AUD=1.5070 SGD/AUD=1.1570, no activity — probe verifies exact zero",
		flatZeroDay, localDate(day(flatZeroDay)),
	))

	if aaplHoldingID == "" || qqqHoldingID == "" || es3HoldingID == "" {
		add(report, "holdings_present", "FAIL", fmt.Sprintf("aapl=%q qqq=%q es3=%q", aaplHoldingID, qqqHoldingID, es3HoldingID))
	} else {
		add(report, "holdings_present", "PASS", fmt.Sprintf("aapl=%s qqq=%s es3=%s", aaplHoldingID, qqqHoldingID, es3HoldingID))
	}

	// 8b) Backdate instrument/holding created_at so historical replay includes them.
	instrumentAt := originAt.Format(time.RFC3339Nano)
	for _, id := range []domain.InstrumentID{aapl.ID, qqq.ID, es3.ID} {
		if _, err := database.SQL.Exec(`UPDATE instruments SET created_at = ? WHERE id = ?`, instrumentAt, string(id)); err != nil {
			panic(err)
		}
	}
	backdateHolding := func(id domain.HoldingID, at time.Time) {
		ts := at.Format(time.RFC3339Nano)
		if _, err := database.SQL.Exec(`UPDATE holdings SET created_at = ?, updated_at = ? WHERE id = ?`, ts, ts, string(id)); err != nil {
			panic(err)
		}
	}
	backdateHolding(aaplHoldingID, day(5))
	backdateHolding(qqqHoldingID, day(qqqSameDay))
	backdateHolding(es3HoldingID, day(es3BuyDay))
	instrumentRows := countQuery(database.SQL, `SELECT COUNT(*) FROM instruments WHERE created_at = ?`, instrumentAt)
	holdingRows := countQuery(database.SQL, `SELECT COUNT(*) FROM holdings`)
	if instrumentRows != expectedInstrumentN || holdingRows != expectedHoldingN {
		add(report, "backdate_entities", "FAIL", fmt.Sprintf("instruments=%d want=%d holdings=%d want=%d origin_created_at=%s", instrumentRows, expectedInstrumentN, holdingRows, expectedHoldingN, instrumentAt))
	} else {
		add(report, "backdate_entities", "PASS", fmt.Sprintf("instruments=%d holdings=%d instrument.created_at=%s", instrumentRows, holdingRows, instrumentAt))
	}

	// 9) Rebuild snapshots in chunks ≤31 days from origin LOCAL date through the
	// fixed fixture end date. A missing-price/FX scenario omits the dependency
	// through day 22; the normal rebuild must produce the incomplete snapshot.
	startDate := localDate(originAt)
	endDate := localDate(day(quoteHorizon))
	cursor, _ := time.Parse("2006-01-02", startDate)
	endT, _ := time.Parse("2006-01-02", endDate)
	totalSnap := 0
	var rebuildErr error
	for !cursor.After(endT) {
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

	if rebuildErr == nil {
		if err := svc.CompleteDailySnapshotRange(ctx, hh, endDate); err != nil {
			add(report, "complete_daily_range", "FAIL", err.Error())
		} else {
			add(report, "complete_daily_range", "PASS", fmt.Sprintf("targetDate=%s", endDate))
		}

		flatQuery := domain.AnalysisQuery{
			Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: localDate(day(flatZeroDay - 1)), To: localDate(day(flatZeroDay)),
			Valuation: domain.ValuationBase, IncludeCash: true, Basis: domain.ReturnBasisInvestment,
		}
		flatResult, flatErr := svc.Analyze(ctx, flatQuery)
		flatFound := false
		flatOK := false
		if flatErr == nil {
			for _, daily := range flatResult.DailyReturns {
				if string(daily.Date) != localDate(day(flatZeroDay)) {
					continue
				}
				flatFound = true
				flatOK = daily.Status == domain.CompletenessOK && daily.Amount != nil && daily.Amount.Amount().IsZero() && daily.Rate != nil && daily.Rate.IsZero()
			}
		}
		if flatErr != nil || !flatFound || !flatOK {
			add(report, "true_zero_day", "FAIL", fmt.Sprintf("date=%s found=%v exact_zero=%v error=%v", localDate(day(flatZeroDay)), flatFound, flatOK, flatErr))
		} else {
			add(report, "true_zero_day", "PASS", fmt.Sprintf("date=%s status=ok amount=0 rate=0 with flat AAPL/QQQ/ES3 and USD+SGD FX", localDate(day(flatZeroDay))))
		}

		var gapSnapID string
		err := database.SQL.QueryRow(`
			SELECT id, complete FROM daily_valuation_snapshots s
			WHERE local_date = ? AND revision = (
				SELECT MAX(revision) FROM daily_valuation_snapshots s2
				WHERE s2.household_id = s.household_id AND s2.local_date = s.local_date
			)
		`, missingLocal).Scan(&gapSnapID, new(int))
		if err != nil {
			add(report, "missing_quote_gap", "FAIL", err.Error())
		} else {
			var missingN int
			_ = database.SQL.QueryRow(`SELECT COUNT(*) FROM daily_valuation_snapshot_items WHERE snapshot_id = ? AND complete = 0`, gapSnapID).Scan(&missingN)
			var gapComplete int
			_ = database.SQL.QueryRow(`SELECT complete FROM daily_valuation_snapshots WHERE id = ?`, gapSnapID).Scan(&gapComplete)
			expectedIncomplete := missingPrice || missingFX
			if expectedIncomplete && (gapComplete != 0 || missingN == 0) {
				add(report, "missing_quote_gap", "FAIL", fmt.Sprintf("scenario=%s local=%s snap=%s complete=%d missing_items=%d want incomplete", scenario, missingLocal, gapSnapID, gapComplete, missingN))
			} else if !expectedIncomplete && (gapComplete == 0 || missingN != 0) {
				add(report, "missing_quote_gap", "FAIL", fmt.Sprintf("scenario=%s local=%s snap=%s complete=%d missing_items=%d want complete", scenario, missingLocal, gapSnapID, gapComplete, missingN))
			} else {
				add(report, "missing_quote_gap", "PASS", fmt.Sprintf("scenario=%s local=%s snap=%s complete=%d missing_items=%d", scenario, missingLocal, gapSnapID, gapComplete, missingN))
			}
			report.IDs["missing_quote_gap_complete"] = fmt.Sprintf("%d", gapComplete)
			report.IDs["missing_quote_gap_snap"] = gapSnapID
		}
	}

	// 10) Verify non-empty origin components + snapshot items with assets.
	latestSnap := `
		SELECT * FROM daily_valuation_snapshots s
		WHERE revision = (
			SELECT MAX(s2.revision) FROM daily_valuation_snapshots s2
			WHERE s2.household_id = s.household_id AND s2.local_date = s.local_date
		)`
	compCount = countQuery(database.SQL, `SELECT COUNT(*) FROM history_origin_components`)
	var usableSnaps int
	_ = database.SQL.QueryRow(`
		SELECT COUNT(*) FROM (` + latestSnap + `) latest
		WHERE component_count > 0 AND assets_amount IS NOT NULL AND assets_amount != '' AND assets_amount != '0'
	`).Scan(&usableSnaps)
	var itemCount int
	_ = database.SQL.QueryRow(`
		SELECT COUNT(*) FROM daily_valuation_snapshot_items i
		JOIN (` + latestSnap + `) latest ON latest.id = i.snapshot_id
	`).Scan(&itemCount)
	var maxComp int
	_ = database.SQL.QueryRow(`SELECT COALESCE(MAX(component_count), 0) FROM (` + latestSnap + `)`).Scan(&maxComp)
	var sampleAssets string
	_ = database.SQL.QueryRow(`
		SELECT COALESCE(assets_amount, '') FROM (` + latestSnap + `) latest
		WHERE component_count > 0 AND assets_amount IS NOT NULL AND assets_amount != '' AND assets_amount != '0'
		ORDER BY local_date DESC LIMIT 1
	`).Scan(&sampleAssets)

	var incompleteDays int
	_ = database.SQL.QueryRow(`
		SELECT COUNT(*) FROM (` + latestSnap + `) latest
		WHERE complete = 0 OR EXISTS (
			SELECT 1 FROM daily_valuation_snapshot_items i
			WHERE i.snapshot_id = latest.id AND i.complete = 0
		)
	`).Scan(&incompleteDays)
	var missingSnapNote string
	_ = database.SQL.QueryRow(`
		SELECT COALESCE(local_date,'') || ' complete=' || COALESCE(CAST(complete AS TEXT),'?') ||
			' comps=' || COALESCE(CAST(component_count AS TEXT),'?') ||
			' missing=' || COALESCE(CAST(missing_count AS TEXT),'?') ||
			' rev=' || COALESCE(CAST(revision AS TEXT),'?')
		FROM (`+latestSnap+`) latest WHERE local_date = ? LIMIT 1
	`, missingLocal).Scan(&missingSnapNote)

	var activityKinds string
	_ = database.SQL.QueryRow(`
		SELECT GROUP_CONCAT(kind || ':' || cnt, ', ') FROM (
			SELECT kind, COUNT(*) AS cnt FROM activities GROUP BY kind ORDER BY kind
		)
	`).Scan(&activityKinds)

	accountN := countQuery(database.SQL, `SELECT COUNT(*) FROM accounts`)
	instrumentN := countQuery(database.SQL, `SELECT COUNT(*) FROM instruments`)
	holdingN := countQuery(database.SQL, `SELECT COUNT(*) FROM holdings`)
	transferN := countQuery(database.SQL, `SELECT COUNT(*) FROM activities WHERE kind = 'cash_transfer'`)
	interestN := countQuery(database.SQL, `SELECT COUNT(*) FROM activities WHERE kind = 'cash_in' AND reason = 'interest'`)
	incomeN := countQuery(database.SQL, `SELECT COUNT(*) FROM activities WHERE kind = 'cash_in' AND reason = 'income'`)
	buyN := countQuery(database.SQL, `SELECT COUNT(*) FROM activities WHERE kind = 'buy'`)
	sellN := countQuery(database.SQL, `SELECT COUNT(*) FROM activities WHERE kind = 'sell'`)
	sameDayN := countQuery(database.SQL, `SELECT COUNT(*) FROM activities WHERE kind IN ('buy','sell') AND effective_local_date = ?`, qqqSameLocal)

	report.IDs["verify_origin_components"] = fmt.Sprintf("%d", compCount)
	report.IDs["verify_usable_snapshots"] = fmt.Sprintf("%d", usableSnaps)
	report.IDs["verify_snapshot_items"] = fmt.Sprintf("%d", itemCount)
	report.IDs["verify_max_component_count"] = fmt.Sprintf("%d", maxComp)
	report.IDs["verify_sample_assets"] = sampleAssets
	report.IDs["verify_incomplete_days"] = fmt.Sprintf("%d", incompleteDays)
	report.IDs["verify_missing_quote_snap"] = missingSnapNote
	report.IDs["verify_activity_kinds"] = activityKinds
	report.IDs["verify_accounts"] = fmt.Sprintf("%d", accountN)
	report.IDs["verify_instruments"] = fmt.Sprintf("%d", instrumentN)
	report.IDs["verify_holdings"] = fmt.Sprintf("%d", holdingN)
	report.IDs["verify_cash_transfers"] = fmt.Sprintf("%d", transferN)
	report.IDs["verify_interest"] = fmt.Sprintf("%d", interestN)
	report.IDs["verify_salary"] = fmt.Sprintf("%d", incomeN)
	report.IDs["verify_buys"] = fmt.Sprintf("%d", buyN)
	report.IDs["verify_sells"] = fmt.Sprintf("%d", sellN)
	report.IDs["verify_qqq_sameday_trades"] = fmt.Sprintf("%d", sameDayN)

	verifyOK := compCount > 0 && usableSnaps > 0 && itemCount > 0
	notes := fmt.Sprintf("origin_components=%d usable_snapshots=%d items=%d max_component_count=%d sample_assets=%s incomplete_days=%d missing_quote_snap=[%s] activities=[%s]",
		compCount, usableSnaps, itemCount, maxComp, sampleAssets, incompleteDays, missingSnapNote, activityKinds)
	if verifyOK {
		add(report, "verify_snapshots", "PASS", notes)
	} else {
		add(report, "verify_snapshots", "FAIL", notes)
	}

	familyOK := accountN == expectedAccountN && instrumentN == expectedInstrumentN && holdingN == expectedHoldingN &&
		transferN == 2 && interestN == 1 && incomeN == 1 && buyN >= 5 && sellN >= 2 && sameDayN == 2
	familyNotes := fmt.Sprintf("accounts=%d instruments=%d holdings=%d transfers=%d interest=%d salary=%d buys=%d sells=%d qqq_sameday=%d",
		accountN, instrumentN, holdingN, transferN, interestN, incomeN, buyN, sellN, sameDayN)
	if familyOK {
		add(report, "verify_v3_family", "PASS", familyNotes)
	} else {
		add(report, "verify_v3_family", "FAIL", familyNotes)
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
