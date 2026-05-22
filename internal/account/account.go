package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/thehappydinoa/signal-go/internal/prekeys"
)

// Identity holds an ACI or PNI identity keypair plus the registration ID
// and most recently issued prekeys for that namespace.
type Identity struct {
	// PublicKey is the 33-byte tagged Curve25519 public identity key.
	PublicKey []byte `json:"publicKey"`
	// PrivateKey is the 32-byte private identity key. Must never leave the
	// device.
	PrivateKey []byte `json:"privateKey"`
	// RegistrationID is the 14-bit per-namespace registration ID.
	RegistrationID uint32 `json:"registrationId"`
	// SignedPreKey is the most recently rotated signed prekey.
	SignedPreKey prekeys.SignedPreKey `json:"signedPreKey"`
	// LastResortKyberPreKey is the long-lived ML-KEM prekey.
	LastResortKyberPreKey prekeys.LastResortKyberPreKey `json:"lastResortKyberPreKey"`
	// NextPreKeyID and NextKyberPreKeyID are the IDs to use for the next
	// rotation / upload batch.
	NextPreKeyID      uint32 `json:"nextPreKeyId"`
	NextKyberPreKeyID uint32 `json:"nextKyberPreKeyId"`
}

// Account is the durable state of a linked Signal device.
type Account struct {
	// ACI is the account UUID.
	ACI string `json:"aci"`
	// PNI is the phone-number identifier UUID.
	PNI string `json:"pni"`
	// Number is the E.164 phone number this account belongs to.
	Number string `json:"number"`
	// DeviceID is assigned by the server during /v1/devices/link.
	DeviceID uint32 `json:"deviceId"`
	// Password is the HTTP Basic credential used for all post-link API
	// calls (Authorization: Basic base64("{ACI}.{DeviceID}:{Password}")).
	Password string `json:"password"`
	// ProfileKey is the 32-byte profile encryption key.
	ProfileKey []byte `json:"profileKey"`
	// AccountEntropyPool is the master backup secret (may be empty for
	// accounts that have not enrolled in the new backup system).
	AccountEntropyPool string `json:"accountEntropyPool,omitempty"`
	// ReadReceipts mirrors the user's preference at link time.
	ReadReceipts bool `json:"readReceipts"`

	ACIIdentity Identity `json:"aciIdentity"`
	PNIIdentity Identity `json:"pniIdentity"`
}

// MarshalJSON ensures stable formatting for store backends that diff files.
func (a *Account) MarshalJSON() ([]byte, error) {
	type alias Account
	return json.MarshalIndent((*alias)(a), "", "  ")
}

// Validate sanity-checks an Account before it is handed to higher layers.
func (a *Account) Validate() error {
	switch {
	case a == nil:
		return errors.New("account: nil")
	case a.ACI == "":
		return errors.New("account: missing ACI")
	case a.Number == "":
		return errors.New("account: missing number")
	case a.DeviceID == 0:
		return errors.New("account: deviceId is zero")
	case a.Password == "":
		return errors.New("account: missing password")
	}
	if err := a.ACIIdentity.Validate(); err != nil {
		return fmt.Errorf("account: ACI identity: %w", err)
	}
	if err := a.PNIIdentity.Validate(); err != nil {
		return fmt.Errorf("account: PNI identity: %w", err)
	}
	return nil
}

// LogValue redacts secret fields when an [Account] is passed to a
// [log/slog] handler. Without it, logging an account by reference would
// dump the [Password], [PrivateKey], [ProfileKey], and
// [AccountEntropyPool] fields in the clear. The Phase-8 audit requires
// these never appear in any logger output.
//
// Callers can still log non-secret fields directly (e.g.
// slog.Info("linked", "aci", acct.ACI)) — only the Account-as-a-value
// case is scrubbed.
func (a *Account) LogValue() slog.Value {
	if a == nil {
		return slog.AnyValue(nil)
	}
	return slog.GroupValue(
		slog.String("aci", a.ACI),
		slog.String("pni", a.PNI),
		slog.String("number", redactNumber(a.Number)),
		slog.Uint64("deviceId", uint64(a.DeviceID)),
		slog.String("password", "[REDACTED]"),
		slog.String("profileKey", redactBytes(a.ProfileKey)),
		slog.String("accountEntropyPool", redactString(a.AccountEntropyPool)),
		slog.Bool("readReceipts", a.ReadReceipts),
		slog.Any("aciIdentity", &a.ACIIdentity),
		slog.Any("pniIdentity", &a.PNIIdentity),
	)
}

// LogValue redacts the private-key half of an identity for [log/slog].
func (i *Identity) LogValue() slog.Value {
	if i == nil {
		return slog.AnyValue(nil)
	}
	return slog.GroupValue(
		slog.String("publicKey", redactBytes(i.PublicKey)),
		slog.String("privateKey", "[REDACTED]"),
		slog.Uint64("registrationId", uint64(i.RegistrationID)),
	)
}

// redactBytes returns "[REDACTED N]" for any non-empty slice, "[empty]"
// otherwise, so the *presence* of secret material is visible but the
// material itself is not. Callers asking "did the key actually get
// loaded?" still get a useful answer.
func redactBytes(b []byte) string {
	if len(b) == 0 {
		return "[empty]"
	}
	return fmt.Sprintf("[REDACTED %d bytes]", len(b))
}

func redactString(s string) string {
	if s == "" {
		return "[empty]"
	}
	return fmt.Sprintf("[REDACTED %d chars]", len(s))
}

// redactNumber keeps the country code (everything up to the third
// character) and replaces the rest. Phone numbers identify the user but
// the country code carries little PII on its own.
func redactNumber(num string) string {
	if num == "" {
		return ""
	}
	if len(num) <= 4 {
		return "[REDACTED]"
	}
	return num[:4] + "[REDACTED]"
}

// Validate sanity-checks an Identity.
func (i *Identity) Validate() error {
	if len(i.PublicKey) != 33 {
		return fmt.Errorf("public key length %d, want 33", len(i.PublicKey))
	}
	if len(i.PrivateKey) != 32 {
		return fmt.Errorf("private key length %d, want 32", len(i.PrivateKey))
	}
	if i.RegistrationID == 0 || i.RegistrationID > prekeys.MaxID {
		return fmt.Errorf("registration id %d out of range", i.RegistrationID)
	}
	if len(i.SignedPreKey.PublicKey) != 33 || len(i.SignedPreKey.Signature) != 64 {
		return fmt.Errorf("signed prekey malformed")
	}
	if len(i.LastResortKyberPreKey.PublicKey) < 1000 || len(i.LastResortKyberPreKey.Signature) != 64 {
		return fmt.Errorf("last-resort kyber prekey malformed")
	}
	return nil
}
