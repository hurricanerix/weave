package resources

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbedded(t *testing.T) {
	embedded, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded() error = %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"template", "web/templates/index.html"},
		{"font", "web/static/fonts/MarckScript-Regular.ttf"},
		{"image", "web/static/images/loading.webp"},
		{"ara prompt", "config/agents/ara.md"},
		{"ara tools prompt", "config/agents/ara_tools.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := fs.Stat(embedded, tt.path)
			if err != nil {
				t.Fatalf("file %q not found: %v", tt.path, err)
			}
			if info.Size() == 0 {
				t.Errorf("file %q is empty", tt.path)
			}
		})
	}
}

func TestEmbedded_AgentPromptContent(t *testing.T) {
	embedded, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded() error = %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantContain string
	}{
		{"ara", "config/agents/ara.md", "Ara"},
		{"ara_tools", "config/agents/ara_tools.md", "update_generation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := fs.ReadFile(embedded, tt.path)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", tt.path, err)
			}
			content := string(data)
			if !strings.Contains(content, tt.wantContain) {
				t.Errorf("content should contain %q", tt.wantContain)
			}
		})
	}
}
