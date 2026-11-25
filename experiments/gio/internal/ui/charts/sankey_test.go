package charts

import (
	"image"
	"math"
	"slices"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func sankeyGeometryFor(width, height int) sankeyGeometry {
	return sankeyGeometry{width: width, height: height, nodeW: 14, gap: 10, labelSpace: 150, slotMin: 16, minNode: 3, minLink: 1}
}

// sankeySpecForTest is a three-column split: a root into two classes, and the
// classes into accounts.
func sankeySpecForTest() SankeySpec {
	return SankeySpec{
		Label: "Composition", Summary: "Where the assets are held.", ColumnTitles: []string{"Total", "Class", "Account"},
		Messages: Messages{Selected: "Selected", Missing: "No value", Empty: "No data"},
		Nodes: []SankeyNode{
			{ID: "root", Label: "Total", Text: "$100.00", HideLabel: true},
			{ID: "cash", Label: "Cash", Text: "$60.00"},
			{ID: "stock", Label: "Stocks", Text: "$40.00"},
			{ID: "checking", Label: "Everyday checking account with a long name", Text: "$40.00"},
			{ID: "savings", Label: "Savings", Text: "$20.00"},
			{ID: "broker", Label: "Brokerage", Text: "$40.00"},
		},
		Links: []SankeyLink{
			{ID: "l-cash", Source: "root", Target: "cash", Value: Num(60), Text: "$60.00"},
			{ID: "l-stock", Source: "root", Target: "stock", Value: Num(40), Text: "$40.00"},
			{ID: "l-checking", Source: "cash", Target: "checking", Value: Num(40), Text: "$40.00"},
			{ID: "l-savings", Source: "cash", Target: "savings", Value: Num(20), Text: "$20.00"},
			{ID: "l-broker", Source: "stock", Target: "broker", Value: Num(40), Text: "$40.00"},
		},
	}
}

func TestSankeyGraphAssignsColumnsAndRejectsUndrawableLinks(t *testing.T) {
	spec := sankeySpecForTest()
	g := spec.graph()
	for i, want := range []int{0, 1, 1, 2, 2, 2} {
		if g.depth[i] != want {
			t.Fatalf("depth[%s] = %d, want %d", spec.Nodes[i].ID, g.depth[i], want)
		}
	}
	if len(spec.Validate()) != 0 || g.drawn() != 5 {
		t.Fatalf("valid spec reports %v, draws %d links", spec.Validate(), g.drawn())
	}

	// A longer path pushes a node right even when a shorter path also reaches it.
	long := sankeySpecForTest()
	long.Links = append(long.Links, SankeyLink{ID: "shortcut", Source: "root", Target: "broker", Value: Num(5)})
	if d := long.graph().depth[5]; d != 2 {
		t.Fatalf("broker depth with a shortcut = %d, want the longest path 2", d)
	}

	bad := sankeySpecForTest()
	bad.Nodes = append(bad.Nodes, SankeyNode{ID: "x"}, SankeyNode{ID: "y"})
	bad.Links = append(bad.Links,
		SankeyLink{ID: "xy", Source: "x", Target: "y", Value: Num(1)},
		SankeyLink{ID: "yx", Source: "y", Target: "x", Value: Num(1)},
		SankeyLink{ID: "self", Source: "cash", Target: "cash", Value: Num(1)},
		SankeyLink{ID: "ghost", Source: "cash", Target: "nowhere", Value: Num(1)},
		SankeyLink{ID: "zero", Source: "cash", Target: "savings", Value: Num(0)},
		SankeyLink{ID: "negative", Source: "cash", Target: "savings", Value: Num(-5)},
		SankeyLink{ID: "unknown", Source: "cash", Target: "savings", Value: NoValue},
		SankeyLink{ID: "l-cash", Source: "root", Target: "cash", Value: Num(1)},
	)
	problems := map[Problem]bool{}
	for _, p := range bad.Validate() {
		problems[p] = true
	}
	for _, want := range []Problem{
		{Kind: ProblemCycle, ID: "xy"}, {Kind: ProblemCycle, ID: "yx"}, {Kind: ProblemCycle, ID: "self"},
		{Kind: ProblemUnknownNode, ID: "ghost"}, {Kind: ProblemRepeatedID, ID: "l-cash"},
	} {
		if !problems[want] {
			t.Fatalf("Validate missed %+v in %v", want, bad.Validate())
		}
	}
	// Non-positive and missing values are ordinary data, not invalid IDs.
	for _, p := range bad.Validate() {
		if p.ID == "zero" || p.ID == "negative" || p.ID == "unknown" {
			t.Fatalf("a link with no flow was reported as a problem: %+v", p)
		}
	}
	p := bad.plan(bad.graph(), sankeyGeometryFor(600, 300))
	for _, id := range []ID{"xy", "yx", "self", "ghost", "zero", "negative", "unknown"} {
		if p.links[p.linkAt[id]].drawn {
			t.Fatalf("link %s must not be drawn", id)
		}
	}
	if len(p.links) != len(bad.Links) || len(bad.Rows()) != len(bad.Nodes)+len(bad.Links) {
		t.Fatal("undrawn links must stay in the plan and the rows")
	}
}

func TestSankeyPlanKeepsOrderWithinColumnsAndInsideThePlot(t *testing.T) {
	spec := sankeySpecForTest()
	p := spec.plan(spec.graph(), sankeyGeometryFor(600, 300))
	for col, members := range p.columns {
		last := -1
		for _, i := range members {
			n := p.nodes[i]
			if n.rect.Min.Y < 0 || n.rect.Max.Y > p.height {
				t.Fatalf("column %d node %s %v leaves the plot of height %d", col, spec.Nodes[i].ID, n.rect, p.height)
			}
			if n.rect.Min.Y < last {
				t.Fatalf("column %d node %s overlaps or reorders: %v after bottom %d", col, spec.Nodes[i].ID, n.rect, last)
			}
			last = n.rect.Max.Y
		}
	}
	// Caller order within a column is preserved: nothing is sorted by value.
	if p.nodes[p.nodeAt["checking"]].rect.Min.Y >= p.nodes[p.nodeAt["savings"]].rect.Min.Y ||
		p.nodes[p.nodeAt["savings"]].rect.Min.Y >= p.nodes[p.nodeAt["broker"]].rect.Min.Y {
		t.Fatal("the last column no longer follows caller order")
	}
	if p.nodeCount != 6 || len(p.order) != 11 || p.order[0].Item != "root" || p.order[1].Item != "cash" || p.order[3].Item != "checking" {
		t.Fatalf("keyboard order = %v", p.order)
	}
	// Column x positions increase and the last column leaves room for labels.
	for col := 1; col < len(p.colX); col++ {
		if p.colX[col] <= p.colX[col-1] {
			t.Fatalf("column x positions %v are not increasing", p.colX)
		}
	}
	if last := p.colX[len(p.colX)-1] + 14; last > 600-150 {
		t.Fatalf("last column ends at %d and overruns the label space", last)
	}
}

func TestSankeyLinkThicknessIsProportionalAndFitsItsNodes(t *testing.T) {
	spec := sankeySpecForTest()
	p := spec.plan(spec.graph(), sankeyGeometryFor(600, 300))
	th := func(id ID) float64 { return p.links[p.linkAt[id]].th }
	if got := th("l-cash") / th("l-stock"); math.Abs(got-1.5) > 1e-9 {
		t.Fatalf("60:40 flows have thickness ratio %v, want 1.5", got)
	}
	// Equal values have equal thickness in every column.
	if th("l-checking") != th("l-broker") || math.Abs(th("l-checking")-th("l-stock")) > 1e-9 {
		t.Fatalf("equal values differ: %v %v %v", th("l-checking"), th("l-broker"), th("l-stock"))
	}
	for i, n := range spec.Nodes {
		var in, out float64
		for _, l := range p.into[i] {
			in += p.links[l].th
		}
		for _, l := range p.from[i] {
			out += p.links[l].th
		}
		if h := p.nodes[i].rect.Dy(); float64(h)+1 < math.Max(in, out) {
			t.Fatalf("node %s is %dpx tall but carries %v", n.ID, h, math.Max(in, out))
		}
	}
	for _, l := range p.links {
		if l.sy < float64(p.nodes[l.src].rect.Min.Y)-1 || l.sy+l.th > float64(p.nodes[l.src].rect.Max.Y)+1 ||
			l.ty < float64(p.nodes[l.dst].rect.Min.Y)-1 || l.ty+l.th > float64(p.nodes[l.dst].rect.Max.Y)+1 {
			t.Fatalf("link %v leaves its nodes", l.sel)
		}
	}
}

func TestSankeyTinyFlowsStayVisible(t *testing.T) {
	spec := SankeySpec{
		Nodes: []SankeyNode{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Links: []SankeyLink{
			{ID: "big", Source: "a", Target: "b", Value: Num(1e9)},
			{ID: "tiny", Source: "a", Target: "c", Value: Num(1)},
		},
	}
	p := spec.plan(spec.graph(), sankeyGeometryFor(600, 300))
	if th := p.links[p.linkAt["tiny"]].th; th < 1 {
		t.Fatalf("a tiny flow is %vpx thick, want at least the minimum", th)
	}
	if h := p.nodes[p.nodeAt["c"]].rect.Dy(); h < 3 {
		t.Fatalf("a tiny node is %dpx tall", h)
	}
}

func TestSankeyPickPrefersNodesThenLinks(t *testing.T) {
	spec := sankeySpecForTest()
	p := spec.plan(spec.graph(), sankeyGeometryFor(600, 300))
	for _, n := range p.nodes {
		centre := n.rect.Min.Add(n.rect.Size().Div(2))
		if sel, ok := p.pick(centre); !ok || sel != n.sel {
			t.Fatalf("node centre %v picked %v %v, want %v", centre, sel, ok, n.sel)
		}
	}
	l := p.links[p.linkAt["l-cash"]]
	mid := image.Pt(int(l.x(0.5)), int(l.centreY(0.5)))
	if sel, ok := p.pick(mid); !ok || sel.Item != "l-cash" {
		t.Fatalf("middle of the band picked %v %v", sel, ok)
	}
	// Just outside the band, in the gap between the two flows' columns, nothing.
	far := image.Pt(int(l.x(0.5)), int(l.centreY(0.5)+l.th/2)+40)
	if sel, ok := p.pick(far); ok && sel.Item == "l-cash" {
		t.Fatalf("a point %v far from the band picked it", far)
	}
	if sel, ok := p.pick(image.Pt(-5, -5)); ok {
		t.Fatalf("outside the plot picked %v", sel)
	}
}

func TestSankeyKeyboardMovesAlongColumnsAndFlows(t *testing.T) {
	spec := sankeySpecForTest()
	p := spec.plan(spec.graph(), sankeyGeometryFor(600, 300))
	step := func(from ID, name key.Name) ID {
		sel, ok := p.move(Selection{Item: from}, name)
		if !ok {
			return from
		}
		return sel.Item
	}
	cases := []struct {
		from ID
		key  key.Name
		want ID
	}{
		{"root", key.NameRightArrow, "cash"},     // the larger flow
		{"cash", key.NameRightArrow, "checking"}, // 40 beats 20
		{"stock", key.NameLeftArrow, "root"},
		{"cash", key.NameDownArrow, "stock"},
		{"stock", key.NameDownArrow, "stock"},
		{"cash", key.NameUpArrow, "cash"},
		{"savings", key.NameUpArrow, "checking"},
		{"root", key.NameLeftArrow, "root"},
		{"broker", key.NameRightArrow, "broker"},
		{"broker", key.NameHome, "root"},
		{"root", key.NameEnd, "broker"},
		// A link a pointer selected can be left with the arrows.
		{"l-cash", key.NameLeftArrow, "root"},
		{"l-cash", key.NameRightArrow, "cash"},
	}
	for _, c := range cases {
		if got := step(c.from, c.key); got != c.want {
			t.Fatalf("%s then %v = %s, want %s", c.from, c.key, got, c.want)
		}
	}
}

func TestSankeyRowsCarryNodesThenLinksWithText(t *testing.T) {
	spec := sankeySpecForTest()
	spec.Selected = Selection{Item: "l-savings"}
	spec.Links = append(spec.Links, SankeyLink{ID: "gap", Source: "root", Target: "stock", Value: NoValue})
	rows := spec.Rows()
	if len(rows) != 12 || rows[0].ID != "root" || rows[0].Value != "$100.00" {
		t.Fatalf("rows = %+v", rows)
	}
	var savings, gap TextRow
	for _, r := range rows {
		switch r.ID {
		case "l-savings":
			savings = r
		case "gap":
			gap = r
		}
	}
	if savings.Label != "Cash"+linkSeparator+"Savings" || savings.Value != "$20.00" || !savings.Selected {
		t.Fatalf("link row = %+v", savings)
	}
	if !gap.Missing || gap.Value != "No value" {
		t.Fatalf("an undrawn flow must say why: %+v", gap)
	}
}

func TestSankeyNodeColorsInheritFromTheLargestSource(t *testing.T) {
	spec := sankeySpecForTest()
	colors := testPalette(false)
	inks := spec.nodeInks(colors, spec.graph())
	idx := func(id ID) int {
		for i, n := range spec.Nodes {
			if n.ID == id {
				return i
			}
		}
		t.Fatalf("no node %s", id)
		return -1
	}
	if inks[idx("cash")] == inks[idx("stock")] {
		t.Fatal("the two branches of the first split share a color")
	}
	if inks[idx("checking")] != inks[idx("cash")] || inks[idx("savings")] != inks[idx("cash")] || inks[idx("broker")] != inks[idx("stock")] {
		t.Fatal("accounts must inherit the color of their class")
	}
	spec.Nodes[idx("savings")].Color = colors.Success
	if got := spec.nodeInks(colors, spec.graph())[idx("savings")]; got != colors.Success {
		t.Fatalf("explicit color = %v", got)
	}
}

func TestSankeyRowsPlanGroupsByColumnAndScalesBars(t *testing.T) {
	spec := sankeySpecForTest()
	gr := spec.graph()
	g := sankeyRowsGeometry{width: 300, inset: 8, valueW: 60, gap: 8, barHeight: 8, rowGap: 6, titleHeights: []int{0, 20, 0}}
	for range spec.Nodes {
		g.rowHeights = append(g.rowHeights, 50)
	}
	p := spec.planRows(gr, g)
	if len(p.rows) != 6 || len(p.titles) != 1 || p.titleAt[0] != 1 {
		t.Fatalf("rows %d titles %v at %v", len(p.rows), p.titles, p.titleAt)
	}
	if p.height != 6*50+20 {
		t.Fatalf("height = %d", p.height)
	}
	var ids []ID
	for _, r := range p.rows {
		ids = append(ids, r.sel.Item)
	}
	want := []ID{"root", "cash", "stock", "checking", "savings", "broker"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("row order %v, want %v", ids, want)
		}
	}
	root, cash, savings := p.rows[0], p.rows[1], p.rows[4]
	if root.bar.Dx() != root.track.Dx() {
		t.Fatalf("the largest node fills its track: %v of %v", root.bar, root.track)
	}
	if got, want := float64(cash.bar.Dx()), 0.6*float64(root.track.Dx()); math.Abs(got-want) > 1.5 {
		t.Fatalf("a 60%% node has bar %v, want %v", got, want)
	}
	if got, want := float64(savings.bar.Dx()), 0.2*float64(root.track.Dx()); math.Abs(got-want) > 1.5 {
		t.Fatalf("a 20%% node has bar %v, want %v", got, want)
	}
	if sel, ok := p.pick(cash.row.Min.Add(image.Pt(3, 3))); !ok || sel.Item != "cash" {
		t.Fatalf("row pick = %v %v", sel, ok)
	}
}

// sankeyScan sweeps the chart column by column, so the first item it reaches is
// the first node, as the shared keyboard test expects.
func sankeyScan(size image.Point) []image.Point {
	var pts []image.Point
	for x := 0; x < size.X; x += 12 {
		for y := 0; y < size.Y; y += 4 {
			pts = append(pts, image.Pt(x, y))
		}
	}
	return pts
}

func sankeySubject() subject {
	var chart SankeyChart
	build := func(sel Selection) SankeySpec {
		spec := sankeySpecForTest()
		spec.Selected = sel
		return spec
	}
	// The pointer reaches nodes and links; the keyboard walks the nodes.
	items := []Selection{{Item: "root"}, {Item: "cash"}, {Item: "stock"}, {Item: "checking"}, {Item: "savings"}, {Item: "broker"},
		{Item: "l-cash"}, {Item: "l-stock"}, {Item: "l-checking"}, {Item: "l-savings"}, {Item: "l-broker"}}
	return subject{
		name: "sankey", tag: &chart, end: Selection{Item: "broker"}, items: items,
		draw: func(gtx layout.Context, view ui.View, sel Selection) (layout.Dimensions, []Event) {
			return chart.Layout(gtx, view, build(sel))
		},
		scan: sankeyScan,
	}
}

func init() { registry = append(registry, sankeySubject()) }

func TestSankeyChartNarrowListsNodesByColumn(t *testing.T) {
	var chart SankeyChart
	spec := sankeySpecForTest()
	draw := func(gtx layout.Context, v ui.View, sel Selection) (layout.Dimensions, []Event) {
		spec.Selected = sel
		return chart.Layout(gtx, v, spec)
	}
	r := newRig(320, false)
	size, _ := r.frame(draw, Selection{})
	r.frame(draw, Selection{})
	seen := map[ID]bool{}
	for y := 0; y < size.Y; y += 5 {
		r.press(image.Pt(size.X/2, y))
		_, events := r.frame(draw, Selection{})
		for _, e := range events {
			seen[e.Selection.Item] = true
		}
	}
	for _, id := range []ID{"root", "cash", "stock", "checking", "savings", "broker"} {
		if !seen[id] {
			t.Fatalf("narrow row for %s never selected", id)
		}
	}
	if seen["l-cash"] {
		t.Fatal("narrow rows must not select links")
	}
}

func TestSankeyChartNoticesForEmptyAndUndrawable(t *testing.T) {
	msgs := Messages{Empty: "No data", Missing: "No values"}
	r := newRig(520, false)
	for name, spec := range map[string]SankeySpec{
		"empty": {Messages: msgs},
		"no flow": {Messages: msgs, Nodes: []SankeyNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
			Links: []SankeyLink{{ID: "l", Source: "a", Target: "b", Value: NoValue}}},
	} {
		var chart SankeyChart
		size, events := r.frame(func(gtx layout.Context, v ui.View, _ Selection) (layout.Dimensions, []Event) {
			return chart.Layout(gtx, v, spec)
		}, Selection{})
		if size.X <= 0 || size.Y <= 0 || len(events) != 0 {
			t.Fatalf("%s: size %v events %v", name, size, events)
		}
	}
}

func TestSankeyCycleKeepsEnteringLeavingAndIndependentLinks(t *testing.T) {
	spec := SankeySpec{
		Nodes: []SankeyNode{{ID: "root"}, {ID: "a"}, {ID: "b"}, {ID: "tail"}, {ID: "class"}, {ID: "account"}},
		Links: []SankeyLink{
			{ID: "root-a", Source: "root", Target: "a", Value: Num(5)},
			{ID: "a-b", Source: "a", Target: "b", Value: Num(5)},
			{ID: "b-a", Source: "b", Target: "a", Value: Num(5)},
			{ID: "b-tail", Source: "b", Target: "tail", Value: Num(5)},
			{ID: "root-class", Source: "root", Target: "class", Value: Num(10)},
			{ID: "class-account", Source: "class", Target: "account", Value: Num(10)},
		},
	}
	want := []Problem{{Kind: ProblemCycle, ID: "a-b"}, {Kind: ProblemCycle, ID: "b-a"}}
	if problems := spec.Validate(); !slices.Equal(problems, want) {
		t.Fatalf("Validate = %v, want %v", problems, want)
	}
	gr := spec.graph()
	plan := spec.plan(gr, sankeyGeometryFor(700, 350))
	for _, link := range plan.links {
		wantDrawn := link.sel.Item != "a-b" && link.sel.Item != "b-a"
		if link.drawn != wantDrawn {
			t.Errorf("%s drawn = %t, want %t", link.sel.Item, link.drawn, wantDrawn)
		}
	}
	if got := gr.depth[gr.nodeAt["account"]]; got != 2 {
		t.Errorf("valid account depth = %d, want 2", got)
	}
	if len(spec.Rows()) != len(spec.Nodes)+len(spec.Links) {
		t.Error("cycle dropped source rows")
	}
}

func sankeyPathsSpec() SankeySpec {
	spec := SankeySpec{
		ColumnTitles: []string{"Total", "Class", "Institution", "Account"},
		Nodes: []SankeyNode{
			{ID: "root", Label: "Total"},
			{ID: "cash", Label: "Cash"}, {ID: "funds", Label: "Funds"},
			{ID: "cash/north", Label: "Harbor Bank", Detail: []string{"North Branch"}},
			{ID: "cash/south", Label: "Harbor Bank", Detail: []string{"South Branch"}},
			{ID: "funds/north", Label: "Harbor Bank", Detail: []string{"North Branch"}},
			{ID: "cash/north/reserve", Label: "Household Reserve", Text: "$0.01"},
			{ID: "cash/north/savings", Label: "Savings", Text: "$25.00"},
			{ID: "cash/south/savings", Label: "Savings", Text: "$4.00"},
			{ID: "funds/north/reserve", Label: "Household Reserve", Text: "$9.00"},
		},
	}
	link := func(id, from, to ID, flow float64) SankeyLink {
		return SankeyLink{ID: id, Source: from, Target: to, Value: Num(flow)}
	}
	spec.Links = []SankeyLink{
		link("root-cash", "root", "cash", 29.01), link("root-funds", "root", "funds", 9),
		link("cash-north", "cash", "cash/north", 25.01), link("cash-south", "cash", "cash/south", 4),
		link("funds-north", "funds", "funds/north", 9),
		link("cash-reserve", "cash/north", "cash/north/reserve", 0.01), link("north-savings", "cash/north", "cash/north/savings", 25),
		link("south-savings", "cash/south", "cash/south/savings", 4), link("funds-reserve", "funds/north", "funds/north/reserve", 9),
	}
	return spec
}

func TestSankeyEmphasisFollowsPathsByID(t *testing.T) {
	spec := sankeyPathsSpec()
	gr := spec.graph()
	for _, test := range []struct {
		name     string
		active   ID
		selected ID
		links    []sankeyEmphasis
		nodes    []sankeyEmphasis
	}{
		{
			name: "tiny leaf only follows its three ancestors", active: "cash/north/reserve",
			links: []sankeyEmphasis{1, 0, 1, 0, 0, 1, 0, 0, 0},
			nodes: []sankeyEmphasis{1, 1, 0, 1, 0, 0, 1, 0, 0, 0},
		},
		{
			name: "institution follows its own children", active: "cash/north",
			links: []sankeyEmphasis{1, 0, 1, 0, 0, 1, 1, 0, 0},
			nodes: []sankeyEmphasis{1, 1, 0, 1, 0, 0, 1, 1, 0, 0},
		},
		{
			name: "link follows ancestors at its source and descendants at its target", active: "cash-south",
			links: []sankeyEmphasis{1, 0, 0, 1, 0, 0, 0, 1, 0},
			nodes: []sankeyEmphasis{1, 1, 0, 0, 1, 0, 0, 0, 1, 0},
		},
		{
			name: "selected account survives hover in another class", active: "cash/north/reserve", selected: "funds/north/reserve",
			links: []sankeyEmphasis{1, 2, 1, 0, 2, 1, 0, 0, 2},
			nodes: []sankeyEmphasis{2, 1, 2, 1, 0, 2, 1, 0, 0, 2},
		},
		{
			name: "class includes both of its institutions", selected: "cash",
			links: []sankeyEmphasis{2, 0, 2, 2, 0, 2, 2, 2, 0},
			nodes: []sankeyEmphasis{2, 2, 0, 2, 2, 0, 2, 2, 2, 0},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			nodes, links := gr.emphasis(Selection{Item: test.active}, Selection{Item: test.selected})
			if !slices.Equal(nodes, test.nodes) || !slices.Equal(links, test.links) {
				t.Fatalf("nodes %v, links %v, want nodes %v, links %v", nodes, links, test.nodes, test.links)
			}
		})
	}
	item := spec.readout(gr, Selection{Item: "cash/north/reserve"})
	if item.value != "$0.01" || !strings.Contains(itemLabel(*item, spec.Messages), "Cash › Harbor Bank") {
		t.Fatalf("compact/accessible readout lost its lineage or exact amount: %+v", item)
	}
}

