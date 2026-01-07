package provider

import (
	"fmt"
	"log"
	"net"
)

// WOLProvider is a WakeupProvider that sends a Wake-on-LAN magic packet.
type WOLProvider struct {
	TargetMAC         string
	TargetBroadcastIP string
}

func (w *WOLProvider) Wake() error {
	return w.sendWOLPacket()
}

// createMagicPacket creates a Wake-on-LAN magic packet from a MAC address string.
func (w *WOLProvider) createMagicPacket() ([]byte, error) {
	hwAddr, err := net.ParseMAC(w.TargetMAC)
	if err != nil {
		return nil, fmt.Errorf("invalid MAC address format: %w", err)
	}

	// Magic packet is 6 bytes of 0xFF followed by 16 repetitions of the MAC address
	packet := make([]byte, 6, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}

	for i := 0; i < 16; i++ {
		packet = append(packet, hwAddr...)
	}

	return packet, nil
}

// sendWOLPacket constructs and sends the Wake-on-LAN packet.
func (w *WOLProvider) sendWOLPacket() error {
	magicPacket, err := w.createMagicPacket()
	if err != nil {
		return err
	}

	// The destination for a WOL packet is typically port 9
	addr := net.JoinHostPort(w.TargetBroadcastIP, "9")

	// We don't need a specific local port, so we can use ":0"
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial UDP for WOL: %w", err)
	}
	defer conn.Close()

	bytesWritten, err := conn.Write(magicPacket)
	if err != nil {
		return fmt.Errorf("failed to write magic packet: %w", err)
	}

	log.Printf("Sent %d byte Wake-on-LAN packet to %s for MAC %s", bytesWritten, w.TargetBroadcastIP, w.TargetMAC)
	return nil
}
