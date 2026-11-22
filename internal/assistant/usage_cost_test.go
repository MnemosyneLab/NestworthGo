package assistant

import (
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
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
func TestUsagePriceCatalogueRollover(t *testing.T) {
	boundary := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		kind, model, usd string
	}{
		{"conversation", "gpt-6-astra", "0.01625"},
		{"conversation", "gpt-6.1-sol", "0.0032"},
		{"conversation", "gpt-6.1-terra", "0.0016"},
		{"title", "gpt-6-luna", "0.0001625"},
		{"transcription", "gpt-transcribe", "0.000925875"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			c := pricedFixture()
			c.Kind, c.Model = tc.kind, tc.model
			if c.Kind == "transcription" {
				seconds := "12.345"
				c.Model, c.RequestedModel, c.AudioSeconds = "", tc.model, &seconds
			}
			// The request start, not its later completion, determines the version.
			finished := boundary.Add(time.Second)
			c.Finished = &finished
			c.Started = boundary.Add(-time.Nanosecond)
			old := estimateUsage(c, usagePrices)
			if old.Status != "estimated" || old.USD != tc.usd || old.Price == nil ||
				old.Price.Version != "openai-public-2026-11-06.v1" || !old.Price.ValidUntil.Equal(boundary) {
				t.Fatalf("before rollover: %+v, price %+v", old, old.Price)
			}
			c.Started = boundary
			next := estimateUsage(c, usagePrices)
			if next.Status != "estimated" || next.USD != tc.usd || next.Price == nil ||
				next.Price.Version != "openai-public-2026-12-01.v1" || !next.Price.ValidFrom.Equal(boundary) {
				t.Fatalf("at rollover: %+v, price %+v", next, next.Price)
			}
		})
	}
}

func TestHistoricalUsageSnapshotSurvivesCatalogueRollover(t *testing.T) {
	c := pricedFixture()
	c.Cost = estimateUsage(c, usagePrices)
	if c.Cost.Price == nil || c.Cost.Price.Version != "openai-public-2026-11-06.v1" {
		t.Fatalf("expected historical snapshot: %+v", c.Cost)
	}
	// The store persists this JSON payload; reports read its frozen amount even
	// when a subsequently loaded catalogue can no longer price this call.
	raw, err := jsoncodec.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var restored UsageCall
	if err := jsoncodec.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !validUsageCost(restored.Cost) || !reflect.DeepEqual(restored.Cost, c.Cost) {
		t.Fatalf("historical snapshot changed: %+v", restored.Cost)
	}
	var successor []priceCatalogue
	for _, catalogue := range usagePrices {
		if catalogue.Version == "openai-public-2026-12-01.v1" {
			successor = append(successor, catalogue)
		}
	}
	if len(successor) != 1 || estimateUsage(restored, successor).Status != "unknown_price" {
		t.Fatal("successor unexpectedly covers the historical call")
	}
	var total UsageTotal
	total.add(restored)
	if total.EstimatedUSD != "0.0032" || total.PricedCalls != 1 || total.UnpricedCalls != 0 {
		t.Fatalf("historical total changed: %+v", total)
	}
}

func TestCostUnknownConditions(t *testing.T) {
	for name, change := range map[string]func(*UsageCall){
		"unknown_model":          func(c *UsageCall) { c.Model = "" },
		"unknown_tier":           func(c *UsageCall) { c.ServiceTier = "priority" },
		"unknown_endpoint":       func(c *UsageCall) { c.PricingScope = "" },
		"missing_usage":          func(c *UsageCall) { c.Output = nil },
		"missing_cache_lifetime": func(c *UsageCall) { c.CacheWrite30m = nil },
		"unknown_price":          func(c *UsageCall) { c.Started = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) },
		"invalid_usage":          func(c *UsageCall) { n := int64(600); c.CacheWrite = &n },
	} {
		t.Run(name, func(t *testing.T) {
			c := pricedFixture()
			c.Started = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
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
