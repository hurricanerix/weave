package startup

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hurricanerix/weave/internal/logging"
	"github.com/hurricanerix/weave/internal/web"
)

const (
	// shutdownTimeout is the maximum time to wait for graceful shutdown
	shutdownTimeout = 30 * time.Second
)

// Run starts the web server and blocks until a shutdown signal is received.
// It handles SIGTERM and SIGINT signals for graceful shutdown.
//
// Parameters:
//   - ctx: Context for server lifecycle (cancellation triggers shutdown)
//   - server: Web server to run
//   - logger: Logger for shutdown messages
//
// Returns nil on clean shutdown, error otherwise.
func Run(ctx context.Context, server *web.Server, logger *logging.Logger) error {
	// Create context that will be cancelled on signal
	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server and wait for it to finish
	// ListenAndServe blocks until context is cancelled or error occurs
	// The web.Server itself logs "Shutting down..." and "Web server stopped"
	err := server.ListenAndServe(shutdownCtx)
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
