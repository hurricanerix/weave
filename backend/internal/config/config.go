// Package config provides configuration management for the weave application.
//
// Configuration is parsed from CLI flags with sensible defaults.
// The Config struct is passed to components during initialization.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Version is the weave application version
	Version = "0.1.0-mvp"

	// Default values for CLI flags
	defaultPort        = 8080
	defaultSteps       = 20
	defaultCFG         = 3.5
	defaultWidth       = 1024
	defaultHeight      = 1024
	defaultSeed        = -1
	defaultLLMSeed     = 0
	defaultOllamaURL   = "http://localhost:11434"
	defaultOllamaModel = "llama3.1:8b"
	defaultLogLevel    = "info"
	// DefaultAgentPrompt is the default path to the agent prompt file
	DefaultAgentPrompt = "config/agents/ara.md"

	// Validation constraints
	minPort    = 1024
	maxPort    = 65535
	minSteps   = 1
	maxSteps   = 100
	minCFG     = 0.0
	maxCFG     = 20.0
	minWidth   = 64
	maxWidth   = 2048
	minHeight  = 64
	maxHeight  = 2048
	widthStep  = 64
	heightStep = 64
	minLLMSeed = 0
	minSeed    = -1
)

var (
	// ErrInvalidPort is returned when port is out of valid range
	ErrInvalidPort = errors.New("port must be between 1024 and 65535")
	// ErrInvalidSteps is returned when steps is out of valid range
	ErrInvalidSteps = errors.New("steps must be between 1 and 100")
	// ErrInvalidCFG is returned when CFG scale is out of valid range
	ErrInvalidCFG = errors.New("cfg must be between 0.0 and 20.0")
	// ErrInvalidWidth is returned when width is invalid
	ErrInvalidWidth = errors.New("width must be between 64 and 2048 and a multiple of 64")
	// ErrInvalidHeight is returned when height is invalid
	ErrInvalidHeight = errors.New("height must be between 64 and 2048 and a multiple of 64")
	// ErrInvalidSeed is returned when seed is less than -1
	ErrInvalidSeed = errors.New("seed must be >= -1 (use -1 for random)")
	// ErrInvalidLLMSeed is returned when llm-seed is negative
	ErrInvalidLLMSeed = errors.New("llm-seed must be >= 0")
	// ErrInvalidLogLevel is returned when log level is not recognized
	ErrInvalidLogLevel = errors.New("log-level must be one of: debug, info, warn, error")
	// ErrShowHelp is returned when --help flag is requested
	ErrShowHelp = errors.New("help requested")
	// ErrShowVersion is returned when --version flag is requested
	ErrShowVersion = errors.New("version requested")
	// ErrInvalidPath is returned when agent prompt path is invalid
	ErrInvalidPath = errors.New("agent prompt path must be relative, not absolute")
)

// Config holds all configuration values for the weave application.
// Values are populated from CLI flags with defaults applied.
type Config struct {
	// Server configuration
	Port int

	// Image generation parameters
	Steps  int
	CFG    float64
	Width  int
	Height int
	Seed   int64

	// LLM configuration
	LLMSeed     int64
	OllamaURL   string
	OllamaModel string

	// Logging configuration
	LogLevel string

	// Agent configuration
	AgentPromptPath string

	// Internal flags
	showHelp    bool
	showVersion bool
}

// Parse parses CLI flags into a Config struct.
// It returns the parsed Config or an error if validation fails.
// If --help or --version is requested, it prints the output and exits.
func Parse(args []string, output io.Writer) (*Config, error) {
	c := &Config{}

	fs := flag.NewFlagSet("weave", flag.ContinueOnError)
	fs.SetOutput(output)

	// Server flags
	fs.IntVar(&c.Port, "port", defaultPort, "HTTP server port")

	// Image generation flags
	fs.IntVar(&c.Steps, "steps", defaultSteps, "Number of inference steps")
	fs.Float64Var(&c.CFG, "cfg", defaultCFG, "CFG (Classifier Free Guidance) scale")
	fs.IntVar(&c.Width, "width", defaultWidth, "Image width in pixels")
	fs.IntVar(&c.Height, "height", defaultHeight, "Image height in pixels")
	fs.Int64Var(&c.Seed, "seed", defaultSeed, "Image generation seed (-1 = random)")

	// LLM flags
	fs.Int64Var(&c.LLMSeed, "llm-seed", defaultLLMSeed, "LLM seed for deterministic responses (0 = random)")
	fs.StringVar(&c.OllamaURL, "ollama-url", defaultOllamaURL, "Ollama API endpoint URL")
	fs.StringVar(&c.OllamaModel, "ollama-model", defaultOllamaModel, "Ollama model name")

	// Logging flags
	fs.StringVar(&c.LogLevel, "log-level", defaultLogLevel, "Log level (debug, info, warn, error)")

	// Agent flags
	fs.StringVar(&c.AgentPromptPath, "agent-prompt", DefaultAgentPrompt, "Path to agent prompt file")

	// Special flags
	fs.BoolVar(&c.showHelp, "help", false, "Show help message")
	fs.BoolVar(&c.showVersion, "version", false, "Show version information")

	// Parse flags
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Handle --help
	if c.showHelp {
		printHelp(output)
		return nil, ErrShowHelp
	}

	// Handle --version
	if c.showVersion {
		printVersion(output)
		return nil, ErrShowVersion
	}

	// Validate configuration
	if err := c.validate(); err != nil {
		return nil, err
	}

	return c, nil
}

