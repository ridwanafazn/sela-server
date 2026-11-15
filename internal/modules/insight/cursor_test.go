package insight

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, 11, 15, 9, 30, 0, 12, time.UTC)
	id := uuid.NewString()

	decodedTime, decodedID, err := decodeCursor(encodeCursor(createdAt, id))
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !decodedTime.Equal(createdAt) {
		t.Errorf("time = %s, want %s", decodedTime, createdAt)
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
