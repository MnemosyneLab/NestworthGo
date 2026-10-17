package domain

import "testing"

func TestReviewLegacyConfigurationRejectsPartialStateWithoutInferringPreferences(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`null`,
		`{"revision":1,"policy":{},"enrolled":{},"acknowledged":{}}`,
		`{"revision":1,"policy":{"enabled":true,"windowDays":30,"lowAnnualizedPercent":"1","lossPercent":"-1","sustainDays":7,"maxAgeDays":7},"enrolled":null,"acknowledged":{}}`,
		`{"revision":1,"policy":{"enabled":true,"windowDays":30,"lowAnnualizedPercent":"1","lossPercent":"-1","sustainDays":7,"maxAgeDays":7},"enrolled":{},"acknowledged":{},"unknownPreference":true}`,
	} {
		if _, err := DecodeReviewLegacyConfiguration([]byte(raw)); err == nil {
			t.Fatalf("accepted partial or unsupported legacy state: %s", raw)
		}
	}
}

func TestReviewLegacyConfigurationRetainsFalseEnrollmentAndUnknownAcknowledgementKey(t *testing.T) {
	raw := `{"revision":4,"policy":{"enabled":false,"windowDays":30,"lowAnnualizedPercent":"1","lossPercent":"-1","sustainDays":7,"maxAgeDays":7},"enrolled":{"00000000-0000-4000-8000-000000000050":false},"acknowledged":{"unresolvable-v1-hash":true},"snoozedUntil":{"00000000-0000-4000-8000-000000000050":"2026-10-30"}}`
	value, err := DecodeReviewLegacyConfiguration([]byte(raw))
	if err != nil || len(value.Enrolled) != 1 || value.Enrolled["00000000-0000-4000-8000-000000000050"] || !value.Acknowledged["unresolvable-v1-hash"] || value.SnoozedUntil["00000000-0000-4000-8000-000000000050"] != "2026-10-30" || value.Policy.Enabled {
		t.Fatalf("legacy intent changed: %+v %v", value, err)
	}
}
