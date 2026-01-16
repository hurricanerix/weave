// Package protocol implements the binary protocol for communication
// between weave (Go orchestration) and weave-compute (C GPU process).
package protocol

import "errors"

// Protocol version constants
const (
	ProtocolVersion1    uint16 = 0x0001
	MinSupportedVersion uint16 = ProtocolVersion1
	MaxSupportedVersion uint16 = ProtocolVersion1
	MagicNumber         uint32 = 0x57455645       // "WEVE"
	MaxMessageSize      uint32 = 10 * 1024 * 1024 // 10 MB
)

// Message type constants
const (
	MsgGenerateRequest  uint16 = 0x0001
	MsgGenerateResponse uint16 = 0x0002
	MsgError            uint16 = 0x00FF
)

// Status codes (HTTP-like)
const (
	StatusOK                  uint32 = 200
	StatusBadRequest          uint32 = 400
	StatusInternalServerError uint32 = 500
)

// Error codes
const (
	ErrCodeNone               uint32 = 0
	ErrCodeInvalidMagic       uint32 = 1
	ErrCodeUnsupportedVersion uint32 = 2
	ErrCodeInvalidModelID     uint32 = 3
	ErrCodeInvalidPrompt      uint32 = 4
	ErrCodeInvalidDimensions  uint32 = 5
	ErrCodeInvalidSteps       uint32 = 6
	ErrCodeInvalidCFG         uint32 = 7
	ErrCodeOutOfMemory        uint32 = 8
	ErrCodeGPUError           uint32 = 9
	ErrCodeTimeout            uint32 = 10
	ErrCodeInternal           uint32 = 99
)

// Model identifiers
const (
	ModelIDSD35 uint32 = 0x00000000 // Stable Diffusion 3.5
)

// Sentinel errors
var (
	ErrInvalidMagic       = errors.New("invalid magic number")
	ErrUnsupportedVersion = errors.New("unsupported protocol version")
	ErrInvalidModelID     = errors.New("invalid model ID")
	ErrInvalidPrompt      = errors.New("invalid prompt")
	ErrInvalidDimensions  = errors.New("invalid dimensions")
	ErrInvalidSteps       = errors.New("invalid steps")
	ErrInvalidCFG         = errors.New("invalid CFG scale")
	ErrOutOfMemory        = errors.New("out of memory")
	ErrGPUError           = errors.New("GPU error")
	ErrTimeout            = errors.New("timeout")
	ErrInternal           = errors.New("internal error")
	ErrBufferTooSmall     = errors.New("buffer too small")
	ErrMessageTooLarge    = errors.New("message too large")
)

// Header represents the common 16-byte header present in every message.
type Header struct {
	Magic      uint32 // 0x57455645 ("WEVE")
	Version    uint16 // Protocol version
	MsgType    uint16 // Message type (request/response/error)
	PayloadLen uint32 // Length of data following header
	Reserved   uint32 // Must be 0x00000000
}

// GenerateRequest represents the common fields in all generation requests.
// Model-specific payload follows these fields.
type GenerateRequest struct {
	Header    Header
	RequestID uint64 // Unique request ID for tracing
	ModelID   uint32 // Model identifier (0 = SD35)
}

// GenerateResponse represents a successful generation response (status 200).
type GenerateResponse struct {
	Header         Header
	RequestID      uint64 // Echoed from request
	Status         uint32 // Status code (200)
	GenerationTime uint32 // Milliseconds elapsed
}

// ErrorResponse represents an error response (status 400/500).
type ErrorResponse struct {
	Header       Header
	RequestID    uint64 // Echoed from request, or 0 if request invalid
	Status       uint32 // Status code (400 or 500)
	ErrorCode    uint32 // Machine-readable error identifier
	ErrorMsgLen  uint16 // Length of error message
	ErrorMessage string // Human-readable error description (UTF-8)
}

// SD35GenerateRequest represents a Stable Diffusion 3.5 generation request.
// This includes the common request fields plus SD35-specific parameters.
type SD35GenerateRequest struct {
	GenerateRequest

	// Generation parameters
	Width    uint32  // Image width (64-2048, multiple of 64)
	Height   uint32  // Image height (64-2048, multiple of 64)
	Steps    uint32  // Inference steps (1-100)
	CFGScale float32 // Classifier-Free Guidance scale (0.0-20.0)
	Seed     uint64  // Random seed (0 = random)

	// Prompt offset table
	CLIPLOffset uint32 // Offset of CLIP-L prompt in PromptData
	CLIPLLength uint32 // Length of CLIP-L prompt
	CLIPGOffset uint32 // Offset of CLIP-G prompt in PromptData
	CLIPGLength uint32 // Length of CLIP-G prompt
	T5Offset    uint32 // Offset of T5 prompt in PromptData
	T5Length    uint32 // Length of T5 prompt

	// Prompt data (contains all three prompts)
	PromptData []byte
}

// SD35GenerateResponse represents a successful SD 3.5 generation response.
type SD35GenerateResponse struct {
	GenerateResponse

	// Image metadata
	ImageWidth   uint32 // Actual image width
	ImageHeight  uint32 // Actual image height
	Channels     uint32 // Number of channels (3=RGB, 4=RGBA)
	ImageDataLen uint32 // Size of image data in bytes

	// Image data (raw RGB/RGBA pixels)
	ImageData []byte
}

// SD35 parameter bounds
const (
	SD35MinWidth       uint32  = 64
	SD35MaxWidth       uint32  = 2048
	SD35MinHeight      uint32  = 64
	SD35MaxHeight      uint32  = 2048
	SD35DimensionAlign uint32  = 64 // Dimensions must be multiple of 64
	SD35MinSteps       uint32  = 1
	SD35MaxSteps       uint32  = 100
	SD35MinCFG         float32 = 0.0
	SD35MaxCFG         float32 = 20.0
	SD35MinPromptLen   uint32  = 1
	SD35MaxPromptLen   uint32  = 256 // Per encoder (limited by CLIP/T5 token mismatch bug)
	SD35MaxPromptData  uint32  = 768 // 3 * 256
	SD35ChannelsRGB    uint32  = 3
	SD35ChannelsRGBA   uint32  = 4
)
