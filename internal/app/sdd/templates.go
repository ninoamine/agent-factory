package sdd

// initFile is one file created by /sdd init (SDD-35).
type initFile struct {
	Path    string
	Content string
}

// initDirs are empty directories created by /sdd init.
var initDirs = []string{"specs/features"}

// initFiles are the built-in, language-neutral templates (SDD-35).
var initFiles = []initFile{
	{"AGENTS.md", `# AGENTS.md

Spec-driven project. Specs in ` + "`specs/`" + ` are the source of truth. Code follows specs, never the reverse.

## Read order (minimal, do not read more)

1. ` + "`specs/index.md`" + ` (feature list, status, dependencies)
2. ` + "`specs/common/project.md`, `specs/common/architecture.md`, `specs/common/conventions.md`" + `
3. Only the feature file(s) you were asked to work on, plus the features listed in their ` + "`depends_on`" + `

## Working rules

- Implement only the rules (IDs like ` + "`AUTH-3`" + `) of the target feature. Do not invent requirements.
- Anything listed under "Open questions" is not decided: ask the user.
- Anything in ` + "`specs/roadmap.md`" + ` outside the current version is out of scope.
- If code and spec disagree, stop and report it. Never silently change either.
- Spec changes are made first (see ` + "`specs/templates/change.md`" + `), code second.
- Cite rule IDs in test names and in reports.
- The latest explicit user decision wins; record it in ` + "`specs/decisions.md`" + `.
`},
	{"specs/index.md", `# Spec index

Status: ` + "`draft`" + ` (not ready to implement) | ` + "`ready`" + ` | ` + "`implemented`" + `.

| ID | File | Status | Version | depends_on | Modules |
|---|---|---|---|---|---|
`},
	{"specs/roadmap.md", `# Roadmap

Versions, goals and the features planned for each. Only the current version is buildable.

## V1 (current)

- Goal: <what the first version delivers>
- Features: <list of feature ids once specified>

## Later

- <ideas that are not decided yet>
`},
	{"specs/decisions.md", `# Decisions

Append only. Each line: id, date, decision, reason.

- D-001 <date> <decision> (<reason>)
`},
	{"specs/common/project.md", `# Project

## Vision

<what the product is and who it is for>

## Principles

1. <principle>

## Glossary

| Term | Meaning |
|---|---|
| <term> | <meaning> |
`},
	{"specs/common/architecture.md", `# Architecture

<layers, modules and how they depend on each other>

## Rules

- ARCH-1 <rule, written as a MUST or MUST NOT>
`},
	{"specs/common/conventions.md", `# Conventions

## Code

- CONV-1 <language, formatting and naming rules>

## Testing

- TEST-1 Every rule ID has at least one test whose name contains the ID.

## Checks

<the commands that must pass before a task is finished>
`},
	{"specs/templates/feature.md", `---
id: <ID>
status: draft
version: <V1>
depends_on: []
modules: [<path/to/module>]
---
# <Feature name>

Goal: <one sentence>

## Rules

- <ID>-1 <rule: one testable statement with MUST or MUST NOT>

## Out of scope

<what this feature does not do>

## Acceptance criteria

- AC1 (<ID>-1): <observable check>

## Open questions

None.
`},
	{"specs/templates/change.md", `# Change to <ID>

- What changes: <rules added, changed or removed>
- Why: <reason>
- Rules affected: <IDs; never renumber or reuse an ID>
- Impacts on other features: <list or none>
- Decision to record in specs/decisions.md: <line or none>
`},
	{"specs/templates/roadmap.md", `# Roadmap

## <Version>

- Goal: <outcome>
- Features: <ids and one line each>
- Not now: <what is deferred>
`},
}
