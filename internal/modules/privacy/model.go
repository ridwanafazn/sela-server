// Package privacy stores user-controlled AI context and memory preferences.
package privacy

import "time"

// AI context modes.
const (
	ContextNone                = "none"
	ContextJournals            = "journals"
	ContextJournalsAndMemories = "journals_and_memories"
)

// Preferences is the persisted privacy state.
type Preferences struct {
	AIContextMode           string
	MemoryExtractionEnabled bool
	UpdatedAt               time.Time
}

// View is the API representation of privacy preferences and relevant consent
// state.
type View struct {
	AIContextMode           string    `json:"ai_context_mode"`
	MemoryExtractionEnabled bool      `json:"memory_extraction_enabled"`
	SensitiveMemoryGranted  bool      `json:"sensitive_memory_granted"`
	AdultConfirmed          bool      `json:"adult_confirmed"`
	TermsVersion            *string   `json:"terms_version"`
	PrivacyVersion          *string   `json:"privacy_version"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// UpdateInput is a partial privacy update.
type UpdateInput struct {
	AIContextMode           *string
	MemoryExtractionEnabled *bool
}
