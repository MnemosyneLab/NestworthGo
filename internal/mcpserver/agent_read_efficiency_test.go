package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func assertCompactRead(t *testing.T, data map[string]any) {
	t.Helper()
	if data["schemaVersion"] != "mcp-read/2" || data["captureId"] == "" || data["capturedAt"] == "" {
		t.Fatal("missing read identity")
	}
	for _, key := range []string{"contentHash", "summaryHash", "query", "expiresAt"} {
		if _, exists := data[key]; exists {
			t.Fatalf("internal metadata leaked: %s", key)
		}
	}
}

func TestAgentAccountReadDirectHoldingsAndFrozenDetail(t *testing.T) {
	fx := responseReviewFixture(t, 21)
	client, recorder := connectPayload(t, fx.service)
	args := AccountValuationReadInput{AccountIDs: []string{fx.brokerage}, Detail: "holdings"}
	initial := scaleCapture(t, client, "get_account_valuations", args)
	assertCompactRead(t, initial)
	components := "components/" + fx.brokerage
	section := initial["sections"].(map[string]any)[components].(map[string]any)
	if len(section["items"].([]any)) != 21 || section["page"].(map[string]any)["hasMore"] != false || initial["delivery"].(map[string]any)["selectionComplete"] != true {
		t.Fatal("small holding result required another call", section["page"])
	}
	before := recorder.total()
	if before.Calls != 1 || before.Max > contextWireLimit {
		t.Fatal(before)
	}
	rows := scaleSection(t, client, initial, "accounts")
	header := rows[0].(map[string]any)
	if header["composition"] != nil {
		t.Fatal("repeated GUI totals")
	}
	if header["priceFreshness"].(map[string]any)["oldestEffectiveDate"] != "2026-09-01" {
		t.Fatal("lost price source date")
	}
	if strings.Contains(string(mustReadJSON(t, header["assetTypes"])), "detailRefs") {
		t.Fatal("GUI identity arrays leaked into summary")
	}

	// In-process and HTTP dispatch expose the same financial rows and fields.
	local := newLocal(t, fx, ReadOnly)
	localResult := localData(t, local, "get_account_valuations", args, false)
	assertCompactRead(t, localResult)
	productJSONEqual(t, localResult["sections"], initial["sections"])

	// A source change must not affect the old capture's inline or later pages.
	original := section["items"].([]any)[0].(map[string]any)
	if _, err := fx.app.AppendManualInstrumentQuote(t.Context(), domain.InstrumentID(original["instrumentId"].(string)), "9.87654321", "2026-10-13T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	detail := scaleCapture(t, client, "get_read_item", ReadItemInput{CaptureID: initial["captureId"].(string), Section: "accounts", ID: fx.brokerage, Limit: 1})
	assertCompactRead(t, detail)
	if len(detail["availableSections"].([]any)) == 0 || len(detail["sections"].(map[string]any)) != 2 {
		t.Fatal("unrequested GUI pages disclosed")
	}
	frozenRows := scaleSection(t, client, detail, components)
	productJSONEqual(t, frozenRows, section["items"])
	if detail["captureId"] != initial["captureId"] {
		t.Fatal("detail recaptured")
	}
	call(t, client, "get_read_item", ReadItemInput{CaptureID: initial["captureId"].(string), Section: "accounts", ID: fx.brokerage, Sections: []string{"not-a-section"}}, true)
	call(t, client, "get_account_valuations", map[string]any{"detail": "everything"}, true)
	t.Logf("native synthetic 21 holdings: direct calls=%d HTTP bytes=%d max=%d", before.Calls, before.Bytes, before.Max)
}

func mustReadJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := jsoncodec.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAgentAccountSearchMatchesInstitutionAndFilters(t *testing.T) {
	fx := newLedgerFixture(t)
	institution, err := fx.app.CreateInstitution(t.Context(), "Synthetic 招商银行", domain.InstitutionBank)
	if err != nil {
		t.Fatal(err)
	}
	members, err := fx.app.ListMembers(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	created, err := fx.app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic 招行储蓄", InitialAmount: "0", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InstitutionID: institution.ID.String(), OwnerIDs: []domain.MemberID{members[0].ID}, IncludeInNetWorth: true})
	if err != nil {
		t.Fatal(err)
	}
	client := ledgerSession(t, fx)
	result := scaleCapture(t, client, "list_accounts", map[string]any{"query": "招商银行"})
	rows := scaleSection(t, client, result, "accounts")
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != created.Account.ID.String() {
		t.Fatal("bank search missed account")
	}
	empty := scaleCapture(t, client, "list_accounts", map[string]any{"query": "招商银行", "filter": map[string]any{"accountType": "credit_card"}})
	if len(scaleSection(t, client, empty, "accounts")) != 0 {
		t.Fatal("search bypassed filter")
	}
}

func TestAgentReadCursorScopeTamperExpiryAndSize(t *testing.T) {
	cache := newBoundedReadCache()
	cache.activate(7)
	now := time.Now()
	cache.now = func() time.Time { return now }
	rows, _ := readRows([]string{"first", "second"})
	initial, err := cache.store(7, readCapture{Kind: "synthetic", Summary: map[string]any{}, Sections: map[string][]json.RawMessage{"rows": rows}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	cursor := *initial.Sections["rows"].Page.NextCursor
	if len(cursor) > 100 {
		t.Fatalf("short section cursor still oversized: %d", len(cursor))
	}
	tampered := []byte(cursor)
	if tampered[10] == 'A' {
		tampered[10] = 'B'
	} else {
		tampered[10] = 'A'
	}
	if _, err = cache.decode(string(tampered)); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err = cache.page(7, ReadPageInput{CaptureID: initial.CaptureID, Section: "other", Cursor: cursor}); err == nil {
		t.Fatal("wrong section accepted")
	}
	other := newBoundedReadCache()
	other.activate(7)
	if _, err = other.decode(cursor); err == nil {
		t.Fatal("cross-session token accepted")
	}
	now = now.Add(readTTL + time.Second)
	if _, err = cache.page(7, ReadPageInput{CaptureID: initial.CaptureID, Section: "rows", Cursor: cursor}); err == nil || !strings.Contains(err.Error(), "read_expired") {
		t.Fatal("expiry lost", err)
	}
	cache.revoke()
	if _, err = cache.page(7, ReadPageInput{CaptureID: initial.CaptureID, Section: "rows", Cursor: cursor}); err == nil || !strings.Contains(err.Error(), "read_revoked") {
		t.Fatal("revocation lost", err)
	}
}

func TestAgentPriceFreshnessKeepsUnknownAndManualSeparate(t *testing.T) {
	date := "2026-09-01"
	got := accountPriceFreshness([]wire.ValuationComponentDTO{{}, {PriceEvidence: &wire.QuoteEvidenceDTO{Freshness: "manual", EffectiveDate: &date}}, {PriceEvidence: &wire.QuoteEvidenceDTO{Freshness: "stale", Carried: true, Delayed: true}}})
	if got["withoutPriceEvidenceCount"] != 1 || got["carriedCount"] != 1 || got["delayedCount"] != 1 || got["oldestEffectiveDate"] != date {
		t.Fatal(got)
	}
	counts := got["counts"].(map[string]int)
	if counts["manual"] != 1 || counts["stale"] != 1 {
		t.Fatal(counts)
	}
	// Compare summary values against the owning calculation, not rounded expectations.
	fx := responseReviewFixture(t, 3)
	values, err := account.NewService(fx.app).AccountValuations(t.Context(), account.AccountFilterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	client := ledgerSession(t, fx)
	capture := scaleCapture(t, client, "get_account_valuations", AccountValuationReadInput{AccountIDs: []string{fx.brokerage}})
	header := scaleSection(t, client, capture, "accounts")[0].(map[string]any)
	for _, value := range values {
		if value.Account.ID == fx.brokerage {
			productJSONEqual(t, header["assetTypes"], accountAssetTypes(value.Composition))
			if header["displayValue"].(map[string]any)["totalExact"] != value.DisplayValue.TotalExact {
				t.Fatal("exact total changed")
			}
		}
	}
}
