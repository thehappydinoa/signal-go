# Send flow (Phase 4 — shipped)

Sending a 1:1 message. Fan-out, retry, and basic-auth send are live.
Sealed-sender is the next planned enhancement.

```mermaid
sequenceDiagram
    autonumber
    participant App as Caller<br/>pkg/signal or pkg/bot
    participant Sig as pkg/signal.Client
    participant LS as internal/libsignal
    participant Web as internal/web
    participant Srv as chat.signal.org

    App->>Sig: Send(ctx, recipientACI, "hello")

    alt device list not cached (first send to this ACI)
        Sig->>Web: GET /v2/keys/{aci}/*
        Web-->>Sig: all devices + bundles
        loop for each device without a cached session
            Sig->>LS: process_prekey_bundle → creates session
        end
        Sig->>Sig: cache device-ID set in Client.knownDevices
    else cached device IDs available
        loop for each cached device with a deleted session
            Sig->>Web: GET /v2/keys/{aci}/{devID}
            Web-->>Sig: fresh bundle
            Sig->>LS: process_prekey_bundle → re-creates session
        end
    end

    loop for each device address
        Sig->>LS: session_cipher_encrypt(padded plaintext)
        LS-->>Sig: SignalMessage / PreKeySignalMessage
    end

    Sig->>Web: PUT /v1/messages/{recipientACI}<br/>[one envelope per device]

    alt HTTP 409 — mismatched devices
        Web-->>Sig: {missingDevices, extraDevices}
        Sig->>Sig: delete sessions for extraDevices
        loop for each missingDevice
            Sig->>Web: GET /v2/keys/{aci}/{devID}
            Web-->>Sig: bundle
            Sig->>LS: process_prekey_bundle
        end
        Sig->>Sig: update knownDevices cache
        Sig->>Web: PUT /v1/messages/{recipientACI} [retry]
    else HTTP 410 — stale devices
        Web-->>Sig: {staleDevices}
        loop for each staleDevice
            Sig->>Sig: delete stale session
            Sig->>Web: GET /v2/keys/{aci}/{devID}
            Web-->>Sig: fresh bundle
            Sig->>LS: process_prekey_bundle
        end
        Sig->>Web: PUT /v1/messages/{recipientACI} [retry]
    end

    Web-->>Sig: 200 OK
    Sig-->>App: Receipt{Timestamp, RecipientACI}
```

## What to look at

- **Session establishment** happens on the first send to a recipient via
  `discoverAndEnsureSessions`. Subsequent sends reuse cached sessions and
  the in-memory device-ID map.
- **Fan-out**: one `SignalMessage` per device is assembled and sent in a
  single PUT. Each envelope is independently encrypted with the device's
  Double Ratchet session.
- **Retry**: at most one retry on 409/410. A second failure propagates
  to the caller.
- **Sealed sender** is the next planned enhancement. Recipients would
  receive envelopes that don't reveal the sender's ACI to the server.
  Requires `signal_sealed_sender_multi_recipient_encrypt` wrapper +
  sender-certificate cache.

## Linked design records

- [ADR 0014 — Send retry and multi-device fan-out strategy](../adr/0014-send-retry-fanout.md)
- [Roadmap Phase 4](../../ROADMAP.md#phase-4--send-11-in-progress)
- [Sealed Sender (Signal blog)](https://signal.org/blog/sealed-sender/)
