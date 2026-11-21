package journal

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var errInvalidCursor = errors.New("invalid cursor")

type cursorPayload struct {
	EntryDate string `json:"d"`
	ID        string `json:"i"`
}

type changeCursorPayload struct {
	UpdatedAt string `json:"t"`
	ID        string `json:"i"`
}

func encodeCursor(entryDate time.Time, id string) string {
	payload, _ := json.Marshal(cursorPayload{
		EntryDate: entryDate.Format("2006-01-02"),
		ID:        id,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func encodeChangeCursor(updatedAt time.Time, id string) string {
	payload, _ := json.Marshal(changeCursorPayload{
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339Nano),
		ID:        id,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeChangeCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	var payload changeCursorPayload
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

func decodeCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	var payload cursorPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	entryDate, err := time.Parse("2006-01-02", payload.EntryDate)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	if _, err := uuid.Parse(payload.ID); err != nil {
		return time.Time{}, "", errInvalidCursor
	}

	return entryDate, payload.ID, nil
}
