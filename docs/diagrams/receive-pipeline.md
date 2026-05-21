# Receive pipeline (Phase 3)

How an inbound Signal message travels from the chat websocket all
the way up to a typed event the caller can switch on. Design is
in [ADR 0010](../adr/0010-phase-3-receive.md); the connection +
dispatch + event layers are implemented; the libsignal decrypt
wrappers are pending.

```mermaid
flowchart TB
    server[(chat.signal.org)]
    chat[internal/chat.Connection<br/>auth ws + reconnect]
    env[signalservice.Envelope]
    dispatch{Envelope.Type}

    sealed[signal_sealed_sender_<br/>decrypt_message<br/>— pending]
    prekey[signal_pre_key_signal_<br/>message_decrypt<br/>— pending]
    session[signal_session_cipher_<br/>decrypt_message<br/>— pending]
    pass[passthrough decryptor<br/>— current default]

    content[signalservice.Content]
    events[Typed events:<br/>MessageEvent / ReceiptEvent /<br/>TypingEvent / SyncMessageEvent /<br/>DecryptionErrorEvent]
    consumer[pkg/signal.Client.Events<br/>or pkg/bot dispatch]

    stores[(SignalStores via cgo:<br/>SessionStore, IdentityStore,<br/>PreKeyStore, KyberPreKeyStore,<br/>SenderKeyStore)]

    server -->|authenticated WSS| chat
    chat -->|InboundRequest envelope| env
    env --> dispatch

    dispatch -->|UNIDENTIFIED_SENDER| sealed
    dispatch -->|PREKEY_BUNDLE| prekey
    dispatch -->|CIPHERTEXT| session
    dispatch -->|PLAINTEXT_CONTENT| pass

    sealed --> content
    prekey --> content
    session --> content
    pass --> content

    content --> events
    events --> consumer

    sealed -. callbacks .-> stores
    prekey -. callbacks .-> stores
    session -. callbacks .-> stores

    classDef proto fill:#dde7ff,stroke:#3a5fb8,color:#000;
    classDef cry fill:#ffe7c2,stroke:#a3661a,color:#000;
    classDef per fill:#f5d6e8,stroke:#a13a78,color:#000;
    classDef pub fill:#d6f5d6,stroke:#3a7d3a,color:#000;
    classDef done fill:#c8e6c9,stroke:#2e7d32,color:#000;
    class chat,env,dispatch,content proto;
    class sealed,prekey,session cry;
    class stores per;
    class events,consumer pub;
    class pass done;
```

## Current status

- **Implemented**: `internal/chat.Connection` (authenticated ws +
  reconnect), `pkg/signal.Client` (event dispatch), typed event
  structs, passthrough decryptor.
- **Pending**: libsignal decrypt wrappers (`signal_sealed_sender_*`,
  `signal_pre_key_signal_message_decrypt`,
  `signal_session_cipher_decrypt_message`). The `Decryptor` interface
  is defined in `pkg/signal`; plug in the real implementation via
  `OpenOptions.Decryptor`.

## What to look at

- **The cgo callback structs are already wired** (Phase 2). Each of
  the three decrypt entrypoints takes one or more of those structs,
  and libsignal calls back into our `//export`'d shells in
  `internal/libsignal/stores.go`. They forward to a per-type
  `*Impl` function (in `stores_impl.go`) that's cgo-free and
  unit-tested.
- **One bad envelope must never kill the loop.** A failed decrypt
  emits a `DecryptionErrorEvent` and the next envelope continues. The
  trust-on-first-use + safety-number-changed events come out of the
  IdentityStore callback path.
- **`PLAINTEXT_CONTENT`** is Signal's own sync mechanism for messages
  we sent from a different device on the same account; no decrypt
  needed.
- **Reconnect/backoff** lives in `internal/chat`. Capped exponential
  with jitter (1s … 60s). Each reconnect re-runs the auth handshake;
  the dispatch loop treats it as transparent.

## Linked design records

- [ADR 0010 — Receive pipeline architecture](../adr/0010-phase-3-receive.md)
- [Roadmap Phase 3](../../ROADMAP.md#phase-3--receive-in-progress)
- [Sealed Sender (Signal blog)](https://signal.org/blog/sealed-sender/)
