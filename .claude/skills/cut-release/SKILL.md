---
name: cut-release
description: Cut a new signal-go release. Use this skill whenever the user mentions cutting a release, tagging a version, publishing a release, bumping the version, or shipping v0.x.y. Also invoke when the user asks how to release or what steps are needed before shipping.
user-invocable: true
---

# Cut a signal-go release

The release pipeline is fully automated once you trigger it correctly.
Your job is to prepare `main` and fire the right workflow. Don't push
tags manually — that breaks the Release workflow trigger (GitHub
doesn't fire `on.push.tags` when you use the built-in `GITHUB_TOKEN`).

## Step 1 — Determine the version

Follow semver. Current convention: `MAJOR.MINOR.PATCH` prefixed with `v`.

- **Patch** (`0.2.1`): bug fixes, no API change
- **Minor** (`0.3.0`): new features, backwards-compatible API
- **Major** (`1.0.0`): breaking change in `pkg/signal`

Check the current version: look at the most recent `## [x.y.z]` heading
in `CHANGELOG.md` or `git tag -l "v*" | sort -V | tail -1`.

## Step 2 — Verify CI is green

Before touching anything:

```bash
gh run list --branch main --limit 5
```

All recent runs on `main` must be passing. Don't release from a red
main. If CI is failing, fix it first.

## Step 3 — Update CHANGELOG.md

Move everything from `[Unreleased]` into a new dated section:

```markdown
## [0.3.0] - 2026-05-28

### Added
- ...

### Fixed
- ...
```

Rules:
- Date format is `YYYY-MM-DD` (today's date)
- Keep `[Unreleased]` at the top but empty (or "Nothing yet.")
- Update the compare links at the **bottom** of the file:
  ```markdown
  [Unreleased]: https://github.com/thehappydinoa/signal-go/compare/v0.3.0...HEAD
  [0.3.0]: https://github.com/thehappydinoa/signal-go/releases/tag/v0.3.0
  ```
  Add the new version line; update the `[Unreleased]` line to point
  at the new tag.

## Step 4 — Update ROADMAP.md

Tick any phase items this release closes. Update the status legend
comment for any phase that changed from "in progress" to "done".

## Step 5 — Merge to main

If you're working on a release branch, open a PR now. The tag must
point at `main`, so everything must be merged before Step 6.

Verify once more:

```bash
git log --oneline origin/main -5
gh run list --branch main --limit 3
```

## Step 6 — Create the release tag (GitHub Actions)

**Do not `git tag` locally.** Use the workflow:

1. Open **Actions → Create release tag → Run workflow** on GitHub.
2. Fill in:
   - **version**: `0.3.0` or `v0.3.0` (the workflow normalises to `v…`)
   - **ref**: `main`
   - **require_changelog**: leave enabled (fails if `## [0.3.0]` is missing)
3. Run.

After the tag is pushed, watch for a **Release** workflow to start
within ~30 seconds. If it doesn't:

```bash
gh workflow run release.yml --ref v0.3.0
```

Or use **Actions → Trigger Release for tag** if it exists.

## Step 7 — Monitor the Release build

```bash
gh run watch   # or open the Actions tab
```

The matrix: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
`windows/amd64`. All five must pass (Windows is no longer experimental
as of v0.2.0). Each produces a platform archive + `.sha256` sidecar.

## Step 8 — Publish the draft release

1. Open **Releases** on GitHub.
2. Find the new **Draft** for your tag.
3. Verify:
   - All five platform archives are present (`signal-go_0.3.0_*.tar.gz` / `.zip`)
   - Each has a `.sha256` sidecar
   - Release notes look correct (auto-generated from CHANGELOG by the workflow)
4. Click **Publish release**.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Create tag fails: "CHANGELOG section missing" | Add `## [0.3.0] - YYYY-MM-DD` to CHANGELOG and re-run |
| Release workflow didn't start | Run `gh workflow run release.yml --ref v0.3.0` |
| Windows leg failed | If it's a flake, re-run just the Windows job. If it's a real failure, investigate before publishing — Windows is no longer experimental |
| Draft release has wrong notes | Edit directly in GitHub UI before publishing |
| Tag already exists | Only delete if the release was a mistake; otherwise pick a new patch version |

## After publishing

- Announce in whatever channels are appropriate.
- Start a new `[Unreleased]` section in CHANGELOG if it's empty.
- If this release closes the last item in a ROADMAP phase, update the
  phase status from "in progress" to "done".

$ARGUMENTS
