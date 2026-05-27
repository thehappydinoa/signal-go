# Support

This document explains where to ask questions, report bugs, and request
features for signal-go.

## Getting help

Use GitHub Issues for usage questions and troubleshooting requests.

Before opening a support issue:

1. Check [README.md](./README.md).
2. Check [docs/guides/getting-started.md](./docs/guides/getting-started.md).
3. Check [docs/guides/testing.md](./docs/guides/testing.md) and existing
   open/closed issues.

When you open an issue, include:

- OS and architecture
- Go version (`go version`)
- Whether `internal/libsignal/lib/libsignal_ffi.a` exists
- Exact command run and full error output

## Bug reports

Use the Bug Report issue template and include reproducible steps and
expected vs actual behavior.

## Feature requests

Use the Feature Request template and map the request to roadmap scope
when possible: [ROADMAP.md](./ROADMAP.md).

## Security issues

Do not file public issues for vulnerabilities. Follow [SECURITY.md](./SECURITY.md).

## Response expectations

- Initial triage target for support/bug issues: within 7 days.
- We may ask for additional logs or a minimal reproduction before taking
  action.
- Pre-alpha caveat: APIs and behavior may change quickly before v0.1.0.

## Supported versions

Until v0.1.0, support is best-effort on `main`.

After v0.1.0, this file will include an explicit support matrix.