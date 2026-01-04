package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

// Helper function to build a valid common header
func buildHeader(msgType uint16, payloadLen uint32) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, MagicNumber)
	binary.Write(buf, binary.BigEndian, ProtocolVersion1)
	binary.Write(buf, binary.BigEndian, msgType)
	binary.Write(buf, binary.BigEndian, payloadLen)
	binary.Write(buf, binary.BigEndian, uint32(0)) // reserved
	return buf.Bytes()
}

// Helper function to build a complete generate response
func buildGenerateResponse(requestID uint64, status uint32, generationTime uint32,
	width, height, channels, imageDataLen uint32, imageData []byte) []byte {
	buf := new(bytes.Buffer)

	// Common header
	payloadLen := uint32(16 + 16 + len(imageData)) // response + image header + data
	buf.Write(buildHeader(MsgGenerateResponse, payloadLen))

	// Common response fields
	binary.Write(buf, binary.BigEndian, requestID)
	binary.Write(buf, binary.BigEndian, status)
	binary.Write(buf, binary.BigEndian, generationTime)

	// Image header
	binary.Write(buf, binary.BigEndian, width)
	binary.Write(buf, binary.BigEndian, height)
	binary.Write(buf, binary.BigEndian, channels)
	binary.Write(buf, binary.BigEndian, imageDataLen)

	// Image data
	buf.Write(imageData)

	return buf.Bytes()
}

// Helper function to build a complete error response
func buildErrorResponse(requestID uint64, status uint32, errorCode uint32, errorMsg string) []byte {
	buf := new(bytes.Buffer)

	// Common header
	payloadLen := uint32(18 + len(errorMsg)) // request_id + status + error_code + msg_len + msg
	buf.Write(buildHeader(MsgError, payloadLen))

	// Error response fields
	binary.Write(buf, binary.BigEndian, requestID)
	binary.Write(buf, binary.BigEndian, status)
	binary.Write(buf, binary.BigEndian, errorCode)
	binary.Write(buf, binary.BigEndian, uint16(len(errorMsg)))
	buf.WriteString(errorMsg)

	return buf.Bytes()
}

