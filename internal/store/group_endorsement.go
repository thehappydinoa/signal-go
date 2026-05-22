package store

import (
	"errors"
	"time"
)

// ErrGroupEndorsementNotFound is returned when no cached group send
// endorsements exist for a master key hex id.
var ErrGroupEndorsementNotFound = errors.New("store: group endorsement not found")

// GroupEndorsementRecord is persisted GSE material for one group.
type GroupEndorsementRecord struct {
	Expiration time.Time
	Response   []byte
	// Endorsements maps member ACI → endorsement blob.
	Endorsements map[string][]byte
}

// GroupEndorsementStore persists group send endorsement caches keyed by
// hex-encoded master key.
type GroupEndorsementStore interface {
	LoadGroupEndorsements(masterKeyHex string) (*GroupEndorsementRecord, error)
	StoreGroupEndorsements(masterKeyHex string, rec *GroupEndorsementRecord) error
	DeleteGroupEndorsements(masterKeyHex string) error
}
