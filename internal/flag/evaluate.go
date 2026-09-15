package flag

import "hash/fnv"

// Evaluate applies rule A:
//  1. per-user override if present
//  2. enabled == false → kill switch off
//  3. rollout_percent bucket via fnv32a(name+":"+userID)%100
//  4. else enabled (true) boolean toggle
//
// flag must be non-nil. override may be nil.
func Evaluate(f *Flag, userID string, override *Override) Result {
	if override != nil {
		return Result{FlagName: f.Name, Enabled: override.Enabled, Reason: ReasonOverride}
	}
	if !f.Enabled {
		return Result{FlagName: f.Name, Enabled: false, Reason: ReasonFlagDisabled}
	}
	if f.RolloutPercent < 100 {
		if int(rolloutBucket(f.Name, userID)) < f.RolloutPercent {
			return Result{FlagName: f.Name, Enabled: true, Reason: ReasonPercentageRollout}
		}
		return Result{FlagName: f.Name, Enabled: false, Reason: ReasonPercentageExcluded}
	}
	return Result{FlagName: f.Name, Enabled: true, Reason: ReasonBooleanToggle}
}

func rolloutBucket(flagName, userID string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(flagName + ":" + userID))
	return h.Sum32() % 100
}