func TestDecodeHeader(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid header",
			data:    buildHeader(MsgGenerateResponse, 100),
			wantErr: false,
		},
		{
			name:    "too small",
			data:    []byte{0x57, 0x45, 0x56}, // Only 3 bytes
			wantErr: true,
			errMsg:  "header too small",
		},
		{
			name: "invalid magic",
			data: func() []byte {
				buf := new(bytes.Buffer)
				binary.Write(buf, binary.BigEndian, uint32(0xDEADBEEF))
				binary.Write(buf, binary.BigEndian, ProtocolVersion1)
				binary.Write(buf, binary.BigEndian, MsgGenerateResponse)
				binary.Write(buf, binary.BigEndian, uint32(0))
				binary.Write(buf, binary.BigEndian, uint32(0))
				return buf.Bytes()
			}(),
			wantErr: true,
			errMsg:  "invalid magic number",
		},
		{
			name: "unsupported version too high",
			data: func() []byte {
				buf := new(bytes.Buffer)
				binary.Write(buf, binary.BigEndian, MagicNumber)
				binary.Write(buf, binary.BigEndian, uint16(0x0099)) // Way too high
				binary.Write(buf, binary.BigEndian, MsgGenerateResponse)
				binary.Write(buf, binary.BigEndian, uint32(0))
				binary.Write(buf, binary.BigEndian, uint32(0))
				return buf.Bytes()
			}(),
			wantErr: true,
			errMsg:  "unsupported protocol version",
		},
		{
			name: "unsupported version too low",
			data: func() []byte {
				buf := new(bytes.Buffer)
				binary.Write(buf, binary.BigEndian, MagicNumber)
				binary.Write(buf, binary.BigEndian, uint16(0x0000))
				binary.Write(buf, binary.BigEndian, MsgGenerateResponse)
				binary.Write(buf, binary.BigEndian, uint32(0))
				binary.Write(buf, binary.BigEndian, uint32(0))
				return buf.Bytes()
			}(),
			wantErr: true,
			errMsg:  "unsupported protocol version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeHeader(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("decodeHeader() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "too small for header",
			data:    []byte{0x57, 0x45},
			wantErr: true,
			errMsg:  "message too small",
		},
		{
			name: "invalid magic",
			data: func() []byte {
				buf := new(bytes.Buffer)
				binary.Write(buf, binary.BigEndian, uint32(0xDEADBEEF))
				binary.Write(buf, binary.BigEndian, ProtocolVersion1)
				binary.Write(buf, binary.BigEndian, MsgGenerateResponse)
				binary.Write(buf, binary.BigEndian, uint32(0))
				binary.Write(buf, binary.BigEndian, uint32(0))
				return buf.Bytes()
			}(),
			wantErr: true,
			errMsg:  "invalid magic number",
		},
		{
			name:    "payload too large",
			data:    buildHeader(MsgGenerateResponse, MaxMessageSize+1),
			wantErr: true,
			errMsg:  "exceeds max",
		},
		{
			name: "truncated payload",
			data: func() []byte {
				// Header says 100 bytes payload, but we only provide 10
				header := buildHeader(MsgGenerateResponse, 100)
				return append(header, make([]byte, 10)...)
			}(),
			wantErr: true,
			errMsg:  "truncated message",
		},
		{
			name: "unknown message type",
			data: func() []byte {
				buf := new(bytes.Buffer)
				binary.Write(buf, binary.BigEndian, MagicNumber)
				binary.Write(buf, binary.BigEndian, ProtocolVersion1)
				binary.Write(buf, binary.BigEndian, uint16(0x9999)) // Invalid type
				binary.Write(buf, binary.BigEndian, uint32(0))
				binary.Write(buf, binary.BigEndian, uint32(0))
				return buf.Bytes()
			}(),
			wantErr: true,
			errMsg:  "unexpected message type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeResponse(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("DecodeResponse() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestDecodeGenerateResponse(t *testing.T) {
	// Valid 64x64 RGB image (smallest valid)
	validImageData := make([]byte, 64*64*3)
	for i := range validImageData {
		validImageData[i] = byte(i % 256)
	}

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid 64x64 RGB",
			data:    buildGenerateResponse(1, StatusOK, 1000, 64, 64, 3, 64*64*3, validImageData),
			wantErr: false,
		},
		{
			name:    "valid 512x512 RGB",
			data:    buildGenerateResponse(42, StatusOK, 5000, 512, 512, 3, 512*512*3, make([]byte, 512*512*3)),
			wantErr: false,
		},
		{
			name:    "valid 1024x1024 RGBA",
			data:    buildGenerateResponse(99, StatusOK, 10000, 1024, 1024, 4, 1024*1024*4, make([]byte, 1024*1024*4)),
			wantErr: false,
		},
		{
			name:    "payload too small",
			data:    append(buildHeader(MsgGenerateResponse, 10), make([]byte, 10)...),
			wantErr: true,
			errMsg:  "payload too small",
		},
		{
			name:    "wrong status code",
			data:    buildGenerateResponse(1, StatusBadRequest, 1000, 512, 512, 3, 512*512*3, make([]byte, 512*512*3)),
			wantErr: true,
			errMsg:  "invalid status for GENERATE_RESPONSE",
		},
		{
			name:    "width too small",
			data:    buildGenerateResponse(1, StatusOK, 1000, 32, 512, 3, 32*512*3, make([]byte, 32*512*3)),
			wantErr: true,
			errMsg:  "invalid dimensions",
		},
		{
			name:    "width too large",
			data:    buildGenerateResponse(1, StatusOK, 1000, 4096, 512, 3, 4096*512*3, make([]byte, 4096*512*3)),
			wantErr: true,
			errMsg:  "invalid dimensions",
		},
		{
			name:    "width not aligned",
			data:    buildGenerateResponse(1, StatusOK, 1000, 100, 512, 3, 100*512*3, make([]byte, 100*512*3)),
			wantErr: true,
			errMsg:  "invalid dimensions",
		},
		{
			name:    "height too small",
			data:    buildGenerateResponse(1, StatusOK, 1000, 512, 32, 3, 512*32*3, make([]byte, 512*32*3)),
			wantErr: true,
			errMsg:  "invalid dimensions",
		},
		{
			name:    "height too large",
			data:    buildGenerateResponse(1, StatusOK, 1000, 512, 4096, 3, 512*4096*3, make([]byte, 512*4096*3)),
			wantErr: true,
			errMsg:  "invalid dimensions",
		},
		{
			name:    "height not aligned",
			data:    buildGenerateResponse(1, StatusOK, 1000, 512, 100, 3, 512*100*3, make([]byte, 512*100*3)),
			wantErr: true,
			errMsg:  "invalid dimensions",
		},
		{
			name:    "invalid channels",
			data:    buildGenerateResponse(1, StatusOK, 1000, 512, 512, 5, 512*512*5, make([]byte, 512*512*5)),
			wantErr: true,
			errMsg:  "invalid channels",
		},
		{
			name:    "image_data_len mismatch (too small)",
			data:    buildGenerateResponse(1, StatusOK, 1000, 512, 512, 3, 1000, make([]byte, 1000)),
			wantErr: true,
			errMsg:  "image_data_len mismatch",
		},
		{
			name:    "image_data_len mismatch (too large)",
			data:    buildGenerateResponse(1, StatusOK, 1000, 512, 512, 3, 999999, make([]byte, 999999)),
			wantErr: true,
			errMsg:  "image_data_len mismatch",
		},
		{
			name: "truncated image data",
			data: func() []byte {
				// Build complete message but report larger payload than we have
				// This will be caught by top-level DecodeResponse as "truncated message"
				buf := new(bytes.Buffer)
				actualDataLen := uint32(1000) // Only provide 1000 bytes of image
				payloadLen := uint32(16 + 16 + actualDataLen)
				buf.Write(buildHeader(MsgGenerateResponse, payloadLen))
				binary.Write(buf, binary.BigEndian, uint64(1))
				binary.Write(buf, binary.BigEndian, StatusOK)
				binary.Write(buf, binary.BigEndian, uint32(1000))
				binary.Write(buf, binary.BigEndian, uint32(512))
				binary.Write(buf, binary.BigEndian, uint32(512))
				binary.Write(buf, binary.BigEndian, uint32(3))
				binary.Write(buf, binary.BigEndian, uint32(512*512*3)) // Claim full size
				// Only write 1000 bytes instead of full image
				buf.Write(make([]byte, actualDataLen))
				return buf.Bytes()
			}(),
			wantErr: true,
			errMsg:  "truncated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeResponse(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("DecodeResponse() error = %v, want error containing %q", err, tt.errMsg)
				return
			}
			if !tt.wantErr {
				resp, ok := result.(*SD35GenerateResponse)
				if !ok {
					t.Errorf("DecodeResponse() returned wrong type: got %T, want *SD35GenerateResponse", result)
					return
				}
				// Verify basic fields are populated correctly
				if resp.Status != StatusOK {
					t.Errorf("Status = %d, want %d", resp.Status, StatusOK)
				}
				if resp.ImageData == nil {
					t.Errorf("ImageData is nil")
				}
			}
		})
	}
}

