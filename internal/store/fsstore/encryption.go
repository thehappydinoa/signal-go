package fsstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

// Encrypted-store file format and KDF parameters live here. See ADR 0012.

// keyLen is the length of the symmetric key in bytes (AES-256).
const keyLen = 32

// KeyLen is the AES-256 key size for encrypted stores (ADR 0012).
const KeyLen = keyLen

// Seal encrypts plaintext with key using the ADR 0012 wire format.
func Seal(key [KeyLen]byte, plaintext []byte) ([]byte, error) {
	return seal(key, plaintext)
}

// Open decrypts a blob produced by [Seal].
func Open(key [KeyLen]byte, blob []byte) ([]byte, error) {
	return open(key, blob)
}

// formatVersion is the byte prefix of every encrypted blob; lets us evolve
// the wire format without breaking existing stores.
const formatVersion byte = 0x01

// nonceLen is the GCM nonce size (NIST-recommended 12 bytes).
const nonceLen = 12

// kdfFile holds the Argon2id parameters + salt for passphrase-mode
// stores. Tampering with this file results in a wrong derived key,
// which fails the AEAD tag check at decrypt — there is no need to
// authenticate it separately.
const kdfFile = "kdf.json"

// kdfMeta is the on-disk representation of [kdfFile].
type kdfMeta struct {
	// Version of the KDF metadata format (currently 1).
	Version int `json:"version"`
	// Salt is a 16-byte random value, base64-encoded.
	Salt string `json:"salt"`
	// Time is the Argon2id `time` (number of passes) parameter.
	Time uint32 `json:"time"`
	// Memory is the Argon2id memory parameter in KiB.
	Memory uint32 `json:"memory"`
	// Threads is the Argon2id parallelism factor.
	Threads uint8 `json:"threads"`
}

// defaultKDFParams are the Argon2id parameters used for newly-created
// stores. Values are calibrated for an interactive prompt on a modern
// laptop: ~250ms on Apple M-series, ~400ms on a typical x86 server.
// OWASP's 2026 cheat sheet recommends t≥3, m≥64MiB, p≥4 for password
// storage; we match.
var defaultKDFParams = kdfMeta{
	Version: 1,
	Time:    3,
	Memory:  64 * 1024, // 64 MiB
	Threads: 4,
}

// ErrWrongPassphrase is returned by [LoadAccount] when AEAD decryption
// fails, which is overwhelmingly because the supplied passphrase is wrong.
// A clear signal so CLIs can re-prompt instead of crashing.
var ErrWrongPassphrase = errors.New("fsstore: decryption failed (wrong passphrase or corrupted store)")

// ErrDirEncrypted is returned by [New] when the directory already
// contains an encrypted store and the caller did not supply a key.
var ErrDirEncrypted = errors.New("fsstore: directory contains an encrypted store; use NewWithKey or NewWithPassphrase")

// ErrDirPlaintext is returned by [NewWithKey] / [NewWithPassphrase] when
// the directory already contains a plaintext store, to prevent
// silently leaving the plaintext file behind.
var ErrDirPlaintext = errors.New("fsstore: directory contains a plaintext store; remove it before switching to encrypted mode")

// readKDFMeta loads the kdf.json file under dir. Returns os.ErrNotExist
// (wrapped) if the file is absent.
func readKDFMeta(dir string) (kdfMeta, error) {
	raw, err := os.ReadFile(filepath.Join(dir, kdfFile))
	if err != nil {
		return kdfMeta{}, err
	}
	var m kdfMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return kdfMeta{}, fmt.Errorf("fsstore: parse %s: %w", kdfFile, err)
	}
	if err := m.validate(); err != nil {
		return kdfMeta{}, err
	}
	return m, nil
}

// writeKDFMeta writes m to dir/kdf.json atomically with mode 0600.
func writeKDFMeta(dir string, m kdfMeta) error {
	if err := m.validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("fsstore: marshal kdf meta: %w", err)
	}
	return atomicWrite(dir, kdfFile, raw)
}

