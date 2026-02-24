// Package relay implements the core logic for forwarding JSON objects from
// an io.Reader (typically stdin) to the orchestrator's HTTP journal endpoint.
package relay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
)

// Poster sends a raw JSON body to the journal endpoint. Implementations
// handle URL construction, headers, and transport. Tests can substitute a
// mock to avoid real HTTP calls.
type Poster interface {
	Post(ctx context.Context, body []byte) error
}

// Run reads newline-delimited JSON objects from r and forwards each one to the
// poster. It logs errors to logger and continues on both malformed JSON and
// POST failures. It returns nil when the reader reaches EOF.
//
// Claude Code's --output-format json produces newline-delimited JSON (one
// JSON object per line). Reading line-by-line ensures that a malformed line
// is skipped cleanly without corrupting the decoder state for subsequent lines.
func Run(ctx context.Context, r io.Reader, poster Poster, logger *log.Logger) error {
	scanner := bufio.NewScanner(r)
	// Claude Code can produce large JSON lines (e.g., assistant responses with
	// code blocks). Increase the buffer beyond the default 64KB to avoid silent
	// data loss when a single JSON object exceeds the default limit.
	const maxLineSize = 1024 * 1024 // 1MB
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		// Skip blank lines.
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		// Verify the line is valid JSON before forwarding.
		if !json.Valid(line) {
			logger.Printf("decode error: invalid JSON: %s", line)
			continue
		}

		if err := poster.Post(ctx, line); err != nil {
			// Best-effort delivery: log and continue.
			logger.Printf("post failed: %v", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	return nil
}
