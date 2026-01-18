package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_Defaults(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPort int
		wantURL  string
	}{
		{
			name:     "no arguments uses defaults",
			args:     []string{},
			wantPort: defaultPort,
			wantURL:  defaultOllamaURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			cfg, err := Parse(tt.args, output)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}

			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
			if cfg.OllamaURL != tt.wantURL {
				t.Errorf("OllamaURL = %s, want %s", cfg.OllamaURL, tt.wantURL)
			}
			if cfg.Steps != defaultSteps {
				t.Errorf("Steps = %d, want %d", cfg.Steps, defaultSteps)
			}
			if cfg.CFG != defaultCFG {
				t.Errorf("CFG = %f, want %f", cfg.CFG, defaultCFG)
			}
			if cfg.Width != defaultWidth {
				t.Errorf("Width = %d, want %d", cfg.Width, defaultWidth)
			}
			if cfg.Height != defaultHeight {
				t.Errorf("Height = %d, want %d", cfg.Height, defaultHeight)
			}
			if cfg.Seed != defaultSeed {
				t.Errorf("Seed = %d, want %d", cfg.Seed, defaultSeed)
			}
			if cfg.LLMSeed != defaultLLMSeed {
				t.Errorf("LLMSeed = %d, want %d", cfg.LLMSeed, defaultLLMSeed)
			}
			if cfg.OllamaModel != defaultOllamaModel {
				t.Errorf("OllamaModel = %s, want %s", cfg.OllamaModel, defaultOllamaModel)
			}
			if cfg.LogLevel != defaultLogLevel {
				t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, defaultLogLevel)
			}
		})
	}
}

