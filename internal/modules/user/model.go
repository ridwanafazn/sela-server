package user

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// Communication styles accepted by the product.
const (
	StyleAdaptive = "adaptive"
	StyleDirect   = "direct"
	StyleGentle   = "gentle"
	StyleBalanced = "balanced"
)

// OptionalString distinguishes an omitted JSON field from an explicit null.
type OptionalString struct {
	Set   bool
	Value *string
}

// UnmarshalJSON records presence and supports explicit null.
func (o *OptionalString) UnmarshalJSON(data []byte) error {
	o.Set = true

	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = nil
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode optional string: %w", err)
	}

	o.Value = &value
	return nil
}

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
