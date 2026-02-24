package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// httpPoster posts raw JSON to the orchestrator's journal endpoint over HTTP.
type httpPoster struct {
	// endpoint is the full URL to POST to, e.g.
	// "http://host:port/api/agent/journal".
	endpoint string

	// runID is included as the X-Run-ID header on every request.
	runID string

	// client is the HTTP client used for requests.
	client *http.Client
}

// Post sends body as a JSON POST to the endpoint with the run ID header.
func (p *httpPoster) Post(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Run-ID", p.runID)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("posting journal entry: %w", err)
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse; errors are irrelevant here.
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("journal endpoint returned %d", resp.StatusCode)
	}
	return nil
}
