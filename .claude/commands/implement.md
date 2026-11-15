---
description: Implement one feature from its spec
argument-hint: <feature ID, e.g. ROUTE>
---
Implement feature: $ARGUMENTS

1. Read only: `specs/index.md`, `specs/common/project.md`, `specs/common/architecture.md`, `specs/common/conventions.md`, the feature file, and the feature files in its `depends_on`. Read `specs/common/config-files.md` only if config files are involved.
2. If the feature status is `draft`, or it has open questions that affect what you build, stop and ask me.
3. Inspect existing code in the listed `modules`.
4. Post a short plan: rule IDs you will cover, files you will create or change. Wait for my OK only if something is ambiguous.
5. Implement exactly the rules. No extra features, no refactors outside the feature.
6. Write tests: at least one per rule ID, with the ID in the test name (TEST-1). No network, no real keys.
7. Run `gofmt`, `go vet ./...`, `go test ./...`. Fix failures.
8. If code and spec disagree, stop and tell me; do not change the spec silently.
9. Final report (short): rules covered, files changed, checks run, anything not done and why. Do not mark the feature `implemented` in `index.md` unless all rules and acceptance criteria pass.
