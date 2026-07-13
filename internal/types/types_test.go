package types

import (
	"encoding/json"
	"testing"
)

func TestAllStepsOrder(t *testing.T) {
	steps := AllSteps()
	if len(steps) != 9 {
		t.Fatalf("expected 9 steps, got %d", len(steps))
	}

	expected := []StepName{StepIntent, StepRebase, StepReview, StepTest, StepDocument, StepLint, StepPush, StepPR, StepCI}
	for i, s := range steps {
		if s != expected[i] {
			t.Errorf("step[%d] = %q, want %q", i, s, expected[i])
		}
	}
}

func TestStepNameOrder(t *testing.T) {
	tests := []struct {
		step StepName
		want int
	}{
		{StepIntent, 1},
		{StepRebase, 2},
		{StepReview, 3},
		{StepTest, 4},
		{StepDocument, 5},
		{StepLint, 6},
		{StepPush, 7},
		{StepPR, 8},
		{StepCI, 9},
		{StepName("unknown"), 0},
	}

	for _, tt := range tests {
		if got := tt.step.Order(); got != tt.want {
			t.Errorf("%q.Order() = %d, want %d", tt.step, got, tt.want)
		}
	}
}

func TestStepNameUnmarshalJSON_LegacyBabysit(t *testing.T) {
	var step StepName
	if err := json.Unmarshal([]byte(`"babysit"`), &step); err != nil {
		t.Fatalf("unmarshal step name: %v", err)
	}
	if step != StepCI {
		t.Fatalf("step = %q, want %q", step, StepCI)
	}
}

func TestStepsAfter(t *testing.T) {
	if got := StepsAfter(StepLint); len(got) != 3 || got[0] != StepPush || got[1] != StepPR || got[2] != StepCI {
		t.Errorf("StepsAfter(lint) = %v, want [push pr ci]", got)
	}
	if got := StepsAfter(StepCI); got != nil {
		t.Errorf("StepsAfter(ci) = %v, want nil", got)
	}
	if got := StepsAfter(StepName("")); got != nil {
		t.Errorf("StepsAfter(empty) = %v, want nil (fail open)", got)
	}
	if got := StepsAfter(StepName("bogus")); got != nil {
		t.Errorf("StepsAfter(bogus) = %v, want nil (fail open)", got)
	}
	if got := StepsAfter(StepName("babysit")); got != nil {
		t.Errorf("StepsAfter(babysit) = %v, want nil (normalizes to ci)", got)
	}
}
