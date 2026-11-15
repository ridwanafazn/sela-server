// Package account implements re-authenticated account deactivation and the
// asynchronous purge job.
package account

import "time"

// Account is the authentication identity needed for deletion re-auth.
type Account struct {
	ID           string
	PasswordHash string
	Status       string
	DeletedAt    *time.Time
}

// DeletionView is the API response for an accepted deletion request.
type DeletionView struct {
	Status           string    `json:"status"`
	PurgeScheduledAt time.Time `json:"purge_scheduled_at"`
	Message          string    `json:"message"`
}
