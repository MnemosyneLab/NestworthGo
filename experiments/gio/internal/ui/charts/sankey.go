package charts

import (
	"image"
	"image/color"
	"math"
	"slices"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// SankeyNode is one stop of a flow diagram, such as an asset class, an
// institution or an account.
type SankeyNode struct {
	// ID is stable across refreshes and shares one namespace with link IDs,
	// because a Selection names either a node or a link.
	ID ID
	// Label is the full name. The plot may shorten it to fit a column; the
	// readout, tooltip, rows and assistive text always carry it in full.
	Label string
	// Text is the exact amount as the caller formats it. It is optional: the
	// size of a node comes from its links, never from this text.
	Text   string
	Detail []string
	// Color overrides the theme color. Without it a node inherits the color
	// of the source of its largest incoming link, except that a node fed
	// only by a root that branches starts a new theme color, so each branch
	// of the first split is distinguishable.
	Color color.NRGBA
	// HideLabel omits the label from the plot, for example for a root whose
	// name the column title already gives. It stays in every other form.
	HideLabel bool
}

// SankeyLink is a flow from one node to another.
type SankeyLink struct {
	ID ID
	// Source and Target are node IDs.
	Source, Target ID
	// Value sizes the flow. Only a finite, strictly positive value is drawn.
	// A zero, negative or missing value is never mirrored into a flow and
	// stays in Rows with its text.
	Value Value
	// Text is the exact amount as the caller formats it.
	Text   string
	Detail []string
}

// SankeySpec describes a left-to-right flow diagram. Nodes and links appear in
// the order given; the chart sorts neither. Columns follow the longest path
// from a node that has no incoming flow. Validate reports self-links, links
// with unknown endpoints, and every positive link that belongs to a cycle.
// Those links are omitted; the remaining links determine the columns.
//
// The chart never derives an amount. Node heights and link widths come from
// the drawn link values, a node being as tall as the larger of its incoming
// and outgoing flow, and every amount a person reads comes from the caller's
// text. A caller that needs the node amounts to reconcile with its links
// passes links that already reconcile.
type SankeySpec struct {
	Label, Summary string
	Nodes          []SankeyNode
	Links          []SankeyLink
	// ColumnTitles optionally names the columns from the left. A column
	// without a title has none.
	ColumnTitles []string
	// Height is the minimum height of the plot. The plot grows with the
	// busiest column so that every label has its own line.
	Height   unit.Dp
	Selected Selection
	Messages Messages
}

// linkSeparator joins the two ends of a link in its label.
const linkSeparator = " › "

// Rows returns the text form of every node and then every link, in the order
// given. Links are listed whether or not they are drawn.
func (s SankeySpec) Rows() []TextRow {
	rows := make([]TextRow, 0, len(s.Nodes)+len(s.Links))
	for _, n := range s.Nodes {
		rows = append(rows, TextRow{
			ID: n.ID, Label: n.Label, Value: n.Text, Detail: n.Detail,
			Selected: s.Selected == Selection{Item: n.ID},
		})
	}
	for _, l := range s.Links {
		value, missing := valueText(l.Value, l.Text, s.Messages)
		rows = append(rows, TextRow{
			ID: l.ID, Label: s.linkLabel(l), Value: value, Detail: l.Detail, Missing: missing,
			Selected: s.Selected == Selection{Item: l.ID},
		})
	}
	return rows
}

// Validate lists empty and repeated node and link IDs, links that name an
// unknown node, self-links, and positive links that belong to a cycle.
func (s SankeySpec) Validate() []Problem {
	var check idCheck
	for _, n := range s.Nodes {
		check.add("", n.ID)
	}
	for _, l := range s.Links {
		check.add("", l.ID)
	}
	return append(check.problems, s.graph().problems...)
}

func (s SankeySpec) linkLabel(l SankeyLink) string {
	end := func(id ID) string {
		for _, n := range s.Nodes {
			if n.ID == id {
				return n.Label
			}
		}
		return s.Messages.Missing
	}
	return end(l.Source) + linkSeparator + end(l.Target)
}

// sankeyGraph is the drawable structure of a spec.
type sankeyGraph struct {
	// src and dst are node indexes per link, or -1 for an unknown node.
	src, dst []int
	// ok marks the links that are drawn.
	ok []bool
	// into and from contain only drawn links.
	into, from     [][]int
	nodeAt, linkAt map[ID]int
	// depth is the column of each node.
	depth    []int
	columns  int
	problems []Problem
}

func (g sankeyGraph) drawn() int {
	count := 0
	for _, ok := range g.ok {
		if ok {
			count++
		}
	}
	return count
}

func (s SankeySpec) graph() sankeyGraph {
	n := len(s.Nodes)
	g := sankeyGraph{
		src: make([]int, len(s.Links)), dst: make([]int, len(s.Links)), ok: make([]bool, len(s.Links)),
		depth: make([]int, n), into: make([][]int, n), from: make([][]int, n),
		nodeAt: make(map[ID]int, n), linkAt: make(map[ID]int, len(s.Links)),
	}
	for i, node := range s.Nodes {
		if _, seen := g.nodeAt[node.ID]; !seen {
			g.nodeAt[node.ID] = i
		}
	}
	lookup := func(id ID) int {
		if i, ok := g.nodeAt[id]; ok {
			return i
		}
		return -1
	}
	for l, link := range s.Links {
		if _, seen := g.linkAt[link.ID]; !seen {
			g.linkAt[link.ID] = l
		}
		g.src[l], g.dst[l] = lookup(link.Source), lookup(link.Target)
		switch {
		case g.src[l] < 0 || g.dst[l] < 0:
			g.problems = append(g.problems, Problem{Kind: ProblemUnknownNode, ID: link.ID})
		case g.src[l] == g.dst[l]:
			g.problems = append(g.problems, Problem{Kind: ProblemCycle, ID: link.ID})
		case link.Value.Valid() && link.Value.Number > 0:
			g.ok[l] = true
			g.from[g.src[l]] = append(g.from[g.src[l]], l)
		}
	}

	// A positive link lies on a cycle exactly when its endpoints are in the
	// same strongly connected component. Keep links entering or leaving it.
	component := g.strongComponents()
	indegree := make([]int, n)
	for l, link := range s.Links {
		if !g.ok[l] {
			continue
		}
		if component[g.src[l]] == component[g.dst[l]] {
			g.ok[l] = false
			g.problems = append(g.problems, Problem{Kind: ProblemCycle, ID: link.ID})
			continue
		}
		indegree[g.dst[l]]++
		g.into[g.dst[l]] = append(g.into[g.dst[l]], l)
	}
	for i := range g.from {
		g.from[i] = slices.DeleteFunc(g.from[i], func(l int) bool { return !g.ok[l] })
	}

	// Kahn's algorithm assigns each node the length of its longest drawn path.
	var queue []int
	for i, d := range indegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		for _, l := range g.from[i] {
			to := g.dst[l]
			g.depth[to] = max(g.depth[to], g.depth[i]+1)
			if indegree[to]--; indegree[to] == 0 {
				queue = append(queue, to)
			}
		}
		g.columns = max(g.columns, g.depth[i]+1)
	}
	return g
}

