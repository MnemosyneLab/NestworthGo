package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type reviewFullPipelineFixture struct {
	service     *Service
	db          *sqlite.DB
	now         *time.Time
	householdID domain.HouseholdID
	dbPath      string
	start       time.Time
	instruments []domain.Instrument
}

type reviewPerfConfig struct {
	// Accounts counts brokerage accounts available to holdings and mixed activity.
	// CashActivities adds one cash-only account so absolute observations cannot
	// overwrite mixed-activity funding.
	Accounts, Instruments, Holdings, Rules, HistoryDays, Quotes, CashActivities, MixedActivities int
	MixedKind                                                                                    string
}

func (c reviewPerfConfig) validate() error {
	if c.Accounts < 1 || c.Instruments < 1 || c.Rules < 1 || c.HistoryDays < 1 || c.Quotes < 0 || c.CashActivities < 0 || c.MixedActivities < 0 || c.Holdings < 0 || c.Holdings > c.Accounts*c.Instruments || (c.MixedActivities > 0 && (c.MixedKind == "" || c.MixedKind == "mixed" || c.MixedKind == "transfer") && c.Accounts < 2) {
		return fmt.Errorf("invalid synthetic review fixture: %+v", c)
	}
	switch c.MixedKind {
	case "", "mixed", "trade", "transfer", "cost":
	default:
		return fmt.Errorf("unknown synthetic activity kind %q", c.MixedKind)
	}
	return nil
}

func newReviewPerfFixture(t *testing.T, config reviewPerfConfig) reviewFullPipelineFixture {
	t.Helper()
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
	historyDays := config.HistoryDays
	if config.Holdings == 0 {
		config.Holdings = config.Instruments
	}
	ctx := context.Background()
	start := time.Date(2025, 10, 17, 12, 0, 0, 0, time.UTC)
	now := start
	dbPath := t.TempDir() + "/review-full-pipeline.db"
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := NewService(sqlite.NewRepository(db))
	service.setClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic review pipeline", BaseCurrency: "CNY", MemberNames: []string{"Synthetic owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accounts := make([]domain.AccountID, config.Accounts)
	for index := range accounts {
		account, err := service.CreateAccount(ctx, AccountInput{Name: fmt.Sprintf("Synthetic broker %03d", index), AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		accounts[index] = account.Account.ID
	}
	cashAccountID := accounts[0]
	if config.CashActivities > 0 {
		account, err := service.CreateAccount(ctx, AccountInput{Name: "Synthetic cash observations", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		cashAccountID = account.Account.ID
	}
	currencies := []string{"CNY", "USD", "EUR", "JPY", "SGD"}
	types := []domain.InstrumentType{domain.InstrumentStock, domain.InstrumentMutualFund, domain.InstrumentCrypto, domain.InstrumentBond}
	instruments := make([]domain.Instrument, config.Instruments)
	costs := make(map[domain.HoldingID]string, config.Instruments)
	for index := range instruments {
		currency := currencies[index%len(currencies)]
		kind := types[index%len(types)]
		instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: fmt.Sprintf("Synthetic holding %03d", index), Type: string(kind), QuoteCurrency: currency, QuoteSource: "manual"})
		if err != nil {
			t.Fatalf("create instrument %d: %v", index, err)
		}
		instruments[index] = instrument
		if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "100", start.Format(time.RFC3339), false); err != nil {
			t.Fatalf("add synthetic unit-cost seed quote %d: %v", index, err)
		}
	}
	holdings := make([]domain.Holding, config.Holdings)
	for index := range holdings {
		instrument := instruments[index%len(instruments)]
		holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: accounts[(index%len(instruments)+index/len(instruments))%len(accounts)].String(), InstrumentID: instrument.ID.String(), Quantity: "2"})
		if err != nil {
			t.Fatal(err)
		}
		holdings[index] = holding
		costs[holding.ID] = "10"
	}
	for _, currency := range currencies[1:] {
		if _, err := service.SetFXPreference(ctx, currency, "CNY", "manual"); err != nil {
			t.Fatalf("set synthetic FX preference for %s: %v", currency, err)
		}
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
		t.Fatal(err)
	}

	if err := seedReviewFullPipelineFacts(ctx, db, bootstrap.Household.ID, instruments, cashAccountID, start, historyDays, config.Quotes, config.CashActivities); err != nil {
		t.Fatalf("seed synthetic source facts: %v", err)
	}
	now = start.AddDate(0, 0, historyDays)
	if config.MixedActivities > 0 {
		seedReviewPerfMixedActivities(t, service, bootstrap.Household.ID, accounts, holdings, instruments, start, config)
	}
	for index := 0; index < config.Rules; index++ {
		spec := syntheticHistoricalReviewSpec()
		spec.Schedule.Timezone = []string{"UTC", "America/Los_Angeles", "Asia/Shanghai", "Europe/London"}[index%4]
		spec.Target = domain.ReviewRuleTarget{Kind: "all_current"}
		metric := index % 3
		if metric == 0 {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: "<", Value: "100000", Currency: "CNY", FXMode: "convert_on_day"}}
		} else if metric == 1 {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "-100", LookbackDays: 7}}
		} else {
			spec.Predicate.All[1] = domain.ReviewPredicateNode{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "0", Currency: "CNY", FXMode: "convert_on_day"}}
		}
		if index%4 == 1 {
			spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3}
		} else if index%4 == 2 {
			spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 7, MinHits: 3}
		}
		if _, err := service.CreateReviewRuleV2(ctx, SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: fmt.Sprintf("Synthetic rule %02d", index), Enabled: true, Severity: "warning", Spec: spec}); err != nil {
			t.Fatalf("create rule %d: %v", index, err)
		}
	}
	return reviewFullPipelineFixture{service: service, db: db, now: &now, householdID: bootstrap.Household.ID, dbPath: dbPath, start: start, instruments: instruments}
}

