package flag

type Reason string

const (
	ReasonOverride            Reason = "OVERRIDE"
	ReasonFlagDisabled        Reason = "FLAG_DISABLED"
	ReasonPercentageRollout   Reason = "PERCENTAGE_ROLLOUT"
	ReasonPercentageExcluded  Reason = "PERCENTAGE_EXCLUDED"
	ReasonBooleanToggle       Reason = "BOOLEAN_TOGGLE"
)

// Result is the evaluation outcome for one flag and user.
type Result struct {
	FlagName string `json:"flag_name"`
	Enabled  bool   `json:"enabled"`
	Reason   Reason `json:"reason"`
}