// strongComponents uses Tarjan's algorithm on the positive candidate links.
func (g sankeyGraph) strongComponents() []int {
	n := len(g.from)
	index, low, component := make([]int, n), make([]int, n), make([]int, n)
	onStack := make([]bool, n)
	var stack []int
	next := 1
	var visit func(int)
	visit = func(i int) {
		index[i], low[i] = next, next
		next++
		stack = append(stack, i)
		onStack[i] = true
		for _, l := range g.from[i] {
			to := g.dst[l]
			if index[to] == 0 {
				visit(to)
				low[i] = min(low[i], low[to])
			} else if onStack[to] {
				low[i] = min(low[i], index[to])
			}
		}
		if low[i] == index[i] {
			for {
				j := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[j], component[j] = false, i
				if j == i {
					break
				}
			}
		}
	}
	for i := range n {
		if index[i] == 0 {
			visit(i)
		}
	}
	return component
}

type sankeyEmphasis uint8

const (
	sankeyUnrelated sankeyEmphasis = iota
	sankeyActivePath
	sankeySelectedPath
)

// emphasis follows incoming and outgoing paths independently. Traversing out
// of an ancestor (or into a descendant) would include unrelated branches.
func (g sankeyGraph) emphasis(active, selected Selection) (nodes, links []sankeyEmphasis) {
	nodes = make([]sankeyEmphasis, len(g.depth))
	links = make([]sankeyEmphasis, len(g.ok))
	add := func(sel Selection, level sankeyEmphasis) {
		if sel.IsZero() || sel.Series != "" {
			return
		}
		walk := func(start int, edges [][]int, next []int) {
			seen := make([]bool, len(nodes))
			queue := []int{start}
			seen[start] = true
			for len(queue) > 0 {
				i := queue[0]
				queue = queue[1:]
				nodes[i] = level
				for _, l := range edges[i] {
					links[l] = level
					if to := next[l]; !seen[to] {
						seen[to] = true
						queue = append(queue, to)
					}
				}
			}
		}
		if i, ok := g.nodeAt[sel.Item]; ok {
			walk(i, g.into, g.src)
			walk(i, g.from, g.dst)
		} else if l, ok := g.linkAt[sel.Item]; ok && g.ok[l] {
			links[l] = level
			walk(g.src[l], g.into, g.src)
			walk(g.dst[l], g.from, g.dst)
		}
	}
	add(active, sankeyActivePath)
	add(selected, sankeySelectedPath)
	return nodes, links
}

// nodeContext gives a tree node's parent path and omits the common root.
// For a join, give its distinct incoming node names without inventing one path.
func (s SankeySpec) nodeContext(g sankeyGraph, i int) string {
	var labels []string
	for len(g.into[i]) == 1 {
		i = g.src[g.into[i][0]]
		if len(g.into[i]) > 0 && s.Nodes[i].Label != "" {
			labels = append(labels, s.Nodes[i].Label)
		}
	}
	slices.Reverse(labels)
	if len(g.into[i]) > 1 {
		var parents []string
		for _, l := range g.into[i] {
			name := s.Nodes[g.src[l]].Label
			if name != "" && !slices.Contains(parents, name) {
				parents = append(parents, name)
			}
		}
		labels = append([]string{strings.Join(parents, ", ")}, labels...)
	}
	return strings.Join(labels, linkSeparator)
}

// nodeInks resolves the color of every node. See SankeyNode.Color.
func (s SankeySpec) nodeInks(colors *ui.Palette, g sankeyGraph) []color.NRGBA {
	n := len(s.Nodes)
	largest := make([]int, n)
	for i := range largest {
		largest[i] = -1
	}
	incoming := make([]int, n)
	outgoing := make([]int, n)
	for l, link := range s.Links {
		if !g.ok[l] {
			continue
		}
		outgoing[g.src[l]]++
		incoming[g.dst[l]]++
		if best := largest[g.dst[l]]; best < 0 || link.Value.Number > s.Links[best].Value.Number {
			largest[g.dst[l]] = l
		}
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return g.depth[a] - g.depth[b] })
	inks := make([]color.NRGBA, n)
	fresh := 0
	for _, i := range order {
		if ink := s.Nodes[i].Color; ink != (color.NRGBA{}) {
			inks[i] = ink
			continue
		}
		if l := largest[i]; l >= 0 {
			from := g.src[l]
			if !(incoming[from] == 0 && outgoing[from] > 1) {
				inks[i] = inks[from]
				continue
			}
		}
		inks[i] = colors.Category[fresh%len(colors.Category)]
		fresh++
	}
	return inks
}

