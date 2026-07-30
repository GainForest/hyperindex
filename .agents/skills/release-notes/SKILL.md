---
name: release-notes
description: Polish Hyperindex's generated Changie release notes on the release/changelog branch into clear, human-friendly release notes while preserving factual scope and keeping the version file and CHANGELOG.md synchronized. Use whenever the user asks to review, rewrite, humanize, curate, polish, or finalize generated release notes or a changelog release PR. Do not use this skill to create individual unreleased fragments, choose a version bump, tag, publish, or deploy a release.
---

# Hyperindex Release Notes

Turn Changie's generated release entry into release notes that someone can understand without opening the pull requests or source code. Preserve the facts supplied by the release fragments; improve their organization, context, and wording.

This skill only polishes an already-generated release. `docs/changelog-workflow.md` remains the source of truth for fragment policy, batching, and publishing.

## Boundaries

Work only on the generated release entry:

- `.changes/vX.Y.Z.md` or `.changes/X.Y.Z.md`
- the matching `## vX.Y.Z` or `## X.Y.Z` section in `CHANGELOG.md`

Do not:

- create, delete, or rewrite `.changes/unreleased/*.yaml`
- add changes merely because they appear in commit history
- change the version number or the release's `Affects` audiences without evidence and user approval
- modify older release entries
- commit, push, merge, tag, publish, deploy, or edit a GitHub Release
- run `changie batch`; the release has already been batched

The version file is the source for the GitHub Release. `CHANGELOG.md` must contain the same polished entry.

## Phase 1: Establish the release

1. Read `docs/changelog-workflow.md` completely.
2. Check the current branch and working tree:

   ```bash
   git status --short --branch
   ```

   Normally this work belongs on `release/changelog`. If another branch is checked out, do not switch branches unless the user asked for it. Inspect all existing changes. Stop if the candidate version file or `CHANGELOG.md` already has edits outside the requested polishing, or if another process may be writing them. Other unrelated dirty files do not block a notes-only edit, but do not touch or stage them and report them in the completion receipt.
3. List generated version files and local release tags:

   ```bash
   find .changes -maxdepth 1 -type f -name '*.md' -printf '%f\n' | sort -V
   git tag --sort=-v:refname
   ```

4. Identify the newest generated version without a matching release tag. It is normally the only unpublished version on `release/changelog`. If there is no clear single candidate, ask the user which version to polish.
5. Confirm that `CHANGELOG.md` contains a matching version heading. Record the exact `Affects` values before editing.
6. Read the generated version file, its matching changelog section, and the previous two or three release files to understand the project's established level of detail.

## Phase 2: Build a factual release map

Treat every generated change item as an obligation. Before rewriting, make a small working checklist containing:

```text
ReleaseChange = {
  originalText: string
  kind: "added" | "breaking" | "changed" | "deprecated" | "removed" | "fixed" | "security"
  affects: Array<"user" | "operator" | "developer">
  practicalOutcome: string
  requiredAction?: string
  evidence: string[]
}
```

Use evidence in this order:

1. The generated version file and the Changie fragments represented by it define release scope.
2. Changed code, tests, and documentation may clarify how a listed change behaves and why it matters.
3. Local commit and merge history may connect listed changes to pull requests.
4. GitHub pull requests and issues may supply motivation and contributor credit, but only query GitHub when the user has explicitly approved network/API access in the current conversation.

Useful local inspection commands include:

```bash
git log --oneline <previous-tag>..HEAD
git log --merges --oneline <previous-tag>..HEAD
git diff --stat <previous-tag>..HEAD
```

Do not expand the release scope from these commands. Use them only to explain a change already present in the generated notes.

If GitHub research is approved, inspect relevant pull requests and linked issues rather than guessing:

```bash
gh pr view <number> --json title,author,body,closingIssuesReferences,labels,url
gh issue view <number> --json title,author,body,url
```

Do not claim that someone is a first-time contributor without verifying it. Do not credit bots as community contributors.

## Phase 3: Choose the shape

Match the amount of structure to the release instead of forcing every version into one template.

### Narrow patch release

For one to four small or closely related changes, prefer:

```markdown
## vX.Y.Z

[One short paragraph explaining the release theme and practical result.]

### Fixed

- [Concrete change and what readers can now expect.]

#### Affects

- operator
```

Keep standard kind headings such as `Fixed` or `Security` when they remain the clearest grouping.

### Substantial or mixed release

When a release has major features, several audiences, or changes that need context, use descriptive sections:

