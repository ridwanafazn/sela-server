package journal

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	entryDate := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	id := uuid.NewString()

	decodedDate, decodedID, err := decodeCursor(encodeCursor(entryDate, id))
	if err != nil {
		t.Fatalf("decodeCursor returned error: %v", err)
	}
	if !decodedDate.Equal(entryDate) {
		t.Errorf("date = %s, want %s", decodedDate, entryDate)
	}
	if decodedID != id {
		t.Errorf("id = %s, want %s", decodedID, id)
	}
}

func TestCursorRejectsInvalidValues(t *testing.T) {
	validID := uuid.NewString()

	tests := []string{
		"",
		"not-base64",
		base64.RawURLEncoding.EncodeToString([]byte("not json")),
		base64.RawURLEncoding.EncodeToString([]byte(`{"d":"03-11-2026","i":"` + validID + `"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"d":"2026-11-03","i":"not-a-uuid"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{}`)),
	}

	for _, raw := range tests {
		if _, _, err := decodeCursor(raw); err == nil {
			t.Errorf("decodeCursor accepted invalid cursor %q", raw)
		}
	}
}
