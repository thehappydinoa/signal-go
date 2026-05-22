# ADR 0029: SQLite-backed store

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 7 tracks a SQLite-backed store. ADR 0005 planned a separate package
so SQLite remains opt-in. Until this ADR, only `account.Store` persisted to
disk (`fsstore`); `store.SignalStores` (sessions, prekeys, identities,
sender keys) was in-memory only via `memstore`, so linked devices lost crypto
state on restart.

Production bots need durable sessions and prekeys across process restarts.

## Decision

1. **Driver:** `modernc.org/sqlite` (pure Go, no second cgo stack alongside
   libsignal). Added to ADR 0002 allowlist.
2. **Package:** `internal/store/sqlstore` — single `signal.db` file with
   WAL journaling, mode 0600.
3. **Interfaces implemented:**
   - `account.Store` (encrypted account blob, ADR 0012 wire format via shared
     `fsstore.Seal` / `fsstore.Open`)
   - `store.SignalStores` (sessions, identities, prekeys, sender keys)
   - `store.GroupDistributionStore`, `store.GroupEndorsementStore`
4. **Constructors:** `Open` (plaintext, tests), `OpenWithKey`,
   `OpenWithPassphrase` (Argon2id + `kdf.json`, same parameters as fsstore).
5. **Schema version:** `meta.schema_version = 1`; normalized tables per record
   kind (not JSON map files).

### Layout

```
.signal-data/
├── kdf.json          # passphrase mode (shared format with fsstore)
└── signal.db         # SQLite WAL database
```

Callers open one `*sqlstore.DB` and pass:

- `db` as `AccountStore`
- `db.SignalStores()` as `SignalStores`
- `db.GroupDistributionStore()` / `GroupEndorsementStore()` optionally

## Consequences

- Production bots can persist full libsignal state in one file.
- `memstore` remains the default for unit tests; `sqlstore` is opt-in.
- No migration from existing fsstore JSON group files (separate paths); new
  deployments should pick one backend.
- Transactional `Store.Update()` from ADR 0005 remains deferred.

## References

- [ADR 0005](./0005-store-interface.md), [ADR 0012](./0012-encrypted-store.md)
- [ADR 0002](./0002-no-third-party-go-deps.md) (allowlist update)
