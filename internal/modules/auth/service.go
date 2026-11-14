package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/platform/ratelimit"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/ids"
	"github.com/ridwanafazn/sela-server/internal/shared/requestid"
)

// Rate-limit actions and windows.
const (
	actionLoginIP      = "auth.login.ip"
	actionLoginAccount = "auth.login.account"
	actionRegisterIP   = "auth.register.ip"
	actionRefreshIP    = "auth.refresh.ip"
)

var (
	loginIPWindow      = time.Minute
	loginAccountWindow = time.Hour
	registerIPWindow   = time.Hour
	refreshIPWindow    = time.Minute

	errInvalidCredentials = apierrors.New(401, apierrors.CodeInvalidCredentials, "Email or password is incorrect.")
	errUnauthenticated    = apierrors.Unauthenticated("Authentication is required.")
	errInvalidSession     = apierrors.Unauthenticated("The session is invalid or has expired.")
)

// Limits are the per-action authentication thresholds.
type Limits struct {
	LoginPerMinuteIP    int
	LoginPerHourAccount int
	RegisterPerHourIP   int
	RefreshPerMinuteIP  int
}

// RegisterInput is a validated registration request.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName *string
}

// LoginInput is a validated login request.
type LoginInput struct {
	Email    string
	Password string
}

// AuthResult is returned by register, login, and refresh.
type AuthResult struct {
	User         *User
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// Service implements authentication and session lifecycle.
type Service struct {
	repo       *Repository
	tokens     *TokenManager
	passwords  *PasswordHasher
	limiter    *ratelimit.Limiter
	limits     Limits
	refreshTTL time.Duration
	log        *slog.Logger
}

// NewService creates the authentication service.
func NewService(
	repo *Repository,
	tokens *TokenManager,
	passwords *PasswordHasher,
	limiter *ratelimit.Limiter,
	limits Limits,
	refreshTTL time.Duration,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}

	return &Service{
		repo:       repo,
		tokens:     tokens,
		passwords:  passwords,
		limiter:    limiter,
		limits:     limits,
		refreshTTL: refreshTTL,
		log:        log,
	}
}

