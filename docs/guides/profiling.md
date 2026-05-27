# Profiling long-running clients

`go test -memprofile` and `go test -cpuprofile` only apply to **tests**. For a
long-lived process (for example [`examples/echo-bot`](../../examples/echo-bot/)),
pass profile paths as **program flags after the subcommand**, not as `go run`
arguments.

## echo-bot soak

Run the bot, exercise it (send messages from another account), then stop with
Ctrl+C. Profiles are written on exit.

```sh
go run ./examples/echo-bot run \
  -store ./.signal-e2e \
  -passphrase-file ./.signal-e2e/passphrase \
  -memprofile=mem.prof \
  -cpuprofile=cpu.prof
```

Inspect heap and CPU:

```sh
go tool pprof -http=:8080 mem.prof
go tool pprof -http=:8081 cpu.prof
```

Useful pprof views for leaks: `top`, `list`, `web` (flame graph in the browser UI).

Recommended reporting commands (paste output into issues or audit notes):

```sh
go tool pprof -top -nodecount=30 cpu.prof
go tool pprof -top -nodecount=30 mem.prof
go tool pprof -top -cum -nodecount=20 cpu.prof
go tool pprof -sample_index=inuse_space -top -nodecount=30 mem.prof
```

## Built binary

Same flags work on a compiled binary:

```sh
task build   # or: go build -o bin/echo-bot ./examples/echo-bot
./bin/echo-bot run -store ./.signal-e2e -passphrase-file ./.signal-e2e/passphrase \
  -memprofile=mem.prof -cpuprofile=cpu.prof
```

## Library helper

[`internal/profile`](../../internal/profile/) registers `-memprofile` / `-cpuprofile`
on a [flag.FlagSet] and flushes on [profile.Session.Close]. Example CLIs can reuse
it the same way echo-bot does.

## E2e tests

Unit/e2e tests still use the standard Go test flags:

```sh
go test -tags=e2e -memprofile=mem.prof -cpuprofile=cpu.prof -run TestE2E_Open ./pkg/signal/...
```

Interactive link remains `task test:e2e:link` (not profiled by default).

## Phase 8 long-running receive bake (recorded)

ROADMAP Phase 8 tracks a maintainer-run heap/CPU soak of a real linked-device
receive loop. Methodology and results live here; the threat-model audit checklist
links back to this section.

### Methodology

1. Build or `go run` [`examples/echo-bot`](../../examples/echo-bot) with
   `-memprofile` and `-cpuprofile` on the **`run`** subcommand (see above).
2. Use a real linked `sqlstore` directory (Argon2id KDF at open — expect a
   large **transient** heap spike that GC reclaims after startup).
3. Keep the process connected on the chat websocket; send inbound 1:1 traffic
   from another account when exercising steady-state CPU (idle bots sample
   almost no CPU time).
4. Stop with Ctrl+C; inspect profiles with `go tool pprof` (commands above).
5. Compare **short** (~1 min) vs **long** (~20 min) runs: short captures
   startup; long confirms steady-state heap.

### Bake — 2026-05-27 (Windows, echo-bot + sqlstore)

| Run | Wall time | CPU samples | Heap `inuse_space` (exit) |
| --- | --- | --- | --- |
| Short | ~38 s | 400 ms (1.05% of wall) | ~75 MB — **64 MB `argon2.initBlocks`** (store open KDF) |
| Long | ~1187 s (~20 min) | 640 ms (0.054% of wall) | **~5.8 MB** — no Argon2, no receive-path growth |

**Heap (long run):** Steady state ~6 MB total (`runtime.allocm`, CPU profile
buffer, TLS cert parse, gzip pool). No `libsignal`, `sqlite` session blobs, or
`handleServerRequest` in the top frames — **no leak signal** for this workload.

**Heap (short run):** The 64 MiB Argon2 working set (ADR 0012 default
`Memory: 64*1024` KiB) dominates at exit; the long run shows it is **not**
retained for the life of the process.

**CPU (both runs):** Process is **I/O-bound** while idle on the websocket (CPU
sample rate ≪ 1% of wall time). Attributed on-CPU work when present:

- `signalgo_store_session` → `database/sql` → SQLite `step` → Windows
  `FlushFileBuffers` / `_winSync` (durable session persist after libsignal
  crypto updates).
- `runtime.cgocall` (libsignal + SQLite C API).
- Minor startup: Argon2 KDF, occasional `save_identity_key`, TLS write.

Decrypt/receive/send frames did not rank in `top` at this traffic level; a
**busier** soak (many messages, CPU sample % well above 1%) is needed to profile
hot decrypt paths. Optional: `-trace=trace.out` + `go tool trace` for latency.

**Conclusion:** Phase 8 **heap bake satisfied** for linked-device echo-bot on
Windows/sqlstore. CPU profile is consistent with an idle linked device, not a
performance regression. Remaining Phase 8 memory item: ASAN/valgrind on a
cgo-linked test binary (see ROADMAP).
