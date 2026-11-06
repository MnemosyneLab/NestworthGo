package assistant

import (
	"embed"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Price files are versioned observations, not a model availability list. Expiry
// is our review deadline, not a promise that provider prices cannot change.
//
//go:embed pricing/*.json
var priceFiles embed.FS

type priceRates struct {
	Model                string `json:"model"`
	Input                string `json:"input"`
	Cached               string `json:"cached"`
	Write30m             string `json:"write30m"`
	Write12h             string `json:"write12h"`
	Output               string `json:"output"`
	PerMinute            string `json:"perMinute"`
	LongContextThreshold int64  `json:"longContextThreshold"`
}
type priceCatalogue struct {
	Version    string       `json:"version"`
	Source     string       `json:"source"`
	VerifiedAt string       `json:"verifiedAt"`
	ValidFrom  time.Time    `json:"validFrom"`
	ValidUntil time.Time    `json:"validUntil"`
	Models     []priceRates `json:"models"`
}

// PriceSnapshot freezes the exact applied public USD rates with each call.
// Updating the catalogue never recalculates saved estimates.
type PriceSnapshot struct {
	Version     string    `json:"version"`
	Source      string    `json:"source"`
	VerifiedAt  string    `json:"verifiedAt"`
	ValidFrom   time.Time `json:"validFrom"`
	ValidUntil  time.Time `json:"validUntil"`
	Model       string    `json:"model"`
	ModelBasis  string    `json:"modelBasis"`
	ServiceTier string    `json:"serviceTier"`
	Currency    string    `json:"currency"`
	Input       string    `json:"input"`
	Cached      string    `json:"cached"`
	Write30m    string    `json:"write30m"`
	Write12h    string    `json:"write12h"`
	Output      string    `json:"output"`
	PerMinute   string    `json:"perMinute"`
	LongContext bool      `json:"longContext"`
}
type UsageCost struct {
	Status        string         `json:"status"`
	USD           string         `json:"usd"`
	CachedUSD     string         `json:"cachedUsd"`
	CacheWriteUSD string         `json:"cacheWriteUsd"`
	Price         *PriceSnapshot `json:"price"`
}

func readPriceCatalogues() []priceCatalogue {
	files, err := priceFiles.ReadDir("pricing")
	if err != nil {
		return nil
	}
	var result []priceCatalogue
	for _, file := range files {
		raw, e := priceFiles.ReadFile("pricing/" + file.Name())
		var c priceCatalogue
		if e != nil || jsoncodec.Unmarshal(raw, &c) != nil || !c.ValidUntil.After(c.ValidFrom) {
			return nil
		}
		result = append(result, c)
	}
	return result
}

var usagePrices = readPriceCatalogues()

func estimateUsage(c UsageCall, catalogues []priceCatalogue) *UsageCost {
	unknown := func(reason string) *UsageCost { return &UsageCost{Status: reason} }
	if c.Status == "pending" {
		return unknown("pending")
	}
	if c.PricingScope != "openai-public" {
		return unknown("unknown_endpoint")
	}
	if c.UsageInvalid {
		return unknown("invalid_usage")
	}
	model, basis := c.Model, "response"
	if c.Kind == "transcription" && model == "" {
		model, basis = c.RequestedModel, "requested"
	}
	if model == "" {
		return unknown("unknown_model")
	}
	var matched *PriceSnapshot
	var rates priceRates
	for _, catalogue := range catalogues {
		if c.Started.Before(catalogue.ValidFrom) || !c.Started.Before(catalogue.ValidUntil) {
			continue
		}
		for _, rate := range catalogue.Models {
			if rate.Model == model {
				if matched != nil {
					return unknown("ambiguous_price")
				}
				rates = rate
				matched = &PriceSnapshot{Version: catalogue.Version, Source: catalogue.Source, VerifiedAt: catalogue.VerifiedAt, ValidFrom: catalogue.ValidFrom, ValidUntil: catalogue.ValidUntil, Model: model, ModelBasis: basis, ServiceTier: c.ServiceTier, Currency: "USD", Input: rate.Input, Cached: rate.Cached, Write30m: rate.Write30m, Write12h: rate.Write12h, Output: rate.Output, PerMinute: rate.PerMinute}
			}
		}
	}
	if matched == nil {
		return unknown("unknown_price")
	}
	parse := func(value string) (decimal.Decimal, bool) {
		if len(value) == 0 || len(value) > 32 {
			return decimal.Zero, false
		}
		d, err := decimal.NewFromString(value)
		return d, err == nil && !d.IsNegative() && d.Exponent() >= -9 && d.Exponent() <= 6
	}
	if rates.PerMinute != "" {
		if c.Kind != "transcription" || c.AudioSeconds == nil {
			return unknown("missing_duration")
		}
		seconds, ok := parse(*c.AudioSeconds)
		if !ok {
			return unknown("invalid_usage")
		}
		rate, ok := parse(rates.PerMinute)
		if !ok {
			return unknown("invalid_price")
		}
		// The published decimal rate is divisible by 60 exactly; no binary float.
		value := seconds.Mul(rate).Div(decimal.NewFromInt(60))
		return &UsageCost{Status: "estimated", USD: value.String(), CachedUSD: "0", CacheWriteUSD: "0", Price: matched}
	}
	// Only verified Standard public endpoint rates. Do not guess a tier from the
	// requested default/auto setting or use a Standard rate for Fast/Flex/Scale.
	if c.ServiceTier != "default" {
		return unknown("unknown_tier")
	}
	if c.Input == nil || c.Output == nil || c.CachedInput == nil || c.CacheWrite == nil {
		return unknown("missing_usage")
	}
	if *c.CachedInput+*c.CacheWrite > *c.Input {
		return unknown("invalid_usage")
	}
	write30, write12 := int64(0), int64(0)
	if *c.CacheWrite > 0 {
		if c.CacheWrite30m == nil || c.CacheWrite12h == nil {
			return unknown("missing_cache_lifetime")
		}
		write30, write12 = *c.CacheWrite30m, *c.CacheWrite12h
		if write30+write12 != *c.CacheWrite {
			return unknown("invalid_usage")
		}
	}
	input, ok1 := parse(rates.Input)
	cached, ok2 := parse(rates.Cached)
	w30, ok3 := parse(rates.Write30m)
	w12, ok4 := parse(rates.Write12h)
	output, ok5 := parse(rates.Output)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return unknown("invalid_price")
	}
	if rates.LongContextThreshold > 0 && *c.Input > rates.LongContextThreshold {
		matched.LongContext = true
		input = input.Mul(decimal.NewFromInt(2))
		cached = cached.Mul(decimal.NewFromInt(2))
		w30 = w30.Mul(decimal.NewFromInt(2))
		w12 = w12.Mul(decimal.NewFromInt(2))
		output = output.Mul(decimal.RequireFromString("1.5"))
	}
	matched.Input = input.String()
	matched.Cached = cached.String()
	matched.Write30m = w30.String()
	matched.Write12h = w12.String()
	matched.Output = output.String()
	perMillion := func(n int64, rate decimal.Decimal) decimal.Decimal { return decimal.NewFromInt(n).Mul(rate).Shift(-6) }
	ordinary := perMillion(*c.Input-*c.CachedInput-*c.CacheWrite, input)
	cacheCost := perMillion(*c.CachedInput, cached)
	writeCost := perMillion(write30, w30).Add(perMillion(write12, w12))
	amount := ordinary.Add(cacheCost).Add(writeCost).Add(perMillion(*c.Output, output))
	return &UsageCost{Status: "estimated", USD: amount.String(), CachedUSD: cacheCost.String(), CacheWriteUSD: writeCost.String(), Price: matched}
}

