// Package resources provides access to embedded application resources.
// All web assets (templates, static files) and agent prompts are embedded
// at compile time via go:embed, making the binary self-contained.
package resources

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:embed
var embeddedFS embed.FS

// Embedded returns the embedded filesystem with all application resources.
// The returned FS contains "web/" (templates, static files) and "config/"
// (agent prompts) as top-level directories. Callers use fs.Sub or fs.ReadFile
// to access specific resources.
func Embedded() (fs.FS, error) {
	sub, err := fs.Sub(embeddedFS, "embed")
	if err != nil {
		return nil, fmt.Errorf("failed to access embedded resources: %w", err)
	}
	return sub, nil
}
