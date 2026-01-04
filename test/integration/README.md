# Integration Tests

This directory contains integration tests that verify the binary protocol implementation across the Go and C boundary.

## Overview

The integration tests verify that:
1. Go correctly encodes SD35GenerateRequest messages
2. C correctly decodes those requests
3. C correctly encodes SD35GenerateResponse messages
4. Go correctly decodes those responses
5. Image data integrity is preserved across the round-trip

## Architecture

The tests use a stub generator approach:

```
Go Test          C Stub Generator         Go Test
--------         ----------------         --------
Encode    --->   stdin
Request          |
                 v
                 Decode Request
                 Generate Pattern
                 Encode Response
                 |
                 v
                 stdout           --->    Decode
                                          Response
                                          Verify
```

### C Stub Generator

Located at `compute-daemon/test/test_stub_generator.c`.

This program:
- Reads a binary protocol request from stdin
- Decodes it using the C decoder
- Generates a checkerboard test pattern (8x8 blocks of 0x00 and 0xFF)
- Encodes a response using the C encoder
- Writes the response to stdout

The stub generator uses the same protocol implementation that the actual compute daemon will use, ensuring the integration test validates the real code paths.

## Running Tests

### Prerequisites

Build the C stub generator:
```bash
cd compute-daemon
make test-stub
```

### Run Integration Tests

From the project root:
```bash
go test -v -tags=integration ./test/integration/
```

### Test Coverage

The integration tests cover:
- 64x64 RGB (minimum dimensions)
- 512x512 RGB (typical dimensions)
- 1024x1024 RGB (larger dimensions)
- Multiple sequential requests

For each test case, we verify:
- Request ID is echoed correctly
- Status code is STATUS_OK (200)
- Image dimensions match the request
- Number of channels is correct (3 for RGB)
- Image data length matches width * height * channels
- Checkerboard pattern is correct

## Test Pattern

The stub generator creates a checkerboard pattern with 8x8 pixel blocks:
- Black blocks: all channels 0x00
- White blocks: all channels 0xFF

The pattern alternates based on block coordinates: if (blockX + blockY) is odd, the block is white, otherwise black.

## Adding New Tests

To add a new integration test:

1. Create a test function in `protocol_roundtrip_test.go`
2. Use `protocol.NewSD35GenerateRequest()` to create a request
3. Encode with `protocol.EncodeSD35GenerateRequest()`
4. Pass to stub generator via `runStubGenerator()`
5. Decode response with `protocol.DecodeResponse()`
6. Verify all fields and image data

Example:
```go
func TestProtocolRoundTrip_MyTest(t *testing.T) {
    req, err := protocol.NewSD35GenerateRequest(
        123,           // request_id
        "my prompt",   // prompt
        256,           // width
        256,           // height
        28,            // steps
        7.0,           // cfg_scale
        0,             // seed
    )
    require.NoError(t, err)

    requestBytes, err := protocol.EncodeSD35GenerateRequest(req)
    require.NoError(t, err)

    responseBytes, err := runStubGenerator(t, requestBytes)
    require.NoError(t, err)

    resp, err := protocol.DecodeResponse(responseBytes)
    require.NoError(t, err)

    sd35Resp := resp.(*protocol.SD35GenerateResponse)
    assert.Equal(t, req.RequestID, sd35Resp.RequestID)
    // ... more assertions

    verifyCheckerboard(t, sd35Resp.ImageWidth, sd35Resp.ImageHeight,
                      sd35Resp.Channels, sd35Resp.ImageData)
}
```

## Debugging

If a test fails:

1. Check the stub generator built successfully:
   ```bash
   ls -la compute-daemon/test/test_stub_generator
   ```

2. Test the stub generator manually:
   ```bash
   # Create a simple test request and pipe it through
   go run ./test/integration/manual_request.go | \
     ./compute-daemon/test/test_stub_generator | \
     hexdump -C
   ```

3. Enable verbose output:
   ```bash
   go test -v -tags=integration ./test/integration/ -run TestName
   ```

4. Check stderr from stub generator (captured in test error messages)

## Notes

- These are tagged as integration tests to separate them from fast unit tests
- The stub generator does NOT perform actual GPU inference
- Test patterns are deterministic and verifiable
- All tests must pass before merging protocol changes
