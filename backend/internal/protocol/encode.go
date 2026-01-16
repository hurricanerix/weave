package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeSD35GenerateRequest encodes an SD35GenerateRequest to bytes.
// Returns the encoded message or an error if validation fails.
func EncodeSD35GenerateRequest(req *SD35GenerateRequest) ([]byte, error) {
	// Validate parameters before encoding
	if err := validateSD35Request(req); err != nil {
		return nil, err
	}

	// Calculate sizes
	// Common request fields: 12 bytes (request_id=8 + model_id=4)
	// SD35 params: 48 bytes (width=4 + height=4 + steps=4 + cfg=4 + seed=8 + offset_table=24)
	// Prompt data: 3 * len(prompt) bytes
	promptLen := uint32(len(req.PromptData))
	sd35PayloadSize := uint32(48 + promptLen)
	payloadLen := 12 + sd35PayloadSize

	// Check total message size
	totalSize := 16 + payloadLen // header + payload
	if totalSize > MaxMessageSize {
		return nil, fmt.Errorf("%w: %d bytes exceeds limit of %d", ErrMessageTooLarge, totalSize, MaxMessageSize)
	}

	buf := new(bytes.Buffer)

	// Common header (16 bytes)
	binary.Write(buf, binary.BigEndian, MagicNumber)
	binary.Write(buf, binary.BigEndian, ProtocolVersion1)
	binary.Write(buf, binary.BigEndian, MsgGenerateRequest)
	binary.Write(buf, binary.BigEndian, payloadLen)
	binary.Write(buf, binary.BigEndian, uint32(0)) // reserved

	// Common request fields (12 bytes)
	binary.Write(buf, binary.BigEndian, req.RequestID)
	binary.Write(buf, binary.BigEndian, req.ModelID)

	// SD35 generation parameters (48 bytes)
	binary.Write(buf, binary.BigEndian, req.Width)
	binary.Write(buf, binary.BigEndian, req.Height)
	binary.Write(buf, binary.BigEndian, req.Steps)
	binary.Write(buf, binary.BigEndian, math.Float32bits(req.CFGScale))
	binary.Write(buf, binary.BigEndian, req.Seed)

	// Prompt offset table (24 bytes)
	binary.Write(buf, binary.BigEndian, req.CLIPLOffset)
	binary.Write(buf, binary.BigEndian, req.CLIPLLength)
	binary.Write(buf, binary.BigEndian, req.CLIPGOffset)
	binary.Write(buf, binary.BigEndian, req.CLIPGLength)
	binary.Write(buf, binary.BigEndian, req.T5Offset)
	binary.Write(buf, binary.BigEndian, req.T5Length)

	// Prompt data (variable)
	buf.Write(req.PromptData)

	return buf.Bytes(), nil
}

