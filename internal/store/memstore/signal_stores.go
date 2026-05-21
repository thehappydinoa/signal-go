package memstore

import (
	"bytes"
	"sync"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// SignalStores is an in-memory implementation of [store.SignalStores].
// It is the test-side backing store for libsignal callbacks; production
// uses fsstore.
//
// Goroutine-safe: a single mutex protects every map.
type SignalStores struct {
	mu sync.Mutex

	// Local identity (one per namespace; the caller scopes per ACI or
	// PNI by using a separate SignalStores instance).
	identityPub  []byte
	identityPriv []byte
	regID        uint32

	sessions     map[string][]byte                       // addr.String() -> session record
	identities   map[string][]byte                       // addr.String() -> identity public key (33B)
	preKeys      map[uint32][]byte                       // id -> PreKeyRecord
	signedKeys   map[uint32][]byte                       // id -> SignedPreKeyRecord
	kyberKeys    map[uint32][]byte                       // id -> KyberPreKeyRecord
	usedKyber    map[uint32]struct{}                     // ids that have been consumed
	senderKeys   map[senderKeyKey][]byte                 // (sender, distId) -> SenderKeyRecord
}

type senderKeyKey struct {
	addr   string
	distID string
}

// NewSignalStores returns an empty in-memory SignalStores.
func NewSignalStores() *SignalStores {
	return &SignalStores{
		sessions:   map[string][]byte{},
		identities: map[string][]byte{},
		preKeys:    map[uint32][]byte{},
		signedKeys: map[uint32][]byte{},
		kyberKeys:  map[uint32][]byte{},
		usedKyber:  map[uint32]struct{}{},
		senderKeys: map[senderKeyKey][]byte{},
	}
}

// SetLocalIdentity initialises the local identity key + registration ID.
// Call this once before exercising any callback that needs them.
func (s *SignalStores) SetLocalIdentity(pub, priv []byte, regID uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identityPub = append([]byte(nil), pub...)
	s.identityPriv = append([]byte(nil), priv...)
	s.regID = regID
}

// LocalIdentityKey implements [store.IdentityStore].
func (s *SignalStores) LocalIdentityKey() ([]byte, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.identityPub == nil {
		return nil, nil, store.ErrRecordNotFound
	}
	return bytes.Clone(s.identityPub), bytes.Clone(s.identityPriv), nil
}

// LocalRegistrationID implements [store.IdentityStore].
func (s *SignalStores) LocalRegistrationID() (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.regID, nil
}

// LoadIdentity implements [store.IdentityStore].
func (s *SignalStores) LoadIdentity(addr store.Address) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.identities[addr.String()]
	if !ok {
		return nil, store.ErrRecordNotFound
	}
	return bytes.Clone(v), nil
}

// SaveIdentity implements [store.IdentityStore].
func (s *SignalStores) SaveIdentity(addr store.Address, pub []byte) (store.SaveIdentityResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := addr.String()
	prev, existed := s.identities[key]
	s.identities[key] = bytes.Clone(pub)
	if !existed || bytes.Equal(prev, pub) {
		return store.IdentityUnchanged, nil
	}
	return store.IdentityReplaced, nil
}

// IsTrustedIdentity implements [store.IdentityStore]. Default policy:
// trust-on-first-use, then trust exactly the stored key.
func (s *SignalStores) IsTrustedIdentity(addr store.Address, pub []byte, _ store.Direction) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.identities[addr.String()]
	if !ok {
		return true, nil
	}
	return bytes.Equal(v, pub), nil
}

// LoadSession implements [store.SessionStore].
func (s *SignalStores) LoadSession(addr store.Address) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[addr.String()]
	if !ok {
		return nil, store.ErrRecordNotFound
	}
	return bytes.Clone(v), nil
}

// StoreSession implements [store.SessionStore].
func (s *SignalStores) StoreSession(addr store.Address, record []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[addr.String()] = bytes.Clone(record)
	return nil
}

// LoadPreKey implements [store.PreKeyStore].
func (s *SignalStores) LoadPreKey(id uint32) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.preKeys[id]
	if !ok {
		return nil, store.ErrRecordNotFound
	}
	return bytes.Clone(v), nil
}

// StorePreKey implements [store.PreKeyStore].
func (s *SignalStores) StorePreKey(id uint32, record []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preKeys[id] = bytes.Clone(record)
	return nil
}

// RemovePreKey implements [store.PreKeyStore]. Idempotent.
func (s *SignalStores) RemovePreKey(id uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.preKeys, id)
	return nil
}

// LoadSignedPreKey implements [store.SignedPreKeyStore].
func (s *SignalStores) LoadSignedPreKey(id uint32) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.signedKeys[id]
	if !ok {
		return nil, store.ErrRecordNotFound
	}
	return bytes.Clone(v), nil
}

// StoreSignedPreKey implements [store.SignedPreKeyStore].
func (s *SignalStores) StoreSignedPreKey(id uint32, record []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signedKeys[id] = bytes.Clone(record)
	return nil
}

// LoadKyberPreKey implements [store.KyberPreKeyStore].
func (s *SignalStores) LoadKyberPreKey(id uint32) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.kyberKeys[id]
	if !ok {
		return nil, store.ErrRecordNotFound
	}
	return bytes.Clone(v), nil
}

// StoreKyberPreKey implements [store.KyberPreKeyStore].
func (s *SignalStores) StoreKyberPreKey(id uint32, record []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kyberKeys[id] = bytes.Clone(record)
	return nil
}

// MarkKyberPreKeyUsed implements [store.KyberPreKeyStore].
func (s *SignalStores) MarkKyberPreKeyUsed(id uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usedKyber[id] = struct{}{}
	return nil
}

// KyberPreKeyUsed reports whether MarkKyberPreKeyUsed has been called for id.
// Test-only inspector.
func (s *SignalStores) KyberPreKeyUsed(id uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.usedKyber[id]
	return ok
}

// LoadSenderKey implements [store.SenderKeyStore].
func (s *SignalStores) LoadSenderKey(sender store.Address, distID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.senderKeys[senderKeyKey{sender.String(), distID}]
	if !ok {
		return nil, store.ErrRecordNotFound
	}
	return bytes.Clone(v), nil
}

// StoreSenderKey implements [store.SenderKeyStore].
func (s *SignalStores) StoreSenderKey(sender store.Address, distID string, record []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.senderKeys[senderKeyKey{sender.String(), distID}] = bytes.Clone(record)
	return nil
}