// Register creates the account, profile, and first session atomically.
func (s *Service) Register(ctx context.Context, clientIP string, input RegisterInput) (*AuthResult, error) {
	email := normalizeEmail(input.Email)

	if err := s.enforce(ctx, actionRegisterIP, clientIP, s.limits.RegisterPerHourIP, registerIPWindow); err != nil {
		return nil, err
	}

	passwordHash, err := s.passwords.Hash(input.Password)
	if err != nil {
		return nil, err
	}

	userID, err := ids.New()
	if err != nil {
		return nil, err
	}
	profileID, err := ids.New()
	if err != nil {
		return nil, err
	}
	sessionID, err := ids.New()
	if err != nil {
		return nil, err
	}
	familyID, err := ids.New()
	if err != nil {
		return nil, err
	}

	refreshToken, refreshHash, err := NewRefreshToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	user := &User{
		ID:           userID,
		Email:        email,
		PasswordHash: passwordHash,
		Status:       UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	session := &Session{
		ID:               sessionID,
		UserID:           userID,
		TokenFamilyID:    familyID,
		RefreshTokenHash: refreshHash,
		ExpiresAt:        now.Add(s.refreshTTL),
		CreatedAt:        now,
	}

	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.repo.CreateUser(tx, user); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return apierrors.Conflict("An account with this email already exists.")
			}
			return err
		}
		if err := s.repo.CreateProfile(tx, profileID, userID, input.DisplayName); err != nil {
			return err
		}
		return s.repo.CreateSessionTx(tx, session)
	})
	if err != nil {
		return nil, err
	}

	accessToken, expiresIn, err := s.tokens.Issue(userID, sessionID)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "user registered",
		slog.String("request_id", requestid.From(ctx)),
		slog.String("user_id", userID),
	)

	return &AuthResult{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

// Login verifies credentials and creates a new refresh-token family.
func (s *Service) Login(ctx context.Context, clientIP string, input LoginInput) (*AuthResult, error) {
	email := normalizeEmail(input.Email)

	if err := s.checkLoginLimits(ctx, clientIP, email); err != nil {
		return nil, err
	}

	user, err := s.repo.FindUserByEmail(ctx, email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.passwords.VerifyDummy(input.Password)
		return nil, errInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	valid := s.passwords.Verify(input.Password, user.PasswordHash)
	if !valid || user.Status != UserStatusActive || user.DeletedAt != nil {
		return nil, errInvalidCredentials
	}

	sessionID, err := ids.New()
	if err != nil {
		return nil, err
	}
	familyID, err := ids.New()
	if err != nil {
		return nil, err
	}

	refreshToken, refreshHash, err := NewRefreshToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := &Session{
		ID:               sessionID,
		UserID:           user.ID,
		TokenFamilyID:    familyID,
		RefreshTokenHash: refreshHash,
		ExpiresAt:        now.Add(s.refreshTTL),
		CreatedAt:        now,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	accessToken, expiresIn, err := s.tokens.Issue(user.ID, sessionID)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "user logged in",
		slog.String("request_id", requestid.From(ctx)),
		slog.String("user_id", user.ID),
	)

	return &AuthResult{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

// Refresh rotates the refresh token and revokes the family on reuse.
func (s *Service) Refresh(ctx context.Context, clientIP, rawToken string) (*AuthResult, error) {
	if err := s.enforce(ctx, actionRefreshIP, clientIP, s.limits.RefreshPerMinuteIP, refreshIPWindow); err != nil {
		return nil, err
	}

	refreshHash := HashRefreshToken(rawToken)

	var (
		userResult     *User
		accessToken    string
		refreshToken   string
		expiresIn      int
		invalidSession bool
	)

	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		old, err := s.repo.SessionForUpdate(tx, refreshHash)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			invalidSession = true
			return nil
		}
		if err != nil {
			return err
		}

		now := time.Now().UTC()

		if old.RevokedAt != nil || old.ReplacedBy != nil {
			if err := s.repo.RevokeFamily(tx, old.TokenFamilyID); err != nil {
				return err
			}
			s.log.WarnContext(ctx, "refresh token reuse detected",
				slog.String("request_id", requestid.From(ctx)),
				slog.String("user_id", old.UserID),
			)
			invalidSession = true
			return nil
		}

		if !old.ExpiresAt.After(now) {
			if err := s.repo.RevokeSessionTx(tx, old.ID); err != nil {
				return err
			}
			invalidSession = true
			return nil
		}

		user, err := s.repo.FindUserByIDTx(tx, old.UserID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.repo.RevokeFamily(tx, old.TokenFamilyID); err != nil {
				return err
			}
			invalidSession = true
			return nil
		}
		if err != nil {
			return err
		}
		if user.Status != UserStatusActive || user.DeletedAt != nil {
			if err := s.repo.RevokeFamily(tx, old.TokenFamilyID); err != nil {
				return err
			}
			invalidSession = true
			return nil
		}

		userResult = user

		newSessionID, err := ids.New()
		if err != nil {
			return err
		}

		newRefreshToken, newRefreshHash, err := NewRefreshToken()
		if err != nil {
			return err
		}

		newSession := &Session{
			ID:               newSessionID,
			UserID:           old.UserID,
			TokenFamilyID:    old.TokenFamilyID,
			RefreshTokenHash: newRefreshHash,
			ExpiresAt:        now.Add(s.refreshTTL),
			CreatedAt:        now,
		}

		if err := s.repo.CreateSessionTx(tx, newSession); err != nil {
			return err
		}
		// Insert the replacement before pointing replaced_by at it so the
		// self-referencing foreign key is satisfied.
		if err := s.repo.MarkReplaced(tx, old.ID, newSessionID); err != nil {
			return err
		}

		token, ttl, err := s.tokens.Issue(old.UserID, newSessionID)
		if err != nil {
			return err
		}

		accessToken = token
		refreshToken = newRefreshToken
		expiresIn = ttl
		return nil
	})
	if err != nil {
		return nil, err
	}
	if invalidSession {
		return nil, errInvalidSession
	}

	s.log.InfoContext(ctx, "session refreshed",
		slog.String("request_id", requestid.From(ctx)),
		slog.String("user_id", userResult.ID),
	)

	return &AuthResult{
		User:         userResult,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

// Logout revokes the current session.
func (s *Service) Logout(ctx context.Context, principal authn.Principal) error {
	if err := s.repo.RevokeSession(ctx, principal.SessionID); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "session revoked",
		slog.String("request_id", requestid.From(ctx)),
		slog.String("user_id", principal.UserID),
	)

	return nil
}

// ParseAccessToken validates an access token.
func (s *Service) ParseAccessToken(raw string) (*Claims, error) {
	return s.tokens.Parse(raw)
}

// Authenticate validates the session in the database on every request.
func (s *Service) Authenticate(ctx context.Context, claims *Claims) (authn.Principal, error) {
	principal, err := s.repo.SessionPrincipal(ctx, claims.SessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return authn.Principal{}, errUnauthenticated
	}
	if err != nil {
		return authn.Principal{}, err
	}

	if principal.UserID != claims.Subject ||
		principal.RevokedAt != nil ||
		!principal.ExpiresAt.After(time.Now().UTC()) ||
		principal.Status != UserStatusActive ||
		principal.DeletedAt != nil {
		return authn.Principal{}, errUnauthenticated
	}

	return authn.Principal{UserID: principal.UserID, SessionID: principal.SessionID}, nil
}

func (s *Service) checkLoginLimits(ctx context.Context, clientIP, email string) error {
	ipDecision, err := s.limiter.Check(ctx, actionLoginIP, clientIP, s.limits.LoginPerMinuteIP, loginIPWindow)
	if err != nil {
		return err
	}

	accountDecision, err := s.limiter.Check(ctx, actionLoginAccount, email, s.limits.LoginPerHourAccount, loginAccountWindow)
	if err != nil {
		return err
	}

	if !ipDecision.Allowed || !accountDecision.Allowed {
		retryAfter := ipDecision.RetryAfter
		if accountDecision.RetryAfter > retryAfter {
			retryAfter = accountDecision.RetryAfter
		}
		return apierrors.RateLimited(retryAfter)
	}

	if err := s.limiter.Record(ctx, actionLoginIP, clientIP); err != nil {
		return err
	}
	return s.limiter.Record(ctx, actionLoginAccount, email)
}

func (s *Service) enforce(ctx context.Context, action, subject string, limit int, window time.Duration) error {
	decision, err := s.limiter.Check(ctx, action, subject, limit, window)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return apierrors.RateLimited(decision.RetryAfter)
	}
	return s.limiter.Record(ctx, action, subject)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
