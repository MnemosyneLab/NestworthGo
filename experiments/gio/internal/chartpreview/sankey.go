// Package chartpreview builds synthetic datasets for the Gio chart previews.
package chartpreview

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

// Name identifies a demo entity independently of its display text.
type Name struct {
	ID, Label, Detail string
}

func (n Name) details() []string {
	if n.Detail == "" {
		return nil
	}
	return []string{n.Detail}
}

// Holding is an allocation of one synthetic account. An empty amount is unknown.
type Holding struct {
	Class, Institution, Account Name
	Amount                      string
}

const (
	nodeClass = iota
	nodeInstitution
	nodeAccount
	linkTo
	nodeTotal
)

func pathID(kind int, parts ...string) string {
	segments := []string{strconv.Itoa(kind)}
	for _, part := range parts {
		segments = append(segments, url.PathEscape(part))
	}
	return strings.Join(segments, "/")
}

// Sankey builds a four-column synthetic allocation with an independent node
// for each account path. Exact decimal sums are formatted by the caller;
// floats are used for the marks. Undrawable allocations remain in Rows.
func Sankey(spec charts.SankeySpec, rootLabel string, rows []Holding, money func(decimal.Decimal) string) charts.SankeySpec {
	positive := func(row Holding) bool {
		return row.Amount != "" && decimal.RequireFromString(row.Amount).IsPositive()
	}
	classID := func(row Holding) string { return pathID(nodeClass, row.Class.ID) }
	institutionID := func(row Holding) string {
		return pathID(nodeInstitution, row.Class.ID, row.Institution.ID)
	}
	sums := map[string]decimal.Decimal{}
	total := decimal.Zero
	for _, row := range rows {
		if positive(row) {
			amount := decimal.RequireFromString(row.Amount)
			total = total.Add(amount)
			sums[classID(row)] = sums[classID(row)].Add(amount)
			sums[institutionID(row)] = sums[institutionID(row)].Add(amount)
		}
	}
	root := pathID(nodeTotal)
	spec.Nodes = append(spec.Nodes, charts.SankeyNode{ID: charts.ID(root), Label: rootLabel, Text: money(total), HideLabel: true})
	seen := map[string]bool{}
	add := func(id string, name Name, parent string) {
		if seen[id] {
			return
		}
		seen[id] = true
		amount := sums[id]
		spec.Nodes = append(spec.Nodes, charts.SankeyNode{ID: charts.ID(id), Label: name.Label, Text: money(amount), Detail: name.details()})
		spec.Links = append(spec.Links, charts.SankeyLink{ID: charts.ID(pathID(linkTo, id)), Source: charts.ID(parent), Target: charts.ID(id),
			Value: charts.Num(amount.InexactFloat64()), Text: money(amount)})
	}
	for _, row := range rows {
		parent := institutionID(row)
		if positive(row) {
			add(classID(row), row.Class, root)
			add(parent, row.Institution, classID(row))
		} else {
			parent = root
		}
		id := pathID(nodeAccount, row.Class.ID, row.Institution.ID, row.Account.ID)
		text := spec.Messages.Missing
		link := charts.SankeyLink{ID: charts.ID(pathID(linkTo, id)), Source: charts.ID(parent), Target: charts.ID(id)}
		if row.Amount != "" {
			amount := decimal.RequireFromString(row.Amount)
			text = money(amount)
			link.Value = charts.Num(amount.InexactFloat64())
		}
		link.Text = text
		detail := row.Account.details()
		if row.Institution.Detail != "" {
			detail = append(detail, row.Institution.Detail)
		}
		link.Detail = detail
		spec.Nodes = append(spec.Nodes, charts.SankeyNode{ID: charts.ID(id), Label: row.Account.Label, Text: text, Detail: detail})
		spec.Links = append(spec.Links, link)
	}
	return spec
}

// DenseHoldings has 24 distinct positive account paths. Both Harbor
// institutions have the same display name, and the same reserve account
// appears in cash and funds under its original institution.
func DenseHoldings(classLabels [4]string) []Holding {
	cash := Name{ID: "cash", Label: classLabels[0]}
	stocks := Name{ID: "stocks", Label: classLabels[1]}
	funds := Name{ID: "funds", Label: classLabels[2]}
	bonds := Name{ID: "bonds", Label: classLabels[3]}
	harborNorth := Name{ID: "harbor-north", Label: "Harbor National Bank", Detail: "North Branch"}
	harborSouth := Name{ID: "harbor-south", Label: "Harbor National Bank", Detail: "South Branch"}
	summit := Name{ID: "summit", Label: "Summit Brokerage and International Custody Services"}
	westlake := Name{ID: "westlake", Label: "Westlake Securities"}
	pacific := Name{ID: "pacific", Label: "Pacific Asset Management"}
	evergreen := Name{ID: "evergreen", Label: "Evergreen Credit Union"}
	reserve := Name{ID: "reserve", Label: "Household Reserve and Long-Term Capital Preservation Account"}
	return []Holding{
		{cash, harborNorth, reserve, "845200.55"},
		{cash, harborNorth, Name{ID: "north-savings", Label: "Savings"}, "52800.60"},
		{cash, harborNorth, Name{ID: "north-sweep", Label: "Cash Sweep"}, "0.01"},
		{cash, harborSouth, Name{ID: "south-savings", Label: "Savings"}, "24100.75"},
		{cash, harborSouth, Name{ID: "south-sweep", Label: "Cash Sweep"}, "0.04"},
		{cash, harborSouth, Name{ID: "call-deposit", Label: "Thirty-Day Notice Deposit"}, "75100.34"},
		{stocks, summit, Name{ID: "growth", Label: "International Growth and Emerging Markets Portfolio"}, "1250400.75"},
		{stocks, summit, Name{ID: "employee-plan", Label: "Employee Share Plan"}, "20410.29"},
		{stocks, summit, Name{ID: "dividend", Label: "Dividend Portfolio"}, "31010.31"},
		{stocks, westlake, Name{ID: "domestic", Label: "Domestic Equity"}, "84120.08"},
		{stocks, westlake, Name{ID: "small-cap", Label: "Small-Cap Shares"}, "32447.59"},
		{stocks, westlake, Name{ID: "employee-plan-2", Label: "Employee Share Plan"}, "2155.02"},
		{funds, harborNorth, reserve, "11229.41"},
		{funds, harborNorth, Name{ID: "global-index", Label: "Global Index Fund"}, "45090.23"},
		{funds, harborNorth, Name{ID: "all-cap", Label: "All-Cap Index Fund"}, "28011.44"},
		{funds, pacific, Name{ID: "commodities", Label: "Broad Commodities ETF"}, "14025.18"},
		{funds, pacific, Name{ID: "small-index", Label: "Small-Cap Index Fund"}, "833.43"},
		{funds, pacific, Name{ID: "core-index", Label: "Core Index Tracker"}, "111.93"},
		{bonds, pacific, Name{ID: "short-duration", Label: "Short-Duration Government Bond Portfolio"}, "86275.61"},
		{bonds, pacific, Name{ID: "bonds-international", Label: "International Bonds"}, "11429.81"},
		{bonds, pacific, Name{ID: "indexed-bonds", Label: "Inflation-Linked Bonds"}, "13841.57"},
		{bonds, evergreen, Name{ID: "bond-ladder", Label: "Five-Year Bond Ladder"}, "51000.48"},
		{bonds, evergreen, Name{ID: "treasury", Label: "Treasury Notes"}, "21700.82"},
		{bonds, evergreen, Name{ID: "bond-reinvestment", Label: "Municipal Bond Reinvestment Account"}, "0.02"},
	}
}
