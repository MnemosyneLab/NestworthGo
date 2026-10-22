package mcpserver

import (
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
)

func TestAccountReadPreservesAllEvidenceLayouts(t *testing.T) {
	fx := responseReviewFixture(t, 12)
	full, err := account.NewService(fx.app).AccountValuations(t.Context(), account.AccountFilterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	rowsFiltered := scaleSection(t, c, scaleCapture(t, c, "list_accounts", map[string]any{"filter": map[string]any{"accountType": "brokerage"}}), "accounts")
	if len(rowsFiltered) != 1 {
		t.Fatal("account filter contract changed")
	}
	cap := scaleCapture(t, c, "get_account_valuations", map[string]any{"limit": 1})
	rows := scaleSection(t, c, cap, "accounts")
	if len(rows) != len(full) {
		t.Fatal("accounts omitted")
	}
	for _, v := range full {
		item := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "accounts", ID: v.Account.ID})
		fullMap, err := readSummary(v)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := readSummary(v)
		if err != nil {
			t.Fatal(err)
		}
		for section := range item["sections"].(map[string]any) {
			key := strings.TrimSuffix(section, "/"+v.Account.ID)
			values := scaleSection(t, c, item, section)
			switch {
			case strings.HasPrefix(key, "composition_"):
				field := strings.TrimPrefix(key, "composition_")
				featureJSONEqual(t, values, fullMap["composition"].(map[string]any)[field])
				delete(expected["composition"].(map[string]any), field)
			case strings.HasPrefix(key, "display_"):
				field := strings.TrimPrefix(key, "display_")
				featureJSONEqual(t, values, fullMap["displayValue"].(map[string]any)[field])
				delete(expected["displayValue"].(map[string]any), field)
			default:
				featureJSONEqual(t, values, fullMap[key])
				delete(expected, key)
			}
		}
		featureJSONEqual(t, item["summary"], expected)
	}
	empty := scaleCapture(t, c, "get_account_valuations", map[string]any{"accountIds": []string{fx.checking}, "memberId": "00000000-0000-4000-8000-000000000000"})
	if len(scaleSection(t, c, empty, "accounts")) != 0 {
		t.Fatal("empty filtered accounts")
	}
}

func TestDefaultAccountSnapshotEvidenceAndMissingIdentity(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	cap := scaleCapture(t, c, "get_account_snapshot", IDInput{ID: fx.checking})
	rows := scaleSection(t, c, cap, "accounts")
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != fx.checking {
		t.Fatal("account lost")
	}
	item := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: cap["captureId"].(string), Section: "accounts", ID: fx.checking})
	for section := range item["sections"].(map[string]any) {
		scaleSection(t, c, item, section)
	}
	if err := account.NewService(fx.app).ArchiveAccount(t.Context(), fx.checking, true); err != nil {
		t.Fatal(err)
	}
	call(t, c, "get_account_snapshot", IDInput{ID: fx.checking}, true)
	call(t, c, "get_account_snapshot", IDInput{ID: "00000000-0000-4000-8000-000000000000"}, true)
}
