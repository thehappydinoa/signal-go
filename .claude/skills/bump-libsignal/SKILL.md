---
name: bump-libsignal
description: Guide through a libsignal pin bump in signal-go. Use this skill whenever the user mentions bumping libsignal, updating the libsignal version, a new libsignal release, signal_ffi.h changes, or the canary workflow flagging a new upstream release. This is a high-risk operation — invoke this skill to make sure no steps are missed.
user-invocable: true
---

# Bump the libsignal pin

Bumping the libsignal pin is the highest-risk routine maintenance task in
signal-go. The cgo boundary (`internal/libsignal/`) calls into Rust via a
C header (`signal_ffi.h`) that libsignal regenerates on every release.
A missed signature change → undefined behavior at runtime; a missed
`Destroy` rename → memory leak or double-free. This skill makes the
process guided so nothing gets skipped.

## Step 0 — Know what you're bumping to

If you're here because the libsignal canary workflow fired, the target
version is in the workflow run output. Otherwise:

```bash
# Current pin:
grep LIBSIGNAL_VERSION scripts/build-libsignal.sh

# Latest upstream release:
gh release list --repo signalapp/libsignal --limit 5
```

Confirm the target tag with the user before proceeding.

## Step 1 — Update the pin

Edit `scripts/build-libsignal.sh`, line:

```bash
LIBSIGNAL_VERSION="${LIBSIGNAL_VERSION:-vX.Y.Z}"
```

Change the default to the new tag. This is the single source of truth
used by `task libsignal`, `scripts/download-libsignal.sh`, and CI.

## Step 2 — Rebuild and update the header

```bash
task libsignal FORCE=1
```

This rebuilds `libsignal_ffi.a` and overwrites
`internal/libsignal/include/signal_ffi.h` from the new source. If
pre-built artifacts are available for the new tag, `task libsignal` will
download them instead of building from Rust source — either way, the
header gets updated.

## Step 3 — Audit the signal_ffi.h diff (critical)

```bash
git diff internal/libsignal/include/signal_ffi.h
```

For every changed or removed function signature in the diff:

1. **Search for callers** in `internal/libsignal/`:
   ```bash
   grep -rn "signal_function_name" internal/libsignal/
   ```
2. **Changed signature**: Update the call site(s) to match — parameter
   types, return type, calling convention.
3. **Removed function**: Find the caller and update to the new API
   (check the libsignal changelog or the commit that removed it for
   the replacement).
4. **New `signal_T_destroy` function**: This means a new type was added.
   You probably don't need to wrap it unless you're adding a feature,
   but note it.

The diff is the contract. Don't skip any line.

After auditing, rebuild to confirm the cgo compilation is clean:

```bash
go build ./internal/libsignal/...
```

## Step 4 — Run the full test suite

```bash
task test    # go test -race -count=1 ./...
task lint    # golangci-lint
go vet ./...
```

If any test fails, diagnose before continuing — a "works without race
detector" failure is still a failure.

## Step 5 — Update the changelog

Add an entry under `[Unreleased]` in `CHANGELOG.md`:

```markdown
### Changed
- Bump libsignal to vX.Y.Z ([compare](https://github.com/signalapp/libsignal/compare/vOLD...vNEW))
```

If the new libsignal version changed any API that signal-go exposes in
`pkg/signal`, note the impact for users.

## Step 6 — Commit

```bash
git add scripts/build-libsignal.sh \
        internal/libsignal/include/signal_ffi.h \
        internal/libsignal/lib/libsignal_ffi.a \  # if committed
        CHANGELOG.md
git commit -m "chore: bump libsignal to vX.Y.Z"
```

The commit body should summarize what changed in `signal_ffi.h` (e.g.,
"No signature changes" or "Renamed `signal_foo_new` → `signal_foo_create`;
updated callers in decrypt.go").

## Common failure modes

| Symptom | Likely cause |
|---------|-------------|
| `cgo: cannot find signal_ffi.h` | `task libsignal` did not run or FORCE=1 not set |
| `undefined: C.signal_foo_new` | Removed function in new header; find replacement |
| test panics with SIGSEGV under race detector | `keepAlive` missing after a cgo call; check callers |
| `task libsignal` downloads fail | Pre-built artifacts not yet published for new tag; run with `SKIP_DOWNLOAD=1` to build from Rust source instead |

## What NOT to do

- Don't skip the `git diff` on `signal_ffi.h`. Even a "minor" libsignal
  bump can rename destroy functions. The canary diff output is a starting
  point, not a substitute for reading the actual diff.
- Don't commit `libsignal_ffi.a` if the repo gitignores it (check `.gitignore`).
  The artifacts workflow handles distributing the `.a`.
- Don't bump without running `-race` tests. ThreadSanitizer catches cgo
  use-after-free patterns that the regular test run misses.

$ARGUMENTS
