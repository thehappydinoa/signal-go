# Changelog

All notable changes to **signal-go** are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) from `v0.1.0`
onward. Pre-1.0 tags may break API without a major bump.

Architectural *why* lives in [`docs/adr/`](./docs/adr/README.md); this file
is *what* changed and *when*.

## [Unreleased]

### Fixed

- **[Security]** `GroupSecretParamsFromMasterKey` and
  `GroupSecretParamsDecryptServiceID` (`internal/libsignal/zkgroup.go`) were
  each missing a `keepAlive` call on a `[]byte` argument after its last use
  in an FFI call, unlike every sibling function in the same file — found
  during the cgo-boundary review for the libsignal v0.99.3 bump below,
  pre-existing and unrelated to that bump itself. The GC could in principle
  reclaim/move the backing array while libsignal still held the borrowed
  pointer. Both now call `keepAlive` immediately after their last FFI use of
  the borrowed slice, matching the pattern documented in `doc.go`.

### Changed

- Bump libsignal to v0.99.3 ([compare](https://github.com/signalapp/libsignal/compare/v0.97.2...v0.99.3)).
  The largest `signal_ffi.h` diff to date (9,212 lines vs. ~436 for the
  previous bump) — cbindgen itself was upgraded upstream, regenerating the
  entire header in a new style. Required real `internal/libsignal/` changes,
  unlike the last several bumps:
  - The header no longer transitively includes `<stdlib.h>` (it now only
    pulls in `<assert.h>`, `<stdalign.h>`, `<stdbool.h>`, `<stddef.h>`,
    `<stdint.h>`). Five files that called `C.free`/`C.malloc` while relying
    on that transitive include (`account_entropy.go`, `cdsi.go`,
    `connection_manager.go`, `lookup_request.go`, `profile_key.go`) now
    `#include <stdlib.h>` directly.
  - Every C string parameter (`SignalCStringPtr`, and raw `const char *`
    params) changed from a `char`-based type to `const int8_t *`. Go's cgo
    treats `char` and `int8_t` as distinct types despite the identical ABI,
    so every `*C.char` obtained from `C.CString` and passed into an FFI call
    now needs an explicit `(*C.int8_t)(unsafe.Pointer(...))` hop; the reverse
    (`C.GoString`/`C.signal_free_string` on a returned `SignalCStringPtr`)
    needs the same treatment in the other direction.
  - cbindgen stopped emitting the ~17 `#define Signal*_LEN` numeric
    constants (`SignalBACKUP_KEY_LEN`, `SignalPROFILE_KEY_LEN`,
    `SignalGROUP_SECRET_PARAMS_LEN`, etc.) entirely, expressing fixed-width
    buffers via unnamed `SignalType_FixedArrayN_uint8_t` typedefs instead.
    Every one of signal-go's `C.Signal*_LEN` references was replaced with
    the equivalent hardcoded Go integer constant (verified against the
    `FixedArrayN` suffix on each affected function signature — no numeric
    value changed, only how it's spelled). These constants no longer
    compile-fail if upstream changes a size; re-verify on the next bump.
  - Named `SignalType_FixedArrayN_uint8_t` typedefs replaced anonymous
    inline C arrays (e.g. `uint8_t (*out)[32]`) for every fixed-width
    buffer parameter. On our GCC/Linux toolchain, cgo's DWARF-based type
    resolution unwraps `const`-qualified fixed-array *parameters* back to
    the plain `*[N]C.uint8_t` array type, but keeps the named
    `C.SignalType_FixedArrayN_uint8_t` type for non-const (`out`)
    parameters — so every `out`-direction buffer pointer (and the
    `c*Out`-suffixed helpers in `zkgroup.go` / `profile_key_presentation.go`
    that build them) needed an explicit
    `(*C.SignalType_FixedArrayN_uint8_t)(unsafe.Pointer(...))` cast, while
    `const`-direction (`*In`-suffixed) helpers were left unchanged. This
    same GCC-vs-clang DWARF asymmetry is already documented for
    `SignalServiceIdFixedWidthBinaryBytes` in
    `service_id_cgo_typedef_default.go` / `_darwin.go`; that split needed no
    changes here since chained typedefs (a typedef of a typedef, as opposed
    to a typedef of an anonymous array) resolve the same on both
    toolchains.
  - `bridge_async.c` (our own C shim for the async CDSI lookup bridge) casts
    its `const char *username`/`password` parameters to `const int8_t *`
    at the `signal_cdsi_lookup_new` call site rather than changing its own
    signature, so `cdsi.go`'s Go-side call needed no change.
  - Also additive/unrelated-to-us: 9 media-sanitizer functions removed
    (`signal_mp4_sanitizer_sanitize`, `signal_webp_sanitizer_sanitize`,
    `signal_sanitized_metadata_*`, `signal_connection_info_destroy` renamed
    to `signal_chat_connection_info_destroy`) — none referenced by
    signal-go; 19 new functions added (chat-connection registration-lock
    and username management, backup-media delete streaming, SVR key
    derivation helpers, zkc auth-credential-without-PNI variants) that
    signal-go does not wrap yet.
  - Verified: `go build ./...`, `go vet ./...`,
    `go test -race -count=1 ./...` (full suite, all packages pass),
    `golangci-lint run` (same 2 pre-existing `prealloc` findings in
    `pkg/signal/group_{endorsement,send}.go` as the last bump, unrelated to
    this change).

- Bump libsignal to v0.97.2 ([compare](https://github.com/signalapp/libsignal/compare/v0.96.4...v0.97.2)).
  No `internal/libsignal/` wrapper changes were required — everything
  signal-go currently calls compiled and passed `go test -race` unchanged.
  Notable upstream changes, none of which touch existing signal-go call
  sites: `SignalMEDIA_ENCRYPTION_KEY_LEN` is now derived from two new named
  constants (`SignalMEDIA_ENCRYPTION_AES_KEY_LEN` / `_HMAC_KEY_LEN`, same
  total of 64 bytes); a new error code
  (`SignalErrorCodeUsernameNotSet`); the `SignalUnwindSafeArgSignalFfiError`
  typedef was removed in favor of using `const SignalFfiError *` directly
  (same underlying C type, so `internal/libsignal/errors.go` needed no
  change beyond a stale comment); and a new, currently-unused
  `signal_copy_backup_media_stream_*` / `signal_authenticated_chat_connection_*`
  device-management and push-token API surface (backup media copy,
  `get_devices`, `remove_device`, `set_push_token_apns`,
  `clear_push_token`, `set_username_link`) that signal-go does not wrap yet.

- Bump libsignal to v0.96.4 ([compare](https://github.com/signalapp/libsignal/compare/v0.96.0...v0.96.4)).
  cbindgen introduced `SignalCStringPtr` (a typedef alias for `const char *`) and
  standardized all string parameters to use it. This is a source-level change only —
  the ABI is identical, so no `internal/libsignal/` wrapper changes are required.
  Additive changes: two new error codes (`SignalErrorCodeDeviceIdNotFound`,
  `SignalErrorCodeUsernameNotAvailable`), new types (`SignalCPromiseu832`,
  `SignalBorrowedSliceOfu832`, `SignalOwnedBufferOfMaxAlignedc_void`), and new
  functions for username reservation, device-name update, and donation permits.

- Bump libsignal to v0.96.0 ([compare](https://github.com/signalapp/libsignal/compare/v0.94.4...v0.96.0)).
  Purely additive: 19 new FFI functions across two feature areas —
  `signal_avatar_upload_credential_*` (avatar ZK credential flow) and
  `signal_zk_credential_key_pair_*` / `signal_zk_credential_public_key_*`
  (ZK credential key pair generation and public key management). No existing
  signatures were removed or modified; no internal/libsignal wrapper changes
  are required.

- Bump libsignal to v0.94.4 ([compare](https://github.com/signalapp/libsignal/compare/v0.94.1...v0.94.4)).
  cbindgen renamed the generated `SignalPairOfc_char*`/`SignalOptionalPairOfc_char*`
  helper types to `SignalPairOfCStringPtr*`/`SignalOptionalPairOfCStringPtr*` and
  changed several `const char **out` parameters (e.g. `signal_address_get_name`,
  `signal_error_get_message`, `signal_account_entropy_pool_generate`,
  `signal_service_id_service_id_string`,
  `signal_sender_certificate_get_sender_uuid`,
  `signal_message_backup_validation_outcome_get_error_message`) to the new
  `SignalCStringPtr *out` alias; updated the affected `internal/libsignal`
  wrappers to declare `C.SignalCStringPtr` locals and convert to `*C.char`
  before calling `C.GoString`/`C.signal_free_string`. New FFI surface adds
  backup-related promises (`signal_unauthenticated_chat_connection_backup_*`)
  that we do not yet wrap. The testing-only
  `signal_connection_manager_internal_testing_set_reflector_proxy` added in
  v0.94.3 was removed upstream; it was never wrapped.

### Added

- `OpenOptions.AutoMarkRead` — when set, automatically sends a READ receipt to
  the sender each time a `MessageEvent` is dispatched. Receipts are fire-and-forget
  and do not block the receive loop.
- `Client.SetExpireTimer(chatID, duration)` — manually set the disappearing-message
  timer for a conversation (ACI for 1:1, hex group master key for groups).
- Outbound `DataMessage` payloads (`Send`, `SendEdit`, `SendReaction`,
  `SendGroup`, `SendGroupReaction`) now include the `expire_timer` field when a
  timer is active for the conversation, so bot-sent messages respect the chat's
  configured disappearing-message setting.
- `Group.ExpireTimer` — new field on the fetched group snapshot exposing the
  group-level disappearing-message duration. `FetchGroup` populates the client's
  internal timer cache automatically, so `SendGroup` picks it up without extra
  configuration.
- Inbound `DataMessage` processing now updates the per-conversation expire timer
  cache whenever a message carries a non-nil `expire_timer` field (including 0 to
  clear the timer). Both 1:1 and group messages are handled.

- [`docs/guides/creating-a-bot.md`](./docs/guides/creating-a-bot.md) — step-by-step
  guide to linking a device, building a `pkg/bot` bot, groups, middleware, and
  deployment.
- `Client.CreateGroup` — create a new Groups v2 chat (`PUT /v2/groups/`) with the
  linked account as administrator; members without profile keys are added as
  pending invites ([ADR 0038](./docs/adr/0038-groups-v2-create.md)).
- `Client.SetGroupTitle`, `Client.SetGroupDescription`, `Client.EnableGroupInviteLink`,
  `Client.GroupInviteLinkURL`, and `FormatGroupInviteLink` for group metadata
  and invite links.
- `libsignal.GenerateGroupMasterKey` for fresh group master keys.

## [0.3.0] - 2026-05-28

### Added

- `signal.OpenFromStore` — opens a `Client` directly from an existing
  `store.Store` without re-linking, useful for callers that manage their own
  store lifecycle (`pkg/signal/open_store.go`).
- `bot.Match.InGroups(groupIDs...)`, `bot.ReactionMatch.InGroups`, and
  `bot.EditMatch.InGroups` — restrict handlers to specific group conversations
  by hex-encoded master key. DM messages always pass; chain
  `.Group().InGroups(...)` to restrict to group-only traffic.
- `libsignal-canary.yml` CI workflow — detects new upstream libsignal releases
  and opens a draft PR automatically.
- Docs-freshness check in CI (`scripts/check-docs-touched.sh`): warns when a PR
  touches source code without updating the corresponding documentation.

### Fixed

- **[Security]** `kdf.json` (Argon2id KDF salt for the sqlstore passphrase) is
  now written atomically via temp-file + rename, preventing permanent store
  corruption if the process crashes during first open.
- **[Security]** Sender certificate is now validated against Signal's production
  trust root on the outbound sealed-sender path, closing the gap that the
  receive path already covered (ADR 0015 Phase 8 audit item).
- **[Security]** PKCS-7 padding check in the backup decryptor is now
  constant-time (accumulate-and-XOR), consistent with the provisioning cipher.
- Auto group sync (`maybeAutoSyncGroupUpdate`) updates the cached revision
  monotonically — a slow-completing goroutine can no longer regress a fresher
  cached revision.
- Identity keys in `SignalStores` are now saved transactionally, preventing
  partial writes under concurrent updates.

## [0.2.0] - 2026-05-27

### Added

- Pre-built `libsignal_ffi.a` artifacts published under dedicated
  `libsignal-v*` GitHub Releases; `task libsignal` now downloads the
  correct platform artifact automatically — **no Rust or cargo required**
  for tagged releases ([ADR 0037](./docs/adr/0037-libsignal-prebuilt-artifacts.md)).
- `task libsignal:download` — download-only task; fails fast when no
  pre-built artifact exists rather than falling back to cargo.
- `go generate ./internal/libsignal/` bootstrap via
  `tools/libsignal_setup.go` (pure Go, no extra tools) for library
  consumers who do not have `task` installed.
- `scripts/download-libsignal.sh` — portable download helper with SHA256
  verification; called by `build-libsignal.sh` as a fast path.
- `.github/workflows/libsignal-artifacts.yml` — matrix workflow that
  builds and publishes pre-built `.a` + `.sha256` files on every libsignal
  version bump (triggers on changes to `scripts/build-libsignal.sh`).
- Three bot examples: `examples/middleware-bot` (middleware composition:
  logging, recovery, rate limiting), `examples/poll-bot` (group poll
  workflows using reactions), `examples/wizard-bot` (multi-stage signup
  conversation via `bot.Wizard`).
- Rate-limit retry middleware and local prekey-fetch rate limiting in
  `pkg/signal` client (avoids thundering-herd on `PUT /v2/keys`).
- CodeQL autobuild step for improved Go project instrumentation.

### Fixed

- Integer overflow in allocation size computation in
  `internal/libsignal` (CodeQL alert #1).
- Device name cipher KDF now matches Signal Android's `DeviceNameCipher`
  — correct synthetic IV derivation and key schedule
  ([ADR 0036](./docs/adr/0036-linked-device-name-cipher.md)).

### Changed

- CI workflow ignores markdown-only changes in pushes and PRs, avoiding
  unnecessary `libsignal_ffi.a` rebuilds on doc-only commits.

## [0.1.0] - 2026-05-27

### Added

- `internal/profile` and `echo-bot run -memprofile` / `-cpuprofile` for long-running
  heap/CPU soaks; guide: [`docs/guides/profiling.md`](./docs/guides/profiling.md)
  (Phase 8 bake results recorded 2026-05-27).
- `signal.Client.SendEdit` for 1:1 outbound edits (`Content.editMessage`).
- Encrypted linked-device display name at `PUT /v1/devices/link` (Android-compatible
  cipher; [ADR 0036](./docs/adr/0036-linked-device-name-cipher.md)).
- Optional `OnChatItem` on link-and-sync / `ImportTransferArchive` to stream
  transfer-archive `ChatItem` frames as protobuf bytes ([ADR 0031](./docs/adr/0031-transfer-archive-frame-import.md)).
- E2e test suite (`go test -tags=e2e`, `task test:e2e`): open, recv, send,
  and group management (`FetchGroup`, `SyncGroup`, optional `SendGroup`) against
  a linked `sqlstore` directory. Guide: [`docs/guides/testing-e2e.md`](./docs/guides/testing-e2e.md).
- Terminal QR for `signal-go link` via audited `github.com/skip2/go-qrcode`
  ([ADR 0035](./docs/adr/0035-go-qrcode-cli-qr.md)); `-no-qr` and `NO_COLOR`
  skip rendering.
- **Trigger Release for tag** workflow to start **Release** for an existing `v*`
  tag (recovery).

### Fixed

- Device linking now authenticates `PUT /v1/devices/link` with the account's
  e164 number as the HTTP Basic username (the provisioning code travels only in
  `verificationCode`), matching signal-cli / libsignal-service-java. Previously
  the provisioning code was sent as the username, which the server rejects.
- Linked-device capabilities now match signal-cli
  (`storage`, `versionedExpirationTimer`, `attachmentBackfill`, `spqr`). The
  previous set omitted `attachmentBackfill` and `spqr`, both of which
  Signal-Server requires for new devices, causing `PUT /v1/devices/link` to
  fail with HTTP 422 "Missing device capabilities".
- **Create release tag** now dispatches **Release** after the tag push. Pushes
  made with the default `GITHUB_TOKEN` do not trigger other workflows on GitHub.

## [0.1.0-rc2] - 2026-05-22

### Added

- **Create release tag** GitHub Actions workflow
  (`.github/workflows/create-release-tag.yml`): maintainer `workflow_dispatch`
  validates SemVer + `CHANGELOG.md`, pushes an annotated `v*` tag. Guide:
  [`docs/guides/releasing.md`](./docs/guides/releasing.md). *(Release dispatch
  fix landed after this tag — use **Trigger Release for tag** for `v0.1.0-rc2`.)*

### Changed

- Documentation pass: README, diagrams, security anchors, and release
  docs aligned with current feature set (groups v2, sealed sender, TLS
  pinning, `bin/signal-go` from `task build`).

## [0.1.0-rc1] - 2026-05-22

First tagged pre-release: cross-platform CLI binaries, Windows local-build
support, and TLS trust fixes for Signal's private CA.

### Added

- Cross-platform release pipeline ([ADR 0033](./docs/adr/0033-release-pipeline.md)):
  `.github/workflows/release.yml` builds `signal-go` on Linux (amd64/arm64),
  macOS (amd64/arm64), and Windows (amd64, experimental), packages archives
  + `.sha256`, and uploads to a draft GitHub Release on `v*` tag push.
  `workflow_dispatch` dry-run skips publish.
- `signal-go version` / `--version` (build tag, Go toolchain, VCS metadata).
- Windows local dev ergonomics: `.env.example`, `scripts/dev-env.sh`,
  `scripts/go.sh`, MinGW `fiat_p256_adx` link stubs in `build-libsignal.sh`,
  pre-push hook sources dev-env for cgo.
- Signal private TLS root pinning for `*.signal.org` — vendored
  `signal-messenger.cer` from Signal-iOS ([ADR 0034](./docs/adr/0034-signal-tls-root-pinning.md)).
- Mozilla NSS fallback roots via `golang.org/x/crypto/x509roots/fallback` for
  hosts where the OS trust store is empty (notably cgo Windows).

### Fixed

- **Release CI (macOS):** portable libsignal version extraction (`sed` instead of
  `grep -oP`); per-toolchain cgo `cServiceID` typedef split (`!darwin` vs
  `darwin`); unified `arduino/setup-protoc@v3` for all platforms.
- **Release CI (Windows):** `rustup target add` inside the libsignal clone;
  explicit `PROTOC` path; `#cgo windows LDFLAGS` for Win32 deps.
- **Release CI (attest):** build-provenance attestation gated behind repo
  variable `ENABLE_BUILD_PROVENANCE=true` (private user-owned repos lack
  Artifact Attestations); `continue-on-error` on the attest step.
- **TLS:** `signal-go link` and REST/WebSocket traffic to Signal now verify
  against Signal's private CA without installing it in the OS store.
- **Tests:** filesystem mode `0600` assertions skipped on Windows
  (`fsstore.AssertFileMode0600`).

### Changed

- `actions/setup-go` v5 → v6 across workflows.
- Security contact published: `signal-go-security@thehappydinoa.dev`
  ([`SECURITY.md`](./SECURITY.md)).
- ROADMAP: Phase B/C CI and release pipeline items marked done.

[Unreleased]: https://github.com/thehappydinoa/signal-go/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/thehappydinoa/signal-go/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/thehappydinoa/signal-go/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/thehappydinoa/signal-go/releases/tag/v0.1.0
[0.1.0-rc2]: https://github.com/thehappydinoa/signal-go/releases/tag/v0.1.0-rc2
[0.1.0-rc1]: https://github.com/thehappydinoa/signal-go/releases/tag/v0.1.0-rc1
