// Package response writes the canonical success and error envelopes.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/requestid"
)

// Pagination is the opaque cursor pagination metadata.
type Pagination struct {
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

type dataEnvelope struct {
	Data any `json:"data"`
}

type collectionEnvelope struct {
	Data       any        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type errorEnvelope struct {
	Error errorObject `json:"error"`
}

type errorObject struct {
	Code      string             `json:"code"`
	Message   string             `json:"message"`
	Details   []apierrors.Detail `json:"details"`
	RequestID *string            `json:"request_id"`
}

// Data writes a single-resource success response.
func Data(c *gin.Context, status int, data any) {
	c.JSON(status, dataEnvelope{Data: data})
}

// Collection writes a paginated collection response.
func Collection(c *gin.Context, data any, pagination Pagination) {
	c.JSON(http.StatusOK, collectionEnvelope{Data: data, Pagination: pagination})
}

// Error writes a canonical error envelope. Internal errors are not exposed;
// unknown errors are converted to a safe 500 response.
func Error(c *gin.Context, err error) {
	apiErr := apierrors.From(err)

	details := apiErr.Details
	if details == nil {
		details = []apierrors.Detail{}
	}

	var requestIDValue *string
	if value := requestid.From(c.Request.Context()); value != "" {
		requestIDValue = &value
	}

	c.AbortWithStatusJSON(apiErr.Status, errorEnvelope{
		Error: errorObject{
			Code:      apiErr.Code,
			Message:   apiErr.Message,
			Details:   details,
			RequestID: requestIDValue,
		},
	})
}

// NoContent writes an empty 204 response.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}
