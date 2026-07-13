package steps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPromptInstructions_FallsBackWithoutOverride(t *testing.T) {
	t.Setenv("NM_HOME", t.TempDir())
	got := promptInstructions("review", defaultReviewInstructions)
	if got != defaultReviewInstructions {
		t.Errorf("expected embedded default without an override file, got %q", got)
	}
}

func TestPromptInstructions_HonorsOverrideFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NM_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "Task:\n- Hunt for race conditions only.\n"
	if err := os.WriteFile(filepath.Join(home, "prompts", "review.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	got := promptInstructions("review", defaultReviewInstructions)
	if got != "Task:\n- Hunt for race conditions only." {
		t.Errorf("override not honored, got %q", got)
	}
}

func TestPromptInstructions_EmptyOverrideFallsBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NM_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "prompts", "review-fix.md"), []byte("  \n\t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := promptInstructions("review-fix", defaultReviewFixInstructions)
	if got != defaultReviewFixInstructions {
		t.Errorf("blank override must fall back to the embedded default")
	}
}
