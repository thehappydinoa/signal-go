package sqlstore

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"

	"github.com/thehappydinoa/signal-go/internal/store/seal"
)

const kdfFile = "kdf.json"

type kdfMeta struct {
	Version int    `json:"version"`
	Salt    string `json:"salt"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
}

var defaultKDFParams = kdfMeta{
	Version: 1,
	Time:    3,
	Memory:  64 * 1024,
	Threads: 4,
}

func loadOrCreatePassphraseKey(dir, passphrase string) ([seal.KeyLen]byte, error) {
	var zero [seal.KeyLen]byte
	path := filepath.Join(dir, kdfFile)
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		var meta kdfMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return zero, fmt.Errorf("sqlstore: parse kdf.json: %w", err)
		}
		if err := meta.validate(); err != nil {
			return zero, err
		}
		return meta.derive(passphrase)
	case errors.Is(err, os.ErrNotExist):
		meta, err := newKDFMeta()
		if err != nil {
			return zero, err
		}
		out, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return zero, err
		}
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return zero, fmt.Errorf("sqlstore: write kdf.json: %w", err)
		}
		return meta.derive(passphrase)
	default:
		return zero, fmt.Errorf("sqlstore: read kdf.json: %w", err)
	}
}

func newKDFMeta() (kdfMeta, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return kdfMeta{}, fmt.Errorf("sqlstore: read salt: %w", err)
	}
	m := defaultKDFParams
	m.Salt = base64.StdEncoding.EncodeToString(salt)
	return m, nil
}

func (m kdfMeta) validate() error {
	if m.Version != 1 {
		return fmt.Errorf("sqlstore: unsupported kdf metadata version %d", m.Version)
	}
	salt, err := base64.StdEncoding.DecodeString(m.Salt)
	if err != nil {
		return fmt.Errorf("sqlstore: kdf salt is not base64: %w", err)
	}
	if len(salt) < 16 {
		return errors.New("sqlstore: kdf salt too short")
	}
	if m.Time == 0 || m.Memory == 0 || m.Threads == 0 {
		return errors.New("sqlstore: kdf parameters cannot be zero")
	}
	return nil
}

func (m kdfMeta) derive(passphrase string) ([seal.KeyLen]byte, error) {
	var out [seal.KeyLen]byte
	salt, err := base64.StdEncoding.DecodeString(m.Salt)
	if err != nil {
		return out, fmt.Errorf("sqlstore: kdf salt not base64: %w", err)
	}
	key := argon2.IDKey([]byte(passphrase), salt, m.Time, m.Memory, m.Threads, seal.KeyLen)
	copy(out[:], key)
	return out, nil
}
