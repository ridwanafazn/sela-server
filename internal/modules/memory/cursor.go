package memory

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var errInvalidCursor = errors.New("invalid cursor")

type cursorPayload struct {
	UpdatedAt string `json:"u"`
	ID        string `json:"i"`
}

func encodeCursor(updatedAt time.Time, id string) string {
	payload, _ := json.Marshal(cursorPayload{
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339Nano),
		ID:        id,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	var payload cursorPayload
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
