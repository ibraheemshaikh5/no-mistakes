package steps

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kunchenguid/no-mistakes/internal/paths"
)

// Review-prompt externalization (fork feature).
//
// The static instruction blocks of the review and review-fix prompts - the
// review strategy - can be overridden per machine by dropping Markdown files
// under <NM_HOME>/prompts/ (default ~/.no-mistakes/prompts/):
//
//	prompts/review.md      replaces defaultReviewInstructions
//	prompts/review-fix.md  replaces defaultReviewFixInstructions
//
// Only the instruction body is overridable. The dynamic scaffolding around it
// (run context, round history, sanitized intent, previous findings, and the
// JSON output schema) stays in Go, so an override can change what the reviewer
// hunts for but never the structured-findings contract the pipeline parses.
// A missing, empty, or unreadable file falls back to the embedded default, so
// the gate keeps working on a fresh machine.
//
// The override lives in NM_HOME (the operator's own state dir), which is the
// same trust domain as the global config: it is never read from the repo or
// the pushed branch, so a contributor cannot weaken the review this way.

// promptOverridePath returns the on-disk override location for a named prompt.
func promptOverridePath(name string) string {
	p, err := paths.New()
	if err != nil {
		return ""
	}
	return filepath.Join(p.Root(), "prompts", name+".md")
}

// promptInstructions returns the operator override for the named prompt when
// one exists and is non-empty, otherwise the embedded fallback.
func promptInstructions(name, fallback string) string {
	path := promptOverridePath(name)
	if path == "" {
		return fallback
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return fallback
	}
	return text
}

// defaultReviewInstructions is the embedded instruction block of the review
// prompt (override: prompts/review.md). The wording of the Task/Rules/Risk
// sections is a pinned contract; see the review-step tests before editing.
const defaultReviewInstructions = `Task:
- Read the relevant history and diff yourself.
- Focus findings on risks introduced by changed code, but inspect surrounding code, call sites, shared helpers, tests, and invariants when needed to understand root cause.
- Do NOT run tests during review. The pipeline has a dedicated test step after review.
- Analyze for bugs, risks, and code simplification opportunities.
- "Simplification" means reducing code complexity through non-functional refactoring (e.g. deduplication, clearer control flow). It does NOT mean removing features, changing product behavior, or stripping intentional user-facing output.
- Treat security issues, performance regressions, breaking changes, and insufficient error handling as risks.
- Do a full review pass before returning. Do not stop after the first valid finding. Continue inspecting the rest of the changed code until you have enumerated all material issues you can substantiate.

Rules:
- Anchor every finding to a specific file and one-indexed line number in the changed code when possible.
- Use severity "error" for problems that should absolutely not get merged, "warning" for things that are worth addressing but can be done in a follow up, and "info" for things that are nice to have.
- Be concise and actionable. No generic advice like "add more tests".
- Only comment on things that genuinely matter.
- Do NOT report styling, formatting, linting, compilation, or type-checking issues.
- If the change is clean, return an empty findings array.
- For each finding, set the action field to one of:
  - "ask-user": the finding is about functional requirements or product behavior, or otherwise challenges the author's deliberate intent. Even if it seems obviously wrong, we should ask the user for review. Examples: "this feature seems unnecessary", "this hardcoded value should be configurable", "this deletion looks wrong". When in doubt, default to "ask-user".
  - "auto-fix": the finding is a non-functional, non user-visible issue (correctness, error handling, security, performance, mechanical code quality) that can be safely fixed without any discussion about the author's intent.
  - "no-op": the finding is informational and does not require any action (e.g. noting a pattern, acknowledging a tradeoff).

Risk assessment (after listing all findings):
- Set risk_level to "low" if the change is well-bounded, mostly cosmetic, or straightforward with little ambiguity.
- Set risk_level to "medium" if the change has room to improve but is safe to merge first with concerns addressed as follow-ups.
- Set risk_level to "high" if the change should not be merged without explicit human approval - it is fundamental, risky, ambiguous, or has strong negative signals.
- Provide a one-sentence risk_rationale explaining why you chose that risk level.`

// defaultReviewFixInstructions is the embedded rules block of the review-fix
// prompt (override: prompts/review-fix.md). The verification-discipline
// wording is a pinned contract; see TestReviewStep_FixMode_FocusedVerificationContract.
const defaultReviewFixInstructions = `Rules:
- Always start with double checking whether the findings are legitimate.
- Before changing code, identify whether each finding is a local defect or a symptom of a deeper design, abstraction, validation, ownership, or test-coverage flaw. Prefer the smallest correct root-cause fix within the changed area over patching only the reported line.
- If a narrow fix would leave the same class of bug likely elsewhere, fix the deepest practical cause instead.
- Avoid resolving a finding by removing or reverting the author's intentional code in their original 1st commit. If the original change introduced something on purpose, fix it forward (e.g. add validation, handle edge cases, tighten logic) rather than deleting it. Similarly, if the original change intentionally deleted or simplified code, do not restore or re-add the removed code unless the finding is a legitimate correctness, reliability, or security issue and the smallest reasonable fix happens to reintroduce a small amount of previously deleted logic. When in doubt about whether code is intentional, leave it and report the finding as unresolved.
- Do not add code comments explaining your fixes.
- Apply all the fixes you intend to make first; do not run any verification in between individual fixes.
- After all fixes are applied, run one focused verification limited to the changed area (the specific package, file, or test you touched) at the end of the fix round to confirm the fixes hold.
- Do NOT run the complete repository test suite or lint suite during this fix round. The pipeline has dedicated test and lint steps after review that are the authoritative test and lint gates; their coverage may itself be focused on the changed area when the repository has no configured test or lint commands.
- Return JSON with a single "summary" field when you are done.
- The summary must be one concise sentence fragment suitable for a git commit subject.
- Keep the summary under 10 words.`
