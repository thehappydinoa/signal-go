# ADR 0010 — Phase 3: authenticated receive pipeline

- Status: Accepted (design only; implementation lands incrementally on
  this branch)
- Date: 2026-05-20

## Context

After Phase 2, a `signal-go` device is registered: it holds an ACI, a PNI,
a deviceId, an account password, identity keys, signed + last-resort
Kyber prekeys, and an initial 100 one-time prekey batch per namespace.

Phase 3 makes that device *receive messages in real time*. Concretely:

1. Open an **authenticated** websocket to
   `wss://chat.signal.org/v1/websocket/?login={ACI}.{deviceId}&password={password}`.
2. Each inbound REQUEST carries a [`signalservice.Envelope`][envelope]
   (sealed-sender, prekey, or normal Double-Ratchet message).
3. Unwrap sealed-sender → run the appropriate libsignal cipher to
   produce a [`signalservice.Content`][content].
4. Hand decoded events to the caller as typed Go structs.

Steps 3 is where the cliff sits: libsignal needs callback structs for
each kind of cryptographic store (sessions, identities, prekeys, signed
prekeys, Kyber prekeys, sender keys). Each is a C struct of function
pointers that libsignal invokes from inside the decrypt call.

[envelope]: ../../internal/proto/gen/signalservicepb
[content]: ../../internal/proto/gen/signalservicepb

## Decision

### Layering

```
pkg/signal.Client                       public API (Receive(ctx), Events)
└── internal/chat.Connection            authenticated ws + reconnect loop
    └── internal/ws.Client              (Phase 1)
└── internal/cipher.Decryptor           orchestrates libsignal decrypt
    ├── libsignal callbacks (Phase 3)
    └── internal/store.{Session,Identity,PreKey,SignedPreKey,
        KyberPreKey,SenderKey}Store
```

### Store sub-interfaces

Each becomes a sub-interface of `store.Store` (ADR 0005 explicitly left
room for these):

- `SessionStore`     — Double Ratchet state per `(address, deviceId)`
- `IdentityStore`    — trusted-identity table keyed by address
- `PreKeyStore`      — one-time Curve25519 prekeys, by id
- `SignedPreKeyStore`— rotating signed prekey
- `KyberPreKeyStore` — Kyber prekeys (last-resort + one-time)
- `SenderKeyStore`   — group v2 sender-key state (Phase 5)

All-in-memory and on-disk impls extend correspondingly. Records are
opaque byte blobs from libsignal's perspective; we only need
`Load(id)`, `Save(id, blob)`, `Delete(id)` operations.

### cgo callback bridging

libsignal callbacks are C function pointers with `void *ctx`. We bridge
them to Go store implementations through `cgo.Handle`:

1. Caller wraps each Go store in an exported cgo function that:
   - Recovers the store via `cgo.Handle(uintptr(ctx)).Value()`
   - Calls the Go method
   - Translates the result to libsignal's return convention
     (0 = success, 1 = not-found, -1 = error stored in out param)
2. We build a `SignalSessionStore{ load: cgoFn, ... }` C struct and
   pass it (plus a handle to the Go store) into `signal_session_cipher_*`.
3. After the call, we release the handle and free any temporary objects.

A dedicated `internal/libsignal/stores.go` will hold the cgo-exported
bridge functions and the constructors that wire them up. This keeps the
unsafe surface in one place.

### Decrypt pipeline

Inbound envelope dispatch:

```go
switch envelope.Type {
case Envelope_PREKEY_BUNDLE:    decryptPreKey(envelope.Content, stores)
case Envelope_CIPHERTEXT:       decryptSession(envelope.Content, stores)
case Envelope_UNIDENTIFIED_SENDER: decryptSealedSender(envelope.Content, stores)
case Envelope_PLAINTEXT_CONTENT: passthrough (sync messages from self)
case Envelope_SENDERKEY_MESSAGE: decryptSenderKey(...) // Phase 5
}
```

For sealed-sender messages we first unwrap with
`signal_sealed_sender_decrypt_message` (which validates the sender
certificate against trust roots and runs the inner session/prekey
decrypt). The trust roots are Signal's well-known X25519 keys, baked
into a const blob in `internal/cipher/roots.go`.

### Public surface

```go
client, err := signal.Open(ctx, signal.OpenOptions{Store: ...})
events := client.Events()                            // <-chan Event
for ev := range events {
    switch e := ev.(type) {
    case *signal.TextMessageEvent: ...
    case *signal.ReceiptEvent:     ...
    case *signal.TypingEvent:      ...
    case *signal.SyncMessageEvent: ...
    }
}
```

`signal.Open` is the post-link entry point; if no account is persisted
it returns `signal.ErrNotLinked` (mapped through from `store.ErrNotLinked`).

### Reconnect

The chat ws stays open indefinitely. Disconnects retry with capped
exponential backoff (1s, 2s, 4s, 8s, 16s, 30s, 60s thereafter), with
jitter. Each reconnect re-runs the auth handshake. The dispatch loop
treats reconnects as transparent.

### Decryption failures

A failed decrypt does **not** kill the receive loop. We:
1. Log the failure with envelope metadata (sender ACI, timestamp).
2. Emit a `*DecryptionErrorEvent` so callers can surface it.
3. Optionally send a `DecryptionErrorMessage` retry request back to the
   sender (libsignal helper) — feature-flagged off until Phase 4 send is
   working.

### Testing strategy (ADR 0006 ring 2)

The bulk of Phase 3 tests use an in-process **double-ended** fake: two
`signal-go` clients pointed at the same fake server, exchanging
messages. Both Alice and Bob run through libsignal locally, so we
exercise the real decrypt path; only the wire is faked.

This also catches Double-Ratchet state regressions (which need a real
pair of clients to surface).

## Consequences

- **Pro**: All ratchet state stays inside libsignal; we only own the
  byte-blob persistence and the dispatcher.
- **Pro**: Public event API is the obvious shape for the bot framework
  in ADR 0008.
- **Con**: cgo callbacks are subtle. Mitigation: every callback gets a
  dedicated unit test using a fake store + a tiny libsignal-driven scenario.
- **Con**: The fake-pair test harness is ~500 LOC. Worth it.

## Out of scope (deferred to later phases)

- Sending messages (Phase 4)
- Groups v2 / sender keys (Phase 5)
- Attachments, storage service sync, CDSI (Phase 7)
