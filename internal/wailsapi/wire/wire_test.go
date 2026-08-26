package wire

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// TestMoneyDoesNotProduceEmptyObject is the permanent regression test for
// domain.Money/Quantity/UnitPrice/FxRate wrap an unexported decimal.Decimal,
// so json.Marshal on the bare
// domain type silently produces "{}". Every wailsapi DTO must instead carry
// the canonical string produced here, never the bare domain value type.
func TestMoneyDoesNotProduceEmptyObjectBaseline(t *testing.T) {
	money, err := domain.ParseMoney("123.45", "USD")
	if err != nil {
		t.Fatalf("ParseMoney: %v", err)
	}
	raw, err := json.Marshal(money)
	if err != nil {
		t.Fatalf("json.Marshal(domain.Money): %v", err)
	}
	if string(raw) != "{}" {
		t.Fatalf("expected the historically-broken bare marshal to still be \"{}\" (got %s); if this changed, domain.Money gained exported fields and this comment/DTO design should be revisited", raw)
	}

	view := FromMoney(money)
	roundTripped, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("json.Marshal(MoneyView): %v", err)
	}
	var decoded MoneyView
	if err := json.Unmarshal(roundTripped, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(MoneyView): %v", err)
	}
	if decoded.Amount != "123.45" || decoded.Currency != "USD" {
		t.Fatalf("MoneyView round-trip = %+v, want {123.45 USD}", decoded)
	}
}

func TestFormatTimeRoundTrip(t *testing.T) {
	original := time.Date(2026, 3, 14, 9, 26, 53, 123_000_000, time.FixedZone("SGT", 8*3600))
	formatted := FormatTime(original)
	if formatted != "2026-03-14T01:26:53.123Z" {
		t.Fatalf("FormatTime = %q, want UTC millisecond RFC3339", formatted)
	}
	parsed, err := ParseTime(formatted)
	if err != nil {
		t.Fatalf("ParseTime: %v", err)
	}
	if !parsed.Equal(original.UTC().Truncate(time.Millisecond)) {
		t.Fatalf("ParseTime(%q) = %v, want %v", formatted, parsed, original.UTC())
	}
}

func TestFormatTimeZeroValue(t *testing.T) {
	if got := FormatTime(time.Time{}); got != "" {
		t.Fatalf("FormatTime(zero) = %q, want empty string", got)
	}
}

func TestFormatTimePtrNil(t *testing.T) {
	if got := FormatTimePtr(nil); got != nil {
		t.Fatalf("FormatTimePtr(nil) = %v, want nil", got)
	}
}

func TestFromMoneyViewNilIsNil(t *testing.T) {
	if got := FromMoneyView(nil); got != nil {
		t.Fatalf("FromMoneyView(nil) = %v, want nil", got)
	}
	if got := FromSignedMoneyView(nil); got != nil {
		t.Fatalf("FromSignedMoneyView(nil) = %v, want nil", got)
	}
}

func TestMoneyViewFromDomainMoneyView(t *testing.T) {
	domainView := &domain.MoneyView{Amount: "1000.00", Currency: "SGD"}
	view := FromMoneyView(domainView)
	b, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded["amount"] != "1000.00" || decoded["currency"] != "SGD" {
		t.Fatalf("decoded = %+v, want amount=1000.00 currency=SGD", decoded)
	}
}

func TestSignedMoneyRoundTrip(t *testing.T) {
	signed, err := domain.NewSignedMoney(decimal.RequireFromString("-42.5000"), "USD")
	if err != nil {
		t.Fatalf("NewSignedMoney: %v", err)
	}
	view := FromSignedMoney(signed)
	b, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded SignedMoneyView
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.Amount != "-42.5" || decoded.Currency != "USD" {
		t.Fatalf("decoded = %+v, want {-42.5 USD}", decoded)
	}
}

func TestHouseholdMemberInstitutionGroupRoundTrip(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	household, err := domain.NewHousehold("The Tans", "SGD", now)
	if err != nil {
		t.Fatalf("NewHousehold: %v", err)
	}
	member, err := domain.NewMember(household.ID, "Alice", now)
	if err != nil {
		t.Fatalf("NewMember: %v", err)
	}
	archived := now.Add(time.Hour)
	member.ArchivedAt = &archived
	assetID := domain.NewMediaAssetID()
	member.AvatarAssetID = &assetID

	institution, err := domain.NewInstitution(household.ID, "DBS", now)
	if err != nil {
		t.Fatalf("NewInstitution: %v", err)
	}
	group, err := domain.NewGroup(household.ID, "Family", now)
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}

	for name, value := range map[string]any{
		"household":   FromHousehold(household),
		"member":      FromMember(member),
		"institution": FromInstitution(institution),
		"group":       FromGroup(group),
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s: json.Marshal: %v", name, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("%s: json.Unmarshal: %v", name, err)
		}
		if len(decoded) == 0 {
			t.Fatalf("%s: round-trip produced an empty object: %s", name, encoded)
		}
	}

	memberDTO := FromMember(member)
	if memberDTO.ArchivedAt == nil || memberDTO.AvatarAssetID == nil {
		t.Fatalf("FromMember dropped an optional field: %+v", memberDTO)
	}
}

func TestStringPtrRoundTrip(t *testing.T) {
	if StringPtr("") != nil {
		t.Fatalf("StringPtr(\"\") must be nil")
	}
	value := StringPtr("hello")
	if value == nil || *value != "hello" {
		t.Fatalf("StringPtr(\"hello\") = %v, want pointer to \"hello\"", value)
	}
	if StringFromPtr(nil) != "" {
		t.Fatalf("StringFromPtr(nil) must be empty")
	}
	if StringFromPtr(value) != "hello" {
		t.Fatalf("StringFromPtr(&hello) must be \"hello\"")
	}
}
