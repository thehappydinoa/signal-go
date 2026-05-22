# ADR 0034 — Pin Signal's private TLS root for *.signal.org

- Status: Accepted
- Date: 2026-05-22

## Context

Signal's production endpoints (`chat.signal.org`, CDNs, storage, CDSI, etc.)
present TLS certificates issued by **Signal Messenger, LLC**, a private CA
not present in public WebPKI trust stores. Official clients embed
`signal-messenger.cer` and pin that root ([Certifiably Fine](https://signal.org/blog/certifiably-fine/)).

`signal-go` initially registered only Mozilla NSS fallback roots
([ADR 0002](./0002-no-third-party-go-deps.md) / `x509roots/fallback`) to
help cgo Windows builds with an empty OS store. That does **not** trust
Signal's private CA, so `link` and browsers without the Signal root installed
still fail with `x509: certificate signed by unknown authority`.

## Decision

1. Vendor the production root from Signal-iOS
   (`SignalServiceKit/Resources/Certificates/signal-messenger.cer`) under
   [`internal/tlsroots/certs/`](../../internal/tlsroots/certs/), with a
   compile-time SHA-256 fingerprint check.
2. For TLS dials to `*.signal.org` (and `signal.org`), set `tls.Config.RootCAs`
   to a pool containing **only** that root via [`tlsroots.ApplyRootCAs`](../../internal/tlsroots/signal.go)
   from [`internal/ws`](../internal/ws/client.go) and [`internal/web`](../internal/web/client.go).
3. Keep `x509roots/fallback` for empty OS stores on **non-Signal** hosts.
4. `web.Options.PinnedRootCAs` continues to override when set (tests, custom
   deployments).

## Consequences

- **Pro**: `signal-go link` and REST/WebSocket traffic work on Windows/macOS/Linux
  without installing Signal's root into the OS trust store.
- **Pro**: Matches official client trust model; MITM with a public CA cannot
  impersonate `chat.signal.org` to this binary.
- **Con**: Root rotation requires vendoring a new `.cer` and updating the
  fingerprint constant (same operational burden as Signal-Android/iOS).
- **Con**: Browsers still need the OS-trusted root for https://chat.signal.org
  unless the user installs `signal-messenger.cer` manually.

## Verification

- Fingerprint `DD:B0:F9:…:F6:5A` matches Signal-iOS `signal-messenger.cer` and
  the root presented by `chat.signal.org` (checked 2026-05-22).
- `go test ./internal/tlsroots/...`
