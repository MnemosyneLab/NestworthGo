package assistant

import "testing"

func TestSelectionDoesNotGrantOrRevokePermission(t *testing.T) {
	for _, full := range []bool{false, true} {
		r, id, _, _ := capabilityRuntime(t, &capabilityTestSession{})
		r.fullEnabled = full
		before := r.state.Revision
		if err := r.SelectModel(t.Context(), r.epoch, before, ModelSelection{Model: "mock"}); err != nil {
			t.Fatal(err)
		}
		if r.fullEnabled != full || r.state.Revision != before || r.active[id] == nil {
			t.Fatal("preference changed authority or active run")
		}
		if err := r.SelectModel(t.Context(), r.epoch, before-1, ModelSelection{Model: "mock"}); err != ErrStale {
			t.Fatal(err)
		}
	}
}