func TestParse_CustomFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantCfg *Config
	}{
		{
			name: "custom port",
			args: []string{"--port", "3000"},
			wantCfg: &Config{
				Port:        3000,
				Steps:       defaultSteps,
				CFG:         defaultCFG,
				Width:       defaultWidth,
				Height:      defaultHeight,
				Seed:        defaultSeed,
				LLMSeed:     defaultLLMSeed,
				OllamaURL:   defaultOllamaURL,
				OllamaModel: defaultOllamaModel,
				LogLevel:    defaultLogLevel,
			},
		},
		{
			name: "custom steps and cfg",
			args: []string{"--steps", "10", "--cfg", "2.5"},
			wantCfg: &Config{
				Port:        defaultPort,
				Steps:       10,
				CFG:         2.5,
				Width:       defaultWidth,
				Height:      defaultHeight,
				Seed:        defaultSeed,
				LLMSeed:     defaultLLMSeed,
				OllamaURL:   defaultOllamaURL,
				OllamaModel: defaultOllamaModel,
				LogLevel:    defaultLogLevel,
			},
		},
		{
			name: "custom dimensions",
			args: []string{"--width", "512", "--height", "512"},
			wantCfg: &Config{
				Port:        defaultPort,
				Steps:       defaultSteps,
				CFG:         defaultCFG,
				Width:       512,
				Height:      512,
				Seed:        defaultSeed,
				LLMSeed:     defaultLLMSeed,
				OllamaURL:   defaultOllamaURL,
				OllamaModel: defaultOllamaModel,
				LogLevel:    defaultLogLevel,
			},
		},
		{
			name: "custom seeds",
			args: []string{"--seed", "42", "--llm-seed", "123"},
			wantCfg: &Config{
				Port:        defaultPort,
				Steps:       defaultSteps,
				CFG:         defaultCFG,
				Width:       defaultWidth,
				Height:      defaultHeight,
				Seed:        42,
				LLMSeed:     123,
				OllamaURL:   defaultOllamaURL,
				OllamaModel: defaultOllamaModel,
				LogLevel:    defaultLogLevel,
			},
		},
		{
			name: "seed -1 for random",
			args: []string{"--seed", "-1"},
			wantCfg: &Config{
				Port:        defaultPort,
				Steps:       defaultSteps,
				CFG:         defaultCFG,
				Width:       defaultWidth,
				Height:      defaultHeight,
				Seed:        -1,
				LLMSeed:     defaultLLMSeed,
				OllamaURL:   defaultOllamaURL,
				OllamaModel: defaultOllamaModel,
				LogLevel:    defaultLogLevel,
			},
		},
		{
			name: "custom ollama settings",
			args: []string{"--ollama-url", "http://localhost:12345", "--ollama-model", "llama3.2:3b"},
			wantCfg: &Config{
				Port:        defaultPort,
				Steps:       defaultSteps,
				CFG:         defaultCFG,
				Width:       defaultWidth,
				Height:      defaultHeight,
				Seed:        defaultSeed,
				LLMSeed:     defaultLLMSeed,
				OllamaURL:   "http://localhost:12345",
				OllamaModel: "llama3.2:3b",
				LogLevel:    defaultLogLevel,
			},
		},
		{
			name: "custom log level",
			args: []string{"--log-level", "debug"},
			wantCfg: &Config{
				Port:        defaultPort,
				Steps:       defaultSteps,
				CFG:         defaultCFG,
				Width:       defaultWidth,
				Height:      defaultHeight,
				Seed:        defaultSeed,
				LLMSeed:     defaultLLMSeed,
				OllamaURL:   defaultOllamaURL,
				OllamaModel: defaultOllamaModel,
				LogLevel:    "debug",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			cfg, err := Parse(tt.args, output)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}

			if cfg.Port != tt.wantCfg.Port {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantCfg.Port)
			}
			if cfg.Steps != tt.wantCfg.Steps {
				t.Errorf("Steps = %d, want %d", cfg.Steps, tt.wantCfg.Steps)
			}
			if cfg.CFG != tt.wantCfg.CFG {
				t.Errorf("CFG = %f, want %f", cfg.CFG, tt.wantCfg.CFG)
			}
			if cfg.Width != tt.wantCfg.Width {
				t.Errorf("Width = %d, want %d", cfg.Width, tt.wantCfg.Width)
			}
			if cfg.Height != tt.wantCfg.Height {
				t.Errorf("Height = %d, want %d", cfg.Height, tt.wantCfg.Height)
			}
			if cfg.Seed != tt.wantCfg.Seed {
				t.Errorf("Seed = %d, want %d", cfg.Seed, tt.wantCfg.Seed)
			}
			if cfg.LLMSeed != tt.wantCfg.LLMSeed {
				t.Errorf("LLMSeed = %d, want %d", cfg.LLMSeed, tt.wantCfg.LLMSeed)
			}
			if cfg.OllamaURL != tt.wantCfg.OllamaURL {
				t.Errorf("OllamaURL = %s, want %s", cfg.OllamaURL, tt.wantCfg.OllamaURL)
			}
			if cfg.OllamaModel != tt.wantCfg.OllamaModel {
				t.Errorf("OllamaModel = %s, want %s", cfg.OllamaModel, tt.wantCfg.OllamaModel)
			}
			if cfg.LogLevel != tt.wantCfg.LogLevel {
				t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, tt.wantCfg.LogLevel)
			}
		})
	}
}

