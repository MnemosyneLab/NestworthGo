// Package media validates and normalizes local avatar and logo images before
// they enter the Household-scoped business database.
package media

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	MaxInputBytes    = 8 << 20
	MaxDimension     = 512
	MaxDecodedPixels = 16 * 1024 * 1024
)

var ErrInvalidImage = errors.New("image must be a bounded PNG, JPEG, or WebP")

func Normalize(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > MaxInputBytes {
		return nil, ErrInvalidImage
	}
	reader := bytes.NewReader(data)
	config, format, err := image.DecodeConfig(reader)
	if err != nil || !supportedFormat(format) || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > MaxDecodedPixels {
		return nil, ErrInvalidImage
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, ErrInvalidImage
	}
	imageData, format, err := image.Decode(reader)
	if err != nil || !supportedFormat(format) {
		return nil, ErrInvalidImage
	}
	imageData = fit(imageData, MaxDimension)
	var output bytes.Buffer
	if err := png.Encode(&output, imageData); err != nil {
		return nil, ErrInvalidImage
	}
	if output.Len() > MaxInputBytes {
		return nil, ErrInvalidImage
	}
	return output.Bytes(), nil
}

func ReadAndNormalize(reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, ErrInvalidImage
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxInputBytes+1))
	if err != nil || len(data) > MaxInputBytes {
		return nil, ErrInvalidImage
	}
	return Normalize(data)
}

func supportedFormat(format string) bool {
	return format == "png" || format == "jpeg" || format == "webp"
}

func fit(source image.Image, maxDimension int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxDimension && height <= maxDimension {
		return source
	}
	newWidth, newHeight := maxDimension, maxDimension
	if width >= height {
		newHeight = scaleDimension(height, width, maxDimension)
	} else {
		newWidth = scaleDimension(width, height, maxDimension)
	}
	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}
	destination := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.ApproxBiLinear.Scale(destination, destination.Bounds(), source, bounds, draw.Over, nil)
	return destination
}

// scaleDimension rounds the minor edge to the scaled size instead of
// truncating, so floating-point drift cannot shave a pixel off the result.
// major*max cannot overflow: MaxDecodedPixels keeps both edges well below
// int32 range.
func scaleDimension(minor, major, maxDimension int) int {
	return (minor*maxDimension + major/2) / major
}
