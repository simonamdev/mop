package main

import (
	"fmt"
	"io"
	"log"
	"mop/provider"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config holds the application configuration, loaded from environment variables.
type Config struct {
	ProxyHost         string
	ProxyPort         int
	TargetHost        string
	TargetPort        int
	TargetMAC         string
	TargetBroadcastIP string
	ProxmoxAPIURL     string
	ProxmoxNode       string
	ProxmoxVMID       string
	ProxmoxToken      string
	ProxmoxType       string
	ProxmoxInsecure   bool
	WakeupMethod      string
	ConnectionRetries int
	RetryDelaySeconds time.Duration
}

// loadConfig loads configuration from environment variables with defaults.
func loadConfig() (*Config, error) {
	// Helper to get an environment variable or return a default value.
	getEnv := func(key, fallback string) string {
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		return fallback
	}

	// Helper to get an integer environment variable.
	getEnvAsInt := func(key string, fallback int) (int, error) {
		strValue := getEnv(key, "")
		if strValue == "" {
			return fallback, nil
		}
		val, err := strconv.Atoi(strValue)
		if err != nil {
			return 0, fmt.Errorf("invalid value for %s: %v", key, err)
		}
		return val, nil
	}

	getEnvAsBool := func(key string, fallback bool) bool {
		strValue := getEnv(key, "")
		if strValue == "" {
			return fallback
		}
		val, err := strconv.ParseBool(strValue)
		if err != nil {
			return fallback
		}
		return val
	}

	targetHost := os.Getenv("TARGET_HOST")
	if targetHost == "" {
		return nil, fmt.Errorf("TARGET_HOST environment variable is required")
	}

	wakeupMethod := strings.ToLower(getEnv("WAKEUP_METHOD", "wol"))
	targetMAC := os.Getenv("TARGET_MAC")

	// Validation depends on wakeup method
	switch wakeupMethod {
	case "wol":
		if targetMAC == "" {
			return nil, fmt.Errorf("TARGET_MAC environment variable is required when WAKEUP_METHOD is 'wol'")
		}
	case "proxmox":
		if getEnv("PROXMOX_API_URL", "") == "" {
			return nil, fmt.Errorf("PROXMOX_API_URL is required when WAKEUP_METHOD is 'proxmox'")
		}
		if getEnv("PROXMOX_NODE", "") == "" {
			return nil, fmt.Errorf("PROXMOX_NODE is required when WAKEUP_METHOD is 'proxmox'")
		}
		if getEnv("PROXMOX_VMID", "") == "" {
			return nil, fmt.Errorf("PROXMOX_VMID is required when WAKEUP_METHOD is 'proxmox'")
		}
		if getEnv("PROXMOX_TOKEN", "") == "" {
			return nil, fmt.Errorf("PROXMOX_TOKEN is required when WAKEUP_METHOD is 'proxmox'")
		}
	}

	proxyPort, err := getEnvAsInt("PROXY_PORT", 2222)
	if err != nil {
		return nil, err
	}

	targetPort, err := getEnvAsInt("TARGET_PORT", 22)
	if err != nil {
		return nil, err
	}

	connectionRetries, err := getEnvAsInt("CONNECTION_RETRIES", 15)
	if err != nil {
		return nil, err
	}

	retryDelay, err := getEnvAsInt("RETRY_DELAY_SECONDS", 5)
	if err != nil {
		return nil, err
	}

	return &Config{
		ProxyHost:         getEnv("PROXY_HOST", "0.0.0.0"),
		ProxyPort:         proxyPort,
		TargetHost:        targetHost,
		TargetPort:        targetPort,
		TargetMAC:         targetMAC,
		TargetBroadcastIP: getEnv("TARGET_BROADCAST_IP", "255.255.255.255"),
		ProxmoxAPIURL:     getEnv("PROXMOX_API_URL", ""),
		ProxmoxNode:       getEnv("PROXMOX_NODE", ""),
		ProxmoxVMID:       getEnv("PROXMOX_VMID", ""),
		ProxmoxToken:      getEnv("PROXMOX_TOKEN", ""),
		ProxmoxType:       getEnv("PROXMOX_TYPE", "qemu"), // default to qemu (VM), can be lxc
		ProxmoxInsecure:   getEnvAsBool("PROXMOX_INSECURE", false),
		WakeupMethod:      wakeupMethod,
		ConnectionRetries: connectionRetries,
		RetryDelaySeconds: time.Duration(retryDelay) * time.Second,
	}, nil
}

// proxyTraffic bi-directionally copies data between two connections.
func proxyTraffic(client, target net.Conn) {
	log.Printf("Starting traffic proxy between %s and %s", client.RemoteAddr(), target.RemoteAddr())
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer target.Close() // Ensure the other connection is closed on exit
		io.Copy(target, client)
	}()

	go func() {
		defer wg.Done()
		defer client.Close() // Ensure the other connection is closed on exit
		io.Copy(client, target)
	}()

	wg.Wait()
	log.Printf("Proxy connection between %s and %s closed.", client.RemoteAddr(), target.RemoteAddr())
}

