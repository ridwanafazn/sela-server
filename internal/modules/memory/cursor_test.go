package memory

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	updatedAt := time.Date(2026, 11, 15, 10, 30, 0, 42, time.UTC)
	id := uuid.NewString()

	decodedTime, decodedID, err := decodeCursor(encodeCursor(updatedAt, id))
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !decodedTime.Equal(updatedAt) {
		t.Errorf("time = %s, want %s", decodedTime, updatedAt)
	}
	if decodedID != id {
		t.Errorf("id = %s, want %s", decodedID, id)
	}
}

func TestCursorRejectsInvalidValues(t *testing.T) {
	if _, _, err := decodeCursor("not-base64"); err == nil {
		t.Error("cursor accepted invalid base64")
	}
}
