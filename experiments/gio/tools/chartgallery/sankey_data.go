package main

import (
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

// holding is one account's allocation in one asset class. An empty amount is a
// value the ledger could not determine.
type holding struct {
	class, institution, account, amount string
}

// sankeyFlows builds an asset composition the way an owning page would from its
// read model: the page sums exact decimals for the node texts, and the chart
// only receives float positions for the geometry. Unknown and non-positive
// amounts keep their links so the text rows still list them.
func sankeyFlows(rows []holding, total string) charts.SankeySpec {
	spec := charts.SankeySpec{
		Label:        "Asset composition",
		Summary:      "Where the assets are held, from asset class to institution and account.",
		ColumnTitles: []string{"Total", "Asset class", "Institution", "Account"},
		Messages:     messages,
	}
	amountOf := func(r holding) (decimal.Decimal, bool) {
		if r.amount == "" {
			return decimal.Zero, false
		}
		return dec(r.amount), true
	}
	sums := map[string]decimal.Decimal{}
	for _, r := range rows {
		if d, ok := amountOf(r); ok && d.IsPositive() {
			sums["class/"+r.class] = sums["class/"+r.class].Add(d)
			sums["inst/"+r.class+"/"+r.institution] = sums["inst/"+r.class+"/"+r.institution].Add(d)
		}
	}
	spec.Nodes = append(spec.Nodes, charts.SankeyNode{ID: "total", Label: "Total assets", Text: total, HideLabel: true})
	seen := map[string]bool{}
	node := func(id, label string, amount decimal.Decimal, link string, parent string) {
		if !seen[id] {
			seen[id] = true
			spec.Nodes = append(spec.Nodes, charts.SankeyNode{ID: charts.ID(id), Label: label, Text: money(amount)})
			spec.Links = append(spec.Links, charts.SankeyLink{
				ID: charts.ID(link), Source: charts.ID(parent), Target: charts.ID(id), Value: plot(amount), Text: money(amount),
			})
		}
	}
	for _, r := range rows {
		d, known := amountOf(r)
		if known && d.IsPositive() {
			node("class/"+r.class, r.class, sums["class/"+r.class], "flow/class/"+r.class, "total")
			node("inst/"+r.class+"/"+r.institution, r.institution, sums["inst/"+r.class+"/"+r.institution],
				"flow/inst/"+r.class+"/"+r.institution, "class/"+r.class)
		}
		account := "acct/" + r.class + "/" + r.institution + "/" + r.account
		parent := "inst/" + r.class + "/" + r.institution
		if !known || !d.IsPositive() {
			// Without a positive amount the account has no parent flow worth
			// drawing; it hangs from the total with a text-only link.
			parent = "total"
		}
		text := "No value"
		if known {
			text = money(d)
		}
		spec.Nodes = append(spec.Nodes, charts.SankeyNode{ID: charts.ID(account), Label: r.account, Text: text})
		link := charts.SankeyLink{ID: charts.ID("flow/" + account), Source: charts.ID(parent), Target: charts.ID(account), Text: text}
		if known {
			link.Value = plot(d)
		}
		spec.Links = append(spec.Links, link)
	}
	return spec
}

func standardSankey() charts.SankeySpec {
	return sankeyFlows([]holding{
		{"Cash and deposits", "Harbor Bank", "Everyday checking", "52400.55"},
		{"Cash and deposits", "Harbor Bank", "Emergency savings", "98000.00"},
		{"Cash and deposits", "Northern Credit Union", "Savings", "32000.00"},
		{"Listed stocks", "Summit Brokerage", "Growth portfolio", "312980.10"},
		{"Listed stocks", "Summit Brokerage", "Retirement account", "100000.00"},
		{"Funds and ETFs", "Summit Brokerage", "Index funds", "201220.00"},
		{"Funds and ETFs", "Pacific Asset Management", "Global bond fund", "100000.00"},
		{"Real estate", "Self-held", "Apartment", "250000.00"},
		{"Digital assets", "Coinline", "Wallet", "31404.20"},
	}, "$1,178,004.85")
}

// edgeSankey adds long names, a tiny flow beside very large ones, and accounts
// with a zero or an unknown amount that cannot be drawn.
func edgeSankey() charts.SankeySpec {
	return sankeyFlows([]holding{
		{"Cash and deposits", "First National Bank of a Very Long Institution Name", "Premium money market account with tiered interest and a long product name", "845200.00"},
		{"Cash and deposits", "First National Bank of a Very Long Institution Name", "Petty cash", "12.40"},
		{"Cash and deposits", "Cash on hand", "Wallet", "0.00"},
		{"Listed stocks", "Summit Brokerage", "Growth portfolio", "1250400.75"},
		{"Listed stocks", "Summit Brokerage", "Employer plan", ""},
		{"Digital assets", "Coinline", "Wallet", "1.05"},
	}, "$2,095,614.20")
}

func missingSankey() charts.SankeySpec {
	return sankeyFlows([]holding{
		{"Cash and deposits", "Harbor Bank", "Everyday checking", ""},
		{"Listed stocks", "Summit Brokerage", "Growth portfolio", "0.00"},
	}, "$0.00")
}

func emptySankey() charts.SankeySpec {
	return charts.SankeySpec{Label: "Asset composition", Summary: "No assets.", Messages: messages}
}
