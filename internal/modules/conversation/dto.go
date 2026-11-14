package conversation

type createConversationRequest struct {
	Title *string `json:"title" binding:"omitempty,max=200"`
}

type sendMessageRequest struct {
	Content string `json:"content" binding:"required,min=1,max=20000"`
}
