# Receive pipeline (Phase 3 — planned)

How an inbound Signal message will travel from the chat websocket all
the way up to a typed event the caller can switch on. This shape is
locked in [ADR 0010](../adr/0010-phase-3-receive.md); implementation
lands incrementally on this branch.

```mermaid
flowchart TB
    classDef proto fill:#dde7ff,stroke:#3a5fb8,color:#000
    classDef crypto fill:#ffe7c2,stroke:#a3661a,color:#000
    classDef store fill:#f5d6e8,stroke:#a13a78,color:#000
    classDef pub fill:#d6f5d6,stroke:#3a7d3a,color:#000

    server[(chat.signal.org)]
    server -->|"wss://…?login={ACI}.{dev}&password=…"| chat[internal/chat]:::proto
    chat -->|"WebSocketMessage envelope"| env["signalservice.Envelope"]:::proto
    env --> dispatch{Envelope.Type}:::proto

    dispatch -->|UNIDENTIFIED_SENDER| sealed["signal_sealed_sender_<br/>decrypt_message"]:::crypto
    dispatch -->|PREKEY_BUNDLE| prekey["signal_pre_key_signal_<br/>message_decrypt"]:::crypto
    dispatch -->|CIPHERTEXT| session["signal_session_cipher_<br/>decrypt_message"]:::crypto
    dispatch -->|PLAINTEXT_CONTENT| skip["(self-sync messages,<br/>no decrypt)"]

    sealed --> content["signalservice.Content"]:::proto
    prekey --> content
    session --> content
    skip --> content

    content --> events["Typed events:<br/>TextMessage / Receipt /<br/>Typing / Sync /<br/>DecryptionError"]:::pub
    events --> consumer[/"pkg/signal.Client.Events()<br/>or pkg/bot dispatch"/]:::pub

    sealed -.-> stores[(SignalStores via cgo<br/>SessionStore, IdentityStore,<br/>PreKeyStore, KyberPreKeyStore,<br/>SenderKeyStore)]:::store
    prekey -.-> stores
    session -.-> stores
```

## What to look at

- **The cgo callback structs are already wired** (Phase 3b). Each of the
  three decrypt entrypoints takes one or more of those structs, and
  libsignal calls back into our `//export`'d shells in
  `internal/libsignal/stores.go`. They forward to a per-type
  `*Impl` function (in `stores_impl.go`) that's cgo-free and unit-tested.
- **One bad envelope must never kill the loop.** A failed decrypt
  emits a `DecryptionErrorEvent` and the next envelope continues. The
  trust-on-first-use + safety-number-changed events come out of the
  IdentityStore callback path.
- **PLAINTEXT_CONTENT** is Signal's own sync mechanism for messages we
  sent from a different device on the same account; no decrypt needed.
- **Reconnect/backoff** lives in `internal/chat`. Capped exponential
  with jitter (1s … 60s). Each reconnect re-runs the auth handshake;
  the dispatch loop treats it as transparent.

## Linked design records

- [ADR 0010 — Receive pipeline architecture](../adr/0010-phase-3-receive.md)
- [Roadmap Phase 3](../../ROADMAP.md#phase-3--receive-in-progress)
- [Sealed Sender (Signal blog)](https://signal.org/blog/sealed-sender/)
