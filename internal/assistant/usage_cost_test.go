package assistant

import (
	"testing"
	"time"
)

func pricedFixture() UsageCall {
	c := UsageCall{Kind: "conversation", Status: "succeeded", Model: "gpt-6.1-sol", PricingScope: "openai-public", Started: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	CaptureUsage(&c, []byte(`{"model":"gpt-6.1-sol","service_tier":"default","usage":{"input_tokens":1000,"output_tokens":200,"input_tokens_details":{"cached_tokens":500,"cache_write_tokens":100,"cache_write_tokens_details":{"ttl_30m_tokens":60,"ttl_12h_tokens":40}},"output_tokens_details":{"reasoning_tokens":100}}}`))
	return c
}
func TestFrozenUsageCost(t *testing.T) {
	c := pricedFixture()
	cost := estimateUsage(c, usagePrices)
	if cost.Status != "estimated" || cost.USD != "0.0032" || cost.CachedUSD != "0.00005" || cost.CacheWriteUSD != "0.00035" || !validUsageCost(cost) {
		t.Fatalf("%+v", cost)
	}
	c.Cost = cost
	var total UsageTotal
	total.add(c)
	total.add(UsageCall{})
	if total.EstimatedUSD != "0.0032" || total.PricedCalls != 1 || total.UnpricedCalls != 1 {
		t.Fatalf("%+v", total)
	}
	// Reporting uses the persisted snapshot even if the catalogue is removed.
	if estimateUsage(c, nil).Status != "unknown_price" || c.Cost.USD != "0.0032" {
		t.Fatal("historical estimate changed")
	}
	c.Status = "failed"
	if estimateUsage(c, usagePrices).USD != "0.0032" {
		t.Fatal("failed calls can still be billable")
	}
}
func TestCostUnknownConditions(t *testing.T) {
	for name, change := range map[string]func(*UsageCall){
		"unknown_model":          func(c *UsageCall) { c.Model = "" },
		"unknown_tier":           func(c *UsageCall) { c.ServiceTier = "priority" },
		"unknown_endpoint":       func(c *UsageCall) { c.PricingScope = "" },
		"missing_usage":          func(c *UsageCall) { c.Output = nil },
		"missing_cache_lifetime": func(c *UsageCall) { c.CacheWrite30m = nil },
		"unknown_price":          func(c *UsageCall) { c.Started = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC) },
		"invalid_usage":          func(c *UsageCall) { n := int64(600); c.CacheWrite = &n },
	} {
		t.Run(name, func(t *testing.T) {
			c := pricedFixture()
			change(&c)
			v := estimateUsage(c, usagePrices)
			if v.Status != name || v.USD != "" || v.Price != nil {
				t.Fatalf("%+v", v)
			}
		})
	}
	c := pricedFixture()
	if estimateUsage(c, append(append([]priceCatalogue{}, usagePrices...), usagePrices...)).Status != "ambiguous_price" {
		t.Fatal("overlap accepted")
	}
}
func TestLongContextAndAudioCost(t *testing.T) {
	c := pricedFixture()
	n := int64(272000)
	zero := int64(0)
	c.Input = &n
	c.CachedInput = &zero
	c.CacheWrite = &zero
	if v := estimateUsage(c, usagePrices); v.USD != "0.546" || v.Price.LongContext {
		t.Fatalf("boundary %+v", v)
	}
	n++
	v := estimateUsage(c, usagePrices)
	if v.USD != "1.091004" || !v.Price.LongContext || v.Price.Input != "4" || v.Price.Output != "15" {
		t.Fatalf("long %+v", v)
	}
	c.Kind = "transcription"
	c.Model = ""
	c.RequestedModel = "gpt-transcribe"
	seconds := "12.345"
	c.AudioSeconds = &seconds
	v = estimateUsage(c, usagePrices)
	if v.USD != "0.000925875" || v.Price.ModelBasis != "requested" {
		t.Fatalf("audio %+v", v)
	}
	c.AudioSeconds = nil
	if estimateUsage(c, usagePrices).Status != "missing_duration" {
		t.Fatal("invented duration")
	}
}
func TestCaptureNonoverlappingCacheAndTTL(t *testing.T) {
	c := pricedFixture()
	if c.CacheWrite30m == nil || *c.CacheWrite30m != 60 || c.CacheWrite12h == nil || *c.CacheWrite12h != 40 || c.ServiceTier != "default" {
		t.Fatal("lost metadata")
	}
	for _, raw := range []string{
		`{"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":60,"cache_write_tokens":50}}}`,
		`{"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":50,"cache_write_tokens_details":{"ttl_30m_tokens":30,"ttl_12h_tokens":30}}}}`,
	} {
		c = pricedFixture()
		CaptureUsage(&c, []byte(raw))
		if !c.UsageInvalid || estimateUsage(c, usagePrices).Status != "invalid_usage" {
			t.Fatal("invalid subsets accepted")
		}
	}
}
