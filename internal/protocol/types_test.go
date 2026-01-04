package protocol

import (
	"math"
	"testing"
	"unsafe"
)

// TestHeaderSize verifies the Header struct has the expected size.
func TestHeaderSize(t *testing.T) {
	// Header should be 16 bytes (4+2+2+4+4)
	expected := uintptr(16)
	actual := unsafe.Sizeof(Header{})

	if actual != expected {
		t.Errorf("Header size = %d bytes, want %d bytes", actual, expected)
	}
}

// TestHeaderFieldOffsets verifies field order matches the protocol spec.
func TestHeaderFieldOffsets(t *testing.T) {
	var h Header

	tests := []struct {
		name           string
		fieldOffset    uintptr
		expectedOffset uintptr
	}{
		{"Magic", unsafe.Offsetof(h.Magic), 0},
		{"Version", unsafe.Offsetof(h.Version), 4},
		{"MsgType", unsafe.Offsetof(h.MsgType), 6},
		{"PayloadLen", unsafe.Offsetof(h.PayloadLen), 8},
		{"Reserved", unsafe.Offsetof(h.Reserved), 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fieldOffset != tt.expectedOffset {
				t.Errorf("Header.%s offset = %d, want %d", tt.name, tt.fieldOffset, tt.expectedOffset)
			}
		})
	}
}

