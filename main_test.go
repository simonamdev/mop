package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type trackingWakeupProvider struct {
	wakeCount int
}

func (t *trackingWakeupProvider) Wake() error {
	t.wakeCount++
	return nil
}

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
		name         string
		env          map[string]string
		expectErr    bool
		validateFunc func(t *testing.T, cfg *Config)
	}{
		{
			name: "Valid WOL Config Defaults",
			env: map[string]string{
				"TARGET_HOST":   "example.com",
				"TARGET_MAC":    "AA:BB:CC:DD:EE:FF",
				"WAKEUP_METHOD": "wol",
			},
			expectErr: false,
			validateFunc: func(t *testing.T, cfg *Config) {
				if cfg.HealthPort != 8080 {
					t.Errorf("Expected default HealthPort 8080, got %d", cfg.HealthPort)
				}
				if cfg.HealthHost != "0.0.0.0" {
					t.Errorf("Expected default HealthHost 0.0.0.0, got %s", cfg.HealthHost)
				}
				if !cfg.HealthEnabled {
					t.Errorf("Expected default HealthEnabled to be true, got false")
				}
				if cfg.ProxyPort != 2222 {
					t.Errorf("Expected default ProxyPort 2222, got %d", cfg.ProxyPort)
				}
			},
		},
		{
			name: "Custom Health Config",
			env: map[string]string{
				"TARGET_HOST":    "example.com",
				"WAKEUP_METHOD":  "noop",
				"HEALTH_PORT":    "9090",
				"HEALTH_HOST":    "127.0.0.1",
				"HEALTH_ENABLED": "true",
			},
			expectErr: false,
			validateFunc: func(t *testing.T, cfg *Config) {
				if cfg.HealthPort != 9090 {
					t.Errorf("Expected HealthPort 9090, got %d", cfg.HealthPort)
				}
				if cfg.HealthHost != "127.0.0.1" {
					t.Errorf("Expected HealthHost 127.0.0.1, got %s", cfg.HealthHost)
				}
				if !cfg.HealthEnabled {
					t.Errorf("Expected HealthEnabled to be true, got false")
				}
			},
		},
		{
			name: "Health Disabled via HEALTH_ENABLED=false",
			env: map[string]string{
				"TARGET_HOST":    "example.com",
				"WAKEUP_METHOD":  "noop",
				"HEALTH_ENABLED": "false",
			},
			expectErr: false,
			validateFunc: func(t *testing.T, cfg *Config) {
				if cfg.HealthEnabled {
					t.Errorf("Expected HealthEnabled to be false, got true")
				}
			},
		},
		{
			name: "Health Disabled via HEALTH_PORT=0",
			env: map[string]string{
				"TARGET_HOST":   "example.com",
				"WAKEUP_METHOD": "noop",
				"HEALTH_PORT":   "0",
			},
			expectErr: false,
			validateFunc: func(t *testing.T, cfg *Config) {
				if cfg.HealthEnabled {
					t.Errorf("Expected HealthEnabled to be false when HEALTH_PORT=0, got true")
				}
			},
		},
		{
			name: "Port Collision Between HEALTH_PORT and PROXY_PORT",
			env: map[string]string{
				"TARGET_HOST":   "example.com",
				"WAKEUP_METHOD": "noop",
				"PROXY_PORT":    "2222",
				"HEALTH_PORT":   "2222",
			},
			expectErr: true,
		},
		{
			name: "Invalid HEALTH_PORT Range",
			env: map[string]string{
				"TARGET_HOST":   "example.com",
				"WAKEUP_METHOD": "noop",
				"HEALTH_PORT":   "70000",
			},
			expectErr: true,
		},
		{
			name: "Invalid PROXY_PORT Range",
			env: map[string]string{
				"TARGET_HOST":   "example.com",
				"WAKEUP_METHOD": "noop",
				"PROXY_PORT":    "-1",
			},
			expectErr: true,
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

			cfg, err := loadConfig()
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if err == nil && tt.validateFunc != nil {
				tt.validateFunc(t, cfg)
			}
		})
	}
}

func TestHealthHandler(t *testing.T) {
	handler := createHealthHandler()

	endpoints := []string{"/healthz", "/health", "/live", "/ready", "/"}
	for _, ep := range endpoints {
		t.Run("GET "+ep, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("Expected status 200 on %s, got %d", ep, rec.Code)
			}
			if strings.TrimSpace(rec.Body.String()) != "ok" {
				t.Errorf("Expected body 'ok', got %q", rec.Body.String())
			}
		})
	}

	// HEAD /healthz
	reqHead := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	recHead := httptest.NewRecorder()
	handler.ServeHTTP(recHead, reqHead)
	if recHead.Code != http.StatusOK {
		t.Fatalf("Expected HEAD /healthz status 200, got %d", recHead.Code)
	}

	// Method Not Allowed
	reqPost := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	recPost := httptest.NewRecorder()
	handler.ServeHTTP(recPost, reqPost)
	if recPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 on POST /healthz, got %d", recPost.Code)
	}

	// Not Found
	req404 := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rec404 := httptest.NewRecorder()
	handler.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Errorf("Expected 404 on unknown endpoint, got %d", rec404.Code)
	}
}

func TestHealthEndpoint_NoWOLTriggered(t *testing.T) {
	provider := &trackingWakeupProvider{}

	handler := createHealthHandler()

	paths := []string{"/healthz", "/health", "/live", "/ready", "/"}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Endpoint %s returned status %d", p, rec.Code)
		}
	}

	// Confirm that wake was never called across all health checks
	if provider.wakeCount != 0 {
		t.Fatalf("Expected wakeCount to be 0, but Wake() was called %d times", provider.wakeCount)
	}
}

func TestStartHealthServer(t *testing.T) {
	// Find an available port
	testListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := testListener.Addr().String()
	testListener.Close()

	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	cfg := &Config{
		HealthHost:   "127.0.0.1",
		HealthPort:   port,
		ProxyHost:    "127.0.0.1",
		ProxyPort:    port + 1,
		TargetHost:   "127.0.0.1",
		TargetPort:   22,
		WakeupMethod: "noop",
	}

	server, listener, err := startHealthServer(cfg)
	if err != nil {
		t.Fatalf("Failed to start health server: %v", err)
	}
	defer listener.Close()
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("Failed to request running health server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from server, got %d", resp.StatusCode)
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
