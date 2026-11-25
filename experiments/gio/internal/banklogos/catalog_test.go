package banklogos

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestAtlasMatchesWailsPresentations(t *testing.T) {
	frontend := filepath.Join("..", "..", "..", "..", "frontend")
	raw, err := os.ReadFile(filepath.Join(frontend, "src", "lib", "bankLogoPresentation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected []Presentation
	if err := jsoncodec.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	entries, tiles, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entries, expected) {
		t.Fatal("Gio and Wails bank-logo catalogs differ")
	}
	var stored catalog
	if err := jsoncodec.Unmarshal(catalogBytes, &stored); err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	digest.Write(raw)
	for i, presentation := range entries {
		source, err := os.ReadFile(filepath.Join(frontend, "public", "bank-logos", presentation.File))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(source)
		digest.Write(hash[:])
		if !hasInk(tiles[i]) {
			t.Errorf("atlas tile for %s is blank", presentation.Key)
		}
		for _, key := range append([]string{presentation.Key}, presentation.LegacyKeys...) {
			if err := domain.ValidateIconKey(key); err != nil {
				t.Errorf("%s: %v", key, err)
			}
		}
	}
	if hex.EncodeToString(digest.Sum(nil)) != stored.SourceDigest {
		t.Fatal("the Wails presentation assets changed, regenerate the Gio atlas")
	}
}

func hasInk(tile image.Image) bool {
	bounds := tile.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, alpha := tile.At(x, y).RGBA(); alpha != 0 {
				return true
			}
		}
	}
	return false
}

func TestFoldSearch(t *testing.T) {
	if got := FoldSearch(" ＳＴＡＮＤＡＲＤ-Chartered_\t"); got != "standardchartered" {
		t.Fatalf("normalized alias %q", got)
	}
}
