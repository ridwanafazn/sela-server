package journal

import (
	"errors"
	"strings"
	"testing"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/optional"
)

func TestValidateCreate(t *testing.T) {
	longTitle := strings.Repeat("a", 201)
	validDate := "2026-11-03"
	draft := StatusDraft

	tests := []struct {
		name  string
		input CreateInput
		field string
	}{
		{
			name:  "title too long",
			input: CreateInput{Title: &longTitle, Content: "content"},
			field: "title",
		},
		{
			name:  "empty content",
			input: CreateInput{Content: ""},
			field: "content",
		},
		{
			name:  "invalid entry date",
			input: CreateInput{Content: "content", EntryDate: stringPointer("03-11-2026")},
			field: "entry_date",
		},
		{
			name:  "invalid status",
			input: CreateInput{Content: "content", Status: stringPointer("published")},
			field: "status",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCreate(test.input)
			requireValidationField(t, err, test.field)
		})
	}

	if err := validateCreate(CreateInput{
		Title:     stringPointer(strings.Repeat("a", 200)),
		Content:   "content",
		EntryDate: &validDate,
		Status:    &draft,
	}); err != nil {
		t.Fatalf("valid create rejected: %v", err)
	}
}

func TestValidateUpdate(t *testing.T) {
	tests := []struct {
		name  string
		input UpdateInput
		field string
	}{
		{
			name:  "title too long",
			input: UpdateInput{Title: optional.String{Set: true, Value: stringPointer(strings.Repeat("a", 201))}},
			field: "title",
		},
		{
			name:  "null content",
			input: UpdateInput{Content: optional.String{Set: true, Value: nil}},
			field: "content",
		},
		{
			name:  "empty content",
			input: UpdateInput{Content: optional.String{Set: true, Value: stringPointer("")}},
			field: "content",
		},
		{
			name:  "null entry date",
			input: UpdateInput{EntryDate: optional.String{Set: true, Value: nil}},
			field: "entry_date",
		},
		{
			name:  "invalid entry date",
			input: UpdateInput{EntryDate: optional.String{Set: true, Value: stringPointer("2026/11/03")}},
			field: "entry_date",
		},
		{
			name:  "null status",
			input: UpdateInput{Status: optional.String{Set: true, Value: nil}},
			field: "status",
		},
		{
			name:  "invalid status",
			input: UpdateInput{Status: optional.String{Set: true, Value: stringPointer("published")}},
			field: "status",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateUpdate(test.input)
			requireValidationField(t, err, test.field)
		})
	}

	if err := validateUpdate(UpdateInput{
		Title:     optional.String{Set: true, Value: nil},
		Content:   optional.String{Set: true, Value: stringPointer("updated")},
		EntryDate: optional.String{Set: true, Value: stringPointer("2026-11-04")},
		Status:    optional.String{Set: true, Value: stringPointer(StatusActive)},
	}); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}
}

func TestCreateRequestHashIsStable(t *testing.T) {
	title := "Title"
	date := "2026-11-03"
	status := StatusDraft
	input := CreateInput{Title: &title, Content: "content", EntryDate: &date, Status: &status}

	first := createRequestHash(input)
	second := createRequestHash(input)
	if first != second {
		t.Error("same request produced different hashes")
	}

	changed := input
	changed.Content = "different"
	if createRequestHash(changed) == first {
		t.Error("different request produced the same hash")
	}
}

func requireValidationField(t *testing.T, err error, field string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *apierrors.Error", err)
	}
	if apiErr.Status != 400 || apiErr.Code != apierrors.CodeValidation {
		t.Errorf("status/code = %d/%s, want 400/VALIDATION_ERROR", apiErr.Status, apiErr.Code)
	}
	for _, detail := range apiErr.Details {
		if detail.Field == field {
			return
		}
	}
	t.Errorf("details = %+v, want field %s", apiErr.Details, field)
}

func stringPointer(value string) *string {
	return &value
}
