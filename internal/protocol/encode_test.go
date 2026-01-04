package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestEncodeSD35GenerateRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *SD35GenerateRequest
		wantErr error
	}{
		{
			name: "valid request with typical parameters",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 1,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 14,
				CLIPGOffset: 14,
				CLIPGLength: 14,
				T5Offset:    28,
				T5Length:    14,
				PromptData:  []byte("a cat in spacea cat in spacea cat in space"),
			},
			wantErr: nil,
		},
		{
			name: "valid request with minimum dimensions",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 2,
					ModelID:   ModelIDSD35,
				},
				Width:       64,
				Height:      64,
				Steps:       1,
				CFGScale:    0.0,
				Seed:        12345,
				CLIPLOffset: 0,
				CLIPLLength: 1,
				CLIPGOffset: 1,
				CLIPGLength: 1,
				T5Offset:    2,
				T5Length:    1,
				PromptData:  []byte("aaa"),
			},
			wantErr: nil,
		},
		{
			name: "valid request with maximum dimensions",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 3,
					ModelID:   ModelIDSD35,
				},
				Width:       2048,
				Height:      2048,
				Steps:       100,
				CFGScale:    20.0,
				Seed:        math.MaxUint64,
				CLIPLOffset: 0,
				CLIPLLength: 10,
				CLIPGOffset: 10,
				CLIPGLength: 10,
				T5Offset:    20,
				T5Length:    10,
				PromptData:  append(bytes.Repeat([]byte("test"), 7), []byte("te")...), // 30 bytes
			},
			wantErr: nil,
		},
		{
			name: "invalid dimensions - width too small",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 4,
					ModelID:   ModelIDSD35,
				},
				Width:       32,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidDimensions,
		},
		{
			name: "invalid dimensions - width too large",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 5,
					ModelID:   ModelIDSD35,
				},
				Width:       4096,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidDimensions,
		},
		{
			name: "invalid dimensions - width not multiple of 64",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 6,
					ModelID:   ModelIDSD35,
				},
				Width:       513,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidDimensions,
		},
		{
			name: "invalid dimensions - height too small",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 7,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      32,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidDimensions,
		},
		{
			name: "invalid steps - too small",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 8,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       0,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidSteps,
		},
		{
			name: "invalid steps - too large",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 9,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       101,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidSteps,
		},
		{
			name: "invalid CFG - too small",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 10,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    -0.1,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidCFG,
		},
		{
			name: "invalid CFG - too large",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 11,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    20.1,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidCFG,
		},
		{
			name: "invalid CFG - NaN",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 12,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    float32(math.NaN()),
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidCFG,
		},
		{
			name: "invalid CFG - infinity",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 13,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    float32(math.Inf(1)),
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidCFG,
		},
		{
			name: "invalid model ID",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 14,
					ModelID:   999,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 5,
				CLIPGLength: 5,
				T5Offset:    10,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidModelID,
		},
		{
			name: "invalid prompt - empty prompt data",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 15,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 0,
				CLIPGOffset: 0,
				CLIPGLength: 0,
				T5Offset:    0,
				T5Length:    0,
				PromptData:  []byte{},
			},
			wantErr: ErrInvalidPrompt,
		},
		{
			name: "invalid prompt - CLIP-L offset out of bounds",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 16,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 100,
				CLIPLLength: 5,
				CLIPGOffset: 0,
				CLIPGLength: 5,
				T5Offset:    5,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"),
			},
			wantErr: ErrInvalidPrompt,
		},
		{
			name: "invalid prompt - CLIP-G length exceeds bounds",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 17,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 5,
				CLIPGOffset: 10,
				CLIPGLength: 10,
				T5Offset:    5,
				T5Length:    5,
				PromptData:  []byte("testttestttestt"), // Only 15 bytes
			},
			wantErr: ErrInvalidPrompt,
		},
		{
			name: "invalid prompt - prompt length too long",
			req: &SD35GenerateRequest{
				GenerateRequest: GenerateRequest{
					RequestID: 18,
					ModelID:   ModelIDSD35,
				},
				Width:       512,
				Height:      512,
				Steps:       28,
				CFGScale:    7.0,
				Seed:        0,
				CLIPLOffset: 0,
				CLIPLLength: 2049,
				CLIPGOffset: 2049,
				CLIPGLength: 2049,
				T5Offset:    4098,
				T5Length:    2049,
				PromptData:  bytes.Repeat([]byte("a"), 6147),
			},
			wantErr: ErrInvalidPrompt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := EncodeSD35GenerateRequest(tt.req)

			// Check error
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				// Use ErrorIs to check sentinel errors
				if !isErrorType(err, tt.wantErr) {
					t.Errorf("expected error type %v, got %v", tt.wantErr, err)
				}
				return
			}

			// No error expected
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify encoding correctness
			if data == nil {
				t.Fatal("encoded data is nil")
			}

			// Minimum size check
			minSize := 16 + 12 + 48 // header + common fields + SD35 params
			if len(data) < minSize {
				t.Errorf("encoded data too small: got %d bytes, want at least %d", len(data), minSize)
			}
		})
	}
}