func appendReviewPerfDailyMarketData(ctx context.Context, fixture reviewFullPipelineFixture, on time.Time) error {
	tx, err := fixture.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	quoteStatement, err := tx.PrepareContext(ctx, `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,value_effective_at,binding_revision,price_basis,timestamp_basis,revision) VALUES(?,?,?,?,'agent','agent',?,?, 'close',?,?,0,?,?,1)`)
	if err != nil {
		return err
	}
	defer quoteStatement.Close()
	date := on.UTC().Format(time.DateOnly)
	dayIndex := int(on.UTC().Sub(fixture.start).Hours() / 24)
	when := time.Date(on.Year(), on.Month(), on.Day(), 17, 0, 0, 0, time.UTC)
	createdAt := when.Add(time.Hour)
	for index, instrument := range fixture.instruments {
		priceBasis, timestampBasis := "agent_raw_close_v1", "date_label"
		if instrument.Type == domain.InstrumentMutualFund || instrument.Type == domain.InstrumentBankInvestmentProduct {
			priceBasis = "agent_unit_nav_v1"
		} else if instrument.Type == domain.InstrumentCrypto {
			priceBasis = domain.CoinGeckoDailyPriceBasis
			timestampBasis = "observed_publication"
		}
		id := fmt.Sprintf("00000000-0000-4000-8005-%012x", dayIndex*len(fixture.instruments)+index+1)
		price := fmt.Sprintf("%d", 100+dayIndex/30)
		if _, err := quoteStatement.ExecContext(ctx, id, instrument.ID.String(), price, instrument.QuoteCurrency.String(), formatReviewPerfTime(when), formatReviewPerfTime(createdAt), date, formatReviewPerfTime(when), priceBasis, timestampBasis); err != nil {
			return fmt.Errorf("insert daily synthetic quote for %s: %w", instrument.ID, err)
		}
	}
	fxStatement, err := tx.PrepareContext(ctx, `INSERT INTO fx_quotes(id,household_id,base_currency,quote_currency,rate,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,fetched_at,value_effective_at,timestamp_basis,revision) VALUES(?,?,?,'CNY','2','manual','manual',?,?, 'daily_reference',?,?,?,'date_label',1)`)
	if err != nil {
		return err
	}
	defer fxStatement.Close()
	for index, currency := range []string{"USD", "EUR", "JPY", "SGD"} {
		id := fmt.Sprintf("00000000-0000-4000-8006-%012x", dayIndex*4+index+1)
		if _, err := fxStatement.ExecContext(ctx, id, fixture.householdID.String(), currency, formatReviewPerfTime(when), formatReviewPerfTime(createdAt), date, formatReviewPerfTime(when), formatReviewPerfTime(when)); err != nil {
			return fmt.Errorf("insert daily synthetic FX reference for %s: %w", currency, err)
		}
	}
	return tx.Commit()
}

