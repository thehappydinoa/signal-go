# ADR 0012 — Encrypted account store on disk

- Status: Accepted
- Date: 2026-05-20
- Supersedes the "encryption at rest is out of scope for v0.1.0" stance
  in [ADR 0011](./0011-security-audit.md). That note is now obsolete;
  v0.1.0 ships with encrypted storage.

## Context

[`internal/store/fsstore`](../../internal/store/fsstore/) currently writes
`account.json` with mode `0600` and a `0700` parent directory. The file
contains everything an attacker needs to fully impersonate the user:

- `Password` — the HTTP Basic credential for chat.signal.org. Lets the
  attacker hold the chat-ws open as the user, fetch messages, send.
- `ACIIdentity.PrivateKey`, `PNIIdentity.PrivateKey` — long-term identity
  keys. Compromise breaks end-to-end guarantees in a way the user cannot
  recover from short of re-linking.
- `ProfileKey`, `AccountEntropyPool` — open profiles, backups.
- The current signed prekey + Kyber prekey *secret* halves.

Filesystem permissions defend against unprivileged users on the same
host while the process is running. They do not defend against:

- Backups (cloud sync, `rsync`, container snapshots) that copy the dir
  with metadata preserved imperfectly.
- Stolen-laptop scenarios where the attacker boots from another OS.
- Compromised root accounts (limited recourse here, but at-rest
  encryption raises the bar).

Other Signal clients address this with at-rest encryption:

- **Signal-Desktop**: SQLite encrypted via sqlcipher; key in the OS
  keyring (libsecret / Keychain / DPAPI).
- **signal-cli** (Java): plaintext JSON; requires user-managed FS perms.
- **signalmeow**: SQLite, can be encrypted via sqlcipher; key
  user-supplied.

## Decision

`signal-go` encrypts `account.enc` on disk using AES-256-GCM with a
32-byte key. The key never touches the filesystem.

### Key sources

Two supported, in priority order:

1. **Passphrase**: caller supplies a UTF-8 string. We derive a 32-byte
   key with **Argon2id** (memory-hard, current OWASP 2026 recommendation).
   - Default parameters: `time=3`, `memory=64 MiB`, `threads=4`, `keyLen=32`
   - Per-store salt (16 bytes from `crypto/rand`) stored in `kdf.json`
     alongside the Argon2id parameters.
   - Constructor: `fsstore.NewWithPassphrase(dir, passphrase)`
2. **Raw 32-byte key** (advanced): caller manages the key themselves
   (e.g. fetched from an OS keyring, an HSM, or derived from a TPM
   sealing). No KDF metadata persisted.
   - Constructor: `fsstore.NewWithKey(dir, key [32]byte)`

We deliberately do **not** support a "plaintext mode" via flag on the
public production path. `fsstore.New(dir)` continues to exist for tests
and is documented as "test-only — does NOT encrypt".

### File layout

```
.signal-data/
├── kdf.json          # passphrase mode only: { salt, time, memory, threads, version }
├── account.enc       # 1-byte version || 12-byte nonce || ciphertext+tag
```

### Wire format of `account.enc`

```
byte 0:        format version (currently 0x01)
bytes 1-12:    GCM nonce (12 bytes, random per write via crypto/rand)
bytes 13-end:  AES-256-GCM(ciphertext || 16-byte tag) over the JSON-marshalled Account
```

The version byte lets us evolve format without breaking existing stores.
Re-keying / parameter upgrades land in a subsequent ADR if needed.

### Mixed-mode safety

If a caller opens an encrypted store with `fsstore.New()` (no key) or
vice versa, we fail at the next read or write with a clear error rather
than silently producing garbage. Concretely:

- `New(dir)` returns `ErrDirEncrypted` if `account.enc` exists.
- `NewWithKey(dir, ...)` returns `ErrDirPlaintext` if `account.json` exists.

### Wrong-passphrase failure

A wrong passphrase produces a different 32-byte key, which fails the
GCM tag check on decrypt. We surface this as a typed
`ErrWrongPassphrase` (wrapping the underlying AEAD error) so the CLI
can re-prompt instead of crashing.

### Key lifetime in memory

The 32-byte key lives in the `Store` struct for the process lifetime.
Go's GC makes deterministic zeroization impossible (the GC may move
data around, and `runtime.KeepAlive` only delays freeing). We document
this limitation and recommend that long-running daemons start the
process with the passphrase prompt and then drop privileges + close
stdin to limit accidental leakage.

### Dependencies

`golang.org/x/crypto/argon2` becomes a runtime dep. Added to the
[ADR 0002](./0002-no-third-party-go-deps.md) allowlist:
golang.org/x/crypto is the Go team's staging area for cryptographic
primitives, has no transitive non-stdlib deps, and is widely audited
(it's the source of every cryptographic library on the planet that
isn't using `crypto/*`).

## Consequences

- **Pro**: A stolen disk, a stray backup, or a compromised non-root
  user no longer leak credentials by themselves. The attacker needs
  the passphrase (or the raw key).
- **Pro**: Headless deployments can fetch the raw key from a keyring,
  HSM, AWS KMS, etc., without us needing to integrate each one.
- **Con**: Passphrase prompts hurt ergonomics. Mitigation: CLI
  supports `-passphrase-file <path>` for service-style setups; the
  bot framework (Phase 6) will surface this in its `bot.Open`.
- **Con**: An attacker with code execution while the process runs can
  still read the key from memory. This is the standard trade-off
  documented in ADR 0011.

## Alternatives considered

1. **OS keyring (libsecret/Keychain/DPAPI) as the only path.** Cleanest
   from a UX angle but pulls in cgo deps (libsecret on Linux) or platform
   APIs that don't exist on headless servers. We may add this as a
   *source* of the raw key in a follow-up; the current ADR is mode-agnostic.
2. **SQLite + sqlcipher.** Heavyweight; pulls in cgo SQLite. Defer until
   Phase 7 if/when the store grows beyond the small-file regime.
3. **age (filippo.io/age).** Lovely library. Argon2id is more standard
   for at-rest credentials and we keep the dep surface smaller.
4. **PBKDF2 / scrypt instead of Argon2id.** PBKDF2 is too cheap to
   attack on modern hardware; scrypt is fine but the industry has
   converged on Argon2id since 2015.

## Future

- TPM- or Secure-Enclave-backed key wrapping (out of scope; needs
  cgo per platform).
- OS keyring integration as an optional key source on top of
  `NewWithKey`. See [github.com/zalando/go-keyring](https://github.com/zalando/go-keyring).
- Key rotation: re-encrypt under a new key. Implementable today via
  `LoadAccount` -> create a new `fsstore.NewWithKey(otherDir, newKey)`
  -> `SaveAccount`.
