package user

import (
	"time"

	"github.com/ridwanafazn/sela-server/internal/shared/optional"
)

// Communication styles accepted by the product.
const (
	StyleAdaptive = "adaptive"
	StyleDirect   = "direct"
	StyleGentle   = "gentle"
	StyleBalanced = "balanced"
)

// OptionalString distinguishes an omitted JSON field from an explicit null.
type OptionalString = optional.String

// Me is the joined current-user and profile view.
type Me struct {
	UserID             string
	Email              string
	UserCreatedAt      time.Time
	DisplayName        *string
	Language           string
	Timezone           string
	CommunicationStyle string
}

// ProfilePatch describes the fields to change. Unset fields are preserved.
type ProfilePatch struct {
	DisplayName        OptionalString
	Language           OptionalString
	Timezone           OptionalString
	CommunicationStyle OptionalString
}

// IsEmpty reports whether the patch changes nothing.
func (p ProfilePatch) IsEmpty() bool {
	return !p.DisplayName.Set && !p.Language.Set && !p.Timezone.Set && !p.CommunicationStyle.Set
}
