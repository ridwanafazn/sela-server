package journal

import "github.com/ridwanafazn/sela-server/internal/shared/optional"

type createRequest struct {
	Title     *string `json:"title" binding:"omitempty,max=200"`
	Content   string  `json:"content" binding:"required,min=1"`
	EntryDate *string `json:"entry_date"`
	Status    *string `json:"status"`
}

type updateRequest struct {
	Title     optional.String `json:"title"`
	Content   optional.String `json:"content"`
	EntryDate optional.String `json:"entry_date"`
	Status    optional.String `json:"status"`
}
