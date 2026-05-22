package store

import "errors"

// ErrGroupDistributionNotFound is returned when no distribution UUID is
// stored for a group master key hex id.
var ErrGroupDistributionNotFound = errors.New("store: group distribution id not found")

// GroupDistributionStore persists per-group sender-key distribution UUIDs
// keyed by hex-encoded master key.
type GroupDistributionStore interface {
	LoadGroupDistributionID(masterKeyHex string) (string, error)
	StoreGroupDistributionID(masterKeyHex, distributionID string) error
}
