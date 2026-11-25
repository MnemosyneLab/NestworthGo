package charts

import (
	"testing"
)

func TestApplyHonorsEventsInOrder(t *testing.T) {
	a, b := Selection{Item: "a"}, Selection{Item: "b"}
	if got := Apply(a, []Event{{Kind: EventSelect, Selection: b}}); got != b {
		t.Fatalf("select = %v", got)
	}
	if got := Apply(a, []Event{{Kind: EventSelect, Selection: b}, {Kind: EventClear, Selection: b}}); !got.IsZero() {
		t.Fatalf("clear = %v", got)
	}
	if got := Apply(a, nil); got != a {
		t.Fatalf("no events changed the selection to %v", got)
	}
}
