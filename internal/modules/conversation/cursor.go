package conversation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var errInvalidCursor = errors.New("invalid cursor")

type conversationCursor struct {
	UpdatedAt string `json:"u"`
	ID        string `json:"i"`
}

type messageCursor struct {
	CreatedAt string `json:"c"`
	ID        string `json:"i"`
}

func encodeConversationCursor(updatedAt time.Time, id string) string {
	payload, _ := json.Marshal(conversationCursor{
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339Nano),
		ID:        id,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeConversationCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	var payload conversationCursor
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	updatedAt, err := time.Parse(time.RFC3339Nano, payload.UpdatedAt)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	if _, err := uuid.Parse(payload.ID); err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	return updatedAt, payload.ID, nil
}

func encodeMessageCursor(createdAt time.Time, id string) string {
	payload, _ := json.Marshal(messageCursor{
		CreatedAt: createdAt.UTC().Format(time.RFC3339Nano),
		ID:        id,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeMessageCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	var payload messageCursor
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	if _, err := uuid.Parse(payload.ID); err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	return createdAt, payload.ID, nil
}
