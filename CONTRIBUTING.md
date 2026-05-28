# Contributing to signal-go

Thanks for your interest. This project welcomes focused PRs that match the roadmap and conventions below.

## Before you code

1. Read [`CLAUDE.md`](./CLAUDE.md) — style, ADRs, docs, and roadmap rules
   apply to every contributor (human or agent).
2. Read [`docs/guides/getting-started.md`](./docs/guides/getting-started.md)
   for build prerequisites (`libsignal`, cgo, Windows MSYS2 notes).
3. Skim [`ROADMAP.md`](./ROADMAP.md) so your change fits an open phase item.

## Local checks

```sh
task setup      # once: tools + git hooks
task libsignal  # once: libsignal_ffi.a
task test
task lint
```

The [pre-push hook](./.githooks/pre-push) runs vet → lint → test when
`libsignal_ffi.a` is present.

## The cgo/libsignal boundary

`internal/libsignal/` is the highest-complexity area in the codebase. Read
[`internal/libsignal/doc.go`](./internal/libsignal/doc.go) before changing
any file there. The short version:

**Ownership.** Every `*C.T` returned by libsignal is either *owned* (call
`signal_T_destroy` exactly once) or *borrowed* (do not free it). The
ownership of every type is documented in `doc.go`. When adding a new wrapper:

1. Find the matching `signal_T_destroy` in
   `internal/libsignal/include/signal_ffi.h`.
2. Call it inside a `runtime.SetFinalizer` on the Go wrapper struct.
3. Make `Destroy` idempotent — use `sync/atomic` to guard the first-free path
   (see `CiphertextMessage` for the pattern).

**No Go pointers into Rust.** cgo's rules forbid passing a Go pointer to C if
the pointed-to value itself contains a Go pointer. All persistent callbacks
use `cgo.Handle` (an integer key into a Go-side map) rather than raw Go
pointers.

**`runtime.KeepAlive`.** Any Go value whose address is passed to a C function
must have `runtime.KeepAlive(val)` called *after* the C call returns. Without
it the GC may collect the value mid-call. Search `keepAlive` in the package
for examples.

**No cgo in tests.** The Go toolchain rejects cgo in `*_test.go` files of
cgo-using packages. To test callback-heavy logic, factor the body into a
Go-typed `*Impl` struct and test that directly — see `stores.go` ↔
`stores_impl.go` for the pattern.

**Checklist for a new libsignal wrapper:**

- [ ] `signal_T_destroy` found and called from `SetFinalizer`
- [ ] `Destroy` is idempotent (`sync/atomic` guard)
- [ ] `runtime.KeepAlive` after every C call that borrows the pointer
- [ ] Callbacks use `cgo.Handle`, not raw Go pointers
- [ ] Test targets the Go-typed `*Impl`, not the cgo wrapper
- [ ] `doc.go` ownership table updated

## Bumping the libsignal pin

The pin lives in `scripts/build-libsignal.sh` (`LIBSIGNAL_VERSION`).

1. Change `LIBSIGNAL_VERSION` to the new tag.
2. Run `task libsignal FORCE=1` to rebuild `libsignal_ffi.a` and update
   `internal/libsignal/include/signal_ffi.h`.
3. **Diff `signal_ffi.h`** against the previous version (`git diff`). Every
   changed or removed function signature must be audited against its callers
   in `internal/libsignal/`.
4. Fix any callers that reference renamed/removed symbols.
5. Run `task test && task lint` to confirm no regressions.

The weekly [libsignal canary](./.github/workflows/libsignal-canary.yml)
workflow flags when a new upstream release is available and surfaces the
`signal_ffi.h` diff automatically, so bumps are rarely a surprise.

## Releases (maintainers)

See [`docs/guides/releasing.md`](./docs/guides/releasing.md). Summary:

1. Add a `## [x.y.z] - date` section to `CHANGELOG.md`.
2. Merge to `main`.
3. Run **Actions → Create release tag** (workflow `create-release-tag.yml`).
4. When **Release** finishes, publish the draft GitHub Release.

Do not push release tags from a laptop unless you know you are duplicating
what the workflow already does.

## Pull requests

- Rebase onto `main`; keep PRs focused (one concept when possible).
- Update **docs**, **ROADMAP** tick-boxes, and **CHANGELOG** `[Unreleased]`
  when behavior or user-visible flags change.
- File an **ADR** when a future contributor could reasonably second-guess
  the decision ([`docs/adr/`](./docs/adr/)).
- Do not add runtime Go dependencies without updating
  [ADR 0002](./docs/adr/0002-no-third-party-go-deps.md).

Use the PR template at [`.github/pull_request_template.md`](./.github/pull_request_template.md)
to ensure docs, ADR, roadmap, and validation details are included.

## Community guidelines

- Conduct expectations: [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md)
- Support channels and response targets: [`SUPPORT.md`](./SUPPORT.md)
- Maintainer model and decision process: [`GOVERNANCE.md`](./GOVERNANCE.md)

## Security

Report vulnerabilities per [`SECURITY.md`](./SECURITY.md). Do not open public
issues for exploit details.

## License

By contributing, you agree your work is licensed under the project's
[AGPL-3.0-only](./LICENSE) terms, consistent with `libsignal`.
