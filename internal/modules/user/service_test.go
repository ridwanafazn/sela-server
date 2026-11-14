package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

type fakeStore struct {
	me      *Me
	err     error
	updated bool
	patch   ProfilePatch
}

func (f *fakeStore) GetMe(context.Context, string) (*Me, error) {
	return f.me, f.err
}

func (f *fakeStore) UpdateMe(_ context.Context, _ string, patch ProfilePatch) (*Me, error) {
	f.updated = true
	f.patch = patch
	return f.me, f.err
}

func testMe() *Me {
	return &Me{
		UserID:             "user-1",
		Email:              "user@example.com",
		DisplayName:        stringPointer("Ridwan"),
		Language:           "id",
		Timezone:           "Asia/Jakarta",
		CommunicationStyle: StyleAdaptive,
	}
}

func TestUpdateMeAppliesValidPatch(t *testing.T) {
	store := &fakeStore{me: testMe()}
	service := NewService(store)

	patch := ProfilePatch{
		DisplayName:        OptionalString{Set: true, Value: nil},
		Language:           OptionalString{Set: true, Value: stringPointer(" en-US ")},
		Timezone:           OptionalString{Set: true, Value: stringPointer("Asia/Makassar")},
		CommunicationStyle: OptionalString{Set: true, Value: stringPointer(StyleDirect)},
	}

	if _, err := service.UpdateMe(context.Background(), "user-1", patch); err != nil {
		t.Fatalf("UpdateMe returned error: %v", err)
	}
	if !store.updated {
		t.Fatal("store was not updated")
	}
	if store.patch.DisplayName.Value != nil {
		t.Error("display_name was not cleared")
	}
	if store.patch.Language.Value == nil || *store.patch.Language.Value != "en-US" {
		t.Errorf("language = %v, want trimmed en-US", store.patch.Language.Value)
	}
	if store.patch.Timezone.Value == nil || *store.patch.Timezone.Value != "Asia/Makassar" {
		t.Errorf("timezone = %v, want Asia/Makassar", store.patch.Timezone.Value)
	}
	if store.patch.CommunicationStyle.Value == nil || *store.patch.CommunicationStyle.Value != StyleDirect {
		t.Errorf("style = %v, want direct", store.patch.CommunicationStyle.Value)
	}
}

func TestUpdateMeNoopReadsProfile(t *testing.T) {
	store := &fakeStore{me: testMe()}
	service := NewService(store)

	if _, err := service.UpdateMe(context.Background(), "user-1", ProfilePatch{}); err != nil {
		t.Fatalf("UpdateMe returned error: %v", err)
	}
	if store.updated {
		t.Error("empty patch triggered an update")
	}
}

func TestUpdateMeRejectsInvalidValues(t *testing.T) {
	longName := strings.Repeat("a", 101)

	tests := []struct {
		name  string
		patch ProfilePatch
		field string
	}{
		{
			name:  "display name too long",
			patch: ProfilePatch{DisplayName: OptionalString{Set: true, Value: &longName}},
			field: "display_name",
		},
		{
			name:  "invalid language",
			patch: ProfilePatch{Language: OptionalString{Set: true, Value: stringPointer("indonesian")}},
			field: "language",
		},
		{
			name:  "null language",
			patch: ProfilePatch{Language: OptionalString{Set: true, Value: nil}},
			field: "language",
		},
		{
			name:  "invalid timezone",
			patch: ProfilePatch{Timezone: OptionalString{Set: true, Value: stringPointer("Mars/Olympus")}},
			field: "timezone",
		},
		{
			name:  "null timezone",
			patch: ProfilePatch{Timezone: OptionalString{Set: true, Value: nil}},
			field: "timezone",
		},
		{
			name:  "invalid style",
			patch: ProfilePatch{CommunicationStyle: OptionalString{Set: true, Value: stringPointer("harsh")}},
			field: "communication_style",
		},
		{
			name:  "null style",
			patch: ProfilePatch{CommunicationStyle: OptionalString{Set: true, Value: nil}},
			field: "communication_style",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStore{me: testMe()}
			service := NewService(store)

			_, err := service.UpdateMe(context.Background(), "user-1", test.patch)
			if err == nil {
				t.Fatal("UpdateMe accepted an invalid patch")
			}

			var apiErr *apierrors.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error type = %T, want *apierrors.Error", err)
			}
			if apiErr.Status != 400 || apiErr.Code != apierrors.CodeValidation {
				t.Errorf("status/code = %d/%s, want 400/VALIDATION_ERROR", apiErr.Status, apiErr.Code)
			}
			if len(apiErr.Details) == 0 || apiErr.Details[0].Field != test.field {
				t.Errorf("details = %+v, want field %s", apiErr.Details, test.field)
			}
			if store.updated {
				t.Error("invalid patch reached the store")
			}
		})
	}
}
