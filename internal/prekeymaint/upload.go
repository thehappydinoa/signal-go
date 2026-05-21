package prekeymaint

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/prekeys"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// UploadIdentity carries the fields needed to publish one-time prekeys.
type UploadIdentity struct {
	PublicKey         []byte
	PrivateKey        []byte
	NextPreKeyID      uint32
	NextKyberPreKeyID uint32
}

// UploadOneTimeBatch generates count one-time Curve25519 + Kyber prekeys,
// uploads them via PUT /v2/keys, and returns ident with next IDs advanced.
// When stores is non-nil, the same keys are persisted locally for inbound
// decrypt.
func UploadOneTimeBatch(
	ctx context.Context,
	webc *web.Client,
	creds web.Credentials,
	kind web.IdentityType,
	ident UploadIdentity,
	count int,
	stores store.SignalStores,
) (UploadIdentity, error) {
	if webc == nil {
		return ident, fmt.Errorf("prekeymaint.UploadOneTimeBatch: nil web client")
	}
	if count <= 0 {
		return ident, fmt.Errorf("prekeymaint.UploadOneTimeBatch: count must be positive")
	}
	identityPriv, err := libsignal.DeserializePrivateKey(ident.PrivateKey)
	if err != nil {
		return ident, fmt.Errorf("prekeymaint.UploadOneTimeBatch: identity priv: %w", err)
	}
	ecBatch, err := prekeys.GenerateOneTimePreKeys(ident.NextPreKeyID, count)
	if err != nil {
		return ident, err
	}
	kemBatch, kemPairs, err := generateOneTimeKyberWithPairs(identityPriv, ident.NextKyberPreKeyID, count)
	if err != nil {
		return ident, err
	}
	if err := prekeys.PersistOneTimePreKeys(stores, ecBatch, kemBatch, kemPairs); err != nil {
		return ident, err
	}
	req := web.UploadPreKeysRequest{
		IdentityKey: base64.StdEncoding.EncodeToString(ident.PublicKey),
		PreKeys:     web.ECPreKeysFrom(ecBatch),
		PqPreKeys:   web.KEMPreKeysFrom(kemBatch),
	}
	if err := webc.UploadPreKeys(ctx, creds, kind, req); err != nil {
		return ident, err
	}
	ident.NextPreKeyID += uint32(count)
	ident.NextKyberPreKeyID += uint32(count)
	return ident, nil
}

func generateOneTimeKyberWithPairs(identityPriv *libsignal.PrivateKey, startID uint32, count int) ([]prekeys.KyberPreKey, []*libsignal.KyberKeyPair, error) {
	if count <= 0 {
		return nil, nil, errors.New("prekeymaint: count must be positive")
	}
	out := make([]prekeys.KyberPreKey, 0, count)
	pairs := make([]*libsignal.KyberKeyPair, 0, count)
	for i := 0; i < count; i++ {
		id := startID + uint32(i)
		kp, err := libsignal.GenerateKyberKeyPair()
		if err != nil {
			return nil, nil, err
		}
		pub, _ := kp.Public()
		sec, _ := kp.Secret()
		pubBytes, _ := pub.Serialize()
		secBytes, _ := sec.Serialize()
		sig, err := libsignal.Sign(identityPriv, pubBytes)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, prekeys.KyberPreKey{ID: id, PublicKey: pubBytes, SecretKey: secBytes, Signature: sig})
		pairs = append(pairs, kp)
	}
	return out, pairs, nil
}
