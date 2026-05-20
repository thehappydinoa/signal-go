package web

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/thehappydinoa/signal-go/internal/prekeys"
)

// IdentityType is the per-namespace selector for prekey endpoints.
type IdentityType string

const (
	// IdentityACI selects the account-identifier namespace.
	IdentityACI IdentityType = "aci"
	// IdentityPNI selects the phone-number-identifier namespace.
	IdentityPNI IdentityType = "pni"
)

// ECPreKey is the wire shape of an unsigned Curve25519 one-time prekey.
type ECPreKey struct {
	KeyID     uint32 `json:"keyId"`
	PublicKey string `json:"publicKey"` // base64
}

// KEMPreKey is the wire shape of a signed Kyber one-time prekey.
type KEMPreKey struct {
	KeyID     uint32 `json:"keyId"`
	PublicKey string `json:"publicKey"` // base64
	Signature string `json:"signature"` // base64
}

// UploadPreKeysRequest is the body of PUT /v2/keys?identity={aci,pni}.
//
// Either or both of the EC and KEM batches may be sent. The signed
// prekey + last-resort Kyber prekey were already uploaded at link time;
// this endpoint is for the rotating one-time batches and for occasional
// rotation of the signed/last-resort keys.
type UploadPreKeysRequest struct {
	IdentityKey     string          `json:"identityKey"`               // base64 of the 33-byte tagged public key
	SignedPreKey    *SignedECPreKey `json:"signedPreKey,omitempty"`    // optional rotation
	PqLastResortKey *SignedKEMPreKey `json:"pqLastResortKey,omitempty"` // optional rotation
	PreKeys         []ECPreKey      `json:"preKeys,omitempty"`         // one-time EC batch
	PqPreKeys       []KEMPreKey     `json:"pqPreKeys,omitempty"`       // one-time Kyber batch
}

// UploadPreKeys issues PUT /v2/keys?identity=<type> for an authenticated
// device, attaching one-time prekey batches.
func (c *Client) UploadPreKeys(ctx context.Context, creds Credentials, ident IdentityType, req UploadPreKeysRequest) error {
	if creds.Username == "" || creds.Password == "" {
		return errors.New("web.UploadPreKeys: credentials required")
	}
	if ident != IdentityACI && ident != IdentityPNI {
		return fmt.Errorf("web.UploadPreKeys: invalid identity %q", ident)
	}
	if req.IdentityKey == "" {
		return errors.New("web.UploadPreKeys: identityKey required")
	}
	q := url.Values{}
	q.Set("identity", string(ident))
	return c.Do(ctx, Request{
		Method:      http.MethodPut,
		Path:        "/v2/keys",
		Query:       q,
		Credentials: creds,
		Body:        req,
	})
}

// ECPreKeyFrom translates one [prekeys.PreKey] into the wire envelope.
func ECPreKeyFrom(p prekeys.PreKey) ECPreKey {
	return ECPreKey{
		KeyID:     p.ID,
		PublicKey: base64.StdEncoding.EncodeToString(p.PublicKey),
	}
}

// KEMPreKeyFrom translates one [prekeys.KyberPreKey] into the wire envelope.
func KEMPreKeyFrom(p prekeys.KyberPreKey) KEMPreKey {
	return KEMPreKey{
		KeyID:     p.ID,
		PublicKey: base64.StdEncoding.EncodeToString(p.PublicKey),
		Signature: base64.StdEncoding.EncodeToString(p.Signature),
	}
}

// ECPreKeysFrom translates a batch of [prekeys.PreKey]s.
func ECPreKeysFrom(in []prekeys.PreKey) []ECPreKey {
	out := make([]ECPreKey, len(in))
	for i, p := range in {
		out[i] = ECPreKeyFrom(p)
	}
	return out
}

// KEMPreKeysFrom translates a batch of [prekeys.KyberPreKey]s.
func KEMPreKeysFrom(in []prekeys.KyberPreKey) []KEMPreKey {
	out := make([]KEMPreKey, len(in))
	for i, p := range in {
		out[i] = KEMPreKeyFrom(p)
	}
	return out
}