func correctReviewPerfHistoricalQuote(ctx context.Context, fixture reviewFullPipelineFixture, sample int) error {
	day := fixture.start.AddDate(0, 0, 5+sample%20).Format(time.DateOnly)
	tx, err := fixture.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM instrument_quotes WHERE instrument_id=? AND effective_date=? AND source_kind='agent' ORDER BY created_at DESC,id DESC LIMIT 1`, fixture.instruments[0].ID.String(), day).Scan(&id); err != nil {
		return err
	}
	correctedPrice := fmt.Sprintf("%d", 250+sample)
	if _, err := tx.ExecContext(ctx, `UPDATE instrument_quotes SET unit_price=? WHERE id=?`, correctedPrice, id); err != nil {
		return err
	}
	return tx.Commit()
}

func seedReviewFullPipelineFacts(ctx context.Context, db *sqlite.DB, householdID domain.HouseholdID, instruments []domain.Instrument, accountID domain.AccountID, start time.Time, historyDays, quoteCount, activityCount int) error {
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	quoteStatement, err := tx.PrepareContext(ctx, `INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,value_effective_at,binding_revision,price_basis,timestamp_basis,revision) VALUES(?,?,?,?,'agent','agent',?,?, 'close',?,?,0,?,?,?)`)
	if err != nil {
		return err
	}
	defer quoteStatement.Close()
	quoteIndex := 0
	quoteSlots := len(instruments) * historyDays
	quotesPerSlot, quoteRemainder := quoteCount/quoteSlots, quoteCount%quoteSlots
	for day := 0; day < historyDays; day++ {
		date := start.AddDate(0, 0, day)
		for instrumentIndex, instrument := range instruments {
			copies := quotesPerSlot
			if instrumentIndex*historyDays+day < quoteRemainder {
				copies++
			}
			for duplicate := 0; duplicate < copies; duplicate++ {
				priceBasis, timestampBasis := "agent_raw_close_v1", "date_label"
				if instrument.Type == domain.InstrumentMutualFund || instrument.Type == domain.InstrumentBankInvestmentProduct {
					priceBasis = "agent_unit_nav_v1"
				} else if instrument.Type == domain.InstrumentCrypto {
					priceBasis = domain.CoinGeckoDailyPriceBasis
					timestampBasis = "observed_publication"
				}
				when := date.Add(17 * time.Hour)
				price := fmt.Sprintf("%d", 100+day/30)
				id := fmt.Sprintf("00000000-0000-4000-8000-%012x", quoteIndex+1)
				createdAt := when.Add(time.Duration(duplicate) * time.Second)
				if _, err := quoteStatement.ExecContext(ctx, id, instrument.ID.String(), price, instrument.QuoteCurrency.String(), formatReviewPerfTime(when), formatReviewPerfTime(createdAt), date.Format(time.DateOnly), formatReviewPerfTime(when), priceBasis, timestampBasis, duplicate+1); err != nil {
					return fmt.Errorf("insert quote %d: %w", quoteIndex, err)
				}
				quoteIndex++
			}
		}
	}
	if quoteIndex != quoteCount {
		return fmt.Errorf("seeded %d quote facts, want %d", quoteIndex, quoteCount)
	}
	fxStatement, err := tx.PrepareContext(ctx, `INSERT INTO fx_quotes(id,household_id,base_currency,quote_currency,rate,source_kind,source_key,quoted_at,created_at,observation_kind,effective_date,fetched_at,value_effective_at,timestamp_basis,revision) VALUES(?,?,?,'CNY',?,'manual','manual',?,?, 'daily_reference',?,?,?,'date_label',1)`)
	if err != nil {
		return err
	}
	defer fxStatement.Close()
	fxIndex := 0
	for day := 0; day < historyDays; day++ {
		date := start.AddDate(0, 0, day)
		for _, currency := range []string{"USD", "EUR", "JPY", "SGD"} {
			when := date.Add(16 * time.Hour)
			id := fmt.Sprintf("00000000-0000-4000-8001-%012x", fxIndex+1)
			if _, err := fxStatement.ExecContext(ctx, id, householdID.String(), currency, "2", formatReviewPerfTime(when), formatReviewPerfTime(when), date.Format(time.DateOnly), formatReviewPerfTime(when), formatReviewPerfTime(when)); err != nil {
				return fmt.Errorf("insert synthetic FX reference %d: %w", fxIndex, err)
			}
			fxIndex++
		}
	}
	activityStatement, err := tx.PrepareContext(ctx, `INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'value_update','other',?,?,?)`)
	if err != nil {
		return err
	}
	defer activityStatement.Close()
	effectStatement, err := tx.PrepareContext(ctx, `INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'observation','added','account_cash','adjustment',?,'0','CNY')`)
	if err != nil {
		return err
	}
	defer effectStatement.Close()
	valueStatement, err := tx.PrepareContext(ctx, `INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'0','CNY',?,?,?,'event')`)
	if err != nil {
		return err
	}
	defer valueStatement.Close()
	for index := 0; index < activityCount; index++ {
		when := start.AddDate(0, 0, index%historyDays).Add(time.Duration(index) * time.Microsecond)
		at := formatReviewPerfTime(when)
		activityID := fmt.Sprintf("00000000-0000-4000-8002-%012x", index+1)
		effectID := fmt.Sprintf("00000000-0000-4000-8003-%012x", index+1)
		valueID := fmt.Sprintf("00000000-0000-4000-8004-%012x", index+1)
		if _, err := activityStatement.ExecContext(ctx, activityID, householdID.String(), at, when.Format(time.DateOnly), at); err != nil {
			return fmt.Errorf("insert activity %d: %w", index, err)
		}
		if _, err := effectStatement.ExecContext(ctx, effectID, activityID, accountID.String()); err != nil {
			return fmt.Errorf("insert activity effect %d: %w", index, err)
		}
		if _, err := valueStatement.ExecContext(ctx, valueID, accountID.String(), at, at, effectID); err != nil {
			return fmt.Errorf("insert account cash value %d: %w", index, err)
		}
	}
	return tx.Commit()
}

func formatReviewPerfTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

// Public financial commands keep effective replay/cost semantics in the mixed
// workload. Fixture construction is excluded from all reported query timings.
// Cash-only bulk rows remain a separate axis, never a proxy for trade replay.
func seedReviewPerfMixedActivities(t *testing.T, s *Service, household domain.HouseholdID, accounts []domain.AccountID, holdings []domain.Holding, instruments []domain.Instrument, start time.Time, c reviewPerfConfig) {
	t.Helper()
	for _, account := range accounts {
		if _, err := s.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: household, AccountID: account, Amount: mustMoney(t, "1000000", "CNY"), Reason: domain.ReasonContribution, EffectiveAt: start.Add(time.Nanosecond)}); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < c.MixedActivities; index++ {
		at := start.AddDate(0, 0, index*c.HistoryDays/c.MixedActivities).Add(time.Hour + time.Duration(index)*time.Nanosecond)
		var change any
		kind := index % 4
		switch c.MixedKind {
		case "trade":
			kind = index % 2
		case "transfer":
			kind = 2
		case "cost":
			kind = 3
		}
		switch kind {
		case 0, 1:
			side := domain.TradeBuy
			if kind == 1 {
				side = domain.TradeSell
			}
			change = domain.TradeInput{HouseholdID: household, Side: side, SettlementAccountID: holdings[0].AccountID, HoldingID: holdings[0].ID, InstrumentID: instruments[0].ID, Quantity: mustQuantity(t, "0.001"), Gross: mustMoney(t, "0.10", "CNY"), EffectiveAt: at}
		case 2:
			change = domain.CashTransferInput{HouseholdID: household, FromAccountID: accounts[0], ToAccountID: accounts[1], Sent: mustMoney(t, "1", "CNY"), Received: mustMoney(t, "1", "CNY"), EffectiveAt: at}
		case 3:
			change = domain.PositionCostAdjustmentInput{HouseholdID: household, HoldingID: holdings[0].ID, UnitCost: reconciliationUnitCost(t, "10.000001"), EffectiveAt: at}
		}
		if _, err := s.RecordChange(t.Context(), change); err != nil {
			t.Fatalf("mixed activity %d: %v", index, err)
		}
	}
}