func (s SankeySpec) readout(g sankeyGraph, sel Selection) *readout {
	if sel.IsZero() || sel.Series != "" {
		return nil
	}
	if i, ok := g.nodeAt[sel.Item]; ok {
		n := s.Nodes[i]
		details := n.Detail
		if context := s.nodeContext(g, i); context != "" {
			details = append([]string{context}, details...)
		}
		return &readout{label: n.Label, value: n.Text, detail: details, selected: sel == s.Selected}
	}
	if i, ok := g.linkAt[sel.Item]; ok {
		l := s.Links[i]
		value, missing := valueText(l.Value, l.Text, s.Messages)
		details := l.Detail
		if from := g.src[i]; from >= 0 {
			if context := s.nodeContext(g, from); context != "" {
				details = append([]string{context}, details...)
			}
		}
		return &readout{label: s.linkLabel(l), value: value, detail: details, selected: sel == s.Selected, missing: missing}
	}
	return nil
}

// sankeyGeometry are the pixel inputs of a plot plan.
type sankeyGeometry struct {
	// width is the width of the plot including the label space of the last
	// column; height is the minimum plot height.
	width, height int
	nodeW, gap    int
	// labelSpace is the room right of the last column for its labels.
	labelSpace int
	// slotMin is the height one node reserves for its label.
	slotMin int
	// minNode and minLink keep a tiny node or flow visible.
	minNode, minLink int
}

type sankeyNodePlan struct {
	sel Selection
	col int
	// rect is the node mark; hit is the pointer target, which spans the node's
	// whole slot vertically and a little beyond it horizontally.
	rect, hit image.Rectangle
	labelHit  image.Rectangle
	// flow is the larger of the incoming and outgoing drawn value; zero means
	// no drawn link touches the node.
	flow float64
}

type sankeyLinkPlan struct {
	sel   Selection
	drawn bool
	src   int
	dst   int
	value float64
	// x0 and x1 are the source's right edge and the target's left edge.
	x0, x1 float64
	// sy and ty are the top of the band at each end; th is its thickness.
	sy, ty, th float64
}

type sankeyPlan struct {
	nodes   []sankeyNodePlan
	links   []sankeyLinkPlan
	columns [][]int
	// colX is the left edge of every column.
	colX []int
	// order lists the nodes column by column, then the drawn links.
	order     []Selection
	nodeCount int
	height    int
	nodeAt    map[ID]int
	linkAt    map[ID]int
	// into and from list the drawn links entering and leaving each node.
	into, from [][]int
}

func (s SankeySpec) plan(gr sankeyGraph, g sankeyGeometry) sankeyPlan {
	n := len(s.Nodes)
	p := sankeyPlan{
		nodes: make([]sankeyNodePlan, n), links: make([]sankeyLinkPlan, len(s.Links)),
		nodeAt: map[ID]int{}, linkAt: map[ID]int{},
		into: make([][]int, n), from: make([][]int, n),
	}
	if n == 0 {
		return p
	}
	p.columns = make([][]int, gr.columns)
	for i, node := range s.Nodes {
		p.nodes[i] = sankeyNodePlan{sel: Selection{Item: node.ID}, col: gr.depth[i]}
		p.columns[gr.depth[i]] = append(p.columns[gr.depth[i]], i)
		if _, seen := p.nodeAt[node.ID]; !seen {
			p.nodeAt[node.ID] = i
		}
	}
	flowIn := make([]float64, n)
	flowOut := make([]float64, n)
	for l, link := range s.Links {
		p.links[l] = sankeyLinkPlan{sel: Selection{Item: link.ID}, drawn: gr.ok[l], src: gr.src[l], dst: gr.dst[l]}
		if _, seen := p.linkAt[link.ID]; !seen {
			p.linkAt[link.ID] = l
		}
		if !gr.ok[l] {
			continue
		}
		p.links[l].value = link.Value.Number
		flowOut[gr.src[l]] += link.Value.Number
		flowIn[gr.dst[l]] += link.Value.Number
		p.from[gr.src[l]] = append(p.from[gr.src[l]], l)
		p.into[gr.dst[l]] = append(p.into[gr.dst[l]], l)
	}
	for i := range p.nodes {
		p.nodes[i].flow = math.Max(flowIn[i], flowOut[i])
	}

	// One value-to-pixel scale serves every column, so equal values have equal
	// thickness everywhere. The scarcest column sets it.
	ky := math.Inf(1)
	for _, members := range p.columns {
		sum := 0.0
		for _, i := range members {
			sum += p.nodes[i].flow
		}
		if sum > 0 {
			room := math.Max(1, float64(g.height-g.gap*(len(members)-1)))
			ky = math.Min(ky, room/sum)
		}
	}
	if !finite(ky) || ky <= 0 {
		ky = 0
	}
	for l := range p.links {
		if p.links[l].drawn {
			p.links[l].th = math.Max(float64(g.minLink), p.links[l].value*ky)
		}
	}
	nodeH := make([]float64, n)
	slotH := make([]float64, n)
	for i := range p.nodes {
		var in, out float64
		for _, l := range p.into[i] {
			in += p.links[l].th
		}
		for _, l := range p.from[i] {
			out += p.links[l].th
		}
		nodeH[i] = math.Max(float64(g.minNode), math.Max(in, out))
		slotH[i] = math.Max(nodeH[i], float64(g.slotMin))
	}

	extent := func(members []int) float64 {
		total := float64(g.gap * max(0, len(members)-1))
		for _, i := range members {
			total += slotH[i]
		}
		return total
	}
	height := float64(g.height)
	for _, members := range p.columns {
		height = math.Max(height, extent(members))
	}
	slotTop := make([]float64, n)
	for _, members := range p.columns {
		y := (height - extent(members)) / 2
		for _, i := range members {
			slotTop[i] = y
			y += slotH[i] + float64(g.gap)
		}
	}
	centre := func(i int) float64 { return slotTop[i] + slotH[i]/2 }
	// Moving the slots toward the weighted center of their neighbors straightens
	// the flows; collisions are then resolved without reordering a column.
	resolve := func(members []int) {
		y := 0.0
		for _, i := range members {
			slotTop[i] = math.Max(slotTop[i], y)
			y = slotTop[i] + slotH[i] + float64(g.gap)
		}
		bottom := height
		for k := len(members) - 1; k >= 0; k-- {
			i := members[k]
			slotTop[i] = math.Min(slotTop[i], bottom-slotH[i])
			bottom = slotTop[i] - float64(g.gap)
		}
	}
	pull := func(col int, links func(int) []int, neighbor func(sankeyLinkPlan) int) {
		for _, i := range p.columns[col] {
			var weight, moment float64
			for _, l := range links(i) {
				weight += p.links[l].th
				moment += p.links[l].th * centre(neighbor(p.links[l]))
			}
			if weight > 0 {
				slotTop[i] += (moment/weight - centre(i)) * 0.5
			}
		}
		resolve(p.columns[col])
	}
	for range 6 {
		for col := len(p.columns) - 2; col >= 0; col-- {
			pull(col, func(i int) []int { return p.from[i] }, func(l sankeyLinkPlan) int { return l.dst })
		}
		for col := 1; col < len(p.columns); col++ {
			pull(col, func(i int) []int { return p.into[i] }, func(l sankeyLinkPlan) int { return l.src })
		}
	}

	p.colX = make([]int, len(p.columns))
	if len(p.columns) > 1 {
		step := float64(max(1, g.width-g.labelSpace-g.nodeW)) / float64(len(p.columns)-1)
		for col := range p.colX {
			p.colX[col] = int(math.Round(float64(col) * step))
		}
	}
	slop := max(2, g.nodeW/4)
	for i := range p.nodes {
		x := p.colX[p.nodes[i].col]
		top := slotTop[i] + (slotH[i]-nodeH[i])/2
		y0, y1 := int(math.Round(top)), int(math.Round(top+nodeH[i]))
		y1 = max(y1, y0+1)
		p.nodes[i].rect = image.Rect(x, y0, x+g.nodeW, y1)
		p.nodes[i].hit = image.Rect(x-slop, int(math.Floor(slotTop[i])), x+g.nodeW+slop, int(math.Ceil(slotTop[i]+slotH[i])))
		// The node's links start at the top of the mark, nearest neighbor first
		// so that flows cross as little as possible.
		for _, side := range []struct {
			links []int
			far   func(sankeyLinkPlan) int
			set   func(l *sankeyLinkPlan, y float64)
		}{
			{p.from[i], func(l sankeyLinkPlan) int { return l.dst }, func(l *sankeyLinkPlan, y float64) { l.sy = y }},
			{p.into[i], func(l sankeyLinkPlan) int { return l.src }, func(l *sankeyLinkPlan, y float64) { l.ty = y }},
		} {
			slices.SortStableFunc(side.links, func(a, b int) int {
				ca, cb := centre(side.far(p.links[a])), centre(side.far(p.links[b]))
				switch {
				case ca < cb:
					return -1
				case ca > cb:
					return 1
				}
				return a - b
			})
			y := top
			for _, l := range side.links {
				side.set(&p.links[l], y)
				y += p.links[l].th
			}
		}
	}
	for l := range p.links {
		if p.links[l].drawn {
			p.links[l].x0 = float64(p.nodes[p.links[l].src].rect.Max.X)
			p.links[l].x1 = float64(p.nodes[p.links[l].dst].rect.Min.X)
		}
	}
	p.height = int(math.Ceil(height))
	for _, members := range p.columns {
		for _, i := range members {
			p.order = append(p.order, p.nodes[i].sel)
		}
	}
	p.nodeCount = len(p.order)
	for l := range p.links {
		if p.links[l].drawn {
			p.order = append(p.order, p.links[l].sel)
		}
	}
	return p
}

