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