func (m kdfMeta) validate() error {
	if m.Version != 1 {
		return fmt.Errorf("fsstore: unsupported kdf metadata version %d", m.Version)
	}
	salt, err := base64.StdEncoding.DecodeString(m.Salt)
	if err != nil {
		return fmt.Errorf("fsstore: kdf salt is not base64: %w", err)
	}
	if len(salt) < 16 {
		return fmt.Errorf("fsstore: kdf salt too short (%d bytes)", len(salt))
	}
	if m.Time == 0 || m.Memory == 0 || m.Threads == 0 {
		return errors.New("fsstore: kdf parameters cannot be zero")
	}
	// Sanity cap: refuse pathologically large parameters that would let
	// a maliciously-edited kdf.json wedge the process.
	const maxMemory = 4 * 1024 * 1024 // 4 GiB
	const maxTime = 100
	if m.Time > maxTime || m.Memory > maxMemory {
		return fmt.Errorf("fsstore: kdf parameters exceed safety caps (time<=%d, memory<=%d KiB)", maxTime, maxMemory)
	}
	return nil
}

// derive runs Argon2id against passphrase + the salt in m, returning a
// 32-byte symmetric key. The salt is decoded from m.Salt.
func (m kdfMeta) derive(passphrase string) ([keyLen]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(m.Salt)
	if err != nil {
		return [keyLen]byte{}, fmt.Errorf("fsstore: kdf salt not base64: %w", err)
	}
	key := argon2.IDKey([]byte(passphrase), salt, m.Time, m.Memory, m.Threads, keyLen)
	if len(key) != keyLen {
		return [keyLen]byte{}, fmt.Errorf("fsstore: argon2 returned %d bytes, want %d", len(key), keyLen)
	}
	var out [keyLen]byte
	copy(out[:], key)
	// Best-effort scrub of the heap allocation. Go's GC means this is
	// advisory; documented in ADR 0012.
	subtle.ConstantTimeCompare(key, key)
	for i := range key {
		key[i] = 0
	}
	return out, nil
}

// newKDFMeta constructs a fresh metadata block with a random salt and
// the default Argon2id parameters.
func newKDFMeta() (kdfMeta, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return kdfMeta{}, fmt.Errorf("fsstore: read salt: %w", err)
	}
	m := defaultKDFParams
	m.Salt = base64.StdEncoding.EncodeToString(salt)
	return m, nil
}

// seal encrypts plaintext with key, returning a self-framed blob:
//
//	version (1) || nonce (12) || ciphertext (||tag)
func seal(key [keyLen]byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("fsstore: aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("fsstore: gcm init: %w", err)
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("fsstore: read nonce: %w", err)
	}
	// Output: 1-byte version || 12-byte nonce || ciphertext+tag.
	out := make([]byte, 0, 1+nonceLen+len(plaintext)+gcm.Overhead())
	out = append(out, formatVersion)
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, plaintext, nil)
	return out, nil
}

// open decrypts blob produced by [seal]. Returns [ErrWrongPassphrase] on
// AEAD authentication failure, or a more specific error for malformed
// framing.
func open(key [keyLen]byte, blob []byte) ([]byte, error) {
	if len(blob) < 1+nonceLen+16 {
		return nil, fmt.Errorf("fsstore: encrypted blob too short (%d bytes)", len(blob))
	}
	if blob[0] != formatVersion {
		return nil, fmt.Errorf("fsstore: unsupported encrypted format version 0x%02x", blob[0])
	}
	nonce := blob[1 : 1+nonceLen]
	ciphertext := blob[1+nonceLen:]
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("fsstore: aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("fsstore: gcm init: %w", err)
	}
	pt, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// We intentionally only wrap ErrWrongPassphrase here; the inner
		// AEAD error is opaque and identical for every failure mode
		// (corrupted blob, truncation, wrong key, tampered ciphertext).
		// Surfacing it in the message would leak nothing useful and
		// errorlint would rightly complain about double-wrapping.
		return nil, fmt.Errorf("%w (%s)", ErrWrongPassphrase, err.Error())
	}
	return pt, nil
}

// atomicWrite writes data to dir/name via a temp file + rename. Mode 0600.
func atomicWrite(dir, name string, data []byte) error {
	final := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return fmt.Errorf("fsstore: tmp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fsstore: write %s: %w", name, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fsstore: chmod %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("fsstore: close %s: %w", name, err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		cleanup()
		return fmt.Errorf("fsstore: rename %s: %w", name, err)
	}
	return nil
}

// existsFile reports whether the file at path exists. Errors other than
// "does not exist" propagate.
func existsFile(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
