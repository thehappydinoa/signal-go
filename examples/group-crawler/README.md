# group-crawler

Example script that **crawls Signal groups** from a linked account:

1. Pulls your group list from [storage sync](https://github.com/thehappydinoa/signal-go/blob/main/pkg/signal/storage.go).
2. For each group, logs title, description, members, and revision (via `SyncGroup`).
3. Scans **group descriptions** and **member profile “about” bios** (when a profile key is known) for `signal.group` invite links.
4. Listens on the live websocket and logs **inbound messages**, edits, reactions, and group updates.
5. When an invite link is found, **previews and joins** the group, then repeats for newly discovered groups.

## Important limits

- **No message history API** — only messages that arrive while the crawler is running are logged. Past chat is not fetched.
- **Profile bios** need a 32-byte profile key (from storage contacts or a message that carried one). Members without a known key are skipped with `PROFILE|SKIP`.
- **Automated joining** can spam communities and may conflict with Signal’s terms or group rules. Use a dedicated test account, rate limits, and only groups you are allowed to probe.

## Run

Link a store (once):

```sh
signal-go link -store ./.signal-group-crawler
```

Start the crawler:

```sh
go run ./examples/group-crawler -store ./.signal-group-crawler
```

Optional flags:

| Flag | Default | Meaning |
|------|---------|---------|
| `-seed-invite` | (empty) | Join this `https://signal.group/#...` URL before crawling storage groups |
| `-max-groups` | `0` | Stop visiting groups after N (0 = unlimited) |
| `-join-cooldown` | `3s` | Minimum time between invite joins |
| `-dry-run` | `false` | Discover and log invites without joining |

Same store flags as other examples: `-passphrase-file`, `-plaintext`, `-client`, `-user-agent`.

## Log format

Every line is prefixed for easy grepping:

```text
CRAWL|START|...
CRAWL|GROUP|VISIT|...
CRAWL|GROUP|MEMBER|...
CRAWL|INVITE|FOUND|...
CRAWL|INVITE|JOIN_OK|...
CRAWL|EVENT|MESSAGE|...
```

Banner lines use `════` between major group visits and invite previews.

## Example

```sh
go run ./examples/group-crawler \
  -store ./.signal-group-crawler \
  -seed-invite 'https://signal.group/#AbCdEf...' \
  -max-groups 20 \
  -join-cooldown 5s
```
