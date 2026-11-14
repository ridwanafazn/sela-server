package conversation

import "time"

// Message roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Conversation is a Companion session.
type Conversation struct {
	ID        string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID    string     `gorm:"column:user_id;type:uuid"`
	Title     *string    `gorm:"column:title"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

// TableName pins the table name.
func (Conversation) TableName() string {
	return "conversations"
}

// Message is one conversation turn.
type Message struct {
	ID             string    `gorm:"column:id;primaryKey;type:uuid"`
	ConversationID string    `gorm:"column:conversation_id;type:uuid"`
	UserID         string    `gorm:"column:user_id;type:uuid"`
	Role           string    `gorm:"column:role"`
	Content        string    `gorm:"column:content"`
	IdempotencyKey *string   `gorm:"column:idempotency_key;type:uuid"`
	Metadata       []byte    `gorm:"column:metadata"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (Message) TableName() string {
	return "messages"
}

// IdempotencyRecord is a stored replayable response.
type IdempotencyRecord struct {
	ID             string    `gorm:"column:id;primaryKey;type:uuid"`
	UserID         string    `gorm:"column:user_id;type:uuid"`
	Key            string    `gorm:"column:key"`
	Endpoint       string    `gorm:"column:endpoint"`
	RequestHash    string    `gorm:"column:request_hash"`
	ResponseStatus *int      `gorm:"column:response_status"`
	ResponseBody   []byte    `gorm:"column:response_body"`
	JobID          *string   `gorm:"column:job_id;type:uuid"`
	ExpiresAt      time.Time `gorm:"column:expires_at"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (IdempotencyRecord) TableName() string {
	return "idempotency_keys"
}

// ConversationView is the API representation of a conversation.
type ConversationView struct {
	ID        string    `json:"id"`
	Title     *string   `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MessageView is the API representation of a message.
type MessageView struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// SendPayload is the response for a Companion message exchange.
type SendPayload struct {
	UserMessage      MessageView `json:"user_message"`
	AssistantMessage MessageView `json:"assistant_message"`
}

// ConversationPage is a page of conversations.
type ConversationPage struct {
	Items      []ConversationView
	NextCursor *string
	HasMore    bool
}

// MessagePage is a page of messages.
type MessagePage struct {
	Items      []MessageView
	NextCursor *string
	HasMore    bool
}

func newConversationView(conversation *Conversation) ConversationView {
	return ConversationView{
		ID:        conversation.ID,
		Title:     conversation.Title,
		CreatedAt: conversation.CreatedAt,
		UpdatedAt: conversation.UpdatedAt,
	}
}

func newMessageView(message *Message) MessageView {
	return MessageView{
		ID:        message.ID,
		Role:      message.Role,
		Content:   message.Content,
		CreatedAt: message.CreatedAt,
	}
}
