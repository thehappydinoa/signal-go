# ADR 0014 — Send retry and multi-device fan-out strategy

**Status:** Accepted
**Date:** 2026-05-21

## Context

Phase 4 shipped basic 1:1 send: one session, device 1 only, and 409/410
responses were surfaced as typed errors rather than handled automatically.
Real Signal deployments commonly involve accounts with multiple linked
devices (e.g. phone + desktop), and the server enforces that every active
device receives a copy of each message. Two error codes drive corrections:

- **HTTP 409 — Mismatched Devices.** The server compares the set of
  device IDs in our PUT body against the account's current device list.
  `missingDevices` are devices we omitted; `extraDevices` are devices
  we targeted that no longer exist.
- **HTTP 410 — Stale Devices.** One or more targeted devices have a new
  registration ID (the account re-linked that device). Our cached session
  uses the old ID, which the server rejects.

In both cases Signal's own clients drop the affected sessions, re-fetch
prekey bundles for the corrected set, and immediately retry once.

A secondary concern is efficient subsequent sends: we must not re-fetch
the `*` bundle on every call — sessions are expensive to establish and
the server rate-limits prekey fetches.

## Decision

### Multi-device fan-out

On the **first send** to a given recipient ACI, `Client.Send` calls
`GET /v2/keys/{aci}/*` to discover all of the recipient's active devices.
For each device that does not already have a cached session, the bundle
from that response is processed to establish one. The resulting device-ID
set is stored in a per-recipient in-memory cache (`Client.knownDevices`,
guarded by `sync.Mutex`).

On **subsequent sends**, the cached device-ID set is used directly. Only
devices whose sessions have been deleted (see below) trigger an individual
bundle fetch.

### At-most-one retry on 409/410

After the first PUT, if the server returns:

- **409**: sessions for `extraDevices` are deleted; bundles for
  `missingDevices` are fetched individually (`GET /v2/keys/{aci}/{devID}`);
  the in-memory device-ID cache is updated to reflect the corrected set.
- **410**: sessions for `staleDevices` are deleted; bundles for those
  devices are re-fetched individually; sessions are re-established.

In both cases the full envelope set is re-encrypted and PUT is retried
exactly once. If the retry also fails, the error is returned to the caller
as-is (wrapped in a `fmt.Errorf("signal.Send: ...")`). The typed error
(`*web.MismatchedDevicesError` / `*web.StaleDevicesError`) remains
reachable via `errors.As`.

### Device-ID cache lifetime

The cache is purely in-memory and not persisted across process restarts.
On restart the first send to each recipient rediscovers devices via `*`.
This is intentional:

- The device list changes infrequently (linking/unlinking happens rarely).
- Re-fetching on restart is a single extra request, not a correctness
  problem.
- Persisting the cache would require a new store schema (ADR 0012 scope).

### `store.SessionStore` extension

A `DeleteSession(addr Address) error` method was added to the
`SessionStore` interface so that the retry path can evict stale/extra
device sessions atomically from the caller's perspective. The method is
idempotent (deleting a non-existent session is not an error).

## Consequences

- Every first-time send to a recipient makes a `GET /*` call. Subsequent
  sends make no extra network calls if sessions are all cached.
- The `DeleteSession` addition is a breaking change to the `SessionStore`
  interface. All implementations (`memstore.SignalStores`, `fsstore` if
  it ever implements `SignalStores`) must be updated.
- At-most-one retry bounds the worst-case number of PUTs to 2. A
  second 409/410 after the retry propagates to the caller, who must
  handle it explicitly (or discard and log).
- Sealed-sender and multi-recipient encrypt are not part of this ADR;
  they remain in the Phase 4 backlog.
