---
id: ABC                      # short uppercase prefix, unique, used in rule IDs (ABC-1, ABC-2 ...)
status: draft                # draft | ready | implemented
version: V1                  # must match specs/roadmap.md
depends_on: []               # feature IDs this one needs, e.g. [CFG, ROUTE]
modules: []                  # source folders touched, e.g. [application/routing]
---
# <Feature name>

Goal: <one sentence: the outcome for the user or the system>.

## Rules

<!-- One behavior per rule. MUST / MUST NOT / SHOULD. Testable. IDs never reused. -->
- ABC-1 MUST <observable behavior>.
- ABC-2 MUST NOT <forbidden behavior>.
- ABC-3 SHOULD <preferred behavior>.

## Out of scope

<!-- What an agent might be tempted to add but must not. -->
- <item>

## Acceptance criteria

<!-- Each maps to rule IDs and becomes a test. Give concrete input and expected output. -->
- AC1 (ABC-1): given <input>, then <expected result>.
- AC2 (ABC-2): given <input>, then <nothing happens / error shown>.

## Open questions

<!-- Anything undecided. Include a proposed answer. The agent must ask before building these. Delete when empty. -->
- Q1 <question>. Proposed: <answer>.