// handleClient manages an incoming client connection.
func handleClient(clientConn net.Conn, cfg *Config, wakeupProvider provider.WakeupProvider) {
	defer clientConn.Close()
	log.Printf("Accepted connection from %s", clientConn.RemoteAddr())

	// 1. Perform Wakeup
	if err := wakeupProvider.Wake(); err != nil {
		log.Printf("Error performing wakeup: %v", err)
		return
	}

	// 2. Wait and attempt to connect to the target SSH server
	var targetConn net.Conn
	var err error
	targetAddr := net.JoinHostPort(cfg.TargetHost, strconv.Itoa(cfg.TargetPort))

	log.Printf("Attempting to connect to target %s...", targetAddr)
	for i := 0; i < cfg.ConnectionRetries; i++ {
		targetConn, err = net.DialTimeout("tcp", targetAddr, cfg.RetryDelaySeconds)
		if err == nil {
			log.Printf("Successfully connected to target %s on attempt %d.", targetAddr, i+1)
			break
		}
		log.Printf("Attempt %d/%d failed to connect to target: %v. Retrying in %v...", i+1, cfg.ConnectionRetries, err, cfg.RetryDelaySeconds)
		time.Sleep(cfg.RetryDelaySeconds)
	}

	if targetConn == nil {
		log.Printf("Could not connect to target server after %d attempts. Closing client connection.", cfg.ConnectionRetries)
		return
	}
	defer targetConn.Close()

	// 3. Start proxying traffic
	proxyTraffic(clientConn, targetConn)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	listenAddr := net.JoinHostPort(cfg.ProxyHost, strconv.Itoa(cfg.ProxyPort))
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to start listener on %s: %v", listenAddr, err)
	}
	defer listener.Close()
	log.Printf("mop server listening on %s", listenAddr)

	var wakeupProvider provider.WakeupProvider
	switch cfg.WakeupMethod {
	case "wol":
		wakeupProvider = &provider.WOLProvider{
			TargetMAC:         cfg.TargetMAC,
			TargetBroadcastIP: cfg.TargetBroadcastIP,
		}
	case "proxmox":
		wakeupProvider = &provider.ProxmoxProvider{
			APIURL:   cfg.ProxmoxAPIURL,
			Node:     cfg.ProxmoxNode,
			VMID:     cfg.ProxmoxVMID,
			Token:    cfg.ProxmoxToken,
			Type:     cfg.ProxmoxType,
			Insecure: cfg.ProxmoxInsecure,
		}
	case "noop":
		wakeupProvider = &provider.NoopProvider{}
	default:
		log.Fatalf("Unknown wakeup method: %s", cfg.WakeupMethod)
	}
	log.Printf("Using wakeup provider: %T", wakeupProvider)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		// Handle each client connection in a new goroutine
		go handleClient(conn, cfg, wakeupProvider)
	}
}
