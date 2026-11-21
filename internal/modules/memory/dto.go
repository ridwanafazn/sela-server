package memory

import "github.com/ridwanafazn/sela-server/internal/shared/optional"

type updateMemoryRequest struct {
	Content  *string         `json:"content"`
	Category optional.String `json:"category"`
}

type feedbackRequest struct {
	Action           string  `json:"action" binding:"required,oneof=confirm correct reject"`
	CorrectedContent *string `json:"corrected_content"`
}

type consentRequest struct {
	Granted *bool `json:"granted" binding:"required"`
}