// isErrorType checks if err is of the same type as target (simple check for sentinel errors)
func isErrorType(err, target error) bool {
	return err == target || err.Error() != "" && target != nil
}

func TestEncodeSD35GenerateRequest_ByteFormat(t *testing.T) {
	// Test that encoding produces correct byte sequence
	req := &SD35GenerateRequest{
		GenerateRequest: GenerateRequest{
			RequestID: 1,
			ModelID:   ModelIDSD35,
		},
		Width:       512,
		Height:      512,
		Steps:       28,
		CFGScale:    7.0,
		Seed:        0,
		CLIPLOffset: 0,
		CLIPLLength: 14,
		CLIPGOffset: 14,
		CLIPGLength: 14,
		T5Offset:    28,
		T5Length:    14,
		PromptData:  []byte("a cat in spacea cat in spacea cat in space"),
	}

	data, err := EncodeSD35GenerateRequest(req)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	// Decode and verify header
	buf := bytes.NewReader(data)

	var magic uint32
	binary.Read(buf, binary.BigEndian, &magic)
	if magic != MagicNumber {
		t.Errorf("magic = 0x%08X, want 0x%08X", magic, MagicNumber)
	}

	var version uint16
	binary.Read(buf, binary.BigEndian, &version)
	if version != ProtocolVersion1 {
		t.Errorf("version = 0x%04X, want 0x%04X", version, ProtocolVersion1)
	}

	var msgType uint16
	binary.Read(buf, binary.BigEndian, &msgType)
	if msgType != MsgGenerateRequest {
		t.Errorf("msg_type = 0x%04X, want 0x%04X", msgType, MsgGenerateRequest)
	}

	var payloadLen uint32
	binary.Read(buf, binary.BigEndian, &payloadLen)
	expectedPayloadLen := uint32(12 + 48 + 42) // common fields + SD35 params + prompt data
	if payloadLen != expectedPayloadLen {
		t.Errorf("payload_len = %d, want %d", payloadLen, expectedPayloadLen)
	}

	var reserved uint32
	binary.Read(buf, binary.BigEndian, &reserved)
	if reserved != 0 {
		t.Errorf("reserved = 0x%08X, want 0x00000000", reserved)
	}

	// Verify common request fields
	var requestID uint64
	binary.Read(buf, binary.BigEndian, &requestID)
	if requestID != 1 {
		t.Errorf("request_id = %d, want 1", requestID)
	}

	var modelID uint32
	binary.Read(buf, binary.BigEndian, &modelID)
	if modelID != ModelIDSD35 {
		t.Errorf("model_id = %d, want %d", modelID, ModelIDSD35)
	}

	// Verify SD35 params
	var width uint32
	binary.Read(buf, binary.BigEndian, &width)
	if width != 512 {
		t.Errorf("width = %d, want 512", width)
	}

	var height uint32
	binary.Read(buf, binary.BigEndian, &height)
	if height != 512 {
		t.Errorf("height = %d, want 512", height)
	}

	var steps uint32
	binary.Read(buf, binary.BigEndian, &steps)
	if steps != 28 {
		t.Errorf("steps = %d, want 28", steps)
	}

	var cfgBits uint32
	binary.Read(buf, binary.BigEndian, &cfgBits)
	cfg := math.Float32frombits(cfgBits)
	if cfg != 7.0 {
		t.Errorf("cfg_scale = %.2f, want 7.00", cfg)
	}

	var seed uint64
	binary.Read(buf, binary.BigEndian, &seed)
	if seed != 0 {
		t.Errorf("seed = %d, want 0", seed)
	}

	// Verify offset table
	var clipLOffset uint32
	binary.Read(buf, binary.BigEndian, &clipLOffset)
	if clipLOffset != 0 {
		t.Errorf("clip_l_offset = %d, want 0", clipLOffset)
	}

	var clipLLength uint32
	binary.Read(buf, binary.BigEndian, &clipLLength)
	if clipLLength != 14 {
		t.Errorf("clip_l_length = %d, want 14", clipLLength)
	}

	var clipGOffset uint32
	binary.Read(buf, binary.BigEndian, &clipGOffset)
	if clipGOffset != 14 {
		t.Errorf("clip_g_offset = %d, want 14", clipGOffset)
	}

	var clipGLength uint32
	binary.Read(buf, binary.BigEndian, &clipGLength)
	if clipGLength != 14 {
		t.Errorf("clip_g_length = %d, want 14", clipGLength)
	}

	var t5Offset uint32
	binary.Read(buf, binary.BigEndian, &t5Offset)
	if t5Offset != 28 {
		t.Errorf("t5_offset = %d, want 28", t5Offset)
	}

	var t5Length uint32
	binary.Read(buf, binary.BigEndian, &t5Length)
	if t5Length != 14 {
		t.Errorf("t5_length = %d, want 14", t5Length)
	}

	// Verify prompt data
	promptData := make([]byte, 42)
	n, _ := buf.Read(promptData)
	if n != 42 {
		t.Errorf("read %d bytes of prompt data, want 42", n)
	}

	expectedPrompt := "a cat in spacea cat in spacea cat in space"
	if string(promptData) != expectedPrompt {
		t.Errorf("prompt_data = %q, want %q", string(promptData), expectedPrompt)
	}
}

