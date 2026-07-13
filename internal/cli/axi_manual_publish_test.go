package cli

import (
	"strings"
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/types"
)

func TestRunViewPushSkipped(t *testing.T) {
	rv := runView{Steps: []stepView{
		{Name: string(types.StepLint), Status: string(types.StepStatusCompleted)},
		{Name: string(types.StepPush), Status: string(types.StepStatusSkipped)},
	}}
	if !rv.pushSkipped() {
		t.Errorf("pushSkipped() = false for a skipped push step")
	}
	rv.Steps[1].Status = string(types.StepStatusCompleted)
	if rv.pushSkipped() {
		t.Errorf("pushSkipped() = true for a completed push step")
	}
	if (runView{}).pushSkipped() {
		t.Errorf("pushSkipped() = true with no push step row")
	}
}

func TestManualPublishHelp(t *testing.T) {
	help := manualPublishHelp("feat-x")
	for _, want := range []string{"git push origin feat-x", "gh pr create", "do NOT run them yourself"} {
		if !strings.Contains(help, want) {
			t.Errorf("manualPublishHelp missing %q in %q", want, help)
		}
	}
	if !strings.Contains(manualPublishHelp(""), "<branch>") {
		t.Errorf("empty branch must render a placeholder")
	}
}
