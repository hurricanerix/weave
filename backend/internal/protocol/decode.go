package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// DecodeResponse decodes a response message from the given byte slice.
// It returns either a *SD35GenerateResponse or *ErrorResponse depending on the message type.
// Returns an error if the message is invalid, truncated, or malformed.
func DecodeResponse(data []byte) (interface{}, error) {
	// Validate minimum message size (common header = 16 bytes)
	if len(data) < 16 {
		return nil, fmt.Errorf("message too small: got %d bytes, need at least 16", len(data))
	}

	// Decode common header
	header, err := decodeHeader(data)
	if err != nil {
		return nil, err
	}

	// Validate payload length
	if header.PayloadLen > MaxMessageSize {
		return nil, fmt.Errorf("%w: payload_len %d exceeds max %d", ErrMessageTooLarge, header.PayloadLen, MaxMessageSize)
	}

	// Check total message size
	expectedSize := 16 + int(header.PayloadLen)
	if len(data) < expectedSize {
		return nil, fmt.Errorf("truncated message: got %d bytes, expected %d", len(data), expectedSize)
	}

	// Route to appropriate decoder based on message type
	switch header.MsgType {
	case MsgGenerateResponse:
		return decodeGenerateResponse(header, data[16:16+header.PayloadLen])
	case MsgError:
		return decodeErrorResponse(header, data[16:16+header.PayloadLen])
	default:
		return nil, fmt.Errorf("unexpected message type: 0x%04X (expected RESPONSE or ERROR)", header.MsgType)
	}
}

