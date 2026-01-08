package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		expectErr   bool
	}{
		{
			name: "Valid WOL Config",
			fileContent: `
proxy_host = "0.0.0.0"
proxy_port = 2222
target_host = "example.com"
target_port = 22
method = "wol"
target_mac = "AA:BB:CC:DD:EE:FF"
`,
			expectErr: false,
		},
		{
			name: "Missing Target Host",
			fileContent: `
method = "wol"
target_mac = "AA:BB:CC:DD:EE:FF"
`,
			expectErr: true,
		},
		{
			name: "Missing MAC for WOL",
			fileContent: `
target_host = "example.com"
method = "wol"
`,
			expectErr: true,
		},
		{
			name: "Valid Noop Config",
			fileContent: `
target_host = "example.com"
method = "noop"
`,
			expectErr: false,
		},
		{
			name: "Invalid Config Syntax",
			fileContent: `
target_host = "example.com
method = "wol"
`, // Missing quote
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.toml")
			err := os.WriteFile(configFile, []byte(tt.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write temp config: %v", err)
			}

			_, err = loadConfig(configFile)
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
