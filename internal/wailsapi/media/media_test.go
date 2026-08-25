package media_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/media"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func tinyPNGBase64(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

type fakeDialog struct {
	path string
	err  error
}

func (d fakeDialog) OpenFile(string) (string, error) { return d.path, d.err }

func TestNormalizeImage(t *testing.T) {
	app := wailstest.NewService(t)
	service := media.NewService(app, fakeDialog{})
	normalized, err := service.NormalizeImage(tinyPNGBase64(t))
	if err != nil {
		t.Fatalf("NormalizeImage: %v", err)
	}
	if normalized == "" {
		t.Fatal("NormalizeImage returned an empty result")
	}
	if _, err := base64.StdEncoding.DecodeString(normalized); err != nil {
		t.Fatalf("normalized result is not valid base64: %v", err)
	}
}

func TestNormalizeImageInvalidBase64(t *testing.T) {
	app := wailstest.NewService(t)
	service := media.NewService(app, fakeDialog{})
	_, err := service.NormalizeImage("not-base64!!!")
	if err == nil {
		t.Fatal("want a validation error for invalid base64")
	}
}

func TestCreateAndReadMediaAsset(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	service := media.NewService(app, fakeDialog{})
	created, err := service.CreateMediaAsset(ctx, "image/png", tinyPNGBase64(t))
	if err != nil {
		t.Fatalf("CreateMediaAsset: %v", err)
	}
	if created.ID == "" || created.MimeType != "image/png" {
		t.Fatalf("created = %+v", created)
	}
	fetched, err := service.MediaAsset(ctx, created.ID)
	if err != nil {
		t.Fatalf("MediaAsset: %v", err)
	}
	if fetched.Data != created.Data {
		t.Fatalf("fetched.Data != created.Data")
	}
}

func TestCreateMediaAssetBeforeOnboarding(t *testing.T) {
	app := wailstest.NewService(t)
	service := media.NewService(app, fakeDialog{})
	_, err := service.CreateMediaAsset(context.Background(), "image/png", tinyPNGBase64(t))
	if err == nil {
		t.Fatal("want an error before onboarding completes")
	}
}

func TestPickImageUserCancelled(t *testing.T) {
	app := wailstest.NewService(t)
	service := media.NewService(app, fakeDialog{path: ""})
	result, err := service.PickImage("Choose a photo")
	if err != nil {
		t.Fatalf("PickImage: %v", err)
	}
	if result != "" {
		t.Fatalf("result = %q, want empty string for a cancelled pick", result)
	}
}

func TestPickImageReadsAndNormalizes(t *testing.T) {
	app := wailstest.NewService(t)
	dir := t.TempDir()
	path := dir + "/photo.png"
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	service := media.NewService(app, fakeDialog{path: path})
	result, err := service.PickImage("Choose a photo")
	if err != nil {
		t.Fatalf("PickImage: %v", err)
	}
	if result == "" {
		t.Fatal("PickImage returned an empty result for a successful pick")
	}
}
