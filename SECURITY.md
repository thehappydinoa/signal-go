# Security Policy

This file is the GitHub-conventional landing page for security reports.
For the full threat model see
[`docs/security/threat-model.md`](./docs/security/threat-model.md) and
[`docs/security.md`](./docs/security.md).

## Reporting a vulnerability

**Please do not open public GitHub issues for security problems.**

Preferred reporting channels, in order:

1. **GitHub Private Vulnerability Reporting** — open a private advisory
   at <https://github.com/thehappydinoa/signal-go/security/advisories/new>.
   This is the path we prefer; it gives you GitHub-mediated tracking
   and audit trail without exposing the report to anyone outside the
   maintainer team.
2. **E-mail**: <signal-go-security@thehappydinoa.dev>. PGP is
   available on request — drop us a line first and we'll exchange
   keys before you send the report body. (No published PGP fingerprint
   yet; once one is, this line will list it.)

Whichever channel you use, include:

- A description of the bug and its impact.
- Reproduction steps or a proof-of-concept (PoC). Test corpora that
  trigger panics in the public fuzz targets
  (`go test -fuzz=...`) are particularly useful — please attach the
  failing-input file.
- The signal-go commit or tag you're testing against.
- Any suggested mitigation or patch.

## Response targets

- **Acknowledgement** within 72 hours of report receipt.
- **Triage decision** (severity + fix scope) within 7 calendar days.
- **Fix landed in `main`** within 30 calendar days for high-severity
  reports; lower severities follow the normal PR cycle.
- **Coordinated disclosure** — we will agree a public-disclosure date
  with you. Default is 90 days from triage, or sooner if the fix has
  been shipped to a tagged release and a meaningful fraction of users.

## Supported versions

`signal-go` is pre-alpha. There are no supported versions yet; all
reports go against the `main` branch HEAD until we cut `v0.1.0`. After
`v0.1.0` lands we will publish a support matrix here listing the active
minor branches and their EOL dates.

## Scope

In scope: anything in this repository — our Go code, our cgo boundary,
our wire-protocol implementation, the build pipeline, the on-disk
storage format. The internal-review checklist that defines "in scope"
operationally is [ROADMAP § Phase 8](./ROADMAP.md#phase-8--security-audit-planned-required-before-v010).

Out of scope (please report directly to upstream):

- Bugs in upstream [`libsignal`](https://github.com/signalapp/libsignal)
  itself. signal-go statically links libsignal at a pinned tag; any
  bug originating below the cgo wrappers should go to Signal.
- Bugs in the Signal service (`chat.signal.org`, CDN, CDSI).
- Bugs in your own application that happens to embed signal-go, unless
  they stem from a documented misuse of our public API.

## Safe harbour

We will not pursue legal action against good-faith security research
that:

- Does not violate Signal's terms of service or the AGPL-3.0 license
  this project ships under.
- Does not access, modify, or exfiltrate data belonging to anyone but
  the researcher.
- Respects the disclosure timelines above.

If in doubt, ask first via the private channels above.
