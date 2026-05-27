# Architecture

`signal-go` is layered so each ring has one job and ring-N can call
ring-N+1 but never the other way around. Cryptography always flows
through the official Rust `libsignal` via cgo; everything above it is
our Go code.

```mermaid
flowchart TB
    bot[pkg/bot<br/>dispatch + middleware]
    sig[pkg/signal<br/>public API:<br/>Link, Open, Client, Events]

    subgraph protocol [Protocol layer]
        prov[internal/provisioning<br/>QR-link orchestration]
        chat[internal/chat<br/>authenticated ws + reconnect]
        web[internal/web<br/>REST + Basic auth]
        ws[internal/ws<br/>WebSocketMessage envelope]
        prekeys[internal/prekeys<br/>signed + Kyber prekey gen]
    end

    subgraph crypto [Cryptography]
        ls[internal/libsignal<br/>cgo wrapper]
        dn[internal/devicename<br/>linked-device name at link]
        ffi[(libsignal_ffi.a)]
    end

    subgraph storage [Persistence]
        account[internal/account<br/>Account model]
        store[internal/store<br/>SessionStore / IdentityStore / …]
        sql[sqlstore<br/>SQLite + seal]
        mem[memstore]
    end

    bot --> sig
    sig --> prov
    sig --> chat
    sig --> web
    sig --> account
    sig --> prekeys
    sig --> dn
    prov --> ws
    prov --> ls
    dn --> ls
    chat --> ws
    web --> account
    ls --> ffi
    ls --> store
    account --> store
    fs -. implements .-> store
    mem -. implements .-> store

    classDef pub fill:#d6f5d6,stroke:#3a7d3a,color:#000;
    classDef proto fill:#dde7ff,stroke:#3a5fb8,color:#000;
    classDef cry fill:#ffe7c2,stroke:#a3661a,color:#000;
    classDef per fill:#f5d6e8,stroke:#a13a78,color:#000;
    class bot,sig pub;
    class prov,chat,web,ws,prekeys proto;
    class ls,dn,ffi cry;
    class store,fs,mem,account per;
```

## What to look at

- **Dashed lines** are "satisfies the interface", not "imports". The
  store layer is plug-in: `fsstore` and `memstore` both implement
  `internal/store` and `account.Store`.
- The cgo seam for **Curve25519 / sessions / sealed sender** is exactly
  one package — `internal/libsignal`. Anyone auditing that trust story
  starts there. `internal/devicename` adds a small Android-compatible
  AES/HMAC layer on top of `libsignal.Agree` for the linked-device name
  field only ([ADR 0036](../adr/0036-linked-device-name-cipher.md)).
- `pkg/signal` is what library consumers depend on. Nothing above it
  (your bot, your bridge, `pkg/bot` itself) needs to know about cgo or
  Signal's wire protocol.
- `internal/chat` wraps `internal/ws` with authenticated connection
  management and exponential-backoff reconnection. The dispatch of
  envelope content into typed events happens in `pkg/signal.Client`.

## Linked design records

- [ADR 0001 — Overall architecture](../adr/0001-overall-architecture.md)
- [ADR 0002 — No third-party Go deps (allowlist)](../adr/0002-no-third-party-go-deps.md)
- [ADR 0005 — Storage interface](../adr/0005-store-interface.md)
- [ADR 0010 — Phase 3 receive pipeline](../adr/0010-phase-3-receive.md)
