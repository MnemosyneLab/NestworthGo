package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAnalysisQuoteDependenciesEquivalence(t *testing.T) {
	input, account, _, _ := reviewTradeScenario(t)
	for i := 0; i < 40; i++ {
		id := domain.NewInstrumentID()
		input.Portfolio.Instruments = append(input.Portfolio.Instruments, domain.Instrument{ID: id, QuoteCurrency: "CNY"})
		other := analysisAccount(account.HouseholdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
		input.Portfolio.Accounts = append(input.Portfolio.Accounts, domain.AccountRecord{Account: other})
		holding := domain.NewHoldingID()
		input.Portfolio.Holdings = append(input.Portfolio.Holdings, domain.Holding{ID: holding, AccountID: other.ID, InstrumentID: id})
		for j := range input.Snapshots {
			input.Snapshots[j].Items = append(input.Snapshots[j].Items, analysisItem(t, other.ID, "CNY", "100", "100", &holding, &id, "", ""))
		}
		input.InstrumentQuotes = append(input.InstrumentQuotes, domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, Currency: "CNY", UnitPrice: mustUnitPrice(t, "100")})
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeAccount, ID: account.ID.String()}, From: "2026-08-02", To: "2026-08-02", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	for _, missing := range []bool{false, true} {
		full := input
		if missing {
			full.InstrumentQuotes = full.InstrumentQuotes[1:]
		}
		ids := planAnalysisQuoteDependencies(full, query)
		if len(ids) != 1 {
			t.Fatalf("requested=%d want 1", len(ids))
		}
		candidate := full
		candidate.InstrumentQuotes = nil
		for _, quote := range full.InstrumentQuotes {
			for _, id := range ids {
				if quote.InstrumentID == id {
					candidate.InstrumentQuotes = append(candidate.InstrumentQuotes, quote)
				}
			}
		}
		want, err := ComputeAnalysis(full, query)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ComputeAnalysis(candidate, query)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatal("complete results differ")
		}
		t.Logf("missing=%v requested instruments 41 -> %d; quote rows %d -> %d", missing, len(ids), len(full.InstrumentQuotes), len(candidate.InstrumentQuotes))
	}
	input.Snapshots[0].Items[0].AccountID = domain.NewAccountID()
	if len(planAnalysisQuoteDependencies(input, query)) != len(input.Portfolio.Instruments) {
		t.Fatal("unknown historical account must fall back")
	}
}

func TestAnalysisQuoteDependencyActivityClosure(t *testing.T) {
	input, account, _, _ := reviewTradeScenario(t)
	other := analysisAccount(account.HouseholdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	input.Portfolio.Accounts = append(input.Portfolio.Accounts, domain.AccountRecord{Account: other})
	id := domain.NewInstrumentID()
	otherHolding := domain.NewHoldingID()
	input.Portfolio.Instruments = append(input.Portfolio.Instruments, domain.Instrument{ID: id}, domain.Instrument{ID: domain.NewInstrumentID()})
	input.Portfolio.Holdings = append(input.Portfolio.Holdings, domain.Holding{ID: otherHolding, AccountID: other.ID, InstrumentID: id})
	activity := input.Activities[0]
	activity.RecordedEffects = []domain.ActivityEffect{{HoldingID: &otherHolding, InstrumentID: &id}}
	input.Activities = []domain.Activity{activity}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeAccount, ID: account.ID.String()}}
	if len(planAnalysisQuoteDependencies(input, query)) != 2 {
		t.Fatal("raw activity dependency omitted")
	}
	missing := input
	missing.Portfolio.Holdings = nil
	if len(planAnalysisQuoteDependencies(missing, query)) != 3 {
		t.Fatal("unresolved historical holding must fall back")
	}
	input.Activities[0].Kind = "future_unsupported"
	if len(planAnalysisQuoteDependencies(input, query)) != 3 {
		t.Fatal("unsupported activity must fall back")
	}
}

func TestAnalysisQuoteDependencyTransferResults(t *testing.T) {
	householdID := domain.NewHouseholdID()
	household := &domain.Household{ID: householdID, BaseCurrency: "CNY"}
	source := analysisAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	receiver := analysisAccount(householdID, "CNY", domain.TrackingHoldings, domain.RoleAsset)
	instrumentID := domain.NewInstrumentID()
	fromHolding, toHolding := domain.NewHoldingID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Type: domain.InstrumentStock, QuoteCurrency: "CNY"}
	openingQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: mustUnitPrice(t, "100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	previous := analysisSnapshot("2026-08-01", analysisItem(t, source.ID, "CNY", "100", "100", &fromHolding, &instrumentID, openingQuote.ID.String(), ""), analysisItem(t, receiver.ID, "CNY", "0", "0", &toHolding, &instrumentID, "", ""))
	current := analysisSnapshot("2026-08-02", analysisItem(t, source.ID, "CNY", "0", "0", &fromHolding, &instrumentID, quote.ID.String(), ""), analysisItem(t, receiver.ID, "CNY", "100", "100", &toHolding, &instrumentID, quote.ID.String(), ""))
	activityID := domain.NewActivityID()
	quantity := mustQuantity(t, "1")
	activity := domain.Activity{ID: activityID, Kind: domain.ActivityPositionTransfer, Reason: domain.ReasonOther, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02", Effects: []domain.ActivityEffect{
		{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Direction: domain.EffectRemoved, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationInternalTransfer, HoldingID: &fromHolding, InstrumentID: &instrumentID, Quantity: &quantity},
		{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 2, Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationInternalTransfer, HoldingID: &toHolding, InstrumentID: &instrumentID, Quantity: &quantity},
	}}
	input := AnalysisInputs{Origin: analysisOrigin(t, householdID, "UTC"), Portfolio: domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: source}, {Account: receiver}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{{ID: fromHolding, AccountID: source.ID, InstrumentID: instrumentID}, {ID: toHolding, AccountID: receiver.ID, InstrumentID: instrumentID}}}, Snapshots: []domain.DailyValuationSnapshot{previous, current}, Activities: []domain.Activity{activity}, InstrumentQuotes: []domain.InstrumentQuote{openingQuote, quote}}
	unrelated := domain.NewInstrumentID()
	input.Portfolio.Instruments = append(input.Portfolio.Instruments, domain.Instrument{ID: unrelated})
	input.InstrumentQuotes = append(input.InstrumentQuotes, domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: unrelated, Currency: "CNY", UnitPrice: mustUnitPrice(t, "1")})
	for _, account := range []domain.Account{source, receiver} {
		query := analysisBaseQuery(domain.ValuationBase)
		query.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: account.ID.String()}
		assertAnalysisDependencyEquivalent(t, input, query)
	}
}

