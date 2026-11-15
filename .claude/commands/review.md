---
description: Review the current changes against specs and conventions
---
Review the uncommitted changes (use `git diff` and `git status`).

1. Read `specs/common/conventions.md`, `specs/common/architecture.md`, and only the feature specs relevant to the changed files.
2. Check: rule IDs respected, nothing from `specs/roadmap.md` "Not in V1" added, layer boundaries kept (ARCH-1 to ARCH-10), no secrets or API keys in code, logs, tests, or fixtures, user-owned files never written, tests exist per rule ID.
3. Report findings by severity (blocker / should fix / nit) with file:line and the rule ID violated. Do not edit files unless I ask.
