# Receive pipeline (Phase 3)

How an inbound Signal message travels from the chat websocket all
the way up to a typed event the caller can switch on. Design is
in [ADR 0010](../adr/0010-phase-3-receive.md).

```mermaid
flowchart TB
    server[(chat.signal.org)]
    chat[internal/chat.Connection<br/>auth ws + reconnect]
    env[signalservice.Envelope]
    dispatch{Envelope.Type}

    cipher[internal/cipher.EnvelopeDecryptor]
    sealed[libsignal.DecryptSealedSender]
    prekey[libsignal.DecryptPreKeySignalMessage]
    session[libsignal.DecryptSignalMessage]
    pass[PLAINTEXT_CONTENT strip]

    content[signalservice.Content]
    events[Typed events:<br/>MessageEvent / ReceiptEvent /<br/>TypingEvent / SyncMessageEvent /<br/>DecryptionErrorEvent]
    consumer[pkg/signal.Client.Events<br/>or pkg/bot dispatch]

    stores[(SignalStores via cgo:<br/>SessionStore, IdentityStore,<br/>PreKeyStore, KyberPreKeyStore,<br/>SenderKeyStore)]

    server -->|authenticated WSS| chat
    chat -->|InboundRequest envelope| env
    env --> dispatch

    dispatch -->|UNIDENTIFIED_SENDER| cipher
    dispatch -->|DOUBLE_RATCHET| cipher
    dispatch -->|PREKEY_MESSAGE| cipher
    dispatch -->|PLAINTEXT_CONTENT| pass

    cipher --> sealed
    cipher --> prekey
    cipher --> session
    pass --> content
    sealed --> content
    prekey --> content
    session --> content

    content --> events
    events --> consumer

    cipher -. callbacks .-> stores

    classDef proto fill:#dde7ff,stroke:#3a5fb8,color:#000;
    classDef cry fill:#ffe7c2,stroke:#a3661a,color:#000;
    classDef per fill:#f5d6e8,stroke:#a13a78,color:#000;
    classDef pub fill:#d6f5d6,stroke:#3a7d3a,color:#000;
    classDef done fill:#c8e6c9,stroke:#2e7d32,color:#000;
    class chat,env,dispatch,content proto;
    class sealed,prekey,session,cipher cry;
    class stores per;
    class events,consumer pub;
    class pass done;
```

## Current status

- **Implemented**: authenticated websocket (`internal/chat`), event
  dispatch (`pkg/signal.Client`), typed events, and libsignal-backed
  decrypt via [`internal/cipher.EnvelopeDecryptor`](../internal/cipher/envelope.go)
  (default for [`signal.Open`](../pkg/signal/client.go)).
- **Still open**: prekey rotation on use + top-up (Phase 3 follow-up),
  sender-key / group decrypt (Phase 5), multi-recipient sealed-sender
  edge cases.

## What to look at

- **The cgo callback structs are wired** in `internal/libsignal/stores.go`.
  Load callbacks return `0` with a **null** out-pointer on
  `store.ErrRecordNotFound` (libsignal FFI treats any non-zero return
  as an error).
- **One bad envelope must never kill the loop.** A failed decrypt
  emits a `DecryptionErrorEvent` and the next envelope continues.
- **`PLAINTEXT_CONTENT`** is used for sync messages and decryption-error
  receipts from peers; the leading marker byte is stripped before
  parsing `Content`.
- **Reconnect/backoff** lives in `internal/chat`. Capped exponential
  with jitter (1s … 60s).

## Linked design records

- [ADR 0010 — Phase 3 receive](../adr/0010-phase-3-receive.md)
- [ADR 0005 — Store interface](../adr/0005-store-interface.md)
- [Sealed Sender (Signal blog)](https://signal.org/blog/sealed-sender/)
