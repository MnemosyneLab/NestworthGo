package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func importSuffixFX(t *testing.T, f seriesZeroFixture, operation, target, on, rate string) AgentMarketDataResult {
	t.Helper()
	item := AgentMarketDataItem{Operation: operation, TargetQuoteID: target, Kind: "daily_reference", BaseCurrency: "USD", QuoteCurrency: "CNY", Date: on, Timezone: "UTC", Value: rate, SourceTitle: "Synthetic direct FX evidence", SourceURL: "https://example.invalid/synthetic-fx"}
	if operation == "retract" {
		item = AgentMarketDataItem{Operation: operation, TargetQuoteID: target, SourceTitle: "Synthetic FX withdrawal"}
	}
	result, err := f.s.ImportAgentMarketData(t.Context(), string(domain.NewActivityID()), AgentMarketDataInput{Items: []AgentMarketDataItem{item}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Inserted != 1 {
		t.Fatal("production import did not persist", result)
	}
	return result
}

// Every financial mutation here uses the same production service API as Agent/
// MCP. Raw SQL is reserved for negative corruption tests, never positive writes.
func productionFXSuffixFixture(t *testing.T, zones ...string) (seriesZeroFixture, *sqlite.Repository, string) {
	t.Helper()
	zone := "UTC"
	if len(zones) > 0 {
		zone = zones[0]
	}
	s, db, owner, now := overviewFixture(t)
	f := seriesZeroFixture{s: s, db: db, now: now, location: time.UTC, fx: true}
	f.day(t, "2026-10-01")
	f.account = overviewAccount(t, s, owner, "Synthetic imported FX", "brokerage", "asset", "holdings", "USD", "")
	var err error
	f.instrument, err = s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic USD stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	f.holding, err = s.CreateHolding(t.Context(), HoldingInput{AccountID: f.account.Account.ID.String(), InstrumentID: f.instrument.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(t.Context(), f.instrument.ID, "100", now.Format(time.RFC3339), false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetFXPreference(t.Context(), "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	importSuffixFX(t, f, "append", "", "2026-10-01", "1")
	if _, err = s.StartHistoryWithCosts(t.Context(), "UTC", map[domain.HoldingID]string{f.holding.ID: "100"}); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-12")
	second := importSuffixFX(t, f, "append", "", "2026-10-08", "2")
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1].Leaf.FXMode = "convert_on_day"
	spec.Schedule.Timezone = zone
	saveSyntheticReviewRule(t, f, "Synthetic production FX suffix", spec)
	r := sqlite.NewRepository(f.db).WithReviewDependencySuffix()
	fake := newRefreshFakeProvider()
	fake.key = "synthetic-pinned"
	f.s = NewService(r, NewMarketDataRegistry(fake, &refreshAliasProvider{key: "synthetic-other", target: fake}))
	if err = f.s.SetFXProvider(fake.key); err != nil {
		t.Fatal(err)
	}
	f.s.setClock(func() time.Time { return *f.now })
	if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var id string
	var revision int
	if err = f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL LIMIT 1`).Scan(&id, &revision); err != nil {
		t.Fatal("fixture must have an episode", err)
	}
	if _, err = f.s.AcknowledgeReviewEpisodeV2(t.Context(), id, string(domain.NewMutationID()), revision); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f, r, second.QuoteIDs[0]
}

func TestReviewDependencyFXProductionImportFullOracle(t *testing.T) {
	for _, scenario := range []string{"append", "correct", "retract", "move-backward", "move-forward", "move-origin", "move-before-origin", "forged-new-date", "forged-target", "intermediate-chain", "mixed-instrument", "provenance-edit", "provenance-journal-damage"} {
		t.Run(scenario, func(t *testing.T) {
			zone := "UTC"
			if scenario == "move-forward" {
				zone = "Asia/Shanghai"
			}
			f, r, target := productionFXSuffixFixture(t, zone)
			*f.now = f.now.Add(time.Hour)
			control := assertSuffixOracle(t, f, r, true)
			on := "2026-10-08"
			if scenario == "append" {
				on = "2026-10-06"
			}
			oldRate := ""
			for _, day := range control.Capture.Suffix.Prefix {
				if day.Key.On == on {
					for _, leaf := range day.Evidence.Result.Leaves {
						if leaf.FX != nil && leaf.FX.Known {
							oldRate = leaf.FX.Rate
						}
					}
				}
			}
			var imported AgentMarketDataResult
			switch scenario {
			case "retract":
				imported = importSuffixFX(t, f, "retract", target, "", "")
			case "correct":
				imported = importSuffixFX(t, f, "correct", target, on, "3.123456789123")
			case "move-backward", "move-forward", "move-origin", "move-before-origin", "forged-new-date", "forged-target", "intermediate-chain":
				replacementOn := "2026-10-07"
				switch scenario {
				case "move-forward":
					replacementOn = "2026-10-09"
				case "move-origin":
					replacementOn = "2026-10-01"
				case "move-before-origin":
					replacementOn = "2026-09-30"
				}
				imported = importSuffixFX(t, f, "correct", target, replacementOn, "3")
			default:
				imported = importSuffixFX(t, f, "append", "", on, "3.123456789123")
			}
			if scenario == "intermediate-chain" {
				importSuffixFX(t, f, "correct", imported.QuoteIDs[0], "2026-10-09", "4")
			}
			if scenario == "forged-new-date" {
				// A plausible later claim must not hide the actual earlier change.
				if _, err := f.db.SQL.Exec(`UPDATE financial_source_changes SET new_scope_json=json_set(new_scope_json,'$.effective_date','2026-10-09') WHERE source_table='fx_quotes' AND json_extract(new_scope_json,'$.id')=?`, imported.QuoteIDs[0]); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "forged-target" {
				if _, err := f.db.SQL.Exec(`UPDATE financial_source_changes SET new_scope_json=json_set(new_scope_json,'$.target_quote_id',?) WHERE source_table='agent_quote_records' AND json_extract(new_scope_json,'$.id')=?`, imported.QuoteIDs[0], imported.RecordIDs[0]); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "mixed-instrument" {
				_, err := f.s.ImportAgentMarketData(t.Context(), string(domain.NewActivityID()), AgentMarketDataInput{Items: []AgentMarketDataItem{{Kind: "close", InstrumentID: f.instrument.ID.String(), Date: "2026-10-08", Timezone: "UTC", Value: "101", Currency: "USD", SourceTitle: "Synthetic unrelated instrument"}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "provenance-edit" {
				if _, err := f.db.SQL.Exec(`UPDATE agent_quote_records SET source_title='modified old evidence' WHERE quote_id=?`, target); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "provenance-journal-damage" {
				if _, err := f.db.SQL.Exec(`UPDATE financial_source_changes SET new_scope_json=json_set(new_scope_json,'$.quote_id','forged-target') WHERE source_table='agent_quote_records' AND json_extract(new_scope_json,'$.id')=?`, imported.RecordIDs[0]); err != nil {
					t.Fatal(err)
				}
			}
			*f.now = f.now.Add(time.Hour)
			eligible := scenario == "append" || scenario == "correct" || scenario == "retract" || scenario == "move-backward" || scenario == "move-forward" || scenario == "move-origin" || scenario == "move-before-origin"
			auditBefore, err := r.ListAgentQuoteRecords(t.Context(), f.account.Account.HouseholdID)
			if err != nil {
				t.Fatal(err)
			}
			stage := assertSuffixOracle(t, f, r, eligible)
			auditAfter, err := r.ListAgentQuoteRecords(t.Context(), f.account.Account.HouseholdID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(auditBefore, auditAfter) {
				t.Fatal("review publication changed full Agent provenance")
			}
			if eligible && len(auditAfter) != 3 {
				t.Fatal("production append/correct/retract did not preserve all prior provenance", len(auditAfter))
			}
			if !eligible {
				return
			}
			if scenario == "move-origin" || scenario == "move-before-origin" {
				if len(stage.Capture.Suffix.Prefix) != 0 {
					t.Fatal("origin movement reused prefix")
				}
				return
			}
			if stage.Capture.Suffix.Plan.Reason != "direct_fx_suffix" || len(stage.Capture.Suffix.Prefix) == 0 {
				t.Fatal("no production FX suffix")
			}
			for _, rule := range stage.Capture.Rules {
				plan, err := PlanReviewRuleInstrument(t.Context(), stage.Capture, rule, f.instrument.ID, *f.now, f.s.FXProviderKey())
				if err != nil {
					t.Fatal(err)
				}
				want := 4
				if scenario == "append" {
					want = 6
				}
				if scenario == "move-backward" {
					want = 5
				}
				if plan.RecomputedDays != want || len(plan.Days) != 11 || len(stage.Capture.Suffix.Prefix) != 11-want {
					t.Fatal("no saved production work", plan.RecomputedDays, len(plan.Days))
				}
				t.Logf("production %s: reused=%d recomputed=%d total=%d", scenario, len(stage.Capture.Suffix.Prefix), plan.RecomputedDays, len(plan.Days))
			}
			if scenario == "move-backward" || scenario == "move-forward" {
				wantRates := map[string]string{"2026-10-07": "3", "2026-10-08": "3"}
				if scenario == "move-forward" {
					wantRates = map[string]string{"2026-10-08": "1", "2026-10-09": "3"}
				}
				for on, want := range wantRates {
					found := false
					for _, rule := range stage.Rules {
						for _, day := range rule.Days {
							if day.On == on {
								for _, leaf := range day.Result.Leaves {
									if leaf.FX != nil && leaf.FX.Known {
										found = true
										if leaf.FX.Rate != want {
											t.Fatalf("%s rate=%s want=%s", on, leaf.FX.Rate, want)
										}
									}
								}
							}
						}
					}
					if !found {
						t.Fatalf("missing FX evidence on %s", on)
					}
				}
				return
			}
			rate := ""
			for _, rule := range stage.Rules {
				for _, day := range rule.Days {
					if day.On == on {
						for _, leaf := range day.Result.Leaves {
							if leaf.FX != nil && leaf.FX.Known {
								rate = leaf.FX.Rate
							}
						}
					}
				}
			}
			if oldRate == "" || rate == "" || oldRate == rate {
				t.Fatal("production financial evidence unchanged", oldRate, rate)
			}
			t.Logf("production %s admitted rate %s -> %s", scenario, oldRate, rate)
		})
	}
}

func TestReviewDependencyFXProvenanceFinalFence(t *testing.T) {
	f, r, target := productionFXSuffixFixture(t)
	*f.now = f.now.Add(time.Hour)
	importSuffixFX(t, f, "correct", target, "2026-10-07", "3")
	stage := certifiedStage(t, f, r)
	if stage.Capture.Suffix == nil || len(stage.Capture.Certified.FXDescription.Provenance) == 0 {
		t.Fatal("no authenticated production provenance")
	}
	before := noChangeProjectionRows(t, f.db)
	clock := func() time.Time {
		stage.Capture.Certified.FXDescription.Provenance[0].Fingerprint = "forged-after-final-clock"
		return *f.now
	}
	if err := certifiedPublish(t.Context(), f, r, stage, clock); err == nil {
		t.Fatal("changed provenance committed")
	}
	if !reflect.DeepEqual(before, noChangeProjectionRows(t, f.db)) {
		t.Fatal("provenance fence half-wrote review")
	}
}
