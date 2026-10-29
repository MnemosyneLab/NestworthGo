package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestPostOriginBalanceEvidenceAtTimestampBoundaries(t *testing.T) {
	t.Parallel()
	for _, fraction := range []time.Duration{0, 50 * time.Millisecond} {
		for _, delay := range []time.Duration{0, 100 * time.Millisecond, time.Second} {
			for _, initial := range []string{"0", "50", "unknown"} {
				t.Run(fraction.String()+"/"+delay.String()+"/"+initial, func(t *testing.T) {
					ctx, s, _, db, boot, setNow := historicalInsertionFixture(t)
					originAt := historicalInsertionTime(25).Add(fraction)
					setNow(originAt)
					if _, err := s.StartHistory(ctx, "UTC"); err != nil {
						t.Fatal(err)
					}
					setNow(originAt.Add(delay))
					amount := initial
					if initial == "unknown" {
						amount = "0"
					}
					a, err := s.CreateAccount(ctx, AccountInput{Name: "Card", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: amount, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
					if err != nil {
						t.Fatal(err)
					}
					if initial == "unknown" {
						// Legacy state without opening evidence, which the current API cannot create.
						if _, err := db.SQL.ExecContext(ctx, "DELETE FROM account_values WHERE account_id=?", a.Account.ID.String()); err != nil {
							t.Fatal(err)
						}
					}
					setNow(historicalInsertionTime(26))
					snapshot, _, err := s.BuildDailyValuationSnapshot(ctx, "2026-09-25")
					if err != nil {
						t.Fatal(err)
					}
					if snapshot.Complete != (initial != "unknown") {
						t.Fatalf("complete=%v for %s", snapshot.Complete, initial)
					}
					if initial != "unknown" && (len(snapshot.Items) != 1 || snapshot.Items[0].NativeAmount != initial || snapshot.LiabilitiesAmount == nil || snapshot.LiabilitiesAmount.CanonicalAmount() != initial) {
						t.Fatalf("opening evidence lost: %+v", snapshot)
					}
					if initial == "unknown" && (len(snapshot.Items) != 1 || snapshot.Items[0].Complete || snapshot.Items[0].NativeAmount != "") {
						t.Fatalf("unknown became zero: %+v", snapshot.Items)
					}
				})
			}
		}
	}
}
