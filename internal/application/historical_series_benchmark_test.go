package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func seriesHistoryScaleFixture(t testing.TB, accountCount, backfillsPerInstrument, activityCount int) (*Service, *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/series.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewService(sqlite.NewRepository(db))
	now := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic series benchmark", BaseCurrency: "CNY", MemberNames: []string{"Synthetic owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := bootstrap.Members[0].ID
	instruments := []domain.Instrument{}
	for n := 0; n < 9; n++ {
		i, err := s.CreateInstrument(ctx, InstrumentInput{Name: fmt.Sprintf("Synthetic %d", n), Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		instruments = append(instruments, i)
		if _, err := s.AppendManualInstrumentQuote(ctx, i.ID, "0.00006", "2020-01-01", false); err != nil {
			t.Fatal(err)
		}
	}
	accounts := []domain.AccountRecord{}
	costs := map[domain.HoldingID]string{}
	for n := 0; n < accountCount; n++ {
		a, err := s.CreateAccount(ctx, AccountInput{Name: fmt.Sprintf("Synthetic broker %03d", n), AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{owner}})
		if err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, a)
		if _, err := s.AppendAccountCashValue(ctx, a.Account.ID, "0", "CNY", "2020-01-01"); err != nil {
			t.Fatal(err)
		}
		for _, i := range instruments {
			h, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "2"})
			if err != nil {
				t.Fatal(err)
			}
			costs[h.ID] = "0.00006"
		}
	}
	if _, err := s.StartHistoryWithCosts(ctx, "UTC", costs); err != nil {
		t.Fatal(err)
	}
	if backfillsPerInstrument > 0 || activityCount > 0 {
		tx, err := db.SQL.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		quote, err := tx.Prepare(`INSERT INTO instrument_quotes(id,instrument_id,unit_price,currency,source_kind,source_key,quoted_at,created_at,delayed) VALUES(?,?,'0.00006','CNY','manual','manual',?,?,0)`)
		if err != nil {
			t.Fatal(err)
		}
		defer quote.Close()
		for _, i := range instruments {
			for n := 0; n < backfillsPerInstrument; n++ {
				at := now.AddDate(0, 0, n+1).Format(time.RFC3339Nano)
				if _, err := quote.Exec(domain.NewInstrumentQuoteID().String(), i.ID.String(), at, at); err != nil {
					t.Fatal(err)
				}
			}
		}
		activity, err := tx.Prepare(`INSERT INTO activities(id,household_id,kind,reason,effective_at,effective_local_date,created_at) VALUES(?,?,'value_update','other',?,?,?)`)
		if err != nil {
			t.Fatal(err)
		}
		defer activity.Close()
		effect, err := tx.Prepare(`INSERT INTO activity_effects(id,activity_id,sequence,role,direction,target,classification,account_id,amount,currency) VALUES(?,?,1,'observation','added','account_cash','adjustment',?,'0','CNY')`)
		if err != nil {
			t.Fatal(err)
		}
		defer effect.Close()
		value, err := tx.Prepare(`INSERT INTO account_cash_values(id,account_id,amount,currency,effective_at,created_at,activity_effect_id,projection_kind) VALUES(?,?,'0','CNY',?,?,?,'event')`)
		if err != nil {
			t.Fatal(err)
		}
		defer value.Close()
		for n := 0; n < activityCount; n++ {
			id, fx := domain.NewActivityID().String(), domain.NewActivityEffectID().String()
			at := now.AddDate(0, 0, 1+n%2350).Add(time.Duration(n) * time.Microsecond).Format(time.RFC3339Nano)
			local := at[:10]
			account := accounts[n%len(accounts)].Account.ID.String()
			if _, err := activity.Exec(id, bootstrap.Household.ID.String(), at, local, at); err != nil {
				t.Fatal(err)
			}
			if _, err := effect.Exec(fx, id, account); err != nil {
				t.Fatal(err)
			}
			if _, err := value.Exec(domain.NewAccountCashValueID().String(), account, at, at, fx); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	now = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.SQL.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	return s, db
}
