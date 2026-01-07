package main

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Save original env
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			pair := splitEnv(e)
			os.Setenv(pair[0], pair[1])
		}
	}()

	tests := []struct {
		name      string
		env       map[string]string
		expectErr bool
	}{
		{
			name: "Valid WOL Config",
			env: map[string]string{
				"TARGET_HOST":   "example.com",
				"TARGET_MAC":    "AA:BB:CC:DD:EE:FF",
				"WAKEUP_METHOD": "wol",
			},
			expectErr: false,
		},
		{
			name: "Missing Target Host",
			env: map[string]string{
				"TARGET_MAC": "AA:BB:CC:DD:EE:FF",
			},
			expectErr: true,
		},
		{
			name: "Missing MAC for WOL",
			env: map[string]string{
				"TARGET_HOST":   "example.com",
				"WAKEUP_METHOD": "wol",
			},
			expectErr: true,
		},
		{
			name: "Valid Noop Config (No MAC)",
			env: map[string]string{
				"TARGET_HOST":   "example.com",
				"WAKEUP_METHOD": "noop",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			_, err := loadConfig()
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func splitEnv(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s, ""}
}
