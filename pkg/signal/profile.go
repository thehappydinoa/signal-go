package signal

import (
	"context"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/profile"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// Profile holds decrypted profile fields for a Signal user.
type Profile struct {
	// GivenName and FamilyName are the decrypted profile name parts.
	GivenName  string
	FamilyName string
	About      string
	AboutEmoji string
	// AvatarPath is the CDN path for the profile photo (not fetched here).
	AvatarPath string
	// UnrestrictedUnidentifiedAccess mirrors the server flag.
	UnrestrictedUnidentifiedAccess bool
}

// DisplayName returns "Given Family" when both parts are set, or whichever
// is non-empty.
func (p *Profile) DisplayName() string {
	if p == nil {
		return ""
	}
	switch {
	case p.GivenName != "" && p.FamilyName != "":
		return p.GivenName + " " + p.FamilyName
	case p.GivenName != "":
		return p.GivenName
	default:
		return p.FamilyName
	}
}

// FetchProfile retrieves and decrypts the versioned profile for recipientACI
// using the supplied 32-byte profile key. On success it also caches the UAK
// derived from the profile key so subsequent [Send] calls can use sealed-sender
// delivery automatically.
//
// If profileKey is nil, FetchProfile looks up a key previously stored from
// an inbound DataMessage ([Client] caches profile keys automatically).
func (c *Client) FetchProfile(ctx context.Context, recipientACI string, profileKey []byte) (*Profile, error) {
	if recipientACI == "" {
		return nil, errors.New("signal.FetchProfile: empty recipient")
	}
	if len(profileKey) == 0 {
		c.mu.Lock()
		profileKey = append([]byte(nil), c.knownProfileKeys[recipientACI]...)
		c.mu.Unlock()
	}
	if len(profileKey) == 0 {
		return nil, errors.New("signal.FetchProfile: no profile key (pass one or receive a message carrying profileKey)")
	}
	if err := libsignal.ValidateProfileKey(profileKey); err != nil {
		return nil, fmt.Errorf("signal.FetchProfile: %w", err)
	}
	if c.webc == nil {
		return nil, errors.New("signal.FetchProfile: Client was opened without send-side dependencies")
	}

	uak, err := libsignal.DeriveAccessKey(profileKey)
	if err != nil {
		return nil, fmt.Errorf("signal.FetchProfile: derive UAK: %w", err)
	}
	version, err := libsignal.ProfileKeyVersion(profileKey, recipientACI)
	if err != nil {
		return nil, fmt.Errorf("signal.FetchProfile: profile key version: %w", err)
	}

	wire, err := c.webc.FetchVersionedProfile(ctx, recipientACI, version, uak[:])
	if err != nil {
		return nil, fmt.Errorf("signal.FetchProfile: %w", err)
	}

	cipher, err := profile.NewCipher(profileKey)
	if err != nil {
		return nil, fmt.Errorf("signal.FetchProfile: %w", err)
	}

	out := &Profile{
		AvatarPath:                     wire.Avatar,
		UnrestrictedUnidentifiedAccess: wire.UnrestrictedUnidentifiedAccess,
	}

	if nameBytes, err := web.DecodeBase64Field(wire.Name); err != nil {
		return nil, fmt.Errorf("signal.FetchProfile: name: %w", err)
	} else if len(nameBytes) > 0 {
		given, family, err := profile.DecryptName(cipher, nameBytes)
		if err != nil {
			return nil, fmt.Errorf("signal.FetchProfile: decrypt name: %w", err)
		}
		out.GivenName, out.FamilyName = given, family
	}

	if aboutBytes, err := web.DecodeBase64Field(wire.About); err != nil {
		return nil, fmt.Errorf("signal.FetchProfile: about: %w", err)
	} else if len(aboutBytes) > 0 {
		out.About, err = cipher.DecryptString(aboutBytes)
		if err != nil {
			return nil, fmt.Errorf("signal.FetchProfile: decrypt about: %w", err)
		}
	}

	if emojiBytes, err := web.DecodeBase64Field(wire.AboutEmoji); err != nil {
		return nil, fmt.Errorf("signal.FetchProfile: aboutEmoji: %w", err)
	} else if len(emojiBytes) > 0 {
		out.AboutEmoji, err = cipher.DecryptString(emojiBytes)
		if err != nil {
			return nil, fmt.Errorf("signal.FetchProfile: decrypt aboutEmoji: %w", err)
		}
	}

	c.storeRecipientProfileKey(recipientACI, profileKey)
	return out, nil
}

// SetRecipientProfileKey stores a 32-byte profile key for recipientACI and
// derives + caches the corresponding UAK for sealed-sender send. Passing
// nil or an empty slice removes any cached key.
func (c *Client) SetRecipientProfileKey(aci string, profileKey []byte) {
	if len(profileKey) == 0 {
		c.mu.Lock()
		delete(c.knownProfileKeys, aci)
		delete(c.knownUAKs, aci)
		c.mu.Unlock()
		return
	}
	c.storeRecipientProfileKey(aci, profileKey)
}

func (c *Client) storeRecipientProfileKey(aci string, profileKey []byte) {
	if len(profileKey) != libsignal.ProfileKeyLen {
		return
	}
	uak, err := libsignal.DeriveAccessKey(profileKey)
	if err != nil {
		c.log.Debug("profile key UAK derivation failed", "aci", aci, "err", err)
		return
	}
	c.mu.Lock()
	if c.knownProfileKeys == nil {
		c.knownProfileKeys = make(map[string][]byte)
	}
	c.knownProfileKeys[aci] = append([]byte(nil), profileKey...)
	if c.knownUAKs == nil {
		c.knownUAKs = make(map[string][]byte)
	}
	c.knownUAKs[aci] = uak[:]
	c.mu.Unlock()
}

// ensureRecipientUAK populates knownUAKs from a stored profile key when the
// UAK has not been set explicitly via [SetRecipientUAK].
func (c *Client) ensureRecipientUAK(aci string) {
	c.mu.Lock()
	hasUAK := len(c.knownUAKs[aci]) > 0
	profileKey := append([]byte(nil), c.knownProfileKeys[aci]...)
	c.mu.Unlock()
	if hasUAK || len(profileKey) == 0 {
		return
	}
	c.storeRecipientProfileKey(aci, profileKey)
}
