# ADR 0042 — Paid phone-numberless primary registration

- Status: Proposed
- Date: 2026-11-14

## Context

ROADMAP.md states as a non-goal:

> We will not target headless registration (creating a brand-new Signal
> account by phone number). Only linking as a secondary device.

That non-goal was written when the only way to get a primary Signal
account was SMS verification of a phone number. For a headless Go bot
that meant: hold a SIM, receive SMS, deal with carriers and verification
anti-abuse — a lot of trust surface and operator pain for a path whose
output (an account tied to a disposable or operator-personal number)
had limited value.

Signal has since added a *paid* primary-device registration variant that
does **not** require a phone number. The account is bound to a
subscription entitlement instead, and is reachable only by ACI, by
[username](./0041-usernames.md), or via a username link. libsignal
v0.108.0 exposes this through
`signal_registration_service_register_account_without_number` (and
`_reregister_account_without_number` for recovery), with the
accompanying session lifecycle:
`signal_registration_service_registration_session`,
`signal_registration_service_request_verification_code`,
`signal_registration_service_submit_verification_code`,
`signal_register_account_request_create`, and the response accessors
for ACI, PNI, backup entitlements (`entitlement_backup_level`,
`entitlement_backup_expiration_seconds`), `entitlement_badges`,
and `storage_capable`.

The forces that made the 2024 non-goal correct have shifted:

1. The SMS / SIM trust surface is gone.
2. Discoverability is opt-in through a username, not an implicit phone
   number leak.
3. Entitlement verification is a server-side concern we don't
   re-implement — we just pay Signal and surface the resulting ACI.
4. The upstream FFI is production-complete; the historical
   "registration is a vendor concern" argument is weaker when
   libsignal itself ships the Rust implementation we would consume.

This ADR revisits the non-goal for the *paid, no-number* variant only.
Classic SMS-verified primary registration remains out of scope.

## Decision

Add primary registration via `signal_registration_service_register_account_without_number`
as a new public API `pkg/signal/register.Primary(ctx, Options) (*Account, error)`,
mirroring the shape of existing `pkg/signal/link.LinkDevice`. The
`Options` carry:

- a subscription credential proving paid entitlement (opaque bytes,
  obtained out-of-band — signal-go does not implement billing);
- the pre-generated identity key pair and signed / Kyber pre-keys the
  session will use;
- an account entropy pool for the SVR master key (same flow linked
  accounts use today — `libsignal.GenerateAccountEntropyPool` plus the
  SVR derive helpers in `internal/libsignal/account_entropy.go`);
- `DeviceTransferSkip` / `BackupRestore` toggles.

The flow:

1. `signal_registration_service_registration_session` with no phone
   number returns a session bound to the paid credential.
2. `signal_registration_service_register_account` (the no-number
   variant) submits the identity bundle and receives an
   `RegisterAccountResponse`; the ACI / PNI / entitlements are copied
   out into an `Account` the caller persists via
   [`account.Store`](../store.md) exactly as today's linked accounts
   are.
3. The account is live but unreachable by anyone who doesn't already
   have its ACI until the operator sets a username. The
   `register.Primary` doc comment says so; the `wizard-bot` example
   grows a "pick a username" step that chains into
   [ADR 0041](./0041-usernames.md)'s surface.

ROADMAP.md is amended: the non-goal becomes "SMS-verified primary
registration" rather than "any primary registration."

Rejected alternatives:

1. *Keep the non-goal as written.* Doesn't reflect that the paid flow
   has a materially different trust model from SMS; rejecting it
   leaves bot operators with no way to run a Signal bot without
   someone's personal phone number.

2. *Also implement SMS-verified primary registration.* The anti-abuse
   dance (captcha, rate limits, SMS/voice dual channel, carrier push
   challenges) is a long tail of UX that doesn't belong in a headless
   library. Keep that path blocked; if someone wants it, they build
   on top.

3. *Expose a thin FFI wrapper only.* The registration flow is
   multi-step with required ordering; an ergonomic `Primary(ctx, opts)`
   call keeps the invariants (identity key generated before session,
   session created before account, account persisted before close) in
   one place and testable.

## Consequences

**Pro** — A headless signal-go bot can run on an account that was never
tied to a phone number, closing the gap between "bot framework" and
"operator-grade deployment". Operators stop needing a SIM.

**Pro** — Removes a recurring footgun: using a personal number for a
bot account mingles personal reachability with service reachability.
The paid no-number path cleanly separates them.

**Pro** — Lets us exercise libsignal's registration path in our test
suite (against the staging environment), which catches upstream
regressions that linking tests miss.

**Con** — Expands the audit surface. New code paths persist identity
material and talk to a new set of server endpoints; the
[Phase 8 audit checklist](../../ROADMAP.md#phase-8--security-audit-internal-pass-done-external-pass-required-before-v010)
grows.

**Con** — Entitlement handling is opaque to signal-go — we hand
libsignal a credential and trust the server's response. If Signal
changes its entitlement format, that becomes a protocol we have to
track like we track the message wire format.

**Con** — The ROADMAP amendment opens a conversation about SMS primary
registration that we would rather not have. Decision stays "no SMS".
Noted for reviewers.

**Con** — No end-to-end CI coverage against paid entitlements (we
don't have a test Signal subscription). Component-level coverage
against a fake registration server has to carry the weight; the
Phase 8 external audit is the backstop.

Related: [ADR 0041](./0041-usernames.md) (usernames — paid no-number
accounts are effectively unreachable without one).
