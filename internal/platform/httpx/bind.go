package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// BodyLimit caps request bodies and returns the canonical error envelopes
// for oversized payloads.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}

	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			response.Error(c, apierrors.New(
				http.StatusRequestEntityTooLarge,
				apierrors.CodePayloadTooLarge,
				"The request body is too large.",
			))
			c.Abort()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// BindJSON decodes and validates a JSON request body. On failure it writes the
// canonical error response and returns false.
func BindJSON(c *gin.Context, dst any) bool {
	if c.ContentType() != "application/json" {
		response.Error(c, apierrors.New(
			http.StatusUnsupportedMediaType,
			apierrors.CodeUnsupportedMediaType,
			"Content-Type must be application/json.",
		))
		return false
	}

	if c.Request.ContentLength == 0 {
		response.Error(c, apierrors.Validation("The request body is required."))
		return false
	}

	if err := c.ShouldBindJSON(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.Error(c, apierrors.New(
				http.StatusRequestEntityTooLarge,
				apierrors.CodePayloadTooLarge,
				"The request body is too large.",
			))
			return false
		}

		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			details := make([]apierrors.Detail, 0, len(validationErrs))
			for _, fieldErr := range validationErrs {
				details = append(details, apierrors.Detail{
					Field:   jsonFieldName(fieldErr.Field()),
					Code:    strings.ToUpper(fieldErr.Tag()),
					Message: validationMessage(fieldErr),
				})
			}
			response.Error(c, apierrors.Validation("The request is invalid.", details...))
			return false
		}

		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			response.Error(c, apierrors.Validation(
				"The request is invalid.",
				apierrors.Detail{
					Field:   typeErr.Field,
					Code:    "INVALID_TYPE",
					Message: "This value has the wrong type.",
				},
			))
			return false
		}

		if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "unexpected end of JSON input") {
			response.Error(c, apierrors.Validation("The request body is required."))
			return false
		}

		response.Error(c, apierrors.Validation("The request body is not valid JSON."))
		return false
	}

	return true
}

func validationMessage(fieldErr validator.FieldError) string {
	switch fieldErr.Tag() {
	case "required":
		return "This field is required."
	case "email":
		return "Enter a valid email address."
	case "min":
		return fmt.Sprintf("Must be at least %s characters.", fieldErr.Param())
	case "max":
		return fmt.Sprintf("Must be at most %s characters.", fieldErr.Param())
	case "oneof":
		return fmt.Sprintf("Must be one of: %s.", fieldErr.Param())
	default:
		return "This value is invalid."
	}
}

// jsonFieldName converts a Go struct field name to its conventional JSON
// snake_case name. Request DTOs keep JSON names aligned with this convention.
func jsonFieldName(field string) string {
	var builder strings.Builder
	builder.Grow(len(field) + 4)

	for index, char := range field {
		if char >= 'A' && char <= 'Z' {
			if index > 0 {
				builder.WriteByte('_')
			}
			builder.WriteByte(byte(char - 'A' + 'a'))
			continue
		}
		builder.WriteRune(char)
	}

	return builder.String()
}
