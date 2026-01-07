package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxmoxProvider(t *testing.T) {
	tests := []struct {
		name           string
		node           string
		vmid           string
		proxType       string
		token          string
		responseStatus int
		expectError    bool
	}{
		{
			name:           "Success VM",
			node:           "pve1",
			vmid:           "100",
			proxType:       "qemu",
			token:          "user@pam!token=secret",
			responseStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Success LXC",
			node:           "pve1",
			vmid:           "101",
			proxType:       "lxc",
			token:          "user@pam!token=secret",
			responseStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "API Error",
			node:           "pve1",
			vmid:           "100",
			proxType:       "qemu",
			token:          "user@pam!token=secret",
			responseStatus: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify URL
				// Note: logic in proxmox.go constructs URL with /nodes/...
				// The test mock server URL will be the base.
				// ProxmoxProvider logic: baseURL := strings.TrimRight(p.APIURL, "/")
				// url := fmt.Sprintf("%s/nodes/%s/%s/%s/%s", baseURL, p.Node, resourceType, p.VMID, endpoint)

				// We need to match what ProxmoxProvider expects.
				// APIURL setup below is server.URL + "/api2/json"

				// But wait, the previous test code was:
				// expectedPath := fmt.Sprintf("/api2/json/nodes/%s/%s/%s/status/start", tt.node, tt.proxType, tt.vmid)
				// This implies the provider was appending /nodes/... to APIURL.

				// Let's check ProxmoxProvider implementation in provider/proxmox.go
				/*
					func (p *ProxmoxProvider) Wake() error {
						baseURL := strings.TrimRight(p.APIURL, "/")
						// ...
						url := fmt.Sprintf("%s/nodes/%s/%s/%s/%s", baseURL, p.Node, resourceType, p.VMID, endpoint)
				*/
				// Yes, so if APIURL is server.URL + "/api2/json", then path will be /api2/json/nodes/...

				// However, verify if GET status/current is called first.
				// Implementation:
				// // 1. Check Status
				// resp, err := makeRequest("GET", "status/current")

				// The test mock in main_test.go ONLY handled POST status/start?
				/*
					if r.Method != "POST" {
						t.Errorf("Expected POST method, got %s", r.Method)
					}
				*/
				// Wait, the original `main.go` implementation of ProxmoxProvider ALSO did a status check first?
				// Let's check original `main.go`.

				/*
					// 1. Check Status
					resp, err := makeRequest("GET", "status/current")
					...
					// 2. Start Request if not running
					respStart, err := makeRequest("POST", "status/start")
				*/

				// So the original test was probably FLAKY or only testing the start part if the previous part succeeded?
				// BUT the mock server handles ALL requests.
				// If the provider makes a GET request first, the mock server sees it.
				// The mock server code:
				/*
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// Verify URL
						expectedPath := fmt.Sprintf("/api2/json/nodes/%s/%s/%s/status/start", tt.node, tt.proxType, tt.vmid)
						if r.URL.Path != expectedPath {
							t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
						}
						// Verify Method
						if r.Method != "POST" {
							t.Errorf("Expected POST method, got %s", r.Method)
						}
						...
					}))
				*/

				// If the provider makes a GET request to `status/current`, the path will be different and method will be GET.
				// So the test would FAIL if it makes a GET request.
				// Did the original `main.go` NOT make a GET request?
				// I checked Step 4 (original main.go). Line 276: `resp, err := makeRequest("GET", "status/current")`.
				// SO THE TEST WAS LIKELY FAILING OR I MISREAD SOMETHING.
				// Or maybe the test logic was different in `main_test.go`.
				// Let's look at `main_test.go` again (Step 38).
				// It sets up the server.

				// Maybe `ProxmoxProvider` in `main.go` was RECENTLY changed to add status check and test wasn't updated?
				// The user asked to "Reorganise main.go".
				// If I run the tests as is, they might fail.

				// I should probably fix the test to handle the status check as well.
				// The provider:
				// 1. GET status/current.
				// 2. If status is NOT running, POST status/start.

				// I will update the mock handler to handle both.

				pathStatus := fmt.Sprintf("/api2/json/nodes/%s/%s/%s/status/current", tt.node, tt.proxType, tt.vmid)
				pathStart := fmt.Sprintf("/api2/json/nodes/%s/%s/%s/status/start", tt.node, tt.proxType, tt.vmid)

				if r.URL.Path == pathStatus {
					if r.Method != "GET" {
						t.Errorf("Expected GET for status, got %s", r.Method)
					}
					// Return stopped status so it proceeds to start
					w.Write([]byte(`{"data":{"status":"stopped"}}`))
					return
				}

				if r.URL.Path == pathStart {
					if r.Method != "POST" {
						t.Errorf("Expected POST for start, got %s", r.Method)
					}
					// Verify Header
					expectedAuth := fmt.Sprintf("PVEAPIToken=%s", tt.token)
					if r.Header.Get("Authorization") != expectedAuth {
						t.Errorf("Expected Authorization %s, got %s", expectedAuth, r.Header.Get("Authorization"))
					}
					w.WriteHeader(tt.responseStatus)
					return
				}

				t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
			}))
			defer server.Close()

			provider := &ProxmoxProvider{
				APIURL:   server.URL + "/api2/json",
				Node:     tt.node,
				VMID:     tt.vmid,
				Token:    tt.token,
				Type:     tt.proxType,
				Insecure: true,
			}

			err := provider.Wake()
			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
