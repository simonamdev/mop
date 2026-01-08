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

	"github.com/BurntSushi/toml"
)

// Config holds the application configuration.
type Config struct {
	ProxyHost         string `toml:"proxy_host"`
	ProxyPort         int    `toml:"proxy_port"`
	TargetHost        string `toml:"target_host"`
	TargetPort        int    `toml:"target_port"`
	TargetMAC         string `toml:"target_mac"`
	TargetBroadcastIP string `toml:"target_broadcast_ip"`
	ProxmoxAPIURL     string `toml:"proxmox_api_url"`
	ProxmoxNode       string `toml:"proxmox_node"`
	ProxmoxVMID       string `toml:"proxmox_vmid"`
	ProxmoxToken      string `toml:"proxmox_token"`
	ProxmoxType       string `toml:"proxmox_type"`
	ProxmoxInsecure   bool   `toml:"proxmox_insecure"`
	WakeupMethod      string `toml:"method"`
	ConnectionRetries int    `toml:"connection_retries"`
	RetryDelaySeconds int    `toml:"retry_delay_seconds"`
}

// loadConfig loads configuration from the specified file.
func loadConfig(configFile string) (*Config, error) {
	if configFile == "" {
		configFile = "config.toml"
	}
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file %s not found. Please create one based on config.example.toml", configFile)
	}

	// Set defaults
	cfg := &Config{
		ProxyHost:         "0.0.0.0",
		ProxyPort:         2222,
		TargetPort:        22,
		TargetBroadcastIP: "255.255.255.255",
		ProxmoxType:       "qemu",
		WakeupMethod:      "wol",
		ConnectionRetries: 15,
		RetryDelaySeconds: 5,
	}

	if _, err := toml.DecodeFile(configFile, cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %v", err)
	}

	// Validation
	if cfg.TargetHost == "" {
		return nil, fmt.Errorf("target_host is required")
	}

	cfg.WakeupMethod = strings.ToLower(cfg.WakeupMethod)
	switch cfg.WakeupMethod {
	case "wol":
		if cfg.TargetMAC == "" {
			return nil, fmt.Errorf("target_mac is required when method is 'wol'")
		}
	case "proxmox":
		if cfg.ProxmoxAPIURL == "" {
			return nil, fmt.Errorf("proxmox_api_url is required when method is 'proxmox'")
		}
		if cfg.ProxmoxNode == "" {
			return nil, fmt.Errorf("proxmox_node is required when method is 'proxmox'")
		}
		if cfg.ProxmoxVMID == "" {
			return nil, fmt.Errorf("proxmox_vmid is required when method is 'proxmox'")
		}
		if cfg.ProxmoxToken == "" {
			return nil, fmt.Errorf("proxmox_token is required when method is 'proxmox'")
		}
	}

	return cfg, nil
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
		targetConn, err = net.DialTimeout("tcp", targetAddr, time.Duration(cfg.RetryDelaySeconds)*time.Second)
		if err == nil {
			log.Printf("Successfully connected to target %s on attempt %d.", targetAddr, i+1)
			break
		}
		log.Printf("Attempt %d/%d failed to connect to target: %v. Retrying in %d seconds...", i+1, cfg.ConnectionRetries, err, cfg.RetryDelaySeconds)
		time.Sleep(time.Duration(cfg.RetryDelaySeconds) * time.Second)
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
	cfg, err := loadConfig("config.toml")
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
