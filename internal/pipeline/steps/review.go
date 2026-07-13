package steps

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kunchenguid/no-mistakes/internal/agent"
	"github.com/kunchenguid/no-mistakes/internal/git"
	"github.com/kunchenguid/no-mistakes/internal/pipeline"
	"github.com/kunchenguid/no-mistakes/internal/types"
)

// ReviewStep reviews the diff for bugs, security issues, and doc gaps.
type ReviewStep struct{}

func (s *ReviewStep) Name() types.StepName { return types.StepReview }

func (s *ReviewStep) Execute(sctx *pipeline.StepContext) (*pipeline.StepOutcome, error) {
	ctx := sctx.Ctx
	baseSHA := resolveBranchBaseSHA(ctx, sctx.WorkDir, sctx.Run.BaseSHA, sctx.Repo.DefaultBranch)
	branch := sctx.Run.Branch
	ignorePatterns := "none"
	if len(sctx.Config.IgnorePatterns) > 0 {
		ignorePatterns = strings.Join(sctx.Config.IgnorePatterns, ", ")
	}

	reviewScope := fmt.Sprintf("branch changes between %s and %s", baseSHA, sctx.Run.HeadSHA)
	if sctx.Fixing {
		reviewScope = fmt.Sprintf("current worktree and HEAD changes relative to base commit %s (starting head %s)", baseSHA, sctx.Run.HeadSHA)
	}

	// Bounded workload size (changed files + net lines) for local telemetry, so
	// review/fix efficiency can be normalized without external git archaeology.
	// Best-effort: a diff-stat failure leaves the workload unknown.
	workload := reviewWorkload(ctx, sctx.WorkDir, baseSHA, sctx.Run.HeadSHA)

	// In fix mode, ask the agent to fix issues first.
	//
	// The verification-discipline rules below (apply all fixes first, then one
	// focused verification of the changed area, and never run the whole repo
	// test/lint suite in the fixer round) exist for wall-clock reasons: a
	// forensic audit of a real multi-round run measured the fixer re-running the
	// entire test+lint suite ~5x per round (27 runs across 5 rounds, ~784s of
	// the 2419s review step), plus the model round-trips that poll those long
	// subprocesses. Review runs before the dedicated Test and Lint steps
	// (pipeline order in common.go), which are the authoritative test and lint
	// gates; their coverage may be focused when the repository has no configured
	// commands. The fixer prohibition stays universal because the fixer only
	// needs to confirm its own edits hold, not re-gate the whole repository. This
	// mirrors the same "relevant"-scoped, cross-tool-forbidden discipline the
	// test and lint fix prompts already carry. The instruction is a contract,
	// not an enforced sandbox - the agent has free shell access - so the pinned
	// regression tests guard the wording, not the runtime.
	var fixSummary string
	if sctx.Fixing {
		previousFindings := sanitizedPreviousFindingsForPrompt(sctx.PreviousFindings)
		historySection := executionContextPromptSection() + roundHistoryPromptSection(sctx) + userIntentPromptSection(sctx)
		fixPrompt := fmt.Sprintf(
			`Investigate previous review findings and address legitimate ones.

Examine the relevant code yourself and apply fixes directly.

Context:
- branch: %s
- base commit: %s
- target commit: %s
- review scope: %s
- default branch: %s
- ignore patterns: %s

%s%s

Previous review findings to address:
%s`,
			branch,
			baseSHA,
			sctx.Run.HeadSHA,
			reviewScope,
			sctx.Repo.DefaultBranch,
			ignorePatterns,
			promptInstructions("review-fix", defaultReviewFixInstructions),
			historySection,
			previousFindings,
		)
		summary, err := executeFixMode(sctx, s.Name(), fixExecutionOptions{
			RequirePreviousFindings: true,
			MissingFindingsError:    "review fix requires previous review findings",
			LogMessage:              "asking agent to fix identified issues...",
			Prompt:                  fixPrompt,
			ErrorPrefix:             "agent fix",
			FallbackSummary:         "address review findings",
			SessionRole:             pipeline.SessionRoleFixer,
			Purpose:                 "review-fix",
			Workload:                workload,
		})
		if err != nil {
			return nil, err
		}
		fixSummary = summary
	}

	// Check whether there are any reviewable changed files after applying ignore patterns.
	var args []string
	if sctx.Fixing {
		args = []string{"diff", "--name-only", baseSHA}
	} else {
		args = []string{"diff", "--name-only", baseSHA + ".." + sctx.Run.HeadSHA}
	}
	changedFiles, err := git.Run(ctx, sctx.WorkDir, args...)
	if err != nil {
		return nil, fmt.Errorf("get changed files: %w", err)
	}

	hasReviewableChanges := false
	for _, path := range strings.Split(changedFiles, "\n") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		ignored := false
		for _, pattern := range sctx.Config.IgnorePatterns {
			if matchIgnorePattern(path, pattern) {
				ignored = true
				break
			}
		}
		if !ignored {
			hasReviewableChanges = true
			break
		}
	}

	if !hasReviewableChanges {
		sctx.Log("no changes to review")
		noChangeFindings := Findings{
			RiskLevel:     "low",
			RiskRationale: "no reviewable changes",
		}
		findingsJSON, _ := json.Marshal(noChangeFindings)
		return &pipeline.StepOutcome{
			Findings:   string(findingsJSON),
			FixSummary: fixSummary,
		}, nil
	}

	// Ask agent to review
	sctx.Log("reviewing changes...")

	// The review turn (initial and every post-fix rereview) carries the intent
	// conformance obligation: when the intent is authoritative acceptance
	// criteria (explicit --intent), a change that contradicts it must park via
	// an ask-user finding. The clause is empty for inferred intent, leaving the
	// prompt unchanged. This is what makes a fixer round that removed a
	// required behavior park instead of silently completing.
	//
	// TODO(intent-conformance-C, HELD): add the deterministic, zero-LLM
	// net-deleted-author-lines git-diff backstop for the removal-of-required
	// class - a fixer round that net-deletes author-added lines parks
	// regardless of intent source. Held pending a scope decision.
	historySection := executionContextPromptSection() + roundHistoryPromptSection(sctx) + userIntentPromptSection(sctx) + intentConformanceReviewClause(sctx)

	prompt := fmt.Sprintf(
		`Review the code changes and return structured findings with a risk assessment.

Context:
- branch: %s
- base commit: %s
- target commit: %s
- review scope: %s
- default branch: %s
- ignore patterns: %s

%s%s`,
		branch,
		baseSHA,
		sctx.Run.HeadSHA,
		reviewScope,
		sctx.Repo.DefaultBranch,
		ignorePatterns,
		promptInstructions("review", defaultReviewInstructions),
		historySection,
	)

	// Every review turn - the initial review and every post-fix rereview -
	// resumes the run's single durable reviewer session. The prompt above
	// still demands a full review of the complete branch diff each turn; the
	// session only carries the reviewer's own prior context, never the
	// fixer's (that role has its own isolated session in executeFixMode).
	result, err := sctx.RunAgentSession(pipeline.SessionRoleReviewer, agent.RunOpts{
		Prompt:     prompt,
		CWD:        sctx.WorkDir,
		JSONSchema: reviewFindingsSchema,
		OnChunk:    sctx.LogChunk,
		Purpose:    "review",
		Workload:   workload,
	})
	if err != nil {
		return nil, fmt.Errorf("agent review: %w", err)
	}

	// Parse structured findings
	var findings Findings
	if result.Output != nil {
		if err := json.Unmarshal(result.Output, &findings); err != nil {
			sctx.Log("could not parse structured output, using text response")
			findings = Findings{Summary: result.Text}
		}
	}

	needsApproval := hasBlockingFindings(findings.Items)
	findingsJSON, _ := json.Marshal(findings)

	return &pipeline.StepOutcome{
		NeedsApproval: needsApproval,
		AutoFixable:   len(findings.Items) > 0,
		Findings:      string(findingsJSON),
		FixSummary:    fixSummary,
	}, nil
}

