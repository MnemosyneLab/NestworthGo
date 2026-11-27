package ui

import (
	"os"
	"path/filepath"
	"sync"

	"gioui.org/font"
	"gioui.org/font/opentype"
	gotextfont "github.com/go-text/typesetting/font"
	gotextopentype "github.com/go-text/typesetting/font/opentype"
)

const (
	pingFangSC       font.Typeface = "PingFang SC"
	heitiFallback    font.Typeface = ", Heiti SC"
	macFontFallbacks font.Typeface = ", Hiragino Sans GB, Apple Color Emoji"
)

func systemTypeface(base font.Typeface) font.Typeface {
	return macTypeface(base, len(supplementalSystemFaces()) > 0)
}

func macTypeface(base font.Typeface, loadedPingFang bool) font.Typeface {
	base += ", " + pingFangSC
	if !loadedPingFang {
		// Keep centered Han text when Gio cannot discover a usable PingFang.
		base += heitiFallback
	}
	return base + macFontFallbacks
}

// Recent macOS releases can store the standard PingFang collection as a font
// asset outside Gio's system scan. Core Text can also resolve the family to
// the reserved UI collection, whose outlines Gio cannot draw. Load the
// standard asset when installed and let Gio keep its usual system scan.
var supplementalSystemFaces = sync.OnceValue(func() []font.FontFace {
	paths, _ := filepath.Glob("/System/Library/AssetsV2/com_apple_MobileAsset_Font*/*.asset/AssetData/PingFang.ttc")
	for _, path := range paths {
		if faces := loadPingFangSC(path); len(faces) > 0 {
			return faces
		}
	}
	return nil
})

func loadPingFangSC(path string) []font.FontFace {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	loaders, err := gotextopentype.NewLoaders(file)
	if err != nil {
		return nil
	}

	var faces []font.FontFace
	var scratch []byte
	for _, loader := range loaders {
		var description gotextfont.Description
		description, scratch = gotextfont.Describe(loader, scratch)
		metadata := opentype.DescriptionToFont(description)
		// Read outlines only for the family and weights the preview uses.
		if metadata.Typeface != pingFangSC || metadata.Style != font.Regular || metadata.Weight < font.Normal {
			continue
		}
		parsed, err := gotextfont.NewFont(loader)
		if err != nil {
			continue
		}
		face := gotextfont.NewFace(parsed)
		glyph, _ := face.NominalGlyph('现')
		outline, ok := face.GlyphDataOutline(glyph)
		if glyph == 0 || !ok || len(outline.Segments) == 0 {
			continue
		}
		faces = append(faces, font.FontFace{Font: metadata, Face: pingFangFace{parsed}})
	}
	return faces
}

type pingFangFace struct{ font *gotextfont.Font }

// Each shaper needs its own mutable face over the shared font data.
func (f pingFangFace) Face() *gotextfont.Face { return gotextfont.NewFace(f.font) }