```markdown
## vX.Y.Z

[One to three sentences summarizing the release's theme and scope.]

### [Descriptive outcome, not a copied PR title]

[What was difficult or unavailable before.]

[What changed and how users or operators experience it now.]

### Additional changes

- **[Short label].** [Concrete behavior and practical effect.]

### Upgrade notes

[Required action, compatibility impact, migration behavior, or rollback concern.]

### Contributors

[Optional evidence-backed credit for meaningful community participation.]

#### Affects

- user
- operator
```

Use only sections the release needs:

- Give a major feature or risky change its own descriptive section.
- Combine small independent items under `Additional changes`.
- Use `Upgrade notes` for breaking changes, deprecations, removals, required configuration, migrations, or operator action.
- Use `For users`, `For operators`, and `For developers` when audience grouping is clearer than feature grouping.
- Add `Contributors` or `Community` only when there is evidence-backed participation worth recognizing.
- Do not add an install block, recent-release table, or duplicated `What's Changed` list; those are not part of Hyperindex's release format.

## Phase 4: Write for people

Follow these principles:

- **Lead with the outcome.** Explain what changed and why someone should care before implementation details.
- **Describe the old and new behavior.** Readers should understand what is different after upgrading.
- **Be concrete.** Name relevant GraphQL fields, configuration variables, ingestion modes, limits, migration effects, or operator actions when they matter.
- **Keep technical detail proportional.** Include internals only when they explain observable behavior, risk, compatibility, or operations.
- **Separate facts from implications.** Do not invent performance gains, security guarantees, compatibility promises, or migration behavior.
- **Use active voice and varied sentence structure.** Prefer concrete subjects and verbs.
- **Use paragraphs for explanation and bullets for discrete changes.** Do not turn the entire release into either a wall of prose or a changelog dump.
- **Preserve code spelling.** Keep identifiers, environment variables, GraphQL fields, collection NSIDs, and commands exact.
- **Make required action unmistakable.** Say who must act, what they must change, and what happens if they do not.

Avoid:

- hype such as “exciting,” “game-changing,” “seamless,” or “powerful”
- vague wording such as “miscellaneous fixes,” “various improvements,” or “enhanced performance”
- PR titles copied verbatim when a clearer user-facing heading is possible
- file paths, internal function names, ticket IDs, and refactor details without external relevance
- clever punchlines or a closing slogan
- repeated summary sentences at the end of each section
- excessive em dashes

### Contributor credit

When contributor context is relevant and verified:

- Use bare `@username` mentions so GitHub renders contributor profiles naturally.
- Link the pull request or issue that supports the credit.
- Credit what the person built, reported, requested, or clarified.
- Prefer a short narrative section for substantial community participation and a compact bullet list for issue reporters.
- Keep contributor recognition secondary to understanding the release.

## Phase 5: Edit and synchronize

Before making a substantial rewrite, briefly tell the user how the generated items will be grouped and whether any text will be condensed or moved. If their current request already explicitly authorizes polishing the generated release files, proceed; otherwise wait for confirmation.

1. Rewrite only the candidate version file.
2. Preserve every original release item exactly once in substance. Related items may be combined, but none may disappear.
3. Preserve the exact `Affects` set unless the user approves a correction.
4. Regenerate the root changelog from the version files:

   ```bash
   changie merge
   ```

   If Changie is unavailable, update only the matching section in `CHANGELOG.md` and verify it is byte-for-byte equivalent to the version file.
5. Inspect the result:

   ```bash
   git diff -- .changes/vX.Y.Z.md CHANGELOG.md
   git diff --check
   ```

6. Confirm that `changie merge` did not alter older release entries. If it did, stop and report the unexpected diff instead of accepting unrelated rewrites.

## Review checklist

Before presenting the draft, verify:

- [ ] The version and `Affects` values are unchanged.
- [ ] Every generated change is still represented exactly once in substance.
- [ ] Nothing outside the generated release scope was added.
- [ ] The opening explains the release theme in plain language.
- [ ] Major changes explain previous behavior, new behavior, and practical impact.
- [ ] Breaking or operator-facing changes include explicit action where needed.
- [ ] Claims are supported by local evidence or approved GitHub research.
- [ ] Contributor credit is accurate and evidence-backed.
- [ ] The version file and its `CHANGELOG.md` section match.
- [ ] Older changelog sections are untouched.
- [ ] `git diff --check` passes.

Present the polished draft and a concise summary of the editorial changes. Wait for review. Do not commit or publish it.
