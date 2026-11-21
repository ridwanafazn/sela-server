// Package prepublic implements account gates, consent summaries,
// entitlements, and user data export.
package prepublic

import "time"

// ConsentSummary is the user-facing account gate state.
type ConsentSummary struct {
	AdultConfirmed           bool       `json:"adult_confirmed"`
	AdultConfirmedAt         *time.Time `json:"adult_confirmed_at"`
	TermsAccepted            bool       `json:"terms_accepted"`
	TermsVersion             *string    `json:"terms_version"`
	TermsAcceptedAt          *time.Time `json:"terms_accepted_at"`
	PrivacyAccepted          bool       `json:"privacy_accepted"`
	PrivacyVersion           *string    `json:"privacy_version"`
	PrivacyAcceptedAt        *time.Time `json:"privacy_accepted_at"`
	SensitiveMemoryGranted   bool       `json:"sensitive_memory_granted"`
	SensitiveMemoryUpdatedAt *time.Time `json:"sensitive_memory_updated_at"`
}

// ConsentInput is a partial consent update.
type ConsentInput struct {
	AdultConfirmed *bool
	TermsVersion   *string
	PrivacyVersion *string
}

// Entitlement is the provider-independent plan state.
type Entitlement struct {
	Plan             string     `json:"plan"`
	Status           string     `json:"status"`
	CurrentPeriodEnd *time.Time `json:"current_period_end"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
