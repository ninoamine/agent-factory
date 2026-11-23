package sdd

import "strings"

const proposalFormat = `Return file changes in exactly this format, one block per file, with the FULL new content of the file (never a diff or patch, never a deletion):

=== FILE: <path relative to the project dir> ===
<full file content>
=== END FILE ===

Do not change the status of a feature; the tool does that. Text outside the blocks is allowed and is shown to the user.`

const verdictFormat = `End your answer with one line, exactly "VERDICT: pass" or "VERDICT: fail".`

// instructions are the built-in instruction per subcommand (SDD-41).
var instructions = map[string]string{
	"roadmap": `You maintain the project roadmap. Using the project description, the current roadmap, the template and the decisions, update specs/roadmap.md for the request in the input. If a decision was made, append it to specs/decisions.md (append only: keep every existing line).
` + proposalFormat,

	"spec": `You write a new feature spec. Follow specs/templates/feature.md. Use a unique feature id that is not in specs/index.md, put the file at specs/features/<name>.md, and add a matching row to specs/index.md (keep every existing row). Number rules as <ID>-1, <ID>-2, ... and add acceptance criteria.
` + proposalFormat,

	"spec-change": `You change an existing feature spec. Follow specs/templates/change.md. Keep the feature id. Never renumber or reuse rule IDs. You may append decision lines to specs/decisions.md (append only: keep every existing line). The tool resets the feature status to draft.
` + proposalFormat,

	"spec-validate": `You validate a feature spec: rules are testable and unambiguous, ids are unique, depends_on and modules are right, no open questions affect the build, and it does not contradict the common specs or the dependencies. If it is ready to implement, say so. If not, list the problems and return a corrected spec file.
` + proposalFormat + "\n" + verdictFormat,

	"implement": `You implement exactly the rules of the feature spec, following AGENTS.md and the common specs. Write code and tests only under the feature's modules folders. Put the rule ID in each test name. Do not add features beyond the rules.
` + proposalFormat,

	"spec-review": `You review the code under the feature's modules against its spec: for each rule ID, say covered or not, and list gaps. Do not return file blocks.
` + verdictFormat,

	"review": `You review the uncommitted changes against the conventions and the specs of the touched features: correctness, rule violations, missing tests, security. List findings by severity. Do not return file blocks.
` + verdictFormat,
}

var subList = []string{"init", "roadmap <input>", "spec <input>", "spec <id> <input>", "spec-validate <id>", "implement <id>", "spec-review <id>", "review"}

func subcommandList() string {
	return "  /sdd " + strings.Join(subList, "\n  /sdd ")
}
