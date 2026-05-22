# ADR 0028: CDSI contact discovery via libsignal-net

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 7 tracks CDSI (Contact Discovery Service Interface) — resolving E.164
phone numbers to Signal ACIs/PNIs inside Signal's SGX enclave at
`cdsi.signal.org`. Unlike storage sync, CDSI has no vendored protobuf; the
wire protocol lives inside libsignal's Rust `libsignal-net` stack.

libsignal v0.94.1 exposes CDSI through async FFI:

- `TokioAsyncContext` + `ConnectionManager`
- `LookupRequest` builder
- `signal_cdsi_lookup_new` / `signal_cdsi_lookup_complete`

Short-lived directory credentials come from `GET /v2/directory/auth` on the
chat service (same pattern as storage auth).

## Decision

1. **Async bridge:** C callbacks in `bridge_async.c` forward promise
   completions to Go channels via `cgo.Handle` (`pointer.go`), matching the
   pattern used by other libsignal Go bindings.
2. **Blocking API:** `libsignal.CDSILookup` blocks until the two-phase async
   lookup completes (60s timeout).
3. **Public API:** `Client.DiscoverContacts` fetches directory auth, runs CDSI,
   returns `[]DiscoveredContact` with E.164, ACI, and PNI strings.
4. **Runtime caching:** One `TokioAsyncContext` + `ConnectionManager` per
   `Client`, torn down in `Client.Close`.

### Layering

| Layer | Responsibility |
|-------|----------------|
| `internal/libsignal` | Tokio, ConnectionManager, LookupRequest, CDSILookup |
| `internal/web` | `FetchDirectoryAuth` (`GET /v2/directory/auth`) |
| `pkg/signal` | `DiscoverContacts`, lazy CDSI runtime |

### Deferred

- Delta lookups with persisted continuation tokens
- `AddPreviousE164` / access-key batching for incremental sync
- Censorship-circumvention proxy configuration on ConnectionManager

## Consequences

- Bots can resolve phone numbers to ACIs without relying on storage sync alone.
- Introduces libsignal-net async infrastructure (first use in signal-go).
- CDSI requires live network access to Signal production services; unit tests
  cover request construction and runtime lifecycle only.

## References

- Signal Desktop `CDSI` class, `config/production.json` (`directoryUrl`)
- libsignal `signal_cdsi_lookup_*` FFI
- [ADR 0027](./0027-storage-service-sync.md)