func TestSankeyCompactKeyboardRevealsLastRowAndKeepsSelection(t *testing.T) {
	spec := sankeyPathsSpec()
	for i := range spec.Nodes {
		spec.Nodes[i].Detail = append(spec.Nodes[i].Detail, "Complete detail for the narrow window, including enough words to wrap.")
	}
	r := newRig(320, false)
	var chart SankeyChart
	draw := func(gtx layout.Context, view ui.View, selected Selection) (layout.Dimensions, []Event) {
		spec.Selected = selected
		return chart.Layout(gtx, view, spec)
	}
	r.frame(draw, Selection{})
	r.press(image.Pt(60, 80))
	r.frame(draw, Selection{})
	r.frame(draw, Selection{})
	r.key(key.NameEnd)
	r.frame(draw, Selection{})
	want := Selection{Item: "funds/north/reserve"}
	if chart.cursor != want || chart.rows.Position.Offset <= 0 {
		t.Fatalf("End focused %v at offset %d, want %v in view", chart.cursor, chart.rows.Position.Offset, want)
	}
	r.key(key.NameReturn)
	_, events := r.frame(draw, Selection{})
	if selected := Apply(Selection{}, events); selected != want {
		t.Fatalf("selection = %v, want %v", selected, want)
	}
	r.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(5, 5)})
	r.key(key.NameHome)
	r.frame(draw, want)
	if chart.cursor.Item != "root" || chart.rows.Position.Offset != 0 {
		t.Fatalf("Home focused %v at offset %d", chart.cursor, chart.rows.Position.Offset)
	}
	r.key(key.NameEscape)
	_, events = r.frame(draw, want)
	if selected := Apply(want, events); !selected.IsZero() {
		t.Fatalf("Escape left selection %v", selected)
	}
}
