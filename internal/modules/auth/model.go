package auth

import "time"

// User statuses from the locked database design.
const (
	UserStatusActive          = "active"
	UserStatusDeletionPending = "deletion_pending"
	UserStatusDeleted         = "deleted"
)

// User is the authentication identity.
type User struct {
	ID                  string     `gorm:"column:id;primaryKey;type:uuid"`
	Email               string     `gorm:"column:email;type:citext"`
	PasswordHash        string     `gorm:"column:password_hash"`
	Status              string     `gorm:"column:status"`
	EmailVerifiedAt     *time.Time `gorm:"column:email_verified_at"`
	DeletionRequestedAt *time.Time `gorm:"column:deletion_requested_at"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
	DeletedAt           *time.Time `gorm:"column:deleted_at"`
}

// TableName pins the table name.
func (User) TableName() string {
	return "users"
}

// Session stores refresh-session metadata and rotation state.
type Session struct {
	ID               string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID           string     `gorm:"column:user_id;type:uuid"`
	TokenFamilyID    string     `gorm:"column:token_family_id;type:uuid"`
	RefreshTokenHash string     `gorm:"column:refresh_token_hash"`
	ExpiresAt        time.Time  `gorm:"column:expires_at"`
	RevokedAt        *time.Time `gorm:"column:revoked_at"`
	ReplacedBy       *string    `gorm:"column:replaced_by;type:uuid"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	LastUsedAt       *time.Time `gorm:"column:last_used_at"`
}

// TableName pins the table name.
func (Session) TableName() string {
	return "auth_sessions"
}

// SessionPrincipal is the joined session and user state used by the
// authentication middleware on every protected request.
type SessionPrincipal struct {
	SessionID string
	UserID    string
	Status    string
	RevokedAt *time.Time
	ExpiresAt time.Time
	DeletedAt *time.Time
}
