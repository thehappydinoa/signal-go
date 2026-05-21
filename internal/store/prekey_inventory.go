package store

// PreKeyInventory is implemented by SignalStores backends that can report
// how many one-time prekeys remain locally (for upload top-up).
type PreKeyInventory interface {
	// CountAvailableOneTimePreKeys returns the number of unused one-time
	// Curve25519 prekeys and Kyber prekeys. lastResortKyberID is excluded
	// from the Kyber count (the long-lived last-resort key).
	CountAvailableOneTimePreKeys(lastResortKyberID uint32) (ec int, kyber int, err error)
}
