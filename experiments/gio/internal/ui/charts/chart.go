package charts

import (
	"strconv"
)

// ID is a stable identifier a caller assigns to a series or an item. Charts
// never invent identifiers and never derive one from an index, so a selection
// survives a refresh that reorders or adds items.
type ID string

// Selection names one chart item. Series is used by the line chart, where an
// item is one point of one series; the other charts leave it empty.
type Selection struct{ Series, Item ID }

// IsZero reports whether s selects nothing.
func (s Selection) IsZero() bool { return s == Selection{} }

// EventKind says what the user asked for.
type EventKind uint8

const (
	// EventSelect asks the owner to select Event.Selection. It is sent for a
	// press or for Return and Space on the keyboard cursor.
	EventSelect EventKind = iota + 1
	// EventClear asks the owner to clear the selection. It is sent when the
	// selected item is chosen again, and for Escape while an item is selected.
	EventClear
)

// Event is a request to change the selection. Selection is controlled: a chart
// only draws the Selected value its spec carries, so the owner decides whether
// to honor an event, for example while a detail view is still loading.
type Event struct {
	Kind      EventKind
	Selection Selection
}

// Apply returns the selection after honoring events in order. Owners that
// accept every request call it once per frame.
func Apply(current Selection, events []Event) Selection {
	for _, e := range events {
		switch e.Kind {
		case EventSelect:
			current = e.Selection
		case EventClear:
			current = Selection{}
		}
	}
	return current
}

// Value is a number to plot together with whether it exists. The zero Value is
// missing, never zero: a caller has to write Num(0) to plot a real zero. NaN and
// infinities are missing as well, so an unavailable quote can never be drawn as
// a flat line at the axis.
//
// The number only positions marks. Every amount, percentage and date a person
// reads comes from the caller's text fields.
type Value struct {
	Number  float64
	Present bool
}

// Num is a present value. A non-finite number yields a missing value.
func Num(v float64) Value { return Value{Number: v, Present: finite(v)} }

// NoValue is the missing value.
var NoValue = Value{}

// Valid reports whether the value can be drawn.
func (v Value) Valid() bool { return v.Present && finite(v.Number) }

// Tone selects the semantic color of a bar. The zero tone follows the sign
// of the geometry; a caller overrides it where the sign is not the meaning,
// for example a residual that is neither a gain nor a loss.
type Tone uint8

const (
	ToneAuto Tone = iota
	ToneGain
	ToneLoss
	ToneNeutral
	ToneWarning
)

// Messages carries the caller's localized words. The charts hold no text of
// their own, so they follow the application language and the caller's rules for
// currency, percentages and dates.
type Messages struct {
	// Empty is shown when a chart has no items at all.
	Empty string
	// Missing is shown for an item that has no value and no text of its own,
	// and when items exist but none can be drawn.
	Missing string
	// Selected marks the selected item in text, so selection never relies on
	// color alone.
	Selected string
}

// TextRow is the plain-text form of one chart item. Pages render it as a data
// table or hand it to assistive technology; it carries exactly what the chart
// shows in its readout.
type TextRow struct {
	Series, ID ID
	Label      string
	// Value is the caller's exact text for the item, or the missing message.
	Value    string
	Detail   []string
	Missing  bool
	Selected bool
}

// valueText is the text shown for an item: the caller's exact text when it has
// any, otherwise the missing message for a missing value, otherwise the plain
// number. The fallback is a diagnostic aid; callers supply exact text.
func valueText(v Value, text string, m Messages) (string, bool) {
	if text != "" {
		return text, !v.Valid()
	}
	if !v.Valid() {
		return m.Missing, true
	}
	return strconv.FormatFloat(v.Number, 'g', -1, 64), false
}

// ProblemKind says what is wrong with an identifier.
type ProblemKind uint8

const (
	// ProblemEmptyID means an item or series has no ID, so it can never be
	// selected: the empty Selection means "nothing".
	ProblemEmptyID ProblemKind = iota + 1
	// ProblemRepeatedID means two items or two series share an ID, so they
	// would select together.
	ProblemRepeatedID
	// ProblemUnknownNode means a Sankey link names a source or target that is
	// not among the nodes, so the link is not drawn.
	ProblemUnknownNode
	// ProblemCycle means a Sankey link joins a node to itself or lies on or
	// after a cycle, so the link is not drawn.
	ProblemCycle
)

// Problem locates one invalid identifier. Series is empty for the items of
// charts that have no series, and for a problem with a series ID itself.
type Problem struct {
	Kind   ProblemKind
	Series ID
	ID     ID
}

// idCheck collects Problems while a spec is validated.
type idCheck struct {
	seen     map[ID]bool
	problems []Problem
}

func (c *idCheck) add(series, id ID) {
	if c.seen == nil {
		c.seen = map[ID]bool{}
	}
	switch {
	case id == "":
		c.problems = append(c.problems, Problem{Kind: ProblemEmptyID, Series: series, ID: id})
	case c.seen[id]:
		c.problems = append(c.problems, Problem{Kind: ProblemRepeatedID, Series: series, ID: id})
	}
	c.seen[id] = true
}
