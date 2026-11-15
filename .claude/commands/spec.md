---
description: Create or change a feature spec (no code)
argument-hint: <feature ID or idea, plus the change in plain words>
---
Task: write or update specs only. Do NOT write code.

Request: $ARGUMENTS

1. Read `specs/index.md`, `specs/common/project.md`, `specs/roadmap.md`, and the feature file if it exists (plus its dependencies).
2. New feature: use `specs/templates/feature.md`. Existing feature: apply `specs/templates/change.md` logic.
3. Write rules as MUST / MUST NOT / SHOULD, one behavior each, with unique IDs. Never reuse or renumber IDs.
4. Add acceptance criteria with concrete input and expected output.
5. Put anything undecided under "Open questions" with a proposed answer. Ask me about the important ones before finishing.
6. Update `specs/index.md` and `specs/decisions.md` if needed. Do not duplicate rules that already live elsewhere; reference the rule ID.
7. Check the request against `specs/roadmap.md`: if it is outside the current version, say so and stop.
8. Report: files changed, rule IDs added/changed/removed, open questions.
