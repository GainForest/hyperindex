# Changelog workflow

This repository uses [Changie](https://github.com/miniscruff/changie) to curate release notes from fragment files.

## Source of truth

Release notes come from `.changes/unreleased/*.yaml`, not from commit history. Keep fragments focused on user-facing, operator-facing, or developer-facing changes that should appear in the next changelog entry.

## Affects

`Affects` describes who or what the change impacts most. Use the smallest audience that still fits the change.

Recommended values:

- `user` — changes that affect product behavior, APIs, queries, or UX
- `operator` — changes that affect deployment, configuration, monitoring, or runtime behavior
- `developer` — changes that affect contributor workflows, tooling, tests, or documentation

## Release-note body guidance

Write the body as a short description of the impact, not the implementation. Good release-note bodies explain what changed, why it matters, and what readers should expect. Bad ones describe internal code paths, file names, or implementation details instead of the visible effect.

## Kinds

Use these fragment kinds:

- `added` — new functionality
- `breaking` — behavior or interface changes that require users, operators, or developers to adapt
- `changed` — changed behavior, enhancements, or workflow changes
- `deprecated` — functionality that still works now but should be migrated away from
- `removed` — functionality removed
- `fixed` — bug fixes
- `security` — security-relevant fixes or hardening worth calling out

## Contributor workflow

1. Add release-note fragments in feature PRs.
2. If you need a new entry, create one with:

   ```bash
   make changie-new
   ```

   or run Changie directly:

   ```bash
   changie new
   ```

## Release PR workflow

1. Merge feature PRs with their `.changes/unreleased/*.yaml` fragments into `main`.
2. When you are ready to prepare release notes, run the **Release** GitHub Actions workflow from `main`.
3. Choose a `release_type` input:

   - `auto` — let Changie infer the next version bump from fragment kinds
   - `patch`, `minor`, or `major` — force the batch level

4. The workflow checks for unreleased fragments first and exits cleanly if none are present.
5. When fragments exist, it runs `go build ./...` and `go test ./...`, then generates the next version note with:

   ```bash
   changie batch <release_type>
   ```

   The workflow passes `auto`, `patch`, `minor`, or `major` directly to `changie batch`.

6. The workflow then merges the version files into the root changelog with:

   ```bash
   changie merge
   ```

7. If Changie produced release changes, the workflow creates or updates a PR from `release/changelog` back into `main`.
8. Inspect the generated changelog diff in that PR before merging.

## Manual fallback

If GitHub Actions is unavailable, a maintainer can still generate the release PR locally from a branch cut from `main`:

1. Create a branch from the latest `main`.
2. Run:

   ```bash
   changie batch auto
   changie merge
   ```

3. Inspect the generated changelog diff before anything else:

   ```bash
   git diff -- CHANGELOG.md
   ```

4. If the changelog looks right, commit the change and open a PR back to `main`.

## Notes

- Keep release notes curated from fragments, not commit messages.
- Do not add Node-based tooling for this workflow.
- Do not treat backend version bumps as part of this phase.
- Release-note batching is automated through the Release workflow, but tagging and release publishing are still intentionally deferred.
