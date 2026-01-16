package image

import (
	"bytes"
	"image/png"
	"testing"
)

func TestEncodePNG_RGB_SolidColor(t *testing.T) {
	width, height := 2, 2
	// Create solid red image (RGB)
	pixels := []byte{
		255, 0, 0, 255, 0, 0, // Row 1
		255, 0, 0, 255, 0, 0, // Row 2
	}

	pngData, err := EncodePNG(width, height, pixels, FormatRGB)
	if err != nil {
		t.Fatalf("EncodePNG failed: %v", err)
	}

	// Verify output is valid PNG by decoding
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Errorf("got dimensions %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
}

func TestEncodePNG_RGB_Gradient(t *testing.T) {
	width, height := 2, 2
	// Create a gradient (different colors per pixel)
	pixels := []byte{
		255, 0, 0, 0, 255, 0, // Red, Green
		0, 0, 255, 255, 255, 0, // Blue, Yellow
	}

	pngData, err := EncodePNG(width, height, pixels, FormatRGB)
	if err != nil {
		t.Fatalf("EncodePNG failed: %v", err)
	}

	// Verify output is valid PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Errorf("got dimensions %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
}

func TestEncodePNG_RGBA_WithTransparency(t *testing.T) {
	width, height := 2, 2
	// Create image with varying transparency (RGBA)
	pixels := []byte{
		255, 0, 0, 255, 0, 255, 0, 128, // Red (opaque), Green (half transparent)
		0, 0, 255, 64, 255, 255, 0, 0, // Blue (quarter transparent), Yellow (transparent)
	}

	pngData, err := EncodePNG(width, height, pixels, FormatRGBA)
	if err != nil {
		t.Fatalf("EncodePNG failed: %v", err)
	}

	// Verify output is valid PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Errorf("got dimensions %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
}

func TestEncodePNG_InvalidDimensions_Zero(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 10},
		{"zero height", 10, 0},
		{"both zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pixels := make([]byte, 3) // Dummy data
			_, err := EncodePNG(tt.width, tt.height, pixels, FormatRGB)
			if err != ErrInvalidDimensions {
				t.Errorf("got error %v, want %v", err, ErrInvalidDimensions)
			}
		})
	}
}

func TestEncodePNG_InvalidDimensions_Negative(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"negative width", -10, 10},
		{"negative height", 10, -10},
		{"both negative", -10, -10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pixels := make([]byte, 3) // Dummy data
			_, err := EncodePNG(tt.width, tt.height, pixels, FormatRGB)
			if err != ErrInvalidDimensions {
				t.Errorf("got error %v, want %v", err, ErrInvalidDimensions)
			}
		})
	}
}

func TestEncodePNG_InvalidPixelDataLength(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		format     PixelFormat
		pixelCount int // Number of pixels to provide (not bytes)
		wantErr    error
	}{
		{"RGB too short", 2, 2, FormatRGB, 3, ErrInvalidPixelDataLength},
		{"RGB too long", 2, 2, FormatRGB, 5, ErrInvalidPixelDataLength},
		{"RGBA too short", 2, 2, FormatRGBA, 3, ErrInvalidPixelDataLength},
		{"RGBA too long", 2, 2, FormatRGBA, 5, ErrInvalidPixelDataLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bytesPerPixel int
			if tt.format == FormatRGB {
				bytesPerPixel = 3
			} else {
				bytesPerPixel = 4
			}
			pixels := make([]byte, tt.pixelCount*bytesPerPixel)
			_, err := EncodePNG(tt.width, tt.height, pixels, tt.format)
			if err != tt.wantErr {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestEncodePNG_UnknownFormat(t *testing.T) {
	width, height := 2, 2
	pixels := make([]byte, width*height*3)
	invalidFormat := PixelFormat(99)

	_, err := EncodePNG(width, height, pixels, invalidFormat)
	if err != ErrUnknownFormat {
		t.Errorf("got error %v, want %v", err, ErrUnknownFormat)
	}
}

func TestEncodePNG_ValidateDecodedPixels(t *testing.T) {
	width, height := 1, 1
	// Single red pixel (RGB)
	pixels := []byte{255, 0, 0}

	pngData, err := EncodePNG(width, height, pixels, FormatRGB)
	if err != nil {
		t.Fatalf("EncodePNG failed: %v", err)
	}

	// Decode and verify pixel color
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	r, g, b, _ := img.At(0, 0).RGBA()
	// RGBA() returns 16-bit values, so scale to 8-bit
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

	if r8 != 255 || g8 != 0 || b8 != 0 {
		t.Errorf("got pixel color RGB(%d, %d, %d), want RGB(255, 0, 0)", r8, g8, b8)
	}
}

func TestEncodePNG_DimensionOverflow(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		wantErr bool
	}{
		{"max allowed dimensions", 4096, 4096, false},
		{"width exceeds max", 4097, 100, true},
		{"height exceeds max", 100, 4097, true},
		{"both exceed max", 5000, 5000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For valid dimensions, create appropriate pixel data
			// For invalid dimensions, minimal data is fine since dimension check comes first
			var pixels []byte
			if !tt.wantErr {
				pixels = make([]byte, tt.width*tt.height*3)
			} else {
				pixels = make([]byte, 3) // Minimal data
			}

			_, err := EncodePNG(tt.width, tt.height, pixels, FormatRGB)
			if tt.wantErr && err == nil {
				t.Error("expected error for invalid dimensions")
			} else if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for valid dimensions: %v", err)
			}
		})
	}
}

func TestEncodePNG_MaxDimensions(t *testing.T) {
	// Test that max allowed dimensions work
	// Note: This test creates a 48MB array (4096*4096*3), which is acceptable for a test
	width, height := 4096, 4096
	pixels := make([]byte, width*height*3)

	_, err := EncodePNG(width, height, pixels, FormatRGB)
	if err != nil {
		t.Errorf("max dimensions should work: %v", err)
	}
}
