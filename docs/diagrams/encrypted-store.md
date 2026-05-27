# Encrypted store

[seal](../../internal/store/seal/) seals the account JSON under AES-256-GCM.
[sqlstore](../../internal/store/sqlstore/) stores the ciphertext in `signal.db`;
the 32-byte symmetric key never touches disk. The caller either supplies it
directly (OS keyring / HSM / TPM-sealed) or derives it from a passphrase via
Argon2id (`kdf.json` beside the store directory).

```mermaid
flowchart TB
    subgraph inputs [Key source]
        pp[Passphrase]
        rk[Raw 32-byte key]
    end

    subgraph kdf [Argon2id<br/>t=3, m=64 MiB, p=4]
        salt[16-byte salt]
        params[Parameters]
    end

    key[32-byte symmetric key<br/>in memory only]
    json[Account JSON]
    nonce[12-byte nonce<br/>crypto/rand per write]
    sealed["account row BLOB<br/>0x01 + nonce + ct + tag"]
    kdfjson[kdf.json<br/>salt + params + version]
    disk[(signal.db + kdf.json<br/>mode 0600 / dir 0700)]

    pp --> kdf
    kdf --> key
    rk --> key

    key --> sealed
    json --> sealed
    nonce --> sealed

    salt --> kdfjson
    params --> kdfjson

    sealed --> disk
    kdfjson --> disk

    classDef in fill:#dde7ff,stroke:#3a5fb8,color:#000;
    classDef sec fill:#ffe0e0,stroke:#a13a3a,color:#000;
    classDef out fill:#d6f5d6,stroke:#3a7d3a,color:#000;
    classDef ext fill:#eee,stroke:#888,color:#000;
    class pp,rk,json,salt,params in;
    class key,nonce sec;
    class sealed,kdfjson out;
    class disk ext;
```

## What to look at

- **The key is never persisted.** Only inputs to the KDF (salt +
  parameters) hit disk; the derived 32 bytes live in `*sqlstore.DB` for the
  process lifetime and die with the process.
- **GCM nonce is fresh per write.** Two saves of the same account
  produce different ciphertexts. Reused nonces under AES-GCM are
  catastrophic — see `internal/store/seal/seal_test.go`.
- **Wrong passphrase fails closed.** Argon2id-deriving with the wrong
  passphrase yields a different key, the GCM tag check fails, and
  `LoadAccount` returns `seal.ErrWrongPassphrase` so callers can re-prompt.
- **Legacy fsstore rejected.** Directories with `account.enc` or
  `account.json` from the removed fsstore package must be migrated manually
  (fresh link) before `sqlstore.Open*`.

## On-disk layout

```
store-dir/
├── kdf.json      # passphrase mode only (0600)
└── signal.db     # SQLite WAL; account BLOB + libsignal tables (0600)
```

Parent directory mode `0700`.
