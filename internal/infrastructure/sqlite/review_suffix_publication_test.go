package sqlite

import (
	"github.com/waltwang/nestworth-go/internal/domain"
	"strings"
	"testing"
)

func TestReviewSuffixAttestationRequiresCompleteTypedOutput(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rule := domain.ReviewRule{ID: domain.NewReviewRuleID(), CurrentConditionVersion: 1}
	id := domain.NewInstrumentID()
	c := domain.ReviewProjectionCapture{HouseholdID: domain.NewHouseholdID(), Certified: &domain.ReviewCertifiedCapture{}}
	input := domain.ReviewDailyInput{InstrumentID: id, On: "2026-10-01", FXProviderKey: "synthetic"}
	item := domain.ReviewRulePublication{RuleID: rule.ID, ConditionVersion: 1, InstrumentID: id, TimezoneVersion: "UTC/v1"}
	day := domain.ReviewPublicationDay{On: input.On, Input: input, Result: domain.ReviewDailyResult{On: input.On, Truth: domain.ReviewFalse}, InputFingerprint: "synthetic-exact"}
	a, b, err := reviewDailyPublicationArguments(c, item, day, "")
	if err != nil {
		t.Fatal(err)
	}
	original := append(append([]any{}, a[:13]...), b[:9]...)
	for _, scenario := range []string{"equal", "quantity", "currency", "type", "amount", "cost", "input-json", "coverage", "truth", "reason", "result-json", "result-fingerprint", "changed-source", "household-key", "rule-key", "timezone-key", "condition-type"} {
		t.Run(scenario, func(t *testing.T) {
			values := append([]any{}, original...)
			switch scenario {
			case "household-key":
				values[0] = "other-household"
			case "rule-key":
				values[13] = "other-rule"
			case "timezone-key":
				values[2] = "UTC/other"
			case "condition-type":
				values[14] = "1"
			case "quantity":
				values[5] = "0"
			case "currency":
				values[6] = "CNY"
			case "type":
				values[7] = "stock"
			case "amount":
				values[8] = "0"
			case "cost":
				values[9] = "0"
			case "input-json":
				values[10] = " " + values[10].(string)
			case "coverage":
				values[11] = " " + values[11].(string)
			case "truth":
				values[18] = "true"
			case "reason":
				values[19] = ""
			case "result-json":
				values[20] = " " + values[20].(string)
			case "result-fingerprint":
				values[21] = "different"
			}
			columns := 22
			if scenario == "changed-source" {
				values = []any{values[1], values[3], values[12], values[10], values[11], values[20], values[21], int64(0)}
				columns = 8
			}
			rows, err := db.SQL.Query("SELECT "+strings.TrimSuffix(strings.Repeat("?,", columns), ","), values...)
			if err != nil {
				t.Fatal(err)
			}
			evidence := map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence{}
			attested := map[domain.ReviewDailyEvidenceKey]bool{}
			if scenario == "changed-source" {
				err = scanReusableReviewWindow(rows, rule, item.TimezoneVersion, evidence)
			} else {
				err = scanReviewSuffixOutput(rows, c, rule, item.TimezoneVersion, evidence, attested, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "changed-source" && len(evidence) != 1 {
				t.Fatal("changed source did not load ordinary un-attested evidence")
			}
			key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: 1, InstrumentID: id, TimezoneVersion: item.TimezoneVersion, On: input.On}
			if attested[key] != (scenario == "equal") {
				t.Fatalf("attested=%v scenario=%s", attested[key], scenario)
			}
		})
	}
}