// TestProtocolConstants verifies protocol constants have correct values.
func TestProtocolConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"MagicNumber", MagicNumber, uint32(0x57455645)},
		{"ProtocolVersion1", ProtocolVersion1, uint16(0x0001)},
		{"MinSupportedVersion", MinSupportedVersion, uint16(0x0001)},
		{"MaxSupportedVersion", MaxSupportedVersion, uint16(0x0001)},
		{"MaxMessageSize", MaxMessageSize, uint32(10 * 1024 * 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestMessageTypes verifies message type constants.
func TestMessageTypes(t *testing.T) {
	tests := []struct {
		name     string
		got      uint16
		expected uint16
	}{
		{"MsgGenerateRequest", MsgGenerateRequest, 0x0001},
		{"MsgGenerateResponse", MsgGenerateResponse, 0x0002},
		{"MsgError", MsgError, 0x00FF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = 0x%04X, want 0x%04X", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestStatusCodes verifies status code constants.
func TestStatusCodes(t *testing.T) {
	tests := []struct {
		name     string
		got      uint32
		expected uint32
	}{
		{"StatusOK", StatusOK, 200},
		{"StatusBadRequest", StatusBadRequest, 400},
		{"StatusInternalServerError", StatusInternalServerError, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestErrorCodes verifies error code constants.
func TestErrorCodes(t *testing.T) {
	tests := []struct {
		name     string
		got      uint32
		expected uint32
	}{
		{"ErrCodeNone", ErrCodeNone, 0},
		{"ErrCodeInvalidMagic", ErrCodeInvalidMagic, 1},
		{"ErrCodeUnsupportedVersion", ErrCodeUnsupportedVersion, 2},
		{"ErrCodeInvalidModelID", ErrCodeInvalidModelID, 3},
		{"ErrCodeInvalidPrompt", ErrCodeInvalidPrompt, 4},
		{"ErrCodeInvalidDimensions", ErrCodeInvalidDimensions, 5},
		{"ErrCodeInvalidSteps", ErrCodeInvalidSteps, 6},
		{"ErrCodeInvalidCFG", ErrCodeInvalidCFG, 7},
		{"ErrCodeOutOfMemory", ErrCodeOutOfMemory, 8},
		{"ErrCodeGPUError", ErrCodeGPUError, 9},
		{"ErrCodeTimeout", ErrCodeTimeout, 10},
		{"ErrCodeInternal", ErrCodeInternal, 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestModelIDs verifies model ID constants.
func TestModelIDs(t *testing.T) {
	if ModelIDSD35 != 0x00000000 {
		t.Errorf("ModelIDSD35 = 0x%08X, want 0x00000000", ModelIDSD35)
	}
}

// TestSentinelErrors verifies sentinel errors are defined.
func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrInvalidMagic", ErrInvalidMagic},
		{"ErrUnsupportedVersion", ErrUnsupportedVersion},
		{"ErrInvalidModelID", ErrInvalidModelID},
		{"ErrInvalidPrompt", ErrInvalidPrompt},
		{"ErrInvalidDimensions", ErrInvalidDimensions},
		{"ErrInvalidSteps", ErrInvalidSteps},
		{"ErrInvalidCFG", ErrInvalidCFG},
		{"ErrOutOfMemory", ErrOutOfMemory},
		{"ErrGPUError", ErrGPUError},
		{"ErrTimeout", ErrTimeout},
		{"ErrInternal", ErrInternal},
		{"ErrBufferTooSmall", ErrBufferTooSmall},
		{"ErrMessageTooLarge", ErrMessageTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil, want non-nil error", tt.name)
			}
			if tt.err.Error() == "" {
				t.Errorf("%s.Error() is empty, want non-empty message", tt.name)
			}
		})
	}
}

// TestGenerateRequestStructure verifies GenerateRequest field order.
func TestGenerateRequestStructure(t *testing.T) {
	var req GenerateRequest

	// After Header (16 bytes), we expect:
	// - RequestID at offset 16 (8 bytes)
	// - ModelID at offset 24 (4 bytes)

	headerSize := unsafe.Sizeof(req.Header)
	requestIDOffset := unsafe.Offsetof(req.RequestID)
	modelIDOffset := unsafe.Offsetof(req.ModelID)

	if headerSize != 16 {
		t.Errorf("Header size = %d, want 16", headerSize)
	}
	if requestIDOffset != 16 {
		t.Errorf("RequestID offset = %d, want 16", requestIDOffset)
	}
	if modelIDOffset != 24 {
		t.Errorf("ModelID offset = %d, want 24", modelIDOffset)
	}
}

// TestSD35GenerateRequestFields verifies SD35GenerateRequest has all required fields.
func TestSD35GenerateRequestFields(t *testing.T) {
	req := SD35GenerateRequest{
		GenerateRequest: GenerateRequest{
			Header: Header{
				Magic:      MagicNumber,
				Version:    ProtocolVersion1,
				MsgType:    MsgGenerateRequest,
				PayloadLen: 102,
				Reserved:   0,
			},
			RequestID: 1,
			ModelID:   ModelIDSD35,
		},
		Width:       512,
		Height:      512,
		Steps:       28,
		CFGScale:    7.0,
		Seed:        12345,
		CLIPLOffset: 0,
		CLIPLLength: 10,
		CLIPGOffset: 10,
		CLIPGLength: 10,
		T5Offset:    20,
		T5Length:    10,
		PromptData:  make([]byte, 30),
	}

	// Verify we can access all fields
	if req.Width != 512 {
		t.Errorf("Width = %d, want 512", req.Width)
	}
	if req.Height != 512 {
		t.Errorf("Height = %d, want 512", req.Height)
	}
	if req.Steps != 28 {
		t.Errorf("Steps = %d, want 28", req.Steps)
	}
	if req.CFGScale != 7.0 {
		t.Errorf("CFGScale = %f, want 7.0", req.CFGScale)
	}
	if req.Seed != 12345 {
		t.Errorf("Seed = %d, want 12345", req.Seed)
	}
}

// TestSD35ParameterBounds verifies SD35 parameter constants.
func TestSD35ParameterBounds(t *testing.T) {
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"SD35MinWidth", SD35MinWidth, uint32(64)},
		{"SD35MaxWidth", SD35MaxWidth, uint32(2048)},
		{"SD35MinHeight", SD35MinHeight, uint32(64)},
		{"SD35MaxHeight", SD35MaxHeight, uint32(2048)},
		{"SD35DimensionAlign", SD35DimensionAlign, uint32(64)},
		{"SD35MinSteps", SD35MinSteps, uint32(1)},
		{"SD35MaxSteps", SD35MaxSteps, uint32(100)},
		{"SD35MinCFG", SD35MinCFG, float32(0.0)},
		{"SD35MaxCFG", SD35MaxCFG, float32(20.0)},
		{"SD35MinPromptLen", SD35MinPromptLen, uint32(1)},
		{"SD35MaxPromptLen", SD35MaxPromptLen, uint32(2048)},
		{"SD35MaxPromptData", SD35MaxPromptData, uint32(6144)},
		{"SD35ChannelsRGB", SD35ChannelsRGB, uint32(3)},
		{"SD35ChannelsRGBA", SD35ChannelsRGBA, uint32(4)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestSD35ValidDimensions verifies dimension validation logic.
func TestSD35ValidDimensions(t *testing.T) {
	tests := []struct {
		name      string
		dimension uint32
		wantValid bool
	}{
		{"min valid", 64, true},
		{"below min", 63, false},
		{"zero", 0, false},
		{"not multiple of 64", 100, false},
		{"valid 512", 512, true},
		{"valid 768", 768, true},
		{"valid 1024", 1024, true},
		{"max valid", 2048, true},
		{"above max", 2112, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.dimension >= SD35MinWidth &&
				tt.dimension <= SD35MaxWidth &&
				tt.dimension%SD35DimensionAlign == 0

			if valid != tt.wantValid {
				t.Errorf("dimension %d: valid = %v, want %v", tt.dimension, valid, tt.wantValid)
			}
		})
	}
}

// TestSD35ValidSteps verifies steps validation logic.
func TestSD35ValidSteps(t *testing.T) {
	tests := []struct {
		name      string
		steps     uint32
		wantValid bool
	}{
		{"zero", 0, false},
		{"min valid", 1, true},
		{"typical", 28, true},
		{"max valid", 100, true},
		{"above max", 101, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.steps >= SD35MinSteps && tt.steps <= SD35MaxSteps

			if valid != tt.wantValid {
				t.Errorf("steps %d: valid = %v, want %v", tt.steps, valid, tt.wantValid)
			}
		})
	}
}

// TestSD35ValidCFG verifies CFG scale validation logic.
func TestSD35ValidCFG(t *testing.T) {
	tests := []struct {
		name      string
		cfg       float32
		wantValid bool
	}{
		{"below min", -0.1, false},
		{"min valid", 0.0, true},
		{"typical", 7.0, true},
		{"max valid", 20.0, true},
		{"above max", 20.1, false},
		{"NaN", float32(math.NaN()), false},
		{"Inf", float32(math.Inf(1)), false},
		{"NegInf", float32(math.Inf(-1)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.cfg >= SD35MinCFG &&
				tt.cfg <= SD35MaxCFG &&
				!math.IsNaN(float64(tt.cfg)) &&
				!math.IsInf(float64(tt.cfg), 0)

			if valid != tt.wantValid {
				t.Errorf("cfg %f: valid = %v, want %v", tt.cfg, valid, tt.wantValid)
			}
		})
	}
}

// TestSD35GenerateResponseStructure verifies response structure.
func TestSD35GenerateResponseStructure(t *testing.T) {
	resp := SD35GenerateResponse{
		GenerateResponse: GenerateResponse{
			Header: Header{
				Magic:      MagicNumber,
				Version:    ProtocolVersion1,
				MsgType:    MsgGenerateResponse,
				PayloadLen: 786464,
				Reserved:   0,
			},
			RequestID:      1,
			Status:         StatusOK,
			GenerationTime: 10000,
		},
		ImageWidth:   512,
		ImageHeight:  512,
		Channels:     SD35ChannelsRGB,
		ImageDataLen: 512 * 512 * 3,
		ImageData:    make([]byte, 512*512*3),
	}

	// Verify fields
	if resp.ImageWidth != 512 {
		t.Errorf("ImageWidth = %d, want 512", resp.ImageWidth)
	}
	if resp.ImageHeight != 512 {
		t.Errorf("ImageHeight = %d, want 512", resp.ImageHeight)
	}
	if resp.Channels != 3 {
		t.Errorf("Channels = %d, want 3", resp.Channels)
	}

	expectedLen := uint32(512 * 512 * 3)
	if resp.ImageDataLen != expectedLen {
		t.Errorf("ImageDataLen = %d, want %d", resp.ImageDataLen, expectedLen)
	}
	if uint32(len(resp.ImageData)) != expectedLen {
		t.Errorf("len(ImageData) = %d, want %d", len(resp.ImageData), expectedLen)
	}
}

// TestErrorResponseStructure verifies error response structure.
func TestErrorResponseStructure(t *testing.T) {
	errMsg := "invalid model id"
	resp := ErrorResponse{
		Header: Header{
			Magic:      MagicNumber,
			Version:    ProtocolVersion1,
			MsgType:    MsgError,
			PayloadLen: 34,
			Reserved:   0,
		},
		RequestID:    1,
		Status:       StatusBadRequest,
		ErrorCode:    ErrCodeInvalidModelID,
		ErrorMsgLen:  uint16(len(errMsg)),
		ErrorMessage: errMsg,
	}

	if resp.Status != StatusBadRequest {
		t.Errorf("Status = %d, want %d", resp.Status, StatusBadRequest)
	}
	if resp.ErrorCode != ErrCodeInvalidModelID {
		t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrCodeInvalidModelID)
	}
	if resp.ErrorMsgLen != 16 {
		t.Errorf("ErrorMsgLen = %d, want 16", resp.ErrorMsgLen)
	}
	if resp.ErrorMessage != errMsg {
		t.Errorf("ErrorMessage = %q, want %q", resp.ErrorMessage, errMsg)
	}
}

// TestPromptOffsetTableBounds verifies prompt offset validation logic.
func TestPromptOffsetTableBounds(t *testing.T) {
	tests := []struct {
		name      string
		offset    uint32
		length    uint32
		totalSize uint32
		wantValid bool
	}{
		{"valid at start", 0, 10, 30, true},
		{"valid in middle", 10, 10, 30, true},
		{"valid at end", 20, 10, 30, true},
		{"offset out of bounds", 31, 10, 30, false},
		{"length too long", 20, 11, 30, false},
		{"offset plus length overflow", 25, 10, 30, false},
		{"zero length", 0, 0, 30, false},
		{"length too long for max", 0, 2049, 2048, false},
		{"valid max length", 0, 2048, 2048, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate offset and length
			valid := tt.length >= SD35MinPromptLen &&
				tt.length <= SD35MaxPromptLen &&
				tt.offset <= tt.totalSize &&
				tt.length <= tt.totalSize-tt.offset

			if valid != tt.wantValid {
				t.Errorf("offset=%d, length=%d, total=%d: valid = %v, want %v",
					tt.offset, tt.length, tt.totalSize, valid, tt.wantValid)
			}
		})
	}
}

// TestImageDataLenCalculation verifies image data length calculation.
func TestImageDataLenCalculation(t *testing.T) {
	tests := []struct {
		name     string
		width    uint32
		height   uint32
		channels uint32
		expected uint32
	}{
		{"512x512 RGB", 512, 512, 3, 786432},
		{"1024x1024 RGB", 1024, 1024, 3, 3145728},
		{"512x512 RGBA", 512, 512, 4, 1048576},
		{"64x64 RGB", 64, 64, 3, 12288},
		{"2048x2048 RGB", 2048, 2048, 3, 12582912},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.width * tt.height * tt.channels
			if result != tt.expected {
				t.Errorf("imageDataLen = %d, want %d", result, tt.expected)
			}
		})
	}
}
