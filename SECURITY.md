# Security Policy

This file is the GitHub-conventional landing page for security reports.
The substance lives in [docs/security.md](./docs/security.md).

## Reporting a vulnerability

**Please do not open public GitHub issues for security problems.**

- E-mail: `security@thehappydinoa.example` *(placeholder — update before public release)*
- PGP fingerprint: *(placeholder — paste your key fingerprint here)*

We aim to acknowledge within 72 hours and triage within a week.

## Supported versions

`signal-go` is pre-alpha. There are no supported versions yet; all
reports go against the `main` branch HEAD until we cut `v0.1.0`.

After `v0.1.0` lands we'll publish a support matrix here.

## Scope

In scope: anything in this repository — our Go code, our cgo boundary,
our wire-protocol implementation, the build pipeline, the on-disk
storage format.

Out of scope: bugs in upstream [`libsignal`](https://github.com/signalapp/libsignal)
itself (report those to Signal directly), bugs in the Signal service,
or bugs in your own application that happens to embed us.

For the full threat model see [docs/security.md](./docs/security.md)
and [docs/adr/0011-security-audit.md](./docs/adr/0011-security-audit.md).
