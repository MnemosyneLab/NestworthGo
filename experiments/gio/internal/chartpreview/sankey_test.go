package chartpreview

import (
	"slices"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

func TestDenseFixturePreservesAllPathsAndExactValues(t *testing.T) {
	rows := DenseHoldings([4]string{"Cash", "Stocks", "Funds", "Bonds"})
	format := func(d decimal.Decimal) string { return d.StringFixed(2) }
	spec := Sankey(charts.SankeySpec{}, "Total", rows, format)
	if problems := spec.Validate(); len(problems) != 0 {
		t.Fatalf("invalid dense fixture: %v", problems)
	}
	if len(rows) != 24 || len(spec.Nodes) != 37 || len(spec.Links) != 36 {
		t.Fatalf("dense fixture has %d holdings, %d nodes and %d links", len(rows), len(spec.Nodes), len(spec.Links))
	}
	parent := make(map[charts.ID]charts.ID, len(spec.Links))
	byID := make(map[charts.ID]charts.SankeyNode, len(spec.Nodes))
	for _, link := range spec.Links {
		if !link.Value.Valid() || link.Value.Number <= 0 {
			t.Errorf("undrawn flow %s", link.ID)
		}
		parent[link.Target] = link.Source
	}
	for _, node := range spec.Nodes {
		byID[node.ID] = node
	}
	root := spec.Nodes[0].ID
	total := decimal.Zero
	for _, row := range rows {
		id := charts.ID(pathID(nodeAccount, row.Class.ID, row.Institution.ID, row.Account.ID))
		node := byID[id]
		if node.Label != row.Account.Label || node.Text != row.Amount {
			t.Errorf("account %s: got %q %q, want %q %q", id, node.Label, node.Text, row.Account.Label, row.Amount)
		}
		class, institution := parent[parent[id]], parent[id]
		if byID[institution].Label != row.Institution.Label || byID[class].Label != row.Class.Label || parent[class] != root {
			t.Errorf("incorrect ancestry for %s", id)
		}
		total = total.Add(decimal.RequireFromString(row.Amount))
	}
	if spec.Nodes[0].Text != format(total) {
		t.Errorf("root total = %q, want %q", spec.Nodes[0].Text, format(total))
	}
	north, south := charts.ID(pathID(nodeInstitution, "cash", "harbor-north")), charts.ID(pathID(nodeInstitution, "cash", "harbor-south"))
	if byID[north].Label != byID[south].Label || slices.Equal(byID[north].Detail, byID[south].Detail) {
		t.Error("same-named institutions need different readable context")
	}
	translated := Sankey(charts.SankeySpec{}, "Total", DenseHoldings([4]string{"A", "B", "C", "D"}), format)
	for i, node := range translated.Nodes {
		if node.ID != spec.Nodes[i].ID {
			t.Errorf("translation changed node ID from %q to %q", spec.Nodes[i].ID, node.ID)
		}
	}
	if pathID(nodeAccount, "class/a", "bank", "acct") == pathID(nodeAccount, "class", "a/bank", "acct") {
		t.Error("slashes in identity segments caused a path collision")
	}
}
