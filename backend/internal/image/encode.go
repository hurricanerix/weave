package image

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"math"
)

// PixelFormat specifies the format of raw pixel data
type PixelFormat int

const (
	// FormatRGB represents 3 bytes per pixel (R, G, B)
	FormatRGB PixelFormat = iota
	// FormatRGBA represents 4 bytes per pixel (R, G, B, A)
	FormatRGBA

	// MaxImageDimension is the maximum allowed width or height (4K resolution)
	MaxImageDimension = 4096
)

var (
	// ErrInvalidDimensions indicates width or height is not positive
	ErrInvalidDimensions = errors.New("invalid dimensions: width and height must be positive")
	// ErrInvalidPixelDataLength indicates pixel data length does not match dimensions
	ErrInvalidPixelDataLength = errors.New("invalid pixel data length")
	// ErrUnknownFormat indicates an unsupported pixel format
	ErrUnknownFormat = errors.New("unknown pixel format")
)

// EncodePNG converts raw pixel data to PNG format.
//
// width, height: image dimensions in pixels
// pixels: raw pixel data (RGB or RGBA bytes)
// format: pixel format (RGB or RGBA)
//
// Returns PNG bytes or error if encoding fails.
func EncodePNG(width, height int, pixels []byte, format PixelFormat) ([]byte, error) {
	// Validate dimensions are positive
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidDimensions
	}

	// Check maximum dimension limits
	if width > MaxImageDimension || height > MaxImageDimension {
		return nil, errors.New("dimensions exceed maximum allowed (4096x4096)")
	}

	// Calculate expected pixel data length
	var bytesPerPixel int
	switch format {
	case FormatRGB:
		bytesPerPixel = 3
	case FormatRGBA:
		bytesPerPixel = 4
	default:
		return nil, ErrUnknownFormat
	}

	// Check for integer overflow before multiplication
	maxPixels := math.MaxInt / bytesPerPixel
	if width > maxPixels/height {
		return nil, errors.New("dimensions too large: would overflow")
	}

	expectedLength := width * height * bytesPerPixel
	if len(pixels) != expectedLength {
		return nil, ErrInvalidPixelDataLength
	}

	// Create image.RGBA from raw bytes
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Copy pixels into image efficiently
	switch format {
	case FormatRGBA:
		// For RGBA, copy directly to underlying buffer
		copy(img.Pix, pixels)
	case FormatRGB:
		// For RGB, expand to RGBA efficiently
		dst := img.Pix
		for i := 0; i < len(pixels)/3; i++ {
			dst[i*4] = pixels[i*3]     // R
			dst[i*4+1] = pixels[i*3+1] // G
			dst[i*4+2] = pixels[i*3+2] // B
			dst[i*4+3] = 255           // A (opaque)
		}
	}

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
