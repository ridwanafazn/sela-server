package auth

import "time"

type registerRequest struct {
	Email       string  `json:"email" binding:"required,email,max=254"`
	Password    string  `json:"password" binding:"required,min=8,max=1024"`
	DisplayName *string `json:"display_name" binding:"omitempty,max=100"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,max=1024"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required,max=512"`
}

type emailVerificationConfirmRequest struct {
	Token string `json:"token" binding:"required,max=512"`
}

type passwordResetRequest struct {
	Email string `json:"email" binding:"required,email,max=254"`
}

type passwordResetConfirmRequest struct {
	Token       string `json:"token" binding:"required,max=512"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=1024"`
}

type userDTO struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	EmailVerified   bool       `json:"email_verified"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type authData struct {
	User         userDTO `json:"user"`
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int     `json:"expires_in"`
}

func newAuthData(result *AuthResult) authData {
	return authData{
		User: userDTO{
			ID:              result.User.ID,
			Email:           result.User.Email,
			EmailVerified:   result.User.EmailVerifiedAt != nil,
			EmailVerifiedAt: result.User.EmailVerifiedAt,
			CreatedAt:       result.User.CreatedAt,
		},
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
	}
}
