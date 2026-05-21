# Send flow (Phase 4 — planned)

Sending a 1:1 message. The full design lives with Phase 4; this is the
target shape so the public API and the receive pipeline don't end up
incompatible with it.

```mermaid
sequenceDiagram
    autonumber
    participant App as Caller<br/>(pkg/signal or pkg/bot)
    participant Sig as pkg/signal.Client
    participant LS as internal/libsignal
    participant Web as internal/web
    participant Srv as chat.signal.org

    App->>Sig: Send(ctx, recipientACI, "hello")
    alt no session for recipient
        Sig->>Web: GET /v2/keys/{aci}/{device}
        Web-->>Sig: PreKeyBundle<br/>(identity, signed prekey, Kyber, one-time)
        Sig->>LS: process_prekey_bundle<br/>(creates session)
    end
    Sig->>LS: session_cipher_encrypt(plaintext)
    LS-->>Sig: SignalMessage / PreKeySignalMessage
    Sig->>LS: sealed_sender_encrypt<br/>(with sender certificate)
    LS-->>Sig: SealedSenderMessage envelope
    Sig->>Web: PUT /v1/messages/{recipientACI}<br/>(envelope, timestamp, …)
    alt mismatched/stale devices
        Web-->>Sig: 409 / 410 with device list
        Sig->>Sig: drop stale sessions, refetch bundles,<br/>retry
    end
    Web-->>Sig: 200 OK
    Sig-->>App: nil
```

## What to look at

- **Session establishment** happens lazily on the first send to a
  recipient. Subsequent sends reuse the Double Ratchet state from the
  SessionStore.
- **Sealed sender** is the default. Recipients get an `Envelope` that
  doesn't reveal the sender's ACI to the server. The sender certificate
  is fetched from `/v1/certificate/delivery` and cached.
- **Mismatched/stale devices** (HTTP 409 / 410 with a device list) are
  the server telling us the recipient has added or removed a device
  since we last fetched their bundle. We drop the corresponding
  sessions, refetch, and resend.

## Linked design records

- [Roadmap Phase 4](../../ROADMAP.md#phase-4--send-11-planned)
- [Sealed Sender (Signal blog)](https://signal.org/blog/sealed-sender/)
