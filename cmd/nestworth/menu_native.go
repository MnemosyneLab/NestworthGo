//go:build !server

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/waltwang/nestworth-go/internal/settings"
)

var menuLabels = map[application.Role]localizedText{
	application.FileMenu:           {zhCN: "文件", zhTW: "檔案"},
	application.EditMenu:           {zhCN: "编辑", zhTW: "編輯"},
	application.ViewMenu:           {zhCN: "视图", zhTW: "顯示"},
	application.WindowMenu:         {zhCN: "窗口", zhTW: "視窗"},
	application.HelpMenu:           {zhCN: "帮助", zhTW: "輔助說明"},
	application.ServicesMenu:       {zhCN: "服务", zhTW: "服務"},
	application.SpeechMenu:         {zhCN: "朗读", zhTW: "朗讀"},
	application.About:              {zhCN: "关于 Nestworth", zhTW: "關於 Nestworth"},
	application.Hide:               {zhCN: "隐藏 Nestworth", zhTW: "隱藏 Nestworth"},
	application.HideOthers:         {zhCN: "隐藏其他应用", zhTW: "隱藏其他 App"},
	application.UnHide:             {zhCN: "显示全部", zhTW: "顯示全部"},
	application.Quit:               {zhCN: "退出 Nestworth", zhTW: "結束 Nestworth"},
	application.CloseWindow:        {zhCN: "关闭", zhTW: "關閉"},
	application.Undo:               {zhCN: "撤销", zhTW: "復原"},
	application.Redo:               {zhCN: "重做", zhTW: "重做"},
	application.Cut:                {zhCN: "剪切", zhTW: "剪下"},
	application.Copy:               {zhCN: "复制", zhTW: "拷貝"},
	application.Paste:              {zhCN: "粘贴", zhTW: "貼上"},
	application.PasteAndMatchStyle: {zhCN: "粘贴并匹配样式", zhTW: "貼上並符合樣式"},
	application.Delete:             {zhCN: "删除", zhTW: "刪除"},
	application.SelectAll:          {zhCN: "全选", zhTW: "全選"},
	application.StartSpeaking:      {zhCN: "开始朗读", zhTW: "開始朗讀"},
	application.StopSpeaking:       {zhCN: "停止朗读", zhTW: "停止朗讀"},
	application.Reload:             {zhCN: "重新加载", zhTW: "重新載入"},
	application.ForceReload:        {zhCN: "强制重新加载", zhTW: "強制重新載入"},
	application.OpenDevTools:       {zhCN: "打开开发者工具", zhTW: "開啟開發人員工具"},
	application.ResetZoom:          {zhCN: "实际大小", zhTW: "實際大小"},
	application.ZoomIn:             {zhCN: "放大", zhTW: "放大"},
	application.ZoomOut:            {zhCN: "缩小", zhTW: "縮小"},
	application.ToggleFullscreen:   {zhCN: "切换全屏", zhTW: "切換全螢幕"},
	application.Minimise:           {zhCN: "最小化", zhTW: "最小化"},
	application.Zoom:               {zhCN: "缩放", zhTW: "縮放"},
	application.Front:              {zhCN: "全部置于顶层", zhTW: "將所有視窗移到最前面"},
}

// Use the Wails roles to retain native actions, keyboard shortcuts and submenus.
func configureApplicationMenu(app *application.App, language settings.Language) func(settings.Language) {
	menu := application.DefaultApplicationMenu()
	localize := applicationMenuLocalizer(menu)
	apply := func(language settings.Language) {
		localize(language)
		// Reinstall the menu so AppKit refreshes its top-level titles as well.
		app.Menu.SetApplicationMenu(menu)
	}
	apply(language)
	return apply
}

func applicationMenuLocalizer(menu *application.Menu) func(settings.Language) {
	labels := make(map[*application.MenuItem]localizedText)
	for role, text := range menuLabels {
		if item := menu.FindByRole(role); item != nil {
			text.en = item.Label()
			labels[item] = text
		}
	}
	// Some platforms add the same role to two submenus.
	for _, parentRole := range []application.Role{application.FileMenu, application.WindowMenu} {
		if parent := menu.FindByRole(parentRole); parent != nil {
			for _, role := range []application.Role{application.Quit, application.CloseWindow} {
				if item := parent.GetSubmenu().FindByRole(role); item != nil {
					text := menuLabels[role]
					text.en = item.Label()
					labels[item] = text
				}
			}
		}
	}
	if item := menu.FindByLabel("Learn More"); item != nil {
		labels[item] = localizedText{item.Label(), "了解更多", "進一步瞭解"}
	}
	return func(language settings.Language) {
		for item, text := range labels {
			item.SetLabel(text.forLanguage(language))
		}
	}
}
