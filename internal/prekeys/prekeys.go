package prekeys

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

// MaxID is the inclusive upper bound on prekey IDs. Signal's wire format
// reserves the top bit and uses 14-bit IDs in practice.
const MaxID uint32 = 0x3FFE

// SignedPreKey is a rotating Curve25519 prekey, signed by the owning
// identity key.
type SignedPreKey struct {
	ID        uint32
	PublicKey []byte // 33 bytes (libsignal tagged Curve25519 point)
	// PrivateKey stays local; never uploaded.
	PrivateKey []byte
	// Signature is XEdDSA(identityPriv, PublicKey).
	Signature []byte
}

// LastResortKyberPreKey is a long-lived Kyber/ML-KEM prekey, signed by the
// owning identity key. The "last resort" name comes from upstream: this
// prekey is consumed when the server runs out of one-time Kyber prekeys.
type LastResortKyberPreKey struct {
	ID        uint32
	PublicKey []byte
	SecretKey []byte
	Signature []byte
	// RecordBlob is the pre-serialized libsignal KyberPreKeyRecord, computed
	// at generation time while the KyberKeyPair is in scope. Stored so the
	// record can be written to SignalStores without reconstructing the pair.
	RecordBlob []byte `json:"recordBlob,omitempty"`
}

// PreKey is a single-use Curve25519 prekey, unsigned.
type PreKey struct {
	ID         uint32
	PublicKey  []byte
	PrivateKey []byte
}

// KyberPreKey is a single-use Kyber prekey, signed (so the recipient can
// verify it came from the claimed identity).
type KyberPreKey struct {
	ID        uint32
	PublicKey []byte
	SecretKey []byte
	Signature []byte
}

// GenerateSignedPreKey creates one signed Curve25519 prekey signed by
// identityPriv. The new key's bytes are added to the record straight from
// libsignal; the signature covers the 33-byte tagged public key.
func GenerateSignedPreKey(identityPriv *libsignal.PrivateKey, id uint32) (*SignedPreKey, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	priv, err := libsignal.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("prekeys: generate signed prekey: %w", err)
	}
	pub, err := priv.PublicKey()
	if err != nil {
		return nil, err
	}
	pubBytes, err := pub.Serialize()
	if err != nil {
		return nil, err
	}
	privBytes, err := priv.Serialize()
	if err != nil {
		return nil, err
	}
	sig, err := libsignal.Sign(identityPriv, pubBytes)
	if err != nil {
		return nil, fmt.Errorf("prekeys: sign: %w", err)
	}
	return &SignedPreKey{
		ID:         id,
		PublicKey:  pubBytes,
		PrivateKey: privBytes,
		Signature:  sig,
	}, nil
}

// GenerateLastResortKyberPreKey creates one signed ML-KEM prekey.
func GenerateLastResortKyberPreKey(identityPriv *libsignal.PrivateKey, id uint32) (*LastResortKyberPreKey, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	kp, err := libsignal.GenerateKyberKeyPair()
	if err != nil {
		return nil, fmt.Errorf("prekeys: generate kyber: %w", err)
	}
	pub, err := kp.Public()
	if err != nil {
		return nil, err
	}
	sec, err := kp.Secret()
	if err != nil {
		return nil, err
	}
	pubBytes, err := pub.Serialize()
	if err != nil {
		return nil, err
	}
	secBytes, err := sec.Serialize()
	if err != nil {
		return nil, err
	}
	sig, err := libsignal.Sign(identityPriv, pubBytes)
	if err != nil {
		return nil, fmt.Errorf("prekeys: sign kyber: %w", err)
	}
	blob, err := libsignal.NewKyberPreKeyRecordBlob(id, uint64(time.Now().UnixMilli()), kp, sig)
	if err != nil {
		return nil, fmt.Errorf("prekeys: serialize kyber: %w", err)
	}
	return &LastResortKyberPreKey{
		ID:         id,
		PublicKey:  pubBytes,
		SecretKey:  secBytes,
		Signature:  sig,
		RecordBlob: blob,
	}, nil
}

// GenerateOneTimePreKeys creates count Curve25519 prekeys starting at
// startID (inclusive) and incrementing by 1 for each.
func GenerateOneTimePreKeys(startID uint32, count int) ([]PreKey, error) {
	if count <= 0 {
		return nil, errors.New("prekeys: count must be positive")
	}
	out := make([]PreKey, 0, count)
	for i := 0; i < count; i++ {
		id := startID + uint32(i)
		if err := validateID(id); err != nil {
			return nil, err
		}
		priv, err := libsignal.GeneratePrivateKey()
		if err != nil {
			return nil, err
		}
		pub, err := priv.PublicKey()
		if err != nil {
			return nil, err
		}
		pubBytes, err := pub.Serialize()
		if err != nil {
			return nil, err
		}
		privBytes, err := priv.Serialize()
		if err != nil {
			return nil, err
		}
		out = append(out, PreKey{ID: id, PublicKey: pubBytes, PrivateKey: privBytes})
	}
	return out, nil
}

// GenerateOneTimeKyberPreKeys creates count signed ML-KEM prekeys, each
// signed under identityPriv.
func GenerateOneTimeKyberPreKeys(identityPriv *libsignal.PrivateKey, startID uint32, count int) ([]KyberPreKey, error) {
	if count <= 0 {
		return nil, errors.New("prekeys: count must be positive")
	}
	out := make([]KyberPreKey, 0, count)
	for i := 0; i < count; i++ {
		id := startID + uint32(i)
		if err := validateID(id); err != nil {
			return nil, err
		}
		kp, err := libsignal.GenerateKyberKeyPair()
		if err != nil {
			return nil, err
		}
		pub, _ := kp.Public()
		sec, _ := kp.Secret()
		pubBytes, _ := pub.Serialize()
		secBytes, _ := sec.Serialize()
		sig, err := libsignal.Sign(identityPriv, pubBytes)
		if err != nil {
			return nil, err
		}
		out = append(out, KyberPreKey{ID: id, PublicKey: pubBytes, SecretKey: secBytes, Signature: sig})
	}
	return out, nil
}

// NewRegistrationID returns a fresh 14-bit registration identifier in the
// valid range [1, MaxID].
func NewRegistrationID() (uint32, error) {
	for {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		// Mask to 14 bits.
		id := binary.BigEndian.Uint32(b[:]) & 0x3FFF
		if id != 0 && id <= MaxID {
			return id, nil
		}
	}
}

func validateID(id uint32) error {
	if id == 0 || id > MaxID {
		return fmt.Errorf("prekeys: id %d out of range [1,%d]", id, MaxID)
	}
	return nil
}
