---
description: Audit code against a feature spec (read-only)
argument-hint: <feature ID, e.g. HIST>
---
Audit feature $ARGUMENTS. Read-only: do not edit any file.

1. Read the feature spec, `specs/common/architecture.md`, `specs/common/conventions.md`.
2. For every rule ID, find the code that implements it and the test that covers it.
3. Output a table: Rule ID | Implemented (yes/partial/no) | Test (name or none) | Note (file:line).
4. List code behavior that no rule justifies (possible scope creep).
5. List spec statements that are ambiguous or untestable.
6. End with a verdict: ready to mark `implemented`, or the list of gaps. Keep it concise.