// Validate frozen snapshots without consulting today's catalogue or repricing.
func validUsageCost(c *UsageCost) bool {
	if c == nil {
		return true
	}
	if c.Status != "estimated" {
		switch c.Status {
		case "pending", "unknown_endpoint", "invalid_usage", "unknown_model", "ambiguous_price", "unknown_price", "missing_duration", "invalid_price", "unknown_tier", "missing_usage", "missing_cache_lifetime":
			return c.Price == nil && c.USD == "" && c.CachedUSD == "" && c.CacheWriteUSD == ""
		}
		return false
	}
	p := c.Price
	if p == nil || p.Currency != "USD" || p.Source != "https://developers.openai.com/api/docs/pricing" || len(p.Version) > 80 || p.Version == "" || !p.ValidUntil.After(p.ValidFrom) {
		return false
	}
	if _, err := time.Parse("2006-01-02", p.VerifiedAt); err != nil {
		return false
	}
	if !(ModelConfig{Provider: "openai-responses", Model: p.Model}).Valid() || (p.ModelBasis != "response" && p.ModelBasis != "requested") {
		return false
	}
	valid := func(s string) bool {
		if len(s) == 0 || len(s) > 64 {
			return false
		}
		d, e := decimal.NewFromString(s)
		return e == nil && !d.IsNegative() && d.Exponent() >= -24 && d.Exponent() <= 6
	}
	for _, s := range []string{c.USD, c.CachedUSD, c.CacheWriteUSD} {
		if !valid(s) {
			return false
		}
	}
	for _, s := range []string{p.Input, p.Cached, p.Write30m, p.Write12h, p.Output, p.PerMinute} {
		if s != "" && !valid(s) {
			return false
		}
	}
	total, _ := decimal.NewFromString(c.USD)
	cached, _ := decimal.NewFromString(c.CachedUSD)
	write, _ := decimal.NewFromString(c.CacheWriteUSD)
	return cached.Add(write).LessThanOrEqual(total)
}
