package main

import (
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

// Everything here is synthetic. Amounts are exact decimals that the gallery
// formats itself, the way an owning page formats a read-model amount; the
// charts only receive the float64 position and the finished text.

var messages = charts.Messages{Empty: "No data in this range", Missing: "No value", Selected: "Selected"}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func plot(d decimal.Decimal) charts.Value { return charts.Num(d.InexactFloat64()) }

const minus = "\u2212"

// money formats an amount with grouping, two decimals and a typographic minus.
func money(d decimal.Decimal) string {
	sign := ""
	if d.IsNegative() {
		sign, d = minus, d.Neg()
	}
	whole, frac, _ := strings.Cut(d.StringFixed(2), ".")
	var groups []string
	for len(whole) > 3 {
		groups = append([]string{whole[len(whole)-3:]}, groups...)
		whole = whole[:len(whole)-3]
	}
	groups = append([]string{whole}, groups...)
	return sign + "$" + strings.Join(groups, ",") + "." + frac
}

func signed(d decimal.Decimal) string {
	if d.IsPositive() {
		return "+" + money(d)
	}
	return money(d)
}

func percent(d decimal.Decimal, places int32) string {
	text := d.StringFixed(places) + "%"
	switch {
	case d.IsPositive():
		return "+" + text
	case d.IsNegative():
		return minus + strings.TrimPrefix(text, "-")
	}
	return text
}

func compact(v float64) string {
	sign := ""
	if v < 0 {
		sign, v = minus, -v
	}
	switch {
	case v >= 1e6:
		return sign + "$" + trim(v/1e6) + "M"
	case v >= 1e3:
		return sign + "$" + trim(v/1e3) + "k"
	}
	return sign + "$" + trim(v)
}

func trim(v float64) string {
	text := decimal.NewFromFloat(math.Round(v*10) / 10).String()
	return text
}

func civil(t time.Time) string { return t.UTC().Format("Jan 2, 2006") }

var epoch = time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC)

// wealthHistory is a weekly net worth, assets and liabilities history with one
// stretch of missing liabilities and a provisional latest asset value.
func wealthHistory() charts.LineSpec {
	const weeks = 78
	var net, assets, liabilities charts.LineSeries
	net = charts.LineSeries{ID: "net-worth", Name: "Net worth"}
	assets = charts.LineSeries{ID: "assets", Name: "Assets"}
	liabilities = charts.LineSeries{ID: "liabilities", Name: "Liabilities"}
	for week := range weeks {
		day := epoch.AddDate(0, 0, 7*week)
		id := charts.ID(day.Format("2006-01-02"))
		a := decimal.NewFromFloat(1150000 + 6500*float64(week) + 18000*math.Sin(float64(week)/5)).Round(2)
		l := decimal.NewFromFloat(410000 - 2100*float64(week)).Round(2)
		n := a.Sub(l)
		assets.Points = append(assets.Points, charts.LinePoint{ID: id, Time: day, Value: plot(a), Text: money(a),
			Detail: []string{"Source: daily valuation"}, Provisional: week == weeks-1})
		if week == weeks-1 {
			assets.Points[len(assets.Points)-1].Detail = []string{"Estimated: today's quotes are not final"}
		}
		lp := charts.LinePoint{ID: id, Time: day, Value: plot(l), Text: money(l), Detail: []string{"Source: loan statements"}}
		if week >= 30 && week <= 33 {
			lp = charts.LinePoint{ID: id, Time: day, Text: "No statement for this week"}
		}
		liabilities.Points = append(liabilities.Points, lp)
		net.Points = append(net.Points, charts.LinePoint{ID: id, Time: day, Value: plot(n), Text: money(n),
			Detail: []string{"Assets minus liabilities"}, Provisional: week == weeks-1})
	}
	return charts.LineSpec{
		Label:       "Wealth history",
		Summary:     "Net worth, assets and liabilities by week, June 2025 to November 2026.",
		Series:      []charts.LineSeries{net, assets, liabilities},
		FormatTime:  civil,
		FormatTick:  func(t time.Time) string { return t.UTC().Format("Jan 2006") },
		FormatValue: compact,
		Messages:    messages,
	}
}

