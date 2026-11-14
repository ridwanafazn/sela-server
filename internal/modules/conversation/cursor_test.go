package conversation

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConversationCursorRoundTrip(t *testing.T) {
	updatedAt := time.Date(2026, 11, 15, 10, 30, 0, 123, time.UTC)
	id := uuid.NewString()

	decodedTime, decodedID, err := decodeConversationCursor(encodeConversationCursor(updatedAt, id))
	if err != nil {
		t.Fatalf("decodeConversationCursor: %v", err)
	}
	if !decodedTime.Equal(updatedAt) {
		t.Errorf("time = %s, want %s", decodedTime, updatedAt)
	}
	if decodedID != id {
		t.Errorf("id = %s, want %s", decodedID, id)
	}
}

func TestMessageCursorRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, 11, 15, 10, 30, 0, 0, time.UTC)
	id := uuid.NewString()

	decodedTime, decodedID, err := decodeMessageCursor(encodeMessageCursor(createdAt, id))
	if err != nil {
		t.Fatalf("decodeMessageCursor: %v", err)
	}
	if !decodedTime.Equal(createdAt) {
		t.Errorf("time = %s, want %s", decodedTime, createdAt)
	}
	if decodedID != id {
		t.Errorf("id = %s, want %s", decodedID, id)
	}
}

func TestCursorsRejectInvalidValues(t *testing.T) {
	if _, _, err := decodeConversationCursor("not-base64"); err == nil {
		t.Error("conversation cursor accepted invalid base64")
	}
	if _, _, err := decodeMessageCursor("not-base64"); err == nil {
		t.Error("message cursor accepted invalid base64")
	}
}
