package provider

import (
	"testing"
)

func TestMagicPacketCreation(t *testing.T) {
	mac := "AA:BB:CC:DD:EE:FF"
	p := &WOLProvider{TargetMAC: mac}
	packet, err := p.createMagicPacket()
	if err != nil {
		t.Fatalf("createMagicPacket failed: %v", err)
	}

	if len(packet) != 102 {
		t.Errorf("Expected packet length 102, got %d", len(packet))
	}

	// Check header
	for i := range 6 {
		if packet[i] != 0xFF {
			t.Errorf("Expected 0xFF at index %d, got %X", i, packet[i])
		}
	}

	// Check MAC repetitions (just spot check a few)
	// First repetition starts at index 6
	expected := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	for i := range 6 {
		if packet[6+i] != expected[i] {
			t.Errorf("Mismatch in MAC repetition at index %d", 6+i)
		}
	}
}
