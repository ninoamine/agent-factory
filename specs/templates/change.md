# Change request template

Use for any change to an existing feature. Paste into the chat with `/spec`, or fill it in yourself. The spec is edited first, then code.

```
Feature: <ID, e.g. ROUTE>
Change type: add rule | modify rule | remove rule | new open question | resolve open question
Rule IDs affected: <e.g. ROUTE-6, new ROUTE-10>

Why: <one sentence>

Before: <old behavior or "none">
After: <new behavior, MUST/MUST NOT wording>

Acceptance criteria to add or change:
- <input> -> <expected>

Impacts: <other features, common files, decisions.md entry>
```

Checklist after the spec edit:
- Rule IDs unique and never reused (removed rules stay gone; do not renumber).
- `index.md` status or dependencies updated if needed.
- `decisions.md` entry added if a decision was made.
- Open question removed once resolved, and the rule updated.