// returnTrend is a monthly return series with a true zero, negative values,
// two missing stretches, an isolated point, a declared break and an estimate.
func returnTrend() charts.LineSpec {
	values := []string{"1.20", "-0.80", "0.00", "2.40", "", "", "-3.10", "", "0.60", "1.90", "2.70", "1.10"}
	series := charts.LineSeries{ID: "return", Name: "Monthly return"}
	for i, v := range values {
		day := time.Date(2025, time.September+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		p := charts.LinePoint{ID: charts.ID(day.Format("2006-01")), Time: day}
		if v == "" {
			p.Text = "No valuation for this month"
		} else {
			d := dec(v)
			p.Value, p.Text = plot(d), percent(d, 2)
		}
		switch i {
		case 9:
			p.BreakBefore = true
			p.Detail = []string{"New valuation method starts here"}
		case 11:
			p.Provisional = true
			p.Detail = []string{"Estimated from month-to-date quotes"}
		}
		series.Points = append(series.Points, p)
	}
	return charts.LineSpec{
		Label:       "Return trend",
		Summary:     "Monthly return, September 2025 to August 2026, with gaps where no valuation exists.",
		Series:      []charts.LineSeries{series},
		IncludeZero: true,
		Area:        true,
		FormatTime:  func(t time.Time) string { return t.UTC().Format("Jan 2006") },
		FormatValue: func(v float64) string { return percent(decimal.NewFromFloat(v), 1) },
		Messages:    messages,
	}
}

func missingLine() charts.LineSpec {
	series := charts.LineSeries{ID: "quotes", Name: "Quotes"}
	for i := range 6 {
		day := epoch.AddDate(0, i, 0)
		series.Points = append(series.Points, charts.LinePoint{ID: charts.ID(day.Format("2006-01")), Time: day, Text: "No quote"})
	}
	return charts.LineSpec{Label: "Quotes", Summary: "No quotes exist in this range.", Series: []charts.LineSeries{series},
		FormatTime: civil, Messages: messages}
}

func emptyLine() charts.LineSpec {
	return charts.LineSpec{Label: "Wealth history", Summary: "No history yet.", Messages: messages}
}

type contribution struct {
	id, label, amount string
	tone              charts.Tone
	missing           bool
	detail            string
}

func bars(rows []contribution, symmetric bool) charts.BarSpec {
	spec := charts.BarSpec{
		Label:       "Contribution to return",
		Summary:     "Contribution of each holding to the return of the selected period.",
		Symmetric:   symmetric,
		FormatValue: func(v float64) string { return compact(v) },
		Messages:    messages,
	}
	for _, r := range rows {
		bar := charts.Bar{ID: charts.ID(r.id), Label: r.label, Tone: r.tone}
		if r.missing {
			bar.Text = "No quote"
		} else {
			d := dec(r.amount)
			bar.Value, bar.Text = plot(d), signed(d)
		}
		if r.detail != "" {
			bar.Detail = []string{r.detail}
		}
		spec.Bars = append(spec.Bars, bar)
	}
	return spec
}

func standardBars() charts.BarSpec {
	return bars([]contribution{
		{id: "tsmc", label: "Taiwan Semiconductor Manufacturing Company Limited ordinary shares, Taipei exchange", amount: "18420.55", detail: "Held in 2 accounts"},
		{id: "vwra", label: "Vanguard FTSE All-World UCITS ETF (USD) Accumulating", amount: "9310.00"},
		{id: "fx", label: "USD/SGD currency effect", amount: "-4120.40"},
		{id: "nvda", label: "NVIDIA", amount: "6002.18"},
		{id: "ladder", label: "Government bond ladder, 2027–2031 maturities, annual coupons reinvested", amount: "-2880.75", detail: "Coupon dates differ by account"},
		{id: "btc", label: "Bitcoin", amount: "12744.90"},
		{id: "fees", label: "Fees and taxes", amount: "-1150.00"},
		{id: "gold", label: "Gold", amount: "1432.10"},
	}, false)
}

func edgeBars() charts.BarSpec {
	return bars([]contribution{
		{id: "mmf", label: "Money market fund", amount: "0.00", detail: "Real zero, not a missing quote"},
		{id: "private", label: "Unlisted private fund with no quote in this period", missing: true},
		{id: "large", label: "Single-stock concentration in the employer share plan", amount: "-38210.00"},
		{id: "tiny", label: "Rounding on foreign dividends", amount: "0.03"},
		{id: "residual", label: "Unexplained residual", amount: "-1840.12", tone: charts.ToneWarning, detail: "Not a loss; caller-chosen tone"},
		{id: "bond", label: "Bond fund", amount: "9610.33"},
	}, true)
}

func missingBars() charts.BarSpec {
	return bars([]contribution{
		{id: "a", label: "Unlisted private fund", missing: true},
		{id: "b", label: "Delisted warrant", missing: true},
	}, false)
}

// waterfall builds steps from caller-supplied levels. The running level is
// computed here with decimals, as an owning read model would; the chart is
// given every level and never adds anything.
func standardWaterfall() charts.WaterfallSpec {
	type change struct {
		id, label, amount string
		tone              charts.Tone
	}
	changes := []change{
		{"contrib", "Contributions", "80000.00", 0},
		{"market", "Market gains", "120000.00", 0},
		{"dividend", "Dividends", "15250.40", 0},
		{"fx", "Currency effect", "-32400.00", 0},
		{"withdraw", "Withdrawals", "-60000.00", 0},
		{"fees", "Fees and taxes", "-4500.25", 0},
		{"residual", "Unexplained residual", "1512.35", charts.ToneWarning},
	}
	level := dec("1000000.00")
	spec := charts.WaterfallSpec{
		Label:       "Asset changes",
		Summary:     "Net worth from 1 January to 30 June 2026, by source of change.",
		FormatValue: compact,
		Messages:    messages,
		Steps: []charts.WaterfallStep{{ID: "begin", Label: "Beginning", Kind: charts.StepTotal, To: plot(level), Text: money(level),
			Detail: []string{"1 Jan 2026"}}},
	}
	for _, c := range changes {
		delta := dec(c.amount)
		next := level.Add(delta)
		spec.Steps = append(spec.Steps, charts.WaterfallStep{ID: charts.ID(c.id), Label: c.label, From: plot(level), To: plot(next),
			Text: signed(delta), Tone: c.tone})
		level = next
	}
	spec.Steps = append(spec.Steps, charts.WaterfallStep{ID: "end", Label: "Ending", Kind: charts.StepTotal, To: plot(level), Text: money(level),
		Detail: []string{"30 Jun 2026"}})
	return spec
}

// edgeWaterfall crosses zero, has a step whose levels are unknown and ends on a
// total that does not reconcile with the last change.
func edgeWaterfall() charts.WaterfallSpec {
	begin, loss, gain := dec("50000.00"), dec("-180000.00"), dec("20000.00")
	afterLoss := begin.Add(loss)
	afterGain := afterLoss.Add(gain)
	ending := dec("-105000.00")
	return charts.WaterfallSpec{
		Label:       "Asset changes",
		Summary:     "Net worth falls below zero; the reported ending does not match the sum of changes.",
		FormatValue: compact,
		Messages:    messages,
		Steps: []charts.WaterfallStep{
			{ID: "begin", Label: "Beginning", Kind: charts.StepTotal, To: plot(begin), Text: money(begin)},
			{ID: "loss", Label: "Market loss", From: plot(begin), To: plot(afterLoss), Text: signed(loss)},
			{ID: "pending", Label: "Valuation pending for one account", Text: "Levels unknown"},
			{ID: "gain", Label: "Dividends", From: plot(afterLoss), To: plot(afterGain), Text: signed(gain)},
			{ID: "end", Label: "Ending as reported", Kind: charts.StepTotal, To: plot(ending), Text: money(ending),
				Detail: []string{"Differs from the sum of changes by " + money(ending.Sub(afterGain))}},
		},
	}
}

func missingWaterfall() charts.WaterfallSpec {
	return charts.WaterfallSpec{
		Label: "Asset changes", Summary: "No levels are available.", Messages: messages,
		Steps: []charts.WaterfallStep{
			{ID: "begin", Label: "Beginning", Kind: charts.StepTotal, Text: "No value"},
			{ID: "end", Label: "Ending", Kind: charts.StepTotal, Text: "No value"},
		},
	}
}

type slice struct {
	id, label, amount string
	missing           bool
}

func donut(rows []slice, centerCaption, notDrawn, zero string, center decimal.Decimal) charts.DonutSpec {
	total := decimal.Zero
	for _, r := range rows {
		if r.missing {
			continue
		}
		if d := dec(r.amount); d.IsPositive() {
			total = total.Add(d)
		}
	}
	spec := charts.DonutSpec{
		Label:         "Composition",
		Summary:       "Share of each category in the total.",
		CenterValue:   money(center),
		CenterCaption: centerCaption,
		NotDrawn:      notDrawn,
		ZeroTotal:     zero,
		Messages:      messages,
	}
	for _, r := range rows {
		s := charts.Slice{ID: charts.ID(r.id), Label: r.label}
		if r.missing {
			s.Text = "No value"
			spec.Slices = append(spec.Slices, s)
			continue
		}
		d := dec(r.amount)
		s.Value, s.Text = plot(d), money(d)
		switch {
		case d.IsPositive() && total.IsPositive():
			s.Share = d.Div(total).Mul(dec("100")).StringFixed(1) + "%"
		case d.IsZero():
			s.Share = "0.0%"
		}
		spec.Slices = append(spec.Slices, s)
	}
	return spec
}

func standardDonut() charts.DonutSpec {
	total := dec("1280485.60")
	return donut([]slice{
		{id: "cash", label: "Cash and deposits", amount: "182400.55"},
		{id: "stocks", label: "Listed stocks", amount: "412980.10"},
		{id: "funds", label: "Funds and ETFs", amount: "301220.00"},
		{id: "bonds", label: "Bonds", amount: "96300.75"},
		{id: "property", label: "Real estate", amount: "250000.00"},
		{id: "crypto", label: "Digital assets", amount: "31404.20"},
		{id: "gold", label: "Gold", amount: "4200.00"},
		{id: "other", label: "Other", amount: "1980.00"},
	}, "Total assets", "", "", total)
}

// edgeDonut lists liabilities with a negative balance, a real zero and a missing
// value. Only the first two positive entries get an arc.
func edgeDonut() charts.DonutSpec {
	return donut([]slice{
		{id: "mortgage", label: "Mortgage, 20-year fixed rate with a long product name that must wrap", amount: "412300.00"},
		{id: "auto", label: "Auto loan", amount: "18900.00"},
		{id: "card", label: "Credit card in credit", amount: "-1250.00"},
		{id: "tax", label: "Tax payable", amount: "0.00"},
		{id: "student", label: "Student loan", missing: true},
	}, "Total liabilities", "Not shown in the ring", "", dec("429950.00"))
}

func zeroDonut() charts.DonutSpec {
	return donut([]slice{
		{id: "cash", label: "Cash and deposits", amount: "0.00"},
		{id: "stocks", label: "Listed stocks", missing: true},
	}, "Total assets", "", "Nothing to draw", dec("0.00"))
}

func emptyDonut() charts.DonutSpec {
	return charts.DonutSpec{Label: "Composition", Summary: "No categories.", Messages: messages}
}
