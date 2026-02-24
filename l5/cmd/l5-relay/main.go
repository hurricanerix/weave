// l5-relay reads streaming JSON from stdin and POSTs each entry to the
// orchestrator's journal endpoint. It runs inside a container, piped to
// an agent's stdout, providing real-time visibility into agent activity.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hurricanerix/l5/internal/relay"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := log.New(os.Stderr, "l5-relay: ", log.LstdFlags)

	orchestratorURL := os.Getenv("ORCHESTRATOR_URL")
	if orchestratorURL == "" {
		fmt.Fprintln(os.Stderr, "l5-relay: ORCHESTRATOR_URL environment variable is required")
		return 1
	}

	runID := os.Getenv("RUN_ID")
	if runID == "" {
		fmt.Fprintln(os.Stderr, "l5-relay: RUN_ID environment variable is required")
		return 1
	}

	endpoint := strings.TrimRight(orchestratorURL, "/") + "/api/agent/journal"

	poster := &httpPoster{
		endpoint: endpoint,
		runID:    runID,
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	if err := relay.Run(context.Background(), os.Stdin, poster, logger); err != nil {
		logger.Printf("relay exited with error: %v", err)
		return 1
	}

	return 0
}
