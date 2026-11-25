package desktop

import (
	"strings"

	"gioui.org/op/paint"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/banklogos"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

var directoryBankLogos, institutionIconChoices = func() (map[string]ui.Choice, map[i18n.Language][]ui.Choice) {
	presentations, tiles, err := banklogos.Load()
	if err != nil {
		panic(err)
	}
	logos := make(map[string]ui.Choice)
	var choices []ui.Choice
	for i, presentation := range presentations {
		choice := ui.Choice{
			ID: presentation.Key, Label: presentation.Name,
			Glyph:      directoryImageGlyph{image: paint.NewImageOp(tiles[i])},
			SearchText: strings.Join(presentation.Aliases, " "),
		}
		logos[choice.ID] = choice
		for _, key := range presentation.LegacyKeys {
			logos[key] = choice
		}
		if presentation.Selectable() {
			choices = append(choices, choice)
		}
	}
	localized := make(map[i18n.Language][]ui.Choice, len(directoryIconChoices))
	for language, generic := range directoryIconChoices {
		items := make([]ui.Choice, 0, len(generic)+len(choices))
		items = append(items, generic...)
		localized[language] = append(items, choices...)
	}
	return logos, localized
}()
