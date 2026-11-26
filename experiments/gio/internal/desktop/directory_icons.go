package desktop

import (
	"bytes"
	_ "embed"
	"image/color"
	"image/png"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var (
	//go:embed directory_icons/btc.png
	btcIcon []byte
	//go:embed directory_icons/eth.png
	ethIcon []byte
	//go:embed directory_icons/sol.png
	solIcon []byte
	//go:embed directory_icons/usdc.png
	usdcIcon []byte
)

type directoryImageGlyph struct{ image paint.ImageOp }

func (g directoryImageGlyph) Layout(gtx layout.Context, _ color.NRGBA) layout.Dimensions {
	return widget.Image{Src: g.image, Fit: widget.Contain}.Layout(gtx)
}

func directoryImage(data []byte) ui.Glyph {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return directoryImageGlyph{image: paint.NewImageOp(img)}
}

var directoryGlyphs, directoryIconChoices = func() (map[string]ui.Glyph, map[i18n.Language][]ui.Choice) {
	glyphs := make(map[string]ui.Glyph)
	localized := make(map[i18n.Language][]ui.Choice)
	for _, entry := range i18n.DirectoryIcons() {
		glyph := loadDirectoryGlyph(entry.ID)
		glyphs[entry.ID] = glyph
		searchText := entry.SearchText()
		for _, language := range []i18n.Language{i18n.English, i18n.SimplifiedChinese, i18n.TraditionalChinese, i18n.Japanese} {
			localized[language] = append(localized[language], ui.Choice{
				ID: entry.ID, Label: entry.Label(language), Glyph: glyph, SearchText: searchText,
			})
		}
	}
	return glyphs, localized
}()

func (p *directoryList) glyph(key string, kind domain.InstitutionType) ui.Glyph {
	return directoryGlyph(key, p.fallbackIcon(kind))
}

func (p *directoryList) fallbackIcon(kind domain.InstitutionType) string {
	return institutionFallbackIcon(kind, p.defaultIcon)
}

func institutionFallbackIcon(kind domain.InstitutionType, defaultIcon string) string {
	if icon := domain.DefaultIconForInstitutionType(kind); icon != "" {
		return icon
	}
	return defaultIcon
}

// directoryGlyph resolves an icon key to a bank logo or a directory glyph and
// falls back to the given key, so every institution shows a mark.
func directoryGlyph(key, fallback string) ui.Glyph {
	if logo, ok := directoryBankLogos[key]; ok {
		return logo.Glyph
	}
	if glyph := directoryGlyphs[key]; glyph != nil {
		return glyph
	}
	return directoryGlyphs[fallback]
}

func loadDirectoryGlyph(key string) ui.Glyph {
	var icon []byte
	switch key {
	case "account", "wallet":
		icon = icons.ActionAccountBalanceWallet
	case "wallet-cards":
		icon = icons.ActionViewCarousel
	case "cash", "money", "brokerage-cash", "liability", "badge-dollar":
		icon = icons.EditorAttachMoney
	case "coins", "savings":
		icon = icons.ActionToll
	case "card", "credit-card":
		icon = icons.ActionCreditCard
	case "receipt", "receivable":
		icon = icons.ActionReceipt
	case "banknote-up":
		icon = icons.NavigationArrowUpward
	case "banknote-down":
		icon = icons.NavigationArrowDownward
	case "term-deposit":
		icon = icons.ImageTimer
	case "bank", "bank-branch", "landmark":
		icon = icons.ActionAccountBalance
	case "building":
		icon = icons.CommunicationBusiness
	case "vault":
		icon = icons.ActionLock
	case "storage":
		icon = icons.ContentArchive
	case "brokerage", "market", "chart":
		icon = icons.ActionTimeline
	case "investment", "stock", "trending-up":
		icon = icons.ActionTrendingUp
	case "percent":
		icon = icons.ActionDonutSmall
	case "badge-percent":
		icon = icons.ActionStars
	case "circle-percent":
		icon = icons.ActionDonutLarge
	case "fund":
		icon = icons.MapsLayers
	case "bond", "document", "file":
		icon = icons.ActionDescription
	case "car":
		icon = icons.MapsDirectionsCar
	case "gem":
		icon = icons.ActionGrade
	case "property", "home":
		icon = icons.ActionHome
	case "land":
		icon = icons.MapsTerrain
	case "commercial-property":
		icon = icons.ActionStore
	case "collectible":
		icon = icons.ActionBookmark
	case "lending":
		icon = icons.ActionRedeem
	case "retirement", "pension", "armchair":
		icon = icons.ActionEventSeat
	case "insurance", "shield":
		icon = icons.ActionVerifiedUser
	case "shield-plus":
		icon = icons.HardwareSecurity
	case "goal":
		icon = icons.ActionTrackChanges
	case "circle-check":
		icon = icons.ActionCheckCircle
	case "bitcoin", "currency", "dollar", "circle-dollar":
		icon = icons.EditorMonetizationOn
	case "digital-asset":
		icon = icons.ActionExtension
	case "euro", "badge-euro":
		icon = icons.ActionEuroSymbol
	case "yen":
		icon = icons.ActionTranslate
	case "badge-yen":
		icon = icons.ActionGTranslate
	case "badge-pound", "badge-rupee", "badge-ruble", "badge-franc":
		icon = icons.ActionLanguage
	case "calendar":
		icon = icons.ActionDateRange
	case "calendar-clock":
		icon = icons.ActionEvent
	case "user":
		icon = icons.SocialPersonOutline
	case "pie-chart":
		icon = icons.EditorPieChart
	case "folder":
		icon = icons.FileFolder
	case "search":
		icon = icons.ActionSearch
	case "settings":
		icon = icons.ActionSettings
	case "warning":
		icon = icons.AlertWarning
	case "info":
		icon = icons.ActionInfoOutline
	case "download":
		icon = icons.FileFileDownload
	case "upload":
		icon = icons.FileFileUpload
	case "visibility":
		icon = icons.ActionVisibility
	case "mail":
		icon = icons.ContentMail
	case "camera":
		icon = icons.ImagePhotoCamera
	case "computer":
		icon = icons.HardwareDesktopWindows
	case "grid":
		icon = icons.ActionDashboard
	case "list":
		icon = icons.ActionList
	case "history":
		icon = icons.ActionHistory
	case "crypto-logo:btc":
		return directoryImage(btcIcon)
	case "crypto-logo:eth":
		return directoryImage(ethIcon)
	case "crypto-logo:sol":
		return directoryImage(solIcon)
	case "crypto-logo:usdc":
		return directoryImage(usdcIcon)
	default:
		return nil
	}
	return ui.MustIcon(icon)
}
