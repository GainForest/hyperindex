---
name: writing-changie
description: Create Changie release-note fragments for user-facing, operator-facing, or developer-facing changes in Hyperindex. Use when a change should appear in the next curated changelog instead of being inferred from commit messages.
---

# Writing Changie Fragments

Create a Changie fragment when a change should appear in the next release notes for Hyperindex.

## When to Use

Add a Changie fragment for changes that affect anyone using, operating, integrating with, or contributing to Hyperindex, including:

- New features or visible behavior changes
- Bug fixes that change observable behavior
- GraphQL schema or API behavior changes
- OAuth flow changes
- Admin workflow changes
- Database or migration behavior changes that operators need to know about
- Configuration or deployment changes
- Tooling or contributor workflow changes worth calling out in release notes

Skip fragments for:

- Internal refactors with no observable behavior change
- Tests-only changes
- Docs-only changes that do not introduce or change an actual workflow
- CI-only changes that do not affect operators or contributors in a meaningful way
- Pure cleanup or renaming with no user, operator, or developer impact

If in doubt, add one.

## Source of Truth

Release notes come from Changie fragment files in:

`.changes/unreleased/`

They do not come from commit messages.

## Format

Create a fragment with:

```bash
make changie-new
```

or:

```bash
changie new
```

Changie will prompt for:

- kind
- body
- `Affects`

## File naming

After creating the fragment, rename the generated file in `.changes/unreleased/` to a short descriptive kebab-case title. Do not keep opaque timestamp-based names when a clearer name is possible.

Good filenames:

- `fix-oauth-refresh-failures.yaml`
- `add-admin-schema-toggle.yaml`
- `document-curated-changelog-workflow.yaml`

Bad filenames:

- `added-20260422-120301.yaml`
- `changed-operator-20260422.yaml`
- `misc-fix.yaml`

The filename should describe the release-note topic, not the implementation detail.

## Kinds

Use these Changie kinds:

- `added`
- `breaking`
- `changed`
- `deprecated`
- `removed`
- `fixed`
- `security`

Choose the kind that best matches the final changelog meaning, not the implementation technique.

General guidance:

- `added` — new functionality
- `breaking` — behavior or interface changes that require users, operators, or developers to adapt
- `changed` — changed behavior, enhancements, or workflow changes
- `deprecated` — still works now, but should be migrated away from
- `removed` — functionality removed
- `fixed` — bug fix
- `security` — security-relevant fix or hardening worth calling out

## Affects

`Affects` describes the smallest audience that best matches the impact of the change.

Use one of:

- `user`
- `operator`
- `developer`

Guidance:

- `user` — changes that affect product behavior, APIs, queries, GraphQL behavior, UI/UX, or other externally visible behavior
- `operator` — changes that affect deployment, configuration, migrations, monitoring, runtime behavior, or maintenance of a running instance
- `developer` — changes that affect contributor workflows, local tooling, test workflows, repo maintenance, or development ergonomics

Pick the narrowest audience that still fits.

## Body Writing Rules

Write the body in the voice of the final changelog entry.

The body should describe:

- what changed
- why it matters
- what readers should expect

Do not write the body like a commit message.

Good:
- `Fix OAuth token refresh failures when nonce handling becomes stale.`
- `Add a curated changelog workflow so release notes can be reviewed before release.`
- `Change admin schema behavior so disabled features no longer appear in production queries.`

Bad:
- `Refactor oauth nonce logic`
- `Update server.go and graphql resolver plumbing`
- `Misc fixes`
- `Implement KAR-123`

Focus on impact, not file names or internal code structure.

## Examples

### Example: operator-facing fix

- kind: `fixed`
- Affects: `operator`
- body: `Fix startup failures when invalid database configuration is present so misconfigured deployments fail with a clearer path to recovery.`

### Example: user-facing feature

- kind: `added`
- Affects: `user`
- body: `Add curated release-note fragments so important behavior changes can be surfaced in the changelog instead of being reconstructed from commits.`

### Example: developer-facing workflow change

- kind: `changed`
- Affects: `developer`
- body: `Update the local changelog workflow so contributors can create release-note fragments with a single make target.`

## Goal

A reader should be able to understand the release-relevant impact of the change without reading the PR, commit history, or source code.