func TestDecodeGenerateResponse_OverflowCheck(t *testing.T) {
	tests := []struct {
		name     string
		width    uint32
		height   uint32
		channels uint32
		wantErr  bool
	}{
		{
			name:     "valid dimensions",
			width:    512,
			height:   512,
			channels: 3,
			wantErr:  false,
		},
		{
			name:     "overflow width * height",
			width:    math.MaxUint32,
			height:   2,
			channels: 1,
			wantErr:  true,
		},
		{
			name:     "overflow width * height * channels",
			width:    math.MaxUint32 / 2,
			height:   2,
			channels: 2,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build response with potentially overflowing dimensions
			// For overflow cases, just use imageDataLen = 0 since we're testing the overflow check
			imageDataLen := uint32(0)
			if !tt.wantErr {
				imageDataLen = tt.width * tt.height * tt.channels
			}
			data := buildGenerateResponse(1, StatusOK, 1000, tt.width, tt.height, tt.channels, imageDataLen, make([]byte, imageDataLen))

			_, err := DecodeResponse(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeErrorResponse(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid error 400",
			data:    buildErrorResponse(1, StatusBadRequest, ErrCodeInvalidPrompt, "prompt too long"),
			wantErr: false,
		},
		{
			name:    "valid error 500",
			data:    buildErrorResponse(42, StatusInternalServerError, ErrCodeGPUError, "GPU out of memory"),
			wantErr: false,
		},
		{
			name:    "empty error message",
			data:    buildErrorResponse(1, StatusBadRequest, ErrCodeInvalidPrompt, ""),
			wantErr: false,
		},
		{
			name:    "long error message",
			data:    buildErrorResponse(1, StatusBadRequest, ErrCodeInvalidPrompt, strings.Repeat("x", 1000)),
			wantErr: false,
		},
		{
			name:    "payload too small",
			data:    append(buildHeader(MsgError, 5), make([]byte, 5)...),
			wantErr: true,
			errMsg:  "payload too small",
		},
		{
			name:    "invalid status code",
			data:    buildErrorResponse(1, StatusOK, ErrCodeInvalidPrompt, "test"),
			wantErr: true,
			errMsg:  "invalid status for ERROR",
		},
		{
			name: "truncated error message",
			data: func() []byte {
				buf := new(bytes.Buffer)
				// Header payload matches actual data, but error_msg_len claims more than we have
				actualMsgLen := uint32(10)
				payloadLen := uint32(18 + actualMsgLen)
				buf.Write(buildHeader(MsgError, payloadLen))
				binary.Write(buf, binary.BigEndian, uint64(1))
				binary.Write(buf, binary.BigEndian, StatusBadRequest)
				binary.Write(buf, binary.BigEndian, ErrCodeInvalidPrompt)
				binary.Write(buf, binary.BigEndian, uint16(100)) // Claim 100 bytes
				buf.Write(make([]byte, actualMsgLen))            // Only provide 10
				return buf.Bytes()
			}(),
			wantErr: true,
			errMsg:  "truncated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeResponse(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("DecodeResponse() error = %v, want error containing %q", err, tt.errMsg)
				return
			}
			if !tt.wantErr {
				resp, ok := result.(*ErrorResponse)
				if !ok {
					t.Errorf("DecodeResponse() returned wrong type: got %T, want *ErrorResponse", result)
					return
				}
				// Verify error response fields
				if resp.Status != StatusBadRequest && resp.Status != StatusInternalServerError {
					t.Errorf("Status = %d, want %d or %d", resp.Status, StatusBadRequest, StatusInternalServerError)
				}
			}
		})
	}
}

