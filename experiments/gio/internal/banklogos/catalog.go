// Package banklogos provides Gio raster views of the Wails bank-logo catalog.
package banklogos

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"strings"
	"unicode"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"golang.org/x/text/unicode/norm"
)

const tileSize, columns = 64, 16

//go:embed catalog.json
var catalogBytes []byte

//go:embed atlas.png
var atlasBytes []byte

type Presentation struct {
	Key, Kind, Name, File string
	LegacyKeys, Aliases   []string
}

func (p Presentation) Selectable() bool { return p.Kind == "symbol" }

type catalog struct {
	SourceDigest  string
	Presentations []Presentation
}

// Load returns presentations in Wails order and a square image for each.
func Load() ([]Presentation, []image.Image, error) {
	var data catalog
	if err := jsoncodec.Unmarshal(catalogBytes, &data); err != nil {
		return nil, nil, err
	}
	atlas, err := png.Decode(bytes.NewReader(atlasBytes))
	if err != nil {
		return nil, nil, err
	}
	size := image.Pt(columns*tileSize, ((len(data.Presentations)+columns-1)/columns)*tileSize)
	if atlas.Bounds() != (image.Rectangle{Max: size}) {
		return nil, nil, fmt.Errorf("bank-logo atlas has bounds %v, expected %v", atlas.Bounds(), size)
	}
	sheet, ok := atlas.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, nil, fmt.Errorf("bank-logo atlas cannot be split into tiles")
	}
	tiles := make([]image.Image, len(data.Presentations))
	for i := range tiles {
		min := image.Pt(i%columns*tileSize, i/columns*tileSize)
		max := min.Add(image.Pt(tileSize, tileSize))
		tiles[i] = sheet.SubImage(image.Rectangle{Min: min, Max: max})
	}
	return data.Presentations, tiles, nil
}

// FoldSearch matches the Wails icon picker's treatment of width, case and separators.
func FoldSearch(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '_' || r == '-' {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFKC.String(value))
}
