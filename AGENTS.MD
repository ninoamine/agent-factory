# AGENTS.md
 
Spec-driven project. Specs in `specs/` are the source of truth. Code follows specs, never the reverse.
 
## Read order (minimal, do not read more)
 
1. `specs/index.md` (feature list, status, dependencies)
2. `specs/common/project.md`, `specs/common/architecture.md`, `specs/common/conventions.md`
3. Only the feature file(s) you were asked to work on, plus the features listed in their `depends_on`
4. `specs/common/config-files.md` only if the task touches files in `~/.agent-factory/`
Do NOT read other feature files unless a rule ID points to them.
 
## Working rules
 
- Implement only the rules (IDs like `ROUTE-3`) of the target feature. Do not invent requirements.
- Anything marked `proposed` or listed under "Open questions" is NOT decided: ask the user before building it.
- Anything in `specs/roadmap.md` outside the current version is out of scope. Do not build it.
- If code and spec disagree, stop and report it. Never silently change either.
- Spec changes are made first (see `specs/templates/change.md`), code second.
- Cite rule IDs in test names and in the final report.
- Latest explicit user decision wins; record it in `specs/decisions.md`.

## Workflow per task
 
1. Read per the order above. 2. Summarize plan: rule IDs, files to touch. 3. Ask about open questions. 4. Implement. 5. Tests for every rule ID. 6. Run `gofmt`, `go vet ./...`, `go test ./...`. 7. Update `README.md` and CI if needed (DOC-1 to DOC-6). 8. Report: rules done, checks run, README/CI changed or not, limitations.
Slash commands: `/spec`, `/implement`, `/spec-check`, `/review` (see `.claude/commands/`).
 
Coding, security, testing and dependency rules: `specs/common/conventions.md`.
 