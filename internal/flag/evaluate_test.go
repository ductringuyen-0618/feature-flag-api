package flag_test

import (
	"fmt"
	"testing"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name     string
		flag     flag.Flag
		userID   string
		override *flag.Override
		wantOn   bool
		wantR    flag.Reason
	}{
		{
			name:     "override wins over kill switch",
			flag:     flag.Flag{Name: "x", Enabled: false, RolloutPercent: 100},
			userID:   "u1",
			override: &flag.Override{FlagName: "x", UserID: "u1", Enabled: true},
			wantOn:   true,
			wantR:    flag.ReasonOverride,
		},
		{
			name:     "override off",
			flag:     flag.Flag{Name: "x", Enabled: true, RolloutPercent: 100},
			userID:   "u1",
			override: &flag.Override{FlagName: "x", UserID: "u1", Enabled: false},
			wantOn:   false,
			wantR:    flag.ReasonOverride,
		},
		{
			name:   "kill switch ignores rollout",
			flag:   flag.Flag{Name: "x", Enabled: false, RolloutPercent: 100},
			userID: "u1",
			wantOn: false,
			wantR:  flag.ReasonFlagDisabled,
		},
		{
			name:   "boolean on at 100 percent",
			flag:   flag.Flag{Name: "x", Enabled: true, RolloutPercent: 100},
			userID: "u1",
			wantOn: true,
			wantR:  flag.ReasonBooleanToggle,
		},
		{
			name:   "zero percent excludes all",
			flag:   flag.Flag{Name: "x", Enabled: true, RolloutPercent: 0},
			userID: "anyone",
			wantOn: false,
			wantR:  flag.ReasonPercentageExcluded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flag.Evaluate(&tt.flag, tt.userID, tt.override)
			if got.Enabled != tt.wantOn || got.Reason != tt.wantR {
				t.Fatalf("Evaluate = %+v, want enabled=%v reason=%s", got, tt.wantOn, tt.wantR)
			}
		})
	}
}

func TestEvaluateStickyRollout(t *testing.T) {
	f := flag.Flag{Name: "checkout", Enabled: true, RolloutPercent: 20}
	first := flag.Evaluate(&f, "alice", nil)
	for i := 0; i < 50; i++ {
		got := flag.Evaluate(&f, "alice", nil)
		if got != first {
			t.Fatalf("unstable: first=%+v got=%+v", first, got)
		}
	}
}

func TestEvaluateDifferentFlagsDifferentBuckets(t *testing.T) {
	// Same user should not be forced into the same bucket for every flag.
	user := "usr_sticky"
	for i := 0; i < 500; i++ {
		nameA := fmt.Sprintf("flag-a-%d", i)
		nameB := fmt.Sprintf("flag-b-%d", i)
		a := flag.Evaluate(&flag.Flag{Name: nameA, Enabled: true, RolloutPercent: 20}, user, nil)
		b := flag.Evaluate(&flag.Flag{Name: nameB, Enabled: true, RolloutPercent: 20}, user, nil)
		if a.Enabled != b.Enabled {
			return // proved flag-scoped hash
		}
	}
	t.Fatal("expected some flag-scoped divergence across 500 pairs")
}

func TestRolloutMonotonic(t *testing.T) {
	user := "bob"
	name := "gradual"
	wasIn := false
	for pct := 0; pct <= 100; pct++ {
		got := flag.Evaluate(&flag.Flag{Name: name, Enabled: true, RolloutPercent: pct}, user, nil)
		if wasIn && !got.Enabled {
			t.Fatalf("user fell out of rollout when percent rose to %d", pct)
		}
		if got.Enabled {
			wasIn = true
		}
	}
	if !wasIn {
		t.Fatal("user never entered rollout by 100%")
	}
}