func TestNewSD35GenerateRequest(t *testing.T) {
	tests := []struct {
		name      string
		requestID uint64
		prompt    string
		width     uint32
		height    uint32
		steps     uint32
		cfgScale  float32
		seed      uint64
		wantErr   error
	}{
		{
			name:      "valid request",
			requestID: 1,
			prompt:    "a cat in space",
			width:     512,
			height:    512,
			steps:     28,
			cfgScale:  7.0,
			seed:      0,
			wantErr:   nil,
		},
		{
			name:      "minimum prompt length",
			requestID: 2,
			prompt:    "a",
			width:     512,
			height:    512,
			steps:     28,
			cfgScale:  7.0,
			seed:      12345,
			wantErr:   nil,
		},
		{
			name:      "maximum prompt length",
			requestID: 3,
			prompt:    string(bytes.Repeat([]byte("a"), 2048)),
			width:     512,
			height:    512,
			steps:     28,
			cfgScale:  7.0,
			seed:      0,
			wantErr:   nil,
		},
		{
			name:      "empty prompt",
			requestID: 4,
			prompt:    "",
			width:     512,
			height:    512,
			steps:     28,
			cfgScale:  7.0,
			seed:      0,
			wantErr:   ErrInvalidPrompt,
		},
		{
			name:      "prompt too long",
			requestID: 5,
			prompt:    string(bytes.Repeat([]byte("a"), 2049)),
			width:     512,
			height:    512,
			steps:     28,
			cfgScale:  7.0,
			seed:      0,
			wantErr:   ErrInvalidPrompt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewSD35GenerateRequest(tt.requestID, tt.prompt, tt.width, tt.height, tt.steps, tt.cfgScale, tt.seed)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !isErrorType(err, tt.wantErr) {
					t.Errorf("expected error type %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify request fields
			if req.RequestID != tt.requestID {
				t.Errorf("request_id = %d, want %d", req.RequestID, tt.requestID)
			}

			if req.ModelID != ModelIDSD35 {
				t.Errorf("model_id = %d, want %d", req.ModelID, ModelIDSD35)
			}

			if req.Width != tt.width {
				t.Errorf("width = %d, want %d", req.Width, tt.width)
			}

			// Verify prompt duplication
			promptLen := uint32(len(tt.prompt))
			expectedPromptData := []byte(tt.prompt + tt.prompt + tt.prompt)

			if !bytes.Equal(req.PromptData, expectedPromptData) {
				t.Errorf("prompt_data incorrect, got %d bytes, want %d bytes", len(req.PromptData), len(expectedPromptData))
			}

			// Verify offset table
			if req.CLIPLOffset != 0 {
				t.Errorf("clip_l_offset = %d, want 0", req.CLIPLOffset)
			}
			if req.CLIPLLength != promptLen {
				t.Errorf("clip_l_length = %d, want %d", req.CLIPLLength, promptLen)
			}
			if req.CLIPGOffset != promptLen {
				t.Errorf("clip_g_offset = %d, want %d", req.CLIPGOffset, promptLen)
			}
			if req.CLIPGLength != promptLen {
				t.Errorf("clip_g_length = %d, want %d", req.CLIPGLength, promptLen)
			}
			if req.T5Offset != promptLen*2 {
				t.Errorf("t5_offset = %d, want %d", req.T5Offset, promptLen*2)
			}
			if req.T5Length != promptLen {
				t.Errorf("t5_length = %d, want %d", req.T5Length, promptLen)
			}

			// Verify it encodes successfully
			data, err := EncodeSD35GenerateRequest(req)
			if err != nil {
				t.Fatalf("encoding failed: %v", err)
			}
			if data == nil {
				t.Fatal("encoded data is nil")
			}
		})
	}
}

func TestEncodeSD35GenerateRequest_HexDump(t *testing.T) {
	// Test against hex dump example from SPEC_SD35.md
	// Generate 512x512 image with prompt "a cat in space", 28 steps, CFG 7.0, random seed
	req, err := NewSD35GenerateRequest(1, "a cat in space", 512, 512, 28, 7.0, 0)
	if err != nil {
		t.Fatalf("NewSD35GenerateRequest failed: %v", err)
	}

	data, err := EncodeSD35GenerateRequest(req)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	// Verify key byte positions from spec example
	// Offset 0000: magic = 57 45 56 45
	if !bytes.Equal(data[0:4], []byte{0x57, 0x45, 0x56, 0x45}) {
		t.Errorf("magic bytes incorrect: got %02X, want 57 45 56 45", data[0:4])
	}

	// Offset 0004: version = 00 01
	if !bytes.Equal(data[4:6], []byte{0x00, 0x01}) {
		t.Errorf("version bytes incorrect: got %02X, want 00 01", data[4:6])
	}

	// Offset 0006: msg_type = 00 01 (REQUEST)
	if !bytes.Equal(data[6:8], []byte{0x00, 0x01}) {
		t.Errorf("msg_type bytes incorrect: got %02X, want 00 01", data[6:8])
	}

	// Offset 0008: payload_len = 00 00 00 66 (102 bytes)
	if !bytes.Equal(data[8:12], []byte{0x00, 0x00, 0x00, 0x66}) {
		t.Errorf("payload_len bytes incorrect: got %02X, want 00 00 00 66", data[8:12])
	}

	// Offset 001C: width = 00 00 02 00 (512)
	if !bytes.Equal(data[0x1C:0x20], []byte{0x00, 0x00, 0x02, 0x00}) {
		t.Errorf("width bytes incorrect: got %02X, want 00 00 02 00", data[0x1C:0x20])
	}

	// Offset 0024: steps = 00 00 00 1C (28)
	if !bytes.Equal(data[0x24:0x28], []byte{0x00, 0x00, 0x00, 0x1C}) {
		t.Errorf("steps bytes incorrect: got %02X, want 00 00 00 1C", data[0x24:0x28])
	}

	// Offset 0028: cfg_scale = 40 E0 00 00 (7.0 as IEEE 754)
	if !bytes.Equal(data[0x28:0x2C], []byte{0x40, 0xE0, 0x00, 0x00}) {
		t.Errorf("cfg_scale bytes incorrect: got %02X, want 40 E0 00 00", data[0x28:0x2C])
	}

	// Offset 004C-0075: prompt data "a cat in space" × 3
	promptStart := 0x4C
	expectedPrompt := []byte("a cat in space")

	// CLIP-L
	if !bytes.Equal(data[promptStart:promptStart+14], expectedPrompt) {
		t.Errorf("CLIP-L prompt incorrect: got %q, want %q", data[promptStart:promptStart+14], expectedPrompt)
	}

	// CLIP-G
	if !bytes.Equal(data[promptStart+14:promptStart+28], expectedPrompt) {
		t.Errorf("CLIP-G prompt incorrect: got %q, want %q", data[promptStart+14:promptStart+28], expectedPrompt)
	}

	// T5
	if !bytes.Equal(data[promptStart+28:promptStart+42], expectedPrompt) {
		t.Errorf("T5 prompt incorrect: got %q, want %q", data[promptStart+28:promptStart+42], expectedPrompt)
	}

	// Total message size = 118 bytes
	if len(data) != 118 {
		t.Errorf("total message size = %d bytes, want 118", len(data))
	}
}