func assertAnalysisDependencyEquivalent(t *testing.T, input AnalysisInputs, query domain.AnalysisQuery) {
	t.Helper()
	ids := planAnalysisQuoteDependencies(input, query)
	if len(ids) >= len(input.Portfolio.Instruments) {
		t.Fatal("fixture did not activate narrowing")
	}
	candidate := input
	candidate.InstrumentQuotes = nil
	for _, quote := range input.InstrumentQuotes {
		for _, id := range ids {
			if quote.InstrumentID == id {
				candidate.InstrumentQuotes = append(candidate.InstrumentQuotes, quote)
			}
		}
	}
	for _, investmentOnly := range []bool{false, true} {
		expected, err := computeAnalysisForValuationUniverseContext(t.Context(), input, query, investmentOnly)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := computeAnalysisForValuationUniverseContext(t.Context(), candidate, query, investmentOnly)
		if err != nil || !reflect.DeepEqual(expected, actual) {
			t.Fatalf("complete result differs: %v", err)
		}
	}
}

func TestAnalysisQuoteDependencyCorrectionResults(t *testing.T) {
	for _, scenario := range []string{"sold", "reversal replacement", "cost origin"} {
		t.Run(scenario, func(t *testing.T) {
			input, account, _, holding := reviewTradeScenario(t)
			unrelated := domain.NewInstrumentID()
			input.Portfolio.Instruments = append(input.Portfolio.Instruments, domain.Instrument{ID: unrelated})
			input.InstrumentQuotes = append(input.InstrumentQuotes, domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: unrelated, Currency: "CNY", UnitPrice: mustUnitPrice(t, "1")})
			if scenario == "sold" {
				input.Snapshots[0].Items, input.Snapshots[1].Items = input.Snapshots[1].Items, input.Snapshots[0].Items
				activity := input.Activities[0]
				detail := *activity.TradeDetail
				detail.Side = domain.TradeSell
				activity.TradeDetail = &detail
				activity.Effects = append([]domain.ActivityEffect{}, activity.Effects...)
				for i := range activity.Effects {
					if activity.Effects[i].Direction == domain.EffectAdded {
						activity.Effects[i].Direction = domain.EffectRemoved
					} else {
						activity.Effects[i].Direction = domain.EffectAdded
					}
				}
				input.Activities = []domain.Activity{activity}
				input.Portfolio.Holdings[0].Quantity = mustQuantity(t, "0")
			}
			if scenario == "reversal replacement" {
				original := input.Activities[0]
				reversal := original
				reversal.ID = domain.NewActivityID()
				reversal.ReversesActivityID = &original.ID
				reversal.Effects = append([]domain.ActivityEffect{}, original.Effects...)
				detail := *original.TradeDetail
				detail.Side = domain.TradeSell
				reversal.TradeDetail = &detail
				for i := range reversal.Effects {
					if reversal.Effects[i].Direction == domain.EffectAdded {
						reversal.Effects[i].Direction = domain.EffectRemoved
					} else {
						reversal.Effects[i].Direction = domain.EffectAdded
					}
				}
				replacement := original
				replacement.ID = domain.NewActivityID()
				input.Activities = []domain.Activity{original, reversal, replacement}
			}
			if scenario == "cost origin" {
				cost := mustUnitPrice(t, "90")
				instrument := input.Portfolio.Holdings[0].InstrumentID
				input.Activities = append(input.Activities, domain.Activity{ID: domain.NewActivityID(), Kind: domain.ActivityCostAdjustment, EffectiveAt: input.Activities[0].EffectiveAt, Effects: []domain.ActivityEffect{{HoldingID: &holding, InstrumentID: &instrument, Target: domain.EffectTargetHoldingCost, CostUnitPrice: &cost}}})
			}
			query := analysisBaseQuery(domain.ValuationBase)
			query.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: account.ID.String()}
			assertAnalysisDependencyEquivalent(t, input, query)
		})
	}
}
