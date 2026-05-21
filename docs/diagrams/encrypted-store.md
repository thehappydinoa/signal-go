# Encrypted store

`fsstore` writes the account JSON sealed under AES-256-GCM. The 32-byte
symmetric key never touches disk; the caller either supplies it directly
(OS keyring / HSM / TPM-sealed) or has it derived from a passphrase via
Argon2id.

```mermaid
flowchart LR
    classDef in fill:#dde7ff,stroke:#3a5fb8,color:#000
    classDef sec fill:#ffe0e0,stroke:#a13a3a,color:#000
    classDef out fill:#d6f5d6,stroke:#3a7d3a,color:#000
    classDef fs fill:#eee,stroke:#888,color:#000

    pp[Passphrase]:::in
    rk[Raw 32-byte key]:::in
    salt[(16-byte salt)]:::fs
    params[Argon2id params<br/>t=3, m=64MiB, p=4]:::fs

    pp -->|"Argon2id<br/>(t=3, m=64MiB, p=4)"| key
    rk --> key

    key[/"32-byte symmetric key<br/>(in memory only)"/]:::sec
    json[Account JSON]:::in
    nonce[/"12-byte nonce<br/>(crypto/rand per write)"/]:::sec

    key -->|AES-256-GCM| sealed
    json --> sealed
    nonce --> sealed

    sealed["account.enc<br/>0x01 || nonce || ct || tag"]:::out

    salt -. "passphrase mode only" .-> pp
    params -. "passphrase mode only" .-> pp

    sealed -->|mode 0600| disk[(disk)]:::fs
    salt --> kdfjson["kdf.json<br/>{salt, time, memory, threads, version}"]
    params --> kdfjson
    kdfjson -->|mode 0600| disk
```

## What to look at

- **The key is never persisted.** Only inputs to the KDF (salt +
  parameters) hit disk; the derived 32 bytes live in the `*fsstore.Store`
  for the process lifetime and die with the process.
- **GCM nonce is fresh per write.** Two saves of the same account
  produce different ciphertexts. Reused nonces under AES-GCM are
  catastrophic — see the tests in `internal/store/fsstore/encryption_test.go`
  for the explicit assertion.
- **Wrong passphrase fails closed.** Argon2id-deriving with the wrong
  passphrase yields a different key, the GCM tag check fails, and
  `LoadAccount` returns a typed `ErrWrongPassphrase` so callers can
  re-prompt rather than crash.
- **Mode mixing is rejected.** A directory with `account.json` blocks
  encrypted constructors with `ErrDirPlaintext`; one with `account.enc`
  blocks `New` with `ErrDirEncrypted`. No silent leftovers.

## Linked design records

- [ADR 0012 — Encrypted account store](../adr/0012-encrypted-store.md)
- [ADR 0011 — Threat model & security audit](../adr/0011-security-audit.md)
- [security.md](../security.md) — short threat model + reporting policy
