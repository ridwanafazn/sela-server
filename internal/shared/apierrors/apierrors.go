// Package apierrors defines the canonical API error codes and the error type
// used across HTTP, application, and repository boundaries.
package apierrors

import (
	"errors"
	"net/http"
	"time"
)

// Canonical error codes from the API contract. Clients branch on these
// values, never on human-readable messages.
const (
	CodeValidation             = "VALIDATION_ERROR"
	CodeUnauthenticated        = "UNAUTHENTICATED"
	CodeInvalidCredentials     = "INVALID_CREDENTIALS"
	CodeEmailNotVerified       = "EMAIL_NOT_VERIFIED"
	CodeInvalidToken           = "INVALID_TOKEN"
	CodeNotFound               = "NOT_FOUND"
	CodeConflict               = "CONFLICT"
	CodeVersionConflict        = "VERSION_CONFLICT"
	CodeIDempotencyKeyReused   = "IDEMPOTENCY_KEY_REUSED"
	CodeAccountDeletionPending = "ACCOUNT_DELETION_PENDING"
	CodePreconditionRequired   = "PRECONDITION_REQUIRED"
	CodePayloadTooLarge        = "PAYLOAD_TOO_LARGE"
	CodeUnsupportedMediaType   = "UNSUPPORTED_MEDIA_TYPE"
	CodeRateLimited            = "RATE_LIMITED"
	CodeInternal               = "INTERNAL_ERROR"
	CodeUpstreamUnavailable    = "UPSTREAM_UNAVAILABLE"
	CodeUpstreamTimeout        = "UPSTREAM_TIMEOUT"
)

// Detail describes a single field-level problem. Field is omitted when the
// problem is not tied to a specific field.
type Detail struct {
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error is a safe, client-facing API error. It intentionally carries no
// internal cause; wrap internal failures separately when logging.
type Error struct {
	Status     int
	Code       string
	Message    string
	Details    []Detail
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

// New creates an API error with the supplied status, code, and message.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Validation creates a 400 validation error.
func Validation(message string, details ...Detail) *Error {
	return &Error{
		Status:  http.StatusBadRequest,
		Code:    CodeValidation,
		Message: message,
		Details: details,
	}
}

// Unauthenticated creates a 401 error for missing or invalid credentials.
func Unauthenticated(message string) *Error {
	return New(http.StatusUnauthorized, CodeUnauthenticated, message)
}

// EmailNotVerified creates a 403 error for an account that must verify its
// email before signing in.
func EmailNotVerified() *Error {
	return New(http.StatusForbidden, CodeEmailNotVerified, "Verify your email before signing in.")
}

// InvalidToken creates a 400 error for an expired or invalid single-use token.
func InvalidToken() *Error {
	return New(http.StatusBadRequest, CodeInvalidToken, "The token is invalid or has expired.")
}

// NotFound creates a 404 error. It is also used for resources owned by
// another user so their existence is not revealed.
func NotFound(message string) *Error {
	return New(http.StatusNotFound, CodeNotFound, message)
}

// Conflict creates a 409 state conflict.
func Conflict(message string) *Error {
	return New(http.StatusConflict, CodeConflict, message)
}

// VersionConflict creates a 409 optimistic-concurrency conflict.
func VersionConflict(message string) *Error {
	return New(http.StatusConflict, CodeVersionConflict, message)
}

// IdempotencyKeyReused creates a 409 conflict for a reused key with a
// different request body.
func IdempotencyKeyReused() *Error {
	return New(http.StatusConflict, CodeIDempotencyKeyReused, "This idempotency key was already used with a different request.")
}

// RateLimited creates a 429 error with an optional retry hint.
func RateLimited(retryAfter time.Duration) *Error {
	err := New(http.StatusTooManyRequests, CodeRateLimited, "Too many attempts. Try again later.")
	err.RetryAfter = retryAfter
	return err
}

// PreconditionRequired creates a 428 error for a missing precondition header.
func PreconditionRequired(message string) *Error {
	return New(http.StatusPreconditionRequired, CodePreconditionRequired, message)
}

// Internal creates a safe 500 error. The underlying cause must be logged
// separately and must never be returned to the client.
func Internal() *Error {
	return New(http.StatusInternalServerError, CodeInternal, "An unexpected error occurred.")
}

// UpstreamUnavailable creates a 503 error for an unavailable dependency.
func UpstreamUnavailable(message string) *Error {
	return New(http.StatusServiceUnavailable, CodeUpstreamUnavailable, message)
}

// BadGateway creates a 502 error for an upstream provider failure.
func BadGateway(message string) *Error {
	return New(http.StatusBadGateway, CodeUpstreamUnavailable, message)
}

// UpstreamTimeout creates a 504 error for a dependency timeout.
func UpstreamTimeout() *Error {
	return New(http.StatusGatewayTimeout, CodeUpstreamTimeout, "The upstream service did not respond in time.")
}

// From converts err into an *Error. Unknown errors become a safe internal
// error so internal details are never exposed.
func From(err error) *Error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return Internal()
}
