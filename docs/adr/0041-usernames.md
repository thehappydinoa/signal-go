# ADR 0041 — Username management

- Status: Accepted
- Date: 2026-11-14

## Context

Signal introduced usernames as an optional alias that lets one Signal
account be reached without exposing its phone number. A user picks a
nickname + discriminator (e.g. `alice.42`), the server stores a
hash-of-username → ACI mapping, and either party can also publish a
username *link* — an opaque URL whose owner can be resolved to a
username (and from there to a Signal address) without ever revealing the
phone number to the discoverer. libsignal exposes the full surface:
`signal_username_candidates_from`, `signal_username_hash`,
`signal_username_proof`, `signal_username_verify`,
`signal_authenticated_chat_connection_reserve_username_hash`,
`signal_authenticated_chat_connection_confirm_username`,
`signal_authenticated_chat_connection_set_username_link`,
`signal_authenticated_chat_connection_delete_username_hash`,
`signal_authenticated_chat_connection_delete_username_link`,
`signal_unauthenticated_chat_connection_look_up_username_hash`, and
`signal_unauthenticated_chat_connection_look_up_username_link`.

Today a signal-go bot can only reach a peer by ACI (and can only
*be reached* by the phone number it was linked to). That is an
obstacle for bot operators who want to publish a stable contact handle
without exposing the operator's personal number, and for discovery of
users who have elected username-only reachability.

Usernames are orthogonal to how the account was created. They work the
same whether signal-go linked as a secondary device (the current path,
the "only linking as a secondary device" non-goal in
[ROADMAP.md](../../ROADMAP.md)) or registered as a primary device
without a phone number
([ADR 0042](./0042-paid-no-number-registration.md), proposed in
parallel). This ADR covers only the username surface.

## Decision

Add a new public package `pkg/signal/username` and extend
`*signal.Client` with the server-facing methods. Local helpers wrap the
sync libsignal FFI listed above; server operations go through
`internal/web` against the Signal-Server REST endpoints
(`/v1/accounts/username_hash/*`, `/v1/accounts/username_link/*`) rather
than libsignal's async `AuthenticatedChatConnection` abstraction. Signal
clients have since migrated to gRPC via that abstraction, but the REST
surface is still live server-side and avoids pulling the async-promise +
tokio scaffolding into signal-go for one feature.

The public shape:

```go
package username

type Candidate struct { Nickname string; Discriminator uint64 }

// Local helpers — no network.
func Candidates(nickname string, minLen, maxLen int) ([]Candidate, error)
func Verify(candidate Candidate, proof []byte) error

// Bound to a *signal.Client. Uses its authenticated web client.
func (c *signal.Client) ReserveUsername(ctx, []username.Candidate) (username.Candidate, error)
func (c *signal.Client) ConfirmUsername(ctx, reserved username.Candidate) (*signal.PublishedUsernameLink, error)
func (c *signal.Client) DeleteUsername(ctx) error
func (c *signal.Client) RotateUsernameLink(ctx, current username.Candidate, keepHandle bool) (*signal.PublishedUsernameLink, error)
func (c *signal.Client) DeleteUsernameLink(ctx) error

// Discovery — unauthenticated requests on the web client.
func (c *signal.Client) LookupUsernameHash(ctx, hash [signal.HashLen]byte) (aci string, err error)
func (c *signal.Client) LookupUsername(ctx, handle string, entropy []byte) (username.Candidate, error)

type signal.PublishedUsernameLink struct { Handle string; Entropy []byte }
func (p signal.PublishedUsernameLink) ShareURL() string // https://signal.me/#eu/...
```

Reserve/confirm follows libsignal's two-step dance — the caller hashes
each candidate locally (`Candidate.Hash`), submits hashes in priority
order, the server returns the one it will hold for ~5 minutes, and
the caller finalizes it by generating a zero-knowledge proof + a fresh
link ciphertext and PUTing them in one `confirm` round trip. Matches
Signal-Desktop and Signal-Android behavior.

Server-facing methods on `*signal.Client` use `internal/web` directly
rather than wrapping libsignal's `signal_authenticated_chat_connection_*`
promise FFI; keeps the signal-go architecture (direct REST over the
authenticated websocket) consistent and avoids importing async-promise
scaffolding for one feature. The REST surface is a stable published
interface on Signal-Server even though upstream clients have migrated
to the gRPC path.

Rejected alternatives:

1. *Expose the raw FFI only.* Users of `pkg/signal` should not import
   `internal/libsignal` for username ergonomics any more than they do
   for messaging; the thin ergonomic wrapper is where the Go-idiomatic
   shape lives.

2. *Fold into `pkg/signal` directly.* Usernames are a self-contained
   feature with its own types (`Candidate`, `Link`) that don't belong
   on `Client`. A subpackage keeps the public surface of `pkg/signal`
   narrow (per CLAUDE.md "One concept per package").

## Consequences

**Pro** — Bot operators can publish a username link as a stable public
contact handle, keeping the operator's phone number out of the
customer-facing surface.

**Pro** — Lookups let a bot resolve a username a user typed into an ACI
and start a conversation without any prior profile exchange.

**Pro** — Groundwork for [ADR 0042](./0042-paid-no-number-registration.md):
an account registered without a number *must* set a username before it
can be messaged by anyone who doesn't already have its ACI, so the two
land as a pair.

**Con** — Adds a public package (`pkg/signal/username`) to the API
surface area. Changes here are governed by the usual compat rules
once v0.1.0 ships.

**Con** — Username reserve/confirm uses the authenticated chat
connection; a bot process that reserves but crashes before confirming
leaves the hash reserved for ~5 minutes. Documented as an operator-
visible behavior, not something the wrapper tries to paper over.

**Con** — Lookup by username publishes the ACI to anyone who can guess
the nickname. This matches upstream Signal and is out of scope for
signal-go to fix; noted in `docs/security/threat-model.md`.
