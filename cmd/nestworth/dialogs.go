package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type lazyPlatform struct {
	mu  sync.Mutex
	app *application.App
}

func (p *lazyPlatform) setApp(app *application.App) {
	p.mu.Lock()
	p.app = app
	p.mu.Unlock()
}

func (p *lazyPlatform) current() *application.App {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.app
}

func (p *lazyPlatform) SaveFile(title, filename, filterName, pattern string) (string, error) {
	app := p.current()
	if app == nil {
		return "", nil
	}
	dialog := app.Dialog.SaveFile().SetMessage(title).SetFilename(filename)
	if filterName != "" && pattern != "" {
		dialog = dialog.AddFilter(filterName, pattern)
	}
	return dialog.PromptForSingleSelection()
}

func (p *lazyPlatform) OpenFile(title, filterName, pattern string) (string, error) {
	app := p.current()
	if app == nil {
		return "", nil
	}
	dialog := app.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true)
	if filterName != "" && pattern != "" {
		dialog = dialog.AddFilter(filterName, pattern)
	}
	return dialog.PromptForSingleSelection()
}

func (p *lazyPlatform) ConfirmReplace(fileName string) (bool, error) {
	app := p.current()
	if app == nil {
		return false, nil
	}
	done := make(chan bool, 1)
	dialog := app.Dialog.Question().SetTitle("Replace existing file?").SetMessage(fileName + " already exists. Replace it?")
	replace := dialog.AddButton("Replace")
	cancel := dialog.AddButton("Cancel")
	replace.OnClick(func() { done <- true })
	cancel.OnClick(func() { done <- false }).SetAsCancel()
	dialog.SetDefaultButton(cancel)
	dialog.Show()
	return <-done, nil
}

func (p *lazyPlatform) Quit() {
	if app := p.current(); app != nil {
		app.Quit()
	}
}
