//go:build !server

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/waltwang/nestworth-go/internal/settings"
)

var menuLabels = map[application.Role]localizedText{
	application.FileMenu:           {zhCN: "文件", zhTW: "檔案", ja: "ファイル"},
	application.EditMenu:           {zhCN: "编辑", zhTW: "編輯", ja: "編集"},
	application.ViewMenu:           {zhCN: "视图", zhTW: "顯示", ja: "表示"},
	application.WindowMenu:         {zhCN: "窗口", zhTW: "視窗", ja: "ウインドウ"},
	application.HelpMenu:           {zhCN: "帮助", zhTW: "輔助說明", ja: "ヘルプ"},
	application.ServicesMenu:       {zhCN: "服务", zhTW: "服務", ja: "サービス"},
	application.SpeechMenu:         {zhCN: "朗读", zhTW: "朗讀", ja: "読み上げ"},
	application.About:              {zhCN: "关于 Nestworth", zhTW: "關於 Nestworth", ja: "Nestworthについて"},
	application.Hide:               {zhCN: "隐藏 Nestworth", zhTW: "隱藏 Nestworth", ja: "Nestworthを隠す"},
	application.HideOthers:         {zhCN: "隐藏其他应用", zhTW: "隱藏其他 App", ja: "ほかを隠す"},
	application.UnHide:             {zhCN: "显示全部", zhTW: "顯示全部", ja: "すべてを表示"},
	application.Quit:               {zhCN: "退出 Nestworth", zhTW: "結束 Nestworth", ja: "Nestworthを終了"},
	application.CloseWindow:        {zhCN: "关闭", zhTW: "關閉", ja: "閉じる"},
	application.Undo:               {zhCN: "撤销", zhTW: "復原", ja: "取り消す"},
	application.Redo:               {zhCN: "重做", zhTW: "重做", ja: "やり直す"},
	application.Cut:                {zhCN: "剪切", zhTW: "剪下", ja: "カット"},
	application.Copy:               {zhCN: "复制", zhTW: "拷貝", ja: "コピー"},
	application.Paste:              {zhCN: "粘贴", zhTW: "貼上", ja: "ペースト"},
	application.PasteAndMatchStyle: {zhCN: "粘贴并匹配样式", zhTW: "貼上並符合樣式", ja: "ペーストしてスタイルを合わせる"},
	application.Delete:             {zhCN: "删除", zhTW: "刪除", ja: "削除"},
	application.SelectAll:          {zhCN: "全选", zhTW: "全選", ja: "すべてを選択"},
	application.StartSpeaking:      {zhCN: "开始朗读", zhTW: "開始朗讀", ja: "読み上げを開始"},
	application.StopSpeaking:       {zhCN: "停止朗读", zhTW: "停止朗讀", ja: "読み上げを停止"},
	application.Reload:             {zhCN: "重新加载", zhTW: "重新載入", ja: "再読み込み"},
	application.ForceReload:        {zhCN: "强制重新加载", zhTW: "強制重新載入", ja: "強制的に再読み込み"},
	application.OpenDevTools:       {zhCN: "打开开发者工具", zhTW: "開啟開發人員工具", ja: "開発者ツールを開く"},
	application.ResetZoom:          {zhCN: "实际大小", zhTW: "實際大小", ja: "実際のサイズ"},
	application.ZoomIn:             {zhCN: "放大", zhTW: "放大", ja: "拡大"},
	application.ZoomOut:            {zhCN: "缩小", zhTW: "縮小", ja: "縮小"},
	application.ToggleFullscreen:   {zhCN: "切换全屏", zhTW: "切換全螢幕", ja: "フルスクリーンを切り替える"},
	application.Minimise:           {zhCN: "最小化", zhTW: "最小化", ja: "しまう"},
	application.Zoom:               {zhCN: "缩放", zhTW: "縮放", ja: "拡大／縮小"},
	application.Front:              {zhCN: "全部置于顶层", zhTW: "將所有視窗移到最前面", ja: "すべてを手前に移動"},
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
		labels[item] = localizedText{item.Label(), "了解更多", "進一步瞭解", "詳しく見る"}
	}
	return func(language settings.Language) {
		for item, text := range labels {
			item.SetLabel(text.forLanguage(language))
		}
	}
}
