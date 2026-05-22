# Architecture Decision Records

Each significant choice gets a short, dated record. Format follows
[Michael Nygard's template](https://github.com/joelparkerhenderson/architecture-decision-record/blob/main/locales/en/templates/decision-record-template-by-michael-nygard/index.md).

Records are immutable once **Accepted**. To change a decision, supersede it
with a new ADR that links back to the one it replaces.

| #    | Title                                          | Status   |
|------|------------------------------------------------|----------|
| 0001 | Overall architecture: cgo to libsignal         | Accepted |
| 0002 | No third-party Go runtime dependencies         | Accepted |
| 0003 | Vendor `.proto` from Signal-Android            | Accepted |
| 0004 | Pin libsignal to a fixed upstream tag          | Accepted |
| 0005 | Storage interface + filesystem reference impl  | Accepted |
| 0006 | Testing strategy                               | Accepted |
| 0007 | Use Taskfile (not Make) for orchestration      | Accepted |
| 0008 | Bot framework: `pkg/bot` API sketch            | Accepted |
| 0009 | Licensing: AGPL-3.0-only (libsignal-driven)    | Accepted |
| 0010 | Phase 3: authenticated receive pipeline        | Accepted |
| 0011 | Security audit methodology + threat model      | Accepted |
| 0012 | Encrypted account store on disk                | Accepted |
| 0013 | GitHub Actions CI                              | Accepted |
| 0014 | Send retry and multi-device fan-out strategy   | Accepted |
| 0015 | Sealed-sender encrypt: per-device USMC + cert cache | Accepted |
| 0016 | Control messages, reactions, and edits         | Accepted |
| 0017 | Profile fetch and UAK derivation               | Accepted |
| 0018 | Groups v2 bootstrap: fetch + decrypt state     | Accepted |
| 0019 | Groups v2 sender-key distribution + group send | Accepted |
| 0020 | Group send endorsements + membership changes   | Accepted |
| 0021 | Group control messages (react, typing, receipts) | Accepted |
| 0022 | Profile-key presentations, add/remove member, wizard | Accepted |
| 0023 | Persistent GSE cache + invite-link join          | Accepted |
| 0024 | Group log sync (snapshot-based)                  | Accepted |
| 0025 | Inbound group update events + optional auto-sync | Accepted |
| 0026 | Attachment cipher (classic AES-CBC + HMAC)       | Accepted |
| 0027 | Storage Service sync (pull-only v1)              | Accepted |
