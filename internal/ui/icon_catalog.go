package ui

import (
	"bytes"
	"embed"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Lucide SVGs are embedded so icon selection remains fully offline and the
// installed application does not depend on the source repository or a web
// font. The catalog below intentionally contains only concepts useful for
// personal finance rather than the entire Lucide collection.
//
//go:embed resources/lucide/*.svg
var lucideIcons embed.FS

type iconChoice struct {
	key         string
	file        string
	labelKey    string
	categoryKey string
}

var iconChoices = []iconChoice{
	{key: "bank", file: "landmark", labelKey: "icons.bank", categoryKey: "icons.category.banking"},
	{key: "bank-branch", file: "building-2", labelKey: "icons.bankBranch", categoryKey: "icons.category.banking"},
	{key: "building", file: "building", labelKey: "icons.building", categoryKey: "icons.category.banking"},
	{key: "vault", file: "vault", labelKey: "icons.vault", categoryKey: "icons.category.banking"},
	{key: "storage", file: "database", labelKey: "icons.storage", categoryKey: "icons.category.banking"},

	{key: "account", file: "wallet", labelKey: "icons.account", categoryKey: "icons.category.accounts"},
	{key: "wallet", file: "wallet", labelKey: "icons.wallet", categoryKey: "icons.category.accounts"},
	{key: "wallet-cards", file: "wallet-cards", labelKey: "icons.walletCards", categoryKey: "icons.category.accounts"},
	{key: "cash", file: "banknote", labelKey: "icons.cash", categoryKey: "icons.category.accounts"},
	{key: "coins", file: "coins", labelKey: "icons.coins", categoryKey: "icons.category.accounts"},
	{key: "money", file: "hand-coins", labelKey: "icons.money", categoryKey: "icons.category.accounts"},
	{key: "card", file: "credit-card", labelKey: "icons.card", categoryKey: "icons.category.accounts"},
	{key: "credit-card", file: "credit-card", labelKey: "icons.creditCard", categoryKey: "icons.category.accounts"},
	{key: "receipt", file: "receipt", labelKey: "icons.receipt", categoryKey: "icons.category.accounts"},
	{key: "banknote-up", file: "banknote-arrow-up", labelKey: "icons.banknoteUp", categoryKey: "icons.category.accounts"},
	{key: "banknote-down", file: "banknote-arrow-down", labelKey: "icons.banknoteDown", categoryKey: "icons.category.accounts"},

	{key: "brokerage", file: "briefcase-business", labelKey: "icons.brokerage", categoryKey: "icons.category.investment"},
	{key: "brokerage-cash", file: "briefcase", labelKey: "icons.brokerageCash", categoryKey: "icons.category.investment"},
	{key: "investment", file: "chart-candlestick", labelKey: "icons.investment", categoryKey: "icons.category.investment"},
	{key: "stock", file: "chart-candlestick", labelKey: "icons.stock", categoryKey: "icons.category.investment"},
	{key: "market", file: "chart-line", labelKey: "icons.market", categoryKey: "icons.category.investment"},
	{key: "chart", file: "chart-no-axes-combined", labelKey: "icons.chart", categoryKey: "icons.category.investment"},
	{key: "trending-up", file: "trending-up", labelKey: "icons.trendingUp", categoryKey: "icons.category.investment"},
	{key: "percent", file: "percent", labelKey: "icons.percent", categoryKey: "icons.category.investment"},
	{key: "badge-percent", file: "badge-percent", labelKey: "icons.badgePercent", categoryKey: "icons.category.investment"},
	{key: "circle-percent", file: "circle-percent", labelKey: "icons.circlePercent", categoryKey: "icons.category.investment"},

	{key: "currency", file: "currency", labelKey: "icons.currency", categoryKey: "icons.category.currency"},
	{key: "dollar", file: "dollar-sign", labelKey: "icons.dollar", categoryKey: "icons.category.currency"},
	{key: "badge-dollar", file: "badge-dollar-sign", labelKey: "icons.badgeDollar", categoryKey: "icons.category.currency"},
	{key: "circle-dollar", file: "circle-dollar-sign", labelKey: "icons.circleDollar", categoryKey: "icons.category.currency"},
	{key: "euro", file: "euro", labelKey: "icons.euro", categoryKey: "icons.category.currency"},
	{key: "badge-euro", file: "badge-euro", labelKey: "icons.badgeEuro", categoryKey: "icons.category.currency"},
	{key: "yen", file: "japanese-yen", labelKey: "icons.yen", categoryKey: "icons.category.currency"},
	{key: "badge-yen", file: "badge-japanese-yen", labelKey: "icons.badgeYen", categoryKey: "icons.category.currency"},
	{key: "pound", file: "badge-pound-sterling", labelKey: "icons.pound", categoryKey: "icons.category.currency"},
	{key: "badge-pound", file: "badge-pound-sterling", labelKey: "icons.badgePound", categoryKey: "icons.category.currency"},
	{key: "badge-rupee", file: "badge-indian-rupee", labelKey: "icons.badgeRupee", categoryKey: "icons.category.currency"},
	{key: "badge-ruble", file: "badge-russian-ruble", labelKey: "icons.badgeRuble", categoryKey: "icons.category.currency"},
	{key: "badge-franc", file: "badge-swiss-franc", labelKey: "icons.badgeFranc", categoryKey: "icons.category.currency"},
	{key: "bitcoin", file: "bitcoin", labelKey: "icons.bitcoin", categoryKey: "icons.category.currency"},

	{key: "retirement", file: "sunset", labelKey: "icons.retirement", categoryKey: "icons.category.protection"},
	{key: "pension", file: "piggy-bank", labelKey: "icons.pension", categoryKey: "icons.category.protection"},
	{key: "savings", file: "piggy-bank", labelKey: "icons.savings", categoryKey: "icons.category.protection"},
	{key: "insurance", file: "umbrella", labelKey: "icons.insurance", categoryKey: "icons.category.protection"},
	{key: "shield", file: "shield-check", labelKey: "icons.shield", categoryKey: "icons.category.protection"},
	{key: "shield-plus", file: "shield-plus", labelKey: "icons.shieldPlus", categoryKey: "icons.category.protection"},
	{key: "goal", file: "target", labelKey: "icons.goal", categoryKey: "icons.category.protection"},
	{key: "armchair", file: "armchair", labelKey: "icons.armchair", categoryKey: "icons.category.protection"},
	{key: "circle-check", file: "circle-check", labelKey: "icons.circleCheck", categoryKey: "icons.category.protection"},

	{key: "calendar", file: "calendar-days", labelKey: "icons.calendar", categoryKey: "icons.category.general"},
	{key: "calendar-clock", file: "calendar-clock", labelKey: "icons.calendarClock", categoryKey: "icons.category.general"},
	{key: "clock", file: "clock-3", labelKey: "icons.clock", categoryKey: "icons.category.general"},
	{key: "property", file: "building-2", labelKey: "icons.property", categoryKey: "icons.category.general"},
	{key: "liability", file: "triangle-alert", labelKey: "icons.liability", categoryKey: "icons.category.general"},
	{key: "receivable", file: "hand-helping", labelKey: "icons.receivable", categoryKey: "icons.category.general"},
	{key: "home", file: "house", labelKey: "icons.home", categoryKey: "icons.category.general"},
	{key: "folder", file: "folder", labelKey: "icons.folder", categoryKey: "icons.category.general"},
	{key: "document", file: "file-text", labelKey: "icons.document", categoryKey: "icons.category.general"},
	{key: "file", file: "file", labelKey: "icons.file", categoryKey: "icons.category.general"},
	{key: "search", file: "search", labelKey: "icons.search", categoryKey: "icons.category.general"},
	{key: "settings", file: "settings", labelKey: "icons.settings", categoryKey: "icons.category.general"},
	{key: "warning", file: "triangle-alert", labelKey: "icons.warning", categoryKey: "icons.category.general"},
	{key: "info", file: "info", labelKey: "icons.info", categoryKey: "icons.category.general"},
	{key: "download", file: "download", labelKey: "icons.download", categoryKey: "icons.category.general"},
	{key: "upload", file: "upload", labelKey: "icons.upload", categoryKey: "icons.category.general"},
	{key: "visibility", file: "eye", labelKey: "icons.visibility", categoryKey: "icons.category.general"},
	{key: "mail", file: "mail", labelKey: "icons.mail", categoryKey: "icons.category.general"},
	{key: "media", file: "image", labelKey: "icons.media", categoryKey: "icons.category.general"},
	{key: "camera", file: "camera", labelKey: "icons.camera", categoryKey: "icons.category.general"},
	{key: "computer", file: "monitor", labelKey: "icons.computer", categoryKey: "icons.category.general"},
	{key: "grid", file: "layout-grid", labelKey: "icons.grid", categoryKey: "icons.category.general"},
	{key: "list", file: "list", labelKey: "icons.list", categoryKey: "icons.category.general"},
	{key: "history", file: "clock-3", labelKey: "icons.history", categoryKey: "icons.category.general"},
}

type themedLucideResource struct {
	name    string
	content []byte
}

func (r *themedLucideResource) Name() string { return r.name }

func (r *themedLucideResource) Content() []byte {
	foreground := fyneTheme.ForegroundColor()
	red, green, blue, _ := foreground.RGBA()
	value := fmt.Sprintf("#%02x%02x%02x", uint8(red>>8), uint8(green>>8), uint8(blue>>8))
	return bytes.ReplaceAll(r.content, []byte("currentColor"), []byte(value))
}

func (r *themedLucideResource) ThemeColorName() fyne.ThemeColorName {
	return fyneTheme.ColorNameForeground
}

func iconResource(key string) fyne.Resource {
	choice, ok := iconChoiceForKey(key)
	if !ok {
		choice = iconChoices[0]
	}
	content, err := lucideIcons.ReadFile("resources/lucide/" + choice.file + ".svg")
	if err != nil {
		return fyneTheme.Current().Icon(fyneTheme.IconNameWarning)
	}
	return &themedLucideResource{name: "lucide-" + choice.key + ".svg", content: content}
}

func iconChoiceForKey(key string) (iconChoice, bool) {
	for _, choice := range iconChoices {
		if choice.key == key {
			return choice, true
		}
	}
	return iconChoice{}, false
}

func iconKeyValue(key *string, fallback string) string {
	if key == nil || *key == "" {
		return fallback
	}
	return *key
}

func iconPreview(key *string) fyne.CanvasObject {
	return iconPreviewWithFallback(key, domain.DefaultAccountIcon)
}

func iconPreviewWithFallback(key *string, fallback string) fyne.CanvasObject {
	image := canvas.NewImageFromResource(iconResource(iconKeyValue(key, fallback)))
	image.FillMode = canvas.ImageFillContain
	image.SetMinSize(fyne.NewSize(32, 32))
	cell := container.NewGridWrap(fyne.NewSize(40, 40), image)
	return container.New(layout.NewCustomPaddedLayout(8, 4, 8, 4), cell)
}

func showIconPicker(c *Controller, title, current string, selected func(string)) {
	current = iconKeyValue(&current, domain.DefaultAccountIcon)
	sections := make([]fyne.CanvasObject, 0, 12)
	category := ""
	cells := make([]fyne.CanvasObject, 0, 10)
	flush := func() {
		if len(cells) == 0 {
			return
		}
		sections = append(sections,
			widget.NewLabelWithStyle(c.translator.T(category), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewGridWrap(fyne.NewSize(96, 76), cells...),
			widget.NewSeparator(),
		)
		cells = make([]fyne.CanvasObject, 0, 10)
	}
	var picker *dialog.CustomDialog
	for _, choice := range iconChoices {
		if choice.categoryKey != category {
			flush()
			category = choice.categoryKey
		}
		choice := choice
		button := widget.NewButtonWithIcon("", iconResource(choice.key), func() {
			picker.Hide()
			selected(choice.key)
		})
		button.Importance = widget.LowImportance
		if choice.key == current {
			button.Importance = widget.HighImportance
		}
		label := widget.NewLabelWithStyle(c.translator.T(choice.labelKey), fyne.TextAlignCenter, fyne.TextStyle{})
		cells = append(cells, container.NewVBox(container.NewCenter(container.NewGridWrap(fyne.NewSize(48, 44), button)), label))
	}
	flush()
	content := container.NewVScroll(container.NewPadded(container.NewVBox(sections...)))
	content.SetMinSize(fyne.NewSize(0, 340))
	picker = dialog.NewCustom(title, c.translator.T("common.cancel"), content, c.window)
	picker.SetButtons([]fyne.CanvasObject{
		widget.NewButtonWithIcon(c.translator.T("common.cancel"), fyneTheme.Current().Icon(fyneTheme.IconNameCancel), func() {
			picker.Hide()
		}),
	})
	picker.Resize(fittedModalSize(c.window, fyne.NewSize(620, 620), fyne.NewSize(460, 380)))
	picker.Show()
}

func newIconPickerButton(c *Controller, current string, changed func(string)) *widget.Button {
	var button *widget.Button
	button = widget.NewButtonWithIcon(c.translator.T("common.chooseIcon"), iconResource(current), func() {
		showIconPicker(c, c.translator.T("common.chooseIcon"), current, func(key string) {
			current = key
			button.SetIcon(iconResource(key))
			button.Refresh()
			changed(key)
		})
	})
	button.Importance = widget.LowImportance
	return button
}
