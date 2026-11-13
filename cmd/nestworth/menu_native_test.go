//go:build !server

package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestApplicationMenuLanguageSwitchPreservesRoles(t *testing.T) {
	menu := application.NewMenu().AddRole(application.EditMenu)
	defer menu.Destroy()
	edit, copyItem := menu.FindByRole(application.EditMenu), menu.FindByRole(application.Copy)
	localize := applicationMenuLocalizer(menu)
	for _, tt := range []struct {
		language   settings.Language
		edit, copy string
	}{
		{settings.LanguageZhCN, "编辑", "复制"},
		{settings.LanguageZhTW, "編輯", "拷貝"},
		{settings.LanguageEnglish, "Edit", "Copy"},
	} {
		localize(tt.language)
		if edit.Label() != tt.edit || copyItem.Label() != tt.copy || menu.FindByRole(application.Copy) != copyItem {
			t.Errorf("%s menu = %q / %q, want %q / %q", tt.language, edit.Label(), copyItem.Label(), tt.edit, tt.copy)
		}
	}
}
