package accessibility

import "gioui.org/layout"

type TestFrame struct{ frame frame }
type TestElement = element
type TestAction = action

const (
	TestPress = press
	TestFocus = focus
)

func (f *TestFrame) Begin(gtx layout.Context, id uint64, action TestAction) layout.Context {
	return f.frame.start(gtx, request{id: id, action: action})
}

func (f *TestFrame) Snapshot(gtx layout.Context) []TestElement {
	f.frame.snapshot(gtx)
	return f.frame.last
}
