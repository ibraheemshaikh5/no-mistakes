package config

import (
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/types"
)

func TestParseEndAfter(t *testing.T) {
	cases := []struct {
		value   string
		want    types.StepName
		wantErr bool
	}{
		{"", "", false},
		{"lint", types.StepLint, false},
		{"  CI  ", types.StepCI, false},
		{"push", types.StepPush, false},
		{"pr", types.StepPR, false},
		{"review", types.StepReview, false},
		{"deploy", "", true},
		{"lint,push", "", true},
	}
	for _, tc := range cases {
		got, err := ParseEndAfter(tc.value)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseEndAfter(%q) expected error, got %q", tc.value, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseEndAfter(%q) unexpected error: %v", tc.value, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseEndAfter(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestMerge_EndAfterDefaultsToLint(t *testing.T) {
	cfg := Merge(DefaultGlobalConfig(), &RepoConfig{})
	if cfg.EndAfter != DefaultEndAfter {
		t.Errorf("EndAfter = %q, want default %q", cfg.EndAfter, DefaultEndAfter)
	}
}

func TestMerge_EndAfterRepoOverridesGlobal(t *testing.T) {
	global := DefaultGlobalConfig()
	global.Pipeline.EndAfter = "push"
	repo := &RepoConfig{Pipeline: PipelineRaw{EndAfter: "ci"}}
	cfg := Merge(global, repo)
	if cfg.EndAfter != types.StepCI {
		t.Errorf("EndAfter = %q, want repo override %q", cfg.EndAfter, types.StepCI)
	}

	repoUnset := &RepoConfig{}
	cfg = Merge(global, repoUnset)
	if cfg.EndAfter != types.StepPush {
		t.Errorf("EndAfter = %q, want global %q", cfg.EndAfter, types.StepPush)
	}
}

func TestMerge_EndAfterInvalidFallsBack(t *testing.T) {
	global := DefaultGlobalConfig()
	global.Pipeline.EndAfter = "not-a-step"
	cfg := Merge(global, &RepoConfig{})
	if cfg.EndAfter != DefaultEndAfter {
		t.Errorf("EndAfter = %q, want default fallback %q", cfg.EndAfter, DefaultEndAfter)
	}
}

// pipeline.end_after gates whether the run pushes with the maintainer's
// credentials, so it must be honored only from the trusted default-branch
// copy: a pushed branch cannot re-enable auto-push for its own run.
func TestEffectiveRepoConfig_PipelineTrustedOnly(t *testing.T) {
	pushed := &RepoConfig{Pipeline: PipelineRaw{EndAfter: "ci"}}
	trusted := &RepoConfig{Pipeline: PipelineRaw{EndAfter: "push"}}

	effective := EffectiveRepoConfig(pushed, trusted, false)
	if effective.Pipeline.EndAfter != "push" {
		t.Errorf("Pipeline.EndAfter = %q, want trusted %q", effective.Pipeline.EndAfter, "push")
	}

	effective = EffectiveRepoConfig(pushed, nil, false)
	if effective.Pipeline.EndAfter != "" {
		t.Errorf("Pipeline.EndAfter with no trusted copy = %q, want empty", effective.Pipeline.EndAfter)
	}
}

func TestLoadRepo_ParsesPipelineEndAfter(t *testing.T) {
	cfg, err := LoadRepoFromBytes([]byte("pipeline:\n  end_after: ci\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Pipeline.EndAfter != "ci" {
		t.Errorf("Pipeline.EndAfter = %q, want %q", cfg.Pipeline.EndAfter, "ci")
	}
}