func TestDecodeErrorResponse_StatusAndErrorCode(t *testing.T) {
	tests := []struct {
		name      string
		status    uint32
		errorCode uint32
		errorMsg  string
	}{
		{
			name:      "bad request - invalid magic",
			status:    StatusBadRequest,
			errorCode: ErrCodeInvalidMagic,
			errorMsg:  "invalid magic number",
		},
		{
			name:      "bad request - unsupported version",
			status:    StatusBadRequest,
			errorCode: ErrCodeUnsupportedVersion,
			errorMsg:  "protocol version not supported",
		},
		{
			name:      "bad request - invalid model",
			status:    StatusBadRequest,
			errorCode: ErrCodeInvalidModelID,
			errorMsg:  "unsupported model",
		},
		{
			name:      "bad request - invalid prompt",
			status:    StatusBadRequest,
			errorCode: ErrCodeInvalidPrompt,
			errorMsg:  "prompt too long",
		},
		{
			name:      "bad request - invalid dimensions",
			status:    StatusBadRequest,
			errorCode: ErrCodeInvalidDimensions,
			errorMsg:  "dimensions must be multiple of 64",
		},
		{
			name:      "bad request - invalid steps",
			status:    StatusBadRequest,
			errorCode: ErrCodeInvalidSteps,
			errorMsg:  "steps out of range",
		},
		{
			name:      "bad request - invalid cfg",
			status:    StatusBadRequest,
			errorCode: ErrCodeInvalidCFG,
			errorMsg:  "cfg_scale out of range",
		},
		{
			name:      "server error - out of memory",
			status:    StatusInternalServerError,
			errorCode: ErrCodeOutOfMemory,
			errorMsg:  "GPU out of memory",
		},
		{
			name:      "server error - gpu error",
			status:    StatusInternalServerError,
			errorCode: ErrCodeGPUError,
			errorMsg:  "CUDA driver error",
		},
		{
			name:      "server error - timeout",
			status:    StatusInternalServerError,
			errorCode: ErrCodeTimeout,
			errorMsg:  "generation timeout",
		},
		{
			name:      "server error - internal",
			status:    StatusInternalServerError,
			errorCode: ErrCodeInternal,
			errorMsg:  "unexpected error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildErrorResponse(123, tt.status, tt.errorCode, tt.errorMsg)
			result, err := DecodeResponse(data)
			if err != nil {
				t.Fatalf("DecodeResponse() unexpected error: %v", err)
			}

			resp, ok := result.(*ErrorResponse)
			if !ok {
				t.Fatalf("DecodeResponse() returned wrong type: got %T, want *ErrorResponse", result)
			}

			if resp.RequestID != 123 {
				t.Errorf("RequestID = %d, want 123", resp.RequestID)
			}
			if resp.Status != tt.status {
				t.Errorf("Status = %d, want %d", resp.Status, tt.status)
			}
			if resp.ErrorCode != tt.errorCode {
				t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, tt.errorCode)
			}
			if resp.ErrorMessage != tt.errorMsg {
				t.Errorf("ErrorMessage = %q, want %q", resp.ErrorMessage, tt.errorMsg)
			}
		})
	}
}

