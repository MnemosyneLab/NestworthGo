package desktop

import (
	"context"
	"errors"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type testPreferenceStore struct {
	value preferences.Values
	err   error
}

func (m *testPreferenceStore) Load(context.Context) (preferences.Values, error) {
	return m.value, m.err
}
func (m *testPreferenceStore) Save(_ context.Context, _, after preferences.Values) (preferences.Values, error) {
	if m.err == nil {
		m.value = after
	}
	return m.value, m.err
}
func (m *testPreferenceStore) Reset(ctx context.Context) (preferences.Values, error) {
	return m.Save(ctx, m.value, preferences.Default())
}

func newTestPreferenceState(t *testing.T) *preferenceState {
	t.Helper()
	values := preferences.Default()
	values.Language, values.Appearance = settings.LanguageZhCN, settings.AppearanceLight
	p := newPreferenceState(t.Context(), &testPreferenceStore{value: values}, func() {})
	p.systemChanged.Store(false)
	p.systemLanguage = settings.LanguageEnglish
	p.systemTimezone = "UTC"
	p.saved, p.ready = values, true
	t.Cleanup(p.wait)
	return p
}

func TestPreferenceWritesOnlyPublishConfirmedValues(t *testing.T) {
	p := newTestPreferenceState(t)
	before := p.saved
	after := before
	after.Appearance = settings.AppearanceDark
	store := p.store.(*testPreferenceStore)
	store.err = errors.New("write failed")
	completions := 0
	done := func() { completions++ }
	p.save(before, after, done)
	p.reset(done)
	p.wait()
	p.update()
	if p.err == nil || p.busy || p.saved != before || completions != 0 {
		t.Fatal("failed or overlapping writes changed the saved appearance")
	}
	store.err = nil
	p.save(before, after, done)
	p.wait()
	p.update()
	if p.err != nil || p.saved != after || !p.appearance().dark || completions != 1 {
		t.Fatal("the retried save did not publish the confirmed appearance")
	}
}
