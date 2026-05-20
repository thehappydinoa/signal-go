# ADR 0009 — Licensing implications of linking libsignal

- Status: Accepted
- Date: 2026-05-20

## Context

[`signalapp/libsignal`](https://github.com/signalapp/libsignal) is licensed
**AGPL-3.0-only**. Every header and source file carries
`SPDX-License-Identifier: AGPL-3.0-only`.

We link `libsignal_ffi.a` statically into the `signal-go` binary (ADR 0001),
and we vendor the cbindgen-generated `signal_ffi.h` into this repo (ADR 0004).
Both qualify as creating a "combined work" under AGPL §1 / §13, which
extends the AGPL obligations to the combined whole — including the network-use
clause (§13) for any service that exposes the combined work to users over a
network.

This is the same constraint that would apply if we had vendored signalmeow
(also AGPL-3.0). The user's stated reason for not vendoring signalmeow was
to avoid *unvetted third-party Go code* (ADR 0002), not to avoid AGPL —
which is unavoidable here regardless.

## Decision

- License `signal-go` under **AGPL-3.0-only** to be compatible with libsignal.
- Add a top-level `LICENSE` file containing the AGPL-3.0 text.
- Add a `NOTICE` file crediting libsignal and noting the static-link.
- Document in `README.md` that AGPL-3.0 applies to any work built on
  signal-go, including network services.

## Consequences

- **Pro**: License-compatible with upstream. No lawyer-bait.
- **Pro**: Aligns with how every other Signal client distributes
  (Signal-Desktop, Signal-iOS, Signal-Android, signal-cli, signalmeow are
  all AGPL or GPL-family).
- **Con**: Anyone embedding `signal-go` in a closed-source SaaS triggers
  the network-use clause; they must offer source to their users. Mitigation:
  document loudly in `README.md`.
- **Con**: Some downstream consumers will need to seek a commercial
  re-license from Signal itself (which Signal does not offer to the
  public). signal-go cannot help with this.

## Alternatives considered

1. **Dynamic-link libsignal instead of static-link.** Does not escape the
   combined-work argument under AGPL §13. Rejected.
2. **Ship `signal-go` under MIT/Apache and rely on the "mere aggregation"
   exception.** Static linking is not mere aggregation under any common
   reading; signalapp's own clients license accordingly. Rejected.
3. **Wrap signal-cli as a subprocess (back to ADR 0001 option 2).** Avoids
   linking libsignal directly, at the cost of pulling in the JVM. Already
   considered and rejected in ADR 0001.
