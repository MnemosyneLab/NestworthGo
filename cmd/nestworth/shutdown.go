package main

import "sync"

// shutdownStack holds the process cleanup steps that a plain defer in run
// would skip on macOS: AppKit calls exit() right after Wails' OnShutdown
// hooks, so app.Run never returns there. Steps run once, last pushed first,
// and a panicking step does not skip the remaining ones. Every push must
// happen before app.Run.
type shutdownStack struct {
	once  sync.Once
	steps []func()
}

func (s *shutdownStack) push(step func()) {
	s.steps = append(s.steps, step)
}

func (s *shutdownStack) run() {
	s.once.Do(func() {
		for _, step := range s.steps {
			defer step()
		}
	})
}
