package flag

import "time"

// Flag is the durable flag document (global settings).
type Flag struct {
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Enabled        bool      `json:"enabled"`
	RolloutPercent int       `json:"rollout_percent"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Override is a per-user force on/off for one flag.
type Override struct {
	FlagName string `json:"flag_name"`
	UserID   string `json:"user_id"`
	Enabled  bool   `json:"enabled"`
}
