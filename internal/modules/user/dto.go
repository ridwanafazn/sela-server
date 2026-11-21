package user

import "time"

type updateMeRequest struct {
	DisplayName        OptionalString `json:"display_name"`
	Language           OptionalString `json:"language"`
	Timezone           OptionalString `json:"timezone"`
	CommunicationStyle OptionalString `json:"communication_style"`
}

type userDTO struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	EmailVerified   bool       `json:"email_verified"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type profileDTO struct {
	DisplayName        *string `json:"display_name"`
	Language           string  `json:"language"`
	Timezone           string  `json:"timezone"`
	CommunicationStyle string  `json:"communication_style"`
}

type meDTO struct {
	User    userDTO    `json:"user"`
	Profile profileDTO `json:"profile"`
}

func newMeDTO(me *Me) meDTO {
	return meDTO{
		User: userDTO{
			ID:              me.UserID,
			Email:           me.Email,
			EmailVerified:   me.EmailVerifiedAt != nil,
			EmailVerifiedAt: me.EmailVerifiedAt,
			CreatedAt:       me.UserCreatedAt,
		},
		Profile: profileDTO{
			DisplayName:        me.DisplayName,
			Language:           me.Language,
			Timezone:           me.Timezone,
			CommunicationStyle: me.CommunicationStyle,
		},
	}
}
