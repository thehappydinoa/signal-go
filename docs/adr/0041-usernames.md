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

Add a new public package `pkg/signal/username` and a small
`internal/username` cgo wrapper layer around the libsignal calls listed
above. The public shape:

```go
package username

type Candidate struct { Nickname string; Discriminator uint64 }

// Local helpers — no network.
func Candidates(nickname string, minLen, maxLen int) ([]Candidate, error)
func Verify(candidate Candidate, proof []byte) error

// Bound to a Client (uses its authenticated chat connection).
func (c *signal.Client) ReserveUsername(ctx, []Candidate) (reserved Candidate, token []byte, err error)
func (c *signal.Client) ConfirmUsername(ctx, token []byte) error
func (c *signal.Client) DeleteUsername(ctx) error
func (c *signal.Client) SetUsernameLink(ctx) (handle uuid.UUID, link []byte, err error)
func (c *signal.Client) DeleteUsernameLink(ctx) error

// Discovery — uses an unauthenticated chat connection.
func (c *signal.Client) LookupUsernameHash(ctx, hash []byte) (aci string, err error)
func (c *signal.Client) LookupUsernameLink(ctx, handle uuid.UUID) (encryptedUsername []byte, err error)
func DecryptUsernameLink(link, encryptedUsername []byte) (string, error)
```

Reserve/confirm follows libsignal's two-step dance — the caller picks
one of the candidates returned by `Candidates` (local, deterministic),
asks the server to reserve its hash, confirms with the server-returned
token, and only then is the username live on the account. This matches
Signal Desktop and Android.

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
