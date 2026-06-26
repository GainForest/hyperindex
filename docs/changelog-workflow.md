# Changelog workflow

This repository uses [Changie](https://github.com/miniscruff/changie) to curate release notes from fragment files.

## Source of truth

This file is the source of truth for when to add, skip, write, batch, and publish Changie release-note fragments. Other repo docs should link here instead of restating the policy.

Release notes come from `.changes/unreleased/*.yaml`, not from commit history. Keep fragments focused on user-facing, operator-facing, or developer-facing changes that should appear in the next changelog entry.

Skip fragments when the diff is only:

- docs-only changes
- tests-only changes, including API smoke tests and smoke expectations
- Docker-only or Docker Compose-only changes
- CI-only changes that do not meaningfully affect operators or contributors
- internal refactors with no meaningful external impact

If skipped support files are changed alongside an externally meaningful product, API, config, migration, or runtime change, write the fragment for the externally meaningful change only.

If you are unsure after checking this policy, ask before creating a fragment for a skip-list-only change.

## Agent decision checklist

Before creating a fragment, check the changed files and answer these questions in order:

1. Is the diff only docs, tests, API smoke checks or expectations, Docker or Docker Compose files, CI-only changes, or internal refactors with no external impact?
   - Yes: do not create a fragment.
   - No: continue.
2. Does the change affect public API behavior, GraphQL behavior, runtime behavior, configuration, migrations, deployment behavior, user workflows, or operator workflows?
   - Yes: create a fragment for that external impact.
   - No: continue.
3. Does the change only touch support files from the skip list, but alongside externally meaningful code or schema changes?
   - Yes: create a fragment for the externally meaningful change only.
   - No: continue.
4. Still unsure?
   - Ask before creating a fragment.

## Changed-file examples

These examples are guidelines, not replacements for the decision checklist:

| Changed files | Fragment? | Why |
| --- | --- | --- |
| `README.md`, `docs/**`, or `.agents/**` only | No | Docs-only changes do not need release notes. |
| `*_test.go`, `tests/**`, or `testdata/**` only | No | Tests-only changes do not affect release behavior. |
| `tests/api-smoke/**` only | No | API smoke checks and expectations are test coverage, not release behavior. |
| `Dockerfile`, `docker-compose*.yml`, `docker-compose*.yaml`, or Docker-only support files only | No | Docker-only changes are intentionally excluded from release fragments. |
| `.github/workflows/**` only | Usually no | CI-only changes are skipped unless they meaningfully change contributor or operator workflow. |
| `internal/graphql/**` or lexicon changes that alter public GraphQL fields, filters, pagination, or errors | Yes | Public API behavior changed. |
| `internal/database/migrations/**` or repository changes that alter persisted schema or migration behavior | Yes | Operators and downstream users may need to understand the runtime data change. |
| `internal/config/**`, startup code, or deployment config that changes required environment variables or runtime defaults | Yes | Operators may need to update deployments. |
| Docker or smoke-test files plus public API/runtime changes | Yes | Write the fragment for the public API/runtime change, not for the support files. |

## Affects

`Affects` describes who or what the change impacts most. Use the smallest audience that still fits the change.

Recommended values:

- `user` — changes that affect product behavior, APIs, queries, or UX
- `operator` — changes that affect deployment, configuration, monitoring, or runtime behavior
- `developer` — changes that affect release-worthy contributor workflows or tooling; docs-only, tests-only, smoke-only, and Docker-only changes do not need fragments

## Release-note body guidance

Write the body for someone reading the changelog without the pull request, commit history, or source code open. Good release-note bodies explain:

- what changed
- why it changed or what problem it solves
- what readers should expect, do, or watch for

Mention implementation details only when they affect user, operator, or developer behavior. Avoid file names, internal code paths, ticket numbers, or vague summaries that force readers to inspect the PR.

Use enough detail for the size and risk of the change:

- Small fixes or narrow behavior changes can be one clear sentence.
- New features, meaningful workflow changes, externally relevant refactors, and `breaking`, `deprecated`, or `removed` changes should usually be two to three sentences or one compact paragraph.
- Operator-facing changes should include configuration, deployment, migration, monitoring, or rollback implications when relevant.
- Refactor-like changes only need fragments when they have external impact; if they do, explain the rationale and resulting behavior, not the internal rearrangement.

Good examples:

- `Fix OAuth token refresh failures when nonce handling becomes stale so affected users can sign in again without clearing local state.`
- `Add a curated changelog workflow so release notes can be reviewed before release. Contributors now add focused fragments during feature work, and maintainers batch them into a release PR before publishing.`
- `Change admin schema behavior so disabled features no longer appear in production queries. This keeps production schemas aligned with the active deployment configuration and avoids exposing fields that cannot return data.`
- `Rework label ingestion recovery so deployments can handle labeler cursor resets without permanently stalling subscriptions. Operators should still review replayed label data before resetting a subscription cursor.`

Bad examples:

- `Refactor oauth nonce logic`
- `Update server.go and graphql resolver plumbing`
- `Add changelog workflow`
- `Misc fixes`
- `Implement KAR-123`

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

1. Check this document before deciding whether a change needs a fragment.
2. If Changie is not installed locally, install the repo tools:

   ```bash
   make tools
   ```

3. Add release-note fragments in feature PRs when this policy says they are needed.
4. If you need a new entry, create one with:

   ```bash
   make changie-new
   ```

   or run Changie directly:

   ```bash
   changie new
   ```

5. After creating the fragment, rename the generated file in `.changes/unreleased/` to a short descriptive kebab-case title. The filename should describe the release-note topic, not the implementation detail.

   Good filenames:

   - `fix-oauth-refresh-failures.yaml`
   - `add-admin-schema-toggle.yaml`
   - `document-curated-changelog-workflow.yaml`

   Bad filenames:

   - `added-20260422-120301.yaml`
   - `changed-operator-20260422.yaml`
   - `misc-fix.yaml`

## Maintainer release workflow

Run one manual workflow to prepare the release notes PR. After review, merging that PR publishes the release automatically.

The **Publish release tag and GitHub Release** workflow remains manually dispatchable from `main` as a fallback when a prepared release file already exists but the automatic publish did not run or did not complete.

### Decision rule

- If no versioned release file exists yet and `.changes/unreleased/*.yaml` fragments exist, run **Prepare release notes PR**.
- Merge the generated `release/changelog` PR after reviewing the changelog. The release publishes automatically only when the merged PR targets `main`, comes from the same-repository `release/changelog` branch, and has the `release` label.
- If a versioned `.changes/vX.Y.Z.md` or `.changes/X.Y.Z.md` file already exists and the automatic publish needs to be retried, run **Publish release tag and GitHub Release** manually from `main`. New unreleased fragments for the next cycle do not block publishing the prepared version.

### 1. Prepare release notes PR

1. Merge feature PRs with their `.changes/unreleased/*.yaml` fragments into `main`.
2. Run the **Prepare release notes PR** GitHub Actions workflow from `main`.
3. Choose a `release_type` input:

   - `auto` — let Changie infer the next version bump from fragment kinds
   - `patch`, `minor`, or `major` — force the batch level

4. The workflow requires at least one unreleased fragment. If none exist, it fails.
5. The workflow runs `go build ./...` and `go test ./...`, then generates the next version note with:

   ```bash
   changie batch <release_type>
   ```

   The workflow passes `auto`, `patch`, `minor`, or `major` directly to `changie batch`.

6. The workflow then merges the version files into the root changelog with:

   ```bash
   changie merge
   ```

7. If Changie does not produce any release diff in `CHANGELOG.md` or `.changes`, the workflow fails.
8. Otherwise, the workflow ensures the `release` label exists and creates or updates a labeled PR from `release/changelog` back into `main`.
9. Inspect the generated changelog diff in that PR before merging. Merging this labeled `release/changelog` PR automatically starts publishing.

### 2. Publish release tag and GitHub Release

1. The workflow runs automatically after the labeled same-repository `release/changelog` PR is merged into `main`, or manually from `main` as a fallback.
2. The workflow requires a prepared release file:

   - a generated `.changes/vX.Y.Z.md` or `.changes/X.Y.Z.md` release file must exist

3. The workflow auto-detects the latest generated changelog version.
4. If newer `.changes/unreleased/*.yaml` fragments exist for the next cycle, the workflow logs them and continues publishing the already-generated version.
5. The workflow runs `go build ./...` and `go test ./...` in a read-only verification job before publishing.
6. It then:

   - creates and pushes the corresponding `vX.Y.Z` git tag if it does not already exist
   - publishes a GitHub Release using the generated `.changes` version file as the release notes body if it does not already exist

7. If the tag already exists but the GitHub Release does not, the workflow creates just the release.
8. If both the tag and GitHub Release already exist, the workflow exits safely as a no-op.

## GitHub token and permissions

The release workflows use `RELEASE_BOT_TOKEN || GITHUB_TOKEN`.

- **Prepare release notes PR** needs permission to push branch updates, open or update pull requests, and apply the `release` label.
- **Publish release tag and GitHub Release** needs permission to push tags and create GitHub Releases.
- If repository settings limit the default `GITHUB_TOKEN`, configure `RELEASE_BOT_TOKEN` with the required write access.
- Treat `RELEASE_BOT_TOKEN` as a privileged credential. Prefer a fine-grained, repository-scoped token with only the access needed for release branch updates, tag pushes, and GitHub Release creation.

The publish workflow runs build and test checks in a read-only verification job before the privileged release token is used for final tag and release publishing.

Workflow permissions are intentionally minimal:

- prepare: `contents: write`, `issues: write`, `pull-requests: write`
- publish verification: `contents: read`
- publish finalization: `contents: write`

## Changelog validation workflow

`.github/workflows/changelog.yml` validates the Changie configuration and fragments.

- It runs on pull requests, pushes to `main`, and manual dispatch.
- If unreleased fragments exist, it runs `changie batch auto --dry-run`.
- If no unreleased fragments exist, it skips validation successfully.
- It does not enforce that every pull request must include a changelog fragment.

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
5. After the PR is merged, identify the generated release notes file: `.changes/vX.Y.Z.md` or `.changes/X.Y.Z.md`.
6. Create and push the matching `vX.Y.Z` tag and create a GitHub Release whose body matches that generated version file. New unreleased fragments for the next cycle do not block publishing the already-prepared release.

## Notes

- Keep release notes curated from fragments, not commit messages.
- Do not add Node-based tooling for this workflow.
- Do not treat backend version bumps as part of this phase.
- The generated `.changes/vX.Y.Z.md` or `.changes/X.Y.Z.md` file is the release-notes source for the published GitHub Release.
