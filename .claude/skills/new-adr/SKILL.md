---
name: new-adr
description: Create a new Architecture Decision Record (ADR) for signal-go. Use this skill whenever the user mentions writing an ADR, filing an ADR, "should this have an ADR", adding a new decision record, or when any CLAUDE.md rule triggers an ADR (new Go dep, store format change, security trade-off, new protocol, new package). Also invoke proactively when the user is about to make a change that the CLAUDE.md "When to ask first" list covers.
user-invocable: true
---

# Create a new ADR

ADRs are how signal-go records decisions that future contributors could
reasonably second-guess. They're cheap to write and expensive to skip —
a missed ADR means the same argument gets re-litigated in code review six
months later.

## Step 1 — Find the next number

Read the last row of `docs/adr/README.md` to find the highest existing
ADR number, then add 1. The number must be zero-padded to 4 digits (e.g.
`0038`).

```bash
tail -5 docs/adr/README.md
```

If the user is superseding an existing ADR, note which one — you'll link
back to it in the Consequences section.

## Step 2 — Draft the title

A good ADR title is short (≤ 60 chars), uses a noun phrase, and says
what the decision *is*, not just the problem. "SQLite-backed store
(modernc.org/sqlite)" is better than "Database for key storage".

Confirm the title with the user before creating the file.

## Step 3 — Create the file

Path: `docs/adr/NNNN-kebab-case-title.md`

Use this exact template:

```markdown
# ADR NNNN — [Title]

- Status: Proposed
- Date: YYYY-MM-DD

## Context

[Why is this decision being made? What forces are at play? What
constraints exist? Keep this factual — describe the situation, not
the solution.]

## Decision

[What are we doing? Be concrete. If there were alternatives considered,
briefly describe them and why this option won.]

## Consequences

**Pro** — [benefit]

**Pro** — [benefit]

**Con** — [drawback or trade-off]

[If this supersedes an ADR, add:]
Supersedes [ADR NNNN](./NNNN-title.md).
```

Fill in today's date for the Date field. Status starts as "Proposed";
change it to "Accepted" when the PR merges.

## Step 4 — Update docs/adr/README.md

Add a row to the table:

```markdown
| NNNN | [Title]                   | Proposed |
```

Keep the table sorted by ADR number (ascending). The status in the table
should match the file.

## Step 5 — Link from the relevant code

The CLAUDE.md "What goes where" table specifies which code/docs to link
from. Common cases:

| Change | Link target |
|--------|-------------|
| New Go dep | `go.mod` comment or ADR 0002 table |
| New `internal/` package | package `doc.go` |
| Public `pkg/signal` change | function/type doc comment |
| Wire format change | `internal/store/` file header |
| Security/crypto change | `docs/security.md` or `docs/security/threat-model.md` |

Add a reference like `// Design: ADR NNNN` or a link in the relevant doc.

## Step 6 — Help draft the content

Offer to help the user write the Context and Decision sections if they
haven't already. Good context sections:

- Name the concrete problem, not the abstract category
- Call out relevant existing ADRs that this interacts with
- Include a short note on why the obvious alternative was rejected (even
  if it's just "we considered X but Y")

Good decision sections:

- Say *what* not just *why*
- Name specific packages, files, or protocols where relevant
- If there were multiple options, number them and say which won

## CLAUDE.md triggers requiring an ADR

These changes always need an ADR per `CLAUDE.md`:

- Adding any runtime Go dependency → must also update ADR 0002 table
- Changing the on-disk store wire format → ADR 0012 + bump format version
- New network endpoint or capability flag
- Any security check loosened (validation order, constant-time, error path)
- Backwards-compatibility break in `pkg/signal` after the first tagged release
- New cryptographic primitive or protocol

$ARGUMENTS
