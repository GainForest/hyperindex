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

## Maintainer release workflow

Use two separate manual workflows:

- **Prepare release notes PR**
- **Publish release tag and GitHub Release**

### Decision rule

- If `.changes/unreleased/*.yaml` fragments exist, run **Prepare release notes PR**.
- If no unreleased fragments remain and a versioned `.changes/vX.Y.Z.md` file already exists, run **Publish release tag and GitHub Release**.

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
8. Otherwise, the workflow creates or updates a PR from `release/changelog` back into `main`.
9. Inspect the generated changelog diff in that PR before merging.

### 2. Publish release tag and GitHub Release

1. After the release PR is merged, run **Publish release tag and GitHub Release** from `main`.
2. The workflow requires a clean release state:

   - no unreleased `.changes/unreleased/*.yaml` fragments may remain
   - a generated `.changes/vX.Y.Z.md` or `.changes/X.Y.Z.md` release file must exist

3. The workflow auto-detects the latest generated changelog version.
4. The workflow runs `go build ./...` and `go test ./...` in a read-only verification job before publishing.
5. It then:

   - creates and pushes the corresponding `vX.Y.Z` git tag if it does not already exist
   - publishes a GitHub Release using the generated `.changes` version file as the release notes body if it does not already exist

6. If the tag already exists but the GitHub Release does not, the workflow creates just the release.
7. If both the tag and GitHub Release already exist, the workflow exits safely as a no-op.

## GitHub token and permissions

The release workflows use `RELEASE_BOT_TOKEN || GITHUB_TOKEN`.

- **Prepare release notes PR** needs permission to push branch updates and open or update pull requests.
- **Publish release tag and GitHub Release** needs permission to push tags and create GitHub Releases.
- If repository settings limit the default `GITHUB_TOKEN`, configure `RELEASE_BOT_TOKEN` with the required write access.
- Treat `RELEASE_BOT_TOKEN` as a privileged credential. Prefer a fine-grained, repository-scoped token with only the access needed for release branch updates, tag pushes, and GitHub Release creation.

The publish workflow runs build and test checks in a read-only verification job before the privileged release token is used for final tag and release publishing.

Workflow permissions are intentionally minimal:

- prepare: `contents: write`, `pull-requests: write`
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
5. After the PR is merged, verify there are no unreleased fragments left.
6. Create and push the matching `vX.Y.Z` tag and create a GitHub Release whose body matches the generated `.changes/vX.Y.Z.md` file.

## Notes

- Keep release notes curated from fragments, not commit messages.
- Do not add Node-based tooling for this workflow.
- Do not treat backend version bumps as part of this phase.
- The generated `.changes/vX.Y.Z.md` file is the release-notes source for the published GitHub Release.
