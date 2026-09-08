package analysis

import (
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAnalysisQueryRequestConvertsTypedScopeAndFilters(t *testing.T) {
	accountID := "00000000-0000-4000-8000-000000000001"
	memberID := "00000000-0000-4000-8000-000000000002"
	request := AnalysisQueryRequest{ScopeKind: "account", ScopeID: accountID, From: "2026-08-01", To: "2026-08-02", Valuation: "base", Basis: "investment", IncludeCash: true, AccountID: &accountID, Currency: stringPtr("usd"), AssetClass: "cash", MemberID: &memberID}
	query, err := request.ToDomain()
	if err != nil {
		t.Fatal(err)
	}
	if query.Scope.Kind != domain.ScopeAccount || query.Scope.ID != accountID || query.Filters.Currency.String() != "USD" || query.Filters.MemberID == nil {
		t.Fatalf("converted query = %+v", query)
	}
}

func TestAnalysisDTOUsesCanonicalSignedMoneyAndCompleteness(t *testing.T) {
	amount, err := domain.ParseSignedMoney("-12.5", domain.CurrencyCode("USD"))
	if err != nil {
		t.Fatal(err)
	}
	forced := "base"
	value := application.AssetChangeResult{
		AnalysisAvailability: application.AnalysisAvailability{Available: true, Status: domain.CompletenessPartial, MissingReason: "missing quote", ValuationForced: &forced},
		Summary:              application.AssetChangeSummary{Change: &amount},
		ResidualIssueCount:   2,
	}
	dto := fromAssetChange(value)
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(encoded, &shape); err != nil {
		t.Fatal(err)
	}
	if shape["available"] != true || shape["status"] != "partial" || shape["missingReason"] != "missing quote" || shape["valuationForced"] != "base" {
		t.Fatalf("metadata shape = %s", encoded)
	}
	if shape["residualIssueCount"] != float64(2) {
		t.Fatalf("residualIssueCount shape = %s, want 2", encoded)
	}
	summary, ok := shape["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary shape = %s", encoded)
	}
	change, ok := summary["change"].(map[string]any)
	if !ok || change["amount"] != "-12.5" || change["currency"] != "USD" {
		t.Fatalf("canonical money shape = %s", encoded)
	}
	unforced, err := json.Marshal(fromAssetChange(application.AssetChangeResult{AnalysisAvailability: application.AnalysisAvailability{Available: true, Status: domain.CompletenessOK}}))
	if err != nil {
		t.Fatal(err)
	}
	var unforcedShape map[string]any
	if err := json.Unmarshal(unforced, &unforcedShape); err != nil {
		t.Fatal(err)
	}
	if value, exists := unforcedShape["valuationForced"]; !exists || value != nil {
		t.Fatalf("unforced valuation shape = %s, want valuationForced null", unforced)
	}
}

func TestAnalysisDTOsSerializeUnforcedValuationAsNull(t *testing.T) {
	values := []struct {
		name  string
		value any
	}{
		{name: "asset driver", value: fromAssetDriverDetail(application.AssetDriverDetailResult{})},
		{name: "asset trend", value: fromAssetTrend(application.AssetTrendResult{})},
		{name: "categories", value: fromCategories(application.CategoriesResult{})},
		{name: "category detail", value: fromCategoryDetail(application.CategoryDetailResult{})},
	}
	for _, testCase := range values {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := json.Marshal(testCase.value)
			if err != nil {
				t.Fatal(err)
			}
			var shape map[string]any
			if err := json.Unmarshal(encoded, &shape); err != nil {
				t.Fatal(err)
			}
			if value, exists := shape["valuationForced"]; !exists || value != nil {
				t.Fatalf("unforced valuation shape = %s, want valuationForced null", encoded)
			}
		})
	}
}

func TestAssetDriverDTOIncludesResidualDetails(t *testing.T) {
	amount, err := domain.ParseSignedMoney("-5", "USD")
	if err != nil {
		t.Fatal(err)
	}
	dto := fromAssetDriverDetail(application.AssetDriverDetailResult{ResidualDetails: []application.AssetResidualDetail{{Date: "2026-08-02", ComponentKey: "account/holding/instrument", Amount: &amount}}})
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(encoded, &shape); err != nil {
		t.Fatal(err)
	}
	details, ok := shape["residualDetails"].([]any)
	if !ok || len(details) != 1 {
		t.Fatalf("residual details shape = %s", encoded)
	}
	if details[0].(map[string]any)["componentKey"] != "account/holding/instrument" {
		t.Fatalf("residual detail = %s", encoded)
	}
}

func stringPtr(value string) *string { return &value }
