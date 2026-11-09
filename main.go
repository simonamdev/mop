package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"mop/provider"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Config holds the application configuration, loaded from environment variables.
type Config struct {
	ProxyHost         string
	ProxyPort         int
	HealthHost        string
	HealthPort        int
	HealthEnabled     bool
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

// createHealthHandler configures and returns an HTTP handler for Kubernetes liveness/readiness probes.
func createHealthHandler() http.Handler {
	mux := http.NewServeMux()

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}

	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/live", healthHandler)
	mux.HandleFunc("/ready", healthHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		healthHandler(w, r)
	})

	return mux
}

// startHealthServer starts the HTTP health server asynchronously.
func startHealthServer(cfg *Config) (*http.Server, net.Listener, error) {
	healthAddr := net.JoinHostPort(cfg.HealthHost, strconv.Itoa(cfg.HealthPort))
	healthListener, err := net.Listen("tcp", healthAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start health listener on %s: %w", healthAddr, err)
	}

	server := &http.Server{
		Handler:           createHealthHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Health server listening on %s", healthAddr)
		if err := server.Serve(healthListener); err != nil && err != http.ErrServerClosed {
			log.Printf("Health server error: %v", err)
		}
	}()

	return server, healthListener, nil
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
	if proxyPort <= 0 || proxyPort > 65535 {
		return nil, fmt.Errorf("PROXY_PORT must be between 1 and 65535")
	}

	targetPort, err := getEnvAsInt("TARGET_PORT", 22)
	if err != nil {
		return nil, err
	}
	if targetPort <= 0 || targetPort > 65535 {
		return nil, fmt.Errorf("TARGET_PORT must be between 1 and 65535")
	}

	healthEnabled := getEnvAsBool("HEALTH_ENABLED", getEnvAsBool("STATUS_ENABLED", true))
	healthPortStr := getEnv("HEALTH_PORT", getEnv("STATUS_PORT", "8080"))
	healthPort, err := strconv.Atoi(healthPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid value for HEALTH_PORT: %v", err)
	}
	if !healthEnabled || healthPort == 0 {
		healthEnabled = false
	} else if healthPort < 0 || healthPort > 65535 {
		return nil, fmt.Errorf("HEALTH_PORT must be between 1 and 65535")
	}
	if healthEnabled && healthPort == proxyPort {
		return nil, fmt.Errorf("HEALTH_PORT (%d) cannot be the same as PROXY_PORT (%d)", healthPort, proxyPort)
	}

	proxyHost := getEnv("PROXY_HOST", "0.0.0.0")
	healthHost := getEnv("HEALTH_HOST", getEnv("STATUS_HOST", proxyHost))

	connectionRetries, err := getEnvAsInt("CONNECTION_RETRIES", 15)
	if err != nil {
		return nil, err
	}

	retryDelay, err := getEnvAsInt("RETRY_DELAY_SECONDS", 5)
	if err != nil {
		return nil, err
	}

	return &Config{
		ProxyHost:         proxyHost,
		ProxyPort:         proxyPort,
		HealthHost:        healthHost,
		HealthPort:        healthPort,
		HealthEnabled:     healthEnabled,
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

	var healthServer *http.Server
	if cfg.HealthEnabled {
		var healthListener net.Listener
		healthServer, healthListener, err = startHealthServer(cfg)
		if err != nil {
			log.Fatalf("Health server error: %v", err)
		}
		defer healthListener.Close()
	}

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

	stopChan := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %s, shutting down...", sig)
		close(stopChan)
		listener.Close()
		if healthServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = healthServer.Shutdown(ctx)
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-stopChan:
				log.Println("Listener closed, mop server shutting down.")
				return
			default:
				log.Printf("Failed to accept connection: %v", err)
				continue
			}
		}
		// Handle each client connection in a new goroutine
		go handleClient(conn, cfg, wakeupProvider)
	}
}
