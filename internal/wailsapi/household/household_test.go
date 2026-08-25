package household_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func TestBootstrapBeforeOnboarding(t *testing.T) {
	service := household.NewService(wailstest.NewService(t))
	result, err := service.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if result.Household != nil {
		t.Fatalf("Household = %+v, want nil before onboarding", result.Household)
	}
	if result.Members == nil || result.Institutions == nil || result.Groups == nil {
		t.Fatalf("expected empty slices, not nil, so the frontend never special-cases null: %+v", result)
	}
}

func TestCompleteOnboardingThenBootstrap(t *testing.T) {
	service := household.NewService(wailstest.NewService(t))
	ctx := context.Background()
	request := household.CompleteOnboardingRequest{
		HouseholdName: "The Tans",
		BaseCurrency:  "sgd",
		MemberNames:   []string{"Alice", "Bob"},
	}
	if err := service.CompleteOnboarding(ctx, request); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	result, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if result.Household == nil {
		t.Fatal("Household = nil, want a household after onboarding")
	}
	if result.Household.BaseCurrency != "SGD" {
		t.Fatalf("BaseCurrency = %q, want SGD (uppercased)", result.Household.BaseCurrency)
	}
	if len(result.Members) != 2 {
		t.Fatalf("Members = %+v, want 2", result.Members)
	}
}

func TestCompleteOnboardingRoundTripsAsJSON(t *testing.T) {
	service := household.NewService(wailstest.NewService(t))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, household.CompleteOnboardingRequest{HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"A"}}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	result, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal(BootstrapResult): %v", err)
	}
	var decoded household.BootstrapResult
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(BootstrapResult): %v", err)
	}
	if decoded.Household == nil || decoded.Household.Name != "H" {
		t.Fatalf("round-tripped BootstrapResult = %+v", decoded)
	}
	if len(decoded.Members) != 1 || decoded.Members[0].Name != "A" {
		t.Fatalf("round-tripped Members = %+v", decoded.Members)
	}
}

func TestCompleteOnboardingTwiceIsConflict(t *testing.T) {
	service := household.NewService(wailstest.NewService(t))
	ctx := context.Background()
	request := household.CompleteOnboardingRequest{HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"A"}}
	if err := service.CompleteOnboarding(ctx, request); err != nil {
		t.Fatalf("first CompleteOnboarding: %v", err)
	}
	err := service.CompleteOnboarding(ctx, request)
	if err == nil {
		t.Fatal("second CompleteOnboarding: want a conflict error")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok {
		t.Fatalf("error is not a parseable WireError: %v", err)
	}
	if wireErr.Code != "conflict" {
		t.Fatalf("Code = %q, want conflict", wireErr.Code)
	}
}

func TestCompleteOnboardingValidationError(t *testing.T) {
	service := household.NewService(wailstest.NewService(t))
	err := service.CompleteOnboarding(context.Background(), household.CompleteOnboardingRequest{HouseholdName: "H", BaseCurrency: "not-a-currency", MemberNames: []string{"A"}})
	if err == nil {
		t.Fatal("want a validation error for an invalid currency")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok {
		t.Fatalf("error is not a parseable WireError: %v", err)
	}
	if wireErr.Code != "validation" {
		t.Fatalf("Code = %q, want validation", wireErr.Code)
	}
}
