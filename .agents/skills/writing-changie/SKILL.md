---
name: writing-changie
description: Create Changie release-note fragments for user-facing, operator-facing, or developer-facing changes in Hyperindex. Use when a change should appear in the next curated changelog instead of being inferred from commit messages.
---

# Writing Changie Fragments

`docs/changelog-workflow.md` at the repository root is the source of truth for Changie policy. It defines when to add or skip fragments, valid kinds, `Affects` values, body style, filename guidance, validation, and maintainer release workflows.

Before creating a fragment:

1. Read `docs/changelog-workflow.md` completely.
2. Follow that document when deciding whether the current change needs a fragment.
3. If the policy says the change does not need a fragment, do not create one.
4. If the change is skip-list-only but still seems ambiguous, ask instead of creating a fragment.

When a fragment is needed, create it with:

```bash
make changie-new
```

or:

```bash
changie new
```

Then follow `docs/changelog-workflow.md` for the filename, kind, `Affects`, and body text.

Do not duplicate or reinterpret the fragment policy in this skill. If the policy changes, update `docs/changelog-workflow.md` first and keep this skill pointing to it.