// x is the horizontal position of a link's center line at curve parameter t.
// Both Bézier control points sit at the midpoint between the nodes, so the
// band leaves and enters horizontally.
func (l sankeyLinkPlan) x(t float64) float64 {
	mid, u := (l.x0+l.x1)/2, 1-t
	return l.x0*u*u*u + 3*mid*u*u*t + 3*mid*u*t*t + l.x1*t*t*t
}

// centreY is the vertical center of the band at parameter t.
func (l sankeyLinkPlan) centreY(t float64) float64 {
	from, to, u := l.sy+l.th/2, l.ty+l.th/2, 1-t
	return from*(u*u*u+3*u*u*t) + to*(3*u*t*t+t*t*t)
}

// at finds the curve parameter for a horizontal position. x(t) is monotonic
// because the control points are ordered, so bisection converges.
func (l sankeyLinkPlan) at(x float64) float64 {
	lo, hi := 0.0, 1.0
	for range 24 {
		mid := (lo + hi) / 2
		if l.x(mid) < x {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// outline is the closed band as a polygon.
func (l sankeyLinkPlan) outline() []f32.Point {
	const steps = 32
	points := make([]f32.Point, 0, 2*(steps+1))
	for i := 0; i <= steps; i++ {
		t := float64(i) / steps
		points = append(points, f32.Pt(float32(l.x(t)), float32(l.centreY(t)-l.th/2)))
	}
	for i := steps; i >= 0; i-- {
		t := float64(i) / steps
		points = append(points, f32.Pt(float32(l.x(t)), float32(l.centreY(t)+l.th/2)))
	}
	return points
}

// pick finds the node or link under a point in plot coordinates. A node wins
// over a link that passes behind it.
func (p sankeyPlan) pick(at image.Point) (Selection, bool) {
	for _, members := range p.columns {
		for _, i := range members {
			if at.In(p.nodes[i].hit) || at.In(p.nodes[i].labelHit) {
				return p.nodes[i].sel, true
			}
		}
	}
	x, y := float64(at.X), float64(at.Y)
	for _, l := range p.links {
		if !l.drawn || l.x1 <= l.x0 || x < l.x0 || x > l.x1 {
			continue
		}
		if math.Abs(y-l.centreY(l.at(x))) <= l.th/2+2 {
			return l.sel, true
		}
	}
	return Selection{}, false
}

// move steps the keyboard cursor. Up and Down walk a column, Left and Right
// follow the largest flow to the previous or next column, and Home and End
// reach the first and last node. Links are read from the node readouts and
// the rows; the arrows never land on one, except to leave a link that a
// pointer selected.
func (p sankeyPlan) move(from Selection, name key.Name) (Selection, bool) {
	if p.nodeCount == 0 {
		return Selection{}, false
	}
	switch name {
	case key.NameHome:
		return p.order[0], true
	case key.NameEnd:
		return p.order[p.nodeCount-1], true
	}
	if from.Series == "" {
		if l, ok := p.linkAt[from.Item]; ok && p.links[l].drawn {
			switch name {
			case key.NameLeftArrow, key.NameUpArrow:
				return p.nodes[p.links[l].src].sel, true
			case key.NameRightArrow, key.NameDownArrow:
				return p.nodes[p.links[l].dst].sel, true
			}
			return Selection{}, false
		}
	}
	i, ok := p.nodeAt[from.Item]
	if !ok || from.Series != "" {
		return p.order[0], true
	}
	switch name {
	case key.NameUpArrow, key.NameDownArrow:
		members := p.columns[p.nodes[i].col]
		at := slices.Index(members, i)
		if name == key.NameUpArrow {
			at = max(0, at-1)
		} else {
			at = min(len(members)-1, at+1)
		}
		return p.nodes[members[at]].sel, true
	case key.NameLeftArrow, key.NameRightArrow:
		links, far := p.from[i], func(l sankeyLinkPlan) int { return l.dst }
		if name == key.NameLeftArrow {
			links, far = p.into[i], func(l sankeyLinkPlan) int { return l.src }
		}
		best := -1
		for _, l := range links {
			if best < 0 || p.links[l].value > p.links[best].value {
				best = l
			}
		}
		if best >= 0 {
			return p.nodes[far(p.links[best])].sel, true
		}
	}
	return Selection{}, false
}

// SankeyChart is the widget state of one flow diagram.
type SankeyChart struct {
	interaction
	rows        widget.List
	rowsVisible bool
}

const (
	// sankeyWideBreakpoint is the content width from which the diagram is
	// drawn; narrower windows list the nodes by column instead.
	sankeyWideBreakpoint unit.Dp = 480
	sankeyNodeWidth      unit.Dp = 14
	sankeyNodeGap        unit.Dp = 10
	sankeyMinHeight      unit.Dp = 268
	sankeyMinNode        unit.Dp = 3
	sankeyBarHeight      unit.Dp = 8
	sankeyRowsMaxHeight  unit.Dp = 500
)

// Layout draws the chart at the width of gtx.Constraints.Max and returns the
// selection requests made this frame. Below sankeyWideBreakpoint the flows
// become one row per node with a bar sized by its flow, grouped by column;
// links are then reached through Rows and the node details.
func (c *SankeyChart) Layout(gtx layout.Context, view ui.View, spec SankeySpec) (layout.Dimensions, []Event) {
	width := gtx.Constraints.Max.X
	if len(spec.Nodes) == 0 {
		return drawNotice(gtx, view, image.Pt(width, gtx.Dp(160)), spec.Messages.Empty), nil
	}
	gr := spec.graph()
	if gr.drawn() == 0 {
		return drawNotice(gtx, view, image.Pt(width, gtx.Dp(160)), spec.Messages.Missing), nil
	}
	pad := gtx.Dp(ringPad)
	inner := max(1, width-2*pad)
	if ui.WidthAtLeast(gtx, inner, sankeyWideBreakpoint) {
		c.rowsVisible = false
		return c.layoutPlot(gtx, view, spec, gr, width, pad, inner)
	}
	return c.layoutRows(gtx, view, spec, gr, width, pad, inner)
}

// frame wraps either layout: input, accessibility, readout and focus ring.
// body paints the plan and returns its height; items.pick receives pointer
// positions in chart coordinates, so it removes the plan's origin itself.
func (c *SankeyChart) frame(gtx layout.Context, view ui.View, spec SankeySpec, gr sankeyGraph, width, pad, inner int, items frameItems, body func(gtx layout.Context, active Selection) int) (layout.Dimensions, []Event) {
	colors := view.Colors
	events := c.update(gtx, c, items)
	active := c.active(gtx, c, items.order)
	dims := accessibility.List(gtx, c, spec.Label, func(gtx layout.Context) layout.Dimensions {
		defer op.Offset(image.Pt(pad, pad)).Push(gtx.Ops).Pop()
		hovering := !c.hover.IsZero()
		y := body(gtx, active) + gtx.Dp(ui.Space8)
		rgtx := gtx
		rgtx.Constraints.Max.X = inner
		shown := active
		if shown.IsZero() {
			shown = spec.Selected
		}
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		read := layoutReadout(rgtx, view, spec.Messages, spec.Summary, spec.readout(gr, shown))
		offset.Pop()
		y += read.Size.Y

		size := image.Pt(width, y+2*pad)
		func() {
			defer op.Offset(image.Pt(-pad, -pad)).Push(gtx.Ops).Pop()
			// The compact list handles wheel and drag scrolling beneath the
			// chart's single focus and hit target.
			pass := pointer.PassOp{}.Push(gtx.Ops)
			register(gtx, c, size, hovering)
			pass.Pop()
			if gtx.Focused(c) {
				colors.FocusRing(gtx, image.Rectangle{Min: image.Pt(pad, pad), Max: size.Sub(image.Pt(pad, pad))}, ui.RadiusMD)
			}
		}()
		return layout.Dimensions{Size: size}
	})
	return dims, events
}

func (c *SankeyChart) layoutPlot(gtx layout.Context, view ui.View, spec SankeySpec, gr sankeyGraph, width, pad, inner int) (layout.Dimensions, []Event) {
	colors := view.Colors
	caption := ui.TextCaption
	lineH := int(float32(gtx.Dp(1)) * float32(caption.Size) * caption.LineHeight)
	counts := make([]int, gr.columns)
	for _, d := range gr.depth {
		counts[d]++
	}
	gap := gtx.Dp(sankeyNodeGap)
	height := max(gtx.Dp(sankeyMinHeight), gtx.Dp(spec.Height), slices.Max(counts)*(lineH+gap)*5/4)
	labelSpace := min(max(gtx.Dp(148), inner/5), gtx.Dp(218))
	g := sankeyGeometry{
		width: inner, height: height, nodeW: gtx.Dp(sankeyNodeWidth), gap: gap,
		labelSpace: labelSpace, slotMin: lineH, minNode: gtx.Dp(sankeyMinNode), minLink: max(1, gtx.Dp(1)),
	}
	plan := spec.plan(gr, g)
	labels := spec.plotLabels(gtx, view, plan, g)
	inks := spec.nodeInks(colors, gr)

	// Titles sit above the columns, left aligned with the node marks.
	headerH := 0
	for col := range plan.columns {
		if col < len(spec.ColumnTitles) && spec.ColumnTitles[col] != "" {
			headerH = lineH + gtx.Dp(ui.Space4)
		}
	}
	origin := image.Pt(pad, pad+headerH)
	items := frameItems{
		order: plan.order, selected: spec.Selected, move: plan.move,
		pick: func(at image.Point) (Selection, bool) { return plan.pick(at.Sub(origin)) },
	}
	return c.frame(gtx, view, spec, gr, width, pad, inner, items, func(gtx layout.Context, active Selection) int {
		if headerH > 0 {
			c.drawTitles(gtx, view, spec, plan, g)
		}
		defer op.Offset(image.Pt(0, headerH)).Push(gtx.Ops).Pop()
		c.drawPlot(gtx, view, spec, gr, plan, g, inks, labels, active)
		return headerH + plan.height
	})
}

func (c *SankeyChart) drawTitles(gtx layout.Context, view ui.View, spec SankeySpec, plan sankeyPlan, g sankeyGeometry) {
	for col, x := range plan.colX {
		if col >= len(spec.ColumnTitles) || spec.ColumnTitles[col] == "" {
			continue
		}
		room := g.width - x
		if col+1 < len(plan.colX) {
			room = plan.colX[col+1] - x - gtx.Dp(ui.Space4)
		}
		textBlock{style: ui.TextCaptionStrong, content: spec.ColumnTitles[col], ink: view.Colors.SecondaryInk, maxWidth: max(1, room), maxLines: 1}.draw(gtx, view, image.Pt(x, 0))
	}
}

// labelRoom is the width a node label may use right of its mark. It ends
// before the next column so a label never covers the next node.
func (p sankeyPlan) labelRoom(gtx layout.Context, g sankeyGeometry, col int) int {
	gap := gtx.Dp(ui.Space4)
	if col+1 >= len(p.colX) {
		return g.labelSpace - gap
	}
	return p.colX[col+1] - p.colX[col] - g.nodeW - 2*gap
}

type sankeyPlotLabel struct {
	block textBlock
	at    image.Point
}

// plotLabels prepares the pills and their pointer targets before input is read.
func (s SankeySpec) plotLabels(gtx layout.Context, view ui.View, plan sankeyPlan, g sankeyGeometry) []sankeyPlotLabel {
	labels := make([]sankeyPlotLabel, len(plan.nodes))
	pill := gtx.Dp(ui.Space2)
	for i, n := range plan.nodes {
		node := s.Nodes[i]
		room := plan.labelRoom(gtx, g, n.col)
		if node.HideLabel || node.Label == "" || room < gtx.Dp(32) {
			continue
		}
		block := textBlock{style: ui.TextCaption, content: node.Label, ink: view.Colors.Ink, maxWidth: room - 2*pill, maxLines: 1}
		size := block.measure(gtx, view)
		at := image.Pt(n.rect.Max.X+gtx.Dp(ui.Space4), (n.rect.Min.Y+n.rect.Max.Y)/2-size.Y/2)
		plan.nodes[i].labelHit = image.Rectangle{Min: at.Sub(image.Pt(pill, pill)), Max: at.Add(size).Add(image.Pt(pill, pill))}
		labels[i] = sankeyPlotLabel{block: block, at: at}
	}
	return labels
}

func (c *SankeyChart) drawPlot(gtx layout.Context, view ui.View, spec SankeySpec, gr sankeyGraph, plan sankeyPlan, g sankeyGeometry, inks []color.NRGBA, labels []sankeyPlotLabel, active Selection) {
	colors := view.Colors
	nodePaths, linkPaths := gr.emphasis(active, spec.Selected)
	emphasizing := slices.ContainsFunc(nodePaths, func(level sankeyEmphasis) bool { return level != sankeyUnrelated })
	for i, l := range plan.links {
		if !l.drawn || linkPaths[i] != sankeyUnrelated {
			continue
		}
		alpha := 0.28
		if emphasizing {
			alpha = 0.12
		}
		fillPolygon(gtx, l.outline(), ui.WithAlpha(inks[l.dst], alpha))
	}
	// Paint path bands over the other flows. Their thin outlines keep them
	// distinguishable without hue, including one-pixel flows.
	for level := sankeyActivePath; level <= sankeySelectedPath; level++ {
		for i, l := range plan.links {
			if linkPaths[i] != level {
				continue
			}
			outline := l.outline()
			fillPolygon(gtx, outline, ui.WithAlpha(inks[l.dst], 0.55))
			ink := ui.WithAlpha(colors.Ink, 0.55)
			if level == sankeySelectedPath {
				ink = ui.WithAlpha(colors.Ink, 0.8)
			}
			stroke := max(1, gtx.Dp(1))
			if l.sel == active || l.sel == spec.Selected {
				stroke = gtx.Dp(2)
			}
			strokePolyline(gtx, append(outline, outline[0]), float32(stroke), ink)
		}
	}
	for i, n := range plan.nodes {
		if n.flow > 0 {
			ink := inks[i]
			if emphasizing && nodePaths[i] == sankeyUnrelated {
				ink = ui.WithAlpha(ink, 0.38)
			}
			ui.FillRoundRect(gtx, n.rect, ui.Space2, ink)
		} else {
			// A hollow mark is a node that no drawn flow reaches.
			ui.StrokeRoundRect(gtx, n.rect, ui.Space2, 1, colors.SecondaryInk)
		}
		switch {
		case n.sel == spec.Selected:
			ui.StrokeRoundRect(gtx, n.rect.Inset(-gtx.Dp(2)), ui.Space4, 2, colors.Ink)
		case n.sel == active:
			ui.StrokeRoundRect(gtx, n.rect.Inset(-gtx.Dp(2)), ui.Space4, 2, ui.WithAlpha(colors.Ink, 0.55))
		case nodePaths[i] != sankeyUnrelated:
			ui.StrokeRoundRect(gtx, n.rect.Inset(-gtx.Dp(1)), ui.Space4, 1, colors.Ink)
		}
		if n.sel == active && gtx.Focused(c) {
			ui.StrokeRoundRect(gtx, n.hit, ui.RadiusSM, 2, ui.WithAlpha(colors.Accent, 0.8))
		}
	}
	for i, n := range plan.nodes {
		if item := spec.readout(gr, n.sel); item != nil {
			semanticItem(gtx, n.hit.Union(n.labelHit), itemLabel(*item, spec.Messages))
		}
		if !n.labelHit.Empty() {
			ui.FillRoundRect(gtx, n.labelHit, ui.RadiusSM, ui.WithAlpha(colors.Surface, 0.85))
			labels[i].block.draw(gtx, view, labels[i].at)
		}
	}
	if active.IsZero() {
		return
	}
	item := spec.readout(gr, active)
	if item == nil {
		return
	}
	bounds := image.Rectangle{Max: image.Pt(g.width, plan.height)}
	if i, ok := plan.nodeAt[active.Item]; ok {
		n := plan.nodes[i]
		drawTooltip(gtx, view, bounds, image.Pt(n.rect.Max.X, (n.rect.Min.Y+n.rect.Max.Y)/2), *item)
	} else if l, ok := plan.linkAt[active.Item]; ok && plan.links[l].drawn {
		link := plan.links[l]
		drawTooltip(gtx, view, bounds, image.Pt(int(link.x(0.5)), int(link.centreY(0.5))), *item)
	}
}

// sankeyRowsGeometry are the pixel inputs of the narrow, row-per-node plan.
type sankeyRowsGeometry struct {
	width, inset, valueW, gap int
	barHeight, rowGap         int
	// Row measurements are indexed like the nodes.
	rowHeights, headHeights []int
	// titleHeights are indexed like the columns; zero means no title.
	titleHeights []int
}

type sankeyRowPlan struct {
	sel                    Selection
	node                   int
	row, track             image.Rectangle
	bar                    image.Rectangle
	label, amount, context image.Point
	labelWidth             int
}

type sankeyRowsPlan struct {
	rows    []sankeyRowPlan
	titles  []image.Point
	titleAt []int
	order   []Selection
	height  int
}

func (s SankeySpec) planRows(gr sankeyGraph, g sankeyRowsGeometry) sankeyRowsPlan {
	var p sankeyRowsPlan
	flow := make([]float64, len(s.Nodes))
	var biggest float64
	// A node is as large as the larger of its incoming and outgoing sum.
	sums := make([][2]float64, len(s.Nodes))
	for l, link := range s.Links {
		if gr.ok[l] {
			sums[gr.src[l]][0] += link.Value.Number
			sums[gr.dst[l]][1] += link.Value.Number
		}
	}
	for i := range flow {
		flow[i] = math.Max(sums[i][0], sums[i][1])
		biggest = math.Max(biggest, flow[i])
	}
	y := 0
	track := image.Rect(g.inset, 0, g.width-g.inset, 0)
	for col := range gr.columns {
		if col < len(g.titleHeights) && g.titleHeights[col] > 0 {
			p.titles = append(p.titles, image.Pt(g.inset, y+g.rowGap))
			p.titleAt = append(p.titleAt, col)
			y += g.titleHeights[col]
		}
		for i := range s.Nodes {
			if gr.depth[i] != col {
				continue
			}
			h := g.rowHeights[i]
			row := sankeyRowPlan{sel: Selection{Item: s.Nodes[i].ID}, node: i, row: image.Rect(0, y, g.width, y+h)}
			row.label = image.Pt(g.inset, y+g.rowGap)
			row.labelWidth = max(1, g.width-2*g.inset-g.valueW-g.gap)
			row.amount = image.Pt(g.width-g.inset-g.valueW, y+g.rowGap)
			if i < len(g.headHeights) {
				row.context = row.label.Add(image.Pt(0, g.headHeights[i]))
			}
			barY := y + h - g.rowGap - g.barHeight
			row.track = image.Rect(track.Min.X, barY, track.Max.X, barY+g.barHeight)
			if flow[i] > 0 && biggest > 0 {
				w := max(g.barHeight/2, int(math.Round(flow[i]/biggest*float64(row.track.Dx()))))
				row.bar = image.Rect(row.track.Min.X, barY, min(row.track.Max.X, row.track.Min.X+w), barY+g.barHeight)
			}
			p.rows = append(p.rows, row)
			p.order = append(p.order, row.sel)
			y += h
		}
	}
	p.height = y
	return p
}

func (p sankeyRowsPlan) pick(at image.Point) (Selection, bool) {
	for _, r := range p.rows {
		if at.In(r.row) {
			return r.sel, true
		}
	}
	return Selection{}, false
}

func (c *SankeyChart) layoutRows(gtx layout.Context, view ui.View, spec SankeySpec, gr sankeyGraph, width, pad, inner int) (layout.Dimensions, []Event) {
	colors := view.Colors
	inset := gtx.Dp(ui.Space12)
	gap := gtx.Dp(ui.Space12)
	valueW := 0
	for _, n := range spec.Nodes {
		valueW = max(valueW, textBlock{style: ui.TextLabel, content: n.Text, maxWidth: inner, maxLines: 1}.measure(gtx, view).X)
	}
	valueW = min(valueW, inner/2)
	g := sankeyRowsGeometry{
		width: inner, inset: inset, valueW: valueW, gap: gap,
		barHeight: gtx.Dp(sankeyBarHeight), rowGap: gtx.Dp(ui.Space8),
	}
	labelW := max(1, inner-2*inset-valueW-gap)
	contextW := max(1, inner-2*inset)
	contexts := make([]string, len(spec.Nodes))
	for i, n := range spec.Nodes {
		item := spec.readout(gr, Selection{Item: n.ID})
		if item != nil {
			contexts[i] = strings.Join(item.detail, " · ")
		}
		labelH := textBlock{style: ui.TextBody, content: n.Label, maxWidth: labelW}.measure(gtx, view).Y
		valueH := textBlock{style: ui.TextLabel, content: n.Text, maxWidth: max(1, valueW)}.measure(gtx, view).Y
		contextH := textBlock{style: ui.TextCaption, content: contexts[i], maxWidth: contextW}.measure(gtx, view).Y
		head := max(labelH, valueH)
		g.headHeights = append(g.headHeights, head)
		g.rowHeights = append(g.rowHeights, head+contextH+g.barHeight+gtx.Dp(ui.Space8)+2*g.rowGap)
	}
	for col := range gr.columns {
		h := 0
		if col < len(spec.ColumnTitles) && spec.ColumnTitles[col] != "" {
			h = textBlock{style: ui.TextCaptionStrong, content: spec.ColumnTitles[col], maxWidth: max(1, inner-2*inset)}.measure(gtx, view).Y + 2*g.rowGap
		}
		g.titleHeights = append(g.titleHeights, h)
	}
	plan := spec.planRows(gr, g)
	inks := spec.nodeInks(colors, gr)
	origin := image.Pt(pad, pad)
	viewport := min(plan.height, gtx.Dp(sankeyRowsMaxHeight))
	scrolling := viewport < plan.height
	if !c.rowsVisible || !scrolling {
		c.rows.List = layout.List{Axis: layout.Vertical}
	}
	c.rowsVisible = scrolling
	// Pointer coordinates refer to the previous frame's scroll position.
	hitBounds := image.Rectangle{Max: image.Pt(inner, viewport)}
	offset := c.rows.Position.Offset
	reveal := func(next Selection) {
		i := indexOf(plan.order, next)
		if scrolling && i >= 0 {
			bounds := plan.rows[i].row
			switch {
			case i == 0:
				c.rows.Position.Offset = 0
			case bounds.Dy() >= viewport || bounds.Min.Y < c.rows.Position.Offset:
				c.rows.Position.Offset = bounds.Min.Y
			case bounds.Max.Y > c.rows.Position.Offset+viewport:
				c.rows.Position.Offset = bounds.Max.Y - viewport
			}
		}
	}
	items := frameItems{
		order: plan.order, selected: spec.Selected,
		pick: func(at image.Point) (Selection, bool) {
			at = at.Sub(origin)
			if !at.In(hitBounds) {
				return Selection{}, false
			}
			return plan.pick(at.Add(image.Pt(0, offset)))
		},
		move: func(from Selection, name key.Name) (Selection, bool) {
			next, ok := linearMove(plan.order, from, name)
			if ok && next != from {
				reveal(next)
			}
			return next, ok
		},
	}
	return c.frame(gtx, view, spec, gr, width, pad, inner, items, func(gtx layout.Context, active Selection) int {
		paths, _ := gr.emphasis(active, spec.Selected)
		content := func(gtx layout.Context, _ int) layout.Dimensions {
			for k, at := range plan.titles {
				textBlock{style: ui.TextCaptionStrong, content: spec.ColumnTitles[plan.titleAt[k]], ink: colors.SecondaryInk, maxWidth: contextW}.draw(gtx, view, at)
			}
			for _, row := range plan.rows {
				node := spec.Nodes[row.node]
				switch {
				case row.sel == spec.Selected:
					ui.FillRoundRect(gtx, row.row, ui.RadiusSM, colors.Tint(colors.Accent, 0.10))
					ui.FillRoundRect(gtx, image.Rect(row.row.Min.X, row.row.Min.Y+gtx.Dp(4), row.row.Min.X+gtx.Dp(3), row.row.Max.Y-gtx.Dp(4)), ui.RadiusFull, colors.Accent)
				case row.sel == active:
					ui.FillRoundRect(gtx, row.row, ui.RadiusSM, colors.HoverSurface)
				case paths[row.node] != sankeyUnrelated:
					ui.StrokeRoundRect(gtx, row.row.Inset(gtx.Dp(1)), ui.RadiusSM, 1, ui.WithAlpha(colors.Ink, 0.38))
				}
				textBlock{style: ui.TextBody, content: node.Label, ink: colors.Ink, maxWidth: row.labelWidth}.draw(gtx, view, row.label)
				textBlock{style: ui.TextLabel, content: node.Text, ink: colors.Ink, maxWidth: max(1, valueW), align: text.End}.draw(gtx, view, row.amount)
				textBlock{style: ui.TextCaption, content: contexts[row.node], ink: colors.SecondaryInk, maxWidth: contextW}.draw(gtx, view, row.context)
				ui.FillRoundRect(gtx, row.track, ui.RadiusFull, colors.Tint(colors.Ink, 0.06))
				if !row.bar.Empty() {
					ui.FillRoundRect(gtx, row.bar, ui.RadiusFull, inks[row.node])
				}
				if row.sel == active && gtx.Focused(c) {
					ui.StrokeRoundRect(gtx, row.row, ui.RadiusSM, 2, ui.WithAlpha(colors.Accent, 0.8))
				}
				if item := spec.readout(gr, row.sel); item != nil {
					semanticItem(gtx, row.row, itemLabel(*item, spec.Messages))
				}
			}
			return layout.Dimensions{Size: image.Pt(inner, plan.height)}
		}
		if !scrolling {
			return content(gtx, 0).Size.Y
		}
		gtx.Constraints = layout.Exact(image.Pt(inner, viewport))
		list := view.List(&c.rows)
		list.AnchorStrategy = material.Overlay
		return list.Layout(gtx, 1, content).Size.Y
	})
}
