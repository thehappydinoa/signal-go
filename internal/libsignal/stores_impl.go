package libsignal

import (
	"github.com/thehappydinoa/signal-go/internal/store"
)

// This file holds the Go-typed business logic that the //export'd C
// callbacks delegate to. Everything here is cgo-free so tests in this
// package can drive these functions directly without invoking the C
// translation layer.
//
// The functions return [store.ErrRecordNotFound] for Load* failures so
// the //export shells in stores.go can map that to libsignal's "1"
// return code; other errors map to "-1".

func loadSessionImpl(s store.SessionStore, addr store.Address) ([]byte, error) {
	return s.LoadSession(addr)
}

func storeSessionImpl(s store.SessionStore, addr store.Address, blob []byte) error {
	return s.StoreSession(addr, blob)
}

func getLocalIdentityKeyPairImpl(s store.IdentityStore) (pub, priv []byte, err error) {
	return s.LocalIdentityKey()
}

func getLocalRegistrationIDImpl(s store.IdentityStore) (uint32, error) {
	return s.LocalRegistrationID()
}

func getIdentityKeyImpl(s store.IdentityStore, addr store.Address) ([]byte, error) {
	return s.LoadIdentity(addr)
}

func saveIdentityKeyImpl(s store.IdentityStore, addr store.Address, pub []byte) (uint8, error) {
	res, err := s.SaveIdentity(addr, pub)
	if err != nil {
		return 0, err
	}
	return uint8(res), nil
}

func isTrustedIdentityImpl(s store.IdentityStore, addr store.Address, pub []byte, dir store.Direction) (bool, error) {
	return s.IsTrustedIdentity(addr, pub, dir)
}

func loadPreKeyImpl(s store.PreKeyStore, id uint32) ([]byte, error) {
	return s.LoadPreKey(id)
}

func storePreKeyImpl(s store.PreKeyStore, id uint32, blob []byte) error {
	return s.StorePreKey(id, blob)
}

func removePreKeyImpl(s store.PreKeyStore, id uint32) error {
	return s.RemovePreKey(id)
}

func loadSignedPreKeyImpl(s store.SignedPreKeyStore, id uint32) ([]byte, error) {
	return s.LoadSignedPreKey(id)
}

func storeSignedPreKeyImpl(s store.SignedPreKeyStore, id uint32, blob []byte) error {
	return s.StoreSignedPreKey(id, blob)
}

func loadKyberPreKeyImpl(s store.KyberPreKeyStore, id uint32) ([]byte, error) {
	return s.LoadKyberPreKey(id)
}

func storeKyberPreKeyImpl(s store.KyberPreKeyStore, id uint32, blob []byte) error {
	return s.StoreKyberPreKey(id, blob)
}

func markKyberPreKeyUsedImpl(s store.KyberPreKeyStore, id uint32) error {
	return s.MarkKyberPreKeyUsed(id)
}

func loadSenderKeyImpl(s store.SenderKeyStore, sender store.Address, distID string) ([]byte, error) {
	return s.LoadSenderKey(sender, distID)
}

func storeSenderKeyImpl(s store.SenderKeyStore, sender store.Address, distID string, blob []byte) error {
	return s.StoreSenderKey(sender, distID, blob)
}

// formatUUID returns the canonical 8-4-4-4-12 string for a 16-byte UUID.
// Pulled out of stores.go (where uuidFromC calls it) so cgo-free tests
// can exercise it.
func formatUUID(b [16]byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	j := 0
	for i, v := range b {
		out[j] = hex[v>>4]
		out[j+1] = hex[v&0x0F]
		j += 2
		if i == 3 || i == 5 || i == 7 || i == 9 {
			out[j] = '-'
			j++
		}
	}
	return string(out)
}