func TestParse_Validation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{
			name:    "port too low",
			args:    []string{"--port", "100"},
			wantErr: ErrInvalidPort,
		},
		{
			name:    "port too high",
			args:    []string{"--port", "70000"},
			wantErr: ErrInvalidPort,
		},
		{
			name:    "steps too low",
			args:    []string{"--steps", "0"},
			wantErr: ErrInvalidSteps,
		},
		{
			name:    "steps too high",
			args:    []string{"--steps", "150"},
			wantErr: ErrInvalidSteps,
		},
		{
			name:    "cfg too low",
			args:    []string{"--cfg", "-1.0"},
			wantErr: ErrInvalidCFG,
		},
		{
			name:    "cfg too high",
			args:    []string{"--cfg", "25.0"},
			wantErr: ErrInvalidCFG,
		},
		{
			name:    "width too low",
			args:    []string{"--width", "32"},
			wantErr: ErrInvalidWidth,
		},
		{
			name:    "width too high",
			args:    []string{"--width", "4096"},
			wantErr: ErrInvalidWidth,
		},
		{
			name:    "width not multiple of 64",
			args:    []string{"--width", "100"},
			wantErr: ErrInvalidWidth,
		},
		{
			name:    "height too low",
			args:    []string{"--height", "32"},
			wantErr: ErrInvalidHeight,
		},
		{
			name:    "height too high",
			args:    []string{"--height", "4096"},
			wantErr: ErrInvalidHeight,
		},
		{
			name:    "height not multiple of 64",
			args:    []string{"--height", "100"},
			wantErr: ErrInvalidHeight,
		},
		{
			name:    "seed too low",
			args:    []string{"--seed", "-2"},
			wantErr: ErrInvalidSeed,
		},
		{
			name:    "negative llm seed",
			args:    []string{"--llm-seed", "-1"},
			wantErr: ErrInvalidLLMSeed,
		},
		{
			name:    "invalid log level",
			args:    []string{"--log-level", "trace"},
			wantErr: ErrInvalidLogLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			_, err := Parse(tt.args, output)
			if err != tt.wantErr {
				t.Errorf("Parse() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPrintHelp(t *testing.T) {
	output := &bytes.Buffer{}
	printHelp(output)

	helpText := output.String()

	// Check that help contains key information
	expectedStrings := []string{
		"weave",
		"USAGE:",
		"FLAGS:",
		"--port",
		"--steps",
		"--cfg",
		"--width",
		"--height",
		"--seed",
		"--llm-seed",
		"--ollama-url",
		"--ollama-model",
		"--log-level",
		"--agent-prompt",
		"--help",
		"--version",
		"EXAMPLES:",
		"REQUIREMENTS:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(helpText, expected) {
			t.Errorf("Help text missing expected string: %q", expected)
		}
	}
}

func TestPrintVersion(t *testing.T) {
	output := &bytes.Buffer{}
	printVersion(output)

	versionText := output.String()
	expected := "weave " + Version

	if !strings.Contains(versionText, expected) {
		t.Errorf("Version text = %q, want to contain %q", versionText, expected)
	}
}

func TestValidate_AllLevels(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		wantErr  bool
	}{
		{"debug level valid", "debug", false},
		{"info level valid", "info", false},
		{"warn level valid", "warn", false},
		{"error level valid", "error", false},
		{"invalid level", "invalid", true},
		{"empty level", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Port:        defaultPort,
				Steps:       defaultSteps,
				CFG:         defaultCFG,
				Width:       defaultWidth,
				Height:      defaultHeight,
				Seed:        defaultSeed,
				LLMSeed:     defaultLLMSeed,
				OllamaURL:   defaultOllamaURL,
				OllamaModel: defaultOllamaModel,
				LogLevel:    tt.logLevel,
			}

			err := c.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParse_HelpFlag(t *testing.T) {
	output := &bytes.Buffer{}
	cfg, err := Parse([]string{"--help"}, output)

	if cfg != nil {
		t.Errorf("Parse() with --help returned config, want nil")
	}
	if !errors.Is(err, ErrShowHelp) {
		t.Errorf("Parse() with --help error = %v, want ErrShowHelp", err)
	}
	if output.Len() == 0 {
		t.Error("Parse() with --help did not write help text to output")
	}
}

func TestParse_VersionFlag(t *testing.T) {
	output := &bytes.Buffer{}
	cfg, err := Parse([]string{"--version"}, output)

	if cfg != nil {
		t.Errorf("Parse() with --version returned config, want nil")
	}
	if !errors.Is(err, ErrShowVersion) {
		t.Errorf("Parse() with --version error = %v, want ErrShowVersion", err)
	}
	if output.Len() == 0 {
		t.Error("Parse() with --version did not write version text to output")
	}
}

func TestLoadAgentPrompt_ValidFile(t *testing.T) {
	// Create tmp directory within current test directory (not using .. traversal)
	tmpDir := "testdata_tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file
	testPath := filepath.Join(tmpDir, "test-prompt.md")
	testContent := "This is a test agent prompt.\nIt has multiple lines."
	if err := os.WriteFile(testPath, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Load the file
	content, err := LoadAgentPrompt(testPath)
	if err != nil {
		t.Fatalf("LoadAgentPrompt() error = %v, want nil", err)
	}

	if content != testContent {
		t.Errorf("LoadAgentPrompt() content = %q, want %q", content, testContent)
	}
}

func TestLoadAgentPrompt_NonExistentFile(t *testing.T) {
	// Use a path that doesn't exist (within current directory, no traversal)
	testPath := filepath.Join("nonexistent_dir", "prompt.md")

	content, err := LoadAgentPrompt(testPath)
	if err == nil {
		t.Error("LoadAgentPrompt() error = nil, want error for non-existent file")
	}

	if content != "" {
		t.Errorf("LoadAgentPrompt() content = %q, want empty string on error", content)
	}

	// Error message should include the path
	if !strings.Contains(err.Error(), testPath) {
		t.Errorf("error message should contain path, got: %v", err)
	}
}

func TestLoadAgentPrompt_UnreadableFile(t *testing.T) {
	// Create tmp directory within current test directory
	tmpDir := "testdata_unreadable"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file with no read permissions
	testPath := filepath.Join(tmpDir, "unreadable.md")
	if err := os.WriteFile(testPath, []byte("test"), 0000); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	defer os.Chmod(testPath, 0644) // Restore permissions for cleanup

	content, err := LoadAgentPrompt(testPath)
	if err == nil {
		t.Error("LoadAgentPrompt() error = nil, want error for unreadable file")
	}

	if content != "" {
		t.Errorf("LoadAgentPrompt() content = %q, want empty string on error", content)
	}
}

func TestParse_AgentPromptFlag(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantPromptPath string
	}{
		{
			name:           "default agent prompt path",
			args:           []string{},
			wantPromptPath: DefaultAgentPrompt,
		},
		{
			name:           "custom agent prompt path",
			args:           []string{"--agent-prompt", "custom/path/prompt.md"},
			wantPromptPath: "custom/path/prompt.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			cfg, err := Parse(tt.args, output)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}

			if cfg.AgentPromptPath != tt.wantPromptPath {
				t.Errorf("AgentPromptPath = %s, want %s", cfg.AgentPromptPath, tt.wantPromptPath)
			}
		})
	}
}

func TestLoadAgentPrompt_AbsolutePath(t *testing.T) {
	// Try to read an absolute path (should be rejected)
	content, err := LoadAgentPrompt("/etc/passwd")
	if err != ErrInvalidPath {
		t.Errorf("LoadAgentPrompt() error = %v, want ErrInvalidPath", err)
	}

	if content != "" {
		t.Errorf("LoadAgentPrompt() content = %q, want empty string on error", content)
	}
}

func TestLoadAgentPrompt_PathTraversal(t *testing.T) {
	// Test that path traversal attacks are rejected
	tests := []struct {
		name string
		path string
	}{
		{"simple traversal", "../etc/passwd"},
		{"double traversal", "../../etc/passwd"},
		{"deep traversal", "../../../../../../../etc/passwd"},
		{"embedded traversal", "foo/../../../etc/passwd"},
		{"dot-dot-slash", ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := LoadAgentPrompt(tt.path)
			if err != ErrInvalidPath {
				t.Errorf("LoadAgentPrompt(%q) error = %v, want ErrInvalidPath", tt.path, err)
			}

			if content != "" {
				t.Errorf("LoadAgentPrompt(%q) content = %q, want empty string on error", tt.path, content)
			}
		})
	}
}