// validateSD35Request validates all parameters of an SD35GenerateRequest.
func validateSD35Request(req *SD35GenerateRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	// Validate dimensions
	if req.Width < SD35MinWidth || req.Width > SD35MaxWidth {
		return fmt.Errorf("%w: width %d not in range [%d, %d]", ErrInvalidDimensions, req.Width, SD35MinWidth, SD35MaxWidth)
	}
	if req.Width%SD35DimensionAlign != 0 {
		return fmt.Errorf("%w: width %d not multiple of %d", ErrInvalidDimensions, req.Width, SD35DimensionAlign)
	}
	if req.Height < SD35MinHeight || req.Height > SD35MaxHeight {
		return fmt.Errorf("%w: height %d not in range [%d, %d]", ErrInvalidDimensions, req.Height, SD35MinHeight, SD35MaxHeight)
	}
	if req.Height%SD35DimensionAlign != 0 {
		return fmt.Errorf("%w: height %d not multiple of %d", ErrInvalidDimensions, req.Height, SD35DimensionAlign)
	}

	// Validate steps
	if req.Steps < SD35MinSteps || req.Steps > SD35MaxSteps {
		return fmt.Errorf("%w: steps %d not in range [%d, %d]", ErrInvalidSteps, req.Steps, SD35MinSteps, SD35MaxSteps)
	}

	// Validate CFG scale
	if math.IsNaN(float64(req.CFGScale)) {
		return fmt.Errorf("%w: cfg_scale is NaN", ErrInvalidCFG)
	}
	if math.IsInf(float64(req.CFGScale), 0) {
		return fmt.Errorf("%w: cfg_scale is infinite", ErrInvalidCFG)
	}
	if req.CFGScale < SD35MinCFG || req.CFGScale > SD35MaxCFG {
		return fmt.Errorf("%w: cfg_scale %.2f not in range [%.1f, %.1f]", ErrInvalidCFG, req.CFGScale, SD35MinCFG, SD35MaxCFG)
	}

	// Validate model ID
	if req.ModelID != ModelIDSD35 {
		return fmt.Errorf("%w: model_id %d not supported (expected %d)", ErrInvalidModelID, req.ModelID, ModelIDSD35)
	}

	// Validate prompt data
	promptDataLen := uint32(len(req.PromptData))
	if promptDataLen == 0 {
		return fmt.Errorf("%w: prompt_data is empty", ErrInvalidPrompt)
	}
	if promptDataLen > SD35MaxPromptData {
		return fmt.Errorf("%w: prompt_data size %d exceeds maximum %d", ErrInvalidPrompt, promptDataLen, SD35MaxPromptData)
	}

	// Validate prompt lengths
	if req.CLIPLLength < SD35MinPromptLen || req.CLIPLLength > SD35MaxPromptLen {
		return fmt.Errorf("%w: clip_l_length %d not in range [%d, %d]", ErrInvalidPrompt, req.CLIPLLength, SD35MinPromptLen, SD35MaxPromptLen)
	}
	if req.CLIPGLength < SD35MinPromptLen || req.CLIPGLength > SD35MaxPromptLen {
		return fmt.Errorf("%w: clip_g_length %d not in range [%d, %d]", ErrInvalidPrompt, req.CLIPGLength, SD35MinPromptLen, SD35MaxPromptLen)
	}
	if req.T5Length < SD35MinPromptLen || req.T5Length > SD35MaxPromptLen {
		return fmt.Errorf("%w: t5_length %d not in range [%d, %d]", ErrInvalidPrompt, req.T5Length, SD35MinPromptLen, SD35MaxPromptLen)
	}

	// Validate prompt offsets (bounds checking)
	// Check CLIP-L bounds
	if req.CLIPLOffset > promptDataLen {
		return fmt.Errorf("%w: clip_l_offset %d exceeds prompt_data size %d", ErrInvalidPrompt, req.CLIPLOffset, promptDataLen)
	}
	if req.CLIPLLength > promptDataLen-req.CLIPLOffset {
		return fmt.Errorf("%w: clip_l region [%d:%d] exceeds prompt_data size %d", ErrInvalidPrompt, req.CLIPLOffset, req.CLIPLOffset+req.CLIPLLength, promptDataLen)
	}

	// Check CLIP-G bounds
	if req.CLIPGOffset > promptDataLen {
		return fmt.Errorf("%w: clip_g_offset %d exceeds prompt_data size %d", ErrInvalidPrompt, req.CLIPGOffset, promptDataLen)
	}
	if req.CLIPGLength > promptDataLen-req.CLIPGOffset {
		return fmt.Errorf("%w: clip_g region [%d:%d] exceeds prompt_data size %d", ErrInvalidPrompt, req.CLIPGOffset, req.CLIPGOffset+req.CLIPGLength, promptDataLen)
	}

	// Check T5 bounds
	if req.T5Offset > promptDataLen {
		return fmt.Errorf("%w: t5_offset %d exceeds prompt_data size %d", ErrInvalidPrompt, req.T5Offset, promptDataLen)
	}
	if req.T5Length > promptDataLen-req.T5Offset {
		return fmt.Errorf("%w: t5 region [%d:%d] exceeds prompt_data size %d", ErrInvalidPrompt, req.T5Offset, req.T5Offset+req.T5Length, promptDataLen)
	}

	return nil
}

// NewSD35GenerateRequest creates a new SD35GenerateRequest with the prompt
// automatically duplicated three times as required by the SD35 spec.
// This is a convenience function for the common case where all three encoders
// use the same prompt text.
func NewSD35GenerateRequest(requestID uint64, prompt string, width, height, steps uint32, cfgScale float32, seed uint64) (*SD35GenerateRequest, error) {
	promptBytes := []byte(prompt)
	promptLen := uint32(len(promptBytes))

	if promptLen < SD35MinPromptLen {
		return nil, fmt.Errorf("%w: prompt length %d less than minimum %d", ErrInvalidPrompt, promptLen, SD35MinPromptLen)
	}
	if promptLen > SD35MaxPromptLen {
		return nil, fmt.Errorf("%w: prompt length %d exceeds maximum %d", ErrInvalidPrompt, promptLen, SD35MaxPromptLen)
	}

	// Duplicate prompt three times
	promptData := make([]byte, 0, promptLen*3)
	promptData = append(promptData, promptBytes...) // CLIP-L
	promptData = append(promptData, promptBytes...) // CLIP-G
	promptData = append(promptData, promptBytes...) // T5

	req := &SD35GenerateRequest{
		GenerateRequest: GenerateRequest{
			Header: Header{
				Magic:      MagicNumber,
				Version:    ProtocolVersion1,
				MsgType:    MsgGenerateRequest,
				PayloadLen: 0, // Will be calculated during encoding
				Reserved:   0,
			},
			RequestID: requestID,
			ModelID:   ModelIDSD35,
		},
		Width:    width,
		Height:   height,
		Steps:    steps,
		CFGScale: cfgScale,
		Seed:     seed,

		// Offset table for three identical prompts
		CLIPLOffset: 0,
		CLIPLLength: promptLen,
		CLIPGOffset: promptLen,
		CLIPGLength: promptLen,
		T5Offset:    promptLen * 2,
		T5Length:    promptLen,

		PromptData: promptData,
	}

	return req, nil
}
