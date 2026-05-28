package store

import "errors"

// ErrProfileKeyNotFound is returned when no profile key is stored for an ACI.
var ErrProfileKeyNotFound = errors.New("store: profile key not found")

// ProfileKeyStore persists recipient profile keys learned at runtime (inbound
// messages, FetchProfile, SetRecipientProfileKey, backup import).
type ProfileKeyStore interface {
	LoadProfileKeys() (map[string][]byte, error)
	StoreProfileKey(aci string, profileKey []byte) error
	DeleteProfileKey(aci string) error
}