func TestDecodeResponse_RoundTrip(t *testing.T) {
	// Test that we can decode what looks like a real response
	t.Run("generate response round trip", func(t *testing.T) {
		// Simulate a 512x512 RGB image with a simple pattern
		imageData := make([]byte, 512*512*3)
		for i := range imageData {
			imageData[i] = byte(i % 256)
		}

		data := buildGenerateResponse(42, StatusOK, 5000, 512, 512, 3, 512*512*3, imageData)
		result, err := DecodeResponse(data)
		if err != nil {
			t.Fatalf("DecodeResponse() unexpected error: %v", err)
		}

		resp, ok := result.(*SD35GenerateResponse)
		if !ok {
			t.Fatalf("DecodeResponse() returned wrong type: %T", result)
		}

		if resp.RequestID != 42 {
			t.Errorf("RequestID = %d, want 42", resp.RequestID)
		}
		if resp.Status != StatusOK {
			t.Errorf("Status = %d, want %d", resp.Status, StatusOK)
		}
		if resp.GenerationTime != 5000 {
			t.Errorf("GenerationTime = %d, want 5000", resp.GenerationTime)
		}
		if resp.ImageWidth != 512 {
			t.Errorf("ImageWidth = %d, want 512", resp.ImageWidth)
		}
		if resp.ImageHeight != 512 {
			t.Errorf("ImageHeight = %d, want 512", resp.ImageHeight)
		}
		if resp.Channels != 3 {
			t.Errorf("Channels = %d, want 3", resp.Channels)
		}
		if resp.ImageDataLen != 512*512*3 {
			t.Errorf("ImageDataLen = %d, want %d", resp.ImageDataLen, 512*512*3)
		}
		if len(resp.ImageData) != 512*512*3 {
			t.Errorf("len(ImageData) = %d, want %d", len(resp.ImageData), 512*512*3)
		}
		// Verify data integrity
		for i := range resp.ImageData {
			if resp.ImageData[i] != byte(i%256) {
				t.Errorf("ImageData[%d] = %d, want %d", i, resp.ImageData[i], byte(i%256))
				break
			}
		}
	})

	t.Run("error response round trip", func(t *testing.T) {
		errorMsg := "test error: dimensions must be multiple of 64"
		data := buildErrorResponse(99, StatusBadRequest, ErrCodeInvalidDimensions, errorMsg)
		result, err := DecodeResponse(data)
		if err != nil {
			t.Fatalf("DecodeResponse() unexpected error: %v", err)
		}

		resp, ok := result.(*ErrorResponse)
		if !ok {
			t.Fatalf("DecodeResponse() returned wrong type: %T", result)
		}

		if resp.RequestID != 99 {
			t.Errorf("RequestID = %d, want 99", resp.RequestID)
		}
		if resp.Status != StatusBadRequest {
			t.Errorf("Status = %d, want %d", resp.Status, StatusBadRequest)
		}
		if resp.ErrorCode != ErrCodeInvalidDimensions {
			t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrCodeInvalidDimensions)
		}
		if resp.ErrorMessage != errorMsg {
			t.Errorf("ErrorMessage = %q, want %q", resp.ErrorMessage, errorMsg)
		}
	})
}
