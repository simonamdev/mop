package provider

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ProxmoxProvider is a WakeupProvider that calls Proxmox API to start a VM/CT.
type ProxmoxProvider struct {
	APIURL   string
	Node     string
	VMID     string
	Token    string
	Type     string
	Insecure bool
}

type ProxmoxStatusResponse struct {
	Data struct {
		Status string `json:"status"`
	} `json:"data"`
}

func (p *ProxmoxProvider) Wake() error {
	// Construct URL base
	baseURL := strings.TrimRight(p.APIURL, "/")

	// Auto-upgrade http to https
	if strings.HasPrefix(baseURL, "http://") {
		log.Printf("Warning: Proxmox API URL uses http. Upgrading to https to avoid redirect issues.")
		baseURL = strings.Replace(baseURL, "http://", "https://", 1)
	}

	resourceType := p.Type
	if resourceType == "" {
		resourceType = "qemu"
	}

	// Helper to make requests
	makeRequest := func(method, endpoint string) (*http.Response, error) {
		url := fmt.Sprintf("%s/nodes/%s/%s/%s/%s", baseURL, p.Node, resourceType, p.VMID, endpoint)
		log.Printf("Proxmox Request: %s %s", method, url)

		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s", p.Token))

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: p.Insecure},
		}

		client := &http.Client{
			Transport: tr,
			Timeout:   10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				if len(via) > 0 {
					lastReq := via[len(via)-1]
					if auth := lastReq.Header.Get("Authorization"); auth != "" {
						req.Header.Set("Authorization", auth)
					}
				}
				return nil
			},
		}

		return client.Do(req)
	}

	// 1. Check Status
	resp, err := makeRequest("GET", "status/current")
	if err != nil {
		return fmt.Errorf("failed to check proxmox status: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read status body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("proxmox status check returned error %d: %s", resp.StatusCode, string(body))
	}

	var statusResp ProxmoxStatusResponse
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return fmt.Errorf("failed to parse status json: %w", err)
	}

	log.Printf("Current Proxmox Status: %s", statusResp.Data.Status)

	if statusResp.Data.Status == "running" {
		log.Println("Container/VM is already running. Skipping start command.")
		return nil
	}

	// 2. Start Request if not running
	// Re-check token format warning (optional, moved from original code)
	if !strings.Contains(p.Token, "!") || !strings.Contains(p.Token, "=") {
		log.Printf("Warning: Proxmox Token format looks incorrect. Expected 'USER@REALM!TOKENID=UUID'. Check your configuration.")
	}

	respStart, err := makeRequest("POST", "status/start")
	if err != nil {
		return fmt.Errorf("proxmox start api call failed: %w", err)
	}
	defer respStart.Body.Close()

	bodyStart, _ := io.ReadAll(respStart.Body)

	if respStart.StatusCode != http.StatusOK {
		return fmt.Errorf("proxmox start api returned error %d: %s", respStart.StatusCode, string(bodyStart))
	}

	log.Printf("Proxmox Wake success: %s", string(bodyStart))
	return nil
}
