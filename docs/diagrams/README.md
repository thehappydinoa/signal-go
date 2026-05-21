# Diagrams

Visual references for the moving parts of `signal-go`. Every diagram is
Mermaid; GitHub renders them inline, or paste them into
[mermaid.live](https://mermaid.live) to edit.

| Diagram | What it shows |
|---|---|
| [Architecture](./architecture.md) | Package layering: public API → protocol → cryptography → persistence |
| [QR-link flow](./qr-link.md) | The full secondary-device pairing sequence end-to-end |
| [Encrypted store](./encrypted-store.md) | How the on-disk credentials are sealed under AES-256-GCM |
| [Receive pipeline](./receive-pipeline.md) | Planned envelope dispatch + libsignal callbacks (Phase 3) |
| [Send flow](./send-flow.md) | Planned 1:1 send pipeline (Phase 4) |

If you want to add one, copy the structure of an existing file: H1
title, a short one-paragraph context, the fenced ```mermaid block, then
a short "what to look at" list under the diagram. Keep diagrams small —
if it's hard to take in, split it.
