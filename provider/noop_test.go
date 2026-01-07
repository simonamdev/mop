package provider

import (
	"testing"
)

func TestNoopProvider(t *testing.T) {
	p := &NoopProvider{}
	if err := p.Wake(); err != nil {
		t.Errorf("NoopProvider.Wake() returned error: %v", err)
	}
}
