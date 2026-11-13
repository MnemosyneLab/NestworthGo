package main

import (
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
)

type lazyPlatform struct {
	mu         sync.Mutex
	app        *application.App
	language   settings.Language
	updateMenu func(settings.Language)
}

func (p *lazyPlatform) setApp(app *application.App) {
	p.mu.Lock()
	p.app = app
	p.mu.Unlock()
}

func (p *lazyPlatform) current() (*application.App, settings.Language) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.app, p.language
}

func (p *lazyPlatform) setLanguage(language settings.Language) {
	p.mu.Lock()
	p.language = language
	updateMenu := p.updateMenu
	p.mu.Unlock()
	if updateMenu != nil {
		application.InvokeAsync(func() {
			// A later settings save can supersede a queued UI update.
			_, current := p.current()
			updateMenu(current)
		})
	}
}

func (p *lazyPlatform) SaveFile(title, filename, filterName, pattern string) (string, error) {
	app, language := p.current()
	if app == nil {
		return "", nil
	}
	dialog := app.Dialog.SaveFile().SetMessage(dialogText(title, language)).SetButtonText(dialogText("save", language)).SetFilename(filename)
	if filterName != "" && pattern != "" {
		dialog = dialog.AddFilter(dialogText(filterName, language), pattern)
	}
	return dialog.PromptForSingleSelection()
}

func (p *lazyPlatform) OpenFile(title, filterName, pattern string) (string, error) {
	app, language := p.current()
	if app == nil {
		return "", nil
	}
	message := dialogText(title, language)
	dialog := app.Dialog.OpenFile().SetTitle(message).SetMessage(message).SetButtonText(dialogText("open", language)).CanChooseFiles(true)
	if filterName != "" && pattern != "" {
		dialog = dialog.AddFilter(dialogText(filterName, language), pattern)
	}
	return dialog.PromptForSingleSelection()
}

func (p *lazyPlatform) ConfirmReplace(fileName string) (bool, error) {
	app, language := p.current()
	if app == nil {
		return false, nil
	}
	done := make(chan bool, 1)
	dialog := app.Dialog.Question().SetTitle(dialogText("replace_title", language)).SetMessage(fmt.Sprintf(dialogText("replace_message", language), fileName))
	replace := dialog.AddButton(dialogText("replace", language))
	cancel := dialog.AddButton(dialogText("cancel", language))
	replace.OnClick(func() { done <- true })
	cancel.OnClick(func() { done <- false }).SetAsCancel()
	dialog.SetDefaultButton(cancel)
	dialog.Show()
	return <-done, nil
}

func (p *lazyPlatform) Quit() {
	if app, _ := p.current(); app != nil {
		app.Quit()
	}
}

type localizedText struct {
	en, zhCN, zhTW, ja string
}

func (text localizedText) forLanguage(language settings.Language) string {
	switch language {
	case settings.LanguageZhCN:
		return text.zhCN
	case settings.LanguageZhTW:
		return text.zhTW
	case settings.LanguageJa:
		return text.ja
	default:
		return text.en
	}
}

var dialogLabels = map[string]localizedText{
	native.BackupTitle:  {"Back up data", "备份数据", "備份資料", "データをバックアップ"},
	native.ExportTitle:  {"Export data", "导出数据", "匯出資料", "データをエクスポート"},
	native.RestoreTitle: {"Restore from backup", "从备份恢复", "從備份還原", "バックアップから復元"},
	native.BackupFilter: {"Nestworth Backup", "Nestworth 备份", "Nestworth 備份", "Nestworth バックアップ"},
	native.ExportFilter: {"Nestworth JSON", "Nestworth JSON", "Nestworth JSON", "Nestworth JSON"},
	"save":              {"Save", "保存", "儲存", "保存"},
	"open":              {"Open", "打开", "開啟", "開く"},
	"replace_title":     {"Replace existing file?", "替换现有文件？", "取代現有檔案？", "既存のファイルを置き換えますか？"},
	"replace_message":   {"%s already exists. Replace it?", "%s 已存在。要替换它吗？", "%s 已存在。要取代它嗎？", "%s は既に存在します。置き換えますか？"},
	"replace":           {"Replace", "替换", "取代", "置き換える"},
	"cancel":            {"Cancel", "取消", "取消", "キャンセル"},
}

func dialogText(key string, language settings.Language) string {
	if text, ok := dialogLabels[key]; ok {
		return text.forLanguage(language)
	}
	return key
}
