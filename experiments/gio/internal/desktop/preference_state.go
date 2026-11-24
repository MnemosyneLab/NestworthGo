package desktop

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type preferenceStore interface {
	Load(context.Context) (preferences.Values, error)
	Save(context.Context, preferences.Values, preferences.Values) (preferences.Values, error)
	Reset(context.Context) (preferences.Values, error)
}

type preferenceResult struct {
	values    preferences.Values
	err       error
	onSuccess func()
}

// The window owns the saved snapshot. The settings form owns its unsubmitted
// fields. Both the form and the sidebar publish through this one writer.
type preferenceState struct {
	ctx             context.Context
	store           preferenceStore
	invalidate      func()
	saved           preferences.Values
	ready, busy     bool
	err             error
	results         chan preferenceResult
	workers         sync.WaitGroup
	systemChanged   atomic.Bool
	systemLanguage  settings.Language
	systemTimezone  string
	systemDark      bool
	setLanguage     func(string)
	appliedLanguage settings.Language
}

func newPreferenceState(ctx context.Context, store preferenceStore, invalidate func()) *preferenceState {
	p := &preferenceState{ctx: ctx, store: store, invalidate: invalidate,
		saved: preferences.Default(), results: make(chan preferenceResult, 1)}
	p.systemChanged.Store(true)
	return p
}

func (p *preferenceState) systemUpdate() {
	p.systemChanged.Store(true)
	p.invalidate()
}

func (p *preferenceState) update() {
	if p.systemChanged.Swap(false) {
		p.systemLanguage = preferences.SystemLanguage()
		p.systemTimezone = preferences.SystemTimezone()
		p.systemDark = preferences.SystemDark()
	}
	select {
	case result := <-p.results:
		p.busy, p.err = false, result.err
		if result.err == nil {
			p.saved, p.ready = result.values, true
			if result.onSuccess != nil {
				result.onSuccess()
			}
		}
	default:
	}
	resolved := settings.ResolveLanguage(p.saved.Language, string(p.systemLanguage))
	if p.ready && p.setLanguage != nil && resolved != p.appliedLanguage {
		p.setLanguage(string(resolved))
		p.appliedLanguage = resolved
	}
}

func (p *preferenceState) reload() {
	p.start(func() (preferences.Values, error) { return p.store.Load(p.ctx) }, nil)
}

func (p *preferenceState) save(before, after preferences.Values, onSuccess func()) {
	if p.ready {
		p.start(func() (preferences.Values, error) { return p.store.Save(p.ctx, before, after) }, onSuccess)
	}
}

func (p *preferenceState) reset(onSuccess func()) {
	if p.ready {
		p.start(func() (preferences.Values, error) { return p.store.Reset(p.ctx) }, onSuccess)
	}
}

func (p *preferenceState) start(work func() (preferences.Values, error), onSuccess func()) {
	if p.busy || p.ctx.Err() != nil {
		return
	}
	p.busy, p.err = true, nil
	p.workers.Go(func() {
		values, err := work()
		p.results <- preferenceResult{values, err, onSuccess}
		p.invalidate()
	})
}

func (p *preferenceState) wait() { p.workers.Wait() }

func (p *preferenceState) appearance() sessionPreferences {
	accent := 0
	for i, value := range settings.AllAccents() {
		if value == p.saved.Accent {
			accent = i
			break
		}
	}
	return sessionPreferences{
		language:    i18n.Language(settings.ResolveLanguage(p.saved.Language, string(p.systemLanguage))),
		dark:        p.saved.Appearance == settings.AppearanceDark || (p.saved.Appearance == settings.AppearanceSystem && p.systemDark),
		accentIndex: accent,
	}
}
