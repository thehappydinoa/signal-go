package prekeys

import (
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/store"
)

// PersistOneTimePreKeys stores generated one-time prekeys in the local
// SignalStores so inbound decrypt can consume them and inventory counts
// stay accurate.
func PersistOneTimePreKeys(stores store.SignalStores, ec []PreKey, kem []KyberPreKey, pairs []*libsignal.KyberKeyPair) error {
	if stores == nil {
		return nil
	}
	if len(kem) != len(pairs) {
		return fmt.Errorf("prekeys.PersistOneTimePreKeys: kem/pair length mismatch")
	}
	ts := uint64(time.Now().UnixMilli())
	for _, p := range ec {
		priv, err := libsignal.DeserializePrivateKey(p.PrivateKey)
		if err != nil {
			return fmt.Errorf("prekeys.PersistOneTimePreKeys: ec priv %d: %w", p.ID, err)
		}
		pub, err := libsignal.DeserializePublicKey(p.PublicKey)
		if err != nil {
			return fmt.Errorf("prekeys.PersistOneTimePreKeys: ec pub %d: %w", p.ID, err)
		}
		rec, err := libsignal.NewPreKeyRecord(p.ID, priv, pub)
		if err != nil {
			return err
		}
		blob, err := rec.Serialize()
		if err != nil {
			return err
		}
		if err := stores.StorePreKey(p.ID, blob); err != nil {
			return err
		}
	}
	for i, p := range kem {
		blob, err := libsignal.NewKyberPreKeyRecordBlob(p.ID, ts, pairs[i], p.Signature)
		if err != nil {
			return fmt.Errorf("prekeys.PersistOneTimePreKeys: kem %d: %w", p.ID, err)
		}
		if err := stores.StoreKyberPreKey(p.ID, blob); err != nil {
			return err
		}
	}
	return nil
}