// decodeHeader decodes the common 16-byte header from the start of data.
func decodeHeader(data []byte) (Header, error) {
	if len(data) < 16 {
		return Header{}, fmt.Errorf("header too small: got %d bytes, need 16", len(data))
	}

	buf := bytes.NewReader(data)
	var h Header

	// Read all header fields in big-endian order
	if err := binary.Read(buf, binary.BigEndian, &h.Magic); err != nil {
		return Header{}, fmt.Errorf("failed to read magic: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &h.Version); err != nil {
		return Header{}, fmt.Errorf("failed to read version: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &h.MsgType); err != nil {
		return Header{}, fmt.Errorf("failed to read msg_type: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &h.PayloadLen); err != nil {
		return Header{}, fmt.Errorf("failed to read payload_len: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &h.Reserved); err != nil {
		return Header{}, fmt.Errorf("failed to read reserved: %w", err)
	}

	// Validate magic number
	if h.Magic != MagicNumber {
		return Header{}, fmt.Errorf("%w: got 0x%08X, expected 0x%08X", ErrInvalidMagic, h.Magic, MagicNumber)
	}

	// Validate protocol version
	if h.Version < MinSupportedVersion || h.Version > MaxSupportedVersion {
		return Header{}, fmt.Errorf("%w: got 0x%04X, supported range 0x%04X-0x%04X",
			ErrUnsupportedVersion, h.Version, MinSupportedVersion, MaxSupportedVersion)
	}

	return h, nil
}

// decodeGenerateResponse decodes a MSG_GENERATE_RESPONSE payload (status 200).
// Payload structure:
//   - request_id (8 bytes)
//   - status (4 bytes)
//   - generation_time (4 bytes)
//   - image_width (4 bytes)
//   - image_height (4 bytes)
//   - channels (4 bytes)
//   - image_data_len (4 bytes)
//   - image_data (variable)
func decodeGenerateResponse(header Header, payload []byte) (*SD35GenerateResponse, error) {
	// Minimum payload: common response (16) + image header (16) = 32 bytes
	if len(payload) < 32 {
		return nil, fmt.Errorf("generate response payload too small: got %d bytes, need at least 32", len(payload))
	}

	buf := bytes.NewReader(payload)
	var resp SD35GenerateResponse
	resp.Header = header

	// Read common response fields (16 bytes)
	if err := binary.Read(buf, binary.BigEndian, &resp.RequestID); err != nil {
		return nil, fmt.Errorf("failed to read request_id: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &resp.Status); err != nil {
		return nil, fmt.Errorf("failed to read status: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &resp.GenerationTime); err != nil {
		return nil, fmt.Errorf("failed to read generation_time: %w", err)
	}

	// Validate status code
	if resp.Status != StatusOK {
		return nil, fmt.Errorf("invalid status for GENERATE_RESPONSE: got %d, expected %d", resp.Status, StatusOK)
	}

	// Read SD35 image header (16 bytes)
	if err := binary.Read(buf, binary.BigEndian, &resp.ImageWidth); err != nil {
		return nil, fmt.Errorf("failed to read image_width: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &resp.ImageHeight); err != nil {
		return nil, fmt.Errorf("failed to read image_height: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &resp.Channels); err != nil {
		return nil, fmt.Errorf("failed to read channels: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &resp.ImageDataLen); err != nil {
		return nil, fmt.Errorf("failed to read image_data_len: %w", err)
	}

	// Validate image dimensions
	if resp.ImageWidth < SD35MinWidth || resp.ImageWidth > SD35MaxWidth || resp.ImageWidth%SD35DimensionAlign != 0 {
		return nil, fmt.Errorf("%w: width %d (must be %d-%d, multiple of %d)",
			ErrInvalidDimensions, resp.ImageWidth, SD35MinWidth, SD35MaxWidth, SD35DimensionAlign)
	}
	if resp.ImageHeight < SD35MinHeight || resp.ImageHeight > SD35MaxHeight || resp.ImageHeight%SD35DimensionAlign != 0 {
		return nil, fmt.Errorf("%w: height %d (must be %d-%d, multiple of %d)",
			ErrInvalidDimensions, resp.ImageHeight, SD35MinHeight, SD35MaxHeight, SD35DimensionAlign)
	}

	// Validate channels
	if resp.Channels != SD35ChannelsRGB && resp.Channels != SD35ChannelsRGBA {
		return nil, fmt.Errorf("invalid channels: got %d, expected %d (RGB) or %d (RGBA)",
			resp.Channels, SD35ChannelsRGB, SD35ChannelsRGBA)
	}

	// Validate image_data_len matches dimensions
	// Check for integer overflow first
	if resp.ImageWidth > math.MaxUint32/resp.ImageHeight {
		return nil, fmt.Errorf("image dimensions too large: width * height would overflow")
	}
	if resp.ImageWidth*resp.ImageHeight > math.MaxUint32/resp.Channels {
		return nil, fmt.Errorf("image dimensions too large: width * height * channels would overflow")
	}
	expectedLen := resp.ImageWidth * resp.ImageHeight * resp.Channels
	if resp.ImageDataLen != expectedLen {
		return nil, fmt.Errorf("image_data_len mismatch: got %d, expected %d (width %d * height %d * channels %d)",
			resp.ImageDataLen, expectedLen, resp.ImageWidth, resp.ImageHeight, resp.Channels)
	}

	// Validate we have enough bytes for image data
	remaining := len(payload) - 32 // Common response (16) + image header (16)
	if uint32(remaining) < resp.ImageDataLen {
		return nil, fmt.Errorf("truncated image data: got %d bytes, expected %d", remaining, resp.ImageDataLen)
	}

	// Read image data
	resp.ImageData = make([]byte, resp.ImageDataLen)
	if _, err := io.ReadFull(buf, resp.ImageData); err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	return &resp, nil
}

// decodeErrorResponse decodes a MSG_ERROR payload (status 400/500).
// Payload structure:
//   - request_id (8 bytes)
//   - status (4 bytes)
//   - error_code (4 bytes)
//   - error_msg_len (2 bytes)
//   - error_msg (variable, UTF-8)
func decodeErrorResponse(header Header, payload []byte) (*ErrorResponse, error) {
	// Minimum payload: request_id (8) + status (4) + error_code (4) + msg_len (2) = 18 bytes
	if len(payload) < 18 {
		return nil, fmt.Errorf("error response payload too small: got %d bytes, need at least 18", len(payload))
	}

	buf := bytes.NewReader(payload)
	var resp ErrorResponse
	resp.Header = header

	// Read error response fields
	if err := binary.Read(buf, binary.BigEndian, &resp.RequestID); err != nil {
		return nil, fmt.Errorf("failed to read request_id: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &resp.Status); err != nil {
		return nil, fmt.Errorf("failed to read status: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &resp.ErrorCode); err != nil {
		return nil, fmt.Errorf("failed to read error_code: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &resp.ErrorMsgLen); err != nil {
		return nil, fmt.Errorf("failed to read error_msg_len: %w", err)
	}

	// Validate status code
	if resp.Status != StatusBadRequest && resp.Status != StatusInternalServerError {
		return nil, fmt.Errorf("invalid status for ERROR: got %d, expected %d or %d",
			resp.Status, StatusBadRequest, StatusInternalServerError)
	}

	// Validate error message length
	remaining := len(payload) - 18
	if int(resp.ErrorMsgLen) > remaining {
		return nil, fmt.Errorf("truncated error message: got %d bytes, expected %d", remaining, resp.ErrorMsgLen)
	}

	// Read error message
	if resp.ErrorMsgLen > 0 {
		msgBytes := make([]byte, resp.ErrorMsgLen)
		if _, err := io.ReadFull(buf, msgBytes); err != nil {
			return nil, fmt.Errorf("failed to read error message: %w", err)
		}
		resp.ErrorMessage = string(msgBytes)
	}

	return &resp, nil
}
