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
