package provider

import "log"

// NoopProvider is a WakeupProvider that does nothing.
type NoopProvider struct{}

func (n *NoopProvider) Wake() error {
	log.Println("Noop wakeup: doing nothing")
	return nil
}
