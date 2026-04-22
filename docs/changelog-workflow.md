# Changelog workflow (Phase 1)

This repository uses [Changie](https://github.com/miniscruff/changie) to curate release notes from fragment files.

## Source of truth

Release notes come from `.changes/unreleased/*.yaml`, not from commit history. Keep fragments focused on user-facing or operator-facing changes that should appear in the next changelog entry.

## Affects

`Affects` describes who or what the change impacts most. Use the smallest audience that still fits the change.

Recommended values:

- `user` — changes that affect product behavior, APIs, queries, or UX
- `operator` — changes that affect deployment, configuration, monitoring, or runtime behavior
- `developer` — changes that affect contributor workflows, tooling, tests, or documentation

## Release-note body guidance

Write the body as a short description of the impact, not the implementation. Good release-note bodies explain what changed, why it matters, and what readers should expect. Bad ones describe internal code paths, file names, or implementation details instead of the visible effect.

## Maintainer workflow

1. Review the pending fragments in `.changes/unreleased/`.
2. If you need a new entry, create one with:

   ```bash
   make changie-new
   ```

   or run Changie directly:

   ```bash
   changie new
   ```

3. When the fragments are ready to batch, generate the next version note:

   ```bash
   changie batch auto
   ```

   Use `major`, `minor`, `patch`, or a concrete version if the release target is already known.

4. Merge the version files into the root changelog:

   ```bash
   changie merge
   ```

5. Inspect the generated changelog diff before anything else:

   ```bash
   git diff -- CHANGELOG.md
   ```

6. If the changelog looks right, commit the change and leave tags and releases for a later phase.

## Notes

- Keep release notes curated from fragments, not commit messages.
- Do not add Node-based tooling for this workflow.
- Do not treat backend version bumps as part of this phase.
- Tagging and release publishing are intentionally deferred to a future phase.
