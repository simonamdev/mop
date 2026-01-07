package provider

// WakeupProvider defines an interface for performing a wake-up action.
type WakeupProvider interface {
	Wake() error
}
