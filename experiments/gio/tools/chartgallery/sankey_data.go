package main

import (
	"github.com/waltwang/nestworth-go/experiments/gio/internal/chartpreview"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

func sankeyFlows(rows []chartpreview.Holding) charts.SankeySpec {
	spec := charts.SankeySpec{
		Label:        "Asset composition",
		Summary:      "Where the assets are held, from asset class to institution and account.",
		ColumnTitles: []string{"Total", "Asset class", "Institution", "Account"},
		Messages:     messages,
	}
	return chartpreview.Sankey(spec, "Total assets", rows, money)
}

func name(id, label string) chartpreview.Name { return chartpreview.Name{ID: id, Label: label} }

func standardSankey() charts.SankeySpec {
	cash, stocks := name("cash", "Cash and deposits"), name("stocks", "Listed stocks")
	funds := name("funds", "Funds and ETFs")
	harbor, summit := name("harbor", "Harbor Bank"), name("summit", "Summit Brokerage")
	return sankeyFlows([]chartpreview.Holding{
		{Class: cash, Institution: harbor, Account: name("checking", "Everyday checking"), Amount: "52400.55"},
		{Class: cash, Institution: harbor, Account: name("emergency", "Emergency savings"), Amount: "98000.00"},
		{Class: cash, Institution: name("northern", "Northern Credit Union"), Account: name("savings", "Savings"), Amount: "32000.00"},
		{Class: stocks, Institution: summit, Account: name("growth", "Growth portfolio"), Amount: "312980.10"},
		{Class: stocks, Institution: summit, Account: name("retirement", "Retirement account"), Amount: "100000.00"},
		{Class: funds, Institution: summit, Account: name("index", "Index funds"), Amount: "201220.00"},
		{Class: funds, Institution: name("pacific", "Pacific Asset Management"), Account: name("global-bond", "Global bond fund"), Amount: "100000.00"},
		{Class: name("real-estate", "Real estate"), Institution: name("self-held", "Self-held"), Account: name("apartment", "Apartment"), Amount: "250000.00"},
		{Class: name("digital", "Digital assets"), Institution: name("coinline", "Coinline"), Account: name("wallet-1", "Wallet"), Amount: "31404.20"},
	})
}

func edgeSankey() charts.SankeySpec {
	cash, stocks := name("cash", "Cash and deposits"), name("stocks", "Listed stocks")
	national := name("first-national", "First National Bank of a Very Long Institution Name")
	summit := name("summit", "Summit Brokerage")
	return sankeyFlows([]chartpreview.Holding{
		{Class: cash, Institution: national, Account: name("market", "Premium money market account with tiered interest and a long product name"), Amount: "845200.00"},
		{Class: cash, Institution: national, Account: name("petty", "Petty cash"), Amount: "12.40"},
		{Class: cash, Institution: name("on-hand", "Cash on hand"), Account: name("wallet-cash", "Wallet"), Amount: "0.00"},
		{Class: stocks, Institution: summit, Account: name("growth", "Growth portfolio"), Amount: "1250400.75"},
		{Class: stocks, Institution: summit, Account: name("employer", "Employer plan")},
		{Class: name("digital", "Digital assets"), Institution: name("coinline", "Coinline"), Account: name("wallet-digital", "Wallet"), Amount: "1.05"},
	})
}

func missingSankey() charts.SankeySpec {
	return sankeyFlows([]chartpreview.Holding{
		{Class: name("cash", "Cash and deposits"), Institution: name("harbor", "Harbor Bank"), Account: name("checking", "Everyday checking")},
		{Class: name("stocks", "Listed stocks"), Institution: name("summit", "Summit Brokerage"), Account: name("growth", "Growth portfolio"), Amount: "0.00"},
	})
}

func emptySankey() charts.SankeySpec {
	return charts.SankeySpec{Label: "Asset composition", Summary: "No assets.", Messages: messages}
}

func denseSankey() charts.SankeySpec {
	return sankeyFlows(chartpreview.DenseHoldings([4]string{"Cash and deposits", "Listed stocks", "Funds and ETFs", "Bonds"}))
}