func sanitizedPreviousFindingsForPrompt(raw string) string {
	findings, err := types.ParseFindingsJSON(raw)
	if err != nil {
		return sanitizePromptMultilineText(raw)
	}
	for i := range findings.Items {
		findings.Items[i].ID = sanitizePromptText(findings.Items[i].ID)
		findings.Items[i].Severity = sanitizePromptText(findings.Items[i].Severity)
		findings.Items[i].File = sanitizePromptText(findings.Items[i].File)
		findings.Items[i].Description = sanitizePromptMultilineText(findings.Items[i].Description)
		findings.Items[i].Source = sanitizePromptText(findings.Items[i].Source)
		findings.Items[i].UserInstructions = sanitizePromptMultilineText(findings.Items[i].UserInstructions)
	}
	findings.Summary = sanitizePromptMultilineText(findings.Summary)
	findings.RiskLevel = sanitizePromptText(findings.RiskLevel)
	findings.RiskRationale = sanitizePromptMultilineText(findings.RiskRationale)
	encoded, err := types.MarshalFindingsJSON(findings)
	if err != nil {
		return sanitizePromptMultilineText(raw)
	}
	return encoded
}

func sanitizePromptText(text string) string {
	return strings.Join(strings.Fields(sanitizePromptMultilineText(text)), " ")
}

func sanitizePromptMultilineText(text string) string {
	text = strings.NewReplacer("<<<<<<<", " ", "=======", " ", ">>>>>>>", " ").Replace(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.Join(strings.Fields(lines[i]), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