// validate checks that all configuration values are within valid ranges
func (c *Config) validate() error {
	// Validate port
	if c.Port < minPort || c.Port > maxPort {
		return ErrInvalidPort
	}

	// Validate steps
	if c.Steps < minSteps || c.Steps > maxSteps {
		return ErrInvalidSteps
	}

	// Validate CFG
	if c.CFG < minCFG || c.CFG > maxCFG {
		return ErrInvalidCFG
	}

	// Validate width
	if c.Width < minWidth || c.Width > maxWidth || c.Width%widthStep != 0 {
		return ErrInvalidWidth
	}

	// Validate height
	if c.Height < minHeight || c.Height > maxHeight || c.Height%heightStep != 0 {
		return ErrInvalidHeight
	}

	// Validate seed (-1 means random, any value >= -1 is valid)
	if c.Seed < minSeed {
		return ErrInvalidSeed
	}

	// Validate LLM seed
	if c.LLMSeed < minLLMSeed {
		return ErrInvalidLLMSeed
	}

	// Validate log level
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// Valid
	default:
		return ErrInvalidLogLevel
	}

	return nil
}

// printHelp prints usage information
func printHelp(w io.Writer) {
	fmt.Fprintf(w, `weave - High-performance image generation system

USAGE:
    weave [FLAGS]

FLAGS:
    --port <PORT>              HTTP server port (default: %d)
    --steps <STEPS>            Number of inference steps (default: %d)
    --cfg <CFG>                CFG scale (default: %.1f)
    --width <WIDTH>            Image width in pixels (default: %d)
    --height <HEIGHT>          Image height in pixels (default: %d)
    --seed <SEED>              Image generation seed, -1 = random (default: %d)
    --llm-seed <SEED>          LLM seed for deterministic responses, 0 = random (default: %d)
    --ollama-url <URL>         Ollama API endpoint (default: %s)
    --ollama-model <MODEL>     Ollama model name (default: %s)
    --log-level <LEVEL>        Log level: debug, info, warn, error (default: %s)
    --agent-prompt <PATH>      Path to agent prompt file (default: %s)
    --help                     Show this help message
    --version                  Show version information

EXAMPLES:
    # Start with defaults
    weave

    # Use custom port
    weave --port 3000

    # Use deterministic generation
    weave --seed 42 --llm-seed 123

    # Use different ollama model
    weave --ollama-model llama3.2:3b

REQUIREMENTS:
    - ollama must be running (default: http://localhost:11434)
    - weave-compute process must be running

For more information, see docs/DEVELOPMENT.md
`,
		defaultPort, defaultSteps, defaultCFG, defaultWidth, defaultHeight,
		defaultSeed, defaultLLMSeed, defaultOllamaURL, defaultOllamaModel,
		defaultLogLevel, DefaultAgentPrompt)
}

// printVersion prints version information
func printVersion(w io.Writer) {
	fmt.Fprintf(w, "weave %s\n", Version)
}

// LoadAgentPrompt loads the agent prompt from a file.
// Returns the file contents or an error if the file doesn't exist or is unreadable.
// Only accepts relative paths that stay within the working directory to prevent
// reading arbitrary system files.
func LoadAgentPrompt(path string) (string, error) {
	// Clean the path to resolve any .., ., or redundant separators
	cleanPath := filepath.Clean(path)

	// Reject absolute paths - only accept relative paths
	if filepath.IsAbs(cleanPath) {
		return "", ErrInvalidPath
	}

	// Reject paths that try to escape the working directory via ..
	// After Clean(), paths like "../foo" or "foo/../../../bar" become "../foo" or "../../bar"
	if strings.HasPrefix(cleanPath, "..") {
		return "", ErrInvalidPath
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to load agent prompt from %s: %w", cleanPath, err)
	}
	return string(data), nil
}
