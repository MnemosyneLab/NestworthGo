package application

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestShortNameEditsPreserveIdentityAndProjections(t *testing.T) {
	s, ctx, bootstrap, _ := newOnboardedService(t, "aliases", nil)
	account := createHoldingsAccount(t, s, ctx, bootstrap.Members[0].ID, "Synthetic")
	aliases := []string{"家庭基金 🌿", "家庭基金 🌿"}
	for idx, code := range []string{"000001", "000002"} {
		i, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Share class " + code, Type: "mutual_fund", QuoteCurrency: "CNY", Symbol: code, ISIN: "CN00000000" + code, QuoteSource: "provider", ProviderKey: "eastmoney", ProviderSymbol: code})
		if err != nil {
			t.Fatal(err)
		}
		holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: i.ID.String(), Quantity: "1.23456789"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.AppendManualInstrumentQuote(ctx, i.ID, "12.3456789", "", false); err != nil {
			t.Fatal(err)
		}
		// Manual quote can change quote source. Capture all identity after that existing behavior.
		original, err := s.repository.Instrument(ctx, i.HouseholdID, i.ID)
		if err != nil {
			t.Fatal(err)
		}
		before, err := s.AccountValuation(ctx, account.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		alias := "  " + aliases[idx] + "  "
		updated, err := s.UpdateInstrument(ctx, i.ID, InstrumentInput{ShortName: &alias})
		if err != nil {
			t.Fatal(err)
		}
		if updated.ShortName != aliases[idx] {
			t.Fatalf("alias %q", updated.ShortName)
		}
		check := updated
		check.ShortName = original.ShortName
		check.UpdatedAt = original.UpdatedAt
		// Preference observation identity is populated on reads, not reconstructed command results.
		check.PreferenceObservationID = original.PreferenceObservationID
		check.ProviderBindingRevision = original.ProviderBindingRevision
		if !reflect.DeepEqual(check, original) {
			t.Fatalf("alias changed identity: before %+v after %+v", original, check)
		}
		after, err := s.AccountValuation(ctx, account.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before.BaseValue, after.BaseValue) {
			t.Fatal("alias changed money")
		}
		if before.Composition == nil || after.Composition == nil || before.Composition.TotalExact != after.Composition.TotalExact {
			t.Fatal("alias edit lost account composition or changed its exact total")
		}
		compositionFound := false
		for _, detail := range after.Composition.Details {
			if detail.Key == "account-composition:holding:"+holding.ID.String() {
				compositionFound = true
				if detail.Label != aliases[idx] {
					t.Fatalf("composition alias %q", detail.Label)
				}
				if detail.IdentityLabel != updated.SelectionLabel() || !strings.Contains(detail.IdentityLabel, code) || !strings.Contains(detail.IdentityLabel, original.Name) {
					t.Fatalf("composition lost full instrument identity: %q", detail.IdentityLabel)
				}
				for _, old := range before.Composition.Details {
					if old.Key == detail.Key && (old.BaseAmountExact != detail.BaseAmountExact || old.ShareBPS != detail.ShareBPS || old.TypeKey != detail.TypeKey || !reflect.DeepEqual(old.DetailRefs, detail.DetailRefs)) {
						t.Fatal("alias edit changed composition amount, share, type or references")
					}
				}
			}
		}
		if !compositionFound {
			t.Fatal("holding identity missing from account composition")
		}

		found := false
		for _, c := range after.Components {
			if c.InstrumentID != nil && *c.InstrumentID == i.ID {
				found = true
				if c.InstrumentShortName != aliases[idx] || c.InstrumentName != original.Name || c.InstrumentSymbol != code {
					t.Fatalf("component %+v", c)
				}
			}
		}
		if !found {
			t.Fatal("missing component")
		}
		groups, err := s.InstrumentHoldings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		found = false
		for _, g := range groups {
			if g.InstrumentID == i.ID {
				found = true
				if g.ShortName != aliases[idx] {
					t.Fatalf("group alias %q", g.ShortName)
				}
			}
		}
		if !found {
			t.Fatal("missing group")
		}
		untouched, err := s.UpdateInstrument(ctx, i.ID, InstrumentInput{Note: strPtr("metadata")})
		if err != nil || untouched.ShortName != aliases[idx] {
			t.Fatalf("omitted alias lost: %+v %v", untouched, err)
		}
		if _, err := s.UpdateInstrument(ctx, i.ID, InstrumentInput{ShortName: strPtr(strings.Repeat("长", 121))}); err == nil {
			t.Fatal("accepted long alias")
		}
		empty := "\u2003 "
		cleared, err := s.UpdateInstrument(ctx, i.ID, InstrumentInput{ShortName: &empty})
		if err != nil || cleared.ShortName != "" {
			t.Fatalf("clear %+v %v", cleared, err)
		}
		current, err := s.repository.Holding(ctx, holding.ID)
		if err != nil || current.Quantity.Canonical() != holding.Quantity.Canonical() {
			t.Fatalf("holding changed %+v %v", current, err)
		}
		if err := seedLegacyInstrumentArchive(t, s, i.ID); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListInstruments(ctx, true)
	if err != nil || len(list) != 2 {
		t.Fatalf("archived instruments %v %v", list, err)
	}
}

func TestManagedShortNameUsesContractEditAndSurvivesReload(t *testing.T) {
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, s, ctx, "150000.01")
	cmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: account.String(), Currency: "USD", Principal: "100000.01", EffectiveAt: "2026-09-20T04:00:00Z", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Synthetic contract", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	preview, err := s.PreviewProductOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.RecordProductOperation(ctx, cmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Product(ctx, receipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	alias := " 存款 🌿 "
	input := UpdateProductTermsInput{ProductID: p.Contract.ID, ExpectedRevision: p.Contract.Revision, Terms: cmd.Open.Terms, Policy: depositPolicy()}
	input.Terms.ShortName = &alias
	next, err := s.UpdateProductTerms(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	i, err := s.repository.Instrument(ctx, p.Contract.HouseholdID, p.Contract.InstrumentID)
	if err != nil {
		t.Fatal(err)
	}
	if i.ShortName != "存款 🌿" || next.Contract.ShortName != i.ShortName || i.Name != next.Contract.Name {
		t.Fatal("incoherent product identity")
	}
	if !reflect.DeepEqual(next.CurrentValue, p.CurrentValue) || next.Contract.Principal.CanonicalAmount() != p.Contract.Principal.CanonicalAmount() {
		t.Fatal("nickname changed financial value")
	}
	if _, err := s.UpdateInstrument(ctx, i.ID, InstrumentInput{ShortName: strPtr("independent")}); err == nil {
		t.Fatal("managed instrument accepted independent edit")
	}
	loaded, err := s.Product(ctx, p.Contract.ID)
	if err != nil || loaded.Contract.ShortName != "存款 🌿" {
		t.Fatalf("reload %+v %v", loaded, err)
	}
	overview, err := s.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	_ = overview
	input.ExpectedRevision = loaded.Contract.Revision
	input.Terms.ShortName = strPtr("")
	cleared, err := s.UpdateProductTerms(ctx, input)
	if err != nil || cleared.Contract.ShortName != "" {
		t.Fatalf("clear %+v %v", cleared, err)
	}
}

func TestShortNameStartingPointCostsRetainShareClassIdentity(t *testing.T) {
	s, ctx, bootstrap, _ := newOnboardedService(t, "alias-cost-inputs", nil)
	account := createHoldingsAccount(t, s, ctx, bootstrap.Members[0].ID, "Brokerage")
	alias := "Family fund"
	holdings := map[string]domain.HoldingID{}
	for _, code := range []string{"000001", "000002"} {
		instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Share class " + code, ShortName: &alias, Symbol: code, ISIN: "CN" + code, QuoteCurrency: "CNY", Type: "mutual_fund"})
		if err != nil {
			t.Fatal(err)
		}
		holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
		if err != nil {
			t.Fatal(err)
		}
		holdings[code] = holding.ID
	}
	rows, err := s.StartingPointDraft(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("draft %+v %v", rows, err)
	}
	names := map[string]bool{}
	for _, row := range rows {
		if !strings.HasPrefix(row.InstrumentName, "Family fund · ") || row.Quantity != "10" {
			t.Fatalf("primary %+v", row)
		}
		found := false
		for code, id := range holdings {
			if row.HoldingID == id {
				found = true
				if !strings.Contains(row.InstrumentName, code) || !strings.Contains(row.InstrumentName, "Share class "+code) || !strings.Contains(row.InstrumentName, "CN"+code) {
					t.Fatalf("missing identity %+v", row)
				}
			}
		}
		if !found || names[row.InstrumentName] {
			t.Fatal("cost inputs collapse duplicate aliases")
		}
		names[row.InstrumentName] = true
	}
}
