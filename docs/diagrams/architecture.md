# Architecture

`signal-go` is layered so each ring has one job and ring-N can call ring-N+1
but never the other way around. Cryptography always flows through the
official Rust `libsignal` via cgo; everything above it is our Go code.

```mermaid
flowchart TB
    classDef pub fill:#d6f5d6,stroke:#3a7d3a,color:#000
    classDef proto fill:#dde7ff,stroke:#3a5fb8,color:#000
    classDef crypto fill:#ffe7c2,stroke:#a3661a,color:#000
    classDef store fill:#f5d6e8,stroke:#a13a78,color:#000
    classDef ext fill:#eee,stroke:#888,color:#000,stroke-dasharray: 4 2

    subgraph public[" Public API "]
        signal[pkg/signal<br/><i>Link, LinkOptions, LinkedAccount</i>]
        bot[pkg/bot<br/><i>Phase 6 — planned</i>]
    end

    subgraph protocol[" Protocol layer "]
        prov[internal/provisioning<br/><i>QR-link orchestration</i>]
        chat[internal/chat<br/><i>Phase 3 — authenticated ws</i>]
        web[internal/web<br/><i>REST + Basic auth</i>]
        ws[internal/ws<br/><i>WebSocketMessage envelope</i>]
        prekeys[internal/prekeys<br/><i>signed + Kyber prekey gen</i>]
    end

    subgraph crypto[" Cryptography "]
        ls[internal/libsignal<br/><i>cgo wrapper</i>]
        ffi[(libsignal_ffi.a<br/>Rust, statically linked)]
    end

    subgraph storage[" Persistence "]
        account[internal/account<br/><i>Account model</i>]
        store[internal/store<br/><i>Session/Identity/PreKey/…<br/>interfaces</i>]
        fs[fsstore<br/><i>AES-256-GCM at rest</i>]
        mem[memstore]
    end

    extws[(coder/websocket)]:::ext
    extcrypto[(x/crypto: Argon2id)]:::ext

    bot --> signal
    signal --> prov
    signal --> web
    signal --> account
    signal --> prekeys
    prov --> ws
    prov --> ls
    chat --> ws
    chat --> ls
    web --> account
    ws --> extws
    ls --> ffi
    ls --> store
    fs --> extcrypto
    account --> store
    account -.-> fs
    account -.-> mem

    class signal,bot pub
    class prov,chat,web,ws,prekeys proto
    class ls,ffi crypto
    class store,fs,mem,account store
```

## What to look at

- **Dashed lines** (`mem` / `fs` from `account`) mean "satisfies the
  interface" rather than "imports". The store is plug-in.
- The cgo seam is exactly one package — `internal/libsignal`. Anyone
  auditing the crypto trust story only has to read it.
- `pkg/signal` is what library consumers depend on. Nothing above it
  (e.g. `pkg/bot`, your bot, your bridge) needs to know about cgo or
  Signal's wire protocol.

## Linked design records

- [ADR 0001 — Overall architecture](../adr/0001-overall-architecture.md)
- [ADR 0002 — No third-party Go deps (allowlist)](../adr/0002-no-third-party-go-deps.md)
- [ADR 0005 — Storage interface](../adr/0005-store-interface.md)
