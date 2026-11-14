package user

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

var languagePattern = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// Store is the persistence contract used by the user service.
type Store interface {
	GetMe(ctx context.Context, userID string) (*Me, error)
	UpdateMe(ctx context.Context, userID string, patch ProfilePatch) (*Me, error)
}

// Service applies profile validation and persistence rules.
type Service struct {
	store Store
}

// NewService creates the user service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// GetMe returns the authenticated user's profile.
func (s *Service) GetMe(ctx context.Context, userID string) (*Me, error) {
	return s.store.GetMe(ctx, userID)
}

// UpdateMe validates and applies a partial profile update.
func (s *Service) UpdateMe(ctx context.Context, userID string, patch ProfilePatch) (*Me, error) {
	if patch.IsEmpty() {
		return s.store.GetMe(ctx, userID)
	}

	sanitized := ProfilePatch{}
	var details []apierrors.Detail

	if patch.DisplayName.Set && patch.DisplayName.Value != nil {
		if utf8.RuneCountInString(*patch.DisplayName.Value) > 100 {
			details = append(details, apierrors.Detail{
				Field:   "display_name",
				Code:    "TOO_LONG",
				Message: "Display name must be at most 100 characters.",
			})
		} else {
			sanitized.DisplayName = patch.DisplayName
		}
	} else if patch.DisplayName.Set {
		sanitized.DisplayName = patch.DisplayName
	}

	if patch.Language.Set {
		value, detail := validateLanguage(patch.Language)
		if detail != nil {
			details = append(details, *detail)
		} else {
			sanitized.Language = OptionalString{Set: true, Value: &value}
		}
	}

	if patch.Timezone.Set {
		value, detail := validateTimezone(patch.Timezone)
		if detail != nil {
			details = append(details, *detail)
		} else {
			sanitized.Timezone = OptionalString{Set: true, Value: &value}
		}
	}

	if patch.CommunicationStyle.Set {
		value, detail := validateCommunicationStyle(patch.CommunicationStyle)
		if detail != nil {
			details = append(details, *detail)
		} else {
			sanitized.CommunicationStyle = OptionalString{Set: true, Value: &value}
		}
	}

	if len(details) > 0 {
		return nil, apierrors.Validation("The request is invalid.", details...)
	}

	return s.store.UpdateMe(ctx, userID, sanitized)
}

func validateLanguage(field OptionalString) (string, *apierrors.Detail) {
	if field.Value == nil {
		return "", &apierrors.Detail{
			Field:   "language",
			Code:    "INVALID_VALUE",
			Message: "Language must not be null.",
		}
	}

	value := strings.TrimSpace(*field.Value)
	if len(value) < 2 || len(value) > 10 || !languagePattern.MatchString(value) {
		return "", &apierrors.Detail{
			Field:   "language",
			Code:    "INVALID_FORMAT",
			Message: "Use a language tag such as \"id\" or \"en-US\".",
		}
	}

	return value, nil
}

func validateTimezone(field OptionalString) (string, *apierrors.Detail) {
	if field.Value == nil {
		return "", &apierrors.Detail{
			Field:   "timezone",
			Code:    "INVALID_VALUE",
			Message: "Timezone must not be null.",
		}
	}

	value := strings.TrimSpace(*field.Value)
	if value == "" || len(value) > 100 {
		return "", &apierrors.Detail{
			Field:   "timezone",
			Code:    "INVALID_VALUE",
			Message: "Enter a valid IANA timezone.",
		}
	}

	if _, err := time.LoadLocation(value); err != nil {
		return "", &apierrors.Detail{
			Field:   "timezone",
			Code:    "INVALID_VALUE",
			Message: "Enter a valid IANA timezone.",
		}
	}

	return value, nil
}

func validateCommunicationStyle(field OptionalString) (string, *apierrors.Detail) {
	if field.Value == nil {
		return "", &apierrors.Detail{
			Field:   "communication_style",
			Code:    "INVALID_VALUE",
			Message: "Communication style must not be null.",
		}
	}

	value := *field.Value
	switch value {
	case StyleAdaptive, StyleDirect, StyleGentle, StyleBalanced:
		return value, nil
	default:
		return "", &apierrors.Detail{
			Field:   "communication_style",
			Code:    "INVALID_VALUE",
			Message: "Communication style must be adaptive, direct, gentle, or balanced.",
		}
	}
}
