package main

import (
	"fmt"
	"os"
	"strings"
)

type v3Config struct {
	Scenario string
	Family   string
	Base     string
}

type fxPair struct {
	Base  string
	Quote string
	Rate  func(int) float64
}

func parseV3Scenario(scenario string) (v3Config, bool) {
	cfg := v3Config{Scenario: scenario, Base: "AUD"}
	switch scenario {
	case "complete", "missing-price", "missing-fx", "missing-both":
		cfg.Family = scenario
	case "complete-usd":
		cfg.Family = "complete"
		cfg.Base = "USD"
	case "complete-cny":
		cfg.Family = "complete"
		cfg.Base = "CNY"
	default:
		return v3Config{}, false
	}
	if v := strings.ToUpper(strings.TrimSpace(os.Getenv("NESTWORTH_QA_BASE_CURRENCY"))); v != "" {
		if cfg.Family != "complete" && v != "AUD" {
			panic(fmt.Errorf("NESTWORTH_QA_BASE_CURRENCY=%s is only supported on complete / complete-usd / complete-cny (missing-* stay AUD)", v))
		}
		cfg.Base = v
	}
	switch cfg.Base {
	case "AUD", "USD", "CNY":
	default:
		panic(fmt.Errorf("NESTWORTH_QA_BASE_CURRENCY must be AUD, USD, or CNY, got %q", cfg.Base))
	}
	return cfg, true
}

func usdCnyRate(d int) float64 {
	if d == 35 || d == 36 {
		return 7.2070
	}
	return 7.20 + float64(d)*0.0002
}

func fxPairsForBase(base string) []fxPair {
	switch base {
	case "AUD":
		return []fxPair{
			{Base: "USD", Quote: "AUD", Rate: usdAudRate},
			{Base: "SGD", Quote: "AUD", Rate: sgdAudRate},
		}
	case "USD":
		return []fxPair{
			{Base: "AUD", Quote: "USD", Rate: func(d int) float64 { return 1 / usdAudRate(d) }},
			{Base: "SGD", Quote: "USD", Rate: func(d int) float64 { return sgdAudRate(d) / usdAudRate(d) }},
		}
	case "CNY":
		return []fxPair{
			{Base: "USD", Quote: "CNY", Rate: usdCnyRate},
			{Base: "AUD", Quote: "CNY", Rate: func(d int) float64 { return usdCnyRate(d) / usdAudRate(d) }},
			{Base: "SGD", Quote: "CNY", Rate: func(d int) float64 { return usdCnyRate(d) * sgdAudRate(d) / usdAudRate(d) }},
		}
	default:
		panic(fmt.Errorf("unsupported household base %q", base))
	}
}

func formatFXRate(base string, d int, pair fxPair) string {
	if base == "AUD" {
		return fmt.Sprintf("%.4f", pair.Rate(d))
	}
	return fmt.Sprintf("%.6f", pair.Rate(d))
}

func fxPairLabel(householdBase string, pairs []fxPair) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, p.Base+"/"+p.Quote+"="+formatFXRate(householdBase, 0, p))
	}
	return strings.Join(parts, " ")
}
